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
	wlanConf       []adapters.UniFiWlanConf
	networkConf    []adapters.UniFiNetworkConf
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

func (m *mockUniFi) GetWlanConf() ([]adapters.UniFiWlanConf, error) {
	return m.wlanConf, m.err
}

func (m *mockUniFi) GetNetworkConf() ([]adapters.UniFiNetworkConf, error) {
	return m.networkConf, m.err
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
	if gw.Status != NetworkDeviceStatusConnected {
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
	if offline.Status != NetworkDeviceStatusDisconnected {
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

// --- topology tests ---

// testNode and testEdge are thin structs for inspecting union topology types via JSON round-trip.
type testNode struct {
	Kind           string `json:"kind"`
	Id             string `json:"id"`
	Type           string `json:"type,omitempty"`
	Status         string `json:"status,omitempty"`
	NumClients     *int   `json:"numClients,omitempty"`
	ConnectionType string `json:"connectionType,omitempty"`
}

type testEdge struct {
	Kind   string `json:"kind"`
	Source struct {
		Kind string `json:"kind"`
		Id   string `json:"id"`
	} `json:"source"`
	Target struct {
		Id string `json:"id"`
	} `json:"target"`
	Port           *int    `json:"port,omitempty"`
	LinkSpeed      *string `json:"linkSpeed,omitempty"`
	Ssid           *string `json:"ssid,omitempty"`
	SignalStrength  *int    `json:"signalStrength,omitempty"`
}

func parseTopologyNode(t *testing.T, n TopologyNode) testNode {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal node: %v", err)
	}
	var tn testNode
	if err := json.Unmarshal(b, &tn); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	return tn
}

func parseTopologyEdge(t *testing.T, e TopologyEdge) testEdge {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal edge: %v", err)
	}
	var te testEdge
	if err := json.Unmarshal(b, &te); err != nil {
		t.Fatalf("unmarshal edge: %v", err)
	}
	return te
}

func TestGetTopology_DevicesOnly(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	topo, err := svc.GetTopology(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 device nodes, 0 client nodes
	if len(topo.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(topo.Nodes))
	}

	// 3 device-device edges; UAP-02 (offline, no uplink) contributes none
	if len(topo.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(topo.Edges))
	}

	// all nodes are kind=device
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Kind != "device" {
			t.Errorf("expected kind=device, got %s (id=%s)", pn.Kind, pn.Id)
		}
	}

	// gateway node: type=gateway, no outgoing edge
	gatewayCount := 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Type == "gateway" {
			gatewayCount++
		}
	}
	if gatewayCount != 1 {
		t.Errorf("expected 1 gateway node, got %d", gatewayCount)
	}
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.usg-3p" {
			t.Error("gateway should not be an edge source")
		}
	}

	// AP node numClients: UAP-01 has user_num_sta=7; UAP-02 has 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Id == "unifi.uap-01" {
			if pn.NumClients == nil || *pn.NumClients != 7 {
				t.Errorf("UAP-01: expected numClients=7, got %v", pn.NumClients)
			}
		}
		if pn.Id == "unifi.uap-02" {
			if pn.NumClients == nil || *pn.NumClients != 0 {
				t.Errorf("UAP-02: expected numClients=0, got %v", pn.NumClients)
			}
		}
		// switches and gateway must not have numClients
		if pn.Type == "switch" || pn.Type == "gateway" {
			if pn.NumClients != nil {
				t.Errorf("%s (%s): expected no numClients, got %d", pn.Id, pn.Type, *pn.NumClients)
			}
		}
	}

	// edge UAP-01 → US 8 60W: wired, port=5, linkSpeed=gbe1
	var uap01Edge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.uap-01" {
			uap01Edge = &pe
			break
		}
	}
	if uap01Edge == nil {
		t.Fatal("expected edge with source=unifi.uap-01")
	}
	if uap01Edge.Target.Id != "unifi.us-8-60w" {
		t.Errorf("UAP-01 edge target: expected unifi.us-8-60w, got %s", uap01Edge.Target.Id)
	}
	if uap01Edge.Port == nil || *uap01Edge.Port != 5 {
		t.Errorf("UAP-01 edge port: expected 5, got %v", uap01Edge.Port)
	}
	if uap01Edge.LinkSpeed == nil || *uap01Edge.LinkSpeed != "gbe1" {
		t.Errorf("UAP-01 edge linkSpeed: expected gbe1, got %v", uap01Edge.LinkSpeed)
	}

	// edge Switch Flex Mini → US 8 60W: wired, port=6, linkSpeed=gbe1
	var flexEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.switch-flex-mini" {
			flexEdge = &pe
			break
		}
	}
	if flexEdge == nil {
		t.Fatal("expected edge with source=unifi.switch-flex-mini")
	}
	if flexEdge.Port == nil || *flexEdge.Port != 6 {
		t.Errorf("Flex Mini edge port: expected 6, got %v", flexEdge.Port)
	}

	// edge US 8 60W → USG: wired, port=nil (uplink_remote_port is null in fixture), linkSpeed=gbe1
	var us8Edge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.us-8-60w" {
			us8Edge = &pe
			break
		}
	}
	if us8Edge == nil {
		t.Fatal("expected edge with source=unifi.us-8-60w")
	}
	if us8Edge.Target.Id != "unifi.usg-3p" {
		t.Errorf("US8 edge target: expected unifi.usg-3p, got %s", us8Edge.Target.Id)
	}
	if us8Edge.Port != nil {
		t.Errorf("US8 edge port: expected nil (no uplink_remote_port in fixture), got %d", *us8Edge.Port)
	}
	if us8Edge.LinkSpeed == nil || *us8Edge.LinkSpeed != "gbe1" {
		t.Errorf("US8 edge linkSpeed: expected gbe1, got %v", us8Edge.LinkSpeed)
	}
}

func TestGetTopology_WithClients(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	history := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")

	mock := &mockUniFi{devices: devices, clients: clients, offlineClients: history}
	svc := NewService(map[string]UniFiBackend{"unifi": mock}, 30)

	topo, err := svc.GetTopology(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 device + 5 online client + 2 offline client = 12 nodes
	if len(topo.Nodes) != 12 {
		t.Fatalf("expected 12 nodes, got %d", len(topo.Nodes))
	}

	// 3 device-device + 5 online-client + 2 offline-client = 10 edges
	if len(topo.Edges) != 10 {
		t.Fatalf("expected 10 edges, got %d", len(topo.Edges))
	}

	// Verify node kinds: 5 device + 7 client
	deviceNodes, clientNodes := 0, 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		switch pn.Kind {
		case "device":
			deviceNodes++
		case "client":
			clientNodes++
		default:
			t.Errorf("unexpected node kind: %s", pn.Kind)
		}
	}
	if deviceNodes != 5 {
		t.Errorf("expected 5 device nodes, got %d", deviceNodes)
	}
	if clientNodes != 7 {
		t.Errorf("expected 7 client nodes (5 online + 2 offline), got %d", clientNodes)
	}

	// Online wired edge: nas-1 → US 8 60W, port=3, linkSpeed=gbe1
	var nasEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wired" && pe.Source.Kind == "client" && pe.Source.Id == "unifi.nas-1-68" {
			nasEdge = &pe
			break
		}
	}
	if nasEdge == nil {
		t.Fatal("expected wired edge for nas-1")
	}
	if nasEdge.Target.Id != "unifi.us-8-60w" {
		t.Errorf("nas-1 edge target: expected unifi.us-8-60w, got %s", nasEdge.Target.Id)
	}
	if nasEdge.Port == nil || *nasEdge.Port != 3 {
		t.Errorf("nas-1 edge port: expected 3, got %v", nasEdge.Port)
	}
	if nasEdge.LinkSpeed == nil || *nasEdge.LinkSpeed != "gbe1" {
		t.Errorf("nas-1 edge linkSpeed: expected gbe1, got %v", nasEdge.LinkSpeed)
	}

	// Online wired edge: ec:b5:fa → UAP-01, port=6, linkSpeed=fe (100Mbps)
	var ecEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wired" && pe.Source.Kind == "client" && pe.Source.Id == "unifi.ec-b5-fa-22-d1-dc-ec" {
			ecEdge = &pe
			break
		}
	}
	if ecEdge == nil {
		t.Fatal("expected wired edge for ec:b5:fa client")
	}
	if ecEdge.Target.Id != "unifi.uap-01" {
		t.Errorf("ec:b5:fa edge target: expected unifi.uap-01, got %s", ecEdge.Target.Id)
	}
	if ecEdge.Port == nil || *ecEdge.Port != 6 {
		t.Errorf("ec:b5:fa edge port: expected 6, got %v", ecEdge.Port)
	}
	if ecEdge.LinkSpeed == nil || *ecEdge.LinkSpeed != "fe" {
		t.Errorf("ec:b5:fa edge linkSpeed: expected fe, got %v", ecEdge.LinkSpeed)
	}

	// Online wireless edge: MacBook Pro → UAP-01, ssid=homelab, signalStrength=-69
	var macbookEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wireless" && pe.Source.Id == "unifi.macbook-pro-3c" {
			macbookEdge = &pe
			break
		}
	}
	if macbookEdge == nil {
		t.Fatal("expected wireless edge for MacBook Pro")
	}
	if macbookEdge.Target.Id != "unifi.uap-01" {
		t.Errorf("MacBook Pro edge target: expected unifi.uap-01, got %s", macbookEdge.Target.Id)
	}
	if macbookEdge.Ssid == nil || *macbookEdge.Ssid != "homelab" {
		t.Errorf("MacBook Pro edge ssid: expected homelab, got %v", macbookEdge.Ssid)
	}
	if macbookEdge.SignalStrength == nil || *macbookEdge.SignalStrength != -69 {
		t.Errorf("MacBook Pro edge signalStrength: expected -69, got %v", macbookEdge.SignalStrength)
	}

	// Offline wireless edge: Kindle Paperwhite → UAP-01, ssid=homelab, no signalStrength
	var kindleEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wireless" && pe.Source.Id == "unifi.kindle-paperwhite-e0" {
			kindleEdge = &pe
			break
		}
	}
	if kindleEdge == nil {
		t.Fatal("expected wireless edge for Kindle Paperwhite")
	}
	if kindleEdge.Target.Id != "unifi.uap-01" {
		t.Errorf("Kindle edge target: expected unifi.uap-01, got %s", kindleEdge.Target.Id)
	}
	if kindleEdge.Ssid == nil || *kindleEdge.Ssid != "homelab" {
		t.Errorf("Kindle edge ssid: expected homelab, got %v", kindleEdge.Ssid)
	}
	if kindleEdge.SignalStrength != nil {
		t.Errorf("Kindle (offline): expected no signalStrength, got %d", *kindleEdge.SignalStrength)
	}

	// Offline wired edge: host-02 aa → Switch Flex Mini, no port
	// ID uses clientNameV2 which returns hostname "host-02" (display_name is ignored), prefix "aa"
	var hostEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wired" && pe.Source.Id == "unifi.host-02-aa" {
			hostEdge = &pe
			break
		}
	}
	if hostEdge == nil {
		t.Fatal("expected wired edge for host-02 aa")
	}
	if hostEdge.Target.Id != "unifi.switch-flex-mini" {
		t.Errorf("host-02 edge target: expected unifi.switch-flex-mini, got %s", hostEdge.Target.Id)
	}
	if hostEdge.Port != nil {
		t.Errorf("host-02 (offline): expected no port, got %d", *hostEdge.Port)
	}

	// Offline client nodes have status=offline; online client nodes have status=online
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Kind != "client" {
			continue
		}
		switch pn.Id {
		case "unifi.kindle-paperwhite-e0", "unifi.host-02-aa":
			if pn.Status != "offline" {
				t.Errorf("%s: expected status=offline, got %s", pn.Id, pn.Status)
			}
		default:
			if pn.Status != "online" {
				t.Errorf("%s: expected status=online, got %s", pn.Id, pn.Status)
			}
		}
	}
}

// --- VLAN list tests ---

func TestListVLANs(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	result, err := svc.ListVLANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 LAN networks; WAN entry is excluded
	if len(result.Items) != 4 {
		t.Fatalf("expected 4 VLANs, got %d", len(result.Items))
	}

	byID := make(map[string]Vlan)
	for _, v := range result.Items {
		byID[v.Id] = v
	}

	mgmt, ok := byID["unifi.lan-mgmt"]
	if !ok {
		t.Fatal("expected unifi.lan-mgmt")
	}
	if mgmt.VlanId != 1 {
		t.Errorf("LAN-MGMT: expected vlanId 1 (untagged), got %d", mgmt.VlanId)
	}
	if mgmt.Subnet != "192.168.1.0/24" {
		t.Errorf("LAN-MGMT: expected subnet 192.168.1.0/24, got %s", mgmt.Subnet)
	}
	if mgmt.Uri != "/network/vlans/unifi.lan-mgmt" {
		t.Errorf("LAN-MGMT: expected uri /network/vlans/unifi.lan-mgmt, got %s", mgmt.Uri)
	}

	iot, ok := byID["unifi.lan-iot"]
	if !ok {
		t.Fatal("expected unifi.lan-iot")
	}
	if iot.VlanId != 20 {
		t.Errorf("LAN-IOT: expected vlanId 20, got %d", iot.VlanId)
	}
	if iot.Subnet != "192.168.20.0/24" {
		t.Errorf("LAN-IOT: expected subnet 192.168.20.0/24, got %s", iot.Subnet)
	}

	srv, ok := byID["unifi.lan-srv"]
	if !ok {
		t.Fatal("expected unifi.lan-srv")
	}
	if srv.VlanId != 100 {
		t.Errorf("LAN-SRV: expected vlanId 100, got %d", srv.VlanId)
	}
}

func TestListVLANsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: []adapters.UniFiNetworkConf{}}}, 30)
	result, err := svc.ListVLANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 VLANs, got %d", len(result.Items))
	}
}

// --- VLAN detail tests ---

func TestGetVLAN_ServerDHCP(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-iot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-IOT to be found")
	}
	if detail.Id != "unifi.lan-iot" {
		t.Errorf("expected id unifi.lan-iot, got %s", detail.Id)
	}
	if detail.VlanId != 20 {
		t.Errorf("expected vlanId 20, got %d", detail.VlanId)
	}
	if detail.Subnet != "192.168.20.0/24" {
		t.Errorf("expected subnet 192.168.20.0/24, got %s", detail.Subnet)
	}
	if detail.GatewayIp != "192.168.20.1" {
		t.Errorf("expected gatewayIp 192.168.20.1, got %s", detail.GatewayIp)
	}
	if detail.BroadcastIp != "192.168.20.255" {
		t.Errorf("expected broadcastIp 192.168.20.255, got %s", detail.BroadcastIp)
	}
	if detail.DhcpMode != DhcpModeServer {
		t.Errorf("expected dhcpMode server, got %s", detail.DhcpMode)
	}
	if detail.DhcpRange == nil {
		t.Fatal("expected dhcpRange to be set")
	}
	if detail.DhcpRange.Start != "192.168.20.6" {
		t.Errorf("expected dhcpRange.start 192.168.20.6, got %s", detail.DhcpRange.Start)
	}
	if detail.DhcpRange.End != "192.168.20.254" {
		t.Errorf("expected dhcpRange.end 192.168.20.254, got %s", detail.DhcpRange.End)
	}
	if len(detail.DnsServers) != 1 || detail.DnsServers[0] != "192.168.20.1" {
		t.Errorf("expected dnsServers [192.168.20.1], got %v", detail.DnsServers)
	}
}

func TestGetVLAN_RelayDHCP(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		networkConf: []adapters.UniFiNetworkConf{
			{
				ID:               "bb30",
				Name:             "LAN-RELAY",
				Purpose:          "corporate",
				Vlan:             float64(30),
				VlanEnabled:      true,
				IPSubnet:         "192.168.30.1/24",
				DHCPRelayEnabled: true,
				DhcpdEnabled:     false,
			},
		},
	}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-relay")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-RELAY to be found")
	}
	if detail.DhcpMode != DhcpModeRelay {
		t.Errorf("expected dhcpMode relay, got %s", detail.DhcpMode)
	}
}

func TestGetVLAN_MultipleDNS(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-int")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-INT to be found")
	}
	if len(detail.DnsServers) != 2 {
		t.Fatalf("expected 2 DNS servers, got %d", len(detail.DnsServers))
	}
	if detail.DnsServers[0] != "192.168.100.5" || detail.DnsServers[1] != "192.168.10.1" {
		t.Errorf("unexpected DNS servers: %v", detail.DnsServers)
	}
}

func TestGetVLAN_NullDNS(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-srv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-SRV to be found")
	}
	if len(detail.DnsServers) != 0 {
		t.Errorf("expected empty dnsServers for LAN-SRV, got %v", detail.DnsServers)
	}
}

func TestGetVLAN_UntaggedVLAN1(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-mgmt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-MGMT to be found")
	}
	if detail.VlanId != 1 {
		t.Errorf("expected vlanId 1 for untagged, got %d", detail.VlanId)
	}
	if detail.GatewayIp != "192.168.1.1" {
		t.Errorf("expected gatewayIp 192.168.1.1, got %s", detail.GatewayIp)
	}
	if detail.BroadcastIp != "192.168.1.255" {
		t.Errorf("expected broadcastIp 192.168.1.255, got %s", detail.BroadcastIp)
	}
}

func TestGetVLANNotFound(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	_, found, err := svc.GetVLAN(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
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
