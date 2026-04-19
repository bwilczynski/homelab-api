package network

import (
	"context"
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

func TestListDevices(t *testing.T) {
	numSta := 5
	svc := NewService("unifi", &mockUniFi{
		devices: []adapters.UniFiDevice{
			{MAC: "aa:bb:cc:dd:00:01", Name: "USG", Model: "USG", Type: "ugw", State: 1, IP: "192.168.1.1", Version: "4.4.57", Uptime: 86400},
			{MAC: "aa:bb:cc:dd:00:02", Name: "Switch", Model: "USW-24-PoE", Type: "usw", State: 1, IP: "192.168.1.2", Version: "6.6.61", Uptime: 86400},
			{MAC: "aa:bb:cc:dd:00:03", Name: "AP Living Room", Model: "U6-Lite", Type: "uap", State: 1, IP: "192.168.1.3", Version: "6.6.77", Uptime: 3600, NumSta: numSta},
			{MAC: "aa:bb:cc:dd:00:04", Name: "AP Office", Model: "U6-Pro", Type: "uap", State: 0, IP: "192.168.1.4", Version: "6.6.77", Uptime: 0},
		},
	})

	result, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 4 {
		t.Fatalf("expected 4 devices, got %d", len(result.Items))
	}

	gw := result.Items[0]
	if gw.Id != "unifi.aabbccdd0001" {
		t.Errorf("expected id unifi.aabbccdd0001, got %s", gw.Id)
	}
	if gw.Type != Gateway {
		t.Errorf("expected type gateway, got %s", gw.Type)
	}
	if gw.Status != Connected {
		t.Errorf("expected status connected, got %s", gw.Status)
	}
	if gw.NumClients != nil {
		t.Errorf("expected nil numClients for gateway, got %v", gw.NumClients)
	}

	ap := result.Items[2]
	if ap.Type != AccessPoint {
		t.Errorf("expected type accessPoint, got %s", ap.Type)
	}
	if ap.NumClients == nil || *ap.NumClients != 5 {
		t.Errorf("expected numClients=5 for AP, got %v", ap.NumClients)
	}

	offline := result.Items[3]
	if offline.Status != Disconnected {
		t.Errorf("expected status disconnected for offline AP, got %s", offline.Status)
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
	svc := NewService("unifi", &mockUniFi{
		clients: []adapters.UniFiSta{
			{MAC: "11:22:33:44:55:01", Name: "MacBook Pro", Hostname: "macbook", IP: "192.168.1.101", IsWired: false, ESSID: "HomeNetwork", Signal: -62, Uptime: 7200},
			{MAC: "11:22:33:44:55:02", Hostname: "nas-1", IP: "192.168.1.10", IsWired: true, Uptime: 604800},
			{MAC: "11:22:33:44:55:03", Hostname: "", Name: "", IP: "", IsWired: false, ESSID: "HomeNetwork", Signal: -71, Uptime: 900},
		},
	})

	result, err := svc.ListClients(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(result.Items))
	}

	wireless := result.Items[0]
	if wireless.Id != "unifi.112233445501" {
		t.Errorf("expected id unifi.112233445501, got %s", wireless.Id)
	}
	if wireless.Name != "MacBook Pro" {
		t.Errorf("expected name MacBook Pro (user alias), got %s", wireless.Name)
	}
	if wireless.ConnectionType != Wireless {
		t.Errorf("expected wireless, got %s", wireless.ConnectionType)
	}
	if wireless.Ssid == nil || *wireless.Ssid != "HomeNetwork" {
		t.Errorf("expected ssid HomeNetwork, got %v", wireless.Ssid)
	}
	if wireless.SignalStrength == nil || *wireless.SignalStrength != -62 {
		t.Errorf("expected signal -62, got %v", wireless.SignalStrength)
	}

	wired := result.Items[1]
	if wired.Name != "nas-1" {
		t.Errorf("expected name nas-1 (hostname fallback), got %s", wired.Name)
	}
	if wired.ConnectionType != Wired {
		t.Errorf("expected wired, got %s", wired.ConnectionType)
	}
	if wired.Ssid != nil {
		t.Errorf("expected nil ssid for wired client, got %v", wired.Ssid)
	}

	noName := result.Items[2]
	if noName.Name != "11:22:33:44:55:03" {
		t.Errorf("expected mac fallback name, got %s", noName.Name)
	}
	if noName.Ip != nil {
		t.Errorf("expected nil ip when empty, got %v", noName.Ip)
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
