# Dex Integration Design

## Overview

Wire the homelab API to [Dex](https://dexidp.io/) as the identity provider, using device authorization flow so the homelab-cli can authenticate users via GitHub, Google, or any other connector Dex supports. Scope enforcement is preserved in code but disabled via config, since Dex does not populate custom resource-server scopes in its access tokens.

Dex runs as a sidecar container alongside the API. The API reverse-proxies all `/dex/*` requests to the Dex container's internal Docker network address, so homelab-cli only ever needs to know the API's host and port — no separate Dex address is required.

## Key Decisions

- **IdP:** Dex — lightweight OIDC broker that delegates to upstream connectors (GitHub, Google, LDAP, etc.) and supports static config for dev/demo
- **Flow:** Device authorization flow (`urn:ietf:params:oauth:grant-type:device_code`) — no browser redirect needed on the CLI side
- **Proxy model:** API mounts a `httputil.ReverseProxy` at `/dex` that forwards to the Dex sidecar on the internal Docker network; Dex is never exposed externally
- **Single address:** homelab-cli discovers and communicates with Dex entirely through `{api-host}/dex/*`; all OIDC endpoints in the discovery document are externally reachable because Dex's `issuer` is set to the external API URL
- **Scope enforcement:** Disabled via `auth.scopes_enabled: false`; the `ScopeMiddleware` remains in code and can be re-enabled when switching to an IdP that populates resource scopes
- **Local dev:** Docker Compose runs API and Dex together; Dex uses a static connector with a test user so no external IdP is needed during development

## Config Changes (homelab-api)

### `Auth` struct — remove `JWKSURL`, keep `ScopesEnabled`

`auth.jwks_url` is removed as a user-facing field. It is derived internally as `{dex.url}/dex/keys` so there is nothing extra to configure. `auth.issuer` is the only auth-side value the user sets.

```go
type Auth struct {
    Enabled       bool   `yaml:"enabled"`
    ScopesEnabled bool   `yaml:"scopes_enabled"`
    Issuer        string `yaml:"issuer"`
    Audience      string `yaml:"audience"`
}
```

### New `Dex` struct

```go
type Dex struct {
    URL string `yaml:"url"` // internal Dex address, e.g. http://dex:5556
}

type Config struct {
    Auth     Auth          `yaml:"auth"`
    Dex      Dex           `yaml:"dex"`
    Backends []Backend     `yaml:"backends"`
    Updates  UpdatesConfig `yaml:"updates"`
}
```

`dex.url` serves two purposes:
- **Proxy target:** API forwards `/dex/*` requests to this address
- **JWKS source:** API derives `{dex.url}/dex/keys` to fetch Dex's public keys for JWT validation

### `config.validate()` update

When `auth.enabled: true`, `dex.url` must be non-empty — it is the only source of JWKS in this design. The old `auth.jwks_url` validation is replaced by this check.

### Sample config

```yaml
auth:
  enabled: true
  scopes_enabled: false
  issuer: http://localhost:8080/dex   # external-facing; validates JWT iss claim + returned by /.well-known/homelab

dex:
  url: http://dex:5556               # internal Docker address; proxy target + JWKS derived as {url}/dex/keys
```

`auth.issuer` must match the `issuer` field in `dex/config.yaml` exactly — they are the same value from different perspectives (API validates against it; Dex stamps it into tokens).

## Reverse Proxy (`internal/auth/dex.go`)

A small constructor following the same package convention as `JWTMiddleware` and `ScopeMiddleware`:

```go
func DexProxy(dexURL string) http.Handler {
    target, _ := url.Parse(dexURL)
    return &httputil.ReverseProxy{
        Rewrite: func(r *httputil.ProxyRequest) {
            r.SetURL(target)
            r.Out.Host = r.In.Host // preserve original Host so Dex generates correct external URLs
        },
    }
}
```

`r.Out.Host = r.In.Host` is critical: without it, Dex receives `Host: dex:5556` and may generate device flow verification URLs with the wrong host. With it, Dex sees the original client host (e.g. `localhost:8080`) and generates correct external URLs. No path rewriting is needed — requests arrive as `/dex/…` and forward as `/dex/…` since Dex is already configured with that sub-path.

The proxy is **not** placed behind `jwtMw` — Dex manages its own auth and all device flow endpoints must be publicly reachable.

Mounted in `main.go`:

```go
jwksURL := cfg.Dex.URL + "/dex/keys"
// ... keyfunc initialization using jwksURL ...

if cfg.Dex.URL != "" {
    r.Mount("/dex", auth.DexProxy(cfg.Dex.URL))
}
```

## Dex Configuration (`dex/config.yaml`)

Key change: `issuer` is set to the external API URL so that all OIDC endpoints in the discovery document (`/.well-known/openid-configuration`) are externally reachable through the proxy. homelab-cli discovers these endpoints and uses them directly — they all route through the API at `{api-host}/dex/*`.

```yaml
issuer: http://localhost:8080/dex   # was: http://localhost:5556/dex

storage:
  type: memory

web:
  http: 0.0.0.0:5556   # internal only; not exposed externally

oauth2:
  skipApprovalScreen: true

deviceFlow: {}

enablePasswordDB: true

staticPasswords:
  - email: "admin@homelab.local"
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: "admin"
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"

staticClients:
  - id: homelab-cli
    name: Homelab CLI
    public: true
    grantTypes:
      - urn:ietf:params:oauth:grant-type:device_code
      - refresh_token
```

## Docker Compose

Dex's port mapping is removed — it is only reachable via the API proxy:

```yaml
services:
  api:
    build: .
    ports:
      - "8080:8080"
    env_file: .env
    environment:
      - CONFIG_FILE=/config.yaml
    volumes:
      - ./config.yaml:/config.yaml:ro
    depends_on:
      - dex

  dex:
    image: ghcr.io/dexidp/dex:latest
    # no ports: — Dex is internal only, accessed through the API proxy at /dex/*
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml:ro
    command: ["dex", "serve", "/etc/dex/config.yaml"]
```

## Local Dev Setup

End-to-end test flow:
1. `docker compose up`
2. `homelab-cli auth login` → hits `/.well-known/homelab`, discovers issuer `http://localhost:8080/dex`
3. CLI fetches `http://localhost:8080/dex/.well-known/openid-configuration` (proxied to Dex) — gets device authorization endpoint
4. CLI starts device flow, prints verification URL + code
5. Open browser, complete device flow with static test user
6. CLI stores token; subsequent commands send it as `Authorization: Bearer <token>`
7. API validates JWT: JWKS fetched from `http://dex:5556/dex/keys` (internal), `iss` validated against `auth.issuer`

CI is unaffected — existing middleware unit tests cover JWT validation with in-memory RSA keys.

## CLI Integration (out of scope)

The homelab-cli needs a device flow implementation (`auth login` / `auth logout`) and Bearer token injection on every API call. Details — token storage, refresh logic, command structure — are out of scope here.

What this design provides on the API/Dex side:
- A Dex client (`homelab-cli`) with device authorization grant enabled
- All OIDC endpoints accessible through the API at a single host:port
- The API accepts any valid JWT issued by Dex as full access (when `scopes_enabled: false`)

## Files Changed

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Dex` struct with `URL string`; remove `JWKSURL` from `Auth`; add `ScopesEnabled bool` to `Auth` |
| `config.sample.yaml` | Replace `auth.jwks_url` with `dex.url`; update issuer example |
| `internal/auth/dex.go` | New — `DexProxy(dexURL string) http.Handler` using `httputil.ReverseProxy` with `Rewrite` |
| `cmd/server/main.go` | Derive JWKS URL as `cfg.Dex.URL + "/dex/keys"`; mount `DexProxy` at `/dex` if `cfg.Dex.URL` set |
| `internal/auth/middleware.go` | Add `!cfg.ScopesEnabled` short-circuit to `ScopeMiddleware` |
| `internal/auth/middleware_test.go` | Add test: scopes disabled → requests pass through |
| `dex/config.yaml` | Change `issuer` to `http://localhost:8080/dex` |
| `docker-compose.yml` | Remove Dex `ports:` mapping |
