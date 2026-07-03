package storage

import (
	"log/slog"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/registry"
)

// Service implements storage and backup business logic.
type Service struct {
	storageBackends []registry.Entry[StorageBackend]
	backupBackends  []registry.Entry[BackupBackend]
	logger          *slog.Logger
	monitor         adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new storage service with storage and backup backends.
// monitor may be nil; when non-nil, unreachable backends are skipped.
func NewService(storageBackends map[string]StorageBackend, backupBackends map[string]BackupBackend, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
	return &Service{
		storageBackends: registry.New(storageBackends),
		backupBackends:  registry.New(backupBackends),
		logger:          logger,
		monitor:         monitor,
	}
}
