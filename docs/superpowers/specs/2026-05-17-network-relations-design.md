# Network Relations Design

**Date:** 2026-05-17
**Spec commit:** `067120b` — "Extend network models: switch ports, AP clients, traffic, topology"

## Goal

Implement the updated OpenAPI contract for `/network/devices` and `/network/clients`:

- Polymorphic `NetworkDeviceDetail` (switch ports, AP connected clients, traffic, uplink topology)
- `connectedTo` cross-references on client detail (replaces flat `switchName`/`switchPort`/`ssid`/`signalStrength`)
- `uri` field on list items for both devices and clients
- Docker restart-policy enum rename (`unlessStopped`, `onFailure`)

All data is present in existing captured responses (`scripts/responses/unifi-devices-raw.json`, `unifi-clients-raw.json`). No new UniFi API endpoints needed.

---

## Section 1: Adapter changes (`internal/adapters/unifi.go`)

### Extend `UniFiDevice`

```go
TxBytes   int64            `json:"tx_bytes"`
RxBytes   int64            `json:"rx_bytes"`
PortTable []UniFiPortEntry `json:"port_table"`
Uplink    *UniFiUplink     `json:"uplink"`
Wan1      *UniFiWanIface   `json:"wan1"`  // gateway only
```

### New `UniFiPortEntry`

```go
type UniFiPortEntry struct {
    PortIdx  int     `json:"port_idx"`
    Up       bool    `json:"up"`
    Speed    int     `json:"speed"`       // Mbps (10, 100, 1000, 2500, 5000, 10000)
    PortPoe  bool    `json:"port_poe"`
    PoeMode  string  `json:"poe_mode"`    // "auto", "passive24v", "passthrough", or absent
    PoePower string  `json:"poe_power"`   // watts as string, e.g. "3.00"; absent when no load
    TxBytes  int64   `json:"tx_bytes"`
    RxBytes  int64   `json:"rx_bytes"`
    TxBytesR float64 `json:"tx_bytes-r"`
    RxBytesR float64 `json:"rx_bytes-r"`
}
```

### New `UniFiUplink`

```go
type UniFiUplink struct {
    UplinkMAC        string  `json:"uplink_mac"`          // MAC of upstream device
    UplinkDeviceName string  `json:"uplink_device_name"`  // display name
    UplinkRemotePort *int    `json:"uplink_remote_port"`  // port number on upstream device; nil if unknown
    Speed            int     `json:"speed"`
    TxBytesR         float64 `json:"tx_bytes-r"`
    RxBytesR         float64 `json:"rx_bytes-r"`
    TxBytes          int64   `json:"tx_bytes"`
    RxBytes          int64   `json:"rx_bytes"`
}
```

### New `UniFiWanIface`

```go
type UniFiWanIface struct {
    TxBytes  int64   `json:"tx_bytes"`
    RxBytes  int64   `json:"rx_bytes"`
    TxBytesR float64 `json:"tx_bytes-r"`
    RxBytesR float64 `json:"rx_bytes-r"`
}
```

### Extend `UniFiSta`

Add two fields:
```go
ApMAC        string `json:"ap_mac"`           // MAC of connected AP (wireless clients)
WiredRateMbps int   `json:"wired_rate_mbps"`  // negotiated link speed (wired clients)
```

### Device-level `NetworkTraffic` sourcing

| Device type | Cumulative (rxBytesTotal/txBytesTotal) | Instant (rxBytesPerSec/txBytesPerSec) |
|---|---|---|
| Switch (usw) | `UniFiDevice.TxBytes` / `RxBytes` | `Uplink.TxBytesR` / `RxBytesR` |
| Access point (uap) | `UniFiDevice.TxBytes` / `RxBytes` | `Uplink.TxBytesR` / `RxBytesR` |
| Gateway (ugw/udm) | `Wan1.TxBytes` / `RxBytes` | `Wan1.TxBytesR` / `RxBytesR` |

---

## Section 2: Service layer — cross-reference resolution

### Approach: Dual fetch + in-memory index on detail endpoints

`GetDevice(id)` and `GetClient(id)` each make two backend calls (devices + active clients). This matches the existing pattern where `GetClient` already makes an extra `GetOfflineClients` call.

### `GetDevice(id)` — new logic

```
1. backend.GetDevices() → all devices
2. backend.GetClients() → all active clients (new call)
3. Build indexes:
   - macToDevice    map[string]UniFiDevice    // mac → device
   - swPortToDevice map[string]UniFiDevice    // "uplinkMAC:remotePort" → downstream device (built from each device's uplink)
   - swPortToClient map[string]UniFiSta       // "swMAC:swPort" → wired client
   - apMacToClients map[string][]UniFiSta    // ap_mac → []wireless client
4. Find target device by suffix
5. Dispatch on type → build typed variant
```

**Type dispatch:**
- `usw` → `SwitchDetail`: `ports[]` with `connectedTo` resolved from `swPortToClient` (→ `NetworkClientRef`) and `macToDevice` via uplink MAC (→ `NetworkDeviceRef`)
- `uap` → `AccessPointDetail`: `connectedClients[]` from `apMacToClients[device.MAC]`
- `ugw/udm/udm-pro` → `GatewayDetail`: base fields only, no uplink
- otherwise → `UnknownDeviceDetail`

All variants include `traffic` (device-level) and `uplink` (except gateway). `uplink` is built from `UniFiUplink` into a `NetworkConnection` pointing to `macToDevice[uplink.UplinkMAC]`.

**Switch port `connectedTo` resolution:**
1. Check `swPortToDevice["switchMAC:portIdx"]` → `NetworkDeviceRef`
2. Check `swPortToClient["switchMAC:portIdx"]` → `NetworkClientRef`
3. If neither matches → `connectedTo` omitted

### `GetClient(id)` — new logic

Add `backend.GetDevices()` call to build `macToDevice`. Used to construct `NetworkDeviceRef` in client `connectedTo`.

**Wired online (STA path):**
```
connectedTo: NetworkConnection{
    device:    NetworkDeviceRef from macToDevice[sta.SwMAC],
    port:      sta.SwPort,
    linkSpeed: mapSpeedToEnum(sta.WiredRateMbps),
}
uptime: &sta.Uptime
```

**Wireless online (STA path):**
```
connectedTo: WirelessConnection{
    device:         NetworkDeviceRef from macToDevice[sta.ApMAC],
    ssid:           *sta.ESSID,
    signalStrength: sta.Signal,
}
uptime: &sta.Uptime
```

**Wired offline (v2 path):**
```
connectedTo: NetworkConnection{
    device: NetworkDeviceRef from macToDevice[c.UplinkMAC],
    // port and linkSpeed omitted
}
// uptime omitted
```

**Wireless offline (v2 path):**
```
connectedTo: WirelessConnection{
    device: NetworkDeviceRef from macToDevice[c.UplinkMAC],
    ssid:   *c.ESSID,
    // signalStrength omitted
}
// uptime omitted
```

### `ListDevices` / `ListClients` changes

- Add `Uri` field: `/network/devices/{id}` and `/network/clients/{id}` respectively
- `ListDevices`: remove `NumClients` (moved to `AccessPointDetail`)

### Helper mappings

**Speed (Mbps int → `NetworkLinkSpeed` enum):**
- 10 → `e`, 100 → `fe`, 1000 → `gbe1`, 2500 → `gbe2_5`, 5000 → `gbe5`, 10000 → `gbe10`
- unknown → omit field

**PoE mode (string → `SwitchPortPoeMode` enum):**
- `""` (absent) → `off`
- `"auto"` → `auto`
- `"passive24v"` → `passive24v`
- `"passthrough"` → `passthrough`

**Port state (`Up bool` → `NetworkPortState`):**
- `true` → `up`, `false` → `down`
- (UniFi doesn't surface `disabled` in `port_table`; use `down` for all non-up ports)

---

## Section 3: Handler + generated code

### Regenerate stubs

Run `make generate` first to get updated generated types.

### New device detail response wrapper in `handler.go`

`NetworkDeviceDetail` is now anyOf+discriminator. The generated `GetNetworkDevice200JSONResponse` loses `MarshalJSON` (same issue as `NetworkClientDetail`). Add:

```go
type networkDeviceDetailResponse struct{ detail NetworkDeviceDetail }

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

Return `networkDeviceDetailResponse{detail: detail}` from `GetNetworkDevice` instead of `GetNetworkDevice200JSONResponse(detail)`.

The existing `networkClientDetailResponse` wrapper is unchanged.

### Docker domain (`internal/docker/`)

The generated `ContainerRestartPolicy` enum renames two values. Update the service mapping:
- `unless-stopped` (Docker API string) → `unlessStopped` (new spec enum)
- `on-failure` (Docker API string) → `onFailure` (new spec enum)

This is a string constant update in one function.

---

## Section 4: Testing + fixture updates

### Fixture updates

**`testdata/unifi-devices.json`** — add sanitized `port_table`, `uplink`, `tx_bytes`, `rx_bytes` per device (sourced from `unifi-devices-raw.json`). Add `wan1` to the gateway entry.

**`testdata/unifi-clients.json`** — add `ap_mac` to wireless STA entries, `wired_rate_mbps` to wired STA entries (sourced from `unifi-clients-raw.json`).

### Existing tests to update

- `TestListDevices`: expect `Uri` on each item; remove `NumClients` assertions
- `TestListClients`: expect `Uri` on each item
- `TestGetDevice_*`: all existing device detail tests will need updating for new polymorphic shape

### New tests

- `TestGetDevice_Switch` — ports with state/linkSpeed/poeMode/traffic/connectedTo
- `TestGetDevice_AccessPoint` — connectedClients with client refs
- `TestGetDevice_Gateway` — base fields, no uplink
- `TestGetDevice_Unknown` — catch-all variant
- `TestGetDevice_SwitchPort_ConnectedToDevice` — port connectedTo resolves NetworkDeviceRef
- `TestGetDevice_SwitchPort_ConnectedToClient` — port connectedTo resolves NetworkClientRef
- `TestGetClient_WiredOnline` — connectedTo with device ref + port + linkSpeed
- `TestGetClient_WirelessOnline` — connectedTo with device ref + ssid + signal
- `TestGetClient_WiredOffline` — device ref present, port/linkSpeed absent
- `TestGetClient_WirelessOffline` — device ref + ssid present, signal absent
