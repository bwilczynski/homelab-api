# Dex Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire homelab-api to Dex as the OIDC identity provider, with JWT validation enabled and scope enforcement controlled by a config toggle.

**Architecture:** `JWTMiddleware` validates Bearer tokens against Dex's JWKS endpoint; `ScopeMiddleware` gains a `scopes_enabled` flag that short-circuits enforcement when `false` (for IdPs that don't populate resource scopes). A `docker-compose.yml` at the repo root brings up the API and Dex together for local integration testing, with a static password connector for dev use.

**Tech Stack:** Dex (`ghcr.io/dexidp/dex:latest`), Docker Compose v2, existing `golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3`

---

### Task 1: Add `ScopesEnabled` to `Auth` config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config.sample.yaml`

- [ ] **Step 1: Write failing test for `ScopesEnabled` field**

Add to `internal/config/config_test.go`:

```go
func TestLoadAuthScopesEnabled(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  scopes_enabled: true
  issuer: https://test-issuer
  jwks_url: https://test-issuer/keys
backends:
  - name: nas
    type: synology
    host: a
    username: u
    password: p
`)
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Auth.ScopesEnabled {
		t.Error("expected ScopesEnabled to be true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

```sh
go test ./internal/config/
```

Expected: compile error — `c.Auth.ScopesEnabled undefined`

- [ ] **Step 3: Add `ScopesEnabled` to `Auth` struct**

In `internal/config/config.go`, update `Auth`:

```go
type Auth struct {
	Enabled       bool   `yaml:"enabled"`
	ScopesEnabled bool   `yaml:"scopes_enabled"`
	Issuer        string `yaml:"issuer"`
	JWKSURL       string `yaml:"jwks_url"`
	Audience      string `yaml:"audience"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

```sh
go test ./internal/config/
```

Expected: PASS

- [ ] **Step 5: Update `config.sample.yaml`**

Replace the existing `auth:` block with:

```yaml
auth:
  enabled: false
  scopes_enabled: false  # set true when IdP supports resource scopes (e.g. read:containers)
  issuer: https://idp.homelab.local
  jwks_url: https://idp.homelab.local/.well-known/jwks.json
  audience: homelab-api  # optional; omit or leave empty to skip audience validation
```

- [ ] **Step 6: Commit**

```sh
git add internal/config/config.go internal/config/config_test.go config.sample.yaml
git commit -m "feat: add scopes_enabled toggle to Auth config"
```

---

### Task 2: Add `scopes_enabled` short-circuit to `ScopeMiddleware`

**Files:**
- Modify: `internal/auth/middleware_test.go`
- Modify: `internal/auth/middleware.go`

- [ ] **Step 1: Write failing test**

Add to `internal/auth/middleware_test.go`:

```go
func TestScopeMiddleware_ScopesDisabled(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss": "https://test-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	cfg := config.Auth{Enabled: true, ScopesEnabled: false, Issuer: "https://test-issuer"}
	// nil required scopes — without the short-circuit this returns 403 (deny by default).
	handler := scopeTestHandler(cfg, staticKeyFunc(pub), nil)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```sh
go test ./internal/auth/ -run TestScopeMiddleware_ScopesDisabled -v
```

Expected: FAIL — got 403, want 200

- [ ] **Step 3: Update `authCfg()` and add `!cfg.ScopesEnabled` short-circuit to `ScopeMiddleware`**

First, update `authCfg()` in `internal/auth/middleware_test.go` to set `ScopesEnabled: true` — the existing scope tests rely on enforcement being active, and `false` is the Go zero value which would silently disable it:

```go
func authCfg() config.Auth {
	return config.Auth{
		Enabled:       true,
		ScopesEnabled: true,
		Issuer:        "https://test-issuer",
	}
}
```

Then, in `internal/auth/middleware.go`, update `ScopeMiddleware` to add the check immediately after the `!cfg.Enabled` block:

```go
func ScopeMiddleware(cfg config.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			if !cfg.ScopesEnabled {
				next.ServeHTTP(w, r)
				return
			}

			requiredScopes, ok := r.Context().Value("bearerAuth.Scopes").([]string)
			if !ok || len(requiredScopes) == 0 {
				writeProblem(w, http.StatusForbidden, apierrors.URNForbidden, apierrors.TitleForbidden, "No required scopes declared for this operation.")
				return
			}

			tokenScopes, _ := r.Context().Value(tokenScopesKey).([]string)
			scopeSet := make(map[string]bool, len(tokenScopes))
			for _, s := range tokenScopes {
				scopeSet[s] = true
			}
			for _, required := range requiredScopes {
				if !scopeSet[required] {
					writeProblem(w, http.StatusForbidden, apierrors.URNForbidden, apierrors.TitleForbidden,
						fmt.Sprintf("Insufficient scopes. Required: %s.", strings.Join(requiredScopes, ", ")))
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run all auth tests**

```sh
go test ./internal/auth/ -v
```

Expected: all PASS

- [ ] **Step 5: Run full test suite**

```sh
make test
```

Expected: PASS

- [ ] **Step 6: Commit**

```sh
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat: short-circuit ScopeMiddleware when scopes_enabled is false"
```

---

### Task 3: Dex configuration

**Files:**
- Create: `dex/config.yaml`

- [ ] **Step 1: Create `dex/` directory and `dex/config.yaml`**

```yaml
# Dex OIDC provider — development configuration.
# Uses an in-memory store and a static password connector.
# For production: replace staticPasswords with a real connector (GitHub, Google, LDAP).

issuer: http://localhost:5556/dex

storage:
  type: memory

web:
  http: 0.0.0.0:5556

oauth2:
  skipApprovalScreen: true

# Enable RFC 8628 device authorization flow.
deviceFlow: {}

# Local password database — for dev/demo only.
enablePasswordDB: true

staticPasswords:
  - email: "admin@homelab.local"
    # bcrypt hash of "password" (cost 10) — for dev use only.
    # Regenerate: htpasswd -bnBC 10 "" password | tr -d ':\n'
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: "admin"
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"

staticClients:
  - id: homelab-cli
    name: Homelab CLI
    # Public client — no secret required; used for device authorization flow.
    public: true
    grantTypes:
      - urn:ietf:params:oauth:grant-type:device_code
      - refresh_token
```

- [ ] **Step 2: Commit**

```sh
git add dex/config.yaml
git commit -m "feat: add Dex config with static connector and homelab-cli device client"
```

---

### Task 4: Docker Compose

**Files:**
- Create: `docker-compose.yml`

- [ ] **Step 1: Create `docker-compose.yml`**

```yaml
services:
  api:
    build: .
    ports:
      - "8080:8080"
    environment:
      - CONFIG_FILE=/config.yaml
    volumes:
      - ./config.yaml:/config.yaml:ro
    depends_on:
      - dex

  dex:
    image: ghcr.io/dexidp/dex:latest
    ports:
      - "5556:5556"
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml:ro
    command: ["dex", "serve", "/etc/dex/config.yaml"]
```

- [ ] **Step 2: Document required `config.yaml` entries for docker-compose**

When running via docker-compose, the `auth:` section in `config.yaml` must use the Docker service name for `jwks_url` (container-to-container DNS), while `issuer` must match what Dex puts in the `iss` claim (the external-facing `localhost` URL). Add this comment block to `config.yaml` (not committed — user's local file):

```yaml
# For docker compose: API container reaches Dex via service name DNS.
# issuer must match the iss claim Dex issues (the externally visible URL).
auth:
  enabled: true
  scopes_enabled: false
  issuer: http://localhost:5556/dex
  jwks_url: http://dex:5556/dex/keys  # Docker service name, not localhost
```

Document this in a comment at the top of `docker-compose.yml`:

```yaml
# Run: docker compose up
# Requires config.yaml with:
#   auth.jwks_url: http://dex:5556/dex/keys  (Docker service DNS)
#   auth.issuer:   http://localhost:5556/dex  (matches Dex iss claim)
```

Update `docker-compose.yml` to include the comment:

```yaml
# Run: docker compose up
# Requires config.yaml with:
#   auth.jwks_url: http://dex:5556/dex/keys  (Docker service DNS, not localhost)
#   auth.issuer:   http://localhost:5556/dex  (must match Dex iss claim)

services:
  api:
    build: .
    ports:
      - "8080:8080"
    environment:
      - CONFIG_FILE=/config.yaml
    volumes:
      - ./config.yaml:/config.yaml:ro
    depends_on:
      - dex

  dex:
    image: ghcr.io/dexidp/dex:latest
    ports:
      - "5556:5556"
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml:ro
    command: ["dex", "serve", "/etc/dex/config.yaml"]
```

- [ ] **Step 3: Commit**

```sh
git add docker-compose.yml
git commit -m "feat: add docker-compose for local API + Dex integration testing"
```

---

### Task 5: Update docs

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add auth section to `README.md` Configuration**

After the existing `backends:` example block, add:

```markdown
### Authorization

JWT-based authorization is optional. Set `auth.enabled: true` and point it at your OIDC provider:

```yaml
auth:
  enabled: true
  scopes_enabled: false   # enable when your IdP populates resource scopes
  issuer: https://dex.homelab.local/dex
  jwks_url: https://dex.homelab.local/dex/keys
  audience: homelab-api   # optional
```

With `enabled: false` (default) all requests pass through without a token.
```

- [ ] **Step 2: Add Local dev with Dex section to `README.md`**

After the Commands table, add:

```markdown
## Local dev with Dex

A Docker Compose file brings up the API and [Dex](https://dexidp.io/) together for testing the full auth flow:

```sh
# 1. Set auth settings in config.yaml (see Authorization section above — use jwks_url: http://dex:5556/dex/keys)
docker compose up
```

Dex starts with a static password connector. Test user: `admin@homelab.local` / `password`.
```

- [ ] **Step 3: Update `CLAUDE.md` architecture tree**

In the `internal/` tree section of `CLAUDE.md`, add `auth/` after `apierrors/`:

```
  auth/               JWT validation middleware + scope enforcement middleware
  apierrors/          Shared error sentinels and RFC 9457 problem+json constants
```

- [ ] **Step 4: Add auth toggle note to `CLAUDE.md`**

After the **Key rules** bullet list in `CLAUDE.md`, add:

```markdown
## Authorization

Auth is split across two chi middleware layers (see `internal/auth/middleware.go`):
- `JWTMiddleware` — validates Bearer tokens via JWKS; registered via `r.Use(...)` before routing.
- `ScopeMiddleware` — checks per-operation scopes from context; registered via `ChiServerOptions.Middlewares`.

Both short-circuit to no-ops when `auth.enabled: false`. `ScopeMiddleware` additionally short-circuits when `auth.scopes_enabled: false` (used when the IdP doesn't populate resource scopes — e.g. Dex without custom scope config).
```

- [ ] **Step 5: Run tests to make sure nothing broke**

```sh
make test
```

Expected: PASS

- [ ] **Step 6: Commit**

```sh
git add README.md CLAUDE.md
git commit -m "docs: document auth config and Dex local dev setup"
```
