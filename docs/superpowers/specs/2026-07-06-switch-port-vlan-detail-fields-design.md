# Switch Port VLAN Policy & Detail Fields — Go Implementation Design

**Date:** 2026-07-06
**Spec version:** 1.3.0 (already merged; stubs regenerated)
**Scope:** Go implementation only — spec and code generation are complete.

---

## Goal

Implement the 5 new optional `SwitchPort` fields added in spec 1.3.0:

| API field | Source |
|---|---|
| `label` | UniFi `name` when it differs from the default `"Port <idx>"` |
| `sfpModulePresent` | UniFi `sfp_found` (present only on SFP-cage ports) |
| `linkUptime` | UniFi per-port `uptime` (present only on up ports) |
| `lagMembership` | Derived from UniFi `aggregated_by` (false or integer) |
| `vlanConfig` | Derived from UniFi `forward`, `native_networkconf_id`, `tagged_vlan_mgmt`, `excluded_networkconf_ids` |

All fields are optional. Existing behavior is unchanged.

---

## Adapter changes — `internal/adapters/unifi.go`

Extend `UniFiPortEntry` with 8 new fields:

```go
Name                   string   `json:"name"`
Forward                string   `json:"forward"`
NativeNetworkConfID    *string  `json:"native_networkconf_id"`
TaggedVlanMgmt         string   `json:"tagged_vlan_mgmt"`
ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids"`
SfpFound               *bool    `json:"sfp_found"`
Uptime                 *int     `json:"uptime"`
AggregatedBy           any      `json:"aggregated_by"`
```

**Type rationale:**
- `NativeNetworkConfID *string` — absent or null in JSON for some trunk-all ports; nil means "no explicit native VLAN."
- `SfpFound *bool` — UniFi only emits this field for ports with an SFP cage. Absent means no cage.
- `Uptime *int` — only present on ports where `up: true`. Absent means the link is down or the controller doesn't report per-port uptime for this model.
- `AggregatedBy any` — UniFi emits `false` (bool) for non-member ports and an integer `port_idx` for member ports.

All fields come from real captured responses (`scripts/responses/unifi-devices-os-raw.json`, `unifi-networkconf-os-raw.json`).

---

## Service layer — `internal/network/devices_service.go`

### Interface change

Add `GetNetworkConf` to `DevicesBackend`:

```go
type DevicesBackend interface {
    GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
    GetClients(ctx context.Context) ([]adapters.UniFiSta, error)
    GetNetworkConf(ctx context.Context) ([]adapters.UniFiNetworkConf, error)
}
```

This is already implemented by `*adapters.UniFiClient`; it's also on `VLANsBackend`. The two interfaces remain separate.

### `GetDevice` flow change

Always call `GetNetworkConf` alongside `GetDevices` and `GetClients`:

```go
confs, err := backend.GetNetworkConf(ctx)
```

The cost is one extra RPC per `GetDevice` call regardless of device type. Accepted for simplicity.

### `buildSwitchPorts` signature change

Add a `confByID map[string]adapters.UniFiNetworkConf` parameter (keyed by `UniFiNetworkConf.ID`), plus the pre-computed `defaultNetID string` (the network conf entry with `purpose: "corporate"` and `VlanEnabled: false`).

### New helper functions

All are package-private and unit-testable.

**`buildPortLabel(p UniFiPortEntry) *string`**
Returns nil if `p.Name == "Port "+strconv.Itoa(p.PortIdx)`, otherwise returns `&p.Name`.

**`buildSfpModulePresent(p UniFiPortEntry) *bool`**
Returns `p.SfpFound` directly (already `*bool`).

**`buildLinkUptime(p UniFiPortEntry) *Seconds`**
Returns nil if `!p.Up`. Otherwise casts `*p.Uptime` to `*Seconds` (nil if `p.Uptime == nil`).

**`buildLagMembership(p UniFiPortEntry, masterSet map[int]bool) *SwitchPortLagMembership`**

Two-pass approach (the caller pre-computes `masterSet`):
- Pass 1 (before this function): for each port, if `AggregatedBy` is a `float64`, add `int(v)` to `masterSet`.
- Pass 2 (this function): type-switch on `p.AggregatedBy`:
  - `float64` → member, `id = int(v)`
  - `bool(false)` and `p.PortIdx` is in `masterSet` → master, `id = p.PortIdx`
  - otherwise → nil (not in LAG)

**`findDefaultNetID(confs []adapters.UniFiNetworkConf) string`**
Returns the `_id` of the first `purpose: "corporate"` + `!VlanEnabled` entry. Returns empty string if none found (logs a warning; `vlanConfig` will be omitted on ports that need it).

**`buildVlanConfig(p UniFiPortEntry, confByID map[string]adapters.UniFiNetworkConf, defaultNetID string) *SwitchPortVlanConfig`**

VLAN mode mapping table (from real captures):

| UniFi fields | API `vlanConfig` |
|---|---|
| `forward: "native"` + `tagged_vlan_mgmt: "block_all"` | `mode: access`, `nativeVlan: resolve(native_networkconf_id)` |
| `forward: "all"` | `mode: trunk`, `taggedVlans: {scope: all}`, `nativeVlan: resolve(native_networkconf_id ?? defaultNetID)` |
| `forward: "customize"` + `excluded_networkconf_ids: []` | `mode: trunk`, `taggedVlans: {scope: all}`, same native resolution |
| `forward: "customize"` + `excluded_networkconf_ids: [...]` | `mode: trunk`, `taggedVlans: {scope: custom, items: all corporate VLANs minus excluded, minus native VLAN}` |
| `forward: "disable"` | omit `vlanConfig` entirely |

Notes:
- "corporate VLANs" = all `confByID` entries with `purpose: "corporate"`.
- If `native_networkconf_id` resolves to nil (not in `confByID`), omit the entire `vlanConfig` sub-object (defensive).
- For trunk-custom `items`, include all corporate VLANs that are (a) not in `excluded_networkconf_ids` and (b) not the native VLAN itself. The native VLAN is untagged on the port, so it must not appear in the tagged items list. Sort items by VLAN ID for deterministic output.

**`confToVlanRef(conf adapters.UniFiNetworkConf, controller string) NetworkVlanRef`**
Converts a `UniFiNetworkConf` to a `NetworkVlanRef` using the same composite-ID convention as the VLANs service: `id = "{controller}.{kebab(name)}"`, `uri = "/network/vlans/{id}"`, `vlanId = extractVlanID(conf)`.

---

## Fixture changes — `internal/network/testdata/unifi-devices.json`

The existing US 8 60W entry already has the right port structure for most VLAN modes:
- Port 1: `forward: "all"`, no native → trunk-all (fallback to default net)
- Ports 2–4: `forward: "native"` + `tagged_vlan_mgmt: "block_all"` → access
- Port 5: `forward: "all"` + PoE → trunk-all
- Port 6: `forward: "customize"` + `excluded_networkconf_ids: [LAN-IOT, LAN-SRV]` → trunk-custom
- Port 7: `forward: "all"` + `native_networkconf_id: LAN-MGMT` → trunk-all with explicit native
- Port 8: `forward: "all"` + PoE → trunk-all

**Synthetic additions (3 ports appended to the US 8 60W `port_table`):**

| Port | New fields | Covers |
|---|---|---|
| Port 1 (update) | add `"uptime": 3600` | `linkUptime` on an up port |
| Port 9 (new) | `media: "SFP+"`, `sfp_found: true`, `up: false`, `forward: "all"`, `aggregated_by: false` | `sfpModulePresent: true` |
| Port 10 (new) | `media: "SFP+"`, `sfp_found: false`, `up: false`, `forward: "all"`, `aggregated_by: false` | `sfpModulePresent: false` |
| Port 11 (new) | `forward: "all"`, `aggregated_by: false`, `up: true`, `name: "LAG Master"` | LAG master + custom label |
| Port 12 (new) | `forward: "all"`, `aggregated_by: 11`, `up: true` | LAG member (points to port 11) |

The `uptime` field shape is taken from real USW Flex 2.5G 8 captures (`scripts/responses/unifi-devices-os-raw.json`, ports 8 and 9). SFP field shape from port 10 of the same device. LAG field shape (`aggregated_by: <int>`) is documented by the UniFi controller API; no real LAG is configured in this homelab.

Test expectations for `len(sw.Ports)` in `TestGetDevice_Switch` will change from 8 to 12.

---

## Test changes — `internal/network/service_test.go`

Extend `mockUniFi` to accept and return network confs (it currently returns nil/empty for `GetNetworkConf`). Load the existing `unifi-networkconf.json` fixture in switch tests.

Add the following focused test cases (or extend `TestGetDevice_Switch`):

| Test | Assertion |
|---|---|
| `TestGetDevice_Switch_VlanAccess` | Port 2: `vlanConfig.mode == "access"`, `nativeVlan.id == "unifi.lan-int"` |
| `TestGetDevice_Switch_VlanTrunkAll` | Port 1: `vlanConfig.mode == "trunk"`, `taggedVlans.scope == "all"`, `nativeVlan.id == "unifi.lan-mgmt"` (fallback to default) |
| `TestGetDevice_Switch_VlanTrunkAllExplicitNative` | Port 7: `vlanConfig.mode == "trunk"`, `nativeVlan.id == "unifi.lan-mgmt"` (explicit, not fallback) |
| `TestGetDevice_Switch_VlanTrunkCustom` | Port 6: `vlanConfig.mode == "trunk"`, `taggedVlans.scope == "custom"`, items contain `"unifi.lan-int"` but not `"unifi.lan-iot"`, `"unifi.lan-srv"`, or `"unifi.lan-mgmt"` (native is excluded from tagged items) |
| `TestGetDevice_Switch_Label` | Port 11: `label == "LAG Master"` |
| `TestGetDevice_Switch_NoLabelDefault` | Port 1: `label == nil` (name is "Port 1") |
| `TestGetDevice_Switch_SfpPresent` | Port 9: `sfpModulePresent == &true` |
| `TestGetDevice_Switch_SfpAbsent` | Port 10: `sfpModulePresent == &false` |
| `TestGetDevice_Switch_SfpNoCage` | Port 1: `sfpModulePresent == nil` |
| `TestGetDevice_Switch_LinkUptime` | Port 1: `linkUptime == &3600` |
| `TestGetDevice_Switch_LagMaster` | Port 11: `lagMembership.role == "master"`, `id == 11` |
| `TestGetDevice_Switch_LagMember` | Port 12: `lagMembership.role == "member"`, `id == 11` |
| `TestGetDevice_Switch_NoLag` | Port 1: `lagMembership == nil` |

---

## What's not changing

- `ListDevices` — no port-level detail surfaced; no changes needed.
- `internal/adapters/unifi.go` `GetNetworkConf` — already implemented and tested.
- Handler layer — no changes; the `StrictServerInterface` implementation delegates directly to the service.
- The spec and generated stubs — already at 1.3.0.
