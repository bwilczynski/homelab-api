package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/network"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	r := chi.NewRouter()

	synology := adapters.NewSynologyClient(
		os.Getenv("DSM_HOST"),
		os.Getenv("DSM_USER"),
		os.Getenv("DSM_PASS"),
	)

	unifi := adapters.NewUniFiClient(
		os.Getenv("UNIFI_HOST"),
		os.Getenv("UNIFI_USER"),
		os.Getenv("UNIFI_PASS"),
	)

	systemSvc := system.NewService("nas-01", synology, unifi)
	systemHandler := system.NewStrictHandler(system.NewHandler(systemSvc), nil)
	system.HandlerFromMux(systemHandler, r)

	containersSvc := containers.NewService("nas-01", synology)
	containersHandler := containers.NewStrictHandler(containers.NewHandler(containersSvc), nil)
	containers.HandlerFromMux(containersHandler, r)

	storageSvc := storage.NewService("nas-01", synology)
	storageHandler := storage.NewStrictHandler(storage.NewHandler(storageSvc), nil)
	storage.HandlerFromMux(storageHandler, r)

	backupsSvc := backups.NewService()
	backupsHandler := backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil)
	backups.HandlerFromMux(backupsHandler, r)

	networkSvc := network.NewService("unifi", unifi)
	networkHandler := network.NewStrictHandler(network.NewHandler(networkSvc), nil)
	network.HandlerFromMux(networkHandler, r)

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
