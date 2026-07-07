# Switch Port VLAN Policy & Detail Fields — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the 5 new optional `SwitchPort` fields from spec 1.3.0 (`label`, `sfpModulePresent`, `linkUptime`, `lagMembership`, `vlanConfig`) by extending the UniFi adapter struct and adding mapping logic in the devices service.

**Architecture:** Spec and code generation are already complete (stubs regenerated, new types exist). This plan only touches the Go implementation: (1) extend `UniFiPortEntry` with 8 new JSON fields from real captures, (2) add `GetNetworkConf` to `DevicesBackend` and plumb confs through `GetDevice`, (3) add helper functions `confToVlanRef`, `findDefaultNetID`, `buildVlanConfig`, `buildPortLabel`, `buildSfpModulePresent`, `buildLinkUptime`, `buildLagMembership`, (4) wire all helpers into `buildSwitchPorts`.

**Tech Stack:** Go 1.26, `slices` package (stdlib), existing `extractVlanID`/`toKebab` helpers in `internal/network/`.

---

## File map

| File | Change |
|---|---|
| `internal/adapters/unifi.go` | Add 8 fields to `UniFiPortEntry` |
| `internal/network/testdata/unifi-devices.json` | Add `uptime` to port 1; add synthetic ports 9–12 to US 8 60W |
| `internal/network/devices_service.go` | Add `GetNetworkConf` to `DevicesBackend`; extend `GetDevice`, `buildDeviceDetail`, `buildSwitchDetail`, `buildSwitchPorts`; add 7 helper functions |
| `internal/network/service_test.go` | Update `TestGetDevice_Switch` (port count + networkConf); add 13 focused tests |

No new files. No handler or spec changes.

---

## Task 1: Extend `UniFiPortEntry` and update fixture

**Files:**
- Modify: `internal/adapters/unifi.go:243-254`
- Modify: `internal/network/testdata/unifi-devices.json`
- Modify: `internal/network/service_test.go:593`

- [ ] **Step 1: Add 8 fields to `UniFiPortEntry`**

In `internal/adapters/unifi.go`, replace the `UniFiPortEntry` struct (currently lines 243–254):

```go
type UniFiPortEntry struct {
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

- [ ] **Step 2: Add `"uptime": 3600` to port 1 of the US 8 60W fixture**

In `internal/network/testdata/unifi-devices.json`, find the US 8 60W device (model `"US8P60"`) and locate port 1 in its `port_table`. Add `"uptime": 3600` next to the other port-level fields (e.g., after `"up": true`). Port 1 currently does not have this field; inserting it verifies `linkUptime` decoding.

- [ ] **Step 3: Append synthetic ports 9–12 to the US 8 60W `port_table`**

Append these four objects to the US 8 60W `port_table` array (after the existing port 8 entry):

```json
{
  "port_idx": 9,
  "up": false,
  "speed": 0,
  "port_poe": false,
  "poe_mode": "",
  "poe_power": "",
  "tx_bytes": 0,
  "rx_bytes": 0,
  "tx_bytes-r": 0.0,
  "rx_bytes-r": 0.0,
  "name": "Port 9",
  "media": "SFP+",
  "forward": "all",
  "sfp_found": true,
  "aggregated_by": false
},
{
  "port_idx": 10,
  "up": false,
  "speed": 0,
  "port_poe": false,
  "poe_mode": "",
  "poe_power": "",
  "tx_bytes": 0,
  "rx_bytes": 0,
  "tx_bytes-r": 0.0,
  "rx_bytes-r": 0.0,
  "name": "Port 10",
  "media": "SFP+",
  "forward": "all",
  "sfp_found": false,
  "aggregated_by": false
},
{
  "port_idx": 11,
  "up": true,
  "speed": 1000,
  "port_poe": false,
  "poe_mode": "",
  "poe_power": "",
  "tx_bytes": 500000,
  "rx_bytes": 300000,
  "tx_bytes-r": 50.0,
  "rx_bytes-r": 30.0,
  "name": "LAG Master",
  "media": "GE",
  "forward": "all",
  "uptime": 7200,
  "aggregated_by": false
},
{
  "port_idx": 12,
  "up": true,
  "speed": 1000,
  "port_poe": false,
  "poe_mode": "",
  "poe_power": "",
  "tx_bytes": 500000,
  "rx_bytes": 300000,
  "tx_bytes-r": 50.0,
  "rx_bytes-r": 30.0,
  "name": "Port 12",
  "media": "GE",
  "forward": "all",
  "aggregated_by": 11
}
```

- [ ] **Step 4: Update `TestGetDevice_Switch` port count assertion**

In `internal/network/service_test.go` at line 593, change `!= 8` to `!= 12`:

```go
if len(sw.Ports) != 12 {
    t.Fatalf("expected 12 ports, got %d", len(sw.Ports))
}
```

- [ ] **Step 5: Run tests to confirm no regressions**

```
go test ./internal/network/ -run TestGetDevice_Switch -v
```

Expected: PASS. The new struct fields decode silently; existing assertions are unchanged.

```
go test ./internal/adapters/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/unifi.go internal/network/testdata/unifi-devices.json internal/network/service_test.go
git commit -m "feat: add new UniFiPortEntry fields and extend switch fixture"
```

---

## Task 2: Add `GetNetworkConf` to `DevicesBackend` and plumb through call chain

**Files:**
- Modify: `internal/network/devices_service.go`

- [ ] **Step 1: Write failing test**

In `internal/network/service_test.go`, add this test after `TestGetDevice_Switch`:

```go
func TestGetDevice_Switch_NetworkConfFetched(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	confs := testhelpers.LoadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients, networkConf: confs}}, map[string]int{"unifi": 30}, slog.Default(), nil)

	detail, err := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}
	// vlanConfig is populated — proves network confs were passed to buildSwitchPorts
	p2 := sw.Ports[1] // port 2, access mode
	if p2.VlanConfig == nil {
		t.Fatal("expected vlanConfig on port 2, got nil")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```
go test ./internal/network/ -run TestGetDevice_Switch_NetworkConfFetched -v
```

Expected: FAIL — `vlanConfig` is nil because confs are not yet plumbed through.

- [ ] **Step 3: Add `GetNetworkConf` to `DevicesBackend` interface**

In `internal/network/devices_service.go`, update the interface:

```go
// DevicesBackend is the narrow interface for device operations.
type DevicesBackend interface {
	GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
	GetClients(ctx context.Context) ([]adapters.UniFiSta, error)
	GetNetworkConf(ctx context.Context) ([]adapters.UniFiNetworkConf, error)
}
```

- [ ] **Step 4: Call `GetNetworkConf` in `GetDevice` and pass confs downstream**

Replace the `GetDevice` function body in `internal/network/devices_service.go` (currently lines 41–77):

```go
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, error) {
	controller, suffix, err := parseID(id)
	if err != nil {
		return NetworkDeviceDetail{}, err
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkDeviceDetail{}, err
	}

	devices, err := backend.GetDevices(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi devices: %w", err)
	}

	clients, err := backend.GetClients(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi clients: %w", err)
	}

	confs, err := backend.GetNetworkConf(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi network conf: %w", err)
	}

	macToDevice := buildMacToDevice(devices)
	swPortToDevice := buildSwPortToDevice(devices)
	swPortToClient := buildSwPortToClient(clients)
	apMacToClients := buildApMacToClients(clients)

	for _, d := range devices {
		if toKebab(d.Name) == suffix {
			detail, err := buildDeviceDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, apMacToClients, confs)
			if err != nil {
				return NetworkDeviceDetail{}, err
			}
			return detail, nil
		}
	}
	return NetworkDeviceDetail{}, fmt.Errorf("Network device not found: %s: %w", id, apierrors.ErrNotFound)
}
```

- [ ] **Step 5: Add `confs` parameter to `buildDeviceDetail`**

Update the `buildDeviceDetail` function signature and body (currently lines 93–111):

```go
func buildDeviceDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	apMacToClients map[string][]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) (NetworkDeviceDetail, error) {
	switch d.Type {
	case "usw":
		return buildSwitchDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, confs)
	case "uap":
		return buildAPDetail(controller, d, macToDevice, apMacToClients)
	case "ugw", "udm", "udm-pro":
		return buildGatewayDetail(controller, d)
	default:
		return buildUnknownDetail(controller, d, macToDevice)
	}
}
```

- [ ] **Step 6: Add `confs` parameter to `buildSwitchDetail` and `buildSwitchPorts`**

Update `buildSwitchDetail` (currently lines 153–181) to accept and forward `confs`:

```go
func buildSwitchDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	ports := buildSwitchPorts(controller, d, swPortToDevice, swPortToClient, confs)

	var det NetworkDeviceDetail
	err := det.FromSwitchDetail(SwitchDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Switch,
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
```

Update `buildSwitchPorts` signature to accept `confs`:

```go
func buildSwitchPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []SwitchPort {
```

(The body of `buildSwitchPorts` does not yet use `confs` — that comes in later tasks.)

- [ ] **Step 7: Run the new test to confirm it still fails for the right reason**

```
go test ./internal/network/ -run TestGetDevice_Switch_NetworkConfFetched -v
```

Expected: still FAIL — `vlanConfig` is nil because `buildSwitchPorts` doesn't call `buildVlanConfig` yet.

- [ ] **Step 8: Run full test suite**

```
go test ./internal/network/ -v 2>&1 | grep -E "PASS|FAIL|---"
```

Expected: all existing tests PASS (the new `confs` parameter defaults to nil in existing tests that don't set `networkConf`).

- [ ] **Step 9: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: add GetNetworkConf to DevicesBackend and plumb confs through GetDevice"
```

---

## Task 3: Implement `confToVlanRef` and `findDefaultNetID`

**Files:**
- Modify: `internal/network/devices_service.go`
- Modify: `internal/network/service_test.go`

These are the foundation helpers used by `buildVlanConfig` in the next task.

- [ ] **Step 1: Write failing tests**

Add to `internal/network/service_test.go`:

```go
func TestConfToVlanRef_Tagged(t *testing.T) {
	conf := adapters.UniFiNetworkConf{
		ID:          "5e1cccb3af427c0011f58cb6",
		Name:        "LAN-IOT",
		Purpose:     "corporate",
		Vlan:        float64(20),
		VlanEnabled: true,
	}
	ref := confToVlanRef(conf, "unifi")
	if ref.Id != "unifi.lan-iot" {
		t.Errorf("expected id unifi.lan-iot, got %s", ref.Id)
	}
	if ref.Uri != "/network/vlans/unifi.lan-iot" {
		t.Errorf("expected uri /network/vlans/unifi.lan-iot, got %s", ref.Uri)
	}
	if ref.Name != "LAN-IOT" {
		t.Errorf("expected name LAN-IOT, got %s", ref.Name)
	}
	if ref.VlanId != 20 {
		t.Errorf("expected vlanId 20, got %d", ref.VlanId)
	}
}

func TestConfToVlanRef_Default(t *testing.T) {
	conf := adapters.UniFiNetworkConf{
		ID:          "5e136551af427c0011f23b55",
		Name:        "LAN-MGMT",
		Purpose:     "corporate",
		Vlan:        "",
		VlanEnabled: false,
	}
	ref := confToVlanRef(conf, "unifi")
	if ref.Id != "unifi.lan-mgmt" {
		t.Errorf("expected id unifi.lan-mgmt, got %s", ref.Id)
	}
	if ref.VlanId != 1 {
		t.Errorf("expected vlanId 1 for default network, got %d", ref.VlanId)
	}
}

func TestFindDefaultNetID(t *testing.T) {
	confs := []adapters.UniFiNetworkConf{
		{ID: "wan1", Purpose: "wan"},
		{ID: "tagged", Purpose: "corporate", VlanEnabled: true},
		{ID: "default", Purpose: "corporate", VlanEnabled: false},
	}
	if got := findDefaultNetID(confs); got != "default" {
		t.Errorf("expected default, got %s", got)
	}
}

func TestFindDefaultNetID_Empty(t *testing.T) {
	if got := findDefaultNetID(nil); got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/network/ -run "TestConfToVlanRef|TestFindDefaultNetID" -v
```

Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement `confToVlanRef` and `findDefaultNetID`**

Add to `internal/network/devices_service.go` (after the `buildSwitchPorts` function):

```go
// confToVlanRef converts a UniFiNetworkConf to a NetworkVlanRef using
// the same composite-ID convention as the VLANs service.
func confToVlanRef(conf adapters.UniFiNetworkConf, controller string) NetworkVlanRef {
	id := fmt.Sprintf("%s.%s", controller, toKebab(conf.Name))
	return NetworkVlanRef{
		Id:     id,
		Uri:    fmt.Sprintf("/network/vlans/%s", id),
		Name:   conf.Name,
		VlanId: extractVlanID(conf.Vlan),
	}
}

// findDefaultNetID returns the _id of the default (untagged, corporate) network conf.
// Returns empty string when none is found.
func findDefaultNetID(confs []adapters.UniFiNetworkConf) string {
	for _, c := range confs {
		if c.Purpose == "corporate" && !c.VlanEnabled {
			return c.ID
		}
	}
	return ""
}
```

Note: `extractVlanID` already exists in `vlans_service.go` (same package) — do not redeclare it.

- [ ] **Step 4: Add `"slices"` to imports in `devices_service.go` if not already present**

The `slices` import is needed in Task 4. Add it now to `devices_service.go`:

```go
import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)
```

- [ ] **Step 5: Run tests to confirm they pass**

```
go test ./internal/network/ -run "TestConfToVlanRef|TestFindDefaultNetID" -v
```

Expected: all 4 PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: add confToVlanRef and findDefaultNetID helpers"
```

---

## Task 4: Implement `buildVlanConfig`

**Files:**
- Modify: `internal/network/devices_service.go`
- Modify: `internal/network/service_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/network/service_test.go`:

```go
func makeTestConfs() ([]adapters.UniFiNetworkConf, map[string]adapters.UniFiNetworkConf) {
	confs := []adapters.UniFiNetworkConf{
		{ID: "id-mgmt", Name: "LAN-MGMT", Purpose: "corporate", VlanEnabled: false, Vlan: ""},
		{ID: "id-iot", Name: "LAN-IOT", Purpose: "corporate", VlanEnabled: true, Vlan: float64(20)},
		{ID: "id-int", Name: "LAN-INT", Purpose: "corporate", VlanEnabled: true, Vlan: float64(10)},
		{ID: "id-srv", Name: "LAN-SRV", Purpose: "corporate", VlanEnabled: true, Vlan: float64(100)},
		{ID: "id-wan", Name: "WAN", Purpose: "wan"},
	}
	byID := make(map[string]adapters.UniFiNetworkConf, len(confs))
	for _, c := range confs {
		byID[c.ID] = c
	}
	return confs, byID
}

func TestBuildVlanConfig_Access(t *testing.T) {
	_, byID := makeTestConfs()
	nativeID := "id-int"
	p := adapters.UniFiPortEntry{
		PortIdx:             2,
		Forward:             "native",
		TaggedVlanMgmt:      "block_all",
		NativeNetworkConfID: &nativeID,
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg == nil {
		t.Fatal("expected vlanConfig, got nil")
	}
	if cfg.Mode != Access {
		t.Errorf("expected mode access, got %s", cfg.Mode)
	}
	if cfg.NativeVlan.Id != "unifi.lan-int" {
		t.Errorf("expected nativeVlan id unifi.lan-int, got %s", cfg.NativeVlan.Id)
	}
	if cfg.NativeVlan.VlanId != 10 {
		t.Errorf("expected nativeVlan vlanId 10, got %d", cfg.NativeVlan.VlanId)
	}
	if cfg.TaggedVlans != nil {
		t.Errorf("expected nil taggedVlans for access mode, got %+v", cfg.TaggedVlans)
	}
}

func TestBuildVlanConfig_TrunkAllForwardAll(t *testing.T) {
	_, byID := makeTestConfs()
	p := adapters.UniFiPortEntry{
		PortIdx: 1,
		Forward: "all",
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg == nil {
		t.Fatal("expected vlanConfig, got nil")
	}
	if cfg.Mode != Trunk {
		t.Errorf("expected mode trunk, got %s", cfg.Mode)
	}
	if cfg.NativeVlan.Id != "unifi.lan-mgmt" {
		t.Errorf("expected fallback nativeVlan unifi.lan-mgmt, got %s", cfg.NativeVlan.Id)
	}
	if cfg.TaggedVlans == nil || cfg.TaggedVlans.Scope != All {
		t.Errorf("expected taggedVlans scope all, got %+v", cfg.TaggedVlans)
	}
	if cfg.TaggedVlans.Items != nil {
		t.Errorf("expected nil items for scope all, got %v", cfg.TaggedVlans.Items)
	}
}

func TestBuildVlanConfig_TrunkAllForwardAllExplicitNative(t *testing.T) {
	_, byID := makeTestConfs()
	nativeID := "id-mgmt"
	p := adapters.UniFiPortEntry{
		PortIdx:             7,
		Forward:             "all",
		NativeNetworkConfID: &nativeID,
		TaggedVlanMgmt:      "auto",
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg == nil {
		t.Fatal("expected vlanConfig, got nil")
	}
	if cfg.NativeVlan.Id != "unifi.lan-mgmt" {
		t.Errorf("expected nativeVlan unifi.lan-mgmt, got %s", cfg.NativeVlan.Id)
	}
	if cfg.TaggedVlans == nil || cfg.TaggedVlans.Scope != All {
		t.Errorf("expected scope all, got %+v", cfg.TaggedVlans)
	}
}

func TestBuildVlanConfig_TrunkCustom(t *testing.T) {
	_, byID := makeTestConfs()
	nativeID := "id-mgmt"
	p := adapters.UniFiPortEntry{
		PortIdx:               6,
		Forward:               "customize",
		NativeNetworkConfID:   &nativeID,
		TaggedVlanMgmt:        "custom",
		ExcludedNetworkConfIDs: []string{"id-iot", "id-srv"},
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg == nil {
		t.Fatal("expected vlanConfig, got nil")
	}
	if cfg.Mode != Trunk {
		t.Errorf("expected mode trunk, got %s", cfg.Mode)
	}
	if cfg.NativeVlan.Id != "unifi.lan-mgmt" {
		t.Errorf("expected nativeVlan unifi.lan-mgmt, got %s", cfg.NativeVlan.Id)
	}
	if cfg.TaggedVlans == nil || cfg.TaggedVlans.Scope != Custom {
		t.Errorf("expected scope custom, got %+v", cfg.TaggedVlans)
	}
	items := *cfg.TaggedVlans.Items
	if len(items) != 1 {
		t.Fatalf("expected 1 tagged VLAN item, got %d: %+v", len(items), items)
	}
	if items[0].Id != "unifi.lan-int" {
		t.Errorf("expected item unifi.lan-int, got %s", items[0].Id)
	}
}

func TestBuildVlanConfig_TrunkCustomizeEmptyExcluded(t *testing.T) {
	_, byID := makeTestConfs()
	nativeID := "id-mgmt"
	p := adapters.UniFiPortEntry{
		PortIdx:               7,
		Forward:               "customize",
		NativeNetworkConfID:   &nativeID,
		ExcludedNetworkConfIDs: []string{},
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg == nil {
		t.Fatal("expected vlanConfig, got nil")
	}
	if cfg.TaggedVlans == nil || cfg.TaggedVlans.Scope != All {
		t.Errorf("expected scope all for empty excluded, got %+v", cfg.TaggedVlans)
	}
}

func TestBuildVlanConfig_DisableOmits(t *testing.T) {
	_, byID := makeTestConfs()
	p := adapters.UniFiPortEntry{PortIdx: 1, Forward: "disable"}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg != nil {
		t.Errorf("expected nil for forward=disable, got %+v", cfg)
	}
}

func TestBuildVlanConfig_UnresolvableNativeOmits(t *testing.T) {
	_, byID := makeTestConfs()
	badID := "nonexistent"
	p := adapters.UniFiPortEntry{
		PortIdx:             1,
		Forward:             "native",
		TaggedVlanMgmt:      "block_all",
		NativeNetworkConfID: &badID,
	}
	cfg := buildVlanConfig(p, byID, "id-mgmt", "unifi")
	if cfg != nil {
		t.Errorf("expected nil when native conf unresolvable, got %+v", cfg)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/network/ -run TestBuildVlanConfig -v
```

Expected: FAIL — `buildVlanConfig` not defined.

- [ ] **Step 3: Implement `buildVlanConfig`**

Add to `internal/network/devices_service.go` (after `findDefaultNetID`):

```go
// buildVlanConfig maps UniFi port VLAN fields to the API SwitchPortVlanConfig.
// Returns nil when the port is disabled or the native VLAN cannot be resolved.
func buildVlanConfig(
	p adapters.UniFiPortEntry,
	confByID map[string]adapters.UniFiNetworkConf,
	defaultNetID string,
	controller string,
) *SwitchPortVlanConfig {
	switch p.Forward {
	case "disable":
		return nil
	case "native":
		// access mode: single untagged VLAN, no tagged traffic
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		return &SwitchPortVlanConfig{
			Mode:       Access,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
	case "all", "customize":
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		cfg := &SwitchPortVlanConfig{
			Mode:       Trunk,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
		if p.Forward == "customize" && len(p.ExcludedNetworkConfIDs) > 0 {
			// trunk-custom: all corporate VLANs minus excluded and minus native
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
			tagged := SwitchPortVlanConfigTaggedVlans{Scope: Custom, Items: &items}
			cfg.TaggedVlans = &tagged
		} else {
			// trunk-all
			tagged := SwitchPortVlanConfigTaggedVlans{Scope: All}
			cfg.TaggedVlans = &tagged
		}
		return cfg
	default:
		return nil
	}
}

// resolveNativeConf returns the network conf for the port's native VLAN.
// Falls back to defaultNetID when NativeNetworkConfID is nil.
func resolveNativeConf(
	nativeID *string,
	defaultNetID string,
	confByID map[string]adapters.UniFiNetworkConf,
) (adapters.UniFiNetworkConf, bool) {
	id := defaultNetID
	if nativeID != nil && *nativeID != "" {
		id = *nativeID
	}
	if id == "" {
		return adapters.UniFiNetworkConf{}, false
	}
	conf, ok := confByID[id]
	return conf, ok
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/network/ -run TestBuildVlanConfig -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: implement buildVlanConfig with 4-mode UniFi VLAN mapping"
```

---

## Task 5: Implement `buildPortLabel`, `buildSfpModulePresent`, `buildLinkUptime`, `buildLagMembership`

**Files:**
- Modify: `internal/network/devices_service.go`
- Modify: `internal/network/service_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/network/service_test.go`:

```go
func TestBuildPortLabel_Default(t *testing.T) {
	p := adapters.UniFiPortEntry{PortIdx: 3, Name: "Port 3"}
	if buildPortLabel(p) != nil {
		t.Errorf("expected nil label for default port name")
	}
}

func TestBuildPortLabel_Custom(t *testing.T) {
	p := adapters.UniFiPortEntry{PortIdx: 11, Name: "LAG Master"}
	label := buildPortLabel(p)
	if label == nil || *label != "LAG Master" {
		t.Errorf("expected label 'LAG Master', got %v", label)
	}
}

func TestBuildSfpModulePresent_True(t *testing.T) {
	v := true
	p := adapters.UniFiPortEntry{SfpFound: &v}
	result := buildSfpModulePresent(p)
	if result == nil || !*result {
		t.Errorf("expected sfpModulePresent true, got %v", result)
	}
}

func TestBuildSfpModulePresent_False(t *testing.T) {
	v := false
	p := adapters.UniFiPortEntry{SfpFound: &v}
	result := buildSfpModulePresent(p)
	if result == nil || *result {
		t.Errorf("expected sfpModulePresent false, got %v", result)
	}
}

func TestBuildSfpModulePresent_Nil(t *testing.T) {
	p := adapters.UniFiPortEntry{}
	if buildSfpModulePresent(p) != nil {
		t.Errorf("expected nil sfpModulePresent for non-SFP port")
	}
}

func TestBuildLinkUptime_Up(t *testing.T) {
	u := 3600
	p := adapters.UniFiPortEntry{Up: true, Uptime: &u}
	result := buildLinkUptime(p)
	if result == nil || int(*result) != 3600 {
		t.Errorf("expected linkUptime 3600, got %v", result)
	}
}

func TestBuildLinkUptime_UpNoUptimeField(t *testing.T) {
	p := adapters.UniFiPortEntry{Up: true}
	if buildLinkUptime(p) != nil {
		t.Errorf("expected nil when uptime field absent")
	}
}

func TestBuildLinkUptime_Down(t *testing.T) {
	u := 999
	p := adapters.UniFiPortEntry{Up: false, Uptime: &u}
	if buildLinkUptime(p) != nil {
		t.Errorf("expected nil linkUptime for down port")
	}
}

func TestBuildLagMembership_Master(t *testing.T) {
	masterSet := map[int]bool{11: true}
	p := adapters.UniFiPortEntry{PortIdx: 11, AggregatedBy: false}
	result := buildLagMembership(p, masterSet)
	if result == nil {
		t.Fatal("expected lagMembership for master port")
	}
	if result.Role != Master {
		t.Errorf("expected role master, got %s", result.Role)
	}
	if result.Id != 11 {
		t.Errorf("expected id 11, got %d", result.Id)
	}
}

func TestBuildLagMembership_Member(t *testing.T) {
	masterSet := map[int]bool{11: true}
	p := adapters.UniFiPortEntry{PortIdx: 12, AggregatedBy: float64(11)}
	result := buildLagMembership(p, masterSet)
	if result == nil {
		t.Fatal("expected lagMembership for member port")
	}
	if result.Role != Member {
		t.Errorf("expected role member, got %s", result.Role)
	}
	if result.Id != 11 {
		t.Errorf("expected id 11, got %d", result.Id)
	}
}

func TestBuildLagMembership_NotInLag(t *testing.T) {
	masterSet := map[int]bool{11: true}
	p := adapters.UniFiPortEntry{PortIdx: 1, AggregatedBy: false}
	if buildLagMembership(p, masterSet) != nil {
		t.Errorf("expected nil lagMembership for non-LAG port")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
go test ./internal/network/ -run "TestBuildPortLabel|TestBuildSfpModule|TestBuildLinkUptime|TestBuildLagMembership" -v
```

Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement the four helpers**

Add to `internal/network/devices_service.go` (after `resolveNativeConf`):

```go
func buildPortLabel(p adapters.UniFiPortEntry) *string {
	if p.Name == "Port "+strconv.Itoa(p.PortIdx) {
		return nil
	}
	return &p.Name
}

func buildSfpModulePresent(p adapters.UniFiPortEntry) *bool {
	return p.SfpFound
}

func buildLinkUptime(p adapters.UniFiPortEntry) *Seconds {
	if !p.Up || p.Uptime == nil {
		return nil
	}
	s := Seconds(*p.Uptime)
	return &s
}

// buildLagMembership derives LAG role from AggregatedBy and a pre-computed masterSet.
// masterSet is keyed by port_idx of every port that has members pointing to it.
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

- [ ] **Step 4: Run tests to confirm they pass**

```
go test ./internal/network/ -run "TestBuildPortLabel|TestBuildSfpModule|TestBuildLinkUptime|TestBuildLagMembership" -v
```

Expected: all 11 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: implement buildPortLabel, buildSfpModulePresent, buildLinkUptime, buildLagMembership"
```

---

## Task 6: Wire helpers into `buildSwitchPorts` and add integration tests

**Files:**
- Modify: `internal/network/devices_service.go`
- Modify: `internal/network/service_test.go`

- [ ] **Step 1: Rewrite `buildSwitchPorts` to use all helpers**

Replace the `buildSwitchPorts` function body in `internal/network/devices_service.go`:

```go
func buildSwitchPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []SwitchPort {
	// Build per-ID conf lookup and find the default (untagged) network ID.
	confByID := make(map[string]adapters.UniFiNetworkConf, len(confs))
	for _, c := range confs {
		confByID[c.ID] = c
	}
	defaultNetID := findDefaultNetID(confs)

	// Pass 1: find which port indices are LAG masters (have members pointing to them).
	masterSet := make(map[int]bool)
	for _, p := range d.PortTable {
		if v, ok := p.AggregatedBy.(float64); ok {
			masterSet[int(v)] = true
		}
	}

	switchMAC := normalizeMac(d.MAC)
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
		if p.Up && p.Speed > 0 {
			ls := mapLinkSpeed(p.Speed)
			if ls != "" {
				port.LinkSpeed = &ls
			}
		}
		if p.PortPoe && p.PoePower != "" {
			watts, err := strconv.ParseFloat(p.PoePower, 64)
			if err == nil {
				w := Watts(watts)
				port.PoePowerWatts = &w
			}
		}
		port.ConnectedTo = resolvePortConnectedTo(controller, switchMAC, p.PortIdx, swPortToDevice, swPortToClient)
		ports = append(ports, port)
	}
	return ports
}
```

- [ ] **Step 2: Run existing tests to confirm no regressions**

```
go test ./internal/network/ -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL|ok)"
```

Expected: all tests PASS. The `TestGetDevice_Switch_NetworkConfFetched` test will now also PASS.

- [ ] **Step 3: Update `TestGetDevice_Switch` to supply network confs**

In `service_test.go`, update `TestGetDevice_Switch` (around line 573) to load and supply network confs:

```go
func TestGetDevice_Switch(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	confs := testhelpers.LoadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients, networkConf: confs}}, map[string]int{"unifi": 30}, slog.Default(), nil)

	detail, err := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	// ... (keep all existing assertions unchanged, port count already updated to 12 in Task 1)
```

- [ ] **Step 4: Add integration tests for the new fields**

Add to `internal/network/service_test.go`:

```go
func switchSvcWithConfs(t *testing.T) *Service {
	t.Helper()
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	confs := testhelpers.LoadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	return NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients, networkConf: confs}}, map[string]int{"unifi": 30}, slog.Default(), nil)
}

func switchPorts(t *testing.T, svc *Service) []SwitchPort {
	t.Helper()
	detail, err := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}
	return sw.Ports
}

func findPort(ports []SwitchPort, number int) *SwitchPort {
	for i := range ports {
		if ports[i].Number == number {
			return &ports[i]
		}
	}
	return nil
}

func TestGetDevice_Switch_VlanAccess(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p2 := findPort(ports, 2)
	if p2 == nil {
		t.Fatal("port 2 not found")
	}
	if p2.VlanConfig == nil {
		t.Fatal("expected vlanConfig on port 2")
	}
	if p2.VlanConfig.Mode != Access {
		t.Errorf("expected mode access, got %s", p2.VlanConfig.Mode)
	}
	if p2.VlanConfig.NativeVlan.Id != "unifi.lan-int" {
		t.Errorf("expected nativeVlan unifi.lan-int, got %s", p2.VlanConfig.NativeVlan.Id)
	}
	if p2.VlanConfig.TaggedVlans != nil {
		t.Errorf("expected nil taggedVlans for access, got %+v", p2.VlanConfig.TaggedVlans)
	}
}

func TestGetDevice_Switch_VlanTrunkAll(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p1 := findPort(ports, 1)
	if p1 == nil {
		t.Fatal("port 1 not found")
	}
	if p1.VlanConfig == nil {
		t.Fatal("expected vlanConfig on port 1")
	}
	if p1.VlanConfig.Mode != Trunk {
		t.Errorf("expected mode trunk, got %s", p1.VlanConfig.Mode)
	}
	// port 1 has forward:"all" with no native_networkconf_id; falls back to default (LAN-MGMT)
	if p1.VlanConfig.NativeVlan.Id != "unifi.lan-mgmt" {
		t.Errorf("expected fallback nativeVlan unifi.lan-mgmt, got %s", p1.VlanConfig.NativeVlan.Id)
	}
	if p1.VlanConfig.TaggedVlans == nil || p1.VlanConfig.TaggedVlans.Scope != All {
		t.Errorf("expected taggedVlans scope all, got %+v", p1.VlanConfig.TaggedVlans)
	}
}

func TestGetDevice_Switch_VlanTrunkAllExplicitNative(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p7 := findPort(ports, 7)
	if p7 == nil {
		t.Fatal("port 7 not found")
	}
	if p7.VlanConfig == nil {
		t.Fatal("expected vlanConfig on port 7")
	}
	// port 7 has forward:"all" AND native_networkconf_id:"5e136551af427c0011f23b55" (LAN-MGMT)
	if p7.VlanConfig.NativeVlan.Id != "unifi.lan-mgmt" {
		t.Errorf("expected nativeVlan unifi.lan-mgmt, got %s", p7.VlanConfig.NativeVlan.Id)
	}
	if p7.VlanConfig.TaggedVlans == nil || p7.VlanConfig.TaggedVlans.Scope != All {
		t.Errorf("expected scope all, got %+v", p7.VlanConfig.TaggedVlans)
	}
}

func TestGetDevice_Switch_VlanTrunkCustom(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p6 := findPort(ports, 6)
	if p6 == nil {
		t.Fatal("port 6 not found")
	}
	if p6.VlanConfig == nil {
		t.Fatal("expected vlanConfig on port 6")
	}
	if p6.VlanConfig.Mode != Trunk {
		t.Errorf("expected mode trunk, got %s", p6.VlanConfig.Mode)
	}
	if p6.VlanConfig.TaggedVlans == nil || p6.VlanConfig.TaggedVlans.Scope != Custom {
		t.Errorf("expected scope custom, got %+v", p6.VlanConfig.TaggedVlans)
	}
	// excluded: LAN-IOT (id-iot), LAN-SRV (id-srv)
	// native: LAN-MGMT (excluded from tagged items)
	// remaining corporate: LAN-INT only → vlanId 10
	items := *p6.VlanConfig.TaggedVlans.Items
	if len(items) != 1 {
		t.Fatalf("expected 1 tagged VLAN, got %d: %+v", len(items), items)
	}
	if items[0].Id != "unifi.lan-int" {
		t.Errorf("expected item unifi.lan-int, got %s", items[0].Id)
	}
}

func TestGetDevice_Switch_Label(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p11 := findPort(ports, 11)
	if p11 == nil {
		t.Fatal("port 11 not found")
	}
	if p11.Label == nil || *p11.Label != "LAG Master" {
		t.Errorf("expected label 'LAG Master', got %v", p11.Label)
	}
}

func TestGetDevice_Switch_NoLabelDefault(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p1 := findPort(ports, 1)
	if p1 == nil {
		t.Fatal("port 1 not found")
	}
	if p1.Label != nil {
		t.Errorf("expected nil label for default port name, got %v", p1.Label)
	}
}

func TestGetDevice_Switch_SfpPresent(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p9 := findPort(ports, 9)
	if p9 == nil {
		t.Fatal("port 9 not found")
	}
	if p9.SfpModulePresent == nil || !*p9.SfpModulePresent {
		t.Errorf("expected sfpModulePresent true, got %v", p9.SfpModulePresent)
	}
}

func TestGetDevice_Switch_SfpAbsent(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p10 := findPort(ports, 10)
	if p10 == nil {
		t.Fatal("port 10 not found")
	}
	if p10.SfpModulePresent == nil || *p10.SfpModulePresent {
		t.Errorf("expected sfpModulePresent false, got %v", p10.SfpModulePresent)
	}
}

func TestGetDevice_Switch_SfpNoCage(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p1 := findPort(ports, 1)
	if p1 == nil {
		t.Fatal("port 1 not found")
	}
	if p1.SfpModulePresent != nil {
		t.Errorf("expected nil sfpModulePresent for GE port, got %v", p1.SfpModulePresent)
	}
}

func TestGetDevice_Switch_LinkUptime(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p1 := findPort(ports, 1)
	if p1 == nil {
		t.Fatal("port 1 not found")
	}
	if p1.LinkUptime == nil || int(*p1.LinkUptime) != 3600 {
		t.Errorf("expected linkUptime 3600, got %v", p1.LinkUptime)
	}
}

func TestGetDevice_Switch_LagMaster(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p11 := findPort(ports, 11)
	if p11 == nil {
		t.Fatal("port 11 not found")
	}
	if p11.LagMembership == nil {
		t.Fatal("expected lagMembership on master port")
	}
	if p11.LagMembership.Role != Master {
		t.Errorf("expected role master, got %s", p11.LagMembership.Role)
	}
	if p11.LagMembership.Id != 11 {
		t.Errorf("expected id 11, got %d", p11.LagMembership.Id)
	}
}

func TestGetDevice_Switch_LagMember(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p12 := findPort(ports, 12)
	if p12 == nil {
		t.Fatal("port 12 not found")
	}
	if p12.LagMembership == nil {
		t.Fatal("expected lagMembership on member port")
	}
	if p12.LagMembership.Role != Member {
		t.Errorf("expected role member, got %s", p12.LagMembership.Role)
	}
	if p12.LagMembership.Id != 11 {
		t.Errorf("expected id 11, got %d", p12.LagMembership.Id)
	}
}

func TestGetDevice_Switch_NoLag(t *testing.T) {
	ports := switchPorts(t, switchSvcWithConfs(t))
	p1 := findPort(ports, 1)
	if p1 == nil {
		t.Fatal("port 1 not found")
	}
	if p1.LagMembership != nil {
		t.Errorf("expected nil lagMembership for non-LAG port, got %+v", p1.LagMembership)
	}
}
```

- [ ] **Step 5: Run all new integration tests**

```
go test ./internal/network/ -run "TestGetDevice_Switch_Vlan|TestGetDevice_Switch_Label|TestGetDevice_Switch_Sfp|TestGetDevice_Switch_LinkUptime|TestGetDevice_Switch_Lag" -v
```

Expected: all 13 integration tests PASS.

- [ ] **Step 6: Run full test suite and build**

```
go test ./... 2>&1 | tail -20
make build
```

Expected: all tests PASS, binary builds cleanly.

- [ ] **Step 7: Commit**

```bash
git add internal/network/devices_service.go internal/network/service_test.go
git commit -m "feat: wire VLAN config and port detail fields into buildSwitchPorts"
```

---

## Self-review notes

**Spec coverage check:**
- `label` ✅ Task 5 (`buildPortLabel`) + Task 6 (integration test)
- `sfpModulePresent` ✅ Task 5 (`buildSfpModulePresent`) + Task 6
- `linkUptime` ✅ Task 5 (`buildLinkUptime`) + Task 6
- `lagMembership` ✅ Task 5 (`buildLagMembership`) + Task 6
- `vlanConfig` (access) ✅ Task 4 + Task 6
- `vlanConfig` (trunk-all) ✅ Task 4 + Task 6
- `vlanConfig` (trunk-custom) ✅ Task 4 + Task 6
- `vlanConfig` (disable → omit) ✅ Task 4
- `vlanConfig` unresolvable native → omit ✅ Task 4
- `GetNetworkConf` added to `DevicesBackend` ✅ Task 2
- Fixture synthetic ports (SFP, LAG, uptime) ✅ Task 1
- `confToVlanRef` same convention as VLANs service ✅ Task 3

**Type consistency:**
- `SwitchPortLagMembershipRole` constants `Master`/`Member` match `api.gen.go:249-250`
- `SwitchPortVlanConfigMode` constants `Access`/`Trunk` match `api.gen.go:291-292`
- `SwitchPortVlanConfigTaggedVlansScope` constants `All`/`Custom` match `api.gen.go:309-310`
- `Seconds` type used for `LinkUptime` matches `api.gen.go` (`type Seconds = int`)
- `extractVlanID` called from `devices_service.go` — it lives in `vlans_service.go` in the same `network` package, no import needed
