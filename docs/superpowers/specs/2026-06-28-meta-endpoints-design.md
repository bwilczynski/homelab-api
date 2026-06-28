# Meta Endpoints Design

**Date:** 2026-06-28
**Status:** Approved

## Problem

Two endpoints exist outside the contract-first approach:

- `GET /.well-known/homelab` — returns auth configuration (`enabled`, `issuer`)
- `GET /version` — returns version strings (`apiVersion`, `serverVersion`)

Both are hardcoded as inline handler closures in `cmd/server/main.go`. No OpenAPI spec definition exists for either. Existing clients depend on these paths.

## Goal

Bring both endpoints under the contract-first approach by defining them in the OpenAPI spec under a new `meta` group. Existing clients must not break; old paths are deprecated with standard HTTP headers pointing clients to the new canonical paths.

## New Paths

| New path | Replaces | operationId |
|---|---|---|
| `GET /meta/version` | `GET /version` | `getMetaVersion` |
| `GET /meta/auth` | `GET /.well-known/homelab` | `getMetaAuth` |

Both endpoints are public (`security: []`) — no bearer token required. `/meta/auth` must be public because clients need it to determine auth configuration before they can authenticate.

## Delivery Sequence

The spec and implementation are in separate repositories. Changes must land in this order:

1. **Open a PR in `homelab-api-spec`** with all spec changes (tag, path files, schemas). The repo's CI runs `make lint` (Redocly + Spectral). The PR must pass all checks before merging.
2. **After the spec PR merges**, update the `spec/` submodule pointer in this repo to the new commit (`git -C spec pull && git add spec`).
3. **Run `make generate`** to regenerate stubs from the updated bundled spec.
4. **Implement and wire** the `internal/meta/` package and update `cmd/server/main.go`.
5. **Open a PR in this repo** with the submodule bump, generated stubs (excluded from commit per `.gitignore`), and implementation files.

## Spec Changes (`spec/` submodule — `homelab-api-spec` repo)

### Tag

Add `meta` tag to `openapi/openapi.yaml`:
```yaml
- name: meta
  description: API metadata — version info and auth configuration discovery.
```

### Path files

**`openapi/paths/meta-version.yaml`** — `GET /meta/version`
- operationId: `getMetaVersion`
- security: `[]`
- 200 response: `$ref: "../components/schemas/Version.yaml"`
- Also declares 401 and 500 (required by Spectral `operation-has-401` rule, even though auth is not enforced)

**`openapi/paths/meta-auth.yaml`** — `GET /meta/auth`
- operationId: `getMetaAuth`
- security: `[]`
- 200 response: `$ref: "../components/schemas/AuthDiscovery.yaml"`
- Also declares 401 and 500

### Schemas

**`openapi/components/schemas/Version.yaml`**
```yaml
type: object
properties:
  apiVersion:
    type: string
    description: Version of the OpenAPI contract this server implements.
  serverVersion:
    type: string
    description: Build version of the server binary.
required:
  - apiVersion
  - serverVersion
```

**`openapi/components/schemas/AuthDiscovery.yaml`**
```yaml
type: object
properties:
  enabled:
    type: boolean
    description: Whether bearer-token authentication is enforced.
  issuer:
    type: string
    description: OAuth 2.0 issuer URL. Omitted when auth is disabled.
required:
  - enabled
```

Both paths are registered in `openapi/openapi.yaml` under `paths:`.

## Implementation (`internal/meta/`)

### Code generation

New `oapi-codegen-meta.yaml`:
```yaml
package: meta
generate:
  chi-server: true
  strict-server: true
  models: true
output: internal/meta/api.gen.go
output-options:
  include-tags:
    - meta
```

`Makefile` `generate` target gains:
```makefile
@mkdir -p internal/meta
$(OAPI_CODEGEN) --config oapi-codegen-meta.yaml $(SPEC_FILE)
```

### `internal/meta/service.go`

`Service` holds four fields set at construction time:
- `apiVersion string`
- `serverVersion string`
- `authEnabled bool`
- `authIssuer string`

Constructor: `NewService(apiVersion, serverVersion string, authEnabled bool, authIssuer string) *Service`

Methods:
- `GetVersion() (apiVersion, serverVersion string)`
- `GetAuth() (enabled bool, issuer string)`

### `internal/meta/handler.go`

Implements the generated `StrictServerInterface`. No backend calls, no health monitor dependency — pure value pass-through from the service.

- `GetMetaVersion` → maps to generated `GetMetaVersion200JSONResponse`
- `GetMetaAuth` → maps to generated `GetMetaAuth200JSONResponse`

## Wiring (`cmd/server/main.go`)

Replace the two inline handler closures. Construct the service and mount it on the **unprotected** router `r`:

```go
metaSvc := meta.NewService(apiVersion, serverVersion, cfg.Auth.Enabled, cfg.Auth.Issuer)
meta.HandlerWithOptions(
    meta.NewStrictHandler(meta.NewHandler(metaSvc), nil),
    meta.ChiServerOptions{
        BaseRouter:       r,
        ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
    },
)
```

No `ScopeMiddleware` — these are public endpoints.

### Deprecated alias routes

Two `r.Get(...)` handlers remain for backward compatibility. Each calls the same service method and adds deprecation headers before writing the response:

```
Deprecation: true
Link: </meta/auth>; rel="successor-version"      // for /.well-known/homelab
Link: </meta/version>; rel="successor-version"   // for /version
```

Old routes are not removed — they stay indefinitely until a future breaking-change release explicitly drops them.

## Out of Scope

- Removing the old paths (deferred to a future breaking release)
- Adding auth to the meta endpoints
- Documenting the deprecated paths in the spec
