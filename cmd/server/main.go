package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/bwilczynski/homelab-api/internal/backups"
	"github.com/bwilczynski/homelab-api/internal/containers"
	"github.com/bwilczynski/homelab-api/internal/storage"
	"github.com/bwilczynski/homelab-api/internal/system"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	r := chi.NewRouter()

	systemSvc := system.NewService()
	systemHandler := system.NewStrictHandler(system.NewHandler(systemSvc), nil)
	system.HandlerFromMux(systemHandler, r)

	containersSvc := containers.NewService()
	containersHandler := containers.NewStrictHandler(containers.NewHandler(containersSvc), nil)
	containers.HandlerFromMux(containersHandler, r)

	storageSvc := storage.NewService()
	storageHandler := storage.NewStrictHandler(storage.NewHandler(storageSvc), nil)
	storage.HandlerFromMux(storageHandler, r)

	backupsSvc := backups.NewService()
	backupsHandler := backups.NewStrictHandler(backups.NewHandler(backupsSvc), nil)
	backups.HandlerFromMux(backupsHandler, r)

	addr := ":8080"
	logger.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}
