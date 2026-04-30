# Authorization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce JWT-based authorization on all API endpoints using scopes already declared in the OpenAPI spec.

**Architecture:** Two chi-compatible middlewares in `internal/auth/` — `JWTMiddleware` validates the bearer token at the chi router level and stores parsed scopes in context; `ScopeMiddleware` runs at the oapi-codegen operation level (after `BearerAuthScopes` is injected into context by generated wrappers) and denies requests that lack required scopes. Both middlewares are no-ops when `auth.enabled: false`.

**Tech Stack:** `github.com/golang-jwt/jwt/v5` for JWT parsing/validation, `github.com/MicahParks/keyfunc/v3` for JWKS fetching with background refresh.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `go.mod` / `go.sum` | Modify | Add jwt/v5 and keyfunc/v3 dependencies |
| `internal/config/config.go` | Modify | Add `Auth` struct + field to `Config`, add auth validation |
| `config.sample.yaml` | Modify | Document `auth:` config section |
| `internal/apierrors/errors.go` | Modify | Add 401/403 URN and title constants |
| `internal/auth/middleware.go` | Create | `JWTMiddleware` + `ScopeMiddleware` + helpers |
| `internal/auth/middleware_test.go` | Create | Tests for both middlewares |
| `cmd/server/main.go` | Modify | Initialize JWKS cache, register middlewares |

---

### Task 1: Add dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Install jwt and keyfunc**

```bash
go get github.com/golang-jwt/jwt/v5
go get github.com/MicahParks/keyfunc/v3
```

Expected output: two lines like `go: added github.com/golang-jwt/jwt/v5 v5.x.x` and `go: added github.com/MicahParks/keyfunc/v3 v3.x.x`

- [ ] **Step 2: Verify go.mod has both entries**

```bash
grep -E "golang-jwt|keyfunc" go.mod
```

Expected: two matching lines.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add golang-jwt and keyfunc dependencies"
```

---

### Task 2: Add Auth config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.sample.yaml`

- [ ] **Step 1: Add `Auth` struct and field to `Config` in `internal/config/config.go`**

Replace:
```go
// Config is the top-level configuration.
type Config struct {
	Backends []Backend `yaml:"backends"`
}
```

With:
```go
// Auth holds JWT/JWKS authorization settings.
type Auth struct {
	Enabled  bool   `yaml:"enabled"`
	Issuer   string `yaml:"issuer"`
	JWKSURL  string `yaml:"jwks_url"`
	Audience string `yaml:"audience"`
}

// Config is the top-level configuration.
type Config struct {
	Auth     Auth      `yaml:"auth"`
	Backends []Backend `yaml:"backends"`
}
```

- [ ] **Step 2: Add auth validation to `validate()` in `internal/config/config.go`**

Add this block inside `validate()`, before the backends loop:

```go
	if c.Auth.Enabled {
		if c.Auth.Issuer == "" {
			return fmt.Errorf("auth.issuer is required when auth is enabled")
		}
		if c.Auth.JWKSURL == "" {
			return fmt.Errorf("auth.jwks_url is required when auth is enabled")
		}
	}
```

- [ ] **Step 3: Update `config.sample.yaml`**

Add the `auth:` section at the top, before `backends:`:

```yaml
# Homelab API backend configuration.
# Copy this file to config.yaml and fill in your values.
# Values can reference environment variables: ${VAR_NAME}

# Authorization settings.
# Set enabled: true and fill in issuer/jwks_url when you have an IdP deployed.
auth:
  enabled: false
  issuer: https://idp.homelab.local
  jwks_url: https://idp.homelab.local/.well-known/jwks.json
  audience: homelab-api  # optional; omit or leave empty to skip audience validation

backends:
  - name: nas-01
    type: synology
    host: 192.168.1.10:5001
    username: admin
    password: ${NAS01_PASS}

  # Uncomment to add a second NAS:
  # - name: nas-02
  #   type: synology
  #   host: 192.168.1.11:5001
  #   username: admin
  #   password: ${NAS02_PASS}

  - name: unifi
    type: unifi
    host: 192.168.1.1
    username: admin
    password: ${UNIFI_PASS}
```

- [ ] **Step 4: Verify existing config tests still pass**

```bash
go test ./internal/config/...
```

Expected: `ok  github.com/bwilczynski/homelab-api/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go config.sample.yaml
git commit -m "Add Auth config struct with JWKS validation"
```

---

### Task 3: Add apierrors constants

**Files:**
- Modify: `internal/apierrors/errors.go`

- [ ] **Step 1: Add 401/403 constants to `internal/apierrors/errors.go`**

Replace the entire file with:

```go
package apierrors

import "errors"

// ErrNotFound is returned by service methods when a requested resource cannot be located.
var ErrNotFound = errors.New("not found")

// RFC 9457 problem+json constants.
const (
	URNNotFound            = "urn:homelab:error:not-found"
	URNInternalServerError = "urn:homelab:error:internal-server-error"
	URNUnauthorized        = "urn:homelab:error:unauthorized"
	URNForbidden           = "urn:homelab:error:forbidden"

	TitleNotFound            = "Not Found"
	TitleInternalServerError = "Internal Server Error"
	TitleUnauthorized        = "Unauthorized"
	TitleForbidden           = "Forbidden"
)
```

- [ ] **Step 2: Build to verify no errors**

```bash
go build ./internal/apierrors/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/apierrors/errors.go
git commit -m "Add Unauthorized and Forbidden apierrors constants"
```

---

### Task 4: Implement JWTMiddleware (TDD)

**Files:**
- Create: `internal/auth/middleware_test.go`
- Create: `internal/auth/middleware.go`

- [ ] **Step 1: Write failing tests for `JWTMiddleware` in `internal/auth/middleware_test.go`**

```go
package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/auth"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// okHandler responds 200 OK for any request.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// testKeyPair generates an RSA key pair for tests.
func testKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return priv, &priv.PublicKey
}

// makeToken signs a JWT with the given private key and claims.
func makeToken(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// staticKeyFunc returns a jwt.Keyfunc that always uses the given public key.
func staticKeyFunc(pub *rsa.PublicKey) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		return pub, nil
	}
}

func authCfg() config.Auth {
	return config.Auth{
		Enabled: true,
		Issuer:  "https://test-issuer",
	}
}

func TestJWTMiddleware_Disabled(t *testing.T) {
	cfg := config.Auth{Enabled: false}
	mw := auth.JWTMiddleware(cfg, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	priv, pub := testKeyPair(t)
	_ = priv
	mw := auth.JWTMiddleware(authCfg(), staticKeyFunc(pub))
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	priv, pub := testKeyPair(t)
	_ = priv
	mw := auth.JWTMiddleware(authCfg(), staticKeyFunc(pub))
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss": "https://test-issuer",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	mw := auth.JWTMiddleware(authCfg(), staticKeyFunc(pub))
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTMiddleware_InvalidSignature(t *testing.T) {
	priv, _ := testKeyPair(t)
	_, otherPub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss": "https://test-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	// Verify with a different public key — signature mismatch.
	mw := auth.JWTMiddleware(authCfg(), staticKeyFunc(otherPub))
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers write:containers",
	})
	mw := auth.JWTMiddleware(authCfg(), staticKeyFunc(pub))
	handler := mw(okHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Create minimal `internal/auth/middleware.go` so tests compile**

```go
package auth

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/config"
)

// JWTMiddleware validates the bearer token on every request and stores the
// parsed token scopes in context. It is registered via r.Use() on the chi router.
func JWTMiddleware(cfg config.Auth, keyFunc jwt.Keyfunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

// ScopeMiddleware checks that the token scopes (from context) satisfy the
// required scopes for the operation (also from context, injected by generated code).
// It is registered via ChiServerOptions.Middlewares on each domain handler.
func ScopeMiddleware(cfg config.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 3: Run tests — confirm they compile but most fail**

```bash
go test ./internal/auth/... -v
```

Expected: `TestJWTMiddleware_Disabled` passes, the rest fail with non-200 or non-401 responses.

- [ ] **Step 4: Implement `JWTMiddleware` fully in `internal/auth/middleware.go`**

Replace the entire file with:

```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// tokenScopesKey is the context key for storing parsed token scopes.
type contextKey struct{}

var tokenScopesKey = contextKey{}

// claims extends the standard registered claims with a scope field.
type claims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

// JWTMiddleware validates the bearer token on every request and stores the
// parsed token scopes in context. It is registered via r.Use() on the chi router.
func JWTMiddleware(cfg config.Auth, keyFunc jwt.Keyfunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeProblem(w, http.StatusUnauthorized, apierrors.URNUnauthorized, apierrors.TitleUnauthorized, "Missing or invalid bearer token.")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			opts := []jwt.ParserOption{
				jwt.WithIssuer(cfg.Issuer),
				jwt.WithExpirationRequired(),
			}
			if cfg.Audience != "" {
				opts = append(opts, jwt.WithAudience(cfg.Audience))
			}

			var c claims
			token, err := jwt.ParseWithClaims(tokenStr, &c, keyFunc, opts...)
			if err != nil || !token.Valid {
				writeProblem(w, http.StatusUnauthorized, apierrors.URNUnauthorized, apierrors.TitleUnauthorized, "Missing or invalid bearer token.")
				return
			}

			scopes := strings.Fields(c.Scope)
			ctx := context.WithValue(r.Context(), tokenScopesKey, scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ScopeMiddleware checks that the token scopes (from context) satisfy the
// required scopes for the operation (also from context, injected by generated code).
// It is registered via ChiServerOptions.Middlewares on each domain handler.
func ScopeMiddleware(cfg config.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

func writeProblem(w http.ResponseWriter, status int, urn, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   urn,
		"title":  title,
		"status": status,
		"detail": detail,
	})
}

// suppress unused import warning during stub phase
var _ = fmt.Sprintf
```

- [ ] **Step 5: Run JWTMiddleware tests — all should pass**

```bash
go test ./internal/auth/... -run TestJWTMiddleware -v
```

Expected: all 5 `TestJWTMiddleware_*` tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "Implement JWTMiddleware with bearer token validation"
```

---

### Task 5: Implement ScopeMiddleware (TDD)

**Files:**
- Modify: `internal/auth/middleware_test.go`
- Modify: `internal/auth/middleware.go`

- [ ] **Step 1: Add `ScopeMiddleware` tests to `internal/auth/middleware_test.go`**

Append to the existing test file:

```go
// scopeTestHandler chains JWTMiddleware + ScopeMiddleware + okHandler.
// It injects bearerAuth.Scopes into context before calling ScopeMiddleware,
// simulating what the oapi-codegen ServerInterfaceWrapper does.
func scopeTestHandler(cfg config.Auth, keyFunc jwt.Keyfunc, requiredScopes []string) http.Handler {
	scopeMw := auth.ScopeMiddleware(cfg)
	inner := scopeMw(okHandler)
	// Wrap inner to inject the required scopes, simulating oapi-codegen's wrapper.
	withScopes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if requiredScopes != nil {
			ctx = context.WithValue(ctx, "bearerAuth.Scopes", requiredScopes)
		}
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
	return auth.JWTMiddleware(cfg, keyFunc)(withScopes)
}

func TestScopeMiddleware_Disabled(t *testing.T) {
	cfg := config.Auth{Enabled: false}
	handler := scopeTestHandler(cfg, nil, []string{"read:containers"})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestScopeMiddleware_NoRequiredScopes(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers",
	})
	handler := scopeTestHandler(authCfg(), staticKeyFunc(pub), nil) // nil = no scopes injected

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestScopeMiddleware_SufficientScope(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers",
	})
	handler := scopeTestHandler(authCfg(), staticKeyFunc(pub), []string{"read:containers"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestScopeMiddleware_InsufficientScope(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers",
	})
	handler := scopeTestHandler(authCfg(), staticKeyFunc(pub), []string{"write:containers"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestScopeMiddleware_MultipleScopes_AllPresent(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers write:containers read:system",
	})
	handler := scopeTestHandler(authCfg(), staticKeyFunc(pub), []string{"read:containers", "write:containers"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestScopeMiddleware_MultipleScopes_OneMissing(t *testing.T) {
	priv, pub := testKeyPair(t)
	token := makeToken(t, priv, jwt.MapClaims{
		"iss":   "https://test-issuer",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "read:containers",
	})
	handler := scopeTestHandler(authCfg(), staticKeyFunc(pub), []string{"read:containers", "write:containers"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
```

Also add `"context"` to the import block in the test file:

```go
import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/auth"
	"github.com/bwilczynski/homelab-api/internal/config"
)
```

- [ ] **Step 2: Run new scope tests — confirm they fail as expected**

```bash
go test ./internal/auth/... -run TestScopeMiddleware -v
```

Expected: `TestScopeMiddleware_Disabled` passes; `TestScopeMiddleware_NoRequiredScopes` fails with 200 (stub passes through); others fail.

- [ ] **Step 3: Implement `ScopeMiddleware` fully — replace stub in `internal/auth/middleware.go`**

Replace the stub `ScopeMiddleware` function and remove the `fmt` import workaround:

```go
// ScopeMiddleware checks that the token scopes (from context) satisfy the
// required scopes for the operation (also from context, injected by generated code).
// It is registered via ChiServerOptions.Middlewares on each domain handler.
func ScopeMiddleware(cfg config.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
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

Also update the imports at the top of `middleware.go` — add `"fmt"` and remove the unused `var _ = fmt.Sprintf` line:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/config"
)
```

- [ ] **Step 4: Run all auth tests**

```bash
go test ./internal/auth/... -v
```

Expected: all 11 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "Implement ScopeMiddleware with deny-by-default scope enforcement"
```

---

### Task 6: Wire middlewares into the server

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Update imports in `cmd/server/main.go`**

The updated import block:

```go
import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"

	"github.com/bwilczynski/homelab-api/internal/auth"
	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/health"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)
```

- [ ] **Step 2: Initialize JWKS and middlewares after config load in `cmd/server/main.go`**

After the line `cfg, err := config.Load(configPath)` block, add:

```go
	var jwtKeyfunc keyfunc.Keyfunc
	if cfg.Auth.Enabled {
		if cfg.Auth.Issuer == "" || cfg.Auth.JWKSURL == "" {
			logger.Error("auth.issuer and auth.jwks_url are required when auth is enabled")
			os.Exit(1)
		}
		k, err := keyfunc.NewDefault([]string{cfg.Auth.JWKSURL})
		if err != nil {
			logger.Error("failed to initialize JWKS", "err", err)
			os.Exit(1)
		}
		jwtKeyfunc = *k
		logger.Info("authorization enabled", "issuer", cfg.Auth.Issuer)
	}

	jwtMw := auth.JWTMiddleware(cfg.Auth, jwtKeyfunc.Keyfunc)
	scopeMw := auth.ScopeMiddleware(cfg.Auth)
```

- [ ] **Step 3: Register `jwtMw` on the chi router**

After the existing `r.Use(httplog.RequestLogger(...))` call, add:

```go
	r.Use(jwtMw)
```

- [ ] **Step 4: Switch all 5 domains from `HandlerFromMux` to `HandlerWithOptions`**

Replace the handler registration section (all 5 domains) with:

```go
	// System: all DSM + all UniFi backends.
	dsmBackends := make(map[string]system.DSMBackendConfig, len(synologyClients))
	for name, client := range synologyClients {
		dsmBackends[name] = system.DSMBackendConfig{
			Backend:       client,
			DockerEnabled: client.SupportsContainers(),
		}
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends, monitor)
	system.HandlerWithOptions(system.NewStrictHandler(system.NewHandler(systemSvc), nil), system.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []system.MiddlewareFunc{scopeMw},
	})

	// Containers: all Synology backends; capability checked per-request via SupportsContainers.
	containerBackends := make(map[string]containers.ContainerBackend, len(synologyClients))
	for name, client := range synologyClients {
		containerBackends[name] = client
	}
	containersSvc := containers.NewService(containerBackends, monitor)
	containers.HandlerWithOptions(containers.NewStrictHandler(containers.NewHandler(containersSvc), nil), containers.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []containers.MiddlewareFunc{scopeMw},
	})

	// Storage: all Synology backends.
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends, monitor)
	storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []storage.MiddlewareFunc{scopeMw},
	})

	// Backups: all Synology backends; capability checked per-request via SupportsBackups.
	backupBackends := make(map[string]backups.BackupBackend, len(synologyClients))
	for name, client := range synologyClients {
		backupBackends[name] = client
	}
	backupsSvc := backups.NewService(backupBackends, monitor)
	backups.HandlerWithOptions(backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil), backups.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []backups.MiddlewareFunc{scopeMw},
	})

	// Network: all UniFi backends.
	networkBackends := make(map[string]network.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		networkBackends[name] = client
	}
	networkSvc := network.NewService(networkBackends, monitor)
	network.HandlerWithOptions(network.NewStrictHandler(network.NewHandler(networkSvc), nil), network.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []network.MiddlewareFunc{scopeMw},
	})
```

- [ ] **Step 5: Handle zero-value `keyfunc.Keyfunc` when auth is disabled**

When `cfg.Auth.Enabled` is `false`, `jwtKeyfunc` is a zero-value `keyfunc.Keyfunc` and its `.Keyfunc` field is `nil`. `JWTMiddleware` short-circuits before calling `keyFunc` when disabled, so `nil` is safe. Verify this with a build:

```bash
go build ./cmd/server/...
```

Expected: no errors.

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/server/main.go
git commit -m "Wire JWT and scope middlewares into server"
```
