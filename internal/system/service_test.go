package system

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// --- Mock backends ---

type mockDSMBackend struct {
	info  *adapters.DSMSystemInfoResponse
	util  *adapters.DSMSystemUtilizationResponse
	err   error
}

func (m *mockDSMBackend) GetSystemInfo() (*adapters.DSMSystemInfoResponse, error) {
	return m.info, m.err
}

func (m *mockDSMBackend) GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error) {
	return m.util, m.err
}

type mockUniFiBackend struct {
	subsystems []adapters.UniFiSubsystemHealth
	err        error
}

func (m *mockUniFiBackend) GetHealth() ([]adapters.UniFiSubsystemHealth, error) {
	return m.subsystems, m.err
}

// --- Fixture helpers ---

func loadDSMSystemInfo(t *testing.T) *adapters.DSMSystemInfoResponse {
	t.Helper()
	raw, err := os.ReadFile("testdata/dsm-system-info.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var envelope struct {
		Data adapters.DSMSystemInfoResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &envelope.Data
}

func loadDSMSystemUtilization(t *testing.T) *adapters.DSMSystemUtilizationResponse {
	t.Helper()
	raw, err := os.ReadFile("testdata/dsm-system-utilization.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var envelope struct {
		Data adapters.DSMSystemUtilizationResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &envelope.Data
}

func loadUniFiHealth(t *testing.T) []adapters.UniFiSubsystemHealth {
	t.Helper()
	raw, err := os.ReadFile("testdata/unifi-health.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var envelope struct {
		Data []adapters.UniFiSubsystemHealth `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return envelope.Data
}

// --- Tests: GetSystemHealth ---

func TestGetSystemHealth_Healthy(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{}, &mockUniFiBackend{
		subsystems: loadUniFiHealth(t),
	})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fixture includes a "vpn" subsystem with status "unknown" → maps to Degraded.
	if health.Status != Degraded {
		t.Errorf("expected status degraded (vpn unknown), got %s", health.Status)
	}
	if len(health.Components) == 0 {
		t.Error("expected non-empty components")
	}
	if health.CheckedAt.IsZero() {
		t.Error("expected non-zero checkedAt")
	}

	// Verify each component maps correctly.
	for _, c := range health.Components {
		if c.Name == "" {
			t.Error("component name must not be empty")
		}
		if !c.Status.Valid() {
			t.Errorf("component %s has invalid status %s", c.Name, c.Status)
		}
	}
}

func TestGetSystemHealth_Degraded_WhenUnknownSubsystem(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{}, &mockUniFiBackend{
		subsystems: []adapters.UniFiSubsystemHealth{
			{Subsystem: "wan", Status: "ok"},
			{Subsystem: "vpn", Status: "unknown"},
		},
	})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != Degraded {
		t.Errorf("expected degraded, got %s", health.Status)
	}
}

func TestGetSystemHealth_Unhealthy(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{}, &mockUniFiBackend{
		subsystems: []adapters.UniFiSubsystemHealth{
			{Subsystem: "wan", Status: "error"},
		},
	})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != Unhealthy {
		t.Errorf("expected unhealthy, got %s", health.Status)
	}
}

func TestGetSystemHealth_EmptyComponents(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{}, &mockUniFiBackend{
		subsystems: []adapters.UniFiSubsystemHealth{},
	})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != Healthy {
		t.Errorf("expected healthy for empty components, got %s", health.Status)
	}
	if health.Components == nil {
		t.Error("components must not be nil")
	}
}

// --- Tests: ListSystemInfo ---

func TestListSystemInfo(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemInfo(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", item.Device)
	}
	if item.Model != "DS918+" {
		t.Errorf("expected model DS918+, got %s", item.Model)
	}
	if item.Firmware == "" {
		t.Error("firmware must not be empty")
	}
	if item.RamMb != 16384 {
		t.Errorf("expected ramMb 16384, got %d", item.RamMb)
	}
	// "100:00:00" = 360000 seconds
	if item.UptimeSeconds != 360000 {
		t.Errorf("expected uptimeSeconds 360000, got %d", item.UptimeSeconds)
	}
}

func TestListSystemInfo_DeviceFilter_Match(t *testing.T) {
	device := "nas-01"
	svc := NewService("nas-01", &mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemInfo(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Errorf("expected 1 item for matching device, got %d", len(result.Items))
	}
}

func TestListSystemInfo_DeviceFilter_NoMatch(t *testing.T) {
	device := "other-device"
	svc := NewService("nas-01", &mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemInfo(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 0 {
		t.Errorf("expected 0 items for non-matching device, got %d", len(result.Items))
	}
}

// --- Tests: ListSystemUtilization ---

func TestListSystemUtilization(t *testing.T) {
	svc := NewService("nas-01", &mockDSMBackend{util: loadDSMSystemUtilization(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemUtilization(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", item.Device)
	}
	if item.SampledAt.IsZero() {
		t.Error("sampledAt must not be zero")
	}

	// CPU
	if item.Cpu.UserPercent != 8 {
		t.Errorf("expected userPercent 8, got %d", item.Cpu.UserPercent)
	}
	if item.Cpu.SystemPercent != 2 {
		t.Errorf("expected systemPercent 2, got %d", item.Cpu.SystemPercent)
	}
	if item.Cpu.TotalPercent != 14 { // user(8) + system(2) + other(4)
		t.Errorf("expected totalPercent 14, got %d", item.Cpu.TotalPercent)
	}

	// Memory: DSM reports in KB, fixture uses values from real response.
	// total_real=16234636 KB → 16624267264 bytes
	expectedTotalBytes := int64(16234636) * 1024
	if item.Memory.TotalBytes != expectedTotalBytes {
		t.Errorf("expected totalBytes %d, got %d", expectedTotalBytes, item.Memory.TotalBytes)
	}
	if item.Memory.UsedPercent != 46 {
		t.Errorf("expected usedPercent 46, got %d", item.Memory.UsedPercent)
	}

	// Network: "total" device should be excluded.
	for _, n := range item.Network {
		if n.Name == "total" {
			t.Error("'total' network device should be filtered out")
		}
	}
	if len(item.Network) == 0 {
		t.Error("expected at least one network interface")
	}

	// Disks
	if len(item.Disks) != 4 {
		t.Errorf("expected 4 disks, got %d", len(item.Disks))
	}
}

func TestListSystemUtilization_DeviceFilter_NoMatch(t *testing.T) {
	device := "other-device"
	svc := NewService("nas-01", &mockDSMBackend{util: loadDSMSystemUtilization(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemUtilization(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 0 {
		t.Errorf("expected 0 items for non-matching device, got %d", len(result.Items))
	}
}

// --- Tests: parseUptime ---

func TestParseUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"0:0:0", 0},
		{"1:0:0", 3600},
		{"1:30:45", 5445},
		{"100:00:00", 360000},
		{"1872:52:14", 6742334},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseUptime(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("parseUptime(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseUptime_InvalidFormat(t *testing.T) {
	_, err := parseUptime("not-valid")
	if err == nil {
		t.Error("expected error for invalid uptime format")
	}
}
