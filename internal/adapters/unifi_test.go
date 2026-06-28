package adapters_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// fakeDeviceResponse is the minimal JSON the fake controller returns on success.
var fakeDeviceResponse = map[string]any{
	"meta": map[string]string{"rc": "ok"},
	"data": []any{},
}

func TestGetDevices_LegacyMode_RetriesOnce_After401(t *testing.T) {
	var deviceCalls, loginCalls atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			loginCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/api/s/default/stat/device":
			n := deviceCalls.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(fakeDeviceResponse) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	client := adapters.NewUniFiClient(host, "admin", "pass", true)

	devices, err := client.GetDevices()
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
	if got := deviceCalls.Load(); got != 2 {
		t.Errorf("expected 2 device calls (initial + retry), got %d", got)
	}
	if got := loginCalls.Load(); got != 2 {
		t.Errorf("expected 2 login calls (initial ensureSession + re-auth), got %d", got)
	}
}

func TestGetDevices_LegacyMode_RepeatedUnauthorized_ReturnsError(t *testing.T) {
	var deviceCalls atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			w.WriteHeader(http.StatusOK)
		case "/api/s/default/stat/device":
			deviceCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	client := adapters.NewUniFiClient(host, "admin", "pass", true)

	_, err := client.GetDevices()
	if err == nil {
		t.Fatal("expected error for persistent 401, got nil")
	}
	if got := deviceCalls.Load(); got != 2 {
		t.Errorf("expected exactly 2 device calls (no infinite loop), got %d", got)
	}
}

func TestGetDevices_APIKeyMode_401_ReturnsError_NoRetry(t *testing.T) {
	var deviceCalls, loginCalls atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			loginCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/api/s/default/stat/device":
			deviceCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	client := adapters.NewUniFiClientWithAPIKey(host, "wrong-key", true)

	_, err := client.GetDevices()
	if err == nil {
		t.Fatal("expected error for API key 401, got nil")
	}
	if got := deviceCalls.Load(); got != 1 {
		t.Errorf("expected 1 device call (no retry for API key), got %d", got)
	}
	if got := loginCalls.Load(); got != 0 {
		t.Errorf("expected 0 login calls (API key mode never logs in), got %d", got)
	}
}

func TestGetDevices_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			w.WriteHeader(http.StatusOK)
		case "/api/s/default/stat/device":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	client := adapters.NewUniFiClient(host, "admin", "pass", true)

	_, err := client.GetDevices()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
