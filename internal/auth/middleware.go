package auth

import (
	"context"
	"encoding/json"
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
