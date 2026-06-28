// Command testserver starts a fixture-backed HTTP server for contract testing.
// It wires mock backends loaded from testdata/ JSON fixtures into the same
// domain services and handlers the production server uses. Schemathesis (or any
// HTTP client) can then validate responses against the OpenAPI spec.
package main

import (
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
	"github.com/bwilczynski/homelab-api/internal/meta"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
	"github.com/bwilczynski/homelab-api/internal/testhelpers"
)

// docker.DockerBackend
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
func (m *mockContainerBackend) ListDockerNetworks() (*adapters.DSMDockerNetworkListResponse, error) {
	return &adapters.DSMDockerNetworkListResponse{}, nil
}
func (m *mockContainerBackend) ListDockerImages() (*adapters.DSMDockerImageListResponse, error) {
	return &adapters.DSMDockerImageListResponse{}, nil
}

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
	devices        []adapters.UniFiDevice
	clients        []adapters.UniFiSta
	activeClients  []adapters.UniFiClientV2
	offlineClients []adapters.UniFiClientV2
}

func (m *mockNetworkBackend) GetDevices() ([]adapters.UniFiDevice, error) {
	return m.devices, nil
}

func (m *mockNetworkBackend) GetClients() ([]adapters.UniFiSta, error) {
	return m.clients, nil
}

func (m *mockNetworkBackend) GetActiveClients() ([]adapters.UniFiClientV2, error) {
	return m.activeClients, nil
}

func (m *mockNetworkBackend) GetOfflineClients(_ int) ([]adapters.UniFiClientV2, error) {
	return m.offlineClients, nil
}

func (m *mockNetworkBackend) GetAllClients(_ int) ([]adapters.UniFiClientV2, error) {
	return append(m.activeClients, m.offlineClients...), nil
}

func (m *mockNetworkBackend) GetWlanConf() ([]adapters.UniFiWlanConf, error) {
	return []adapters.UniFiWlanConf{}, nil
}

func (m *mockNetworkBackend) GetNetworkConf() ([]adapters.UniFiNetworkConf, error) {
	return []adapters.UniFiNetworkConf{}, nil
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
	containerList := new(testhelpers.MustLoadFixture[adapters.DSMContainerListResponse](base + "/docker/testdata/container_list.json"))
	cb := &mockContainerBackend{
		list:      containerList,
		detail:    new(testhelpers.MustLoadFixture[adapters.DSMContainerDetailResponse](base + "/docker/testdata/container_detail.json")),
		resources: new(testhelpers.MustLoadFixture[adapters.DSMContainerResourceResponse](base + "/docker/testdata/container_resources.json")),
	}
	dockerSvc := docker.NewService(map[string]docker.DockerBackend{"nas-01": cb}, slog.Default(), nil)
	docker.HandlerWithOptions(docker.NewStrictHandler(docker.NewHandler(dockerSvc), nil), docker.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// System
	dsm := &mockDSMBackend{
		info:       new(testhelpers.MustLoadFixture[adapters.DSMSystemInfoResponse](base + "/system/testdata/dsm-system-info.json")),
		util:       new(testhelpers.MustLoadFixture[adapters.DSMSystemUtilizationResponse](base + "/system/testdata/dsm-system-utilization.json")),
		volumes:    new(testhelpers.MustLoadFixture[adapters.DSMStorageVolumeResponse](base + "/system/testdata/dsm-storage-volumes.json")),
		containers: containerList,
	}
	unifiHealth := &mockUniFiHealthBackend{
		health: testhelpers.MustLoadFixture[[]adapters.UniFiSubsystemHealth](base + "/system/testdata/unifi-health.json"),
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
		nil,
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
		volumes: new(testhelpers.MustLoadFixture[adapters.DSMStorageVolumeResponse](base + "/storage/testdata/storage_volumes.json")),
	}
	bb := &mockBackupBackend{
		tasks:      new(testhelpers.MustLoadFixture[adapters.DSMBackupTaskListResponse](base + "/storage/testdata/backup_tasks.json")),
		taskDetail: new(testhelpers.MustLoadFixture[adapters.DSMBackupTaskDetailResponse](base + "/storage/testdata/backup_task_detail.json")),
		taskStatus: new(testhelpers.MustLoadFixture[adapters.DSMBackupTaskStatusResponse](base + "/storage/testdata/backup_task_status.json")),
		target:     new(testhelpers.MustLoadFixture[adapters.DSMBackupTargetResponse](base + "/storage/testdata/backup_target.json")),
	}
	storageSvc := storage.NewService(map[string]storage.StorageBackend{"nas-01": sb}, map[string]storage.BackupBackend{"nas-01": bb}, logger, nil)
	storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Network
	nb := &mockNetworkBackend{
		devices:        testhelpers.MustLoadFixture[[]adapters.UniFiDevice](base + "/network/testdata/unifi-devices.json"),
		clients:        testhelpers.MustLoadFixture[[]adapters.UniFiSta](base + "/network/testdata/unifi-clients.json"),
		activeClients:  testhelpers.MustLoadFixture[[]adapters.UniFiClientV2](base + "/network/testdata/unifi-v2-active.json"),
		offlineClients: testhelpers.MustLoadFixture[[]adapters.UniFiClientV2](base + "/network/testdata/unifi-v2-history.json"),
	}
	networkSvc := network.NewService(map[string]network.UniFiBackend{"unifi": nb}, 30, slog.Default(), nil)
	network.HandlerWithOptions(network.NewStrictHandler(network.NewHandler(networkSvc), nil), network.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
	})

	// Meta (auth, version)
	metaSvc := meta.NewService("test", "test", false, "")
	meta.HandlerWithOptions(
		meta.NewStrictHandler(meta.NewHandler(metaSvc), nil),
		meta.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
		},
	)

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
