package network

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
	TopologyBackend
	SSIDsBackend
	VLANsBackend
	WANsBackend
}

// Service implements network domain business logic.
type Service struct {
	backends    adapters.Registry[UniFiBackend]
	logger      *slog.Logger
	monitor     adapters.AvailabilityChecker // optional; nil means all backends available
	historyDays map[string]int               // controller name → days of offline client history
}

// NewService creates a new network service with one or more UniFi backends.
// monitor may be nil; when non-nil, unreachable backends are skipped.
// historyDays maps controller name to days of offline client history (default 30 if missing/zero).
func NewService(backends map[string]UniFiBackend, historyDays map[string]int, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
	days := make(map[string]int, len(backends))
	for controller := range backends {
		d := historyDays[controller]
		if d <= 0 {
			d = 30
		}
		days[controller] = d
	}
	return &Service{backends: adapters.NewRegistry(backends), historyDays: days, logger: logger, monitor: monitor}
}

func (s *Service) findBackend(controller string) (UniFiBackend, error) {
	backend, ok := s.backends.Find(controller)
	if !ok {
		return nil, fmt.Errorf("unknown controller %q: %w", controller, apierrors.ErrNotFound)
	}
	return backend, nil
}

// toKebab converts a display name to kebab-case (lowercase, spaces and special chars → hyphens).
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func toKebab(name string) string {
	lower := strings.ToLower(name)
	kebab := nonAlphanumRe.ReplaceAllString(lower, "-")
	return strings.Trim(kebab, "-")
}

// parseID splits a composite ID "{controller}.{suffix}" into its parts.
func parseID(id string) (controller, suffix string, err error) {
	return apierrors.ParseCompositeID(id, "ID", "controller.suffix")
}

func normalizeMac(mac string) string {
	return strings.ToLower(mac)
}
