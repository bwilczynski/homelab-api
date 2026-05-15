# Offline Clients Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `GET /network/clients` (list + detail) to return offline clients alongside online ones, with a `?status` filter and `status` field per client.

**Architecture:** UniFi v2 API supplies active clients (current session data) and history clients (last-known data) via separate endpoints. The adapter exposes three public methods sharing two private fetch helpers to avoid redundant logins. The service routes to the right adapter methods based on the requested status filter.

**Tech Stack:** Go, oapi-codegen (chi strict server), UniFi Controller v2 REST API, YAML (OpenAPI spec).

---

## File Map

| File | Change |
|---|---|
| `spec/openapi/components/schemas/NetworkClient.yaml` | Add `status` field + update `ip` description |
| `spec/openapi/components/schemas/NetworkClientStatus.yaml` | **New** — `online \| offline` enum |
| `spec/openapi/components/schemas/NetworkClientList.yaml` | Update description |
| `spec/openapi/components/schemas/WiredNetworkClientDetail.yaml` | Make `switchName`, `switchPort`, `uptime` optional |
| `spec/openapi/components/schemas/WirelessNetworkClientDetail.yaml` | Make `ssid`, `signalStrength`, `uptime` optional |
| `spec/openapi/paths/network-clients.yaml` | Add `?status` param, update description |
| `spec/openapi/paths/network-clients-id.yaml` | Remove "offline returns 404" from description |
| `internal/network/api.gen.go` | Regenerated — do not hand-edit |
| `internal/config/config.go` | Add `ClientHistoryDays int` to `Backend` |
| `config.sample.yaml` | Document `client_history_days` |
| `internal/adapters/unifi.go` | Add `UniFiClientV2` struct + fetch/public methods |
| `internal/network/service.go` | Add `historyDays` to `Service` + `NewService`, extend `UniFiBackend` |
| `internal/network/clients_service.go` | Update `ClientsBackend`, `ListClients`, `GetClient`, fix pointer fields |
| `internal/network/testdata/unifi-v2-active.json` | **New** — sanitized v2 active fixture |
| `internal/network/testdata/unifi-v2-history.json` | **New** — sanitized v2 history fixture |
| `internal/network/service_test.go` | Update mock, rewrite list tests, add offline tests |
| `internal/network/handler.go` | Pass `status` param to `ListClients` |
| `cmd/server/main.go` | Pass `historyDays` to `network.NewService` |
| `cmd/testserver/main.go` | Update `mockNetworkBackend` to implement new interface |

---

### Task 1: Spec changes in the `spec/` submodule

The `spec/` directory is a git submodule pointing to `github.com/bwilczynski/homelab-api-spec`. Make changes there and open a PR. Implementation continues against local changes without waiting for the PR to merge.

**Files:**
- Modify: `spec/openapi/components/schemas/NetworkClient.yaml`
- Create: `spec/openapi/components/schemas/NetworkClientStatus.yaml`
- Modify: `spec/openapi/components/schemas/NetworkClientList.yaml`
- Modify: `spec/openapi/components/schemas/WiredNetworkClientDetail.yaml`
- Modify: `spec/openapi/components/schemas/WirelessNetworkClientDetail.yaml`
- Modify: `spec/openapi/paths/network-clients.yaml`
- Modify: `spec/openapi/paths/network-clients-id.yaml`

- [ ] **Step 1: Create `NetworkClientStatus.yaml`**

```bash
cat > spec/openapi/components/schemas/NetworkClientStatus.yaml << 'EOF'
type: string
description: |
  Whether the client is currently connected to the network.
  - `online` — currently connected
  - `offline` — previously seen, currently disconnected
enum:
  - online
  - offline
EOF
```

- [ ] **Step 2: Update `NetworkClient.yaml`** — add `status` field, add to `required`, update `ip` description

Replace the file content:

```yaml
type: object
description: A client device on the network (online or offline).
properties:
  id:
    type: string
    description: |
      Globally unique client identifier in the form
      `{controller}.{hostname}-{macPrefix}`, where `hostname` is the
      client's name in kebab-case and `macPrefix` is the first two
      characters of the MAC address for disambiguation. The
      implementation ensures uniqueness across controllers.
    example: "unifi.macbook-pro-3c"
  name:
    type: string
    description: |
      Display name for the client: the user-assigned alias when set,
      otherwise the DHCP hostname.
    example: "MacBook Pro"
  mac:
    type: string
    description: Client MAC address in lowercase colon-separated notation.
    example: "3c:22:fb:09:aa:b1"
  ip:
    type: string
    description: Current IP address for online clients; last known IP address for offline clients.
    example: "192.168.1.100"
  connectionType:
    $ref: "./NetworkClientConnectionType.yaml"
  status:
    $ref: "./NetworkClientStatus.yaml"
required:
  - id
  - name
  - mac
  - connectionType
  - status
```

- [ ] **Step 3: Update `NetworkClientList.yaml`** — update description

```yaml
type: object
description: List of network clients.
properties:
  items:
    type: array
    description: Network clients matching the query (online and offline). Empty array, never null.
    items:
      $ref: "./NetworkClient.yaml"
required:
  - items
```

- [ ] **Step 4: Update `WiredNetworkClientDetail.yaml`** — remove `switchName`, `switchPort`, `uptime` from `required`

```yaml
allOf:
  - $ref: "./NetworkClient.yaml"
  - type: object
    description: Detail for a wired network client, including switch and port information.
    properties:
      connectionType:
        type: string
        enum: [wired]
      switchName:
        type: string
        description: Name of the switch this client is connected to. May be available for offline clients (last known switch).
        example: "Switch Living Room"
      switchPort:
        type: integer
        description: Physical port number on the switch.
        example: 8
      uptime:
        type: integer
        description: Seconds since the client's current session started.
        example: 604800
    required:
      - connectionType
```

- [ ] **Step 5: Update `WirelessNetworkClientDetail.yaml`** — remove `ssid`, `signalStrength`, `uptime` from `required`

```yaml
allOf:
  - $ref: "./NetworkClient.yaml"
  - type: object
    description: Detail for a wireless network client, including signal and SSID.
    properties:
      connectionType:
        type: string
        enum: [wireless]
      ssid:
        type: string
        description: SSID the client is associated with.
        example: "HomeNetwork"
      signalStrength:
        type: integer
        description: |
          Received signal strength in dBm.
          Typical range: -30 (excellent) to -90 (poor).
        example: -62
      uptime:
        type: integer
        description: Seconds since the client's current session started.
        example: 7200
    required:
      - connectionType
```

- [ ] **Step 6: Update `network-clients.yaml`** — add `status` query param, update description

Replace file content:

```yaml
get:
  operationId: listNetworkClients
  x-stability-level: draft
  summary: List network clients
  description: |
    Returns all known client devices from all configured controllers,
    including currently connected (online) and recently seen (offline) clients.

    Each client includes its MAC address, IP address, connection type
    (wired or wireless), and connection status (online or offline).

    Use the `status` query parameter to filter by connection status.

    A homelab typically has a manageable number of clients, so this
    endpoint returns all results without pagination.
  tags:
    - network
  security:
    - bearerAuth: [read:network]
  parameters:
    - name: status
      in: query
      required: false
      schema:
        $ref: "../components/schemas/NetworkClientStatus.yaml"
      description: Filter by connection status. Omit to return all clients (online and offline).
  responses:
    "200":
      description: List of network clients.
      content:
        application/json:
          schema:
            $ref: "../components/schemas/NetworkClientList.yaml"
          examples:
            typicalHomelab:
              summary: A mix of online and offline clients.
              value:
                items:
                  - id: "unifi.macbook-pro-3c"
                    name: "MacBook Pro"
                    mac: "3c:22:fb:09:aa:b1"
                    ip: "192.168.1.101"
                    connectionType: wireless
                    status: online
                  - id: "unifi.sonos-one-sl-c4"
                    name: "Sonos One SL"
                    mac: "c4:38:75:7a:8a:46"
                    ip: "192.168.1.55"
                    connectionType: wireless
                    status: offline
    "401":
      $ref: "../components/responses/Unauthorized.yaml"
    "403":
      $ref: "../components/responses/Forbidden.yaml"
    "429":
      $ref: "../components/responses/TooManyRequests.yaml"
    "500":
      $ref: "../components/responses/InternalServerError.yaml"
```

- [ ] **Step 7: Update `network-clients-id.yaml`** — remove the "offline returns 404" sentence from the description

In `spec/openapi/paths/network-clients-id.yaml`, replace the description block:

```yaml
  description: |
    Returns a single client by its composite identifier
    (`{controller}.{hostname}-{macPrefix}`, e.g. `unifi.macbook-pro-3c`).
    The response includes connection type and session uptime. The shape of the
    response varies by connection type: wired clients include switch
    name and port number; wireless clients include SSID and signal
    strength.

    For offline clients, session-specific fields (uptime, switchPort, ssid,
    signalStrength) are absent. switchName may be present as the last known switch.
```

- [ ] **Step 8: Commit spec changes in the submodule and open PR**

```bash
cd spec
git checkout -b feature/offline-clients
git add openapi/components/schemas/NetworkClientStatus.yaml \
        openapi/components/schemas/NetworkClient.yaml \
        openapi/components/schemas/NetworkClientList.yaml \
        openapi/components/schemas/WiredNetworkClientDetail.yaml \
        openapi/components/schemas/WirelessNetworkClientDetail.yaml \
        openapi/paths/network-clients.yaml \
        openapi/paths/network-clients-id.yaml
git commit -m "feat: add offline client support (status field, optional session fields)"
git push -u origin feature/offline-clients
gh pr create --title "feat: add offline client support" --body "$(cat <<'EOF'
## Summary
- Add `NetworkClientStatus` schema (`online` | `offline`)
- Add required `status` field to `NetworkClient`
- Add optional `?status` query param to `GET /network/clients`
- Make session-specific fields optional in wired/wireless detail schemas
- Update descriptions to reflect offline client support

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
cd ..
```

---

### Task 2: Regenerate server stubs

**Files:**
- Modify: `internal/network/api.gen.go` (generated)

- [ ] **Step 1: Bundle spec and regenerate**

```bash
make generate
```

Expected: `api.gen.go` updated. After this, the code will not compile because:
- `NetworkClient` now requires `Status NetworkClientStatus`
- `WiredNetworkClientDetail.SwitchName/SwitchPort/Uptime` are now `*string`/`*int`/`*int` (pointers, were value types)
- `WirelessNetworkClientDetail.Ssid/SignalStrength/Uptime` are now `*string`/`*int`/`*int`
- `ListNetworkClientsRequestObject` now has `.Params.Status *NetworkClientStatus`

This is expected — Tasks 3–8 restore compilation.

---

### Task 3: Add `client_history_days` to config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.sample.yaml`

- [ ] **Step 1: Add field to `Backend` struct**

In `internal/config/config.go`, add to the `Backend` struct after `InsecureTLS`:

```go
ClientHistoryDays int `yaml:"client_history_days"` // optional; UniFi only — how many days of offline client history to include (default: 30)
```

- [ ] **Step 2: Document in `config.sample.yaml`**

Find the UniFi backend example block and add the new optional field with a comment:

```yaml
  - name: unifi
    type: unifi
    host: unifi.home.example.com
    username: agent
    password: ${UNIFI_PASS}
    insecure_tls: true
    # client_history_days: 30  # optional; how many days of offline client history to include (default: 30)
```

- [ ] **Step 3: Build to verify config compiles**

```bash
make build
```

Expected: build fails only on network package (compilation errors from regenerated types). Config compiles cleanly.

---

### Task 4: Add `UniFiClientV2` struct and adapter methods

**Files:**
- Modify: `internal/adapters/unifi.go`

- [ ] **Step 1: Add `UniFiClientV2` struct**

In `internal/adapters/unifi.go`, after the existing `--- Client types ---` section, add a new section:

```go
// --- Client v2 types ---

// UniFiClientV2 represents a client from the UniFi Controller v2 API.
// Active clients have IP, Uptime, ESSID, Signal populated; history clients have LastIP, LastSeen.
type UniFiClientV2 struct {
	ID             string  `json:"id"`
	MAC            string  `json:"mac"`
	DisplayName    string  `json:"display_name"`
	Name           *string `json:"name"`
	Hostname       *string `json:"hostname"`
	IP             string  `json:"ip"`
	LastIP         string  `json:"last_ip"`
	IsWired        bool    `json:"is_wired"`
	Status         string  `json:"status"` // "online" | "offline"
	LastUplinkName string  `json:"last_uplink_name"`
	Uptime         int     `json:"uptime"`
	ESSID          *string `json:"essid"`
	Signal         *int    `json:"signal"`
	LastSeen       int64   `json:"last_seen"`
}
```

- [ ] **Step 2: Add private fetch methods**

Add after the struct:

```go
// fetchActiveClients calls the v2 active clients endpoint. Caller must have already called login().
func (c *UniFiClient) fetchActiveClients() ([]UniFiClientV2, error) {
	var result []UniFiClientV2
	if err := c.get("/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// fetchOfflineClients calls the v2 history clients endpoint. Caller must have already called login().
func (c *UniFiClient) fetchOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	path := fmt.Sprintf("/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=%d", historyDays*24)
	var result []UniFiClientV2
	if err := c.get(path, &result); err != nil {
		return nil, err
	}
	return result, nil
}
```

- [ ] **Step 3: Add public methods**

```go
// GetActiveClients retrieves currently connected clients from the UniFi Controller v2 API.
func (c *UniFiClient) GetActiveClients() ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	return c.fetchActiveClients()
}

// GetOfflineClients retrieves recently disconnected clients from the UniFi Controller v2 API.
// historyDays controls how far back to look (passed as withinHours=historyDays*24).
func (c *UniFiClient) GetOfflineClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	return c.fetchOfflineClients(historyDays)
}

// GetAllClients retrieves all clients (active and history) with a single login.
func (c *UniFiClient) GetAllClients(historyDays int) ([]UniFiClientV2, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
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

- [ ] **Step 4: Verify adapter compiles**

```bash
go build ./internal/adapters/...
```

Expected: PASS.

---

### Task 5: Create v2 test fixtures

**Files:**
- Create: `internal/network/testdata/unifi-v2-active.json`
- Create: `internal/network/testdata/unifi-v2-history.json`

Note: The v2 API returns a bare JSON array (no `{meta, data}` envelope). Fixtures are wrapped in `{"data": [...]}` so the existing `loadFixture[T]` helper can be reused.

- [ ] **Step 1: Create `unifi-v2-active.json`**

```json
{
  "data": [
    {
      "id": "aabbcc000001",
      "mac": "3c:22:fb:aa:bb:01",
      "display_name": "MacBook Pro",
      "name": "MacBook Pro",
      "hostname": "macbook",
      "ip": "192.168.10.67",
      "last_ip": "",
      "is_wired": false,
      "status": "online",
      "last_uplink_name": "UAP-01",
      "uptime": 27075,
      "essid": "homelab",
      "signal": -69,
      "last_seen": 0
    },
    {
      "id": "aabbcc000002",
      "mac": "68:d7:9a:aa:bb:02",
      "display_name": "nas-1",
      "name": null,
      "hostname": "nas-1",
      "ip": "192.168.100.5",
      "last_ip": "",
      "is_wired": true,
      "status": "online",
      "last_uplink_name": "Switch Living Room",
      "uptime": 1024199,
      "essid": null,
      "signal": null,
      "last_seen": 0
    },
    {
      "id": "aabbcc000003",
      "mac": "c4:38:75:aa:bb:03",
      "display_name": "Sonos One SL (L)",
      "name": "Sonos One SL (L)",
      "hostname": "SonosZP",
      "ip": "192.168.10.55",
      "last_ip": "",
      "is_wired": false,
      "status": "online",
      "last_uplink_name": "UAP-01",
      "uptime": 86400,
      "essid": "homelab-iot",
      "signal": -58,
      "last_seen": 0
    }
  ]
}
```

- [ ] **Step 2: Create `unifi-v2-history.json`**

```json
{
  "data": [
    {
      "id": "aabbcc000004",
      "mac": "e0:f7:28:aa:bb:04",
      "display_name": "Kindle Paperwhite",
      "name": "Kindle Paperwhite",
      "hostname": null,
      "ip": "",
      "last_ip": "192.168.10.37",
      "is_wired": false,
      "status": "offline",
      "last_uplink_name": "UAP-01",
      "uptime": 0,
      "essid": null,
      "signal": null,
      "last_seen": 1778837212
    },
    {
      "id": "aabbcc000005",
      "mac": "aa:bb:cc:aa:bb:05",
      "display_name": "host-02 aa",
      "name": null,
      "hostname": "host-02",
      "ip": "",
      "last_ip": "192.168.10.42",
      "is_wired": true,
      "status": "offline",
      "last_uplink_name": "Switch Flex Mini",
      "uptime": 0,
      "essid": null,
      "signal": null,
      "last_seen": 1778829796
    }
  ]
}
```

---

### Task 6: Update service interfaces, mock, and `NewService`

After this task all existing tests compile and pass. No new behaviour yet.

**Files:**
- Modify: `internal/network/service.go`
- Modify: `internal/network/clients_service.go`
- Modify: `internal/network/service_test.go`
- Modify: `cmd/testserver/main.go`

- [ ] **Step 1: Extend `ClientsBackend` in `clients_service.go`**

Replace the existing interface:

```go
// ClientsBackend is the narrow interface for client operations.
type ClientsBackend interface {
	GetClients() ([]adapters.UniFiSta, error)
	GetActiveClients() ([]adapters.UniFiClientV2, error)
	GetOfflineClients(historyDays int) ([]adapters.UniFiClientV2, error)
	GetAllClients(historyDays int) ([]adapters.UniFiClientV2, error)
}
```

- [ ] **Step 2: Add `historyDays` to `Service` struct and `NewService` in `service.go`**

Update `Service`:
```go
type Service struct {
	backends    []controllerBackend
	monitor     adapters.AvailabilityChecker
	historyDays int
}
```

Update `NewService` signature and body:
```go
func NewService(backends map[string]UniFiBackend, historyDays int, monitor ...adapters.AvailabilityChecker) *Service {
	cbs := make([]controllerBackend, 0, len(backends))
	for controller, unifi := range backends {
		cbs = append(cbs, controllerBackend{controller: controller, unifi: unifi})
	}
	sort.Slice(cbs, func(i, j int) bool { return cbs[i].controller < cbs[j].controller })
	svc := &Service{backends: cbs, historyDays: historyDays}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}
```

- [ ] **Step 3: Update mock in `service_test.go`**

Replace `mockUniFi`:

```go
type mockUniFi struct {
	devices        []adapters.UniFiDevice
	clients        []adapters.UniFiSta
	activeClients  []adapters.UniFiClientV2
	offlineClients []adapters.UniFiClientV2
	err            error
}

func (m *mockUniFi) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, m.err
}

func (m *mockUniFi) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, m.err
}

func (m *mockUniFi) GetActiveClients() ([]adapters.UniFiClientV2, error) {
	return m.activeClients, m.err
}

func (m *mockUniFi) GetOfflineClients(_ int) ([]adapters.UniFiClientV2, error) {
	return m.offlineClients, m.err
}

func (m *mockUniFi) GetAllClients(_ int) ([]adapters.UniFiClientV2, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append(m.activeClients, m.offlineClients...), nil
}
```

- [ ] **Step 4: Fix existing `NewService` calls in `service_test.go`**

Add `historyDays` argument (use `30`) to every existing `NewService(...)` call in the test file:

```go
// Before:
svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}})
// After:
svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)
```

Apply this change to all occurrences (there are ~8).

- [ ] **Step 5: Update `mockNetworkBackend` in `cmd/testserver/main.go`**

Add the new fields and methods to the existing struct:

```go
type mockNetworkBackend struct {
	devices        []adapters.UniFiDevice
	clients        []adapters.UniFiSta
	activeClients  []adapters.UniFiClientV2
	offlineClients []adapters.UniFiClientV2
}

func (m *mockNetworkBackend) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, nil
}

func (m *mockNetworkBackend) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, nil
}

func (m *mockNetworkBackend) GetActiveClients() ([]adapters.UniFiClientV2, error) {
	return m.activeClients, nil
}

func (m *mockNetworkBackend) GetOfflineClients(_ int) ([]adapters.UniFiClientV2, error) {
	return m.offlineClients, nil
}

func (m *mockNetworkBackend) GetAllClients(_ int) ([]adapters.UniFiClientV2, error) {
	return append(m.activeClients, m.offlineClients...), nil
}
```

Also update the `network.NewService` call in testserver `main()`:

```go
networkSvc := network.NewService(map[string]network.UniFiBackend{"unifi": nb}, 30)
```

- [ ] **Step 6: Fix `clientToDetail` and `clientToList` for new pointer types**

After `make generate`, `WiredNetworkClientDetail` and `WirelessNetworkClientDetail` have pointer types for optional fields and a new `Status` field (inherited from `NetworkClient`). Update `clients_service.go`:

In `clientToList`, add the `Status` field:
```go
func clientToList(controller string, sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	client := NetworkClient{
		Id:             fmt.Sprintf("%s.%s", controller, clientSuffix(sta)),
		Name:           clientName(sta),
		Mac:            mac,
		ConnectionType: mapConnectionType(sta.IsWired),
		Status:         NetworkClientStatusOnline,
	}
	if sta.IP != "" {
		ip := sta.IP
		client.Ip = &ip
	}
	return client
}
```

In `clientToDetail`, update pointer fields for wired:
```go
// wired branch — replace the WiredNetworkClientDetail literal
switchName := sta.LastUplinkName
switchPort := sta.SwPort
uptime := sta.Uptime
err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
	ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
	Id:             id,
	Name:           name,
	Mac:            mac,
	Ip:             ip,
	Status:         NetworkClientStatusOnline,
	SwitchName:     &switchName,
	SwitchPort:     &switchPort,
	Uptime:         &uptime,
})
```

For wireless:
```go
// wireless branch — replace the WirelessNetworkClientDetail literal
ssid := ""
if sta.ESSID != nil {
	ssid = *sta.ESSID
}
signal := 0
if sta.Signal != nil {
	signal = *sta.Signal
}
uptime := sta.Uptime
err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
	ConnectionType: Wireless,
	Id:             id,
	Name:           name,
	Mac:            mac,
	Ip:             ip,
	Status:         NetworkClientStatusOnline,
	Ssid:           &ssid,
	SignalStrength:  &signal,
	Uptime:         &uptime,
})
```

- [ ] **Step 7: Fix existing detail tests for pointer field access**

In `service_test.go`, update the assertions that read wired/wireless detail fields:

```go
// TestGetClientWireless — dereference pointer fields:
if wireless.Ssid == nil || *wireless.Ssid != "homelab" {
	t.Errorf("expected ssid homelab, got %v", wireless.Ssid)
}
if wireless.SignalStrength == nil || *wireless.SignalStrength != -69 {
	t.Errorf("expected signal -69, got %v", wireless.SignalStrength)
}
if wireless.Uptime == nil || *wireless.Uptime != 27075 {
	t.Errorf("expected uptime 27075, got %v", wireless.Uptime)
}

// TestGetClientWired — dereference pointer fields:
if wired.SwitchName == nil || *wired.SwitchName != "Switch Living Room" {
	t.Errorf("expected switchName Switch Living Room, got %v", wired.SwitchName)
}
if wired.SwitchPort == nil || *wired.SwitchPort != 3 {
	t.Errorf("expected switchPort 3, got %v", wired.SwitchPort)
}
if wired.Uptime == nil || *wired.Uptime != 1024199 {
	t.Errorf("expected uptime 1024199, got %v", wired.Uptime)
}
```

- [ ] **Step 8: Run all existing tests — must pass**

```bash
make test
```

Expected: all tests pass. `ListClients` still calls `GetClients()` at this point — that will change in Task 7.

---

### Task 7: Rewrite `ListClients` with status filter (TDD)

**Files:**
- Modify: `internal/network/service_test.go`
- Modify: `internal/network/clients_service.go`

- [ ] **Step 1: Write failing list tests**

Replace `TestListClients` and `TestListClientsEmpty` in `service_test.go` with:

```go
func TestListClientsAll(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 clients (3 active + 2 offline), got %d", len(result.Items))
	}
}

func TestListClientsOnlineFilter(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "online")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 online clients, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != NetworkClientStatusOnline {
			t.Errorf("expected status online, got %s", item.Status)
		}
	}
}

func TestListClientsOfflineFilter(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "offline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 offline clients, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != NetworkClientStatusOffline {
			t.Errorf("expected status offline, got %s", item.Status)
		}
	}
}

func TestListClientsIDAndFields(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byID := make(map[string]NetworkClient, len(result.Items))
	for _, item := range result.Items {
		byID[item.Id] = item
	}

	// Online wireless with user alias — ID uses alias, not display_name
	mb, ok := byID["unifi.macbook-pro-3c"]
	if !ok {
		t.Fatal("expected unifi.macbook-pro-3c")
	}
	if mb.Name != "MacBook Pro" {
		t.Errorf("expected name MacBook Pro, got %s", mb.Name)
	}
	if mb.ConnectionType != NetworkClientConnectionTypeWireless {
		t.Errorf("expected wireless, got %s", mb.ConnectionType)
	}
	if mb.Status != NetworkClientStatusOnline {
		t.Errorf("expected online, got %s", mb.Status)
	}
	if mb.Ip == nil || *mb.Ip != "192.168.10.67" {
		t.Errorf("expected ip 192.168.10.67, got %v", mb.Ip)
	}

	// Online wired with hostname only (no user alias)
	nas, ok := byID["unifi.nas-1-68"]
	if !ok {
		t.Fatal("expected unifi.nas-1-68")
	}
	if nas.ConnectionType != NetworkClientConnectionTypeWired {
		t.Errorf("expected wired, got %s", nas.ConnectionType)
	}

	// Offline wireless — ip comes from last_ip
	kindle, ok := byID["unifi.kindle-paperwhite-e0"]
	if !ok {
		t.Fatal("expected unifi.kindle-paperwhite-e0")
	}
	if kindle.Status != NetworkClientStatusOffline {
		t.Errorf("expected offline, got %s", kindle.Status)
	}
	if kindle.Ip == nil || *kindle.Ip != "192.168.10.37" {
		t.Errorf("expected last_ip 192.168.10.37, got %v", kindle.Ip)
	}

	// Offline wired with hostname only
	host, ok := byID["unifi.host-02-aa"]
	if !ok {
		t.Fatal("expected unifi.host-02-aa")
	}
	if host.ConnectionType != NetworkClientConnectionTypeWired {
		t.Errorf("expected wired, got %s", host.ConnectionType)
	}
}

func TestListClientsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		activeClients:  []adapters.UniFiClientV2{},
		offlineClients: []adapters.UniFiClientV2{},
	}}, 30)
	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(result.Items))
	}
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/network/ -run "TestListClients" -v
```

Expected: FAIL — `ListClients` has wrong signature and still calls `GetClients()`.

- [ ] **Step 3: Implement `clientNameV2` and `clientToListV2` helpers in `clients_service.go`**

```go
func clientNameV2(c adapters.UniFiClientV2) string {
	if c.Name != nil && *c.Name != "" {
		return *c.Name
	}
	if c.Hostname != nil && *c.Hostname != "" {
		return *c.Hostname
	}
	return c.MAC
}

func clientToListV2(controller string, c adapters.UniFiClientV2) NetworkClient {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)

	var ip string
	if c.Status == "online" {
		ip = c.IP
	} else {
		ip = c.LastIP
	}

	client := NetworkClient{
		Id:             id,
		Name:           name,
		Mac:            mac,
		ConnectionType: mapConnectionType(c.IsWired),
		Status:         NetworkClientStatus(c.Status),
	}
	if ip != "" {
		client.Ip = &ip
	}
	return client
}
```

- [ ] **Step 4: Replace `ListClients` implementation**

```go
// ListClients retrieves clients from all backends. status filters by "online", "offline", or "" for all.
func (s *Service) ListClients(ctx context.Context, status string) (NetworkClientList, error) {
	var items []NetworkClient
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		var raw []adapters.UniFiClientV2
		var err error
		switch status {
		case "online":
			raw, err = cb.unifi.GetActiveClients()
		case "offline":
			raw, err = cb.unifi.GetOfflineClients(s.historyDays)
		default:
			raw, err = cb.unifi.GetAllClients(s.historyDays)
		}
		if err != nil {
			return NetworkClientList{}, fmt.Errorf("get unifi clients from %s: %w", cb.controller, err)
		}
		for _, c := range raw {
			items = append(items, clientToListV2(cb.controller, c))
		}
	}
	if items == nil {
		items = []NetworkClient{}
	}
	return NetworkClientList{Items: items}, nil
}
```

- [ ] **Step 5: Run list tests — must pass**

```bash
go test ./internal/network/ -run "TestListClients" -v
```

Expected: all `TestListClients*` PASS.

- [ ] **Step 6: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/network/clients_service.go \
        internal/network/service.go \
        internal/network/service_test.go \
        internal/network/testdata/unifi-v2-active.json \
        internal/network/testdata/unifi-v2-history.json
git commit -m "feat: implement ListClients with status filter using UniFi v2 API"
```

---

### Task 8: Implement `GetClient` offline path (TDD)

**Files:**
- Modify: `internal/network/service_test.go`
- Modify: `internal/network/clients_service.go`

- [ ] **Step 1: Write failing offline detail tests**

Add to `service_test.go`:

```go
func TestGetClientOfflineWired(t *testing.T) {
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		clients:        []adapters.UniFiSta{},
		offlineClients: offline,
	}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.host-02-aa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected offline wired client to be found")
	}

	wired, err := detail.AsWiredNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wired detail: %v", err)
	}
	if wired.Id != "unifi.host-02-aa" {
		t.Errorf("expected id unifi.host-02-aa, got %s", wired.Id)
	}
	if wired.Status != NetworkClientStatusOffline {
		t.Errorf("expected status offline, got %s", wired.Status)
	}
	// switchName populated from last_uplink_name
	if wired.SwitchName == nil || *wired.SwitchName != "Switch Flex Mini" {
		t.Errorf("expected switchName Switch Flex Mini, got %v", wired.SwitchName)
	}
	// session fields absent for offline clients
	if wired.SwitchPort != nil {
		t.Errorf("expected nil switchPort for offline client, got %v", wired.SwitchPort)
	}
	if wired.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wired.Uptime)
	}
}

func TestGetClientOfflineWireless(t *testing.T) {
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		clients:        []adapters.UniFiSta{},
		offlineClients: offline,
	}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.kindle-paperwhite-e0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected offline wireless client to be found")
	}

	wireless, err := detail.AsWirelessNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wireless detail: %v", err)
	}
	if wireless.Status != NetworkClientStatusOffline {
		t.Errorf("expected status offline, got %s", wireless.Status)
	}
	if wireless.Ip == nil || *wireless.Ip != "192.168.10.37" {
		t.Errorf("expected last_ip 192.168.10.37, got %v", wireless.Ip)
	}
	// session fields absent
	if wireless.Ssid != nil {
		t.Errorf("expected nil ssid for offline client, got %v", wireless.Ssid)
	}
	if wireless.SignalStrength != nil {
		t.Errorf("expected nil signalStrength for offline client, got %v", wireless.SignalStrength)
	}
	if wireless.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wireless.Uptime)
	}
}

func TestGetClientNotFoundInEither(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		clients:        []adapters.UniFiSta{},
		offlineClients: []adapters.UniFiClientV2{},
	}}, 30)

	_, found, err := svc.GetClient(context.Background(), "unifi.nobody-00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}
```

- [ ] **Step 2: Run offline detail tests — must fail**

```bash
go test ./internal/network/ -run "TestGetClientOffline|TestGetClientNotFoundInEither" -v
```

Expected: FAIL — `GetClient` has no offline path.

- [ ] **Step 3: Add `clientToDetailV2` helper in `clients_service.go`**

```go
func clientToDetailV2(controller string, c adapters.UniFiClientV2) (NetworkClientDetail, error) {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)

	var ip *string
	if c.LastIP != "" {
		v := c.LastIP
		ip = &v
	}

	var detail NetworkClientDetail
	if c.IsWired {
		var switchName *string
		if c.LastUplinkName != "" {
			s := c.LastUplinkName
			switchName = &s
		}
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         NetworkClientStatusOffline,
			SwitchName:     switchName,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wired client detail: %w", err)
		}
	} else {
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         NetworkClientStatusOffline,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wireless client detail: %w", err)
		}
	}
	return detail, nil
}
```

- [ ] **Step 4: Add offline path to `GetClient`**

In `clients_service.go`, update `GetClient` — after the existing active-client loop (which returns early on found), add:

```go
// Not found in active clients — check offline history.
backend, err := s.findBackend(controller)
if err != nil {
	return NetworkClientDetail{}, false, nil
}

offline, err := backend.GetOfflineClients(s.historyDays)
if err != nil {
	return NetworkClientDetail{}, false, fmt.Errorf("get unifi offline clients: %w", err)
}

for _, c := range offline {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	if fmt.Sprintf("%s-%s", toKebab(name), prefix) == suffix {
		detail, err := clientToDetailV2(controller, c)
		if err != nil {
			return NetworkClientDetail{}, false, err
		}
		return detail, true, nil
	}
}
return NetworkClientDetail{}, false, nil
```

- [ ] **Step 5: Run offline detail tests — must pass**

```bash
go test ./internal/network/ -run "TestGetClientOffline|TestGetClientNotFoundInEither" -v
```

Expected: all PASS.

- [ ] **Step 6: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/network/clients_service.go internal/network/service_test.go
git commit -m "feat: implement GetClient offline path returning wired/wireless detail"
```

---

### Task 9: Update handler and wire `historyDays` in main

**Files:**
- Modify: `internal/network/handler.go`
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/backends.go`
- Modify: `cmd/testserver/main.go`

- [ ] **Step 1: Update `ListNetworkClients` handler to pass status param**

In `internal/network/handler.go`, replace the `ListNetworkClients` method:

```go
func (h *ServerHandler) ListNetworkClients(ctx context.Context, request ListNetworkClientsRequestObject) (ListNetworkClientsResponseObject, error) {
	var status string
	if request.Params.Status != nil {
		status = string(*request.Params.Status)
	}
	result, err := h.svc.ListClients(ctx, status)
	if err != nil {
		detail := err.Error()
		return ListNetworkClients500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListNetworkClients200JSONResponse(result), nil
}
```

- [ ] **Step 2: Wire `historyDays` in `cmd/server/main.go`**

Find the network service wiring section and replace:

```go
// Before:
networkSvc := network.NewService(networkBackends, monitor)

// After:
historyDays := 30
for _, b := range cfg.ByType(config.BackendTypeUniFi) {
	if b.ClientHistoryDays > 0 {
		historyDays = b.ClientHistoryDays
		break
	}
}
networkSvc := network.NewService(networkBackends, historyDays, monitor)
```

- [ ] **Step 3: Build**

```bash
make build
```

Expected: PASS.

- [ ] **Step 4: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/network/handler.go cmd/server/main.go cmd/testserver/main.go \
        internal/config/config.go config.sample.yaml \
        internal/adapters/unifi.go
git commit -m "feat: wire offline clients support end to end"
```

---

### Task 10: Update submodule pointer and final verification

Once the spec PR is merged into `homelab-api-spec`:

**Files:**
- Modify: `spec/` (submodule pointer)
- Modify: `internal/network/api.gen.go` (regenerated)

- [ ] **Step 1: Update submodule to merged commit**

```bash
cd spec && git checkout main && git pull && cd ..
git add spec
git commit -m "chore: update spec submodule to merged offline clients changes"
```

- [ ] **Step 2: Regenerate to confirm stubs match merged spec**

```bash
make generate
```

- [ ] **Step 3: Final test run**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 4: Final build**

```bash
make build
```

Expected: PASS.
