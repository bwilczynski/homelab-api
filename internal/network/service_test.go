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

	// Gateway: ID uses kebab name, list fields only
	gw := result.Items[0]
	if gw.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", gw.Id)
	}
	if gw.Type != Gateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != Connected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}
	// Gateways have no directly-connected clients — numClients must be nil
	if gw.NumClients != nil {
		t.Errorf("expected nil numClients for gateway, got %v", gw.NumClients)
	}

	// Switch
	sw := result.Items[1]
	if sw.Id != "unifi.us-8-60w" {
		t.Errorf("expected id unifi.us-8-60w, got %s", sw.Id)
	}
	if sw.Type != Switch {
		t.Errorf("expected type switch, got %s", sw.Type)
	}

	// Access point with clients
	ap := result.Items[3]
	if ap.Id != "unifi.uap-01" {
		t.Errorf("expected id unifi.uap-01, got %s", ap.Id)
	}
	if ap.Type != AccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}
	if ap.NumClients == nil || *ap.NumClients != 7 {
		t.Errorf("expected numClients=7 for AP, got %v", ap.NumClients)
	}

	// Offline AP: numClients=0 (AP type, just disconnected)
	offline := result.Items[4]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected, got %s", offline.Status)
	}
	if offline.NumClients == nil || *offline.NumClients != 0 {
		t.Errorf("expected numClients=0 for offline AP, got %v", offline.NumClients)
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

func TestGetDevice(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	detail, found, err := svc.GetDevice(context.Background(), "unifi.usg-3p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected device to be found")
	}

	if detail.Id != "unifi.usg-3p" {
		t.Errorf("expected id unifi.usg-3p, got %s", detail.Id)
	}
	if detail.Model != "UGW3" {
		t.Errorf("expected model UGW3, got %s", detail.Model)
	}
	if detail.FirmwareVersion != "4.4.57.5578372" {
		t.Errorf("expected version 4.4.57.5578372, got %s", detail.FirmwareVersion)
	}
	if detail.Uptime != 16066061 {
		t.Errorf("expected uptime 16066061, got %d", detail.Uptime)
	}
	if detail.NumClients != nil {
		t.Errorf("expected nil numClients for gateway detail, got %v", detail.NumClients)
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
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.macbook-pro-3c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wireless, err := detail.AsWirelessNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wireless detail, got error: %v", err)
	}
	if wireless.Ssid == nil || *wireless.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %v", wireless.Ssid)
	}
	if wireless.SignalStrength == nil || *wireless.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %v", wireless.SignalStrength)
	}
	if wireless.Uptime == nil || *wireless.Uptime != 27075 {
		t.Errorf("expected uptime 27075, got %v", wireless.Uptime)
	}
}

func TestGetClientWired(t *testing.T) {
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{clients: clients}}, 30)

	detail, found, err := svc.GetClient(context.Background(), "unifi.nas-1-68")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected client to be found")
	}

	wired, err := detail.AsWiredNetworkClientDetail()
	if err != nil {
		t.Fatalf("expected wired detail, got error: %v", err)
	}
	if wired.SwitchName == nil || *wired.SwitchName != "Switch Living Room" {
		t.Errorf("expected switchName Switch Living Room, got %v", wired.SwitchName)
	}
	if wired.SwitchPort == nil || *wired.SwitchPort != 3 {
		t.Errorf("expected switchPort 3, got %v", wired.SwitchPort)
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

func TestMapDeviceType(t *testing.T) {
	tests := []struct {
		input string
		want  NetworkDeviceType
	}{
		{"uap", AccessPoint},
		{"usw", Switch},
		{"ugw", Gateway},
		{"udm", Gateway},
		{"udm-pro", Gateway},
		{"other", Unknown},
		{"", Unknown},
	}
	for _, tt := range tests {
		got := mapDeviceType(tt.input)
		if got != tt.want {
			t.Errorf("mapDeviceType(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}
