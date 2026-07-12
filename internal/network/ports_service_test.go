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

func modePtr(m DevicePortVlanMode) *DevicePortVlanMode { return &m }
func statePtr(s NetworkPortState) *NetworkPortState     { return &s }

// --- no-filter ---

func TestListNetworkPorts_NoFilter(t *testing.T) {
	result, err := portsSvc(t).ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 40 {
		t.Fatalf("expected 40 ports (35 switch + 5 gateway LAN), got %d", len(result.Items))
	}
	// each port must carry a device ref
	for _, p := range result.Items {
		if p.Device.Id == "" {
			t.Errorf("port %d: empty device.id", p.Number)
		}
		if p.Device.Kind != NetworkDeviceRefKindDevice {
			t.Errorf("port %d: expected device.kind=device, got %s", p.Number, p.Device.Kind)
		}
	}
}

func TestListNetworkPorts_DeviceRef(t *testing.T) {
	result, err := portsSvc(t).ListPorts(context.Background(), ListNetworkPortsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// count ports where device.id == unifi.us-8-60w — fixture has 12
	var count int
	for _, p := range result.Items {
		if p.Device.Id == "unifi.us-8-60w" {
			count++
		}
	}
	if count != 12 {
		t.Errorf("expected 12 ports for unifi.us-8-60w, got %d", count)
	}
}

// --- deviceId filter ---

func TestListNetworkPorts_DeviceIdFilter(t *testing.T) {
	params := ListNetworkPortsParams{DeviceId: strPtr("unifi.us-8-60w")}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 12 {
		t.Fatalf("expected 12 ports for us-8-60w, got %d", len(result.Items))
	}
	for _, p := range result.Items {
		if p.Device.Id != "unifi.us-8-60w" {
			t.Errorf("expected device unifi.us-8-60w, got %s", p.Device.Id)
		}
	}
}

// --- state filter ---

func TestListNetworkPorts_StateFilter_Up(t *testing.T) {
	params := ListNetworkPortsParams{
		DeviceId: strPtr("unifi.us-8-60w"),
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
		DeviceId: strPtr("unifi.us-8-60w"),
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
		DeviceId: strPtr("unifi.us-8-60w"),
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
			t.Errorf("port %d on %s: mode filter returned port with nil vlanConfig", p.Number, p.Device.Id)
		}
	}
}

// --- vlanId filter ---

func TestListNetworkPorts_VlanIdFilter_NativeMatch(t *testing.T) {
	// us-8-60w ports 2,3,4 are access ports with native VLAN = LAN-INT (vlanId=10).
	// Filtering mode=access + vlanId=10 on us-8-60w should return exactly those 3 ports.
	params := ListNetworkPortsParams{
		DeviceId: strPtr("unifi.us-8-60w"),
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
		DeviceId: strPtr("unifi.us-8-60w"),
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
		DeviceId: strPtr("unifi.us-8-60w"),
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

// --- uplink connectedTo ---

// The uplink port surfaces in ListPorts with connectedTo pointing at the
// upstream device. Fixture: unifi.us-8 port 1 uplinks to unifi.us-8-60w.
func TestListNetworkPorts_UplinkPort_ConnectedToDevice(t *testing.T) {
	params := ListNetworkPortsParams{DeviceId: strPtr("unifi.us-8")}
	result, err := portsSvc(t).ListPorts(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var port1 *NetworkPort
	for i := range result.Items {
		if result.Items[i].Number == 1 {
			port1 = &result.Items[i]
			break
		}
	}
	if port1 == nil {
		t.Fatal("port 1 not found on unifi.us-8")
	}
	if port1.ConnectedTo == nil {
		t.Fatal("expected uplink port 1 connectedTo to be set")
	}
	ref, err := port1.ConnectedTo.AsNetworkDeviceRef()
	if err != nil {
		t.Fatalf("expected device ref on uplink port 1: %v", err)
	}
	if ref.Kind != NetworkDeviceRefKindDevice {
		t.Errorf("expected kind=device, got %s", ref.Kind)
	}
	if ref.Id != "unifi.us-8-60w" {
		t.Errorf("expected device id unifi.us-8-60w, got %s", ref.Id)
	}
}

// --- gateway ports ---

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
