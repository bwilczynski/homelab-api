package auth_test

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
	return func(token *jwt.Token) (any, error) {
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
