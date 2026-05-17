# Network Relations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the updated OpenAPI contract: polymorphic NetworkDeviceDetail (switch ports, AP clients, traffic, uplink topology), connectedTo cross-references on client detail, uri on list items, and Docker restart-policy enum rename.

**Architecture:** Dual-fetch on detail endpoints (GetDevice + GetClient each call GetDevices() + GetClients()). Build in-memory MAC indexes per request to resolve cross-references between devices and clients. No new UniFi API endpoints — all data is in the existing /stat/device and /stat/sta responses. NetworkDeviceDetail becomes anyOf+discriminator (same MarshalJSON wrapper pattern as NetworkClientDetail).

**Tech Stack:** Go 1.23, chi router, oapi-codegen strict server, UniFi Controller API.

---

## File map

| File | Change |
|---|---|
| `internal/adapters/unifi.go` | Add UniFiPortEntry, UniFiUplink, UniFiWanIface structs; extend UniFiDevice and UniFiSta |
| `internal/network/testdata/unifi-devices.json` | Add port_table, uplink, tx_bytes, rx_bytes, wan1 |
| `internal/network/testdata/unifi-clients.json` | Add ap_mac, wired_rate_mbps |
| `internal/network/testdata/unifi-v2-history.json` | Add last_uplink_mac, fix essid for offline wireless |
| `internal/network/testdata/unifi-v2-active.json` | Add last_uplink_mac |
| `internal/network/devices_service.go` | Dual-fetch, indexes, polymorphic device variants, uri |
| `internal/network/clients_service.go` | Device lookup for connectedTo, uri on list items |
| `internal/network/handler.go` | Add networkDeviceDetailResponse wrapper |
| `internal/network/service_test.go` | Update all broken tests, add new tests |
| `internal/docker/containers_service.go` | Fix mapRestartPolicy for camelCase enum values |
| `internal/docker/containers_service_test.go` | Add tests for renamed enum values |

---

## Task 1: Regenerate server stubs

**Files:**
- Run: `make generate` (updates `internal/network/api.gen.go`, `internal/docker/api.gen.go`)

- [ ] **Step 1: Update the spec submodule and regenerate**

```bash
cd /path/to/homelab-api
make generate
```

Expected: all `api.gen.go` files regenerated without error.

- [ ] **Step 2: Verify new network types exist**

```bash
grep -n "SwitchDetail\|AccessPointDetail\|GatewayDetail\|UnknownDeviceDetail\|NetworkTraffic\|NetworkConnection\|WirelessConnection\|NetworkDeviceRef\|NetworkClientRef\|NetworkConnectionRef\|SwitchPort" internal/network/api.gen.go | head -40
```

Expected: all new types present.

- [ ] **Step 3: Note the exact field names from NetworkTraffic, SwitchPort, NetworkConnection, WirelessConnection**

```bash
grep -A 20 "type NetworkTraffic struct" internal/network/api.gen.go
grep -A 20 "type SwitchPort struct" internal/network/api.gen.go
grep -A 15 "type NetworkConnection struct" internal/network/api.gen.go
grep -A 15 "type WirelessConnection struct" internal/network/api.gen.go
grep -A 10 "type NetworkDeviceRef struct" internal/network/api.gen.go
grep -A 10 "type NetworkClientRef struct" internal/network/api.gen.go
```

Keep these names handy — you'll use them in every subsequent task.

- [ ] **Step 4: Verify Docker enum values renamed**

```bash
grep -n "UnlessStopped\|OnFailure\|unlessStopped\|onFailure\|unless-stopped\|on-failure" internal/docker/api.gen.go
```

Expected: new constants `UnlessStopped ContainerDetailRestartPolicy = "unlessStopped"` and `OnFailure ContainerDetailRestartPolicy = "onFailure"`.

- [ ] **Step 5: Verify build still compiles (it will have errors — note them)**

```bash
make build 2>&1 | head -40
```

Expected: compile errors due to removed fields (`SwitchName`, `SwitchPort`, `Ssid`, `SignalStrength` on client detail; `NumClients` on NetworkDevice; flat fields on NetworkDeviceDetail). These are fixed in later tasks. Do not fix them yet.

- [ ] **Step 6: Commit generated files**

```bash
git add internal/network/api.gen.go internal/docker/api.gen.go
git commit -m "chore: regenerate stubs for extended network models and docker enum rename"
```

---

## Task 2: Fix Docker restart-policy enum

**Files:**
- Modify: `internal/docker/containers_service.go` — `mapRestartPolicy`
- Modify: `internal/docker/containers_service_test.go` — add mapping tests

The generated `ContainerDetailRestartPolicy` constants have new values: `UnlessStopped = "unlessStopped"` and `OnFailure = "onFailure"`. The Docker API returns the hyphenated strings `"unless-stopped"` and `"on-failure"`. The `mapRestartPolicy` function must translate these.

- [ ] **Step 1: Write failing tests**

In `internal/docker/containers_service_test.go`, add after the existing tests:

```go
func TestMapRestartPolicy(t *testing.T) {
	tests := []struct {
		input string
		want  ContainerDetailRestartPolicy
	}{
		{"always", Always},
		{"no", No},
		{"unless-stopped", UnlessStopped},
		{"on-failure", OnFailure},
		{"unknown", No},
		{"", No},
	}
	for _, tt := range tests {
		got := mapRestartPolicy(tt.input)
		if got != tt.want {
			t.Errorf("mapRestartPolicy(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
go test ./internal/docker/ -run TestMapRestartPolicy -v
```

Expected: FAIL — `mapRestartPolicy("unless-stopped")` returns `"no"`, not `"unlessStopped"`.

- [ ] **Step 3: Fix mapRestartPolicy**

Replace the existing `mapRestartPolicy` function in `internal/docker/containers_service.go`:

```go
func mapRestartPolicy(name string) ContainerDetailRestartPolicy {
	switch name {
	case "always":
		return Always
	case "no":
		return No
	case "unless-stopped":
		return UnlessStopped
	case "on-failure":
		return OnFailure
	default:
		return No
	}
}
```

- [ ] **Step 4: Run to confirm it passes**

```bash
go test ./internal/docker/ -run TestMapRestartPolicy -v
```

Expected: PASS.

- [ ] **Step 5: Run all docker tests**

```bash
go test ./internal/docker/ -v
```

Expected: all pass. The existing `TestGetContainerDetail` tests `RestartPolicy != Always` — verify it still passes.

- [ ] **Step 6: Commit**

```bash
git add internal/docker/containers_service.go internal/docker/containers_service_test.go
git commit -m "fix: map Docker restart-policy strings to camelCase enum values (unlessStopped, onFailure)"
```

---

## Task 3: Extend UniFi adapter structs

**Files:**
- Modify: `internal/adapters/unifi.go`

Add three new structs and extend `UniFiDevice` and `UniFiSta`. No logic changes — pure struct additions.

- [ ] **Step 1: Add new structs and extend UniFiDevice**

In `internal/adapters/unifi.go`, add after the existing `UniFiDevice` struct definition (around line 118):

```go
// UniFiPortEntry represents one physical port from port_table in the UniFi device response.
type UniFiPortEntry struct {
	PortIdx  int     `json:"port_idx"`
	Up       bool    `json:"up"`
	Speed    int     `json:"speed"`      // negotiated speed in Mbps (0 when down)
	PortPoe  bool    `json:"port_poe"`
	PoeMode  string  `json:"poe_mode"`   // "auto", "passive24v", "passthrough", or "" when off
	PoePower string  `json:"poe_power"`  // watts as string e.g. "3.00"; empty when no load
	TxBytes  int64   `json:"tx_bytes"`
	RxBytes  int64   `json:"rx_bytes"`
	TxBytesR float64 `json:"tx_bytes-r"`
	RxBytesR float64 `json:"rx_bytes-r"`
}

// UniFiUplink represents the uplink block in the UniFi device response.
type UniFiUplink struct {
	UplinkMAC        string  `json:"uplink_mac"`          // MAC of upstream device; empty for root gateway
	UplinkDeviceName string  `json:"uplink_device_name"`
	UplinkRemotePort *int    `json:"uplink_remote_port"`  // port on upstream device; nil when unknown
	Speed            int     `json:"speed"`
	TxBytesR         float64 `json:"tx_bytes-r"`
	RxBytesR         float64 `json:"rx_bytes-r"`
	TxBytes          int64   `json:"tx_bytes"`
	RxBytes          int64   `json:"rx_bytes"`
}

// UniFiWanIface represents the wan1 block present on gateway devices.
type UniFiWanIface struct {
	TxBytes  int64   `json:"tx_bytes"`
	RxBytes  int64   `json:"rx_bytes"`
	TxBytesR float64 `json:"tx_bytes-r"`
	RxBytesR float64 `json:"rx_bytes-r"`
}
```

- [ ] **Step 2: Extend UniFiDevice**

Replace the existing `UniFiDevice` struct:

```go
// UniFiDevice represents a managed network device from the UniFi Controller.
type UniFiDevice struct {
	ID          string          `json:"_id"`
	MAC         string          `json:"mac"`
	Name        string          `json:"name"`
	Model       string          `json:"model"`
	Type        string          `json:"type"` // uap, usw, ugw, udm, udm-pro
	State       int             `json:"state"`
	IP          string          `json:"ip"`
	Version     string          `json:"version"`
	Uptime      int             `json:"uptime"`
	UserNumSta  int             `json:"user-num_sta"`
	GuestNumSta int             `json:"guest-num_sta"`
	TxBytes     int64           `json:"tx_bytes"`
	RxBytes     int64           `json:"rx_bytes"`
	PortTable   []UniFiPortEntry `json:"port_table"`
	Uplink      *UniFiUplink    `json:"uplink"`
	Wan1        *UniFiWanIface  `json:"wan1"`
}
```

- [ ] **Step 3: Extend UniFiSta**

Replace the existing `UniFiSta` struct:

```go
// UniFiSta represents an active client device from the UniFi Controller.
type UniFiSta struct {
	MAC            string  `json:"mac"`
	Hostname       *string `json:"hostname"`
	Name           *string `json:"name"`
	IP             string  `json:"ip"`
	IsWired        bool    `json:"is_wired"`
	ESSID          *string `json:"essid"`
	Signal         *int    `json:"signal"`
	Uptime         int     `json:"uptime"`
	SwMAC          string  `json:"sw_mac"`
	SwPort         int     `json:"sw_port"`
	LastUplinkName string  `json:"last_uplink_name"`
	ApMAC          string  `json:"ap_mac"`          // MAC of connected AP (wireless clients)
	WiredRateMbps  int     `json:"wired_rate_mbps"` // negotiated link speed in Mbps (wired clients)
}
```

- [ ] **Step 4: Extend UniFiClientV2**

Replace the existing `UniFiClientV2` struct to add `LastUplinkMAC`:

```go
// UniFiClientV2 represents a client from the UniFi Controller v2 API.
type UniFiClientV2 struct {
	ID             string  `json:"id"`
	MAC            string  `json:"mac"`
	DisplayName    string  `json:"display_name"`
	Name           *string `json:"name"`
	Hostname       *string `json:"hostname"`
	IP             string  `json:"ip"`
	LastIP         string  `json:"last_ip"`
	IsWired        bool    `json:"is_wired"`
	Status         string  `json:"status"`
	LastUplinkName string  `json:"last_uplink_name"`
	LastUplinkMAC  string  `json:"last_uplink_mac"` // MAC of uplink device (switch for wired, AP for wireless)
	Uptime         int     `json:"uptime"`
	ESSID          *string `json:"essid"`
	Signal         *int    `json:"signal"`
	LastSeen       int64   `json:"last_seen"`
}
```

- [ ] **Step 5: Verify build**

```bash
make build 2>&1 | grep -v "network\|docker" | head -20
```

The adapter package itself should compile. Network/docker errors are expected (broken by generate step).

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/unifi.go
git commit -m "feat: extend UniFi adapter structs with port_table, uplink, traffic, and client fields"
```

---

## Task 4: Update testdata fixtures

**Files:**
- Rewrite: `internal/network/testdata/unifi-devices.json`
- Rewrite: `internal/network/testdata/unifi-clients.json`
- Rewrite: `internal/network/testdata/unifi-v2-history.json`
- Rewrite: `internal/network/testdata/unifi-v2-active.json`

Cross-reference key (used by tests throughout):
- Device `aa:bb:cc:dd:00:01` = USG 3P (gateway, root)
- Device `aa:bb:cc:dd:00:02` = US 8 60W (switch, ports 3→nas-1, 5→UAP-01, 6→Switch Flex Mini)
- Device `aa:bb:cc:dd:00:03` = Switch Flex Mini (switch, uplink→US 8 60W port 6)
- Device `aa:bb:cc:dd:00:04` = UAP-01 (AP, uplink→US 8 60W port 5)
- Device `aa:bb:cc:dd:00:05` = UAP-02 (AP, disconnected)
- Client `3c:22:fb:09:aa:b1` = MacBook Pro (wireless, ap_mac→UAP-01)
- Client `68:d7:9a:12:bb:c2` = nas-1 (wired, sw_mac→US 8 60W port 3, speed 1000)
- Client `11:22:33:44:55:03` = Nintendo Switch (wireless, ap_mac→UAP-01)
- Client `a4:83:e7:5f:cc:d3` = iPhone (wireless, ap_mac→UAP-01)
- Client `ec:b5:fa:22:d1:dc` = unnamed (wired, sw_mac→aa:bb:cc:dd:00:04 port 6)

- [ ] **Step 1: Rewrite unifi-devices.json**

```json
{
  "meta": {"rc": "ok"},
  "data": [
    {
      "_id": "000000000000000000000001",
      "mac": "aa:bb:cc:dd:00:01",
      "name": "USG 3P",
      "model": "UGW3",
      "type": "ugw",
      "state": 1,
      "ip": "192.168.0.1",
      "version": "4.4.57.5578372",
      "uptime": 16066061,
      "user-num_sta": 9,
      "guest-num_sta": 0,
      "tx_bytes": 306168680348,
      "rx_bytes": 3913232707026,
      "wan1": {
        "tx_bytes": 306168680348,
        "rx_bytes": 3913232707026,
        "tx_bytes-r": 79558.0,
        "rx_bytes-r": 134932.0
      }
    },
    {
      "_id": "000000000000000000000002",
      "mac": "aa:bb:cc:dd:00:02",
      "name": "US 8 60W",
      "model": "US8P60",
      "type": "usw",
      "state": 1,
      "ip": "192.168.1.40",
      "version": "7.4.1.16850",
      "uptime": 987970,
      "user-num_sta": 1,
      "guest-num_sta": 0,
      "tx_bytes": 226683708402,
      "rx_bytes": 25312100378,
      "uplink": {
        "uplink_mac": "aa:bb:cc:dd:00:01",
        "uplink_device_name": "USG 3P",
        "uplink_remote_port": null,
        "speed": 1000,
        "tx_bytes-r": 87631.0,
        "rx_bytes-r": 163452.0,
        "tx_bytes": 25312100378,
        "rx_bytes": 226683708402
      },
      "port_table": [
        {"port_idx": 1, "up": true,  "speed": 1000, "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 25312100378, "rx_bytes": 226683708402, "tx_bytes-r": 87631.0, "rx_bytes-r": 163452.0},
        {"port_idx": 2, "up": true,  "speed": 100,  "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 100000,       "rx_bytes": 200000,       "tx_bytes-r": 100.0,    "rx_bytes-r": 200.0},
        {"port_idx": 3, "up": true,  "speed": 1000, "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 36706804229,  "rx_bytes": 8150054814,   "tx_bytes-r": 7380.0,   "rx_bytes-r": 54517.0},
        {"port_idx": 4, "up": false, "speed": 0,    "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 0,            "rx_bytes": 0,            "tx_bytes-r": 0.0,      "rx_bytes-r": 0.0},
        {"port_idx": 5, "up": true,  "speed": 1000, "port_poe": true,  "poe_mode": "auto", "poe_power": "3.00", "tx_bytes": 88380586202, "rx_bytes": 7997575920, "tx_bytes-r": 674.0, "rx_bytes-r": 222.0},
        {"port_idx": 6, "up": true,  "speed": 1000, "port_poe": true,  "poe_mode": "auto", "poe_power": "1.61", "tx_bytes": 20169620222, "rx_bytes": 1080138674, "tx_bytes-r": 262.0, "rx_bytes-r": 479.0},
        {"port_idx": 7, "up": true,  "speed": 1000, "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 5000000,      "rx_bytes": 3000000,      "tx_bytes-r": 0.0,      "rx_bytes-r": 0.0},
        {"port_idx": 8, "up": false, "speed": 0,    "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 0,            "rx_bytes": 0,            "tx_bytes-r": 0.0,      "rx_bytes-r": 0.0}
      ]
    },
    {
      "_id": "000000000000000000000003",
      "mac": "aa:bb:cc:dd:00:03",
      "name": "Switch Flex Mini",
      "model": "USMINI",
      "type": "usw",
      "state": 1,
      "ip": "192.168.1.41",
      "version": "2.1.6.762",
      "uptime": 16065407,
      "user-num_sta": 4,
      "guest-num_sta": 0,
      "tx_bytes": 1220180199127,
      "rx_bytes": 27798639955,
      "uplink": {
        "uplink_mac": "aa:bb:cc:dd:00:02",
        "uplink_device_name": "US 8 60W",
        "uplink_remote_port": 6,
        "speed": 1000,
        "tx_bytes-r": 311.0,
        "rx_bytes-r": 200.0,
        "tx_bytes": 27798639955,
        "rx_bytes": 1220180199127
      },
      "port_table": [
        {"port_idx": 1, "up": true,  "speed": 1000, "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 1220180199127, "rx_bytes": 27798639955, "tx_bytes-r": 311.0, "rx_bytes-r": 200.0},
        {"port_idx": 2, "up": true,  "speed": 100,  "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 50000,         "rx_bytes": 100000,      "tx_bytes-r": 50.0,  "rx_bytes-r": 100.0},
        {"port_idx": 3, "up": false, "speed": 0,    "port_poe": false, "poe_mode": null, "poe_power": null, "tx_bytes": 0,             "rx_bytes": 0,           "tx_bytes-r": 0.0,   "rx_bytes-r": 0.0}
      ]
    },
    {
      "_id": "000000000000000000000004",
      "mac": "aa:bb:cc:dd:00:04",
      "name": "UAP-01",
      "model": "U7LT",
      "type": "uap",
      "state": 1,
      "ip": "192.168.1.26",
      "version": "6.8.2.15592",
      "uptime": 89636,
      "user-num_sta": 7,
      "guest-num_sta": 0,
      "tx_bytes": 97191129029,
      "rx_bytes": 8316685650,
      "uplink": {
        "uplink_mac": "aa:bb:cc:dd:00:02",
        "uplink_device_name": "US 8 60W",
        "uplink_remote_port": 5,
        "speed": 1000,
        "tx_bytes-r": 674.0,
        "rx_bytes-r": 222.0,
        "tx_bytes": 8316685650,
        "rx_bytes": 97191129029
      }
    },
    {
      "_id": "000000000000000000000005",
      "mac": "aa:bb:cc:dd:00:05",
      "name": "UAP-02",
      "model": "U7LT",
      "type": "uap",
      "state": 0,
      "ip": "192.168.1.6",
      "version": "6.8.2.15592",
      "uptime": 0,
      "user-num_sta": 0,
      "guest-num_sta": 0,
      "tx_bytes": 0,
      "rx_bytes": 0
    }
  ]
}
```

- [ ] **Step 2: Rewrite unifi-clients.json**

```json
{
  "meta": {"rc": "ok"},
  "data": [
    {
      "_id": "000000000000000000000011",
      "mac": "3c:22:fb:09:aa:b1",
      "hostname": "macbook",
      "name": "MacBook Pro",
      "ip": "192.168.10.67",
      "is_wired": false,
      "essid": "homelab",
      "signal": -69,
      "rssi": 27,
      "uptime": 27075,
      "sw_mac": "",
      "sw_port": 0,
      "last_uplink_name": "",
      "ap_mac": "aa:bb:cc:dd:00:04",
      "wired_rate_mbps": 0
    },
    {
      "_id": "000000000000000000000012",
      "mac": "68:d7:9a:12:bb:c2",
      "hostname": "nas-1",
      "name": null,
      "ip": "192.168.100.5",
      "is_wired": true,
      "essid": null,
      "signal": null,
      "rssi": null,
      "uptime": 1024199,
      "sw_mac": "aa:bb:cc:dd:00:02",
      "sw_port": 3,
      "last_uplink_name": "Switch Living Room",
      "ap_mac": "",
      "wired_rate_mbps": 1000
    },
    {
      "_id": "000000000000000000000013",
      "mac": "11:22:33:44:55:03",
      "hostname": null,
      "name": "Nintendo Switch",
      "ip": "192.168.20.7",
      "is_wired": false,
      "essid": "homelab-iot",
      "signal": -49,
      "rssi": 47,
      "uptime": 83007,
      "sw_mac": "",
      "sw_port": 0,
      "last_uplink_name": "",
      "ap_mac": "aa:bb:cc:dd:00:04",
      "wired_rate_mbps": 0
    },
    {
      "_id": "000000000000000000000014",
      "mac": "a4:83:e7:5f:cc:d3",
      "hostname": "iPhone",
      "name": null,
      "ip": "192.168.10.45",
      "is_wired": false,
      "essid": "homelab",
      "signal": -66,
      "rssi": 30,
      "uptime": 6130,
      "sw_mac": "",
      "sw_port": 0,
      "last_uplink_name": "",
      "ap_mac": "aa:bb:cc:dd:00:04",
      "wired_rate_mbps": 0
    },
    {
      "_id": "000000000000000000000015",
      "mac": "ec:b5:fa:22:d1:dc",
      "hostname": null,
      "name": null,
      "ip": "192.168.10.13",
      "is_wired": true,
      "essid": null,
      "signal": null,
      "rssi": null,
      "uptime": 988376,
      "sw_mac": "aa:bb:cc:dd:00:04",
      "sw_port": 6,
      "last_uplink_name": "Switch Office",
      "ap_mac": "",
      "wired_rate_mbps": 100
    }
  ]
}
```

- [ ] **Step 3: Rewrite unifi-v2-history.json**

`essid` is now populated for offline wireless (last-known SSID, required by WirelessConnection.ssid).

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
      "last_uplink_mac": "aa:bb:cc:dd:00:04",
      "uptime": 0,
      "essid": "homelab",
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
      "last_uplink_mac": "aa:bb:cc:dd:00:03",
      "uptime": 0,
      "essid": null,
      "signal": null,
      "last_seen": 1778829796
    }
  ]
}
```

- [ ] **Step 4: Rewrite unifi-v2-active.json**

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
      "last_uplink_mac": "aa:bb:cc:dd:00:04",
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
      "last_uplink_mac": "aa:bb:cc:dd:00:02",
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
      "last_uplink_mac": "aa:bb:cc:dd:00:04",
      "uptime": 86400,
      "essid": "homelab-iot",
      "signal": -58,
      "last_seen": 0
    }
  ]
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/network/testdata/
git commit -m "test: update fixtures with port_table, uplink, traffic, ap_mac, wired_rate_mbps, last_uplink_mac"
```

---

## Task 5: uri on list items + remove numClients from device list

**Files:**
- Modify: `internal/network/devices_service.go` — `deviceToList`
- Modify: `internal/network/clients_service.go` — `clientToList`, `clientToListV2`
- Modify: `internal/network/service_test.go` — update `TestListDevices`, `TestListClientsIDAndFields`, add uri assertions

After `make generate`, `NetworkDevice` has a required `Uri string` field and no longer has `NumClients`. `NetworkClient` has a required `Uri string` field. Build currently fails because the code sets `NumClients` on `NetworkDevice` (field gone) and doesn't set `Uri`.

- [ ] **Step 1: Write failing tests**

Replace `TestListDevices` and add uri assertions in `TestListClientsIDAndFields` in `service_test.go`:

```go
func TestListDevices(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	result, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 devices, got %d", len(result.Items))
	}

	gw := result.Items[0]
	if gw.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.usg-3p" {
		t.Errorf("expected uri /network/devices/unifi.usg-3p, got %s", gw.Uri)
	}
	if gw.Type != Gateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != Connected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}

	sw := result.Items[1]
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Uri != "/network/devices/unifi.us-8-60w" {
		t.Errorf("expected uri /network/devices/unifi.us-8-60w, got %s", sw.Uri)
	}
	if sw.Type != Switch {
		t.Errorf("expected type switch, got %s", sw.Type)
	}

	ap := result.Items[3]
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Type != AccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}

	offline := result.Items[4]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected, got %s", offline.Status)
	}
}
```

Add to `TestListClientsIDAndFields` (the existing test body stays; add these assertions):
```go
	// Verify uri on list items
	if mb.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected uri /network/clients/unifi.macbook-pro-3c, got %s", mb.Uri)
	}
	if nas.Uri != "/network/clients/unifi.nas-1-68" {
		t.Errorf("expected uri /network/clients/unifi.nas-1-68, got %s", nas.Uri)
	}
```

- [ ] **Step 2: Run to confirm failures**

```bash
go test ./internal/network/ -run "TestListDevices|TestListClientsIDAndFields" -v 2>&1 | head -30
```

Expected: compile errors (NumClients field gone, Uri field missing).

- [ ] **Step 3: Fix deviceToList in devices_service.go**

Replace the `deviceToList` function:

```go
func deviceToList(controller string, d adapters.UniFiDevice) NetworkDevice {
	mac := normalizeMac(d.MAC)
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	return NetworkDevice{
		Id:     id,
		Uri:    fmt.Sprintf("/network/devices/%s", id),
		Name:   d.Name,
		Mac:    mac,
		Ip:     d.IP,
		Type:   mapDeviceType(d.Type),
		Status: mapDeviceStatus(d.State),
	}
}
```

- [ ] **Step 4: Fix clientToList and clientToListV2 in clients_service.go**

Replace `clientToList`:

```go
func clientToList(controller string, sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	client := NetworkClient{
		Id:             id,
		Uri:            fmt.Sprintf("/network/clients/%s", id),
		Name:           clientName(sta),
		Mac:            mac,
		ConnectionType: mapConnectionType(sta.IsWired),
		Status:         Online,
	}
	if sta.IP != "" {
		ip := sta.IP
		client.Ip = &ip
	}
	return client
}
```

Replace `clientToListV2`:

```go
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
		Uri:            fmt.Sprintf("/network/clients/%s", id),
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

- [ ] **Step 5: Run tests**

```bash
go test ./internal/network/ -run "TestListDevices|TestListClientsIDAndFields|TestListClientsAll|TestListClientsOnlineFilter|TestListClientsOfflineFilter|TestListClientsEmpty|TestListDevicesEmpty" -v
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/network/devices_service.go internal/network/clients_service.go internal/network/service_test.go
git commit -m "feat: add uri to NetworkDevice and NetworkClient list items; remove numClients from device list"
```

---

## Task 6: GetDevice — GatewayDetail and UnknownDeviceDetail

**Files:**
- Modify: `internal/network/devices_service.go` — refactor GetDevice to dual-fetch + dispatch
- Modify: `internal/network/service_test.go` — replace TestGetDevice, add TestGetDevice_Unknown

This task establishes the dual-fetch + index pattern and the From*/As* polymorphic dispatch. Switch and AP variants are stubbed; they'll be filled in Tasks 7–9.

- [ ] **Step 1: Add helper functions at the bottom of devices_service.go**

```go
// buildMacToDevice indexes devices by normalized MAC.
func buildMacToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice, len(devices))
	for _, d := range devices {
		m[normalizeMac(d.MAC)] = d
	}
	return m
}

// buildSwPortToDevice indexes downstream devices by "uplinkMAC:remotePort" key.
// Key: normalized MAC of the upstream device + ":" + port index on upstream.
func buildSwPortToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice)
	for _, d := range devices {
		if d.Uplink == nil || d.Uplink.UplinkMAC == "" || d.Uplink.UplinkRemotePort == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", normalizeMac(d.Uplink.UplinkMAC), *d.Uplink.UplinkRemotePort)
		m[key] = d
	}
	return m
}

// buildSwPortToClient indexes wired clients by "switchMAC:portIdx" key.
func buildSwPortToClient(clients []adapters.UniFiSta) map[string]adapters.UniFiSta {
	m := make(map[string]adapters.UniFiSta)
	for _, c := range clients {
		if !c.IsWired || c.SwMAC == "" || c.SwPort == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", normalizeMac(c.SwMAC), c.SwPort)
		m[key] = c
	}
	return m
}

// buildApMacToClients groups wireless clients by their AP MAC.
func buildApMacToClients(clients []adapters.UniFiSta) map[string][]adapters.UniFiSta {
	m := make(map[string][]adapters.UniFiSta)
	for _, c := range clients {
		if c.IsWired || c.ApMAC == "" {
			continue
		}
		mac := normalizeMac(c.ApMAC)
		m[mac] = append(m[mac], c)
	}
	return m
}

// deviceRef builds a NetworkDeviceRef from a known device and its controller.
func deviceRef(controller string, d adapters.UniFiDevice) NetworkDeviceRef {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	return NetworkDeviceRef{
		Kind: "device",
		Id:   id,
		Uri:  fmt.Sprintf("/network/devices/%s", id),
		Name: d.Name,
	}
}

// clientRef builds a NetworkClientRef from a known STA client and its controller.
func clientRef(controller string, sta adapters.UniFiSta) NetworkClientRef {
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	return NetworkClientRef{
		Kind: "client",
		Id:   id,
		Uri:  fmt.Sprintf("/network/clients/%s", id),
		Name: clientName(sta),
	}
}

// deviceTraffic extracts NetworkTraffic from a UniFiDevice.
func deviceTraffic(d adapters.UniFiDevice) NetworkTraffic {
	switch d.Type {
	case "ugw", "udm", "udm-pro":
		if d.Wan1 != nil {
			return NetworkTraffic{
				RxBytesTotal:  d.Wan1.RxBytes,
				TxBytesTotal:  d.Wan1.TxBytes,
				RxBytesPerSec: int(d.Wan1.RxBytesR),
				TxBytesPerSec: int(d.Wan1.TxBytesR),
			}
		}
		return NetworkTraffic{}
	default:
		rxR, txR := 0.0, 0.0
		if d.Uplink != nil {
			rxR = d.Uplink.RxBytesR
			txR = d.Uplink.TxBytesR
		}
		return NetworkTraffic{
			RxBytesTotal:  d.RxBytes,
			TxBytesTotal:  d.TxBytes,
			RxBytesPerSec: int(rxR),
			TxBytesPerSec: int(txR),
		}
	}
}

// deviceUplink builds the NetworkConnection for a device's uplink (nil for gateway / unknown uplink).
func deviceUplink(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice) *NetworkConnection {
	if d.Uplink == nil || d.Uplink.UplinkMAC == "" {
		return nil
	}
	upstream, ok := macToDevice[normalizeMac(d.Uplink.UplinkMAC)]
	if !ok {
		return nil
	}
	ref := deviceRef(controller, upstream)
	conn := &NetworkConnection{Device: ref}
	if d.Uplink.UplinkRemotePort != nil {
		port := *d.Uplink.UplinkRemotePort
		conn.Port = &port
	}
	if d.Uplink.Speed > 0 {
		ls := mapLinkSpeed(d.Uplink.Speed)
		if ls != "" {
			conn.LinkSpeed = &ls
		}
	}
	return conn
}

// mapLinkSpeed converts a UniFi speed in Mbps to the NetworkLinkSpeed enum value.
// Returns "" for unknown speeds.
func mapLinkSpeed(mbps int) NetworkLinkSpeed {
	switch mbps {
	case 10:
		return "e"
	case 100:
		return "fe"
	case 1000:
		return "gbe1"
	case 2500:
		return "gbe2_5"
	case 5000:
		return "gbe5"
	case 10000:
		return "gbe10"
	default:
		return ""
	}
}

// mapPortState converts Up bool to NetworkPortState.
func mapPortState(up bool) NetworkPortState {
	if up {
		return "up"
	}
	return "down"
}

// mapPoeMode converts a UniFi poe_mode string to SwitchPortPoeMode.
func mapPoeMode(mode string) SwitchPortPoeMode {
	switch mode {
	case "auto":
		return "auto"
	case "passive24v":
		return "passive24v"
	case "passthrough":
		return "passthrough"
	default:
		return "off"
	}
}
```

**Note:** After running `make generate`, verify that `NetworkLinkSpeed`, `NetworkPortState`, `SwitchPortPoeMode` are defined as `type X string` in `api.gen.go`. The string literals `"e"`, `"fe"`, `"up"`, `"down"`, `"auto"`, `"off"` must match the generated constant values. Check with:
```bash
grep -A 10 "type NetworkLinkSpeed\|type NetworkPortState\|type SwitchPortPoeMode" internal/network/api.gen.go
```

- [ ] **Step 2: Replace GetDevice and deviceToDetail in devices_service.go**

Replace the existing `GetDevice` function and `deviceToDetail` function:

```go
// GetDevice looks up a single device by composite ID and returns its typed detail.
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkDeviceDetail{}, false, nil
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkDeviceDetail{}, false, nil
	}

	devices, err := backend.GetDevices()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}

	clients, err := backend.GetClients()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	macToDevice := buildMacToDevice(devices)
	swPortToDevice := buildSwPortToDevice(devices)
	swPortToClient := buildSwPortToClient(clients)
	apMacToClients := buildApMacToClients(clients)

	for _, d := range devices {
		if toKebab(d.Name) == suffix {
			detail, err := buildDeviceDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, apMacToClients)
			if err != nil {
				return NetworkDeviceDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkDeviceDetail{}, false, nil
}

func buildDeviceDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	apMacToClients map[string][]adapters.UniFiSta,
) (NetworkDeviceDetail, error) {
	switch d.Type {
	case "usw":
		return buildSwitchDetail(controller, d, macToDevice, swPortToDevice, swPortToClient)
	case "uap":
		return buildAPDetail(controller, d, macToDevice, apMacToClients)
	case "ugw", "udm", "udm-pro":
		return buildGatewayDetail(controller, d)
	default:
		return buildUnknownDetail(controller, d, macToDevice)
	}
}

func buildGatewayDetail(controller string, d adapters.UniFiDevice) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	var det NetworkDeviceDetail
	err := det.FromGatewayDetail(GatewayDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            "gateway",
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
	})
	return det, err
}

func buildUnknownDetail(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	var det NetworkDeviceDetail
	err := det.FromUnknownDeviceDetail(UnknownDeviceDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            "unknown",
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
		Uplink:          uplink,
	})
	return det, err
}

// buildSwitchDetail and buildAPDetail are stubbed — implemented in Tasks 7–9.
func buildSwitchDetail(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice, swPortToDevice map[string]adapters.UniFiDevice, swPortToClient map[string]adapters.UniFiSta) (NetworkDeviceDetail, error) {
	// TODO: implemented in Task 7
	return buildGatewayDetail(controller, d) // stub: compiles
}

func buildAPDetail(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice, apMacToClients map[string][]adapters.UniFiSta) (NetworkDeviceDetail, error) {
	// TODO: implemented in Task 9
	return buildGatewayDetail(controller, d) // stub: compiles
}
```

**Note:** Check the generated type names for `GatewayDetail` and `UnknownDeviceDetail` struct fields (especially `Type` field type — it might be `GatewayDetailType` not a plain string). Adjust literals accordingly:
```bash
grep -A 20 "type GatewayDetail struct\|type UnknownDeviceDetail struct" internal/network/api.gen.go
```

- [ ] **Step 3: Update TestGetDevice in service_test.go**

Replace the existing `TestGetDevice` and `TestGetDeviceNotFound` and `TestGetDeviceWrongController`:

```go
func TestGetDevice_Gateway(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.usg-3p")
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
	if gw.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.usg-3p" {
		t.Errorf("expected uri, got %s", gw.Uri)
	}
	if gw.Model != "UGW3" {
		t.Errorf("expected model UGW3, got %s", gw.Model)
	}
	if gw.FirmwareVersion != "4.4.57.5578372" {
		t.Errorf("expected version 4.4.57.5578372, got %s", gw.FirmwareVersion)
	}
	if gw.Uptime != 16066061 {
		t.Errorf("expected uptime 16066061, got %d", gw.Uptime)
	}
	// Gateway has no uplink (root device)
	if gw.Uplink != nil {
		t.Errorf("expected nil uplink for gateway, got %v", gw.Uplink)
	}
	// Traffic from wan1
	if gw.Traffic.TxBytesTotal != 306168680348 {
		t.Errorf("expected tx_bytes 306168680348, got %d", gw.Traffic.TxBytesTotal)
	}
	if gw.Traffic.RxBytesPerSec != 134932 {
		t.Errorf("expected rx_bytes-r 134932, got %d", gw.Traffic.RxBytesPerSec)
	}
}

func TestGetDevice_Unknown(t *testing.T) {
	devices := []adapters.UniFiDevice{{
		ID: "x", MAC: "bb:bb:bb:bb:bb:01", Name: "Weird Box", Model: "WB1",
		Type: "weird", State: 1, IP: "192.168.1.99", Version: "1.0", Uptime: 1000,
		TxBytes: 100, RxBytes: 200,
	}}
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.weird-box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected unknown device to be found")
	}

	u, err := detail.AsUnknownDeviceDetail()
	if err != nil {
		t.Fatalf("expected unknown detail: %v", err)
	}
	if u.Id != "unifi.weird-box" {
		t.Errorf("expected id unifi.weird-box, got %s", u.Id)
	}
	if u.Model != "WB1" {
		t.Errorf("expected model WB1, got %s", u.Model)
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	_, found, err := svc.GetDevice(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected device not to be found")
	}
}

func TestGetDeviceWrongController(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	_, found, err := svc.GetDevice(context.Background(), "other.usg-3p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found for wrong controller")
	}
}
```

**Note:** `AsGatewayDetail()` must exist on `NetworkDeviceDetail` after generate. If the generated name is different, adjust. Check with:
```bash
grep "AsGatewayDetail\|AsUnknownDeviceDetail\|AsSwitchDetail\|AsAccessPointDetail" internal/network/api.gen.go
```

Also check exact field names on `GatewayDetail` (e.g. `Traffic`, `Uplink`, `TxBytesTotal`) from:
```bash
grep -A 30 "type GatewayDetail struct" internal/network/api.gen.go
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/network/ -run "TestGetDevice_Gateway|TestGetDevice_Unknown|TestGetDeviceNotFound|TestGetDeviceWrongController" -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: refactor GetDevice to dual-fetch with indexes; implement GatewayDetail and UnknownDeviceDetail"
```

---

## Task 7: GetDevice — SwitchDetail with ports

**Files:**
- Modify: `internal/network/devices_service.go` — replace `buildSwitchDetail` stub
- Modify: `internal/network/service_test.go` — add `TestGetDevice_Switch`

Builds a `SwitchDetail` with `ports[]`. ConnectedTo on ports is handled in Tasks 8 and 9; ports without a known connection simply have nil `ConnectedTo`.

- [ ] **Step 1: Write failing test**

Add to `service_test.go`:

```go
func TestGetDevice_Switch(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected switch to be found")
	}

	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Model != "US8P60" {
		t.Errorf("expected model US8P60, got %s", sw.Model)
	}
	if len(sw.Ports) != 8 {
		t.Fatalf("expected 8 ports, got %d", len(sw.Ports))
	}
	// Port 1: up, 1000 Mbps → gbe1, no PoE
	p1 := sw.Ports[0]
	if p1.Number != 1 {
		t.Errorf("expected port number 1, got %d", p1.Number)
	}
	if p1.State != "up" {
		t.Errorf("expected port 1 up, got %s", p1.State)
	}
	if p1.LinkSpeed == nil || *p1.LinkSpeed != "gbe1" {
		t.Errorf("expected link speed gbe1, got %v", p1.LinkSpeed)
	}
	if p1.PoeMode != "off" {
		t.Errorf("expected poe mode off, got %s", p1.PoeMode)
	}
	if p1.PoePowerWatts != nil {
		t.Errorf("expected nil poe power for non-poe port, got %v", p1.PoePowerWatts)
	}
	// Port 4: down → no linkSpeed
	p4 := sw.Ports[3]
	if p4.State != "down" {
		t.Errorf("expected port 4 down, got %s", p4.State)
	}
	if p4.LinkSpeed != nil {
		t.Errorf("expected nil link speed for down port, got %v", p4.LinkSpeed)
	}
	// Port 5: PoE auto, 3.00 W
	p5 := sw.Ports[4]
	if p5.PoeMode != "auto" {
		t.Errorf("expected poe auto, got %s", p5.PoeMode)
	}
	if p5.PoePowerWatts == nil || *p5.PoePowerWatts != 3.00 {
		t.Errorf("expected poe power 3.00, got %v", p5.PoePowerWatts)
	}
	// Traffic on port 1
	if p1.Traffic.TxBytesTotal != 25312100378 {
		t.Errorf("expected port 1 tx_bytes 25312100378, got %d", p1.Traffic.TxBytesTotal)
	}
	// Device-level traffic
	if sw.Traffic.TxBytesTotal != 226683708402 {
		t.Errorf("expected device tx_bytes 226683708402, got %d", sw.Traffic.TxBytesTotal)
	}
	// Uplink to USG
	if sw.Uplink == nil {
		t.Fatal("expected uplink for switch")
	}
	if sw.Uplink.Device.Id != "unifi.usg-3p" {
		t.Errorf("expected uplink device unifi.usg-3p, got %s", sw.Uplink.Device.Id)
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
go test ./internal/network/ -run TestGetDevice_Switch -v
```

Expected: FAIL — stub returns gateway detail, not switch detail.

- [ ] **Step 3: Replace buildSwitchDetail stub**

```go
func buildSwitchDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	ports := buildSwitchPorts(controller, d, swPortToDevice, swPortToClient)

	var det NetworkDeviceDetail
	err := det.FromSwitchDetail(SwitchDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            "switch",
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
		Uplink:          uplink,
		Ports:           ports,
	})
	return det, err
}

func buildSwitchPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) []SwitchPort {
	ports := make([]SwitchPort, 0, len(d.PortTable))
	switchMAC := normalizeMac(d.MAC)
	for _, p := range d.PortTable {
		port := SwitchPort{
			Number:  p.PortIdx,
			State:   mapPortState(p.Up),
			PoeMode: mapPoeMode(p.PoeMode),
			Traffic: NetworkTraffic{
				RxBytesTotal:  p.RxBytes,
				TxBytesTotal:  p.TxBytes,
				RxBytesPerSec: int(p.RxBytesR),
				TxBytesPerSec: int(p.TxBytesR),
			},
		}
		if p.Up && p.Speed > 0 {
			ls := mapLinkSpeed(p.Speed)
			if ls != "" {
				port.LinkSpeed = &ls
			}
		}
		if p.PortPoe && p.PoePower != "" {
			watts, err := strconv.ParseFloat(p.PoePower, 64)
			if err == nil {
				port.PoePowerWatts = &watts
			}
		}
		port.ConnectedTo = resolvePortConnectedTo(controller, switchMAC, p.PortIdx, swPortToDevice, swPortToClient)
		ports = append(ports, port)
	}
	return ports
}

// resolvePortConnectedTo returns a NetworkConnectionRef for the device or client on this port, or nil.
func resolvePortConnectedTo(
	controller string,
	switchMAC string,
	portIdx int,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) *NetworkConnectionRef {
	key := fmt.Sprintf("%s:%d", switchMAC, portIdx)
	if dev, ok := swPortToDevice[key]; ok {
		ref := deviceRef(controller, dev)
		var conn NetworkConnectionRef
		if err := conn.FromNetworkDeviceRef(ref); err == nil {
			return &conn
		}
	}
	if sta, ok := swPortToClient[key]; ok {
		ref := clientRef(controller, sta)
		var conn NetworkConnectionRef
		if err := conn.FromNetworkClientRef(ref); err == nil {
			return &conn
		}
	}
	return nil
}
```

Add `"strconv"` to the import block in `devices_service.go`.

**Note:** Check generated type for `SwitchDetail.Type` field — it may be `SwitchDetailType` not `string`. Check `SwitchPort.PoeMode` type (may be `SwitchPortPoeMode`), `SwitchPort.State` type (may be `NetworkPortState`), `SwitchPort.LinkSpeed` type. Verify `NetworkConnectionRef` has `FromNetworkDeviceRef` / `FromNetworkClientRef` methods. All checks via:
```bash
grep -A 25 "type SwitchDetail struct\|type SwitchPort struct" internal/network/api.gen.go
grep "FromNetworkDeviceRef\|FromNetworkClientRef" internal/network/api.gen.go
```

- [ ] **Step 4: Run test**

```bash
go test ./internal/network/ -run TestGetDevice_Switch -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: implement SwitchDetail with port table, traffic, uplink, and port connectedTo resolution"
```

---

## Task 8: GetDevice_Switch — verify port connectedTo device and client

**Files:**
- Modify: `internal/network/service_test.go` — add cross-reference assertions

The cross-ref logic is already in `resolvePortConnectedTo` from Task 7. This task adds explicit test assertions to verify it works end-to-end.

- [ ] **Step 1: Add cross-ref tests**

Add to `service_test.go`:

```go
func TestGetDevice_SwitchPort_ConnectedToDevice(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, _, _ := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}

	// Port 5 → UAP-01 (uplink_remote_port=5 in fixture)
	var port5 *SwitchPort
	for i := range sw.Ports {
		if sw.Ports[i].Number == 5 {
			port5 = &sw.Ports[i]
			break
		}
	}
	if port5 == nil {
		t.Fatal("port 5 not found")
	}
	if port5.ConnectedTo == nil {
		t.Fatal("expected port 5 connectedTo to be set")
	}
	ref, err := port5.ConnectedTo.AsNetworkDeviceRef()
	if err != nil {
		t.Fatalf("expected device ref on port 5: %v", err)
	}
	if ref.Kind != "device" {
		t.Errorf("expected kind=device, got %s", ref.Kind)
	}
	if ref.Id != "unifi.uap-01" {
		t.Errorf("expected device id unifi.uap-01, got %s", ref.Id)
	}
	if ref.Uri != "/network/devices/unifi.uap-01" {
		t.Errorf("expected device uri, got %s", ref.Uri)
	}
	if ref.Name != "UAP-01" {
		t.Errorf("expected device name UAP-01, got %s", ref.Name)
	}
}

func TestGetDevice_SwitchPort_ConnectedToClient(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, _, _ := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}

	// Port 3 → nas-1 (sw_mac=aa:bb:cc:dd:00:02, sw_port=3 in fixture)
	var port3 *SwitchPort
	for i := range sw.Ports {
		if sw.Ports[i].Number == 3 {
			port3 = &sw.Ports[i]
			break
		}
	}
	if port3 == nil {
		t.Fatal("port 3 not found")
	}
	if port3.ConnectedTo == nil {
		t.Fatal("expected port 3 connectedTo to be set")
	}
	ref, err := port3.ConnectedTo.AsNetworkClientRef()
	if err != nil {
		t.Fatalf("expected client ref on port 3: %v", err)
	}
	if ref.Kind != "client" {
		t.Errorf("expected kind=client, got %s", ref.Kind)
	}
	if ref.Id != "unifi.nas-1-68" {
		t.Errorf("expected client id unifi.nas-1-68, got %s", ref.Id)
	}
}
```

**Note:** Check `NetworkConnectionRef` for `AsNetworkDeviceRef` / `AsNetworkClientRef` method names:
```bash
grep "AsNetworkDeviceRef\|AsNetworkClientRef" internal/network/api.gen.go
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/network/ -run "TestGetDevice_SwitchPort" -v
```

Expected: both pass.

- [ ] **Step 3: Commit**

```bash
git add internal/network/service_test.go
git commit -m "test: add switch port connectedTo device and client cross-reference tests"
```

---

## Task 9: GetDevice — AccessPointDetail

**Files:**
- Modify: `internal/network/devices_service.go` — replace `buildAPDetail` stub
- Modify: `internal/network/service_test.go` — add `TestGetDevice_AccessPoint`

- [ ] **Step 1: Write failing test**

Add to `service_test.go`:

```go
func TestGetDevice_AccessPoint(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.uap-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected AP to be found")
	}

	ap, err := detail.AsAccessPointDetail()
	if err != nil {
		t.Fatalf("expected access point detail: %v", err)
	}
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Model != "U7LT" {
		t.Errorf("expected model U7LT, got %s", ap.Model)
	}
	// 3 wireless clients in fixture have ap_mac=aa:bb:cc:dd:00:04 (MacBook Pro, Nintendo Switch, iPhone)
	if ap.NumClients != 3 {
		t.Errorf("expected 3 numClients, got %d", ap.NumClients)
	}
	if len(ap.ConnectedClients) != 3 {
		t.Fatalf("expected 3 connectedClients, got %d", len(ap.ConnectedClients))
	}
	// Find MacBook Pro
	var mb *AccessPointClient
	for i := range ap.ConnectedClients {
		if ap.ConnectedClients[i].Client.Name == "MacBook Pro" {
			mb = &ap.ConnectedClients[i]
			break
		}
	}
	if mb == nil {
		t.Fatal("MacBook Pro not found in connectedClients")
	}
	if mb.Client.Id != "unifi.macbook-pro-3c" {
		t.Errorf("expected client id unifi.macbook-pro-3c, got %s", mb.Client.Id)
	}
	if mb.Client.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected client uri, got %s", mb.Client.Uri)
	}
	if mb.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", mb.Ssid)
	}
	if mb.SignalStrength == nil || *mb.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %v", mb.SignalStrength)
	}
	// Uplink to US 8 60W
	if ap.Uplink == nil {
		t.Fatal("expected uplink for AP")
	}
	if ap.Uplink.Device.Id != "unifi.us-8-60w" {
		t.Errorf("expected uplink device unifi.us-8-60w, got %s", ap.Uplink.Device.Id)
	}
	if ap.Uplink.Port == nil || *ap.Uplink.Port != 5 {
		t.Errorf("expected uplink port 5, got %v", ap.Uplink.Port)
	}
}
```

- [ ] **Step 2: Run to confirm it fails**

```bash
go test ./internal/network/ -run TestGetDevice_AccessPoint -v
```

Expected: FAIL — stub returns gateway detail.

- [ ] **Step 3: Replace buildAPDetail stub**

```go
func buildAPDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	apMacToClients map[string][]adapters.UniFiSta,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	apMAC := normalizeMac(d.MAC)
	stas := apMacToClients[apMAC]

	connectedClients := make([]AccessPointClient, 0, len(stas))
	for _, sta := range stas {
		ref := clientRef(controller, sta)
		apc := AccessPointClient{Client: ref}
		if sta.ESSID != nil {
			apc.Ssid = *sta.ESSID
		}
		if sta.Signal != nil {
			apc.SignalStrength = sta.Signal
		}
		connectedClients = append(connectedClients, apc)
	}

	var det NetworkDeviceDetail
	err := det.FromAccessPointDetail(AccessPointDetail{
		Id:               id,
		Uri:              fmt.Sprintf("/network/devices/%s", id),
		Name:             d.Name,
		Mac:              normalizeMac(d.MAC),
		Ip:               d.IP,
		Type:             "accessPoint",
		Status:           mapDeviceStatus(d.State),
		Model:            d.Model,
		FirmwareVersion:  d.Version,
		Uptime:           d.Uptime,
		Traffic:          deviceTraffic(d),
		Uplink:           uplink,
		NumClients:       len(connectedClients),
		ConnectedClients: connectedClients,
	})
	return det, err
}
```

**Note:** Check `AccessPointDetail` and `AccessPointClient` struct field names:
```bash
grep -A 20 "type AccessPointDetail struct\|type AccessPointClient struct" internal/network/api.gen.go
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/network/ -run TestGetDevice_AccessPoint -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: implement AccessPointDetail with connectedClients and client refs"
```

---

## Task 10: GetClient — wired and wireless online connectedTo

**Files:**
- Modify: `internal/network/clients_service.go` — refactor `clientToDetail` for new connectedTo shape
- Modify: `internal/network/service_test.go` — replace `TestGetClientWired`, `TestGetClientWireless`

`GetClient` for online clients calls `GetClients()` (STA format). We add a `GetDevices()` call to build `macToDevice` for resolving device refs.

- [ ] **Step 1: Write failing tests**

Replace `TestGetClientWired` and `TestGetClientWireless` in `service_test.go`:

```go
func TestGetClientWireless(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.macbook-pro-3c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wireless, err := detail.AsWirelessNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wireless detail: %v", err)
	}
	if wireless.ConnectedTo.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", wireless.ConnectedTo.Ssid)
	}
	if wireless.ConnectedTo.SignalStrength == nil || *wireless.ConnectedTo.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %v", wireless.ConnectedTo.SignalStrength)
	}
	if wireless.ConnectedTo.Device.Id != "unifi.uap-01" {
		t.Errorf("expected device unifi.uap-01, got %s", wireless.ConnectedTo.Device.Id)
	}
	if wireless.ConnectedTo.Device.Uri != "/network/devices/unifi.uap-01" {
		t.Errorf("expected device uri, got %s", wireless.ConnectedTo.Device.Uri)
	}
	if wireless.Uptime == nil || *wireless.Uptime != 27075 {
		t.Errorf("expected uptime 27075, got %v", wireless.Uptime)
	}
	if wireless.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected uri, got %s", wireless.Uri)
	}
}

func TestGetClientWired(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.nas-1-68")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wired, err := detail.AsWiredNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wired detail: %v", err)
	}
	if wired.ConnectedTo.Device.Id != "unifi.us-8-60w" {
		t.Errorf("expected device unifi.us-8-60w, got %s", wired.ConnectedTo.Device.Id)
	}
	if wired.ConnectedTo.Port == nil || *wired.ConnectedTo.Port != 3 {
		t.Errorf("expected port 3, got %v", wired.ConnectedTo.Port)
	}
	if wired.ConnectedTo.LinkSpeed == nil || *wired.ConnectedTo.LinkSpeed != "gbe1" {
		t.Errorf("expected link speed gbe1, got %v", wired.ConnectedTo.LinkSpeed)
	}
	if wired.Uptime == nil || *wired.Uptime != 1024199 {
		t.Errorf("expected uptime 1024199, got %v", wired.Uptime)
	}
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
go test ./internal/network/ -run "TestGetClientWired|TestGetClientWireless" -v 2>&1 | head -20
```

Expected: FAIL — `ConnectedTo` field doesn't exist (old fields `SwitchName`, `Ssid` are gone after generate).

- [ ] **Step 3: Refactor GetClient in clients_service.go**

Add `GetDevices()` call and refactor `clientToDetail`:

In `GetClient`, replace the section that calls `clientToDetail(controller, sta)`:

```go
// GetClient looks up a single client by composite ID and returns its typed detail.
func (s *Service) GetClient(ctx context.Context, id string) (NetworkClientDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkClientDetail{}, false, nil
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkClientDetail{}, false, nil
	}

	// Fetch devices for cross-reference (device refs in connectedTo).
	devices, err := backend.GetDevices()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}
	macToDevice := buildMacToDevice(devices)

	raw, err := backend.GetClients()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	for _, sta := range raw {
		if clientSuffix(sta) == suffix {
			detail, err := clientToDetail(controller, sta, macToDevice)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}

	// Not found in active clients — check offline history.
	offline, err := backend.GetOfflineClients(s.historyDays)
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi offline clients: %w", err)
	}

	for _, c := range offline {
		name := clientNameV2(c)
		mac := normalizeMac(c.MAC)
		prefix := strings.ReplaceAll(mac, ":", "")[:2]
		if fmt.Sprintf("%s-%s", toKebab(name), prefix) == suffix {
			detail, err := clientToDetailV2(controller, c, macToDevice)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkClientDetail{}, false, nil
}
```

Replace `clientToDetail`:

```go
func clientToDetail(controller string, sta adapters.UniFiSta, macToDevice map[string]adapters.UniFiDevice) (NetworkClientDetail, error) {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	name := clientName(sta)
	var ip *string
	if sta.IP != "" {
		v := sta.IP
		ip = &v
	}

	var detail NetworkClientDetail
	if sta.IsWired {
		conn := NetworkConnection{}
		if dev, ok := macToDevice[normalizeMac(sta.SwMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if sta.SwPort > 0 {
			port := sta.SwPort
			conn.Port = &port
		}
		if sta.WiredRateMbps > 0 {
			ls := mapLinkSpeed(sta.WiredRateMbps)
			if ls != "" {
				conn.LinkSpeed = &ls
			}
		}
		uptime := sta.Uptime
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			ConnectedTo:    conn,
			Uptime:         &uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wired client detail: %w", err)
		}
	} else {
		conn := WirelessConnection{}
		if dev, ok := macToDevice[normalizeMac(sta.ApMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if sta.ESSID != nil {
			conn.Ssid = *sta.ESSID
		}
		conn.SignalStrength = sta.Signal
		uptime := sta.Uptime
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			ConnectedTo:    conn,
			Uptime:         &uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wireless client detail: %w", err)
		}
	}
	return detail, nil
}
```

**Note:** After generate, `WiredNetworkClientDetail` has `ConnectedTo NetworkConnection` and `Uri string` field. `WirelessNetworkClientDetail` has `ConnectedTo WirelessConnection` and `Uri string`. Verify field names:
```bash
grep -A 25 "type WiredNetworkClientDetail struct\|type WirelessNetworkClientDetail struct" internal/network/api.gen.go
```

Also check the `WirelessNetworkClientDetailConnectionType` constant name — currently it's `Wireless`:
```bash
grep "Wireless\b" internal/network/api.gen.go | head -5
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/network/ -run "TestGetClientWired|TestGetClientWireless|TestGetClientNotFound" -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/network/clients_service.go internal/network/service_test.go
git commit -m "feat: implement online client connectedTo with NetworkConnection and WirelessConnection device refs"
```

---

## Task 11: GetClient — offline connectedTo

**Files:**
- Modify: `internal/network/clients_service.go` — refactor `clientToDetailV2`
- Modify: `internal/network/service_test.go` — replace `TestGetClientOfflineWired`, `TestGetClientOfflineWireless`

- [ ] **Step 1: Write failing tests**

Replace `TestGetClientOfflineWired` and `TestGetClientOfflineWireless`:

```go
func TestGetClientOfflineWired(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		devices:        devices,
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
	if wired.Status != Offline {
		t.Errorf("expected offline, got %s", wired.Status)
	}
	// Device ref from last_uplink_mac → Switch Flex Mini (aa:bb:cc:dd:00:03)
	if wired.ConnectedTo.Device.Id != "unifi.switch-flex-mini" {
		t.Errorf("expected device unifi.switch-flex-mini, got %s", wired.ConnectedTo.Device.Id)
	}
	if wired.ConnectedTo.Device.Name != "Switch Flex Mini" {
		t.Errorf("expected name Switch Flex Mini, got %s", wired.ConnectedTo.Device.Name)
	}
	// Port and linkSpeed absent for offline wired
	if wired.ConnectedTo.Port != nil {
		t.Errorf("expected nil port for offline client, got %v", wired.ConnectedTo.Port)
	}
	if wired.ConnectedTo.LinkSpeed != nil {
		t.Errorf("expected nil linkSpeed for offline client, got %v", wired.ConnectedTo.LinkSpeed)
	}
	// Uptime absent for offline
	if wired.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wired.Uptime)
	}
}

func TestGetClientOfflineWireless(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		devices:        devices,
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
	if wireless.Status != Offline {
		t.Errorf("expected offline, got %s", wireless.Status)
	}
	// Device ref from last_uplink_mac → UAP-01 (aa:bb:cc:dd:00:04)
	if wireless.ConnectedTo.Device.Id != "unifi.uap-01" {
		t.Errorf("expected device unifi.uap-01, got %s", wireless.ConnectedTo.Device.Id)
	}
	// SSID from essid (last known)
	if wireless.ConnectedTo.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", wireless.ConnectedTo.Ssid)
	}
	// signalStrength absent for offline
	if wireless.ConnectedTo.SignalStrength != nil {
		t.Errorf("expected nil signalStrength for offline, got %v", wireless.ConnectedTo.SignalStrength)
	}
	if wireless.Ip == nil || *wireless.Ip != "192.168.10.37" {
		t.Errorf("expected ip 192.168.10.37, got %v", wireless.Ip)
	}
	if wireless.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wireless.Uptime)
	}
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
go test ./internal/network/ -run "TestGetClientOfflineWired|TestGetClientOfflineWireless" -v 2>&1 | head -20
```

- [ ] **Step 3: Replace clientToDetailV2**

```go
func clientToDetailV2(controller string, c adapters.UniFiClientV2, macToDevice map[string]adapters.UniFiDevice) (NetworkClientDetail, error) {
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
		conn := NetworkConnection{}
		if dev, ok := macToDevice[normalizeMac(c.LastUplinkMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		// port and linkSpeed absent for offline wired clients
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Offline,
			ConnectedTo:    conn,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wired client detail: %w", err)
		}
	} else {
		conn := WirelessConnection{}
		if dev, ok := macToDevice[normalizeMac(c.LastUplinkMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if c.ESSID != nil {
			conn.Ssid = *c.ESSID
		}
		// signalStrength absent for offline wireless
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Offline,
			ConnectedTo:    conn,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wireless client detail: %w", err)
		}
	}
	return detail, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/network/ -run "TestGetClientOffline|TestGetClientNotFoundInEither" -v
```

Expected: all pass.

- [ ] **Step 5: Run all network tests to catch regressions**

```bash
go test ./internal/network/ -v 2>&1 | tail -30
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/network/clients_service.go internal/network/service_test.go
git commit -m "feat: implement offline client connectedTo with device refs from last_uplink_mac"
```

---

## Task 12: Handler — networkDeviceDetailResponse wrapper

**Files:**
- Modify: `internal/network/handler.go`

`NetworkDeviceDetail` is now anyOf+discriminator. The generated `GetNetworkDevice200JSONResponse` is a new type — not an alias — so `MarshalJSON` is invisible to the encoder and produces `{}`. Need a custom response type.

- [ ] **Step 1: Add the wrapper type to handler.go**

After the existing `networkClientDetailResponse` type, add:

```go
// networkDeviceDetailResponse correctly serializes the polymorphic NetworkDeviceDetail.
// The generated GetNetworkDevice200JSONResponse loses MarshalJSON across the type definition;
// this wrapper delegates encoding directly to NetworkDeviceDetail.MarshalJSON.
type networkDeviceDetailResponse struct {
	detail NetworkDeviceDetail
}

func (r networkDeviceDetailResponse) VisitGetNetworkDeviceResponse(w http.ResponseWriter) error {
	b, err := r.detail.MarshalJSON()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(b)
	return err
}
```

- [ ] **Step 2: Update GetNetworkDevice to return the wrapper**

In `handler.go`, replace the success return in `GetNetworkDevice`:

```go
return networkDeviceDetailResponse{detail: detail}, nil
```

Instead of:

```go
return GetNetworkDevice200JSONResponse(detail), nil
```

- [ ] **Step 3: Build**

```bash
make build
```

Expected: compiles cleanly.

- [ ] **Step 4: Run all tests**

```bash
make test
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/network/handler.go
git commit -m "fix: add networkDeviceDetailResponse wrapper to correctly serialize polymorphic NetworkDeviceDetail"
```

---

## Task 13: Final verification

**Files:** None (read-only verification)

- [ ] **Step 1: Build**

```bash
make build
```

Expected: PASS, no errors.

- [ ] **Step 2: Run all tests**

```bash
make test
```

Expected: PASS, all tests green.

- [ ] **Step 3: Lint**

```bash
make lint
```

Expected: PASS (go vet clean).

- [ ] **Step 4: Smoke-check the full test count**

```bash
go test ./... -v 2>&1 | grep -E "^--- (PASS|FAIL)" | wc -l
```

Expected: count is higher than before this feature branch (new tests added).

- [ ] **Step 5: Confirm no TODO stubs remain**

```bash
grep -rn "TODO: implemented" internal/network/
```

Expected: no output (all stubs replaced in Tasks 7 and 9).
