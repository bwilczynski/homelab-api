# Authorization Design

## Overview

Add JWT-based authorization to all API endpoints. The OpenAPI spec already defines a `bearerAuth` OAuth 2.0 security scheme with per-operation scopes. This design adds runtime enforcement: validate bearer tokens and check scopes before requests reach handlers.

## Key Decisions

- **Token validation:** JWKS-based JWT verification with background key refresh
- **Middleware placement:** Single chi router-level middleware for both token validation and scope checking
- **Dev mode:** Config toggle (`auth.enabled: false`) to bypass auth entirely during development
- **Dependencies:** `golang-jwt/jwt/v5` for JWT parsing, `MicahParks/keyfunc/v3` for JWKS caching
- **Error format:** RFC 9457 problem+json, consistent with existing `apierrors` patterns

## Configuration

Add an `Auth` section to `internal/config/config.go` and `config.sample.yaml`:

```yaml
auth:
  enabled: false
  issuer: https://idp.homelab.local
  jwks_url: https://idp.homelab.local/.well-known/jwks.json
  audience: homelab-api
```

```go
type Config struct {
    Auth     Auth      `yaml:"auth"`
    Backends []Backend `yaml:"backends"`
}

type Auth struct {
    Enabled  bool   `yaml:"enabled"`
    Issuer   string `yaml:"issuer"`
    JWKSURL  string `yaml:"jwks_url"`
    Audience string `yaml:"audience"`
}
```

- `enabled` — when `false`, middleware passes all requests through
- `issuer` — validated against the `iss` claim; required when enabled
- `jwks_url` — JWKS endpoint for fetching signing keys; required when enabled
- `audience` — validated against the `aud` claim; optional

When `enabled: true`, the server fails to start if `issuer` or `jwks_url` are missing.

## Auth Middleware

New package: `internal/auth/`

### Execution order in oapi-codegen

The generated code has three middleware layers:

1. **Chi router middleware** (`r.Use(...)`) — runs before routing/parameter binding, **no scopes in context yet**.
2. **`ServerInterfaceWrapper`** — injects `BearerAuthScopes` into context, then runs `ChiServerOptions.Middlewares` (type `MiddlewareFunc = func(http.Handler) http.Handler`).
3. **`StrictMiddlewareFunc`** — runs inside the strict handler, after request unmarshaling.

The auth middleware needs access to scopes in context, so it must be registered at layer 2 — via `HandlerWithOptions` with `ChiServerOptions.Middlewares`, not via `r.Use()`.

### `middleware.go`

A middleware (`func(http.Handler) http.Handler`) registered via `ChiServerOptions.Middlewares` on each domain:

1. If auth is disabled, call `next` immediately.
2. Extract `Authorization: Bearer <token>` header; return 401 if missing or malformed.
3. Parse and validate JWT using JWKS keyset (signature, expiry, issuer, audience).
4. Extract scopes from the token's `scope` claim (space-delimited string per RFC 8693).
5. Read required scopes from context (`BearerAuthScopes` — injected by oapi-codegen generated wrapper before this middleware runs).
6. If no required scopes in context, allow through.
7. Check that token scopes are a superset of required scopes; return 403 if insufficient.
8. Call `next`.

Constructor:

```go
func NewMiddleware(cfg config.Auth, jwks *keyfunc.Keyfunc) func(http.Handler) http.Handler
```

When `cfg.Enabled` is `false`, `jwks` can be nil — the middleware short-circuits.

### `middleware_test.go`

Test cases:
- Auth disabled: requests pass through without a token
- Missing Authorization header → 401
- Malformed Authorization header (not "Bearer ...") → 401
- Expired token → 401
- Invalid signature → 401
- Valid token, missing required scope → 403
- Valid token, correct scope → 200
- Valid token, multiple required scopes, token has all → 200
- Valid token, multiple required scopes, token missing one → 403

Tests generate RSA keys in-memory and sign test JWTs — no external JWKS endpoint needed.

## Error Responses

Two new entries in `internal/apierrors/errors.go`:

```go
const (
    URNUnauthorized = "urn:homelab:error:unauthorized"
    URNForbidden    = "urn:homelab:error:forbidden"

    TitleUnauthorized = "Unauthorized"
    TitleForbidden    = "Forbidden"
)
```

401 response body:
```json
{
  "type": "urn:homelab:error:unauthorized",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Missing or invalid bearer token."
}
```

403 response body:
```json
{
  "type": "urn:homelab:error:forbidden",
  "title": "Forbidden",
  "status": 403,
  "detail": "Insufficient scopes. Required: read:containers."
}
```

The `detail` field on 403 lists the required scopes for debuggability. No token contents or internal details are leaked.

## Wiring in `cmd/server/main.go`

On startup:

1. Load config (already done).
2. If `auth.enabled`:
   - Validate that `issuer` and `jwks_url` are present; `log.Fatal` if not.
   - Initialize `keyfunc.NewDefault` with the JWKS URL (provides background refresh).
   - Perform initial JWKS fetch to fail fast if the endpoint is unreachable.
3. Create auth middleware via `auth.NewMiddleware(cfg.Auth, jwksKeyfunc)`.
4. Switch each domain from `HandlerFromMux` to `HandlerWithOptions`, passing the auth middleware via `ChiServerOptions.Middlewares`:

```go
authMw := auth.NewMiddleware(cfg.Auth, jwksKeyfunc)
opts := system.ChiServerOptions{
    BaseRouter:  r,
    Middlewares: []system.MiddlewareFunc{authMw},
}
system.HandlerWithOptions(system.NewStrictHandler(system.NewHandler(systemSvc), nil), opts)
```

Same pattern for all 5 domains (containers, storage, backups, network). The `MiddlewareFunc` type is identical across packages (`func(http.Handler) http.Handler`), so the same `authMw` value works for all.

## Dependencies

- `github.com/golang-jwt/jwt/v5` — JWT parsing and validation
- `github.com/MicahParks/keyfunc/v3` — JWKS fetching and caching with background refresh, wraps `golang-jwt`

## Files Changed

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Auth` struct and field to `Config` |
| `config.sample.yaml` | Add `auth:` section |
| `internal/apierrors/errors.go` | Add `URNUnauthorized`, `URNForbidden`, `TitleUnauthorized`, `TitleForbidden` |
| `internal/auth/middleware.go` | New — auth middleware |
| `internal/auth/middleware_test.go` | New — middleware tests |
| `cmd/server/main.go` | Initialize JWKS, register auth middleware |
| `go.mod` / `go.sum` | Add new dependencies |
