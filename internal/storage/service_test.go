package storage

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// mockBackend implements StorageBackend for testing.
type mockBackend struct {
	resp *adapters.DSMStorageVolumeResponse
	err  error
}

func (m *mockBackend) GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error) {
	return m.resp, m.err
}

func loadFixture[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	// The fixture is a full Synology response envelope; extract .data
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return envelope.Data
}

func TestListStorageVolumes(t *testing.T) {
	resp := loadFixture[adapters.DSMStorageVolumeResponse](t, "testdata/storage_volumes.json")

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	result, err := svc.ListStorageVolumes(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(result.Items))
	}

	v := result.Items[0]
	if v.Id != "nas-01.volume_1" {
		t.Errorf("expected id nas-01.volume_1, got %s", v.Id)
	}
	if v.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", v.Device)
	}
	if v.Name != "volume_1" {
		t.Errorf("expected name volume_1, got %s", v.Name)
	}
	if v.FileSystem != "btrfs" {
		t.Errorf("expected filesystem btrfs, got %s", v.FileSystem)
	}
	if v.RaidType != "single" {
		t.Errorf("expected raidType single, got %s", v.RaidType)
	}
	if v.Status != VolumeStatusNormal {
		t.Errorf("expected status normal, got %s", v.Status)
	}
	if v.TotalBytes != 11508017246208 {
		t.Errorf("expected totalBytes 11508017246208, got %d", v.TotalBytes)
	}
	if v.UsedBytes != 8230590287872 {
		t.Errorf("expected usedBytes 8230590287872, got %d", v.UsedBytes)
	}
}

func TestListStorageVolumesDiskMapping(t *testing.T) {
	resp := loadFixture[adapters.DSMStorageVolumeResponse](t, "testdata/storage_volumes.json")

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	result, err := svc.ListStorageVolumes(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v := result.Items[0]
	if len(v.Disks) != 4 {
		t.Fatalf("expected 4 disks, got %d", len(v.Disks))
	}

	d := v.Disks[0]
	if d.Id != "sda" {
		t.Errorf("expected disk id sda, got %s", d.Id)
	}
	if d.Model != "WD Red Plus 4TB" {
		t.Errorf("expected model WD Red Plus 4TB, got %s", d.Model)
	}
	if d.Status != DiskStatusNormal {
		t.Errorf("expected disk status normal, got %s", d.Status)
	}
	if d.TemperatureCelsius != 35 {
		t.Errorf("expected temperature 35, got %d", d.TemperatureCelsius)
	}
}

func TestListStorageVolumesWithDeviceFilter(t *testing.T) {
	resp := loadFixture[adapters.DSMStorageVolumeResponse](t, "testdata/storage_volumes.json")

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	// Matching device
	device := "nas-01"
	result, err := svc.ListStorageVolumes(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 volume for matching device, got %d", len(result.Items))
	}

	// Non-matching device
	other := "nas-02"
	result, err = svc.ListStorageVolumes(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 volumes for non-matching device, got %d", len(result.Items))
	}
}

func TestGetStorageVolume(t *testing.T) {
	resp := loadFixture[adapters.DSMStorageVolumeResponse](t, "testdata/storage_volumes.json")

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	v, err := svc.GetStorageVolume(context.Background(), "nas-01.volume_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil {
		t.Fatal("expected volume, got nil")
	}
	if v.Id != "nas-01.volume_1" {
		t.Errorf("expected id nas-01.volume_1, got %s", v.Id)
	}
	if v.MountPath != "/volume1" {
		t.Errorf("expected mountPath /volume1, got %s", v.MountPath)
	}
	if v.PoolStatus != VolumeStatusNormal {
		t.Errorf("expected poolStatus normal, got %s", v.PoolStatus)
	}
}

func TestGetStorageVolumePoolStatus(t *testing.T) {
	resp := adapters.DSMStorageVolumeResponse{
		Volumes: []adapters.DSMStorageVolume{
			{ID: "volume_1", VolPath: "/volume1", Status: "normal", FsType: "btrfs", RaidType: "SHR", PoolPath: "reuse_1", Size: adapters.DSMStorageVolumeSize{Total: "1000", Used: "500"}},
		},
		Disks: []adapters.DSMStorageDisk{},
		StoragePools: []adapters.DSMStoragePool{
			{ID: "reuse_1", Disks: []string{}, RaidType: "SHR", Status: "degraded"},
		},
	}

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	v, err := svc.GetStorageVolume(context.Background(), "nas-01.volume_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil {
		t.Fatal("expected volume, got nil")
	}
	if v.PoolStatus != VolumeStatusDegraded {
		t.Errorf("expected poolStatus degraded, got %s", v.PoolStatus)
	}
}

func TestGetStorageVolumeNotFound(t *testing.T) {
	resp := loadFixture[adapters.DSMStorageVolumeResponse](t, "testdata/storage_volumes.json")

	svc := NewService("nas-01", &mockBackend{resp: &resp})

	v, err := svc.GetStorageVolume(context.Background(), "nas-01.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Errorf("expected nil for missing volume, got %+v", v)
	}
}

func TestGetStorageVolumeInvalidID(t *testing.T) {
	svc := NewService("nas-01", &mockBackend{})

	_, err := svc.GetStorageVolume(context.Background(), "invalid-id")
	if err == nil {
		t.Fatal("expected error for invalid volume ID")
	}
}

func TestListStorageVolumesEmpty(t *testing.T) {
	svc := NewService("nas-01", &mockBackend{
		resp: &adapters.DSMStorageVolumeResponse{
			Volumes:      []adapters.DSMStorageVolume{},
			Disks:        []adapters.DSMStorageDisk{},
			StoragePools: []adapters.DSMStoragePool{},
		},
	})

	result, err := svc.ListStorageVolumes(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 volumes, got %d", len(result.Items))
	}
}

func TestMapVolumeStatus(t *testing.T) {
	tests := []struct {
		status string
		want   VolumeStatus
	}{
		{"normal", VolumeStatusNormal},
		{"degraded", VolumeStatusDegraded},
		{"repairing", VolumeStatusRepairing},
		{"crashed", VolumeStatusCrashed},
		{"unknown", VolumeStatusCrashed},
	}

	for _, tt := range tests {
		got := mapVolumeStatus(tt.status)
		if got != tt.want {
			t.Errorf("mapVolumeStatus(%q) = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestMapDiskStatus(t *testing.T) {
	tests := []struct {
		status string
		want   DiskStatus
	}{
		{"normal", DiskStatusNormal},
		{"warning", DiskStatusWarning},
		{"failing", DiskStatusFailing},
		{"critical", DiskStatusCritical},
		{"unknown", DiskStatusCritical},
	}

	for _, tt := range tests {
		got := mapDiskStatus(tt.status)
		if got != tt.want {
			t.Errorf("mapDiskStatus(%q) = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestParseVolumeID(t *testing.T) {
	tests := []struct {
		id         string
		wantDevice string
		wantName   string
		wantErr    bool
	}{
		{"nas-01.volume_1", "nas-01", "volume_1", false},
		{"device.name.with.dots", "device", "name.with.dots", false},
		{"invalid", "", "", true},
		{".name", "", "", true},
		{"device.", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		device, name, err := parseVolumeID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseVolumeID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			continue
		}
		if device != tt.wantDevice {
			t.Errorf("parseVolumeID(%q) device = %q, want %q", tt.id, device, tt.wantDevice)
		}
		if name != tt.wantName {
			t.Errorf("parseVolumeID(%q) name = %q, want %q", tt.id, name, tt.wantName)
		}
	}
}
