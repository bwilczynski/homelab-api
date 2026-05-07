// Command testserver starts a fixture-backed HTTP server for contract testing.
// It wires mock backends loaded from testdata/ JSON fixtures into the same
// domain services and handlers the production server uses. Schemathesis (or any
// HTTP client) can then validate responses against the OpenAPI spec.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/docker"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

// loadFixture reads a JSON file and extracts the .data field from the
// Synology/UniFi response envelope ({"data": T, ...}).
func loadFixture[T any](path string) T {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("read fixture %s: %v", path, err))
	}
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		panic(fmt.Sprintf("parse fixture %s: %v", path, err))
	}
	return envelope.Data
}

// docker.ContainerBackend
type mockContainerBackend struct {
	list      *adapters.DSMContainerListResponse
	detail    *adapters.DSMContainerDetailResponse
	resources *adapters.DSMContainerResourceResponse
}

func (m *mockContainerBackend) checkContainer(name string) error {
	for _, c := range m.list.Containers {
		if c.Name == name {
			return nil
		}
	}
	return fmt.Errorf("container %q: %w", name, apierrors.ErrNotFound)
}

func (m *mockContainerBackend) ListContainers() (*adapters.DSMContainerListResponse, error) {
	return m.list, nil
}

func (m *mockContainerBackend) GetContainer(name string) (*adapters.DSMContainerDetailResponse, error) {
	if err := m.checkContainer(name); err != nil {
		return nil, err
	}
	return m.detail, nil
}

func (m *mockContainerBackend) GetContainerResources() (*adapters.DSMContainerResourceResponse, error) {
	return m.resources, nil
}
func (m *mockContainerBackend) SupportsContainers() bool           { return true }
func (m *mockContainerBackend) StartContainer(name string) error   { return m.checkContainer(name) }
func (m *mockContainerBackend) StopContainer(name string) error    { return m.checkContainer(name) }
func (m *mockContainerBackend) RestartContainer(name string) error { return m.checkContainer(name) }

// system.DSMBackend
type mockDSMBackend struct {
	info       *adapters.DSMSystemInfoResponse
	util       *adapters.DSMSystemUtilizationResponse
	volumes    *adapters.DSMStorageVolumeResponse
	containers *adapters.DSMContainerListResponse
}

func (m *mockDSMBackend) GetSystemInfo() (*adapters.DSMSystemInfoResponse, error) {
	return m.info, nil
}

func (m *mockDSMBackend) GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error) {
	return m.util, nil
}

func (m *mockDSMBackend) GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error) {
	return m.volumes, nil
}

func (m *mockDSMBackend) ListContainers() (*adapters.DSMContainerListResponse, error) {
	return m.containers, nil
}

// system.UniFiBackend
type mockUniFiHealthBackend struct {
	health []adapters.UniFiSubsystemHealth
}

func (m *mockUniFiHealthBackend) GetHealth() ([]adapters.UniFiSubsystemHealth, error) {
	return m.health, nil
}

// storage.StorageBackend
type mockStorageBackend struct {
	volumes *adapters.DSMStorageVolumeResponse
}

func (m *mockStorageBackend) GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error) {
	return m.volumes, nil
}

// storage.BackupBackend
type mockBackupBackend struct {
	tasks      *adapters.DSMBackupTaskListResponse
	taskDetail *adapters.DSMBackupTaskDetailResponse
	taskStatus *adapters.DSMBackupTaskStatusResponse
	target     *adapters.DSMBackupTargetResponse
}

func (m *mockBackupBackend) SupportsBackups() bool    { return true }
func (m *mockBackupBackend) Location() *time.Location { return time.UTC }

func (m *mockBackupBackend) ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error) {
	return m.tasks, nil
}

func (m *mockBackupBackend) GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error) {
	return m.taskDetail, nil
}

func (m *mockBackupBackend) GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error) {
	return m.taskStatus, nil
}

func (m *mockBackupBackend) GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error) {
	return m.target, nil
}

// network.UniFiBackend
type mockNetworkBackend struct {
	devices []adapters.UniFiDevice
	clients []adapters.UniFiSta
}

func (m *mockNetworkBackend) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, nil
}

func (m *mockNetworkBackend) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	base := os.Getenv("FIXTURE_DIR")
	if base == "" {
		base = "internal"
	}

	r := chi.NewRouter()

	// Docker containers
	containerList := new(loadFixture[adapters.DSMContainerListResponse](base + "/docker/testdata/container_list.json"))
	cb := &mockContainerBackend{
		list:      containerList,
		detail:    new(loadFixture[adapters.DSMContainerDetailResponse](base + "/docker/testdata/container_detail.json")),
		resources: new(loadFixture[adapters.DSMContainerResourceResponse](base + "/docker/testdata/container_resources.json")),
	}
	dockerSvc := docker.NewService(map[string]docker.ContainerBackend{"nas-01": cb})
	docker.HandlerWithOptions(docker.NewStrictHandler(docker.NewHandler(dockerSvc), nil), docker.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// System
	dsm := &mockDSMBackend{
		info:       new(loadFixture[adapters.DSMSystemInfoResponse](base + "/system/testdata/dsm-system-info.json")),
		util:       new(loadFixture[adapters.DSMSystemUtilizationResponse](base + "/system/testdata/dsm-system-utilization.json")),
		volumes:    new(loadFixture[adapters.DSMStorageVolumeResponse](base + "/system/testdata/dsm-storage-volumes.json")),
		containers: containerList,
	}
	unifiHealth := &mockUniFiHealthBackend{
		health: loadFixture[[]adapters.UniFiSubsystemHealth](base + "/system/testdata/unifi-health.json"),
	}
	systemSvc := system.NewService(
		map[string]system.DSMBackendConfig{"nas-01": {Backend: dsm, DockerEnabled: true}},
		map[string]system.UniFiBackend{"unifi": unifiHealth},
		config.UpdatesConfig{
			Sources: []config.ImageSourceConfig{
				{Image: "prom/prometheus", Source: "prometheus/prometheus"},
			},
		},
		slog.Default(),
	)
	now := time.Now().UTC()
	systemSvc.SeedGitHubReleases(map[string]*system.GitHubRelease{
		"immich-app/immich-server":           {TagName: "v2.7.0", HTMLURL: "https://github.com/immich-app/immich/releases/tag/v2.7.0", PublishedAt: now},
		"immich-app/immich-machine-learning": {TagName: "v2.6.3", HTMLURL: "https://github.com/immich-app/immich/releases/tag/v2.6.3", PublishedAt: now},
		"prometheus/prometheus":              {TagName: "v3.2.1", HTMLURL: "https://github.com/prometheus/prometheus/releases/tag/v3.2.1", PublishedAt: now},
	})
	system.HandlerWithOptions(system.NewStrictHandler(system.NewHandler(systemSvc), nil), system.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Storage (volumes + backups)
	sb := &mockStorageBackend{
		volumes: new(loadFixture[adapters.DSMStorageVolumeResponse](base + "/storage/testdata/storage_volumes.json")),
	}
	bb := &mockBackupBackend{
		tasks:      new(loadFixture[adapters.DSMBackupTaskListResponse](base + "/storage/testdata/backup_tasks.json")),
		taskDetail: new(loadFixture[adapters.DSMBackupTaskDetailResponse](base + "/storage/testdata/backup_task_detail.json")),
		taskStatus: new(loadFixture[adapters.DSMBackupTaskStatusResponse](base + "/storage/testdata/backup_task_status.json")),
		target:     new(loadFixture[adapters.DSMBackupTargetResponse](base + "/storage/testdata/backup_target.json")),
	}
	storageSvc := storage.NewService(map[string]storage.StorageBackend{"nas-01": sb}, map[string]storage.BackupBackend{"nas-01": bb})
	storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Network
	nb := &mockNetworkBackend{
		devices: loadFixture[[]adapters.UniFiDevice](base + "/network/testdata/unifi-devices.json"),
		clients: loadFixture[[]adapters.UniFiSta](base + "/network/testdata/unifi-clients.json"),
	}
	networkSvc := network.NewService(map[string]network.UniFiBackend{"unifi": nb})
	network.HandlerWithOptions(network.NewStrictHandler(network.NewHandler(networkSvc), nil), network.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	addr := ":" + port()
	logger.Info("starting test server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8081"
}
