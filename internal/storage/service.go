package storage

import (
	"log/slog"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// Service implements storage and backup business logic.
type Service struct {
	storageBackends []storageDeviceBackend
	backupBackends  []backupDeviceBackend
	logger          *slog.Logger
	monitor         adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new storage service with storage and backup backends.
// monitor may be nil; when non-nil, unreachable backends are skipped.
func NewService(storageBackends map[string]StorageBackend, backupBackends map[string]BackupBackend, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
	return &Service{
		storageBackends: newStorageDeviceBackends(storageBackends),
		backupBackends:  newBackupDeviceBackends(backupBackends),
		logger:          logger,
		monitor:         monitor,
	}
}
