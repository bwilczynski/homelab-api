// Package routing provides chi router middleware shared by the production
// and contract-test servers.
package routing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// EscapedPathRouting makes chi match routes against the percent-encoded path.
//
// net/url leaves URL.RawPath empty when the request path is in canonical
// encoded form, so chi falls back to routing on the decoded URL.Path and
// URL params come back already decoded. The oapi-codegen runtime then calls
// url.PathUnescape a second time, which rejects valid requests whose param
// values contain a literal '%' (and silently double-decodes others). Routing
// on EscapedPath() guarantees params are decoded exactly once, and keeps
// encoded slashes (%2F) within a single path segment.
func EscapedPathRouting(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		if rctx != nil && rctx.RoutePath == "" {
			rctx.RoutePath = r.URL.EscapedPath()
		}
		next.ServeHTTP(w, r)
	})
}
