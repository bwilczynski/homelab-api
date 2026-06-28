# UniFi OS Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the UniFi adapter from legacy standalone-controller API paths to UniFi OS (Cloud Gateway Fiber) by adding `/proxy/network/` prefix routing and fixing per-request session login.

**Architecture:** Single `UniFiClient` struct gains a `pathPrefix()` method that returns `/proxy/network` when an API key is configured (UniFi OS mode) and `""` otherwise (legacy mode). Session login is refactored with double-checked locking (`ensureSession`) and silent re-auth on 401, matching the existing Synology client pattern. A new integration test file exercises the retry and path-routing behaviours end-to-end against an httptest TLS server.

**Tech Stack:** Go 1.24, `net/http`, `net/http/httptest`, `sync.RWMutex`, `encoding/json`

---

## MAC mapping (used across all tasks)

The following sanitised MAC addresses are used consistently across all fixture files. Any real MAC not in this table is a client device MAC — use the convention `{first-byte}:{second-byte}:{third-byte}:aa:bb:NN` where NN increments per client.

| Real MAC | Device | Sanitised MAC |
|---|---|---|
| `e0:63:da:e1:ce:9a` | US 8 | `aa:bb:cc:dd:01:01` |
| `18:e8:29:fd:71:95` | UAP-02 | `aa:bb:cc:dd:02:01` |
| `70:a7:41:c1:41:c3` | Switch Flex Mini | `aa:bb:cc:dd:03:01` |
| `18:e8:29:fd:72:21` | UAP-01 | `aa:bb:cc:dd:04:01` |
| `a8:9c:6c:80:7c:c6` | USW Flex 2.5G 8 | `aa:bb:cc:dd:05:01` |
| `74:83:c2:dd:95:e9` | US 8 60W | `aa:bb:cc:dd:06:01` |
| `a8:9c:6c:88:41:78` | CGF-01 | `aa:bb:cc:dd:07:01` |

Old fixture device MACs to replace in client fixtures:

| Old sanitised MAC | Was | New sanitised MAC |
|---|---|---|
| `aa:bb:cc:dd:00:04` | UAP-01 | `aa:bb:cc:dd:04:01` |
| `aa:bb:cc:dd:00:02` | US 8 60W | `aa:bb:cc:dd:06:01` |
| `aa:bb:cc:dd:00:03` | Switch Flex Mini | `aa:bb:cc:dd:03:01` |

---

## Task 1: Write integration tests

**Files:**
- Create: `internal/adapters/unifi_test.go`

- [ ] **Create the test file:**

```go
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
```

---

## Task 2: Confirm tests fail

**Files:** none

- [ ] **Run the new tests:**

```bash
go test ./internal/adapters/... -run TestGetDevices -v
```

Expected: all four tests fail — `TestGetDevices_LegacyMode_RetriesOnce_After401` fails because there is no retry; `TestGetDevices_APIKeyMode_401_ReturnsError_NoRetry` fails because the path hits 404 (no proxy prefix); `TestGetDevices_Non2xx_ReturnsError` may pass or fail depending on current error path. All are expected to fail until Task 6.

---

## Task 3: Add session fields and loginLocked()

**Files:**
- Modify: `internal/adapters/unifi.go`

The existing `login()` method is called on every request. Rename it to `loginLocked()` (called only while holding the write lock), add `c.loggedIn = true` on success, and add `mu` + `loggedIn` fields to the struct.

- [ ] **Add `sync` import:**

In the import block at the top of `unifi.go`, add `"sync"`:

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)
```

- [ ] **Add `mu` and `loggedIn` to the struct:**

```go
// UniFiClient handles authentication and API calls to the UniFi Controller.
type UniFiClient struct {
	host        string
	user        string
	pass        string
	apiKey      string
	insecureTLS bool
	client      *http.Client
	mu          sync.RWMutex // guards loggedIn
	loggedIn    bool         // legacy mode only; true when cookie jar holds a live session
}
```

- [ ] **Rename `login()` to `loginLocked()` and add `c.loggedIn = true`:**

Replace the entire `login()` function:

```go
// loginLocked authenticates with the UniFi Controller using session auth.
// Must be called while holding c.mu (write lock). Sets c.loggedIn on success.
func (c *UniFiClient) loginLocked() error {
	body, _ := json.Marshal(map[string]string{
		"username": c.user,
		"password": c.pass,
	})
	resp, err := c.client.Post(
		fmt.Sprintf("https://%s/api/login", c.host),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("unifi login: %w", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unifi login: status %d", resp.StatusCode)
	}
	c.loggedIn = true
	return nil
}
```

---

## Task 4: Add ensureSession() and invalidateSession()

**Files:**
- Modify: `internal/adapters/unifi.go`

- [ ] **Add `ensureSession()` after `loginLocked()`:**

```go
// ensureSession returns once a valid session exists in the cookie jar.
// Uses double-checked locking: fast path for already-logged-in calls.
func (c *UniFiClient) ensureSession() error {
	c.mu.RLock()
	loggedIn := c.loggedIn
	c.mu.RUnlock()
	if loggedIn {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loggedIn {
		return nil
	}
	return c.loginLocked()
}
```

- [ ] **Add `invalidateSession()` after `ensureSession()`:**

```go
// invalidateSession clears the cached session flag and immediately re-authenticates
// under the write lock. Holding the lock across clear+relogin prevents concurrent
// goroutines from each attempting their own re-login after the same expired session.
func (c *UniFiClient) invalidateSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loggedIn = false
	return c.loginLocked()
}
```

---

## Task 5: Update maybeLogin() and add pathPrefix()

**Files:**
- Modify: `internal/adapters/unifi.go`

- [ ] **Replace `maybeLogin()` to delegate to `ensureSession()`:**

```go
// maybeLogin ensures a session exists when using session-based auth.
// It is a no-op when the client is configured with an API key.
func (c *UniFiClient) maybeLogin() error {
	if c.apiKey != "" {
		return nil
	}
	return c.ensureSession()
}
```

- [ ] **Add `pathPrefix()` after `maybeLogin()`:**

```go
// pathPrefix returns the URL prefix for the Network application API.
// UniFi OS (API key mode) proxies the Network app at /proxy/network/.
// Legacy standalone controllers expose the API directly with no prefix.
func (c *UniFiClient) pathPrefix() string {
	if c.apiKey != "" {
		return "/proxy/network"
	}
	return ""
}
```

---

## Task 6: Rewrite get() with status check and 401 retry

**Files:**
- Modify: `internal/adapters/unifi.go`

Replace the entire `get()` function. The new version calls `maybeLogin()`, checks HTTP status before unmarshalling, and retries once after re-authentication on 401 (legacy mode only).

- [ ] **Replace `get()`:**

```go
// get performs an authenticated GET request against the UniFi API and decodes the response.
// For legacy session auth, it retries once after re-authenticating on a 401 response.
func (c *UniFiClient) get(path string, out any) error {
	if err := c.maybeLogin(); err != nil {
		return err
	}

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s%s", c.host, path), nil)
		if err != nil {
			return nil, fmt.Errorf("unifi request: %w", err)
		}
		if c.apiKey != "" {
			req.Header.Set("X-API-KEY", c.apiKey)
		}
		return c.client.Do(req)
	}

	resp, err := doRequest()
	if err != nil {
		return fmt.Errorf("unifi request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		if c.apiKey != "" {
			return fmt.Errorf("unifi request: invalid API key (401)")
		}
		if err := c.invalidateSession(); err != nil {
			return fmt.Errorf("unifi re-auth: %w", err)
		}
		resp, err = doRequest()
		if err != nil {
			return fmt.Errorf("unifi request: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return fmt.Errorf("unifi request: unauthorized after re-auth")
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unifi request: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("unifi read body: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unifi parse response: %w", err)
	}
	return nil
}
```

---

## Task 7: Remove maybeLogin() from endpoint methods and add pathPrefix() to all paths

**Files:**
- Modify: `internal/adapters/unifi.go`

`get()` now calls `maybeLogin()` internally. Remove the redundant call from each public method. Add `c.pathPrefix()` prefix to all 7 endpoint paths.

- [ ] **Update `GetDevices()`:**

```go
func (c *UniFiClient) GetDevices() ([]UniFiDevice, error) {
	var result unifiResponse[[]UniFiDevice]
	if err := c.get(c.pathPrefix()+"/api/s/default/stat/device", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

- [ ] **Update `GetClients()`:**

```go
func (c *UniFiClient) GetClients() ([]UniFiSta, error) {
	var result unifiResponse[[]UniFiSta]
	if err := c.get(c.pathPrefix()+"/api/s/default/stat/sta", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

- [ ] **Update `fetchActiveClients()`:**

```go
func (c *UniFiClient) fetchActiveClients() ([]UniFiClientV2, error) {
	var result []UniFiClientV2
	if err := c.getV2(c.pathPrefix()+"/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false", &result); err != nil {
		return nil, err
	}
	return result, nil
}
```

- [ ] **Update `fetchOfflineClients()`:**

```go
func (c *UniFiClient) fetchOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	path := fmt.Sprintf(c.pathPrefix()+"/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=%d", historyDays*24)
	var result []UniFiClientV2
	if err := c.getV2(path, &result); err != nil {
		return nil, err
	}
	return result, nil
}
```

- [ ] **Update `GetActiveClients()` — remove maybeLogin():**

```go
func (c *UniFiClient) GetActiveClients() ([]UniFiClientV2, error) {
	return c.fetchActiveClients()
}
```

- [ ] **Update `GetOfflineClients()` — remove maybeLogin():**

```go
func (c *UniFiClient) GetOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	return c.fetchOfflineClients(historyDays)
}
```

- [ ] **Update `GetAllClients()` — remove maybeLogin():**

```go
func (c *UniFiClient) GetAllClients(historyDays int) ([]UniFiClientV2, error) {
	active, err := c.fetchActiveClients()
	if err != nil {
		return nil, fmt.Errorf("fetch active clients: %w", err)
	}
	offline, err := c.fetchOfflineClients(historyDays)
	if err != nil {
		return nil, fmt.Errorf("fetch offline clients: %w", err)
	}
	return append(active, offline...), nil
}
```

- [ ] **Update `GetHealth()` — remove maybeLogin(), add pathPrefix():**

```go
func (c *UniFiClient) GetHealth() ([]UniFiSubsystemHealth, error) {
	var result unifiResponse[[]UniFiSubsystemHealth]
	if err := c.get(c.pathPrefix()+"/api/s/default/stat/health", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

- [ ] **Update `GetWlanConf()` — remove maybeLogin(), add pathPrefix():**

```go
func (c *UniFiClient) GetWlanConf() ([]UniFiWlanConf, error) {
	var result unifiResponse[[]UniFiWlanConf]
	if err := c.get(c.pathPrefix()+"/api/s/default/rest/wlanconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

- [ ] **Update `GetNetworkConf()` — remove maybeLogin(), add pathPrefix():**

```go
func (c *UniFiClient) GetNetworkConf() ([]UniFiNetworkConf, error) {
	var result unifiResponse[[]UniFiNetworkConf]
	if err := c.get(c.pathPrefix()+"/api/s/default/rest/networkconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

---

## Task 8: Run integration tests

**Files:** none

- [ ] **Run integration tests:**

```bash
go test ./internal/adapters/... -run TestGetDevices -v
```

Expected output (all four tests pass):
```
--- PASS: TestGetDevices_LegacyMode_RetriesOnce_After401 (0.00s)
--- PASS: TestGetDevices_LegacyMode_RepeatedUnauthorized_ReturnsError (0.00s)
--- PASS: TestGetDevices_APIKeyMode_401_ReturnsError_NoRetry (0.00s)
--- PASS: TestGetDevices_Non2xx_ReturnsError (0.00s)
PASS
```

- [ ] **Run the full adapter package:**

```bash
go test ./internal/adapters/... -v
```

Expected: all tests pass.

---

## Task 9: Run full suite — confirm network tests now fail

**Files:** none

- [ ] **Run:**

```bash
go test ./... 2>&1 | head -40
```

Expected: `internal/network` tests fail because fixtures still have old device data (5 devices, old MACs, old gateway ID `unifi.usg-3p`). The adapter and other packages pass. This confirms the scope of Task 11.

---

## Task 10: Commit adapter changes

**Files:** none

- [ ] **Stage and commit:**

```bash
git add internal/adapters/unifi.go internal/adapters/unifi_test.go
git diff --cached --name-only | grep gen.go  # must print nothing
git commit -m "feat: migrate UniFi adapter to UniFi OS proxy paths with session caching"
```

---

## Task 11: Replace test fixtures

**Files:**
- Modify: `internal/network/testdata/unifi-devices.json`
- Modify: `internal/network/testdata/unifi-clients.json`
- Modify: `internal/network/testdata/unifi-v2-active.json`
- Modify: `internal/network/testdata/unifi-v2-history.json`
- Modify: `internal/network/testdata/unifi-wlanconf.json`
- Modify: `internal/network/testdata/unifi-networkconf.json`

Raw source files: `scripts/responses/unifi-*-os-raw.json` (captured from the live device).

### 11a — Device fixture

- [ ] **Sanitise the device fixture:**

Apply MAC and sensitive-field replacements. Run from the repo root:

```bash
cat scripts/responses/unifi-devices-os-raw.json \
  | sed \
    -e 's/e0:63:da:e1:ce:9a/aa:bb:cc:dd:01:01/g' \
    -e 's/18:e8:29:fd:71:95/aa:bb:cc:dd:02:01/g' \
    -e 's/70:a7:41:c1:41:c3/aa:bb:cc:dd:03:01/g' \
    -e 's/18:e8:29:fd:72:21/aa:bb:cc:dd:04:01/g' \
    -e 's/a8:9c:6c:80:7c:c6/aa:bb:cc:dd:05:01/g' \
    -e 's/74:83:c2:dd:95:e9/aa:bb:cc:dd:06:01/g' \
    -e 's/a8:9c:6c:88:41:78/aa:bb:cc:dd:07:01/g' \
  | jq '
      .data[].x_authkey = "REDACTED" |
      .data[].x_fingerprint = "REDACTED" |
      .data[].x_ssh_hostkey_fingerprint = "REDACTED" |
      .data[].x_aes_gcm = "REDACTED" |
      .data[].syslog_key = "REDACTED" |
      .data[].anon_id = "REDACTED" |
      .data[].hash_id = "REDACTED" |
      (.data[] | select(.wan1 != null)).wan1.ip = "203.0.113.1" |
      .data[].ip |= if test("^192\\.168\\.0\\.") then gsub("^192\\.168\\.0\\."; "192.168.1.") else . end |
      .data[].inform_ip = "192.168.1.1" |
      .data[].inform_url = "https://192.168.1.1:8080/inform" |
      .data[].connect_request_ip = "192.168.1.1"
    ' \
  > internal/network/testdata/unifi-devices.json
```

- [ ] **Verify top-level key sets match:**

```bash
diff \
  <(jq 'keys' scripts/responses/unifi-devices-os-raw.json) \
  <(jq 'keys' internal/network/testdata/unifi-devices.json)
```

Expected: no diff output.

### 11b — Sta (v1 clients) fixture

The sta fixture keeps the same client entries; only the device MAC references (`ap_mac`, `sw_mac`) are updated to the new sanitised device MACs.

- [ ] **Update device MAC references:**

```bash
cat internal/network/testdata/unifi-clients.json \
  | sed \
    -e 's/aa:bb:cc:dd:00:04/aa:bb:cc:dd:04:01/g' \
    -e 's/aa:bb:cc:dd:00:02/aa:bb:cc:dd:06:01/g' \
    -e 's/aa:bb:cc:dd:00:03/aa:bb:cc:dd:03:01/g' \
  > /tmp/unifi-clients-updated.json && mv /tmp/unifi-clients-updated.json internal/network/testdata/unifi-clients.json
```

### 11c — v2 active client fixture

Keep the same three client entries; update `last_uplink_mac` to new device MACs and add representative new fields from the real API response.

- [ ] **Update v2-active fixture** by editing `internal/network/testdata/unifi-v2-active.json`:

Apply the MAC replacements and add new fields present in the real API:

```bash
cat internal/network/testdata/unifi-v2-active.json \
  | sed \
    -e 's/aa:bb:cc:dd:00:04/aa:bb:cc:dd:04:01/g' \
    -e 's/aa:bb:cc:dd:00:02/aa:bb:cc:dd:06:01/g' \
    -e 's/aa:bb:cc:dd:00:03/aa:bb:cc:dd:03:01/g' \
  > /tmp/v2-active-tmp.json && mv /tmp/v2-active-tmp.json internal/network/testdata/unifi-v2-active.json
```

Then add new API fields to the first entry by editing the file. The real UniFi OS v2 API adds these fields not present in the old fixture. Open `internal/network/testdata/unifi-v2-active.json` and add the following to each entry under `"data"`:

```json
"anomalies": -1,
"authorized": true,
"blocked": false,
"noted": false,
"type": "CLIENT",
"virtual_network_override_enabled": false,
"is_allowed_in_visual_programming": false,
"is_mlo": false,
"network_members_group_ids": [],
"tags": []
```

### 11d — v2 history client fixture

- [ ] **Update v2-history fixture:**

```bash
cat internal/network/testdata/unifi-v2-history.json \
  | sed \
    -e 's/aa:bb:cc:dd:00:04/aa:bb:cc:dd:04:01/g' \
    -e 's/aa:bb:cc:dd:00:02/aa:bb:cc:dd:06:01/g' \
    -e 's/aa:bb:cc:dd:00:03/aa:bb:cc:dd:03:01/g' \
  > /tmp/v2-hist-tmp.json && mv /tmp/v2-hist-tmp.json internal/network/testdata/unifi-v2-history.json
```

Add the same new fields to each entry as in 11c.

### 11e — WLAN conf fixture

- [ ] **Sanitise wlanconf:**

```bash
cat scripts/responses/unifi-wlanconf-os-raw.json \
  | jq 'del(.data[].x_iapp_key, .data[].x_passphrase)' \
  > internal/network/testdata/unifi-wlanconf.json
```

- [ ] **Verify:**

```bash
diff \
  <(jq 'keys' scripts/responses/unifi-wlanconf-os-raw.json) \
  <(jq 'keys' internal/network/testdata/unifi-wlanconf.json)
```

### 11f — Network conf fixture

- [ ] **Sanitise networkconf (no sensitive fields to remove):**

```bash
cp scripts/responses/unifi-networkconf-os-raw.json \
   internal/network/testdata/unifi-networkconf.json
```

- [ ] **Verify:**

```bash
diff \
  <(jq 'keys' scripts/responses/unifi-networkconf-os-raw.json) \
  <(jq 'keys' internal/network/testdata/unifi-networkconf.json)
```

---

## Task 12: Update service test assertions

**Files:**
- Modify: `internal/network/service_test.go`

The new device fixture has 7 devices (was 5) in this order: US 8, UAP-02, Switch Flex Mini, UAP-01, USW Flex 2.5G 8, US 8 60W, CGF-01. The gateway is now CGF-01 (type `udm`, model `UDMA6A8`) at index 6.

### 12a — TestListDevices

- [ ] **Update device count, gateway lookup, switch lookup, AP lookup, and remove offline assertion:**

```go
func TestListDevices(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30, slog.Default(), nil)

	result, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 7 {
		t.Fatalf("expected 7 devices, got %d", len(result.Items))
	}

	// CGF-01 is the gateway (type udm), at index 6 in fixture order
	gw := result.Items[6]
	if gw.Id != "unifi.cgf-01" {
		t.Errorf("expected id unifi.cgf-01, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.cgf-01" {
		t.Errorf("expected uri /network/devices/unifi.cgf-01, got %s", gw.Uri)
	}
	if gw.Type != NetworkDeviceTypeGateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != NetworkDeviceStatusConnected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}

	// US 8 60W is at index 5
	sw := result.Items[5]
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Uri != "/network/devices/unifi.us-8-60w" {
		t.Errorf("expected uri /network/devices/unifi.us-8-60w, got %s", sw.Uri)
	}
	if sw.Type != NetworkDeviceTypeSwitch {
		t.Errorf("expected type switch, got %s", sw.Type)
	}

	// UAP-01 is at index 3
	ap := result.Items[3]
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Type != NetworkDeviceTypeAccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}
}
```

### 12b — TestGetDevice_Gateway

- [ ] **Update all gateway assertions to CGF-01:**

```go
func TestGetDevice_Gateway(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30, slog.Default(), nil)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.cgf-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected device to be found")
	}

	gw, err := detail.AsGatewayDetail()
	if err != nil {
		t.Fatalf("expected gateway detail: %v", err)
	}
	if gw.Id != "unifi.cgf-01" {
		t.Errorf("expected id unifi.cgf-01, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.cgf-01" {
		t.Errorf("expected uri, got %s", gw.Uri)
	}
	if gw.Model != "UDMA6A8" {
		t.Errorf("expected model UDMA6A8, got %s", gw.Model)
	}
	if gw.FirmwareVersion != "5.1.19.33549" {
		t.Errorf("expected version 5.1.19.33549, got %s", gw.FirmwareVersion)
	}
	if gw.Uptime != 5627 {
		t.Errorf("expected uptime 5627, got %d", gw.Uptime)
	}
	if gw.Uplink != nil {
		t.Errorf("expected nil uplink for gateway, got %v", gw.Uplink)
	}
	// Traffic comes from wan1 fields for udm type
	if gw.Traffic.TxBytesTotal != 37143304 {
		t.Errorf("expected tx_bytes 37143304, got %d", gw.Traffic.TxBytesTotal)
	}
	if gw.Traffic.RxBytesPerSec != 946 {
		t.Errorf("expected rx_bytes-r 946, got %d", gw.Traffic.RxBytesPerSec)
	}
}
```

### 12c — TestGetDeviceNotFound and TestGetDeviceWrongController

These tests look up `"unifi.nonexistent"` and `"other.usg-3p"` respectively — neither exists in either old or new fixture, so they pass unchanged. No edit needed.

### 12d — TestGetDevice_Switch

- [ ] **Update switch traffic and uplink assertions:**

Four assertions have changed — update them:

```go
// Line: p1.Traffic.TxBytesTotal — port 1 tx_bytes in new fixture
if p1.Traffic.TxBytesTotal != 107288538 {
    t.Errorf("expected port 1 tx_bytes 107288538, got %d", p1.Traffic.TxBytesTotal)
}
// Line: sw.Traffic.TxBytesTotal — device tx_bytes in new fixture
if sw.Traffic.TxBytesTotal != 60801611 {
    t.Errorf("expected device tx_bytes 60801611, got %d", sw.Traffic.TxBytesTotal)
}
// Line: port 5 PoePowerWatts — was 3.00, now 2.88
if p5.PoePowerWatts == nil || *p5.PoePowerWatts != 2.88 {
    t.Errorf("expected poe power 2.88, got %v", p5.PoePowerWatts)
}
// Line: sw.Uplink.Device.Id — US 8 60W now uplinks to CGF-01
if sw.Uplink.Device.Id != "unifi.cgf-01" {
    t.Errorf("expected uplink device unifi.cgf-01, got %s", sw.Uplink.Device.Id)
}
```

Port 1 state (up), link speed (gbe1), port 4 state (down), port count (8), and model (US8P60) are unchanged — leave those assertions as-is.

### 12e — TestGetDevice_AccessPoint

AP model (`U7LT`), uplink device (`unifi.us-8-60w`), and uplink port (`5`) are unchanged. The `ap_mac` in `unifi-clients.json` was updated in Task 11b to `aa:bb:cc:dd:04:01`, which matches the new UAP-01 device MAC. No assertion changes needed.

### 12f — Topology tests: gateway is not an edge source

- [ ] **Update the gateway-not-a-source check:**

```go
// was: if pe.Source.Id == "unifi.usg-3p"
for _, e := range topo.Edges {
    pe := parseTopologyEdge(t, e)
    if pe.Source.Id == "unifi.cgf-01" {
        t.Error("gateway should not be an edge source")
    }
}
```

### 12g — Topology tests: US 8 60W → CGF-01 edge

- [ ] **Update us8Edge assertions:**

```go
// was: us8Edge.Target.Id != "unifi.usg-3p"
if us8Edge.Target.Id != "unifi.cgf-01" {
    t.Errorf("US8 edge target: expected unifi.cgf-01, got %s", us8Edge.Target.Id)
}
// was: us8Edge.Port != nil (old fixture had null uplink_remote_port)
// new fixture: CGF-01 is at uplink_remote_port=1 for US 8 60W
if us8Edge.Port == nil || *us8Edge.Port != 1 {
    t.Errorf("US8 edge port: expected 1, got %v", us8Edge.Port)
}
```

Remove the old comment about "nil (no uplink_remote_port in fixture)".

---

## Task 13: Run service tests

**Files:** none

- [ ] **Run:**

```bash
go test ./internal/network/... -v 2>&1 | tail -30
```

Expected: all tests pass. If any fail, the error message will include the actual vs. expected value — compare against the device fixture to fix the assertion.

- [ ] **Run the full suite:**

```bash
go test ./... 
```

Expected: `ok` for all packages.

---

## Task 14: Commit fixture and test changes

**Files:** none

- [ ] **Stage and commit:**

```bash
git add internal/network/testdata/ internal/network/service_test.go
git diff --cached --name-only | grep gen.go  # must print nothing
git commit -m "test: replace UniFi fixtures with sanitised real UniFi OS responses"
```

---

## Task 15: Update shell scripts

**Files:**
- Modify: `scripts/unifi-devices.sh`
- Modify: `scripts/unifi-clients.sh`
- Modify: `scripts/unifi-clients-all.sh`
- Modify: `scripts/unifi-health.sh`
- Modify: `scripts/unifi-networkconf.sh`
- Modify: `scripts/unifi-wlanconf.sh`

Each script currently POSTs to `/api/login`, stores a cookie jar, and calls legacy paths. Replace with a single API-key authenticated curl to the proxy path.

- [ ] **`scripts/unifi-devices.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/device" | jq .
```

- [ ] **`scripts/unifi-clients.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/sta" | jq .
```

- [ ] **`scripts/unifi-clients-all.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

RESPONSES_DIR="$(dirname "$0")/responses"

echo "=== /proxy/network/stat/sta (active clients v1) ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/sta" \
  | tee "${RESPONSES_DIR}/unifi-sta-raw.json" \
  | jq '{count: (.data|length), keys: (.data[0]|keys)}' 2>/dev/null || true

echo "" >&2
echo "=== /proxy/network/v2/clients/active ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false" \
  | tee "${RESPONSES_DIR}/unifi-v2-active-raw.json" \
  | jq '{count: length, keys: (.[0]|keys)}' 2>/dev/null || true

echo "" >&2
echo "=== /proxy/network/v2/clients/history ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=720" \
  | tee "${RESPONSES_DIR}/unifi-v2-history-raw.json" \
  | jq '{count: length, keys: (.[0]|keys)}' 2>/dev/null || true
```

- [ ] **`scripts/unifi-health.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/health" | jq .
```

- [ ] **`scripts/unifi-networkconf.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/rest/networkconf" | jq .
```

- [ ] **`scripts/unifi-wlanconf.sh`:**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/rest/wlanconf" | jq .
```

---

## Task 16: Add UNIFI_HOST to .env

**Files:**
- Modify: `.env`

- [ ] **Add the variable** (replace with your actual gateway IP):

```bash
echo 'UNIFI_HOST=192.168.1.1' >> .env
```

- [ ] **Verify scripts work:**

```bash
bash scripts/unifi-health.sh
```

Expected: JSON with `"meta": {"rc": "ok"}` and health data for wlan, wan, www, lan subsystems.

---

## Task 17: Commit script changes

**Files:** none

- [ ] **Stage and commit:**

```bash
git add scripts/unifi-devices.sh scripts/unifi-clients.sh scripts/unifi-clients-all.sh \
        scripts/unifi-health.sh scripts/unifi-networkconf.sh scripts/unifi-wlanconf.sh \
        .env
git diff --cached --name-only | grep gen.go  # must print nothing
git commit -m "chore: update UniFi scripts to API key auth and proxy paths"
```
