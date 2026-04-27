package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/health"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Build adapter clients from config.
	synologyClients := make(map[string]*adapters.SynologyClient)
	unifiClients := make(map[string]*adapters.UniFiClient)

	for _, b := range cfg.Backends {
		switch b.Type {
		case config.BackendTypeSynology:
			synologyClients[b.Name] = adapters.NewSynologyClient(b.Host, b.Username, b.Password, b.AuthVersion, b.InsecureTLS)
		case config.BackendTypeUniFi:
			unifiClients[b.Name] = adapters.NewUniFiClient(b.Host, b.Username, b.Password, b.InsecureTLS)
		}
	}

	logger.Info("backends configured",
		"synology", len(synologyClients),
		"unifi", len(unifiClients),
	)
	for _, b := range cfg.Backends {
		logger.Info("backend registered", "name", b.Name, "type", string(b.Type), "host", b.Host)
	}

	// Discover supported APIs on each backend in parallel so capability checks
	// are local (no per-request round-trip to a non-existent API).
	var wg sync.WaitGroup
	for name, client := range synologyClients {
		wg.Add(1)
		go func(name string, client *adapters.SynologyClient) {
			defer wg.Done()
			if err := client.DiscoverAPIs(); err != nil {
				logger.Warn("API discovery failed, assuming all APIs available", "backend", name, "err", err)
			} else {
				logger.Info("API discovery complete",
					"backend", name,
					"docker", client.SupportsAPI(adapters.APISynoDockerContainer),
					"backup", client.SupportsAPI(adapters.APISynoBackupTask),
				)
			}
		}(name, client)
	}
	wg.Wait()

	// Build the health monitor covering all backends.
	healthCheckers := make(map[string]adapters.HealthChecker, len(synologyClients)+len(unifiClients))
	for name, c := range synologyClients {
		healthCheckers[name] = c
	}
	for name, c := range unifiClients {
		healthCheckers[name] = c
	}
	monitor := health.NewMonitor(healthCheckers, 30*time.Second, logger)

	// Start the health monitor in the background.
	go monitor.Start(context.Background())

	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	// System: all DSM + all UniFi backends.
	synologyBackends := cfg.ByType(config.BackendTypeSynology)
	dsmBackends := make(map[string]system.DSMBackendConfig, len(synologyBackends))
	for _, b := range synologyBackends {
		client := synologyClients[b.Name]
		dsmBackends[b.Name] = system.DSMBackendConfig{
			Backend:       client,
			DockerEnabled: client.SupportsAPI(adapters.APISynoDockerContainer),
		}
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends, monitor)
	systemHandler := system.NewStrictHandler(system.NewHandler(systemSvc), nil)
	system.HandlerFromMux(systemHandler, r)

	// Containers: Synology backends that support Docker.
	containerBackends := make(map[string]containers.ContainerBackend)
	for name, client := range synologyClients {
		if client.SupportsAPI(adapters.APISynoDockerContainer) {
			containerBackends[name] = client
		}
	}
	containersSvc := containers.NewService(containerBackends, monitor)
	containersHandler := containers.NewStrictHandler(containers.NewHandler(containersSvc), nil)
	containers.HandlerFromMux(containersHandler, r)

	// Storage: all Synology backends.
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends, monitor)
	storageHandler := storage.NewStrictHandler(storage.NewHandler(storageSvc), nil)
	storage.HandlerFromMux(storageHandler, r)

	// Backups: Synology backends that support Hyper Backup.
	backupBackends := make(map[string]backups.BackupBackend)
	for name, client := range synologyClients {
		if client.SupportsAPI(adapters.APISynoBackupTask) {
			backupBackends[name] = client
		}
	}
	backupsSvc := backups.NewService(backupBackends, monitor)
	backupsHandler := backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil)
	backups.HandlerFromMux(backupsHandler, r)

	// Network: all UniFi backends.
	networkBackends := make(map[string]network.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		networkBackends[name] = client
	}
	networkSvc := network.NewService(networkBackends, monitor)
	networkHandler := network.NewStrictHandler(network.NewHandler(networkSvc), nil)
	network.HandlerFromMux(networkHandler, r)

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
