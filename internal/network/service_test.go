package network

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// mockUniFi implements UniFiBackend for testing.
type mockUniFi struct {
	devices        []adapters.UniFiDevice
	clients        []adapters.UniFiSta
	activeClients  []adapters.UniFiClientV2
	offlineClients []adapters.UniFiClientV2
	err            error
}

func (m *mockUniFi) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, m.err
}

func (m *mockUniFi) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, m.err
}

func (m *mockUniFi) GetActiveClients() ([]adapters.UniFiClientV2, error) {
	return m.activeClients, m.err
}

func (m *mockUniFi) GetOfflineClients(_ int) ([]adapters.UniFiClientV2, error) {
	return m.offlineClients, m.err
}

func (m *mockUniFi) GetAllClients(_ int) ([]adapters.UniFiClientV2, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append(m.activeClients, m.offlineClients...), nil
}

func loadFixture[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return envelope.Data
}

// --- device list tests ---

func TestListDevices(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	result, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 devices, got %d", len(result.Items))
	}

	gw := result.Items[0]
	if gw.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.usg-3p" {
		t.Errorf("expected uri /network/devices/unifi.usg-3p, got %s", gw.Uri)
	}
	if gw.Type != NetworkDeviceTypeGateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != Connected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}

	sw := result.Items[1]
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Uri != "/network/devices/unifi.us-8-60w" {
		t.Errorf("expected uri /network/devices/unifi.us-8-60w, got %s", sw.Uri)
	}
	if sw.Type != NetworkDeviceTypeSwitch {
		t.Errorf("expected type switch, got %s", sw.Type)
	}

	ap := result.Items[3]
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Type != NetworkDeviceTypeAccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}

	offline := result.Items[4]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected, got %s", offline.Status)
	}
}

func TestListDevicesEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: []adapters.UniFiDevice{}}}, 30)
	result, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(result.Items))
	}
}

// --- device detail tests ---

func TestGetDevice_Gateway(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.usg-3p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected device to be found")
	}

	gw, err := detail.AsGatewayDetail()
	if err != nil {
		t.Fatalf("expected gateway detail: %v", err)
	}
	if gw.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", gw.Id)
	}
	if gw.Uri != "/network/devices/unifi.usg-3p" {
		t.Errorf("expected uri, got %s", gw.Uri)
	}
	if gw.Model != "UGW3" {
		t.Errorf("expected model UGW3, got %s", gw.Model)
	}
	if gw.FirmwareVersion != "4.4.57.5578372" {
		t.Errorf("expected version 4.4.57.5578372, got %s", gw.FirmwareVersion)
	}
	if gw.Uptime != 16066061 {
		t.Errorf("expected uptime 16066061, got %d", gw.Uptime)
	}
	if gw.Uplink != nil {
		t.Errorf("expected nil uplink for gateway, got %v", gw.Uplink)
	}
	if gw.Traffic.TxBytesTotal != 306168680348 {
		t.Errorf("expected tx_bytes 306168680348, got %d", gw.Traffic.TxBytesTotal)
	}
	if gw.Traffic.RxBytesPerSec != 134932 {
		t.Errorf("expected rx_bytes-r 134932, got %d", gw.Traffic.RxBytesPerSec)
	}
}

func TestGetDevice_Unknown(t *testing.T) {
	devices := []adapters.UniFiDevice{{
		ID: "x", MAC: "bb:bb:bb:bb:bb:01", Name: "Weird Box", Model: "WB1",
		Type: "weird", State: 1, IP: "192.168.1.99", Version: "1.0", Uptime: 1000,
		TxBytes: 100, RxBytes: 200,
	}}
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.weird-box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected unknown device to be found")
	}

	u, err := detail.AsUnknownDeviceDetail()
	if err != nil {
		t.Fatalf("expected unknown detail: %v", err)
	}
	if u.Id != "unifi.weird-box" {
		t.Errorf("expected id unifi.weird-box, got %s", u.Id)
	}
	if u.Model != "WB1" {
		t.Errorf("expected model WB1, got %s", u.Model)
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	_, found, err := svc.GetDevice(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected device not to be found")
	}
}

func TestGetDeviceWrongController(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	_, found, err := svc.GetDevice(context.Background(), "other.usg-3p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found for wrong controller")
	}
}

// --- client list tests ---

func TestListClientsAll(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 clients (3 active + 2 offline), got %d", len(result.Items))
	}
}

func TestListClientsOnlineFilter(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "online")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 online clients, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != Online {
			t.Errorf("expected status online, got %s", item.Status)
		}
	}
}

func TestListClientsOfflineFilter(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "offline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 offline clients, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != Offline {
			t.Errorf("expected status offline, got %s", item.Status)
		}
	}
}

func TestListClientsIDAndFields(t *testing.T) {
	active := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-active.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{activeClients: active, offlineClients: offline}}, 30)

	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byID := make(map[string]NetworkClient, len(result.Items))
	for _, item := range result.Items {
		byID[item.Id] = item
	}

	// Online wireless with user alias — ID uses alias, not display_name
	mb, ok := byID["unifi.macbook-pro-3c"]
	if !ok {
		t.Fatal("expected unifi.macbook-pro-3c")
	}
	if mb.Name != "MacBook Pro" {
		t.Errorf("expected name MacBook Pro, got %s", mb.Name)
	}
	if mb.ConnectionType != NetworkClientConnectionTypeWireless {
		t.Errorf("expected wireless, got %s", mb.ConnectionType)
	}
	if mb.Status != Online {
		t.Errorf("expected online, got %s", mb.Status)
	}
	if mb.Ip == nil || *mb.Ip != "192.168.10.67" {
		t.Errorf("expected ip 192.168.10.67, got %v", mb.Ip)
	}

	// Online wired with hostname only (no user alias)
	nas, ok := byID["unifi.nas-1-68"]
	if !ok {
		t.Fatal("expected unifi.nas-1-68")
	}
	if nas.ConnectionType != NetworkClientConnectionTypeWired {
		t.Errorf("expected wired, got %s", nas.ConnectionType)
	}

	// Offline wireless — ip comes from last_ip
	kindle, ok := byID["unifi.kindle-paperwhite-e0"]
	if !ok {
		t.Fatal("expected unifi.kindle-paperwhite-e0")
	}
	if kindle.Status != Offline {
		t.Errorf("expected offline, got %s", kindle.Status)
	}
	if kindle.Ip == nil || *kindle.Ip != "192.168.10.37" {
		t.Errorf("expected last_ip 192.168.10.37, got %v", kindle.Ip)
	}

	// Offline wired with hostname only
	host, ok := byID["unifi.host-02-aa"]
	if !ok {
		t.Fatal("expected unifi.host-02-aa")
	}
	if host.ConnectionType != NetworkClientConnectionTypeWired {
		t.Errorf("expected wired, got %s", host.ConnectionType)
	}

	// Verify uri on list items
	if mb.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected uri /network/clients/unifi.macbook-pro-3c, got %s", mb.Uri)
	}
	if nas.Uri != "/network/clients/unifi.nas-1-68" {
		t.Errorf("expected uri /network/clients/unifi.nas-1-68, got %s", nas.Uri)
	}
}

func TestListClientsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		activeClients:  []adapters.UniFiClientV2{},
		offlineClients: []adapters.UniFiClientV2{},
	}}, 30)
	result, err := svc.ListClients(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 clients, got %d", len(result.Items))
	}
}

// --- client detail tests ---

func TestGetClientWireless(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.macbook-pro-3c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wireless, err := detail.AsWirelessNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wireless detail: %v", err)
	}
	if wireless.ConnectedTo.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", wireless.ConnectedTo.Ssid)
	}
	if wireless.ConnectedTo.SignalStrength == nil || *wireless.ConnectedTo.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %v", wireless.ConnectedTo.SignalStrength)
	}
	if wireless.ConnectedTo.Device.Id != "unifi.uap-01" {
		t.Errorf("expected device unifi.uap-01, got %s", wireless.ConnectedTo.Device.Id)
	}
	if wireless.ConnectedTo.Device.Uri != "/network/devices/unifi.uap-01" {
		t.Errorf("expected device uri, got %s", wireless.ConnectedTo.Device.Uri)
	}
	if wireless.Uptime == nil || *wireless.Uptime != 27075 {
		t.Errorf("expected uptime 27075, got %v", wireless.Uptime)
	}
	if wireless.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected uri, got %s", wireless.Uri)
	}
}

func TestGetClientWired(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.nas-1-68")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wired, err := detail.AsWiredNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wired detail: %v", err)
	}
	if wired.ConnectedTo.Device.Id != "unifi.us-8-60w" {
		t.Errorf("expected device unifi.us-8-60w, got %s", wired.ConnectedTo.Device.Id)
	}
	if wired.ConnectedTo.Port == nil || *wired.ConnectedTo.Port != 3 {
		t.Errorf("expected port 3, got %v", wired.ConnectedTo.Port)
	}
	if wired.ConnectedTo.LinkSpeed == nil || *wired.ConnectedTo.LinkSpeed != "gbe1" {
		t.Errorf("expected link speed gbe1, got %v", wired.ConnectedTo.LinkSpeed)
	}
	if wired.Uptime == nil || *wired.Uptime != 1024199 {
		t.Errorf("expected uptime 1024199, got %v", wired.Uptime)
	}
}

func TestGetClientNotFound(t *testing.T) {
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{clients: clients}}, 30)

	_, found, err := svc.GetClient(context.Background(), "unifi.nobody-00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected client not to be found")
	}
}

func TestGetClientOfflineWired(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		devices:        devices,
		clients:        []adapters.UniFiSta{},
		offlineClients: offline,
	}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.host-02-aa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected offline wired client to be found")
	}

	wired, err := detail.AsWiredNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wired detail: %v", err)
	}
	if wired.Status != Offline {
		t.Errorf("expected offline, got %s", wired.Status)
	}
	if wired.ConnectedTo.Device.Id != "unifi.switch-flex-mini" {
		t.Errorf("expected device unifi.switch-flex-mini, got %s", wired.ConnectedTo.Device.Id)
	}
	if wired.ConnectedTo.Device.Name != "Switch Flex Mini" {
		t.Errorf("expected name Switch Flex Mini, got %s", wired.ConnectedTo.Device.Name)
	}
	if wired.ConnectedTo.Port != nil {
		t.Errorf("expected nil port for offline client, got %v", wired.ConnectedTo.Port)
	}
	if wired.ConnectedTo.LinkSpeed != nil {
		t.Errorf("expected nil linkSpeed for offline client, got %v", wired.ConnectedTo.LinkSpeed)
	}
	if wired.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wired.Uptime)
	}
}

func TestGetClientOfflineWireless(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	offline := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		devices:        devices,
		clients:        []adapters.UniFiSta{},
		offlineClients: offline,
	}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.kindle-paperwhite-e0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected offline wireless client to be found")
	}

	wireless, err := detail.AsWirelessNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wireless detail: %v", err)
	}
	if wireless.Status != Offline {
		t.Errorf("expected offline, got %s", wireless.Status)
	}
	if wireless.ConnectedTo.Device.Id != "unifi.uap-01" {
		t.Errorf("expected device unifi.uap-01, got %s", wireless.ConnectedTo.Device.Id)
	}
	if wireless.ConnectedTo.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", wireless.ConnectedTo.Ssid)
	}
	if wireless.ConnectedTo.SignalStrength != nil {
		t.Errorf("expected nil signalStrength for offline, got %v", wireless.ConnectedTo.SignalStrength)
	}
	if wireless.Ip == nil || *wireless.Ip != "192.168.10.37" {
		t.Errorf("expected ip 192.168.10.37, got %v", wireless.Ip)
	}
	if wireless.Uptime != nil {
		t.Errorf("expected nil uptime for offline client, got %v", wireless.Uptime)
	}
}

func TestGetClientNotFoundInEither(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		clients:        []adapters.UniFiSta{},
		offlineClients: []adapters.UniFiClientV2{},
	}}, 30)

	_, found, err := svc.GetClient(context.Background(), "unifi.nobody-00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}

// --- helper unit tests ---

func TestToKebab(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"USG 3P", "usg-3p"},
		{"AP Living Room", "ap-living-room"},
		{"Switch Flex Mini", "switch-flex-mini"},
		{"US 8 60W", "us-8-60w"},
		{"UAP-01", "uap-01"},
	}
	for _, tt := range tests {
		got := toKebab(tt.input)
		if got != tt.want {
			t.Errorf("toKebab(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetDevice_Switch(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected switch to be found")
	}

	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Model != "US8P60" {
		t.Errorf("expected model US8P60, got %s", sw.Model)
	}
	if len(sw.Ports) != 8 {
		t.Fatalf("expected 8 ports, got %d", len(sw.Ports))
	}
	p1 := sw.Ports[0]
	if p1.Number != 1 {
		t.Errorf("expected port number 1, got %d", p1.Number)
	}
	if p1.State != "up" {
		t.Errorf("expected port 1 up, got %s", p1.State)
	}
	if p1.LinkSpeed == nil || *p1.LinkSpeed != "gbe1" {
		t.Errorf("expected link speed gbe1, got %v", p1.LinkSpeed)
	}
	if p1.PoeMode != "off" {
		t.Errorf("expected poe mode off, got %s", p1.PoeMode)
	}
	if p1.PoePowerWatts != nil {
		t.Errorf("expected nil poe power for non-poe port, got %v", p1.PoePowerWatts)
	}
	p4 := sw.Ports[3]
	if p4.State != "down" {
		t.Errorf("expected port 4 down, got %s", p4.State)
	}
	if p4.LinkSpeed != nil {
		t.Errorf("expected nil link speed for down port, got %v", p4.LinkSpeed)
	}
	p5 := sw.Ports[4]
	if p5.PoeMode != "auto" {
		t.Errorf("expected poe auto, got %s", p5.PoeMode)
	}
	if p5.PoePowerWatts == nil || *p5.PoePowerWatts != 3.00 {
		t.Errorf("expected poe power 3.00, got %v", p5.PoePowerWatts)
	}
	if p1.Traffic.TxBytesTotal != 25312100378 {
		t.Errorf("expected port 1 tx_bytes 25312100378, got %d", p1.Traffic.TxBytesTotal)
	}
	if sw.Traffic.TxBytesTotal != 226683708402 {
		t.Errorf("expected device tx_bytes 226683708402, got %d", sw.Traffic.TxBytesTotal)
	}
	if sw.Uplink == nil {
		t.Fatal("expected uplink for switch")
	}
	if sw.Uplink.Device.Id != "unifi.usg-3p" {
		t.Errorf("expected uplink device unifi.usg-3p, got %s", sw.Uplink.Device.Id)
	}
}

func TestGetDevice_SwitchPort_ConnectedToDevice(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, _, _ := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}

	var port5 *SwitchPort
	for i := range sw.Ports {
		if sw.Ports[i].Number == 5 {
			port5 = &sw.Ports[i]
			break
		}
	}
	if port5 == nil {
		t.Fatal("port 5 not found")
	}
	if port5.ConnectedTo == nil {
		t.Fatal("expected port 5 connectedTo to be set")
	}
	ref, err := port5.ConnectedTo.AsNetworkDeviceRef()
	if err != nil {
		t.Fatalf("expected device ref on port 5: %v", err)
	}
	if ref.Kind != NetworkDeviceRefKindDevice {
		t.Errorf("expected kind=device, got %s", ref.Kind)
	}
	if ref.Id != "unifi.uap-01" {
		t.Errorf("expected device id unifi.uap-01, got %s", ref.Id)
	}
	if ref.Uri != "/network/devices/unifi.uap-01" {
		t.Errorf("expected device uri, got %s", ref.Uri)
	}
	if ref.Name != "UAP-01" {
		t.Errorf("expected device name UAP-01, got %s", ref.Name)
	}
}

func TestGetDevice_SwitchPort_ConnectedToClient(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, _, _ := svc.GetDevice(context.Background(), "unifi.us-8-60w")
	sw, err := detail.AsSwitchDetail()
	if err != nil {
		t.Fatalf("expected switch detail: %v", err)
	}

	var port3 *SwitchPort
	for i := range sw.Ports {
		if sw.Ports[i].Number == 3 {
			port3 = &sw.Ports[i]
			break
		}
	}
	if port3 == nil {
		t.Fatal("port 3 not found")
	}
	if port3.ConnectedTo == nil {
		t.Fatal("expected port 3 connectedTo to be set")
	}
	ref, err := port3.ConnectedTo.AsNetworkClientRef()
	if err != nil {
		t.Fatalf("expected client ref on port 3: %v", err)
	}
	if ref.Kind != NetworkClientRefKindClient {
		t.Errorf("expected kind=client, got %s", ref.Kind)
	}
	if ref.Id != "unifi.nas-1-68" {
		t.Errorf("expected client id unifi.nas-1-68, got %s", ref.Id)
	}
}

func TestGetDevice_AccessPoint(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, clients: clients}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.uap-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected AP to be found")
	}

	ap, err := detail.AsAccessPointDetail()
	if err != nil {
		t.Fatalf("expected access point detail: %v", err)
	}
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Model != "U7LT" {
		t.Errorf("expected model U7LT, got %s", ap.Model)
	}
	if len(ap.ConnectedClients) != 3 {
		t.Fatalf("expected 3 connectedClients, got %d", len(ap.ConnectedClients))
	}
	var mb *AccessPointClient
	for i := range ap.ConnectedClients {
		if ap.ConnectedClients[i].Client.Name == "MacBook Pro" {
			mb = &ap.ConnectedClients[i]
			break
		}
	}
	if mb == nil {
		t.Fatal("MacBook Pro not found in connectedClients")
	}
	if mb.Client.Id != "unifi.macbook-pro-3c" {
		t.Errorf("expected client id unifi.macbook-pro-3c, got %s", mb.Client.Id)
	}
	if mb.Client.Uri != "/network/clients/unifi.macbook-pro-3c" {
		t.Errorf("expected client uri, got %s", mb.Client.Uri)
	}
	if mb.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", mb.Ssid)
	}
	if mb.SignalStrength != -69 {
		t.Errorf("expected signal strength -69, got %d", mb.SignalStrength)
	}
	if ap.Uplink == nil {
		t.Fatal("expected uplink for AP")
	}
	if ap.Uplink.Device.Id != "unifi.us-8-60w" {
		t.Errorf("expected uplink device unifi.us-8-60w, got %s", ap.Uplink.Device.Id)
	}
	if ap.Uplink.Port == nil || *ap.Uplink.Port != 5 {
		t.Errorf("expected uplink port 5, got %v", ap.Uplink.Port)
	}
}

func TestMapDeviceType(t *testing.T) {
	tests := []struct {
		input string
		want  NetworkDeviceType
	}{
		{"uap", NetworkDeviceTypeAccessPoint},
		{"usw", NetworkDeviceTypeSwitch},
		{"ugw", NetworkDeviceTypeGateway},
		{"udm", NetworkDeviceTypeGateway},
		{"udm-pro", NetworkDeviceTypeGateway},
		{"other", NetworkDeviceTypeUnknown},
		{"", NetworkDeviceTypeUnknown},
	}
	for _, tt := range tests {
		got := mapDeviceType(tt.input)
		if got != tt.want {
			t.Errorf("mapDeviceType(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}
