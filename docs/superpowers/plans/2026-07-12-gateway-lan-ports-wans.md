# Gateway LAN Ports & WAN References Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapt the Go service layer to spec v1.5.0: rename `SwitchPort*` → `DevicePort*`, make `poeMode` optional, add LAN `ports` and `wans` arrays to `GatewayDetail`, and broaden `/network/ports` to include gateway LAN ports alongside switch ports.

**Architecture:** Code generation has already been run (`make generate`) on branch `worktree-feat+gateway-lan-ports-wans`. The generated `api.gen.go` has all new types; the service layer currently fails to compile because it still references the old `SwitchPort*` names. Tasks 1–4 restore compilation, Tasks 5–8 add gateway functionality with tests.

**Tech Stack:** Go 1.23+, chi router, oapi-codegen strict server. Test fixtures in `internal/network/testdata/`. Run tests with `go test ./internal/network/...`.

---

## Fixture facts (needed for test assertions)

- Gateway device: `CGF-01` (type `udm`), composite ID `unifi.cgf-01`
- Gateway has 7 ports in `port_table`; `wan1.name="eth4"` (port 5), `wan2.name="eth6"` (port 7) → **5 LAN ports** (idx 1,2,3,4,6), **2 WAN ports** (idx 5,7)
- Port 4 (`eth3`) is a LAN port with `port_poe=true` — it will carry `poeMode`. All other LAN ports have `port_poe=false` → no `poeMode`.
- 4 switches in fixture with 35 total ports → after gateway: **40 total**
- WAN network confs: 2 entries (`Internet 1` → id `unifi.internet-1`, `Internet 2` → id `unifi.internet-2`)
- Switch port 1 of `us-8-60w` has `port_poe=false` → after Task 2, its `PoeMode` becomes `nil` (was `"off"`)
- Switch port 5 of `us-8-60w` has `port_poe=true, poe_mode="auto"` → `PoeMode = &Auto`

---

## File map

| File | Change |
|---|---|
| `internal/adapters/unifi.go` | Add `Ifname` field to `UniFiPortEntry` |
| `internal/network/devices_service.go` | Rename `SwitchPort*` → `DevicePort*`; fix poeMode; add gateway port/wan builders |
| `internal/network/ports_service.go` | Rename `SwitchPort*` → `DevicePort*`; add `isSwitchOrGateway`; broaden loop |
| `internal/network/service_test.go` | Fix poeMode pointer assertions; rename `*SwitchPort` → `*DevicePort` locals |
| `internal/network/ports_service_test.go` | Rename params/assertions; update count; add gateway tests |

---

## Task 1: Add `Ifname` to `UniFiPortEntry`

**Files:**
- Modify: `internal/adapters/unifi.go`

Gateway ports in the UniFi API include an `ifname` field (e.g. `"eth0"`–`"eth6"`). The struct doesn't decode it yet. We need it to identify which ports are WAN-role.

- [ ] **Step 1: Add the field**

Open `internal/adapters/unifi.go`. In `UniFiPortEntry` (the struct starting around line 243), add `Ifname` as the first field:

```go
type UniFiPortEntry struct {
	Ifname                string   `json:"ifname"`
	PortIdx               int      `json:"port_idx"`
	Up                    bool     `json:"up"`
	Speed                 int      `json:"speed"`
	PortPoe               bool     `json:"port_poe"`
	PoeMode               string   `json:"poe_mode"`
	PoePower              string   `json:"poe_power"`
	TxBytes               int64    `json:"tx_bytes"`
	RxBytes               int64    `json:"rx_bytes"`
	TxBytesR              float64  `json:"tx_bytes-r"`
	RxBytesR              float64  `json:"rx_bytes-r"`
	Name                  string   `json:"name"`
	Forward               string   `json:"forward"`
	NativeNetworkConfID   *string  `json:"native_networkconf_id"`
	TaggedVlanMgmt        string   `json:"tagged_vlan_mgmt"`
	ExcludedNetworkConfIDs []string `json:"excluded_networkconf_ids"`
	SfpFound              *bool    `json:"sfp_found"`
	Uptime                *int     `json:"uptime"`
	AggregatedBy          any      `json:"aggregated_by"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/adapters/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/unifi.go
git commit -m "feat: add Ifname to UniFiPortEntry for WAN port identification"
```

---

## Task 2: Fix `devices_service.go` — renames + poeMode pointer

**Files:**
- Modify: `internal/network/devices_service.go`

This task fixes all undefined-symbol compile errors in `devices_service.go` by renaming every `SwitchPort*` reference to its `DevicePort*` equivalent, and changes `poeMode` to be set only when the port has PoE hardware (`p.PortPoe == true`).

- [ ] **Step 1: Rename `buildSwitchPorts` to `buildDevicePorts` and fix its return type**

Find the function signature (around line 191):
```go
func buildSwitchPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []SwitchPort {
```

Replace with:
```go
func buildDevicePorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []DevicePort {
```

- [ ] **Step 2: Fix the slice declaration and struct literal inside `buildDevicePorts`**

Inside the function body, replace:
```go
ports := make([]SwitchPort, 0, len(d.PortTable))
for _, p := range d.PortTable {
	port := SwitchPort{
		Number:           p.PortIdx,
		State:            mapPortState(p.Up),
		PoeMode:          mapPoeMode(p.PoeMode),
		Label:            buildPortLabel(p),
		SfpModulePresent: buildSfpModulePresent(p),
		LinkUptime:       buildLinkUptime(p),
		LagMembership:    buildLagMembership(p, masterSet),
		VlanConfig:       buildVlanConfig(p, confByID, defaultNetID, controller),
		Traffic: NetworkTraffic{
			RxBytesTotal:  p.RxBytes,
			TxBytesTotal:  p.TxBytes,
			RxBytesPerSec: int64(p.RxBytesR),
			TxBytesPerSec: int64(p.TxBytesR),
		},
	}
```

with:
```go
ports := make([]DevicePort, 0, len(d.PortTable))
for _, p := range d.PortTable {
	port := DevicePort{
		Number:           p.PortIdx,
		State:            mapPortState(p.Up),
		Label:            buildPortLabel(p),
		SfpModulePresent: buildSfpModulePresent(p),
		LinkUptime:       buildLinkUptime(p),
		LagMembership:    buildLagMembership(p, masterSet),
		VlanConfig:       buildVlanConfig(p, confByID, defaultNetID, controller),
		Traffic: NetworkTraffic{
			RxBytesTotal:  p.RxBytes,
			TxBytesTotal:  p.TxBytes,
			RxBytesPerSec: int64(p.RxBytesR),
			TxBytesPerSec: int64(p.TxBytesR),
		},
	}
	if p.PortPoe {
		pm := mapPoeMode(p.PoeMode)
		port.PoeMode = &pm
	}
```

- [ ] **Step 3: Fix `buildVlanConfig` return type and internals**

Change the function signature from:
```go
func buildVlanConfig(
	p adapters.UniFiPortEntry,
	confByID map[string]adapters.UniFiNetworkConf,
	defaultNetID string,
	controller string,
) *SwitchPortVlanConfig {
```
to:
```go
func buildVlanConfig(
	p adapters.UniFiPortEntry,
	confByID map[string]adapters.UniFiNetworkConf,
	defaultNetID string,
	controller string,
) *DevicePortVlanConfig {
```

Inside the function, replace every `SwitchPortVlanConfig` with `DevicePortVlanConfig`, every `SwitchPortVlanConfigTaggedVlansScope` with `DevicePortVlanConfigTaggedVlansScope`, and every `SwitchPortVlanMode` (the type used in `Mode:`) with `DevicePortVlanMode`. The full updated function body:

```go
func buildVlanConfig(
	p adapters.UniFiPortEntry,
	confByID map[string]adapters.UniFiNetworkConf,
	defaultNetID string,
	controller string,
) *DevicePortVlanConfig {
	switch p.Forward {
	case "disable":
		return nil
	case "native":
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		return &DevicePortVlanConfig{
			Mode:       Access,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
	case "all", "customize":
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		cfg := &DevicePortVlanConfig{
			Mode:       Trunk,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
		if p.Forward == "customize" && len(p.ExcludedNetworkConfIDs) > 0 {
			excludedSet := make(map[string]bool, len(p.ExcludedNetworkConfIDs))
			for _, id := range p.ExcludedNetworkConfIDs {
				excludedSet[id] = true
			}
			var items []NetworkVlanRef
			for _, conf := range confByID {
				if conf.Purpose != "corporate" {
					continue
				}
				if excludedSet[conf.ID] || conf.ID == nativeConf.ID {
					continue
				}
				items = append(items, confToVlanRef(conf, controller))
			}
			slices.SortFunc(items, func(a, b NetworkVlanRef) int {
				return a.VlanId - b.VlanId
			})
			cfg.TaggedVlans = &struct {
				Items *[]NetworkVlanRef                          `json:"items,omitempty"`
				Scope DevicePortVlanConfigTaggedVlansScope `json:"scope"`
			}{Scope: Custom, Items: &items}
		} else {
			cfg.TaggedVlans = &struct {
				Items *[]NetworkVlanRef                          `json:"items,omitempty"`
				Scope DevicePortVlanConfigTaggedVlansScope `json:"scope"`
			}{Scope: All}
		}
		return cfg
	default:
		return nil
	}
}
```

- [ ] **Step 4: Fix `buildLagMembership` return type**

Change the function signature from:
```go
func buildLagMembership(p adapters.UniFiPortEntry, masterSet map[int]bool) *SwitchPortLagMembership {
	switch v := p.AggregatedBy.(type) {
	case float64:
		masterIdx := int(v)
		return &SwitchPortLagMembership{Id: masterIdx, Role: Member}
	case bool:
		if !v && masterSet[p.PortIdx] {
			return &SwitchPortLagMembership{Id: p.PortIdx, Role: Master}
		}
	}
	return nil
}
```
to:
```go
func buildLagMembership(p adapters.UniFiPortEntry, masterSet map[int]bool) *DevicePortLagMembership {
	switch v := p.AggregatedBy.(type) {
	case float64:
		masterIdx := int(v)
		return &DevicePortLagMembership{Id: masterIdx, Role: Member}
	case bool:
		if !v && masterSet[p.PortIdx] {
			return &DevicePortLagMembership{Id: p.PortIdx, Role: Master}
		}
	}
	return nil
}
```

- [ ] **Step 5: Fix `mapPoeMode` return type**

Change from:
```go
func mapPoeMode(mode string) SwitchPortPoeMode {
```
to:
```go
func mapPoeMode(mode string) DevicePortPoeMode {
```

The constants (`Auto`, `Off`, `Passive24v`, `Passthrough`) are unchanged — they are values of the renamed type and keep the same names.

- [ ] **Step 6: Update `buildSwitchDetail` to call `buildDevicePorts`**

Find the call to `buildSwitchPorts` inside `buildSwitchDetail` (around line 170):
```go
ports := buildSwitchPorts(controller, d, swPortToDevice, swPortToClient, confs)
```
Replace with:
```go
ports := buildDevicePorts(controller, d, swPortToDevice, swPortToClient, confs)
```

Also update the `SwitchDetail` struct literal — the `Ports` field type is now `[]DevicePort` in the generated code; the field name `Ports` is unchanged, so this just compiles once the return type matches.

- [ ] **Step 7: Verify `devices_service.go` compiles**

```bash
go build ./internal/network/
```

Expected: only errors from `ports_service.go` (not `devices_service.go`). If `devices_service.go` errors remain, fix them before proceeding.

- [ ] **Step 8: Commit**

```bash
git add internal/network/devices_service.go
git commit -m "refactor: rename SwitchPort* to DevicePort* and make poeMode optional in devices_service"
```

---

## Task 3: Fix `ports_service.go` — renames

**Files:**
- Modify: `internal/network/ports_service.go`

- [ ] **Step 1: Fix `toNetworkPort` signature and `Device` field**

Replace the entire `toNetworkPort` function:
```go
func toNetworkPort(port SwitchPort, sw NetworkDeviceRef) NetworkPort {
	return NetworkPort{
		Number:           port.Number,
		Label:            port.Label,
		State:            port.State,
		LinkSpeed:        port.LinkSpeed,
		LinkUptime:       port.LinkUptime,
		SfpModulePresent: port.SfpModulePresent,
		PoeMode:          port.PoeMode,
		PoePowerWatts:    port.PoePowerWatts,
		LagMembership:    port.LagMembership,
		VlanConfig:       port.VlanConfig,
		Traffic:          port.Traffic,
		ConnectedTo:      port.ConnectedTo,
		Switch:           sw,
	}
}
```
with:
```go
func toNetworkPort(port DevicePort, sw NetworkDeviceRef) NetworkPort {
	return NetworkPort{
		Number:           port.Number,
		Label:            port.Label,
		State:            port.State,
		LinkSpeed:        port.LinkSpeed,
		LinkUptime:       port.LinkUptime,
		SfpModulePresent: port.SfpModulePresent,
		PoeMode:          port.PoeMode,
		PoePowerWatts:    port.PoePowerWatts,
		LagMembership:    port.LagMembership,
		VlanConfig:       port.VlanConfig,
		Traffic:          port.Traffic,
		ConnectedTo:      port.ConnectedTo,
		Device:           sw,
	}
}
```

- [ ] **Step 2: Fix `matchesFilter` — `DeviceId` and `Device` field**

Replace the entire `matchesFilter` function:
```go
func matchesFilter(port NetworkPort, params ListNetworkPortsParams) bool {
	if params.SwitchId != nil && port.Switch.Id != *params.SwitchId {
		return false
	}
	if params.State != nil && port.State != *params.State {
		return false
	}
	if params.Mode != nil {
		if port.VlanConfig == nil || port.VlanConfig.Mode != *params.Mode {
			return false
		}
	}
	if params.VlanId != nil {
		if !matchesVlanID(port.VlanConfig, *params.VlanId) {
			return false
		}
	}
	return true
}
```
with:
```go
func matchesFilter(port NetworkPort, params ListNetworkPortsParams) bool {
	if params.DeviceId != nil && port.Device.Id != *params.DeviceId {
		return false
	}
	if params.State != nil && port.State != *params.State {
		return false
	}
	if params.Mode != nil {
		if port.VlanConfig == nil || port.VlanConfig.Mode != *params.Mode {
			return false
		}
	}
	if params.VlanId != nil {
		if !matchesVlanID(port.VlanConfig, *params.VlanId) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: Fix `matchesVlanID` parameter type**

Replace:
```go
func matchesVlanID(cfg *SwitchPortVlanConfig, vlanID int) bool {
```
with:
```go
func matchesVlanID(cfg *DevicePortVlanConfig, vlanID int) bool {
```

The body is unchanged (field names `NativeVlan`, `TaggedVlans`, `Scope`, `Items` are the same on `DevicePortVlanConfig`).

- [ ] **Step 4: Verify full package compiles**

```bash
go build ./internal/network/
```

Expected: no errors.

- [ ] **Step 5: Verify production code builds**

```bash
go build ./internal/network/
```

Expected: no errors. (The test files still reference `*SwitchPort` and `SwitchPortVlanMode` — those are fixed in Task 4. Running `go test` at this point would fail to compile; that is expected.)

- [ ] **Step 6: Commit**

```bash
git add internal/network/ports_service.go
git commit -m "refactor: rename SwitchPort*/SwitchId to DevicePort*/DeviceId in ports_service"
```

---

## Task 4: Fix tests — poeMode pointer + `SwitchPort` → `DevicePort` renames

**Files:**
- Modify: `internal/network/service_test.go`
- Modify: `internal/network/ports_service_test.go`

- [ ] **Step 1: Fix `service_test.go` — poeMode assertions**

In `TestGetDevice_Switch` (around line 664):

Replace:
```go
if p1.PoeMode != "off" {
    t.Errorf("expected poe mode off, got %s", p1.PoeMode)
}
```
with:
```go
if p1.PoeMode != nil {
    t.Errorf("expected nil poeMode for non-PoE port, got %v", p1.PoeMode)
}
```

Replace:
```go
if p5.PoeMode != "auto" {
    t.Errorf("expected poe auto, got %s", p5.PoeMode)
}
```
with:
```go
if p5.PoeMode == nil || *p5.PoeMode != Auto {
    t.Errorf("expected poeMode=auto, got %v", p5.PoeMode)
}
```

- [ ] **Step 2: Fix `service_test.go` — `*SwitchPort` local variable types**

Three tests declare locals of type `*SwitchPort`. Replace each with `*DevicePort`:

In `TestGetDevice_SwitchPort_ConnectedToDevice` (around line 730):
```go
var port5 *DevicePort
```

In `TestGetDevice_SwitchPort_ConnectedToClient` (around line 772):
```go
var port3 *DevicePort
```

In `TestGetDevice_SwitchPort_UplinkConnectedToDevice` (around line 814):
```go
var port1 *DevicePort
```

- [ ] **Step 3: Fix `ports_service_test.go` — `modePtr` helper**

Replace:
```go
func modePtr(m SwitchPortVlanMode) *SwitchPortVlanMode { return &m }
```
with:
```go
func modePtr(m DevicePortVlanMode) *DevicePortVlanMode { return &m }
```

- [ ] **Step 4: Fix `ports_service_test.go` — rename `SwitchId` → `DeviceId` in all params**

In every `ListNetworkPortsParams{...}` that uses `SwitchId:`, change the field name to `DeviceId:`. There are five occurrences:

```go
// TestListNetworkPorts_SwitchIdFilter
params := ListNetworkPortsParams{DeviceId: strPtr("unifi.us-8-60w")}

// TestListNetworkPorts_StateFilter_Up
params := ListNetworkPortsParams{
    DeviceId: strPtr("unifi.us-8-60w"),
    State:    statePtr(NetworkPortStateUp),
}

// TestListNetworkPorts_ModeFilter_Access
params := ListNetworkPortsParams{
    DeviceId: strPtr("unifi.us-8-60w"),
    Mode:     modePtr(Access),
}

// TestListNetworkPorts_ModeFilter_Trunk
params := ListNetworkPortsParams{
    DeviceId: strPtr("unifi.us-8-60w"),
    Mode:     modePtr(Trunk),
}

// TestListNetworkPorts_VlanIdFilter_NativeMatch
params := ListNetworkPortsParams{
    DeviceId: strPtr("unifi.us-8-60w"),
    Mode:     modePtr(Access),
    VlanId:   intPtr(10),
}
```

Also update: `TestListNetworkPorts_VlanIdFilter_TrunkAllMatchesAnyVlan` and `TestListNetworkPorts_VlanIdFilter_TaggedCustomMatch` use `SwitchId` as well — change those too.

- [ ] **Step 5: Fix `ports_service_test.go` — rename `p.Switch` → `p.Device` in assertions**

In `TestListNetworkPorts_NoFilter`:
```go
for _, p := range result.Items {
    if p.Device.Id == "" {
        t.Errorf("port %d: empty device.id", p.Number)
    }
    if p.Device.Kind != NetworkDeviceRefKindDevice {
        t.Errorf("port %d: expected device.kind=device, got %s", p.Number, p.Device.Kind)
    }
}
```

In `TestListNetworkPorts_SwitchRef`, rename every `p.Switch` → `p.Device` and `switch.id` → `device.id` in messages.

In `TestListNetworkPorts_SwitchIdFilter` (now `DeviceIdFilter`), rename `p.Switch.Id` → `p.Device.Id`.

Rename the test function itself:
```go
func TestListNetworkPorts_SwitchRef(t *testing.T) → TestListNetworkPorts_DeviceRef
func TestListNetworkPorts_SwitchIdFilter(t *testing.T) → TestListNetworkPorts_DeviceIdFilter
```

- [ ] **Step 6: Run tests — expect all to pass**

```bash
go test ./internal/network/...
```

Expected: all existing tests pass. (Gateway port count is still 35 because `ListPorts` only loops over switches — that's fine; the new gateway tests in Tasks 7–8 will drive those changes.)

- [ ] **Step 7: Commit**

```bash
git add internal/network/service_test.go internal/network/ports_service_test.go
git commit -m "fix: update tests for DevicePort* renames and poeMode pointer change"
```

---

## Task 5: Add gateway port and WAN ref builders to `devices_service.go`

**Files:**
- Modify: `internal/network/devices_service.go`

- [ ] **Step 1: Add `wanIfnames` helper**

Add this function after `buildGatewayDetail`:

```go
// wanIfnames returns the set of ifnames that carry WAN traffic on this device.
// These ports must be excluded from the LAN port listing.
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

- [ ] **Step 2: Add `buildGatewayPorts` function**

Add after `wanIfnames`:

```go
// buildGatewayPorts returns the LAN switch-fabric ports on a gateway device,
// excluding any ports whose ifname matches a WAN interface (wan1/wan2).
func buildGatewayPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []DevicePort {
	wan := wanIfnames(d)
	var lan adapters.UniFiDevice = d
	lan.PortTable = nil
	for _, p := range d.PortTable {
		if !wan[p.Ifname] {
			lan.PortTable = append(lan.PortTable, p)
		}
	}
	return buildDevicePorts(controller, lan, swPortToDevice, swPortToClient, confs)
}
```

- [ ] **Step 3: Add `buildGatewayWanRefs` function**

Add after `buildGatewayPorts`:

```go
// buildGatewayWanRefs builds lightweight WAN references from network configs.
// The id/uri/name follow the same convention as buildWan in wans_service.go.
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

- [ ] **Step 4: Update `buildGatewayDetail` to accept port/wan data and populate the new fields**

Replace:
```go
func buildGatewayDetail(controller string, d adapters.UniFiDevice) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	var det NetworkDeviceDetail
	err := det.FromGatewayDetail(GatewayDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Gateway,
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
	})
	return det, err
}
```
with:
```go
func buildGatewayDetail(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	var det NetworkDeviceDetail
	err := det.FromGatewayDetail(GatewayDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Gateway,
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
		Ports:           buildGatewayPorts(controller, d, swPortToDevice, swPortToClient, confs),
		Wans:            buildGatewayWanRefs(controller, confs),
	})
	return det, err
}
```

- [ ] **Step 5: Update the call site in `buildDeviceDetail`**

Find the gateway branch in `buildDeviceDetail` (around line 113):
```go
case "ugw", "udm", "udm-pro":
    return buildGatewayDetail(controller, d)
```
Replace with:
```go
case "ugw", "udm", "udm-pro":
    return buildGatewayDetail(controller, d, swPortToDevice, swPortToClient, confs)
```

- [ ] **Step 6: Verify compilation**

```bash
go build ./internal/network/
```

Expected: no errors.

- [ ] **Step 7: Run existing tests**

```bash
go test ./internal/network/...
```

Expected: all pass. `TestGetDevice_Gateway` still passes because it only checks base fields.

- [ ] **Step 8: Commit**

```bash
git add internal/network/devices_service.go
git commit -m "feat: add gateway LAN ports and WAN refs to GatewayDetail"
```

---

## Task 6: Test gateway device detail — ports and wans

**Files:**
- Modify: `internal/network/service_test.go`

- [ ] **Step 1: Write the failing test**

Add `TestGetDevice_Gateway_PortsAndWans` to `service_test.go` (after `TestGetDevice_Gateway`):

```go
func TestGetDevice_Gateway_PortsAndWans(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	confs := testhelpers.LoadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(
		map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients, networkConf: confs}},
		map[string]int{"unifi": 30},
		slog.Default(),
		nil,
	)

	detail, err := svc.GetDevice(context.Background(), "unifi.cgf-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gw, err := detail.AsGatewayDetail()
	if err != nil {
		t.Fatalf("expected gateway detail: %v", err)
	}

	// 7 ports in port_table, 2 are WAN (eth4, eth6) → 5 LAN ports
	if len(gw.Ports) != 5 {
		t.Fatalf("expected 5 LAN ports, got %d", len(gw.Ports))
	}

	// Port 4 (eth3) has port_poe=true → poeMode present
	var port4 *DevicePort
	for i := range gw.Ports {
		if gw.Ports[i].Number == 4 {
			port4 = &gw.Ports[i]
			break
		}
	}
	if port4 == nil {
		t.Fatal("expected port 4 in gateway LAN ports")
	}
	if port4.PoeMode == nil {
		t.Error("expected poeMode set on port 4 (has PoE hardware)")
	}

	// Port 1 (eth0) has port_poe=false → poeMode absent
	var port1 *DevicePort
	for i := range gw.Ports {
		if gw.Ports[i].Number == 1 {
			port1 = &gw.Ports[i]
			break
		}
	}
	if port1 == nil {
		t.Fatal("expected port 1 in gateway LAN ports")
	}
	if port1.PoeMode != nil {
		t.Errorf("expected nil poeMode on port 1 (no PoE hardware), got %v", port1.PoeMode)
	}

	// WAN ports (idx 5=eth4, idx 7=eth6) must NOT appear in gw.Ports
	for _, p := range gw.Ports {
		if p.Number == 5 || p.Number == 7 {
			t.Errorf("WAN port %d should not appear in gateway LAN ports", p.Number)
		}
	}

	// 2 WAN network confs → 2 WanRefs
	if len(gw.Wans) != 2 {
		t.Fatalf("expected 2 WAN refs, got %d", len(gw.Wans))
	}

	// WanRef ids follow "controller.kebab-name" convention
	wanByID := make(map[string]WanRef)
	for _, w := range gw.Wans {
		wanByID[w.Id] = w
	}
	wan1, ok := wanByID["unifi.internet-1"]
	if !ok {
		t.Fatalf("expected WanRef unifi.internet-1, got ids: %v", func() []string {
			ids := make([]string, 0, len(wanByID))
			for id := range wanByID {
				ids = append(ids, id)
			}
			return ids
		}())
	}
	if wan1.Uri != "/network/wans/unifi.internet-1" {
		t.Errorf("expected uri /network/wans/unifi.internet-1, got %s", wan1.Uri)
	}
	if wan1.Name != "Internet 1" {
		t.Errorf("expected name Internet 1, got %s", wan1.Name)
	}
	if _, ok := wanByID["unifi.internet-2"]; !ok {
		t.Error("expected WanRef unifi.internet-2")
	}
}
```

- [ ] **Step 2: Run the test to confirm it passes**

```bash
go test ./internal/network/... -run TestGetDevice_Gateway_PortsAndWans -v
```

Expected: PASS. (Implementation was done in Task 5; if it fails, check `buildGatewayWanRefs` and `buildGatewayPorts`.)

- [ ] **Step 3: Commit**

```bash
git add internal/network/service_test.go
git commit -m "test: verify GatewayDetail ports and wans (spec v1.5.0)"
```

---

## Task 7: Broaden `ListPorts` to include gateway LAN ports

**Files:**
- Modify: `internal/network/ports_service.go`

- [ ] **Step 1: Add `isSwitchOrGateway` helper**

Add after the `matchesVlanID` function:

```go
// isSwitchOrGateway reports whether the device type contributes LAN ports to the /network/ports listing.
func isSwitchOrGateway(deviceType string) bool {
	switch deviceType {
	case "usw", "ugw", "udm", "udm-pro":
		return true
	}
	return false
}
```

- [ ] **Step 2: Update `ListPorts` to use `isSwitchOrGateway` and call `buildGatewayPorts` for gateways**

Replace the inner loop in `ListPorts`:

```go
// existing — replace this entire block:
for _, d := range devices {
    if d.Type != "usw" {
        continue
    }
    switchID := fmt.Sprintf("%s.%s", entry.Name, toKebab(d.Name))
    sw := NetworkDeviceRef{
        Kind: NetworkDeviceRefKindDevice,
        Id:   switchID,
        Uri:  fmt.Sprintf("/network/devices/%s", switchID),
        Name: d.Name,
    }
    ports := buildSwitchPorts(entry.Name, d, swPortToDevice, swPortToClient, confs)
    for _, p := range ports {
        np := toNetworkPort(p, sw)
        if matchesFilter(np, params) {
            items = append(items, np)
        }
    }
}
```

with:

```go
for _, d := range devices {
    if !isSwitchOrGateway(d.Type) {
        continue
    }
    deviceID := fmt.Sprintf("%s.%s", entry.Name, toKebab(d.Name))
    devRef := NetworkDeviceRef{
        Kind: NetworkDeviceRefKindDevice,
        Id:   deviceID,
        Uri:  fmt.Sprintf("/network/devices/%s", deviceID),
        Name: d.Name,
    }
    var ports []DevicePort
    switch d.Type {
    case "ugw", "udm", "udm-pro":
        ports = buildGatewayPorts(entry.Name, d, swPortToDevice, swPortToClient, confs)
    default:
        ports = buildDevicePorts(entry.Name, d, swPortToDevice, swPortToClient, confs)
    }
    for _, p := range ports {
        np := toNetworkPort(p, devRef)
        if matchesFilter(np, params) {
            items = append(items, np)
        }
    }
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/network/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/network/ports_service.go
git commit -m "feat: include gateway LAN ports in /network/ports listing"
```

---

## Task 8: Test gateway ports in the listing

**Files:**
- Modify: `internal/network/ports_service_test.go`

- [ ] **Step 1: Update the no-filter port count from 35 to 40**

In `TestListNetworkPorts_NoFilter`, change:
```go
if len(result.Items) != 35 {
    t.Fatalf("expected 35 ports, got %d", len(result.Items))
}
```
to:
```go
if len(result.Items) != 40 {
    t.Fatalf("expected 40 ports (35 switch + 5 gateway LAN), got %d", len(result.Items))
}
```

- [ ] **Step 2: Run the updated test**

```bash
go test ./internal/network/... -run TestListNetworkPorts_NoFilter -v
```

Expected: PASS.

- [ ] **Step 3: Add `TestListNetworkPorts_GatewayPorts`**

```go
func TestListNetworkPorts_GatewayPorts(t *testing.T) {
	result, err := portsSvc(t).ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect all gateway ports
	var gwPorts []NetworkPort
	for _, p := range result.Items {
		if p.Device.Id == "unifi.cgf-01" {
			gwPorts = append(gwPorts, p)
		}
	}

	// 5 LAN ports (WAN ports eth4=idx5, eth6=idx7 excluded)
	if len(gwPorts) != 5 {
		t.Fatalf("expected 5 gateway LAN ports, got %d", len(gwPorts))
	}

	// device ref points at the gateway
	for _, p := range gwPorts {
		if p.Device.Id != "unifi.cgf-01" {
			t.Errorf("port %d: expected device.id=unifi.cgf-01, got %s", p.Number, p.Device.Id)
		}
		if p.Device.Kind != NetworkDeviceRefKindDevice {
			t.Errorf("port %d: expected device.kind=device, got %s", p.Number, p.Device.Kind)
		}
		if p.Device.Uri != "/network/devices/unifi.cgf-01" {
			t.Errorf("port %d: expected device.uri=/network/devices/unifi.cgf-01, got %s", p.Number, p.Device.Uri)
		}
	}

	// WAN ports (idx 5 and 7) must not appear
	gwPortNums := make(map[int]bool)
	for _, p := range gwPorts {
		gwPortNums[p.Number] = true
	}
	if gwPortNums[5] {
		t.Error("WAN port 5 (eth4) must not appear in gateway LAN port listing")
	}
	if gwPortNums[7] {
		t.Error("WAN port 7 (eth6) must not appear in gateway LAN port listing")
	}
}
```

- [ ] **Step 4: Add `TestListNetworkPorts_DeviceIdFilter_Gateway`**

```go
func TestListNetworkPorts_DeviceIdFilter_Gateway(t *testing.T) {
	params := ListNetworkPortsParams{DeviceId: strPtr("unifi.cgf-01")}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 ports for unifi.cgf-01, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.Device.Id != "unifi.cgf-01" {
			t.Errorf("expected device unifi.cgf-01, got %s", p.Device.Id)
		}
	}
}
```

- [ ] **Step 5: Run all network tests**

```bash
go test ./internal/network/... -v 2>&1 | tail -40
```

Expected: all tests pass.

- [ ] **Step 6: Run the full test suite**

```bash
go test ./...
```

Expected: all packages pass.

- [ ] **Step 7: Commit**

```bash
git add internal/network/ports_service_test.go
git commit -m "test: verify gateway LAN ports in /network/ports listing (spec v1.5.0)"
```
