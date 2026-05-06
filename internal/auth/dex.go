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
