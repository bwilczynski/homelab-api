package storage

import (
	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// Service implements storage and backup business logic.
type Service struct {
	storageBackends []storageDeviceBackend
	backupBackends  []backupDeviceBackend
	monitor         adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new storage service with storage and backup backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(storageBackends map[string]StorageBackend, backupBackends map[string]BackupBackend, monitor ...adapters.AvailabilityChecker) *Service {
	svc := &Service{
		storageBackends: newStorageDeviceBackends(storageBackends),
		backupBackends:  newBackupDeviceBackends(backupBackends),
	}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}
