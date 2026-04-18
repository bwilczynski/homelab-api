package containers

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// mockBackend implements ContainerBackend for testing.
type mockBackend struct {
	listResp      *adapters.DSMContainerListResponse
	detailResp    *adapters.DSMContainerDetailResponse
	resourcesResp *adapters.DSMContainerResourceResponse
	startErr      error
	stopErr       error
	restartErr    error
}

func (m *mockBackend) ListContainers() (*adapters.DSMContainerListResponse, error) {
	return m.listResp, nil
}

func (m *mockBackend) GetContainer(name string) (*adapters.DSMContainerDetailResponse, error) {
	return m.detailResp, nil
}

func (m *mockBackend) GetContainerResources() (*adapters.DSMContainerResourceResponse, error) {
	return m.resourcesResp, nil
}

func (m *mockBackend) StartContainer(name string) error   { return m.startErr }
func (m *mockBackend) StopContainer(name string) error    { return m.stopErr }
func (m *mockBackend) RestartContainer(name string) error { return m.restartErr }

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

func TestListContainers(t *testing.T) {
	listResp := loadFixture[adapters.DSMContainerListResponse](t, "testdata/container_list.json")
	resourcesResp := loadFixture[adapters.DSMContainerResourceResponse](t, "testdata/container_resources.json")

	svc := NewService("nas-01", &mockBackend{
		listResp:      &listResp,
		resourcesResp: &resourcesResp,
	})

	result, err := svc.ListContainers(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(result.Items))
	}

	// Check first container mapping
	c := result.Items[0]
	if c.Id != "nas-01.immich_server" {
		t.Errorf("expected id nas-01:immich_server, got %s", c.Id)
	}
	if c.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", c.Device)
	}
	if c.Name != "immich_server" {
		t.Errorf("expected name immich_server, got %s", c.Name)
	}
	if c.Image != "ghcr.io/immich-app/immich-server:v2.6.3" {
		t.Errorf("expected image ghcr.io/immich-app/immich-server:v2.6.3, got %s", c.Image)
	}
	if c.Status != Running {
		t.Errorf("expected status running, got %s", c.Status)
	}
	if c.Resources.CpuPercent != 0.325 {
		t.Errorf("expected cpu 0.325, got %f", c.Resources.CpuPercent)
	}
	if c.Resources.MemoryBytes != 394334208 {
		t.Errorf("expected memory 394334208, got %d", c.Resources.MemoryBytes)
	}

	// Check stopped container
	stopped := result.Items[2]
	if stopped.Status != Stopped {
		t.Errorf("expected status stopped, got %s", stopped.Status)
	}
}

func TestListContainersWithDeviceFilter(t *testing.T) {
	listResp := loadFixture[adapters.DSMContainerListResponse](t, "testdata/container_list.json")
	resourcesResp := loadFixture[adapters.DSMContainerResourceResponse](t, "testdata/container_resources.json")

	svc := NewService("nas-01", &mockBackend{
		listResp:      &listResp,
		resourcesResp: &resourcesResp,
	})

	// Filter for matching device
	device := "nas-01"
	result, err := svc.ListContainers(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 containers for matching device, got %d", len(result.Items))
	}

	// Filter for non-matching device
	other := "nas-02"
	result, err = svc.ListContainers(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 containers for non-matching device, got %d", len(result.Items))
	}
}

func TestGetContainer(t *testing.T) {
	detailResp := loadFixture[adapters.DSMContainerDetailResponse](t, "testdata/container_detail.json")
	resourcesResp := loadFixture[adapters.DSMContainerResourceResponse](t, "testdata/container_resources.json")

	svc := NewService("nas-01", &mockBackend{
		detailResp:    &detailResp,
		resourcesResp: &resourcesResp,
	})

	c, err := svc.GetContainer(context.Background(), "nas-01.immich_server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Id != "nas-01.immich_server" {
		t.Errorf("expected id nas-01:immich_server, got %s", c.Id)
	}
	if c.RestartCount != 0 {
		t.Errorf("expected restart count 0, got %d", c.RestartCount)
	}
	if c.Image != "ghcr.io/immich-app/immich-server:v2.6.3" {
		t.Errorf("expected image from Config.Image, got %s", c.Image)
	}
	if c.Resources.CpuPercent != 0.325 {
		t.Errorf("expected cpu 0.325, got %f", c.Resources.CpuPercent)
	}
}

func TestGetContainerDetailFields(t *testing.T) {
	detailResp := loadFixture[adapters.DSMContainerDetailResponse](t, "testdata/container_detail.json")
	resourcesResp := loadFixture[adapters.DSMContainerResourceResponse](t, "testdata/container_resources.json")

	svc := NewService("nas-01", &mockBackend{
		detailResp:    &detailResp,
		resourcesResp: &resourcesResp,
	})

	c, err := svc.GetContainer(context.Background(), "nas-01.immich_server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.EnvVariables) != 5 {
		t.Errorf("expected 5 env variables, got %d", len(c.EnvVariables))
	}
	if c.EnvVariables[0].Key != "PATH" {
		t.Errorf("expected first env var key PATH, got %s", c.EnvVariables[0].Key)
	}
	if c.EnvVariables[3].Key != "DB_PASSWORD" {
		t.Errorf("expected sensitive env var key DB_PASSWORD, got %s", c.EnvVariables[3].Key)
	}
	if c.EnvVariables[3].Value != "REDACTED" {
		t.Errorf("expected REDACTED for DB_PASSWORD, got %s", c.EnvVariables[3].Value)
	}

	if len(c.Networks) != 2 {
		t.Errorf("expected 2 networks, got %d", len(c.Networks))
	}
	if c.Networks[0].Name != "immich_default" {
		t.Errorf("expected first network immich_default, got %s", c.Networks[0].Name)
	}
	if c.Networks[0].Driver != "bridge" {
		t.Errorf("expected driver bridge, got %s", c.Networks[0].Driver)
	}

	if len(c.PortBindings) != 1 {
		t.Errorf("expected 1 port binding, got %d", len(c.PortBindings))
	}
	if c.PortBindings[0].ContainerPort != 2283 {
		t.Errorf("expected container port 2283, got %d", c.PortBindings[0].ContainerPort)
	}
	if c.PortBindings[0].HostPort != 12080 {
		t.Errorf("expected host port 12080, got %d", c.PortBindings[0].HostPort)
	}
	if c.PortBindings[0].Protocol != Tcp {
		t.Errorf("expected protocol tcp, got %s", c.PortBindings[0].Protocol)
	}

	if len(c.VolumeBindings) != 2 {
		t.Errorf("expected 2 volume bindings, got %d", len(c.VolumeBindings))
	}
	if c.VolumeBindings[0].Source != "/docker/immich/upload" {
		t.Errorf("expected source /docker/immich/upload, got %s", c.VolumeBindings[0].Source)
	}
	if c.VolumeBindings[0].Destination != "/data" {
		t.Errorf("expected destination /data, got %s", c.VolumeBindings[0].Destination)
	}
	if c.VolumeBindings[0].Mode != Rw {
		t.Errorf("expected mode rw, got %s", c.VolumeBindings[0].Mode)
	}
	if c.VolumeBindings[1].Mode != Ro {
		t.Errorf("expected read-only mode ro, got %s", c.VolumeBindings[1].Mode)
	}

	if c.RestartPolicy != Always {
		t.Errorf("expected restart policy always, got %s", c.RestartPolicy)
	}

	if c.Privileged != false {
		t.Errorf("expected privileged false, got %v", c.Privileged)
	}

	if c.MemoryLimit != 0 {
		t.Errorf("expected memory limit 0, got %d", c.MemoryLimit)
	}

	if len(c.Entrypoint) != 4 {
		t.Errorf("expected 4 entrypoint args, got %d", len(c.Entrypoint))
	}
	if c.Entrypoint[0] != "tini" {
		t.Errorf("expected entrypoint[0] tini, got %s", c.Entrypoint[0])
	}

	if len(c.Cmd) != 1 {
		t.Errorf("expected 1 cmd arg, got %d", len(c.Cmd))
	}
	if c.Cmd[0] != "start.sh" {
		t.Errorf("expected cmd start.sh, got %s", c.Cmd[0])
	}

	if c.Labels == nil || (*c.Labels)["com.docker.compose.project"] != "immich" {
		t.Errorf("expected label com.docker.compose.project=immich")
	}
}

func TestGetContainerStatusFields(t *testing.T) {
	detailResp := loadFixture[adapters.DSMContainerDetailResponse](t, "testdata/container_detail.json")
	resourcesResp := loadFixture[adapters.DSMContainerResourceResponse](t, "testdata/container_resources.json")

	svc := NewService("nas-01", &mockBackend{
		detailResp:    &detailResp,
		resourcesResp: &resourcesResp,
	})

	c, err := svc.GetContainer(context.Background(), "nas-01.immich_server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.StartedAt.IsZero() {
		t.Error("expected non-zero startedAt")
	}

	if c.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", c.ExitCode)
	}

	if c.OomKilled != false {
		t.Errorf("expected oomKilled false, got %v", c.OomKilled)
	}
}

func TestGetContainerInvalidID(t *testing.T) {
	svc := NewService("nas-01", &mockBackend{})

	_, err := svc.GetContainer(context.Background(), "invalid-id")
	if err == nil {
		t.Fatal("expected error for invalid container ID")
	}
}

func TestParseContainerID(t *testing.T) {
	tests := []struct {
		id         string
		wantDevice string
		wantName   string
		wantErr    bool
	}{
		{"nas-01.immich_server", "nas-01", "immich_server", false},
		{"device.name.with.dots", "device", "name.with.dots", false},
		{"invalid", "", "", true},
		{":name", "", "", true},
		{"device:", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		device, name, err := parseContainerID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseContainerID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			continue
		}
		if device != tt.wantDevice {
			t.Errorf("parseContainerID(%q) device = %q, want %q", tt.id, device, tt.wantDevice)
		}
		if name != tt.wantName {
			t.Errorf("parseContainerID(%q) name = %q, want %q", tt.id, name, tt.wantName)
		}
	}
}

func TestMapStatus(t *testing.T) {
	tests := []struct {
		state adapters.DSMContainerState
		want  ContainerStatus
	}{
		{adapters.DSMContainerState{Running: true}, Running},
		{adapters.DSMContainerState{Dead: true}, Dead},
		{adapters.DSMContainerState{Paused: true}, Paused},
		{adapters.DSMContainerState{Restarting: true}, Restarting},
		{adapters.DSMContainerState{}, Stopped},
	}

	for _, tt := range tests {
		got := mapStatus(tt.state)
		if got != tt.want {
			t.Errorf("mapStatus(%+v) = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestListContainersEmptyList(t *testing.T) {
	svc := NewService("nas-01", &mockBackend{
		listResp: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{},
		},
		resourcesResp: &adapters.DSMContainerResourceResponse{
			Resources: []adapters.DSMContainerResource{},
		},
	})

	result, err := svc.ListContainers(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 containers, got %d", len(result.Items))
	}
}
