# Network Ports Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `GET /network/ports` — a flat listing of switch ports across all UniFi controllers with four server-side filters.

**Architecture:** Add `ports_service.go` with `ListPorts` + `matchesFilter`; extend `service.go`'s `UniFiBackend` composite with a narrow `PortsBackend` interface; add `ListNetworkPorts` to `handler.go`. All port-building logic is reused from the existing `buildSwitchPorts` helper.

**Tech Stack:** Go, chi, oapi-codegen (stubs already generated), UniFi adapter (no changes needed).

---

## Task 1: Generate stubs and commit the spec submodule pointer

**Files:**
- Run: `make generate` (regenerates `internal/network/api.gen.go` — do not commit)
- Commit: `spec` submodule pointer only

- [ ] **Step 1: Run code generation**

```bash
make generate
```

Expected output ends with four `oapi-codegen` invocations (system, docker, storage, network, meta). No errors.

- [ ] **Step 2: Verify the new types are present**

```bash
grep -c "NetworkPort\|ListNetworkPorts" internal/network/api.gen.go
```

Expected: at least 20 matches. Spot-check:

```bash
grep "type NetworkPort struct\|type ListNetworkPortsParams\|type NetworkPortList" internal/network/api.gen.go
```

Expected: all three lines appear.

- [ ] **Step 3: Verify the build now fails (StrictServerInterface requires ListNetworkPorts)**

```bash
make build 2>&1 | grep -c "ListNetworkPorts"
```

Expected: at least 1 — the build fails because `ServerHandler` does not yet implement `StrictServerInterface.ListNetworkPorts`.

- [ ] **Step 4: Commit the spec submodule pointer — NOT api.gen.go**

```bash
git diff --cached --name-only | grep gen.go  # must be empty
git add spec
git commit -m "chore: update spec submodule to v1.4.0"
```

---

## Task 2: Restore the build — PortsBackend interface + stubs

**Files:**
- Modify: `internal/network/service.go` (add `PortsBackend` to `UniFiBackend`)
- Modify: `internal/network/handler.go` (add stub `ListNetworkPorts`)
- Create: `internal/network/ports_service.go` (stub `ListPorts`)

- [ ] **Step 1: Add PortsBackend to service.go**

In `internal/network/service.go`, replace the `UniFiBackend` block:

```go
// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
	TopologyBackend
	SSIDsBackend
	VLANsBackend
	WANsBackend
	PortsBackend
}
```

- [ ] **Step 2: Create ports_service.go with PortsBackend interface and stub ListPorts**

Create `internal/network/ports_service.go`:

```go
package network

import (
	"context"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// PortsBackend is the narrow interface for port listing.
type PortsBackend interface {
	GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
	GetClients(ctx context.Context) ([]adapters.UniFiSta, error)
	GetNetworkConf(ctx context.Context) ([]adapters.UniFiNetworkConf, error)
}

// ListPorts returns all switch ports across all backends, filtered by params.
func (s *Service) ListPorts(ctx context.Context, params ListNetworkPortsParams) (NetworkPortList, error) {
	return NetworkPortList{Items: []NetworkPort{}}, nil
}
```

- [ ] **Step 3: Add stub ListNetworkPorts to handler.go**

In `internal/network/handler.go`, add after `GetNetworkTopology`:

```go
// ListNetworkPorts implements StrictServerInterface.
func (h *ServerHandler) ListNetworkPorts(ctx context.Context, request ListNetworkPortsRequestObject) (ListNetworkPortsResponseObject, error) {
	result, err := h.svc.ListPorts(ctx, request.Params)
	if err != nil {
		return ListNetworkPorts500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListNetworkPorts200JSONResponse(result), nil
}
```

- [ ] **Step 4: Verify the build passes**

```bash
make build
```

Expected: exits 0, binary written to `bin/server`.

---

## Task 3: Write failing tests for ListPorts

**Files:**
- Create: `internal/network/ports_service_test.go`

- [ ] **Step 1: Create the test file**

Create `internal/network/ports_service_test.go`:

```go
package network

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/testhelpers"
)

var errTest = errors.New("test error")

// mockMonitor implements adapters.AvailabilityChecker.
type mockMonitor struct{ available bool }

func (m *mockMonitor) Available(_ string) bool { return m.available }

// portsSvc builds a Service loaded with the full device+client+networkconf fixtures.
// 4 switches: us-8 (8 ports), switch-flex-mini (5), usw-flex-2-5g-8 (10), us-8-60w (12) = 35 total.
func portsSvc(t *testing.T) *Service {
	t.Helper()
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := testhelpers.LoadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	confs := testhelpers.LoadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	return NewService(
		map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients, networkConf: confs}},
		map[string]int{"unifi": 30},
		slog.Default(),
		nil,
	)
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int        { return &i }

func modePtr(m SwitchPortVlanMode) *SwitchPortVlanMode { return &m }
func statePtr(s NetworkPortState) *NetworkPortState     { return &s }

// --- no-filter ---

func TestListNetworkPorts_NoFilter(t *testing.T) {
	result, err := portsSvc(t).ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 35 {
		t.Fatalf("expected 35 ports, got %d", len(result.Items))
	}
	// each port must carry a switch ref
	for _, p := range result.Items {
		if p.Switch.Id == "" {
			t.Errorf("port %d: empty switch.id", p.Number)
		}
		if p.Switch.Kind != NetworkDeviceRefKindDevice {
			t.Errorf("port %d: expected switch.kind=device, got %s", p.Number, p.Switch.Kind)
		}
	}
}

func TestListNetworkPorts_SwitchRef(t *testing.T) {
	result, err := portsSvc(t).ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// count ports where switch.id == unifi.us-8-60w — fixture has 12
	var count int
	for _, p := range result.Items {
		if p.Switch.Id == "unifi.us-8-60w" {
			count++
		}
	}
	if count != 12 {
		t.Errorf("expected 12 ports for unifi.us-8-60w, got %d", count)
	}
}

// --- switchId filter ---

func TestListNetworkPorts_SwitchIdFilter(t *testing.T) {
	params := ListNetworkPortsParams{SwitchId: strPtr("unifi.us-8-60w")}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 12 {
		t.Fatalf("expected 12 ports for us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.Switch.Id != "unifi.us-8-60w" {
			t.Errorf("expected switch unifi.us-8-60w, got %s", p.Switch.Id)
		}
	}
}

// --- state filter ---

func TestListNetworkPorts_StateFilter_Up(t *testing.T) {
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		State:    statePtr(NetworkPortStateUp),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// us-8-60w has 7 up ports (1,5,6,7,8,11,12)
	if len(result.Items) != 7 {
		t.Fatalf("expected 7 up ports on us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.State != NetworkPortStateUp {
			t.Errorf("port %d: expected state=up, got %s", p.Number, p.State)
		}
	}
}

// --- mode filter ---

func TestListNetworkPorts_ModeFilter_Access(t *testing.T) {
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		Mode:     modePtr(Access),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// us-8-60w ports 2,3,4 have forward=native (access mode)
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 access ports on us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.VlanConfig == nil || p.VlanConfig.Mode != Access {
			t.Errorf("port %d: expected mode=access, got %+v", p.Number, p.VlanConfig)
		}
	}
}

func TestListNetworkPorts_ModeFilter_Trunk(t *testing.T) {
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		Mode:     modePtr(Trunk),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// us-8-60w: 8 forward=all + 1 forward=customize = 9 trunk ports
	if len(result.Items) != 9 {
		t.Fatalf("expected 9 trunk ports on us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.VlanConfig == nil || p.VlanConfig.Mode != Trunk {
			t.Errorf("port %d: expected mode=trunk", p.Number)
		}
	}
}

func TestListNetworkPorts_ModeFilter_NilVlanConfigExcluded(t *testing.T) {
	// Non-switch device ports (UAP, gateway) are excluded; switch ports with
	// vlanConfig=nil would also be excluded. Verify mode filter never returns
	// a port with nil vlanConfig.
	params := ListNetworkPortsParams{Mode: modePtr(Trunk)}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range result.Items {
		if p.VlanConfig == nil {
			t.Errorf("port %d on %s: mode filter returned port with nil vlanConfig", p.Number, p.Switch.Id)
		}
	}
}

// --- vlanId filter ---

func TestListNetworkPorts_VlanIdFilter_NativeMatch(t *testing.T) {
	// us-8-60w ports 2,3,4 are access ports with native VLAN = LAN-INT (vlanId=10).
	// Filtering mode=access + vlanId=10 on us-8-60w should return exactly those 3 ports.
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		Mode:     modePtr(Access),
		VlanId:   intPtr(10),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 ports with native vlanId=10 on us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.VlanConfig == nil || p.VlanConfig.NativeVlan.VlanId != 10 {
			t.Errorf("port %d: expected native vlanId=10, got %+v", p.Number, p.VlanConfig)
		}
	}
}

func TestListNetworkPorts_VlanIdFilter_TrunkAllMatchesAnyVlan(t *testing.T) {
	// Trunk-all ports carry every VLAN (scope=all).
	// us-8-60w has 8 trunk-all ports; filtering vlanId=100 + mode=trunk should include them
	// but exclude port 6 (trunk-custom whose tagged list does not contain vlan 100).
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		Mode:     modePtr(Trunk),
		VlanId:   intPtr(100),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 trunk-all ports match; port 6 (customize, tagged=[LAN-INT(10)], excluded=[LAN-SRV(100)]) does not
	if len(result.Items) != 8 {
		t.Fatalf("expected 8 trunk-all ports matching vlanId=100 on us-8-60w, got %d", len(result.Items))
	}
}

func TestListNetworkPorts_VlanIdFilter_TaggedCustomMatch(t *testing.T) {
	// Port 6 on us-8-60w is trunk-custom with tagged=[LAN-INT(vlanId=10)].
	// vlanId=10 + mode=trunk + state=up + switchId=us-8-60w should include port 6 along
	// with the 6 trunk-all up ports (1,5,7,8,11,12) = 7 total.
	params := ListNetworkPortsParams{
		SwitchId: strPtr("unifi.us-8-60w"),
		Mode:     modePtr(Trunk),
		State:    statePtr(NetworkPortStateUp),
		VlanId:   intPtr(10),
	}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 7 {
		t.Fatalf("expected 7 trunk up ports matching vlanId=10 on us-8-60w, got %d", len(result.Items))
	}
	// verify port 6 is present (tagged-custom match)
	var port6Found bool
	for _, p := range result.Items {
		if p.Number == 6 {
			port6Found = true
		}
	}
	if !port6Found {
		t.Error("expected port 6 (trunk-custom) in vlanId=10 results")
	}
}

// --- backend availability ---

func TestListNetworkPorts_BackendError(t *testing.T) {
	// When GetDevices returns an error, that backend is skipped; result is empty not an error.
	svc := NewService(
		map[string]UniFiBackend{"unifi": &mockUniFi{err: errTest}},
		map[string]int{"unifi": 30},
		slog.Default(),
		nil,
	)
	result, err := svc.ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 ports on backend error, got %d", len(result.Items))
	}
}

func TestListNetworkPorts_UnavailableBackend(t *testing.T) {
	devices := testhelpers.LoadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(
		map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}},
		map[string]int{"unifi": 30},
		slog.Default(),
		&mockMonitor{available: false},
	)
	result, err := svc.ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 ports when backend unavailable, got %d", len(result.Items))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/network/ -run TestListNetworkPorts -v 2>&1 | head -30
```

Expected: all `TestListNetworkPorts_*` tests FAIL (ListPorts returns empty list).

---

## Task 4: Implement ListPorts

**Files:**
- Modify: `internal/network/ports_service.go`

- [ ] **Step 1: Replace the stub with the full implementation**

Replace the contents of `internal/network/ports_service.go`:

```go
package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// PortsBackend is the narrow interface for port listing.
type PortsBackend interface {
	GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
	GetClients(ctx context.Context) ([]adapters.UniFiSta, error)
	GetNetworkConf(ctx context.Context) ([]adapters.UniFiNetworkConf, error)
}

// ListPorts returns all switch ports across all backends, filtered by params.
func (s *Service) ListPorts(ctx context.Context, params ListNetworkPortsParams) (NetworkPortList, error) {
	var items []NetworkPort
	for _, entry := range s.backends {
		if s.monitor != nil && !s.monitor.Available(entry.Name) {
			continue
		}
		devices, err := entry.Backend.GetDevices(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		clients, err := entry.Backend.GetClients(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		confs, err := entry.Backend.GetNetworkConf(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		swPortToDevice := buildSwPortToDevice(devices)
		swPortToClient := buildSwPortToClient(clients)
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
	}
	if items == nil {
		items = []NetworkPort{}
	}
	return NetworkPortList{Items: items}, nil
}

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

// matchesVlanID reports whether the given VLAN ID is carried by the port.
// Implements the three-way spec rule: native match OR tagged-custom item match OR trunk-all (scope=all).
// Returns false when cfg is nil (disabled / unresolvable ports).
func matchesVlanID(cfg *SwitchPortVlanConfig, vlanID int) bool {
	if cfg == nil {
		return false
	}
	if cfg.NativeVlan.VlanId == vlanID {
		return true
	}
	if cfg.TaggedVlans == nil {
		return false
	}
	if cfg.TaggedVlans.Scope == All {
		return true
	}
	if cfg.TaggedVlans.Items != nil {
		for _, v := range *cfg.TaggedVlans.Items {
			if v.VlanId == vlanID {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 2: Run the tests and confirm they pass**

```bash
go test ./internal/network/ -run TestListNetworkPorts -v 2>&1 | tail -20
```

Expected: all `TestListNetworkPorts_*` PASS.

- [ ] **Step 3: Run the full network test suite to check for regressions**

```bash
go test ./internal/network/ -v 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: `ok github.com/bwilczynski/homelab-api/internal/network` — no FAILs.

---

## Task 5: Build, run all tests, and commit

**Files:**
- No new changes — handler was already wired in Task 2 Step 3.

- [ ] **Step 1: Build the binary**

```bash
make build
```

Expected: exits 0, `bin/server` produced.

- [ ] **Step 2: Run the full test suite**

```bash
make test
```

Expected: `ok` for every package, no FAILs.

- [ ] **Step 3: Verify no api.gen.go files are staged**

```bash
git diff --cached --name-only | grep gen.go
```

Expected: no output. If any appear, run `git restore --staged internal/*/api.gen.go` before committing.

- [ ] **Step 4: Commit**

```bash
git add internal/network/ports_service.go internal/network/ports_service_test.go internal/network/service.go internal/network/handler.go
git commit -m "feat: implement GET /network/ports endpoint with server-side filters"
```
