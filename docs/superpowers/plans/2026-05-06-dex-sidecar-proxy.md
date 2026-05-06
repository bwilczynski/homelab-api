# Dex Sidecar Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route all Dex OIDC traffic through the homelab-api at `/dex/*` via a reverse proxy, so homelab-cli only needs one address.

**Architecture:** Dex runs as a Docker sidecar without an exposed port. The API mounts a `httputil.ReverseProxy` at `/dex` that forwards requests to the internal Dex address, preserving the original `Host` header so Dex generates correct external URLs. JWKS URL is derived from `dex.url` — no separate config field.

**Tech Stack:** Go stdlib `net/http/httputil`, chi v5, `github.com/MicahParks/keyfunc/v3`

---

## File Map

| File | Action | What changes |
|---|---|---|
| `internal/config/config.go` | Modify | Remove `JWKSURL` from `Auth`; add `Dex` struct + field; update `validate()` |
| `internal/config/config_test.go` | Modify | Update test that uses `jwks_url`; add tests for `dex.url` |
| `internal/auth/dex.go` | Create | `DexProxy(dexURL string) http.Handler` |
| `internal/auth/dex_test.go` | Create | Tests for `DexProxy` |
| `cmd/server/main.go` | Modify | Derive JWKS URL from `cfg.Dex.URL`; mount proxy |
| `config.sample.yaml` | Modify | Replace `auth.jwks_url` with `dex.url` |
| `dex/config.yaml` | Modify | Change `issuer` to `http://localhost:8080/dex` |
| `docker-compose.yml` | Modify | Remove Dex `ports:` mapping |

> **Already done — no changes needed:**
> - `internal/auth/middleware.go` — `ScopesEnabled` short-circuit already exists
> - `internal/auth/middleware_test.go` — `TestScopeMiddleware_ScopesDisabled` already exists
> - `internal/config/config.go` — `ScopesEnabled bool` already in `Auth` struct

---

## Task 1: Update config struct and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

### Step 1.1: Write failing tests for new config shape

- [ ] Add to `internal/config/config_test.go`:

```go
func TestLoadDexURL(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  issuer: http://localhost:8080/dex
dex:
  url: http://dex:5556
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
	if c.Dex.URL != "http://dex:5556" {
		t.Errorf("expected dex.url http://dex:5556, got %s", c.Dex.URL)
	}
}

func TestLoadAuthEnabled_RequiresDexURL(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  issuer: http://localhost:8080/dex
backends:
  - name: nas
    type: synology
    host: a
    username: u
    password: p
`)
	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error when auth enabled but dex.url missing")
	}
}
```

- [ ] Run tests to confirm they fail:

```
go test ./internal/config/ -run "TestLoadDexURL|TestLoadAuthEnabled_RequiresDexURL" -v
```

Expected: both FAIL — `Dex` field doesn't exist yet.

### Step 1.2: Update the existing `TestLoadAuthScopesEnabled` test

The test currently sets `jwks_url` which will become an unknown field after we remove it. Update it now so it's ready:

- [ ] In `internal/config/config_test.go`, replace the existing `TestLoadAuthScopesEnabled`:

```go
func TestLoadAuthScopesEnabled(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  scopes_enabled: true
  issuer: https://test-issuer
dex:
  url: http://dex:5556
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

### Step 1.3: Implement the config changes

- [ ] In `internal/config/config.go`, replace the `Auth` struct and add `Dex`:

```go
// Auth holds JWT/JWKS authorization settings.
type Auth struct {
	Enabled       bool   `yaml:"enabled"`
	ScopesEnabled bool   `yaml:"scopes_enabled"`
	Issuer        string `yaml:"issuer"`
	Audience      string `yaml:"audience"`
}

// Dex holds configuration for the embedded Dex OIDC proxy.
type Dex struct {
	URL string `yaml:"url"` // internal Dex address, e.g. http://dex:5556
}
```

- [ ] In `internal/config/config.go`, add the `Dex` field to `Config`:

```go
// Config is the top-level configuration.
type Config struct {
	Auth     Auth          `yaml:"auth"`
	Dex      Dex           `yaml:"dex"`
	Backends []Backend     `yaml:"backends"`
	Updates  UpdatesConfig `yaml:"updates"`
}
```

- [ ] In `internal/config/config.go`, replace the `validate()` auth block:

```go
func (c *Config) validate() error {
	if c.Auth.Enabled {
		if c.Auth.Issuer == "" {
			return fmt.Errorf("auth.issuer is required when auth is enabled")
		}
		if c.Dex.URL == "" {
			return fmt.Errorf("dex.url is required when auth is enabled")
		}
	}
	// ... rest of validate unchanged
```

### Step 1.4: Run tests

- [ ] Run all config tests:

```
go test ./internal/config/ -v
```

Expected: all PASS.

### Step 1.5: Fix compilation errors in `main.go`

`main.go` references `cfg.Auth.JWKSURL` which no longer exists. Add a temporary placeholder to make it compile while Task 3 is pending:

- [ ] In `cmd/server/main.go`, find the `keyfunc.NewDefault` call and replace `cfg.Auth.JWKSURL` with a derived value:

```go
if cfg.Auth.Enabled {
    jwksURL := cfg.Dex.URL + "/dex/keys"
    k, err := keyfunc.NewDefault([]string{jwksURL})
    if err != nil {
        logger.Error("failed to initialize JWKS", "err", err)
        os.Exit(1)
    }
    jwkKeyFunc = k.Keyfunc
    logger.Info("authorization enabled", "issuer", cfg.Auth.Issuer)
}
```

- [ ] Build to confirm no compilation errors:

```
make build
```

Expected: `bin/server` produced with no errors.

### Step 1.6: Commit

- [ ] Commit:

```bash
git add internal/config/config.go internal/config/config_test.go cmd/server/main.go
git commit -m "feat: replace auth.jwks_url with dex.url derived from Dex struct"
```

---

## Task 2: Implement `DexProxy`

**Files:**
- Create: `internal/auth/dex_test.go`
- Create: `internal/auth/dex.go`

### Step 2.1: Write the failing test

- [ ] Create `internal/auth/dex_test.go`:

```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/auth"
)

func TestDexProxy_ForwardsRequest(t *testing.T) {
	// Simulate Dex: echo back the Host header in a custom response header.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Host", r.Host)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := auth.DexProxy(backend.URL)

	req := httptest.NewRequest("GET", "/dex/.well-known/openid-configuration", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	// Host header must be preserved so Dex generates correct external URLs.
	if got := rec.Header().Get("X-Received-Host"); got != "localhost:8080" {
		t.Errorf("expected Host localhost:8080 forwarded to Dex, got %q", got)
	}
}

func TestDexProxy_ProxiesPath(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := auth.DexProxy(backend.URL)

	req := httptest.NewRequest("GET", "/dex/token", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedPath != "/dex/token" {
		t.Errorf("expected path /dex/token forwarded unchanged, got %q", receivedPath)
	}
}
```

- [ ] Run to confirm FAIL:

```
go test ./internal/auth/ -run "TestDexProxy" -v
```

Expected: FAIL — `auth.DexProxy` undefined.

### Step 2.2: Implement `DexProxy`

- [ ] Create `internal/auth/dex.go`:

```go
package auth

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// DexProxy returns a reverse proxy that forwards requests to the internal Dex
// server at dexURL. The original Host header is preserved so Dex generates
// correct external-facing URLs in its OIDC discovery document and device flow.
func DexProxy(dexURL string) http.Handler {
	target, err := url.Parse(dexURL)
	if err != nil {
		panic("auth.DexProxy: invalid dex URL: " + err.Error())
	}
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
		},
	}
}
```

### Step 2.3: Run tests

- [ ] Run:

```
go test ./internal/auth/ -run "TestDexProxy" -v
```

Expected: both PASS.

### Step 2.4: Run the full test suite

- [ ] Run:

```
make test
```

Expected: all tests PASS.

### Step 2.5: Commit

- [ ] Commit:

```bash
git add internal/auth/dex.go internal/auth/dex_test.go
git commit -m "feat: add DexProxy reverse proxy handler to internal/auth"
```

---

## Task 3: Mount the proxy in `main.go`

**Files:**
- Modify: `cmd/server/main.go`

### Step 3.1: Mount the proxy

The JWKS derivation was already fixed in Task 1, Step 1.5. Now add the proxy mount.

- [ ] In `cmd/server/main.go`, after the `r.Get("/.well-known/homelab", ...)` block and before `protected := r.With(jwtMw)`, add:

```go
if cfg.Dex.URL != "" {
    r.Mount("/dex", auth.DexProxy(cfg.Dex.URL))
}
```

### Step 3.2: Build

- [ ] Build:

```
make build
```

Expected: `bin/server` produced with no errors.

### Step 3.3: Run the full test suite

- [ ] Run:

```
make test
```

Expected: all PASS.

### Step 3.4: Commit

- [ ] Commit:

```bash
git add cmd/server/main.go
git commit -m "feat: mount DexProxy at /dex in chi router"
```

---

## Task 4: Update config files and Docker Compose

**Files:**
- Modify: `config.sample.yaml`
- Modify: `dex/config.yaml`
- Modify: `docker-compose.yml`

### Step 4.1: Update `config.sample.yaml`

- [ ] Replace the `auth:` block in `config.sample.yaml`:

```yaml
# Authorization settings.
# Set enabled: true and configure dex.url when Dex is running as a sidecar.
auth:
  enabled: false
  scopes_enabled: false  # set true when IdP supports resource scopes (e.g. read:containers)
  issuer: http://localhost:8080/dex  # must match dex/config.yaml issuer exactly
  audience: homelab-api              # optional; omit or leave empty to skip audience validation

# Dex OIDC sidecar settings.
# dex.url is the internal address of the Dex container (Docker network DNS).
# The API proxies /dex/* to this address and derives the JWKS URL as {url}/dex/keys.
dex:
  url: http://dex:5556
```

### Step 4.2: Update `dex/config.yaml` issuer

- [ ] In `dex/config.yaml`, change the `issuer` line:

```yaml
issuer: http://localhost:8080/dex
```

The `web.http` binding stays on `0.0.0.0:5556` — internal Docker network only.

### Step 4.3: Remove Dex external port from `docker-compose.yml`

- [ ] In `docker-compose.yml`, remove the `ports:` block from the `dex` service so it reads:

```yaml
  dex:
    image: ghcr.io/dexidp/dex:latest
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml:ro
    command: ["dex", "serve", "/etc/dex/config.yaml"]
```

### Step 4.4: Commit

- [ ] Commit:

```bash
git add config.sample.yaml dex/config.yaml docker-compose.yml
git commit -m "feat: configure Dex sidecar proxy — update issuer, remove external port"
```

---

## Self-Review

**Spec coverage:**

| Spec requirement | Covered by |
|---|---|
| Remove `auth.jwks_url`; derive from `dex.url` | Task 1 |
| Add `Dex` struct with `URL string` to config | Task 1 |
| `validate()` requires `dex.url` when auth enabled | Task 1 |
| `DexProxy` using `httputil.ReverseProxy` + `Rewrite` | Task 2 |
| `r.Out.Host = r.In.Host` to preserve Host header | Task 2 |
| Proxy NOT behind JWT middleware | Task 3 |
| Mount at `/dex` | Task 3 |
| JWKS URL derived as `{dex.url}/dex/keys` | Task 1 (Step 1.5) |
| `dex/config.yaml` issuer → external API URL | Task 4 |
| Remove Dex `ports:` from docker-compose | Task 4 |
| `config.sample.yaml` updated | Task 4 |
| `ScopesEnabled` short-circuit + test | Already implemented — no task needed |
