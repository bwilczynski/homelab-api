# Network Topology Endpoint Design

**Date:** 2026-05-17
**Spec:** `GET /network/topology` (`getNetworkTopology`)

## Overview

Implement the `GET /network/topology` endpoint defined in the spec submodule. Returns the network as a graph of nodes (devices and optionally clients) and edges (wired uplinks and wireless associations).

## Spec contract

- **Response:** `NetworkTopology` — plain object with `nodes: []TopologyNode` and `edges: []TopologyEdge`.
- **Query param:** `includeClients` (boolean, default false) — when true, adds all clients (online + offline) as nodes with their edges.
- **Node types:** `TopologyDeviceNode` (kind=device) and `TopologyClientNode` (kind=client), discriminated by `kind`.
- **Edge types:** `TopologyWiredEdge` (kind=wired) and `TopologyWirelessEdge` (kind=wireless), discriminated by `kind`.

## Architecture

Follows the existing domain pattern. No new adapter methods required.

### Files touched

| File | Change |
|------|--------|
| `internal/network/topology_service.go` | New file — `GetTopology` method + `TopologyBackend` interface |
| `internal/network/handler.go` | Add `GetNetworkTopology` handler method |
| `internal/network/service_test.go` | Extend with topology tests |

## Data-fetching strategy (Option A)

Always: `GetDevices()`.

When `includeClients=true`:
- `GetClients()` (V1 stas) — online clients; provides `SwMAC`, `SwPort`, `ApMAC`, `Signal`.
- `GetOfflineClients(historyDays)` (V2) — offline clients; provides `LastUplinkMAC`, `ESSID`.

V1 stas are indexed by normalized MAC for edge-building. V2 history clients are those not already in the V1 set (offline only).

## Assembly — three passes

**Pass 1 — Device nodes + device-device edges** (always):
- Each `UniFiDevice` → `TopologyDeviceNode` (type, status, numClients for APs only).
- Each device with non-nil `Uplink` → `TopologyWiredEdge` (source=device, target=upstream via `UplinkMAC`→`macToDevice`, port=`UplinkRemotePort`, linkSpeed from `Uplink.Speed`).
- Gateway has no `Uplink` → no outgoing edge; identified by `type: gateway`.

**Pass 2 — Client nodes** (includeClients=true):
- V1 sta → `TopologyClientNode` (connectionType from `IsWired`, status=online).
- V2 history → `TopologyClientNode` (connectionType from `IsWired`, status=offline).

**Pass 3 — Client-device edges** (includeClients=true):
- Online wired (`UniFiSta.IsWired=true`): wired edge, source=client, target=switch via `SwMAC`→`macToDevice`, port=`SwPort`, linkSpeed from `WiredRateMbps`.
- Online wireless: wireless edge, source=client, target=AP via `ApMAC`→`macToDevice`, ssid=`ESSID`, signalStrength=`Signal`.
- Offline (V2, wired): wired edge, target via `LastUplinkMAC`→`macToDevice`, port omitted.
- Offline (V2, wireless): wireless edge, target via `LastUplinkMAC`→`macToDevice`, ssid=`ESSID`, signalStrength omitted.

## Response serialization

`NetworkTopology` is a plain struct (not a union type). The generated `GetNetworkTopology200JSONResponse` wraps it as a new type definition, but since `NetworkTopology` itself has no `MarshalJSON`, Go uses struct reflection. The `Nodes []TopologyNode` and `Edges []TopologyEdge` slice fields encode correctly — each element's own `MarshalJSON` fires via standard `json.Marshal`. **No custom response wrapper needed.** Verify after `make generate`.

## Error handling

- Backend errors → 500 problem+json, same pattern as `ListDevices`.
- No 404: topology always returns a result (empty nodes/edges if all backends unreachable).

## Testing

Extend `service_test.go` using existing fixtures (`testdata/unifi-devices.json`, `unifi-clients.json`, `unifi-v2-history.json`).

Test cases:
1. `TestGetTopology_DevicesOnly` — `includeClients=false`: correct device node count, gateway node present, device-device wired edges with correct port and linkSpeed, no client nodes.
2. `TestGetTopology_WithClients` — `includeClients=true`: correct total node count, online wired client edge has port, offline client edge omits port, wireless edge has ssid + signalStrength for online, ssid only for offline.
