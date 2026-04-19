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
	devices []adapters.UniFiDevice
	clients []adapters.UniFiSta
	err     error
}

func (m *mockUniFi) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, m.err
}

func (m *mockUniFi) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, m.err
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
	svc := NewService("unifi", &mockUniFi{devices: devices})

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
	// List shape: no Model/FirmwareVersion/Uptime
	if gw.NumClients == nil || *gw.NumClients != 9 {
		t.Errorf("expected numClients=9, got %v", gw.NumClients)
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

	// Offline AP: numClients nil (zero total)
	offline := result.Items[4]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected, got %s", offline.Status)
	}
	if offline.NumClients != nil {
		t.Errorf("expected nil numClients for zero-count device, got %v", offline.NumClients)
	}
}

func TestListDevicesEmpty(t *testing.T) {
	svc := NewService("unifi", &mockUniFi{devices: []adapters.UniFiDevice{}})
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
	svc := NewService("unifi", &mockUniFi{devices: devices})

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
}

func TestGetDeviceNotFound(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService("unifi", &mockUniFi{devices: devices})

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
	svc := NewService("unifi", &mockUniFi{devices: devices})

	_, found, err := svc.GetDevice(context.Background(), "other.usg-3p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found for wrong controller")
	}
}

// --- client list tests ---

func TestListClients(t *testing.T) {
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService("unifi", &mockUniFi{clients: clients})

	result, err := svc.ListClients(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 5 {
		t.Fatalf("expected 5 clients, got %d", len(result.Items))
	}

	// Wireless: user alias "MacBook Pro", mac starts with "3c"
	wireless := result.Items[0]
	if wireless.Id != "unifi.macbook-pro-3c" {
		t.Errorf("expected id unifi.macbook-pro-3c, got %s", wireless.Id)
	}
	if wireless.Name != "MacBook Pro" {
		t.Errorf("expected name MacBook Pro, got %s", wireless.Name)
	}
	if wireless.ConnectionType != NetworkClientConnectionTypeWireless {
		t.Errorf("expected wireless, got %s", wireless.ConnectionType)
	}
	// List shape: no ssid/signal/uptime
	if wireless.Ip == nil || *wireless.Ip != "192.168.10.67" {
		t.Errorf("expected ip 192.168.10.67, got %v", wireless.Ip)
	}

	// Wired: hostname "nas-1", mac starts with "68"
	wired := result.Items[1]
	if wired.Id != "unifi.nas-1-68" {
		t.Errorf("expected id unifi.nas-1-68, got %s", wired.Id)
	}
	if wired.ConnectionType != NetworkClientConnectionTypeWired {
		t.Errorf("expected wired, got %s", wired.ConnectionType)
	}

	// Wireless with user alias and no hostname: "Nintendo Switch", mac starts with "11"
	nintendo := result.Items[2]
	if nintendo.Id != "unifi.nintendo-switch-11" {
		t.Errorf("expected id unifi.nintendo-switch-11, got %s", nintendo.Id)
	}

	// Client with neither name nor hostname: falls back to MAC, mac starts with "ec"
	noName := result.Items[4]
	if noName.Name != "ec:b5:fa:22:d1:dc" {
		t.Errorf("expected MAC fallback name, got %s", noName.Name)
	}
}

func TestListClientsEmpty(t *testing.T) {
	svc := NewService("unifi", &mockUniFi{clients: []adapters.UniFiSta{}})
	result, err := svc.ListClients(context.Background())
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
	svc := NewService("unifi", &mockUniFi{clients: clients})

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
	if wireless.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %s", wireless.Ssid)
	}
	if wireless.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %d", wireless.SignalStrength)
	}
	if wireless.Uptime != 27075 {
		t.Errorf("expected uptime 27075, got %d", wireless.Uptime)
	}
}

func TestGetClientWired(t *testing.T) {
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService("unifi", &mockUniFi{clients: clients})

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
	if wired.SwitchName != "Switch Living Room" {
		t.Errorf("expected switchName Switch Living Room, got %s", wired.SwitchName)
	}
	if wired.SwitchPort != 3 {
		t.Errorf("expected switchPort 3, got %d", wired.SwitchPort)
	}
	if wired.Uptime != 1024199 {
		t.Errorf("expected uptime 1024199, got %d", wired.Uptime)
	}
}

func TestGetClientNotFound(t *testing.T) {
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	svc := NewService("unifi", &mockUniFi{clients: clients})

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
