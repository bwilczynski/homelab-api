# Design: GET /network/ports endpoint (spec v1.4.0)

**Date:** 2026-07-09
**Spec change:** homelab-api-spec v1.4.0, PR #19 — adds `listNetworkPorts` operation

## Overview

Implement `GET /network/ports` — a flat listing of physical switch ports across all managed switches on all configured UniFi controllers. Each port carries a reference to its parent switch. Four server-side filters (AND-composed): `switchId`, `state`, `mode`, `vlanId`.

## Architecture

One new file: `internal/network/ports_service.go`. One new handler method in `handler.go`. No new adapter code — the existing `GetDevices`, `GetClients`, and `GetNetworkConf` adapter calls provide all required data.

```
ports_service.go
  PortsBackend interface  (GetDevices, GetClients, GetNetworkConf)
  Service.ListPorts(ctx, params) (NetworkPortList, error)
  matchesFilter(port, switchID, params) bool

handler.go
  ServerHandler.ListNetworkPorts(ctx, request) (ListNetworkPortsResponseObject, error)
```

`PortsBackend` is added to the `UniFiBackend` composite in `service.go` (alongside `DevicesBackend`, `ClientsBackend`, etc.).

## Data Flow

For each available backend (skipped if health monitor marks it unreachable):

1. `GetDevices` — filter to `type == "usw"` switches only
2. `GetClients` + `GetNetworkConf` — fetched once per backend, shared across all switches
3. Build index maps (`swPortToDevice`, `swPortToClient`) once per backend using existing helpers
4. For each switch: call existing `buildSwitchPorts(controller, device, ...)` → `[]SwitchPort`
5. Wrap each `SwitchPort` into `NetworkPort` by attaching `switch: NetworkDeviceRef{kind, id, uri, name}`
6. Apply `matchesFilter` — drop non-matching ports
7. Accumulate across all backends; return `NetworkPortList{Items: items}` (empty slice, never nil)

Backend errors: log warning and continue (best-effort, same pattern as `ListDevices`).

## Filter Logic

All filters are optional and AND-composed. `matchesFilter` returns `true` only when every supplied filter matches.

| Filter | Match rule |
|--------|-----------|
| `switchId` | switch composite ID `{controller}.{kebab-name}` equals param |
| `state` | `port.State` string equals param |
| `mode` | `port.VlanConfig != nil && port.VlanConfig.Mode == mode`; no vlanConfig → never matches |
| `vlanId` | see below; no vlanConfig → never matches |

**`vlanId` three-way match** (spec-compliant):
1. `nativeVlan.vlanId == vlanId`
2. tagged scope is `custom` and `vlanId` appears in `taggedVlans.items[*].vlanId`
3. tagged scope is `all` (trunk-all carries every VLAN — always matches)

## Handler

`ListNetworkPorts` in `handler.go` delegates to `svc.ListPorts`. The response schema is a plain `NetworkPortList` (no anyOf/discriminator), so the generated `ListNetworkPorts200JSONResponse` works directly — no hand-written response wrapper needed.

## Testing

Table-driven tests in `service_test.go` (or a new `ports_service_test.go`), using existing `testdata/devices.json` fixture. No new fixture capture required — adapter layer is unchanged.

Test cases:
- No filters → all ports from all switches, each with correct `switch` ref
- `switchId` filter → only ports from that switch
- `state` filter (`up`, `down`)
- `mode` filter (`trunk`, `access`); port without `vlanConfig` excluded
- `vlanId` filter — three sub-cases: native VLAN match, tagged custom match, trunk-all match
- Port without `vlanConfig` → excluded from `mode` and `vlanId` filter results
- Unavailable backend skipped (mock monitor returning false)
