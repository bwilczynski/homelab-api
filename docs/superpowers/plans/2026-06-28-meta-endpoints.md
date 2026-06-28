# Meta Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /meta/version` and `GET /meta/auth` to the OpenAPI spec and implement them via the contract-first pattern, while keeping the old paths (`/version`, `/.well-known/homelab`) alive with deprecation headers.

**Architecture:** Spec changes go to `homelab-api-spec` as a PR first; after merge the `spec/` submodule is bumped. A new `internal/meta/` package holds the service and handler wired onto the unprotected chi router. Old paths become thin alias handlers that call the same service and add `Deprecation` / `Link` headers.

**Tech Stack:** Go 1.22+, chi, oapi-codegen (strict-server), OpenAPI 3.0.3, Redocly + Spectral for spec lint.

---

## File Map

### Phase 1 — homelab-api-spec (inside `spec/` submodule)

| Action | Path |
|--------|------|
| Create | `openapi/components/schemas/Version.yaml` |
| Create | `openapi/components/schemas/AuthDiscovery.yaml` |
| Create | `openapi/paths/meta-version.yaml` |
| Create | `openapi/paths/meta-auth.yaml` |
| Modify | `openapi/openapi.yaml` |

### Phase 2 — homelab-api (after spec PR merges)

| Action | Path |
|--------|------|
| Create | `oapi-codegen-meta.yaml` |
| Modify | `Makefile` |
| Create | `internal/meta/service.go` |
| Create | `internal/meta/service_test.go` |
| Create | `internal/meta/handler.go` |
| Modify | `cmd/server/main.go` |

---

## Phase 1 — Spec PR in homelab-api-spec

> Work inside the `spec/` directory. This is a git submodule pointing at `github.com/bwilczynski/homelab-api-spec`. All commits and the PR happen there.

---

### Task 1: Create schema files

**Files:**
- Create: `spec/openapi/components/schemas/Version.yaml`
- Create: `spec/openapi/components/schemas/AuthDiscovery.yaml`

- [ ] **Step 1: Create `Version.yaml`**

```yaml
type: object
description: API and server version information.
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

- [ ] **Step 2: Create `AuthDiscovery.yaml`**

```yaml
type: object
description: Auth configuration for this API instance.
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

- [ ] **Step 3: Commit**

```bash
git -C spec add openapi/components/schemas/Version.yaml \
                openapi/components/schemas/AuthDiscovery.yaml
git -C spec commit -m "feat: add Version and AuthDiscovery schemas for meta endpoints"
```

---

### Task 2: Create path files and register them

**Files:**
- Create: `spec/openapi/paths/meta-version.yaml`
- Create: `spec/openapi/paths/meta-auth.yaml`
- Modify: `spec/openapi/openapi.yaml`

- [ ] **Step 1: Create `meta-version.yaml`**

```yaml
get:
  operationId: getMetaVersion
  x-stability-level: stable
  summary: Get API and server version
  description: |
    Returns the version of the OpenAPI contract this server implements
    and the build version of the server binary. Public endpoint — no
    authentication required.
  tags:
    - meta
  security: []
  responses:
    "200":
      description: Version information.
      content:
        application/json:
          schema:
            $ref: "../components/schemas/Version.yaml"
    "401":
      $ref: "../components/responses/Unauthorized.yaml"
    "500":
      $ref: "../components/responses/InternalServerError.yaml"
```

- [ ] **Step 2: Create `meta-auth.yaml`**

```yaml
get:
  operationId: getMetaAuth
  x-stability-level: stable
  summary: Get auth configuration
  description: |
    Returns the auth configuration for this API instance. Clients use
    this to determine whether authentication is required and which
    issuer to use before attempting to obtain a token. Public endpoint
    — no authentication required.
  tags:
    - meta
  security: []
  responses:
    "200":
      description: Auth configuration.
      content:
        application/json:
          schema:
            $ref: "../components/schemas/AuthDiscovery.yaml"
    "401":
      $ref: "../components/responses/Unauthorized.yaml"
    "500":
      $ref: "../components/responses/InternalServerError.yaml"
```

- [ ] **Step 3: Add `meta` tag to `openapi/openapi.yaml`**

In `spec/openapi/openapi.yaml`, add to the `tags:` list (after the existing four tags):

```yaml
  - name: meta
    description: API metadata — version info and auth configuration discovery.
```

- [ ] **Step 4: Register paths in `openapi/openapi.yaml`**

Add to the `paths:` section (e.g. at the top, before `/system/health`):

```yaml
  /meta/version:
    $ref: "./paths/meta-version.yaml"
  /meta/auth:
    $ref: "./paths/meta-auth.yaml"
```

- [ ] **Step 5: Run lint to verify**

```bash
cd spec && make lint
```

Expected: no errors. Fix any lint failures before proceeding.

- [ ] **Step 6: Commit and open PR**

```bash
git -C spec add openapi/paths/meta-version.yaml \
                openapi/paths/meta-auth.yaml \
                openapi/openapi.yaml
git -C spec commit -m "feat: add meta tag with /meta/version and /meta/auth endpoints"
git -C spec push origin HEAD
```

Then open a PR in `github.com/bwilczynski/homelab-api-spec`. Wait for all CI checks to pass and merge before continuing to Phase 2.

---

## Phase 2 — Implementation in homelab-api

> Start only after the spec PR is merged.

---

### Task 3: Bump the spec submodule

**Files:**
- Modify: `spec` (submodule pointer)

- [ ] **Step 1: Pull latest spec**

```bash
git -C spec pull origin main
```

- [ ] **Step 2: Stage submodule bump and commit**

```bash
git add spec
git commit -m "chore: bump spec submodule to include meta endpoints"
```

---

### Task 4: Add oapi-codegen config and regenerate stubs

**Files:**
- Create: `oapi-codegen-meta.yaml`
- Modify: `Makefile`

- [ ] **Step 1: Create `oapi-codegen-meta.yaml`**

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

- [ ] **Step 2: Add meta to the `generate` target in `Makefile`**

Find the `generate:` target. It currently ends with the `network` invocation. Add two lines:

```makefile
	@mkdir -p internal/meta
	$(OAPI_CODEGEN) --config oapi-codegen-meta.yaml $(SPEC_FILE)
```

The full updated `generate` target:

```makefile
generate: bundle ## Generate server stubs from the bundled spec
	@mkdir -p internal/system internal/docker internal/storage internal/network internal/meta
	$(OAPI_CODEGEN) --config oapi-codegen-system.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-docker.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-storage.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-network.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-meta.yaml $(SPEC_FILE)
```

- [ ] **Step 3: Run code generation**

```bash
make generate
```

Expected: `internal/meta/api.gen.go` is created with `StrictServerInterface` containing `GetMetaVersion` and `GetMetaAuth`.

- [ ] **Step 4: Verify generated interface**

```bash
grep -n "GetMeta" internal/meta/api.gen.go
```

Expected output (exact names may vary slightly by oapi-codegen version):
```
GetMetaVersion(ctx context.Context, request GetMetaVersionRequestObject) (GetMetaVersionResponseObject, error)
GetMetaAuth(ctx context.Context, request GetMetaAuthRequestObject) (GetMetaAuthResponseObject, error)
```

- [ ] **Step 5: Commit config and Makefile (do NOT stage api.gen.go)**

```bash
git diff --cached --name-only | grep gen.go   # must print nothing
git add oapi-codegen-meta.yaml Makefile
git commit -m "chore: add oapi-codegen config for meta tag"
```

---

### Task 5: Write failing tests for the meta service

**Files:**
- Create: `internal/meta/service_test.go`

- [ ] **Step 1: Create `service_test.go`**

```go
package meta_test

import (
	"testing"

	"github.com/bwilczynski/homelab-api/internal/meta"
)

func TestGetVersion(t *testing.T) {
	svc := meta.NewService("0.1.0", "v1.2.3", false, "")

	apiVersion, serverVersion := svc.GetVersion()

	if apiVersion != "0.1.0" {
		t.Errorf("apiVersion: got %q, want %q", apiVersion, "0.1.0")
	}
	if serverVersion != "v1.2.3" {
		t.Errorf("serverVersion: got %q, want %q", serverVersion, "v1.2.3")
	}
}

func TestGetAuth_Enabled(t *testing.T) {
	svc := meta.NewService("0.1.0", "dev", true, "https://dex.example.com")

	enabled, issuer := svc.GetAuth()

	if !enabled {
		t.Error("expected enabled=true")
	}
	if issuer != "https://dex.example.com" {
		t.Errorf("issuer: got %q, want %q", issuer, "https://dex.example.com")
	}
}

func TestGetAuth_Disabled(t *testing.T) {
	svc := meta.NewService("0.1.0", "dev", false, "")

	enabled, issuer := svc.GetAuth()

	if enabled {
		t.Error("expected enabled=false")
	}
	if issuer != "" {
		t.Errorf("issuer: got %q, want empty", issuer)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/meta/ -run TestGet -v
```

Expected: compile error — `meta.NewService` undefined.

---

### Task 6: Implement the meta service

**Files:**
- Create: `internal/meta/service.go`

- [ ] **Step 1: Create `service.go`**

```go
package meta

// Service holds the static configuration values returned by meta endpoints.
type Service struct {
	apiVersion    string
	serverVersion string
	authEnabled   bool
	authIssuer    string
}

// NewService creates a Service with version and auth configuration.
func NewService(apiVersion, serverVersion string, authEnabled bool, authIssuer string) *Service {
	return &Service{
		apiVersion:    apiVersion,
		serverVersion: serverVersion,
		authEnabled:   authEnabled,
		authIssuer:    authIssuer,
	}
}

// GetVersion returns the API contract version and server build version.
func (s *Service) GetVersion() (apiVersion, serverVersion string) {
	return s.apiVersion, s.serverVersion
}

// GetAuth returns the auth configuration: whether auth is enabled and the issuer URL.
func (s *Service) GetAuth() (enabled bool, issuer string) {
	return s.authEnabled, s.authIssuer
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/meta/ -run TestGet -v
```

Expected:
```
--- PASS: TestGetVersion (0.00s)
--- PASS: TestGetAuth_Enabled (0.00s)
--- PASS: TestGetAuth_Disabled (0.00s)
PASS
```

- [ ] **Step 3: Commit**

```bash
git add internal/meta/service.go internal/meta/service_test.go
git commit -m "feat: add meta service with GetVersion and GetAuth"
```

---

### Task 7: Implement the meta handler

**Files:**
- Create: `internal/meta/handler.go`

- [ ] **Step 1: Check the exact generated type names for responses**

```bash
grep -n "200JSONResponse\|RequestObject\|ResponseObject" internal/meta/api.gen.go | head -20
```

Note the exact struct names — you need them in the handler. They should follow the pattern:
- `GetMetaVersion200JSONResponse` (struct with `ApiVersion`, `ServerVersion` fields)
- `GetMetaAuth200JSONResponse` (struct with `Enabled`, `Issuer *string` fields)
- `InternalServerErrorApplicationProblemPlusJSONResponse`

- [ ] **Step 2: Create `handler.go`**

```go
package meta

import (
	"context"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements StrictServerInterface by delegating to the Service.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new ServerHandler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func internalServerError(detail string) InternalServerErrorApplicationProblemPlusJSONResponse {
	resp := InternalServerErrorApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNInternalServerError,
		Title:  apierrors.TitleInternalServerError,
		Status: 500,
	}
	if detail != "" {
		resp.Detail = &detail
	}
	return resp
}

// GetMetaVersion implements StrictServerInterface.
func (h *ServerHandler) GetMetaVersion(ctx context.Context, _ GetMetaVersionRequestObject) (GetMetaVersionResponseObject, error) {
	apiVersion, serverVersion := h.svc.GetVersion()
	return GetMetaVersion200JSONResponse{
		ApiVersion:    apiVersion,
		ServerVersion: serverVersion,
	}, nil
}

// GetMetaAuth implements StrictServerInterface.
func (h *ServerHandler) GetMetaAuth(ctx context.Context, _ GetMetaAuthRequestObject) (GetMetaAuthResponseObject, error) {
	enabled, issuer := h.svc.GetAuth()
	resp := GetMetaAuth200JSONResponse{Enabled: enabled}
	if issuer != "" {
		resp.Issuer = &issuer
	}
	return resp, nil
}
```

> **Note:** If `make generate` produced different field names (e.g. `APIVersion` instead of `ApiVersion`), adjust to match. Always check against `api.gen.go`.

- [ ] **Step 3: Build to verify it compiles**

```bash
make build
```

Expected: `bin/server` built without errors. If the handler doesn't satisfy `StrictServerInterface`, the compiler will tell you which methods are missing or have wrong signatures.

- [ ] **Step 4: Commit**

```bash
git add internal/meta/handler.go
git commit -m "feat: add meta handler implementing StrictServerInterface"
```

---

### Task 8: Wire meta into the server and add deprecated alias routes

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add `meta` import**

In `cmd/server/main.go`, add to the import block:

```go
"github.com/bwilczynski/homelab-api/internal/meta"
```

- [ ] **Step 2: Replace the `/version` inline handler**

Remove this block from `main.go`:

```go
r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    type response struct {
        APIVersion    string `json:"apiVersion"`
        ServerVersion string `json:"serverVersion"`
    }
    _ = json.NewEncoder(w).Encode(response{
        APIVersion:    apiVersion,
        ServerVersion: serverVersion,
    })
})
```

- [ ] **Step 3: Replace the `/.well-known/homelab` inline handler**

Remove this block from `main.go`:

```go
r.Get("/.well-known/homelab", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    type response struct {
        Enabled bool   `json:"enabled"`
        Issuer  string `json:"issuer,omitempty"`
    }
    _ = json.NewEncoder(w).Encode(response{
        Enabled: cfg.Auth.Enabled,
        Issuer:  cfg.Auth.Issuer,
    })
})
```

- [ ] **Step 4: Add meta service construction and contract-first wiring**

After `discoverAPIs(synologyClients)` and before `monitor := ...`, add:

```go
metaSvc := meta.NewService(apiVersion, serverVersion, cfg.Auth.Enabled, cfg.Auth.Issuer)
```

After the router and middleware setup (after `r.Use(httplog.RequestLogger(...))`), add the contract-first handler on the **unprotected** router `r`:

```go
meta.HandlerWithOptions(
    meta.NewStrictHandler(meta.NewHandler(metaSvc), nil),
    meta.ChiServerOptions{
        BaseRouter:       r,
        ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
    },
)
```

- [ ] **Step 5: Add deprecated alias routes**

After the meta handler block, add:

```go
r.Get("/.well-known/homelab", func(w http.ResponseWriter, req *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Deprecation", "true")
    w.Header().Set("Link", `</meta/auth>; rel="successor-version"`)
    enabled, issuer := metaSvc.GetAuth()
    resp := meta.AuthDiscovery{Enabled: enabled}
    if issuer != "" {
        resp.Issuer = &issuer
    }
    _ = json.NewEncoder(w).Encode(resp)
})

r.Get("/version", func(w http.ResponseWriter, req *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Deprecation", "true")
    w.Header().Set("Link", `</meta/version>; rel="successor-version"`)
    apiVer, serverVer := metaSvc.GetVersion()
    _ = json.NewEncoder(w).Encode(meta.Version{
        ApiVersion:    apiVer,
        ServerVersion: serverVer,
    })
})
```

> **Note:** `meta.AuthDiscovery` and `meta.Version` are the types generated by oapi-codegen from the spec schemas. Adjust field names if `make generate` produced different names (check `api.gen.go`).

- [ ] **Step 6: Remove unused `encoding/json` import if now unused in main.go**

```bash
go build ./cmd/server/
```

If you see `"encoding/json" imported and not used`, remove it from the import block. The `json.NewEncoder` calls in the deprecated alias handlers keep it in use — it should not be needed to remove it.

- [ ] **Step 7: Build and run tests**

```bash
make build && make test
```

Expected: binary builds, all tests pass.

- [ ] **Step 8: Smoke test the endpoints manually**

```bash
bin/server &
SERVER_PID=$!

curl -s http://localhost:8080/meta/version | jq .
# Expected: {"apiVersion":"0.1.0","serverVersion":"dev"}

curl -s http://localhost:8080/meta/auth | jq .
# Expected: {"enabled":false} (or with issuer if auth is configured)

curl -si http://localhost:8080/version | head -10
# Expected: HTTP 200 with Deprecation: true and Link headers, same JSON body

curl -si http://localhost:8080/.well-known/homelab | head -10
# Expected: HTTP 200 with Deprecation: true and Link headers, same JSON body

kill $SERVER_PID
```

- [ ] **Step 9: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire meta endpoints and deprecate old paths"
```

---

### Task 9: Open PR in homelab-api

- [ ] **Step 1: Push and open PR**

```bash
git push origin HEAD
gh pr create \
  --title "feat: add /meta/version and /meta/auth via contract-first approach" \
  --body "$(cat <<'EOF'
## Summary

- Adds `GET /meta/version` and `GET /meta/auth` defined in the OpenAPI spec (spec submodule bumped)
- Implements `internal/meta/` package with service and handler following the existing domain pattern
- Keeps `GET /version` and `GET /.well-known/homelab` alive with `Deprecation: true` and `Link` headers pointing to the new canonical paths
- Both new endpoints are public (`security: []`) — no bearer token required

## Test plan

- [ ] `make test` passes
- [ ] `make build` succeeds
- [ ] `GET /meta/version` returns `{"apiVersion":"...","serverVersion":"..."}`
- [ ] `GET /meta/auth` returns `{"enabled":...}` (with optional `issuer`)
- [ ] `GET /version` returns same body + `Deprecation: true` + `Link: </meta/version>; rel="successor-version"`
- [ ] `GET /.well-known/homelab` returns same body + `Deprecation: true` + `Link: </meta/auth>; rel="successor-version"`
EOF
)"
```
