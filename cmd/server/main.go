package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/config"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

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
			synologyClients[b.Name] = adapters.NewSynologyClient(b.Host, b.Username, b.Password)
		case config.BackendTypeUniFi:
			unifiClients[b.Name] = adapters.NewUniFiClient(b.Host, b.Username, b.Password)
		}
	}

	r := chi.NewRouter()

	// System: all DSM + all UniFi backends.
	dsmBackends := make(map[string]system.DSMBackend, len(synologyClients))
	for name, client := range synologyClients {
		dsmBackends[name] = client
	}
	unifiBackends := make(map[string]system.UniFiBackend, len(unifiClients))
	for name, client := range unifiClients {
		unifiBackends[name] = client
	}
	systemSvc := system.NewService(dsmBackends, unifiBackends)
	systemHandler := system.NewStrictHandler(system.NewHandler(systemSvc), nil)
	system.HandlerFromMux(systemHandler, r)

	// Containers: all Synology backends.
	containerBackends := make(map[string]containers.ContainerBackend, len(synologyClients))
	for name, client := range synologyClients {
		containerBackends[name] = client
	}
	containersSvc := containers.NewService(containerBackends)
	containersHandler := containers.NewStrictHandler(containers.NewHandler(containersSvc), nil)
	containers.HandlerFromMux(containersHandler, r)

	// Storage: all Synology backends.
	storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
	for name, client := range synologyClients {
		storageBackends[name] = client
	}
	storageSvc := storage.NewService(storageBackends)
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
	networkSvc := network.NewService(networkBackends)
	networkHandler := network.NewStrictHandler(network.NewHandler(networkSvc), nil)
	network.HandlerFromMux(networkHandler, r)

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
