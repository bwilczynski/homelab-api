package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// dsmTestServer is a minimal httptest-backed DSM that records call counts
// and exposes the SIDs presented on each entry.cgi request.
type dsmTestServer struct {
	server     *httptest.Server
	loginCount atomic.Int32
	entryCount atomic.Int32

	mu       sync.Mutex
	seenSIDs []string
}

func newDSMTestServer(entry http.HandlerFunc) *dsmTestServer {
	d := &dsmTestServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/webapi/query.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"SYNO.API.Auth": map[string]any{"path": "auth.cgi", "maxVersion": 7},
			},
		})
	})

	mux.HandleFunc("/webapi/auth.cgi", func(w http.ResponseWriter, r *http.Request) {
		n := d.loginCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"sid": fmt.Sprintf("SID-%d", n)},
		})
	})

	mux.HandleFunc("/webapi/entry.cgi", func(w http.ResponseWriter, r *http.Request) {
		d.entryCount.Add(1)
		d.mu.Lock()
		d.seenSIDs = append(d.seenSIDs, r.URL.Query().Get("_sid"))
		d.mu.Unlock()
		entry(w, r)
	})

	d.server = httptest.NewTLSServer(mux)
	return d
}

func (d *dsmTestServer) close() { d.server.Close() }

func (d *dsmTestServer) sids() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.seenSIDs))
	copy(out, d.seenSIDs)
	return out
}

func newTestClient(t *testing.T, server *httptest.Server) *SynologyClient {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSynologyClient("test", parsed.Host, "user", "pass", "6", true, logger, time.UTC)
}

func TestSynologyClient_Call_RetriesAfterSessionExpired(t *testing.T) {
	t.Parallel()

	var entryCalls atomic.Int32
	d := newDSMTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if entryCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   map[string]any{"code": 106},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]string{"hello": "world"},
		})
	})
	defer d.close()

	client := newTestClient(t, d.server)

	data, err := client.Call("SYNO.Test", "do", "1", nil)
	if err != nil {
		t.Fatalf("Call: unexpected error: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("data: got %v, want hello=world", got)
	}

	if n := d.loginCount.Load(); n != 2 {
		t.Errorf("loginCount: got %d, want 2 (initial + post-expiry)", n)
	}
	if n := d.entryCount.Load(); n != 2 {
		t.Errorf("entryCount: got %d, want 2 (failed + retry)", n)
	}
	if sids := d.sids(); len(sids) != 2 || sids[0] != "SID-1" || sids[1] != "SID-2" {
		t.Errorf("SIDs used: got %v, want [SID-1 SID-2]", sids)
	}
}

func TestSynologyClient_Call_DoesNotRetryOnNonSessionError(t *testing.T) {
	t.Parallel()

	d := newDSMTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": dsmErrContainerNotFound},
		})
	})
	defer d.close()

	client := newTestClient(t, d.server)

	_, err := client.Call("SYNO.Docker.Container", "get", "1", nil)
	if err == nil {
		t.Fatal("Call: expected error, got nil")
	}
	var apiErr *DSMAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Call: got %T, want *DSMAPIError", err)
	}
	if apiErr.Code != dsmErrContainerNotFound {
		t.Errorf("error code: got %d, want %d", apiErr.Code, dsmErrContainerNotFound)
	}

	if n := d.loginCount.Load(); n != 1 {
		t.Errorf("loginCount: got %d, want 1 (no re-auth on non-session error)", n)
	}
	if n := d.entryCount.Load(); n != 1 {
		t.Errorf("entryCount: got %d, want 1 (no retry on non-session error)", n)
	}
}
