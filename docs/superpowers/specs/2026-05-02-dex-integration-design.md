# Dex Integration Design

## Overview

Wire the homelab API to [Dex](https://dexidp.io/) as the identity provider, using device authorization flow so the homelab-cli can authenticate users via GitHub, Google, or any other connector Dex supports. Scope enforcement is preserved in code but disabled via config, since Dex does not populate custom resource-server scopes in its access tokens.

## Key Decisions

- **IdP:** Dex — lightweight OIDC broker that delegates to upstream connectors (GitHub, Google, LDAP, etc.) and supports static config for dev/demo
- **Flow:** Device authorization flow (`urn:ietf:params:oauth:grant-type:device_code`) — no browser redirect needed on the CLI side
- **Scope enforcement:** Disabled via `auth.scopes_enabled: false`; the `ScopeMiddleware` remains in code and can be re-enabled when switching to an IdP that populates resource scopes
- **Local dev:** Docker Compose runs the API and Dex together; Dex uses a static connector with a test user so no external IdP is needed during development

## Config Change (homelab-api)

Add `scopes_enabled` to the `Auth` struct in `internal/config/config.go`:

```go
type Auth struct {
    Enabled       bool   `yaml:"enabled"`
    ScopesEnabled bool   `yaml:"scopes_enabled"`
    Issuer        string `yaml:"issuer"`
    JWKSURL       string `yaml:"jwks_url"`
    Audience      string `yaml:"audience"`
}
```

Sample config:

```yaml
auth:
  enabled: true
  scopes_enabled: false   # set true when IdP supports resource scopes
  issuer: https://dex.homelab.local/dex
  jwks_url: https://dex.homelab.local/dex/keys
  audience: homelab-api
```

`ScopeMiddleware` gets a second early-return: if `!cfg.ScopesEnabled`, call `next` immediately. The deny-by-default behaviour is preserved for when scopes are turned on.

## Dex Configuration

Dex config lives at `dex/config.yaml` in the repo. Key settings:

- **Issuer:** `http://localhost:5556/dex` (local) / `https://dex.homelab.local/dex` (prod)
- **Static client:** `homelab-cli` with device authorization grant enabled
- **Static connector (dev):** one test user (email + password) — no external IdP needed
- **GitHub / Google connectors (prod):** added to the deployed Dex config; no code changes needed
- **`oauth2.skipApprovalScreen: true`** for frictionless device flow on a personal setup

The JWKS endpoint (`<issuer>/keys`) is set as `auth.jwks_url` in the API config. The API validates `iss` and `aud` claims.

## CLI Integration (out of scope)

The homelab-cli needs a device flow implementation (`auth login` / `auth logout`) and Bearer token injection on every API call. Details — token storage, refresh logic, command structure — are out of scope here and belong in a separate spec in the `homelab-cli` repo.

What this design provides on the API/Dex side:
- A Dex client (`homelab-cli`) with device authorization grant enabled
- The API accepts any valid JWT issued by Dex as full access (when `scopes_enabled: false`)

## Local Dev Setup

`docker-compose.yml` at the repo root:

```yaml
services:
  api:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    depends_on:
      - dex

  dex:
    image: ghcr.io/dexidp/dex:latest
    ports:
      - "5556:5556"
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml
    command: ["dex", "serve", "/etc/dex/config.yaml"]
```

End-to-end test flow:
1. `docker compose up`
2. `homelab-cli auth login` → prints verification URL + code
3. Open browser, complete device flow with the static test user
4. CLI stores token; subsequent commands send it as `Authorization: Bearer <token>`
5. API validates JWT against Dex's JWKS endpoint, returns data

CI is unaffected — the existing middleware unit tests cover JWT validation with in-memory RSA keys. Docker Compose is for local integration testing only.

## Files Changed

| File | Change |
|---|---|
| `internal/config/config.go` | Add `ScopesEnabled bool` to `Auth` struct |
| `config.sample.yaml` | Add `scopes_enabled: false` to `auth:` section |
| `internal/auth/middleware.go` | Add `!cfg.ScopesEnabled` short-circuit to `ScopeMiddleware` |
| `internal/auth/middleware_test.go` | Add test: scopes disabled → requests pass through |
| `dex/config.yaml` | New — Dex config with static connector and `homelab-cli` device client |
| `docker-compose.yml` | New — runs API + Dex for local integration testing |
