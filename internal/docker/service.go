package docker

import (
	"fmt"
	"log/slog"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/registry"
)

// DockerBackend is the combined interface satisfied by the Synology adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type DockerBackend interface {
	ContainersBackend
	NetworksBackend
	ImagesBackend
}

// Service implements Docker domain business logic.
type Service struct {
	backends []registry.Entry[DockerBackend]
	logger   *slog.Logger
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new Docker service with one or more backends.
// monitor may be nil; when non-nil, unreachable backends are skipped.
func NewService(backends map[string]DockerBackend, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
	return &Service{backends: registry.New(backends), logger: logger, monitor: monitor}
}

func (s *Service) findBackend(device string) (DockerBackend, error) {
	backend, ok := registry.Find(s.backends, device)
	if !ok {
		return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
	}
	if !backend.SupportsContainers() {
		return nil, fmt.Errorf("device %q does not support docker: %w", device, apierrors.ErrNotFound)
	}
	return backend, nil
}

// parseDockerID splits a composite ID "device.suffix" into its parts.
func parseDockerID(id string) (device, suffix string, err error) {
	return apierrors.ParseCompositeID(id, "ID", "device.name")
}
