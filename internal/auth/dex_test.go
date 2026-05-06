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
