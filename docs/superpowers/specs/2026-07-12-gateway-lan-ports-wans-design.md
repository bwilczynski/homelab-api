# Gateway LAN Ports & WAN References — Implementation Design (spec v1.5.0)

## Goal

Implement spec v1.5.0 in the Go server. The spec renames the `SwitchPort*` schema family to `DevicePort*`, makes `poeMode` optional, renames `NetworkPort.switch` to `NetworkPort.device`, adds `ports` and `wans` arrays to `GatewayDetail`, and broadens `/network/ports` to include gateway LAN ports.

## What the spec changes

| Change | Type |
|---|---|
| `SwitchPort*` schemas → `DevicePort*` (5 schemas, 2 parameters) | Rename |
| `DevicePort.poeMode` removed from `required` | Optionality |
| `NetworkPort.switch` → `NetworkPort.device` | Field rename |
| New `WanRef` schema (`id`, `uri`, `name`) | Addition |
| `GatewayDetail` gains required `ports: [DevicePort]` and `wans: [WanRef]` | Addition |
| `/network/ports` now includes gateway LAN ports; `switchId` filter → `deviceId` | Broadening + rename |

## Approach

Sequential: run `make generate` first to get new Go types, then adapt the service layer to compile and add gateway functionality. The compiler identifies every broken site from the rename cascade.

## Phase 1 — `make generate`

Regenerates `internal/network/api.gen.go`. Produces:

- `SwitchPort*` → `DevicePort*` for all types (`DevicePort`, `DevicePortPoeMode`, `DevicePortVlanConfig`, `DevicePortVlanMode`, `DevicePortLagMembership`)
- `DevicePort.PoeMode *DevicePortPoeMode` — pointer (optional field)
- `NetworkPort.Device NetworkDeviceRef` — renamed from `Switch`
- `ListNetworkPortsParams.DeviceId *string` — renamed from `SwitchId`
- `GatewayDetail.Ports []DevicePort` and `GatewayDetail.Wans []WanRef` — new required fields
- New `WanRef` type with `Id`, `Uri`, `Name string`

No hand-edits to generated files.

## Phase 2 — Adapter: add `Ifname` to `UniFiPortEntry`

`internal/adapters/unifi.go`:

Add `Ifname string \`json:"ifname"\`` to `UniFiPortEntry`. This field is already present in real captured fixture data (gateway ports have `ifname: "eth0"` through `"eth6"`); the struct just doesn't decode it yet. No other adapter changes.

## Phase 3 — `poeMode` optionality

`internal/network/devices_service.go` — `buildSwitchPorts` (to be renamed `buildDevicePorts`):

Change the unconditional `PoeMode: mapPoeMode(p.PoeMode)` assignment to:

```go
if p.PortPoe {
    pm := mapPoeMode(p.PoeMode)
    port.PoeMode = &pm
}
```

Applies to all device types. Switch ports with `port_poe: false` that previously returned `"off"` now omit the field — correct per spec ("no PoE hardware"). Switch ports with `port_poe: true` continue to report the configured mode.

## Phase 4 — Gateway ports and WAN refs in `buildGatewayDetail`

### WAN port identification

UniFi device fields `wan1.name` and `wan2.name` hold the `ifname` of the WAN-role physical port (e.g. `"eth4"`, `"eth6"` on UDM). Build a WAN ifname set before calling the port builder; exclude any port whose `Ifname` is in that set.

```go
func wanIfnames(d adapters.UniFiDevice) map[string]bool {
    s := make(map[string]bool)
    if d.Wan1 != nil && d.Wan1.Name != "" {
        s[d.Wan1.Name] = true
    }
    if d.Wan2 != nil && d.Wan2.Name != "" {
        s[d.Wan2.Name] = true
    }
    return s
}
```

### `buildGatewayPorts`

New function that filters port_table to LAN-only ports and delegates to `buildDevicePorts`:

```go
func buildGatewayPorts(
    controller string,
    d adapters.UniFiDevice,
    swPortToDevice map[string]adapters.UniFiDevice,
    swPortToClient map[string]adapters.UniFiSta,
    confs []adapters.UniFiNetworkConf,
) []DevicePort {
    wan := wanIfnames(d)
    var lanDevice adapters.UniFiDevice
    lanDevice = d
    lanDevice.PortTable = nil
    for _, p := range d.PortTable {
        if !wan[p.Ifname] {
            lanDevice.PortTable = append(lanDevice.PortTable, p)
        }
    }
    return buildDevicePorts(controller, lanDevice, swPortToDevice, swPortToClient, confs)
}
```

### `buildGatewayWanRefs`

New function that builds `[]WanRef` from network configs with `purpose=="wan"`, using the same ID derivation as `buildWan`:

```go
func buildGatewayWanRefs(controller string, confs []adapters.UniFiNetworkConf) []WanRef {
    var refs []WanRef
    for _, n := range confs {
        if n.Purpose != "wan" {
            continue
        }
        id := fmt.Sprintf("%s.%s", controller, toKebab(n.Name))
        refs = append(refs, WanRef{
            Id:   id,
            Uri:  fmt.Sprintf("/network/wans/%s", id),
            Name: n.Name,
        })
    }
    if refs == nil {
        refs = []WanRef{}
    }
    return refs
}
```

### `buildGatewayDetail` signature change

Currently: `buildGatewayDetail(controller string, d adapters.UniFiDevice)`.

New: also accepts `swPortToDevice`, `swPortToClient`, `confs` — the same data already computed in `buildDeviceDetail` and passed to `buildSwitchDetail`. No new backend calls.

```go
GatewayDetail{
    ...base fields...
    Ports: buildGatewayPorts(controller, d, swPortToDevice, swPortToClient, confs),
    Wans:  buildGatewayWanRefs(controller, confs),
}
```

`buildDeviceDetail` already has all required data; it just needs to pass it to `buildGatewayDetail`.

## Phase 5 — Broaden `ListPorts` for gateway ports

`internal/network/ports_service.go`:

Replace the `d.Type != "usw"` guard with a helper:

```go
func isSwitchOrGateway(deviceType string) bool {
    switch deviceType {
    case "usw", "ugw", "udm", "udm-pro":
        return true
    }
    return false
}
```

Loop body: `if !isSwitchOrGateway(d.Type) { continue }`.

For gateway devices, pass the filtered-LAN port slice to `buildDevicePorts` (reuse `buildGatewayPorts` logic). The `device` ref in `toNetworkPort` points at the parent device regardless of whether it is a switch or gateway.

`matchesFilter`:
- `params.SwitchId` → `params.DeviceId`
- `port.Switch.Id` → `port.Device.Id`

`toNetworkPort`: `Switch: sw` → `Device: sw`.

## Phase 6 — Mechanical renames in service code and tests

| Before | After |
|---|---|
| `buildSwitchPorts` | `buildDevicePorts` |
| return type `[]SwitchPort` | `[]DevicePort` |
| `SwitchPortVlanConfig` | `DevicePortVlanConfig` |
| `SwitchPortVlanMode` | `DevicePortVlanMode` |
| `SwitchPortLagMembership` | `DevicePortLagMembership` |
| `SwitchPortPoeMode` | `DevicePortPoeMode` |
| `buildVlanConfig` return type | `*DevicePortVlanConfig` |
| `buildLagMembership` return type | `*DevicePortLagMembership` |
| `mapPoeMode` return type | `DevicePortPoeMode` |
| `matchesVlanID(cfg *SwitchPortVlanConfig, ...)` | `*DevicePortVlanConfig` |
| `modePtr(m SwitchPortVlanMode)` in tests | `DevicePortVlanMode` |
| `p.Switch.Id` in test assertions | `p.Device.Id` |
| `ListNetworkPortsParams{SwitchId: ...}` in tests | `DeviceId` |

## Testing

### Updates to existing tests

- `TestListNetworkPorts_NoFilter`: port count 35 → **40** (35 switch + 5 gateway LAN ports; UDM has 7 ports, 2 are WAN via `wan1.name="eth4"`, `wan2.name="eth6"`)
- `TestListNetworkPorts_SwitchRef` / `_SwitchIdFilter`: rename `Switch` → `Device` in assertions and params
- `TestListNetworkPorts_ModeFilter_NilVlanConfigExcluded`: comment update only (gateway ports now included but filtered correctly by mode)

### New tests

- `TestListNetworkPorts_GatewayPorts` — verify gateway LAN ports appear in unfiltered listing; `device.id == "unifi.cgf-01"`; WAN ports (`eth4`, `eth6`) absent
- `TestListNetworkPorts_DeviceIdFilter_Gateway` — filter by `deviceId="unifi.cgf-01"`, expect exactly 5 ports
- `TestGetDevice_Gateway_PortsAndWans` — verify `GatewayDetail.Ports` non-empty, `GatewayDetail.Wans` matches networkconf WAN entry count
- `TestBuildGatewayWanRefs` — unit: WanRef IDs follow `controller.kebab-name` convention, Name matches networkconf Name

## Not in scope

- New endpoints (no `/network/ports/{portId}`)
- WAN-role physical port modelling in `DevicePort`
- Upstream ISP / modem modelling
- Any change to `/network/wans` or `/network/wans/{id}` endpoints
