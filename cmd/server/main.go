package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"

	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/health"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

func main() {
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)
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

	synologyClients, unifiClients := buildClients(cfg, logger)
	logger.Info("backends configured", "synology", len(synologyClients), "unifi", len(unifiClients))
	for _, b := range cfg.Backends {
		logger.Info("backend registered", "name", b.Name, "type", string(b.Type), "host", b.Host)
	}

	discoverAPIs(synologyClients)

	monitor := health.NewMonitor(buildHealthCheckers(synologyClients, unifiClients), 30*time.Second, logger)
	go monitor.Start(context.Background())

	r := chi.NewRouter()
	r.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	// System: all DSM + all UniFi backends.
	dsmBackends := make(map[string]system.DSMBackendConfig, len(synologyClients))
	for name, client := range synologyClients {
		dsmBackends[name] = system.DSMBackendConfig{
			Backend:       client,
			DockerEnabled: client.SupportsContainers(),
		}
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends, cfg.Updates, logger, monitor)
	system.HandlerFromMux(system.NewStrictHandler(system.NewHandler(systemSvc), nil), r)

	// Containers: all Synology backends; capability checked per-request via SupportsContainers.
	containerBackends := make(map[string]containers.ContainerBackend, len(synologyClients))
	for name, client := range synologyClients {
		containerBackends[name] = client
	}
	containersSvc := containers.NewService(containerBackends, monitor)
	containers.HandlerFromMux(containers.NewStrictHandler(containers.NewHandler(containersSvc), nil), r)

	// Storage: all Synology backends.
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends, monitor)
	storage.HandlerFromMux(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), r)

	// Backups: all Synology backends; capability checked per-request via SupportsBackups.
	backupBackends := make(map[string]backups.BackupBackend, len(synologyClients))
	for name, client := range synologyClients {
		backupBackends[name] = client
	}
	backupsSvc := backups.NewService(backupBackends, monitor)
	backups.HandlerFromMux(backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil), r)

	// Network: all UniFi backends.
	networkBackends := make(map[string]network.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		networkBackends[name] = client
	}
	networkSvc := network.NewService(networkBackends, monitor)
	network.HandlerFromMux(network.NewStrictHandler(network.NewHandler(networkSvc), nil), r)

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
