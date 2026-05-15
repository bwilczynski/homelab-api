# Offline Clients Support

**Date:** 2026-05-15
**Status:** Approved

## Problem

The `GET /network/clients` endpoint currently returns only active (online) clients. Devices that should always be connected — Sonos speakers, NAS, always-on IoT — are invisible when they go offline. There is no way to detect a disconnection through the API.

## Goal

Return both online and offline clients from the list and detail endpoints, with enough information to monitor presence of always-on devices.

## Approach

Two UniFi v2 API calls per backend, results combined:

- `/v2/api/site/default/clients/active` — 24 currently connected clients; includes current `ip`, `uptime`, `essid`, `signal`, `last_uplink_name`
- `/v2/api/site/default/clients/history` — 35 historically seen but currently offline clients; includes `last_ip`, `last_uplink_name`, `name`, `hostname`, `is_wired`, `last_seen`

The existing v1 `GetClients()` call (`/stat/sta`) is kept unchanged for the detail path so online detail tests are unaffected.

Name resolution for IDs uses `name → hostname → mac` (same as today), not `display_name`, so client IDs remain stable for clients with user-assigned aliases.

## Spec Changes (`homelab-api-spec` repo)

### `NetworkClientConnectionType.yaml`
No change. Enum stays `wired | wireless`. `offline` is NOT a connection type.

### `NetworkClient.yaml`
Add required `status` field:
```yaml
status:
  $ref: "./NetworkClientStatus.yaml"
required: [..., status]
```
`ip` description updated to "current IP address for online clients, last known IP for offline clients."

### `NetworkClientStatus.yaml` (new file)
```yaml
type: string
description: |
  Whether the client is currently connected to the network.
  - `online` — currently connected
  - `offline` — previously seen, currently disconnected
enum:
  - online
  - offline
```

### `NetworkClientList.yaml`
Description updated to "Returns all known client devices, online and offline."

### `network-clients.yaml`
- Description updated: remove "Only active clients are returned".
- Add optional query parameter:
  ```yaml
  - name: status
    in: query
    required: false
    schema:
      $ref: "../components/schemas/NetworkClientStatus.yaml"
    description: Filter by connection status. Omit to return all clients.
  ```

### `WiredNetworkClientDetail.yaml`
All session fields become optional (were required):
- `switchName` — optional; may be available for offline clients (last known switch)
- `switchPort` — optional; online only
- `uptime` — optional; online only

### `WirelessNetworkClientDetail.yaml`
All session fields become optional (were required):
- `ssid` — optional; may not be available for offline clients
- `signalStrength` — optional; online only
- `uptime` — optional; online only

### `network-clients-id.yaml`
- Description updated: remove "Only currently connected clients can be retrieved; requesting an offline client returns 404."
- Offline clients return 200 with the wired or wireless detail variant (session fields absent).

## Adapter (`internal/adapters/unifi.go`)

Existing `UniFiSta` and `GetClients()` are untouched.

Add:

```go
type UniFiClientV2 struct {
    ID             string  `json:"id"`
    MAC            string  `json:"mac"`
    DisplayName    string  `json:"display_name"`
    Name           *string `json:"name"`
    Hostname       *string `json:"hostname"`
    IP             string  `json:"ip"`              // active only
    LastIP         string  `json:"last_ip"`         // history only
    IsWired        bool    `json:"is_wired"`
    Status         string  `json:"status"`          // "online" | "offline"
    LastUplinkName string  `json:"last_uplink_name"`
    Uptime         int     `json:"uptime"`          // active only
    ESSID          *string `json:"essid"`           // active only
    Signal         *int    `json:"signal"`          // active only
    LastSeen       int64   `json:"last_seen"`       // history only
}

func (c *UniFiClient) GetActiveClients() ([]UniFiClientV2, error)
func (c *UniFiClient) GetOfflineClients(historyDays int) ([]UniFiClientV2, error)
```

`GetActiveClients()`: one `login()`, GET v2 active endpoint, return.
`GetOfflineClients(historyDays int)`: one `login()`, GET v2 history endpoint with `withinHours=historyDays*24`, return.

## Config (`internal/config/config.go`)

Add optional field to `Backend`:
```go
ClientHistoryDays int `yaml:"client_history_days"` // UniFi only; defaults to 30
```

Default applied at the service wiring site (`cmd/server/`): if `ClientHistoryDays == 0`, use 30. The value is passed through to `GetOfflineClients()`. Example `config.yaml` entry:
```yaml
- name: unifi
  type: unifi
  host: unifi.home.bwilczynski.com
  username: agent
  password: ${UNIFI_PASS}
  client_history_days: 7  # optional, default 30
```

## Service (`internal/network/`)

### `ClientsBackend` interface
```go
type ClientsBackend interface {
    GetClients() ([]adapters.UniFiSta, error)                      // detail path (unchanged)
    GetActiveClients() ([]adapters.UniFiClientV2, error)           // list: online
    GetOfflineClients(historyDays int) ([]adapters.UniFiClientV2, error) // list: offline
}
```

### `ListClients`
Accepts optional `status` param. Calls only the endpoints needed:
- `status=online` → `GetActiveClients()` only
- `status=offline` → `GetOfflineClients(historyDays)` only
- no filter → both, results concatenated

Name: `name → hostname → mac` fallback (ignores `display_name`).
IP: `IP` for online clients, `LastIP` for offline clients.
`connectionType`: from `IsWired`.
`status`: from `Status` field (`"online"` → `online`, `"offline"` → `offline`).

### `GetClient`
- First checks `GetClients()` (v1 active) — if found, returns existing wired/wireless detail (unchanged path).
- If not found in active, calls `GetOfflineClients(historyDays)` and searches by composite ID suffix.
- If found in history, returns wired or wireless detail with only `switchName` populated (from `LastUplinkName`) and session fields absent.
- If not found in either, returns 404.

## Testing

### New fixtures
- `testdata/unifi-v2-active.json` — sanitized from `scripts/responses/unifi-v2-active-raw.json`
- `testdata/unifi-v2-history.json` — sanitized from `scripts/responses/unifi-v2-history-raw.json`

### Mock update
`mockUniFi` gains `GetActiveClients()` and `GetOfflineClients(historyDays int)` returning `[]adapters.UniFiClientV2` from separate fixture slices. Existing `GetClients()` mock unchanged — all existing detail tests compile and pass without modification.

### New list tests
- All clients returned by default (online + offline count matches fixture total).
- `status=online` filter returns only online entries.
- `status=offline` filter returns only offline entries.
- Online client: `status: online`, current `ip`, `connectionType: wired|wireless`.
- Offline client: `status: offline`, `last_ip` in `ip` field, `connectionType: wired|wireless`.
- Name fallback: alias → hostname → mac produces correct ID.

### New detail tests
- `GetClient` for an offline wired client returns wired detail with `switchName` set, `switchPort`/`uptime` absent.
- `GetClient` for an offline wireless client returns wireless detail with all session fields absent.
- `GetClient` for unknown ID returns 404.

### ID stability
Existing `TestListClients` is rewritten against v2 fixtures. IDs for clients with user-assigned aliases must match expected values, verifying `display_name` is not used for ID generation.

## Out of Scope
- SSID resolution for offline wireless clients (`wlanconf_id` → SSID lookup requires additional API call).
- Pagination (homelab client counts remain manageable).
- Detail for offline clients showing `switchPort`, `ssid`, `signalStrength`, `uptime` — these are unavailable from the history endpoint.
