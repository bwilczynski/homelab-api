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

	// Gateway (ugw)
	gw := result.Items[0]
	if gw.Id != "unifi.aabbccdd0001" {
		t.Errorf("expected id unifi.aabbccdd0001, got %s", gw.Id)
	}
	if gw.Name != "USG 3P" {
		t.Errorf("expected name USG 3P, got %s", gw.Name)
	}
	if gw.Type != Gateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != Connected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}
	if gw.Model != "UGW3" {
		t.Errorf("expected model UGW3, got %s", gw.Model)
	}
	if gw.FirmwareVersion != "4.4.57.5578372" {
		t.Errorf("expected version 4.4.57.5578372, got %s", gw.FirmwareVersion)
	}
	// Gateway has 9 user clients — numClients should be set
	if gw.NumClients == nil || *gw.NumClients != 9 {
		t.Errorf("expected numClients=9 for gateway, got %v", gw.NumClients)
	}

	// Switch (usw)
	sw := result.Items[1]
	if sw.Type != Switch {
		t.Errorf("expected type switch, got %s", sw.Type)
	}

	// Access point with clients
	ap := result.Items[3]
	if ap.Type != AccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}
	if ap.NumClients == nil || *ap.NumClients != 7 {
		t.Errorf("expected numClients=7 for AP, got %v", ap.NumClients)
	}

	// Offline AP
	offline := result.Items[4]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected, got %s", offline.Status)
	}
	if offline.NumClients != nil {
		t.Errorf("expected nil numClients when count is 0, got %v", offline.NumClients)
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

	// Wireless client with hostname (no user alias)
	wireless := result.Items[0]
	if wireless.Id != "unifi.112233445501" {
		t.Errorf("expected id unifi.112233445501, got %s", wireless.Id)
	}
	if wireless.Name != "mac-host" {
		t.Errorf("expected hostname fallback name mac-host, got %s", wireless.Name)
	}
	if wireless.ConnectionType != Wireless {
		t.Errorf("expected wireless, got %s", wireless.ConnectionType)
	}
	if wireless.Ssid == nil || *wireless.Ssid != "homelab" {
		t.Errorf("expected ssid homelab, got %v", wireless.Ssid)
	}
	if wireless.SignalStrength == nil || *wireless.SignalStrength != -69 {
		t.Errorf("expected signal -69, got %v", wireless.SignalStrength)
	}
	if wireless.Ip == nil || *wireless.Ip != "192.168.10.67" {
		t.Errorf("expected ip 192.168.10.67, got %v", wireless.Ip)
	}

	// Wired client
	wired := result.Items[1]
	if wired.ConnectionType != Wired {
		t.Errorf("expected wired, got %s", wired.ConnectionType)
	}
	if wired.Ssid != nil {
		t.Errorf("expected nil ssid for wired client, got %v", wired.Ssid)
	}
	if wired.SignalStrength != nil {
		t.Errorf("expected nil signalStrength for wired client, got %v", wired.SignalStrength)
	}

	// Wireless client with user alias (no hostname)
	named := result.Items[2]
	if named.Name != "Nintendo Switch" {
		t.Errorf("expected user alias Nintendo Switch, got %s", named.Name)
	}

	// Client where user alias takes priority over hostname
	aliased := result.Items[3]
	if aliased.Name != "Synology DS918+" {
		t.Errorf("expected user alias Synology DS918+, got %s", aliased.Name)
	}

	// Client with neither name nor hostname — falls back to MAC
	noName := result.Items[4]
	if noName.Name != "11:22:33:44:55:05" {
		t.Errorf("expected MAC fallback, got %s", noName.Name)
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
