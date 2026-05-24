package docker

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// DockerBackend is the combined interface satisfied by the Synology adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type DockerBackend interface {
	ContainersBackend
	NetworksBackend
	ImagesBackend
}

type deviceBackend struct {
	device  string
	backend DockerBackend
}

// Service implements Docker domain business logic.
type Service struct {
	backends []deviceBackend
	logger   *slog.Logger
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new Docker service with one or more backends.
// monitor may be nil; when non-nil, unreachable backends are skipped.
func NewService(backends map[string]DockerBackend, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
	dbs := make([]deviceBackend, 0, len(backends))
	for device, backend := range backends {
		dbs = append(dbs, deviceBackend{device: device, backend: backend})
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].device < dbs[j].device })
	return &Service{backends: dbs, logger: logger, monitor: monitor}
}

func (s *Service) findBackend(device string) (DockerBackend, error) {
	for _, db := range s.backends {
		if db.device == device {
			if !db.backend.SupportsContainers() {
				return nil, fmt.Errorf("device %q does not support docker: %w", device, apierrors.ErrNotFound)
			}
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
}

// parseDockerID splits a composite ID "device.suffix" into its parts.
func parseDockerID(id string) (device, suffix string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid ID %q: expected format device.name: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}
