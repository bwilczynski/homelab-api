package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

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

	// System: all DSM + all UniFi backends.
	synologyBackends := cfg.ByType(config.BackendTypeSynology)
	dsmBackends := make(map[string]system.DSMBackendConfig, len(synologyBackends))
	for _, b := range synologyBackends {
		dsmBackends[b.Name] = system.DSMBackendConfig{
			Backend:       synologyClients[b.Name],
			DockerEnabled: !b.Disabled("docker"),
		}
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends, monitor)
	systemHandler := system.NewStrictHandler(system.NewHandler(systemSvc), nil)
	system.HandlerFromMux(systemHandler, r)

	// Containers: Synology backends with Docker enabled.
	containerBackends := make(map[string]containers.ContainerBackend)
	for _, b := range synologyBackends {
		if !b.Disabled("docker") {
			containerBackends[b.Name] = synologyClients[b.Name]
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

	// Backups: no backends yet.
	backupsSvc := backups.NewService()
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
