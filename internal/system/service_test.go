package system

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// --- Mock backends ---

type mockDSMBackend struct {
	info     *adapters.DSMSystemInfoResponse
	util     *adapters.DSMSystemUtilizationResponse
	volumes  *adapters.DSMStorageVolumeResponse
	conts    *adapters.DSMContainerListResponse
	err      error
}

func (m *mockDSMBackend) GetSystemInfo() (*adapters.DSMSystemInfoResponse, error) {
	return m.info, m.err
}

func (m *mockDSMBackend) GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error) {
	return m.util, m.err
}

func (m *mockDSMBackend) GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error) {
	if m.volumes != nil {
		return m.volumes, nil
	}
	return &adapters.DSMStorageVolumeResponse{}, m.err
}

func (m *mockDSMBackend) ListContainers() (*adapters.DSMContainerListResponse, error) {
	if m.conts != nil {
		return m.conts, nil
	}
	return &adapters.DSMContainerListResponse{}, m.err
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

func loadDSMStorageVolumes(t *testing.T) *adapters.DSMStorageVolumeResponse {
	t.Helper()
	raw, err := os.ReadFile("testdata/dsm-storage-volumes.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var envelope struct {
		Data adapters.DSMStorageVolumeResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &envelope.Data
}

// newTestService creates a service with a single DSM backend "nas-01" and a single UniFi backend "unifi".
func newTestService(dsm DSMBackend, unifi UniFiBackend) *Service {
	return NewService(
		map[string]DSMBackendConfig{"nas-01": {Backend: dsm, DockerEnabled: true}},
		map[string]UniFiBackend{"unifi": unifi},
		config.UpdatesConfig{},
		slog.Default(),
	)
}

// --- Tests: GetSystemHealth ---

func TestGetSystemHealth_Healthy(t *testing.T) {
	svc := newTestService(&mockDSMBackend{volumes: loadDSMStorageVolumes(t)}, &mockUniFiBackend{
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
	svc := newTestService(&mockDSMBackend{}, &mockUniFiBackend{
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
	svc := newTestService(&mockDSMBackend{}, &mockUniFiBackend{
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
	svc := newTestService(&mockDSMBackend{}, &mockUniFiBackend{
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

func TestGetSystemHealth_StorageDegraded(t *testing.T) {
	svc := newTestService(&mockDSMBackend{
		volumes: &adapters.DSMStorageVolumeResponse{
			Volumes: []adapters.DSMStorageVolume{
				{ID: "volume_1", Status: "normal"},
				{ID: "volume_2", Status: "degraded"},
			},
		},
	}, &mockUniFiBackend{})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != Degraded {
		t.Errorf("expected degraded due to storage, got %s", health.Status)
	}

	var storageComp *ComponentHealth
	for i := range health.Components {
		if health.Components[i].Name == "storage" {
			storageComp = &health.Components[i]
			break
		}
	}
	if storageComp == nil {
		t.Fatal("expected storage component")
	}
	if storageComp.Status != Degraded {
		t.Errorf("expected storage status degraded, got %s", storageComp.Status)
	}
	if storageComp.Message == nil || *storageComp.Message == "" {
		t.Error("expected non-empty message for degraded storage")
	}
}

func TestGetSystemHealth_StorageCrashed(t *testing.T) {
	svc := newTestService(&mockDSMBackend{
		volumes: &adapters.DSMStorageVolumeResponse{
			Volumes: []adapters.DSMStorageVolume{
				{ID: "volume_1", Status: "crashed"},
			},
		},
	}, &mockUniFiBackend{})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Status != Unhealthy {
		t.Errorf("expected unhealthy due to crashed volume, got %s", health.Status)
	}
}

func TestGetSystemHealth_ContainersNotRunning(t *testing.T) {
	svc := newTestService(&mockDSMBackend{
		conts: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{
				{Name: "app1", State: adapters.DSMContainerState{Running: true}},
				{Name: "app2", State: adapters.DSMContainerState{Running: false}},
			},
		},
	}, &mockUniFiBackend{})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var containersComp *ComponentHealth
	for i := range health.Components {
		if health.Components[i].Name == "containers" {
			containersComp = &health.Components[i]
			break
		}
	}
	if containersComp == nil {
		t.Fatal("expected containers component")
	}
	if containersComp.Status != Degraded {
		t.Errorf("expected containers status degraded, got %s", containersComp.Status)
	}
	if containersComp.Message == nil || *containersComp.Message == "" {
		t.Error("expected non-empty message for stopped containers")
	}
}

func TestGetSystemHealth_AllComponentsPresent(t *testing.T) {
	svc := newTestService(&mockDSMBackend{volumes: loadDSMStorageVolumes(t)}, &mockUniFiBackend{
		subsystems: loadUniFiHealth(t),
	})

	health, err := svc.GetSystemHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range health.Components {
		names[c.Name] = true
	}
	for _, required := range []string{"storage", "containers"} {
		if !names[required] {
			t.Errorf("expected component %q in health response", required)
		}
	}
}

// --- Tests: ListSystemInfo ---

func TestListSystemInfo(t *testing.T) {
	svc := newTestService(&mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

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
	svc := newTestService(&mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

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
	svc := newTestService(&mockDSMBackend{info: loadDSMSystemInfo(t)}, &mockUniFiBackend{})

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
	svc := newTestService(&mockDSMBackend{util: loadDSMSystemUtilization(t)}, &mockUniFiBackend{})

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
	svc := newTestService(&mockDSMBackend{util: loadDSMSystemUtilization(t)}, &mockUniFiBackend{})

	result, err := svc.ListSystemUtilization(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 0 {
		t.Errorf("expected 0 items for non-matching device, got %d", len(result.Items))
	}
}

// --- Tests: ListSystemUpdates ---

// overrideGitHubClient swaps the package-level githubBaseURL and githubClient
// to point at the given test server, restoring originals via t.Cleanup.
func overrideGitHubClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origBaseURL := githubBaseURL
	origClient := githubClient
	githubBaseURL = srv.URL
	githubClient = srv.Client()
	t.Cleanup(func() {
		srv.Close()
		githubBaseURL = origBaseURL
		githubClient = origClient
	})
}

// mockGitHubServer creates an httptest server that serves the GitHub release fixture
// and swaps the package-level githubBaseURL and githubClient for the test duration.
func mockGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()
	fixture, err := os.ReadFile("testdata/github-release-latest.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	overrideGitHubClient(t, srv)
	return srv
}

func newTestServiceWithUpdates(t *testing.T, dsm DSMBackend) *Service {
	t.Helper()
	mockGitHubServer(t)
	return NewService(
		map[string]DSMBackendConfig{"nas-01": {Backend: dsm, DockerEnabled: true}},
		map[string]UniFiBackend{},
		config.UpdatesConfig{},
		slog.Default(),
	)
}

func TestListSystemUpdates_PreservesCachedStatusOnGitHubFailure(t *testing.T) {
	// Set up a mock server that always returns 429 to simulate rate limiting.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	overrideGitHubClient(t, srv)

	svc := NewService(
		map[string]DSMBackendConfig{"nas-01": {Backend: &mockDSMBackend{
			conts: &adapters.DSMContainerListResponse{
				Containers: []adapters.DSMContainer{
					{Name: "app", Image: "ghcr.io/owner/repo:v1.0.0"},
				},
			},
		}, DockerEnabled: true}},
		map[string]UniFiBackend{},
		config.UpdatesConfig{},
		slog.Default(),
	)

	// Seed GitHub releases cache with a known-good release.
	svc.SeedGitHubReleases(map[string]*GitHubRelease{
		"owner/repo": {TagName: "v1.0.0", HTMLURL: "https://github.com/owner/repo/releases/tag/v1.0.0"},
	})

	// Force a refresh — GitHub API returns 429,
	// so fetchReleases will fail. Cached release data should be preserved:
	// container v1.0.0 == release v1.0.0 → UpToDate.
	result, err := svc.CheckSystemUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Status != UpToDate {
		t.Errorf("expected status preserved as upToDate, got %s", result.Items[0].Status)
	}
}

func TestListSystemUpdates_PicksUpNewVersionAfterUpgrade(t *testing.T) {
	// Backend starts with container running v1.0.0.
	backend := &mockDSMBackend{
		conts: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{
				{Name: "app", Image: "ghcr.io/owner/repo:v1.0.0"},
			},
		},
	}
	// GitHub fixture returns tag_name "1.35.8", so v1.0.0 is behind.
	svc := newTestServiceWithUpdates(t, backend)

	result, err := svc.ListSystemUpdates(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].CurrentVersion != "v1.0.0" {
		t.Errorf("expected CurrentVersion v1.0.0, got %s", result.Items[0].CurrentVersion)
	}
	if result.Items[0].Status != UpdateAvailable {
		t.Errorf("expected updateAvailable before upgrade, got %s", result.Items[0].Status)
	}

	// Simulate upgrade: container now runs 1.35.8 (matches GitHub latest).
	backend.conts = &adapters.DSMContainerListResponse{
		Containers: []adapters.DSMContainer{
			{Name: "app", Image: "ghcr.io/owner/repo:1.35.8"},
		},
	}

	// Second call (still within TTL): DSM is scanned live, so new version is picked up immediately.
	result, err = svc.ListSystemUpdates(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].CurrentVersion != "1.35.8" {
		t.Errorf("expected CurrentVersion 1.35.8 after upgrade, got %s", result.Items[0].CurrentVersion)
	}
	if result.Items[0].Status != UpToDate {
		t.Errorf("expected upToDate after upgrade, got %s", result.Items[0].Status)
	}
}

func TestListSystemUpdates_FiltersVersionTags(t *testing.T) {
	svc := newTestServiceWithUpdates(t, &mockDSMBackend{
		conts: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{
				{Name: "app1", Image: "ghcr.io/owner/repo:1.2.3"},
				{Name: "app2", Image: "ghcr.io/owner/repo:latest"},
				{Name: "app3", Image: "ghcr.io/owner/other"},
			},
		},
	})

	result, err := svc.ListSystemUpdates(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only app1 has a version tag; latest and no-tag are excluded.
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Name != "app1" {
		t.Errorf("expected app1, got %s", result.Items[0].Name)
	}
}

func TestListSystemUpdates_StatusFilter(t *testing.T) {
	// The fixture returns tag_name "1.35.8". Use one container matching
	// (upToDate) and one not matching (updateAvailable) to test filtering.
	svc := newTestServiceWithUpdates(t, &mockDSMBackend{
		conts: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{
				{Name: "a", Image: "ghcr.io/owner/repo:1.35.8"},  // matches fixture → upToDate
				{Name: "b", Image: "ghcr.io/other/lib:1.0.0"},    // doesn't match → updateAvailable
			},
		},
	})

	status := UpToDate
	result, err := svc.ListSystemUpdates(context.Background(), &status, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 upToDate item, got %d", len(result.Items))
	}

	status = UpdateAvailable
	result, err = svc.ListSystemUpdates(context.Background(), &status, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 updateAvailable item, got %d", len(result.Items))
	}
}

func TestListSystemUpdates_IDFormat(t *testing.T) {
	svc := newTestServiceWithUpdates(t, &mockDSMBackend{
		conts: &adapters.DSMContainerListResponse{
			Containers: []adapters.DSMContainer{
				{Name: "vaultwarden", Image: "ghcr.io/dani-garcia/vaultwarden:1.32.0"},
			},
		},
	})

	result, err := svc.ListSystemUpdates(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Id != "nas-01.vaultwarden" {
		t.Errorf("expected id nas-01.vaultwarden, got %s", result.Items[0].Id)
	}
}

func TestListSystemUpdates_SkipsNonDockerBackend(t *testing.T) {
	svc := NewService(
		map[string]DSMBackendConfig{
			"nas-01": {
				Backend: &mockDSMBackend{
					conts: &adapters.DSMContainerListResponse{
						Containers: []adapters.DSMContainer{
							{Name: "app", Image: "ghcr.io/owner/repo:v1"},
						},
					},
				},
				DockerEnabled: false,
			},
		},
		map[string]UniFiBackend{},
		config.UpdatesConfig{},
		slog.Default(),
	)

	result, err := svc.ListSystemUpdates(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items when docker disabled, got %d", len(result.Items))
	}
}

// --- Tests: helper functions ---

func TestSplitImageTag(t *testing.T) {
	tests := []struct {
		input     string
		wantImage string
		wantTag   string
	}{
		{"ghcr.io/owner/repo:v1.2.3", "ghcr.io/owner/repo", "v1.2.3"},
		{"nginx:latest", "nginx", "latest"},
		{"nginx", "nginx", ""},
		{"registry.io/img:sha256:abc", "registry.io/img:sha256", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			img, tag := splitImageTag(tt.input)
			if img != tt.wantImage || tag != tt.wantTag {
				t.Errorf("splitImageTag(%q) = (%q, %q), want (%q, %q)", tt.input, img, tag, tt.wantImage, tt.wantTag)
			}
		})
	}
}

func TestIsVersionTag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"1.2.3", true},
		{"v1.0.0", true},
		{"latest", false},
		{"", false},
		{"sha256:abcdef", false},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := isVersionTag(tt.tag); got != tt.want {
				t.Errorf("isVersionTag(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestGithubRepoFromGHCR(t *testing.T) {
	tests := []struct {
		image    string
		wantRepo string
		wantOK   bool
	}{
		{"ghcr.io/immich-app/immich-server", "immich-app/immich-server", true},
		{"ghcr.io/dani-garcia/vaultwarden", "dani-garcia/vaultwarden", true},
		{"docker.io/grafana/grafana", "", false},
		{"ghcr.io/solo", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			repo, ok := githubRepoFromGHCR(tt.image)
			if repo != tt.wantRepo || ok != tt.wantOK {
				t.Errorf("githubRepoFromGHCR(%q) = (%q, %v), want (%q, %v)", tt.image, repo, ok, tt.wantRepo, tt.wantOK)
			}
		})
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
