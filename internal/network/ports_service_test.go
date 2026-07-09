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
