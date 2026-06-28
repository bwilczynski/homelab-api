# UniFi OS Migration Design

**Date:** 2026-06-28
**Status:** Approved

## Background

The UniFi adapter (`internal/adapters/unifi.go`) was written against a legacy standalone Network Controller where the API lives directly at `/api/s/{site}/...`. The homelab has since fully migrated to a Cloud Gateway Fiber running UniFi OS, where the Network application is sandboxed and proxied at `/proxy/network/`. All legacy paths now return 404 on UniFi OS.

A previous session added API key authentication (`X-API-KEY` header, `NewUniFiClientWithAPIKey()`) and a `maybeLogin()` no-op guard, but left all endpoint paths unchanged. That is why API key auth passes but real requests fail.

### Verified via probe

A diagnostic script (`scripts/unifi-probe-api.sh`) confirmed against the live device:

| Path family | HTTP status |
|---|---|
| `/api/s/default/stat/device` (legacy) | 404 |
| `/proxy/network/api/s/default/stat/device` (UniFi OS) | 200 |
| `/v2/api/site/default/clients/active` (legacy) | 200 HTML (SPA catch-all) |
| `/proxy/network/v2/api/site/default/clients/active` (UniFi OS) | 200 JSON |

Response envelopes are identical between old and new paths — v1 uses `{"meta":{"rc":"ok"},"data":[...]}`, v2 uses a bare array `[...]`. No Go struct changes are needed.

## Goals

1. Make all 7 API endpoints work against UniFi OS (Cloud Gateway Fiber).
2. Preserve backward compatibility with legacy standalone controllers.
3. Fix the per-request session login — currently `login()` is called on every request.
4. Match the Synology session caching pattern (double-checked locking, silent re-auth + single retry on 401).
5. Replace test fixtures with sanitized real responses from the live device.
6. Update shell scripts to use the API key and proxy paths.

## Non-goals

- Auto-detection of controller type at startup.
- Support for username/password auth against UniFi OS (Ubiquiti supports it but we don't need it).
- Changes to the network service layer, handlers, or OpenAPI spec.

## Approach

**Single `UniFiClient` struct, mode inferred from auth method.** API key present → UniFi OS mode (proxy prefix). No API key → legacy mode (no prefix, session auth). No new config field required; API keys are a UniFi OS-only feature, so the auth method is a reliable proxy for controller type.

## Design

### 1. `UniFiClient` struct

New fields added to the existing struct:

```go
type UniFiClient struct {
    host        string
    user        string
    pass        string
    apiKey      string       // set by NewUniFiClientWithAPIKey; empty for legacy
    insecureTLS bool
    client      *http.Client
    mu          sync.RWMutex // guards loggedIn
    loggedIn    bool         // legacy mode only; true when cookie jar holds a live session
}
```

The `apiKey` field and `NewUniFiClientWithAPIKey()` constructor are already present in the working copy. `mu` and `loggedIn` are new.

### 2. Path routing

```go
func (c *UniFiClient) pathPrefix() string {
    if c.apiKey != "" {
        return "/proxy/network"
    }
    return ""
}
```

All 7 endpoint paths prepend `c.pathPrefix()`:

| Method | Old path | New path (UniFi OS) |
|---|---|---|
| `GetDevices` | `/api/s/default/stat/device` | `/proxy/network/api/s/default/stat/device` |
| `GetClients` | `/api/s/default/stat/sta` | `/proxy/network/api/s/default/stat/sta` |
| `GetHealth` | `/api/s/default/stat/health` | `/proxy/network/api/s/default/stat/health` |
| `GetWlanConf` | `/api/s/default/rest/wlanconf` | `/proxy/network/api/s/default/rest/wlanconf` |
| `GetNetworkConf` | `/api/s/default/rest/networkconf` | `/proxy/network/api/s/default/rest/networkconf` |
| `fetchActiveClients` | `/v2/api/site/default/clients/active` | `/proxy/network/v2/api/site/default/clients/active` |
| `fetchOfflineClients` | `/v2/api/site/default/clients/history` | `/proxy/network/v2/api/site/default/clients/history` |

Legacy mode (`apiKey == ""`) prepends `""` — paths are unchanged, preserving full backward compatibility.

### 3. Session management (legacy mode)

Mirrors the `SynologyClient` pattern exactly.

**`loginLocked()`** — the existing `login()` body, renamed. Called only while holding the write lock. Sets `c.loggedIn = true` on success.

**`ensureSession()`** — double-checked locking:

```
RLock → if loggedIn → RUnlock → return   (fast path)
RUnlock
Lock
  if loggedIn → Unlock → return          (another goroutine beat us)
  loginLocked()
  set loggedIn = true
Unlock
```

**`invalidateSession()`** — acquires the write lock, sets `loggedIn = false`, and immediately calls `loginLocked()` to re-authenticate as a single atomic operation. This prevents the Synology-style race where two goroutines both observe an expired session and both try to re-login — the second one finds `loggedIn == true` already and exits. Because UniFi uses a cookie jar (no version token to compare), the lock-during-reauth approach is used instead of Synology's compare-and-clear pattern.

**`maybeLogin()`** — kept as a thin wrapper:

```go
func (c *UniFiClient) maybeLogin() error {
    if c.apiKey != "" {
        return nil
    }
    return c.ensureSession()
}
```

### 4. `get()` — HTTP status check and 401 retry

Current `get()` has two gaps: it does not check the HTTP status code before unmarshalling, and it has no retry logic. Both are fixed.

New behaviour:

1. Call `maybeLogin()`.
2. Build request; set `X-API-KEY` header if `apiKey` is set (already in working copy).
3. Execute request.
4. **Non-2xx, non-401**: return a descriptive error (`"unifi request: status %d"`). This catches 404s and other errors that previously silently produced a JSON parse failure.
5. **401 in legacy mode** (`apiKey == ""`): call `invalidateSession()` (which re-authenticates under the write lock), rebuild and re-execute the request once. If 401 again, return error — no further looping.
6. **401 in API key mode**: return error immediately with a clear message (`"unifi request: invalid API key (401)"`). API keys don't expire; a 401 means the key is wrong.
7. Read body, unmarshal into `out`.

### 5. Testing

**Existing fixture-based unit tests** (`internal/network/service_test.go`) are unchanged — they test the service layer via `mockUniFiBackend` and do not touch HTTP transport.

**Test fixtures** (`internal/network/testdata/*.json`) — six files replaced with sanitized versions of the real UniFi OS responses captured from the live device (`scripts/responses/unifi-*-os-raw.json`). Sanitization per CLAUDE.md rules: IPs → `192.168.1.10/11/...`, MACs → `aa:bb:cc:dd:ee:01/02/...`, hostnames → `host-01/02/...`, tokens/keys → `REDACTED`. Device model names and software versions kept as-is.

**Integration test** (`internal/adapters/unifi_test.go`) — new file using `httptest.NewServer`. A fake controller tracks per-endpoint call counts and flips behaviour after the first hit. Covers:

- Legacy mode, session expiry: first call → `401` → client re-authenticates → retry → `200` with valid JSON → success.
- Legacy mode, repeated 401: re-auth succeeds but endpoint returns `401` again → error returned, no infinite loop.
- API key mode, 401: returns error immediately, no retry, no login attempt.
- Non-2xx (e.g. 500): returns a descriptive error without attempting unmarshal.

The fake server serves inline JSON; no fixture files are used in the integration test.

### 6. Shell scripts

All six existing `scripts/unifi-*.sh` files updated:
- Remove `COOKIE_JAR`, `curl ... /api/login`, `curl ... /api/logout`.
- Replace with single `curl -H "X-API-KEY: ${UNIFI_API_KEY}"` per endpoint.
- Update all paths to include `/proxy/network/` prefix.
- Require `UNIFI_HOST` in addition to `UNIFI_API_KEY` (add to `.env`).

The diagnostic scripts added this session (`unifi-probe-api.sh`, `unifi-capture-responses.sh`) are kept as-is.

## Files changed

| File | Change |
|---|---|
| `internal/adapters/unifi.go` | Add `mu`, `loggedIn`; add `pathPrefix()`, `loginLocked()`, `ensureSession()`, `invalidateSession()`; update `maybeLogin()` and `get()`; prepend prefix on all 7 endpoints |
| `internal/adapters/unifi_test.go` | New — integration tests for session retry and API key 401 handling |
| `internal/network/testdata/*.json` | Replace 6 fixtures with sanitized real responses |
| `scripts/unifi-*.sh` | Update 6 scripts to API key + proxy paths |
| `.env` | Add `UNIFI_HOST` variable |

No changes to: `internal/config/`, `cmd/server/backends.go`, `internal/network/` service/handler files, OpenAPI spec, or CI.

Note: `internal/config/config.go`, `cmd/server/backends.go`, and `config.sample.yaml` already contain the working copy changes (APIKey field, NewUniFiClientWithAPIKey branching, sample config comments) and require no further modification.
