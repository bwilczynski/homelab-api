package network

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
}

type controllerBackend struct {
	controller string
	unifi      UniFiBackend
}

// Service implements network domain business logic.
type Service struct {
	backends []controllerBackend
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new network service with one or more UniFi backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(backends map[string]UniFiBackend, monitor ...adapters.AvailabilityChecker) *Service {
	cbs := make([]controllerBackend, 0, len(backends))
	for controller, unifi := range backends {
		cbs = append(cbs, controllerBackend{controller: controller, unifi: unifi})
	}
	sort.Slice(cbs, func(i, j int) bool { return cbs[i].controller < cbs[j].controller })
	svc := &Service{backends: cbs}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}

func (s *Service) findBackend(controller string) (UniFiBackend, error) {
	for _, cb := range s.backends {
		if cb.controller == controller {
			return cb.unifi, nil
		}
	}
	return nil, fmt.Errorf("unknown controller %q: %w", controller, apierrors.ErrNotFound)
}

// toKebab converts a display name to kebab-case (lowercase, spaces and special chars → hyphens).
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func toKebab(name string) string {
	lower := strings.ToLower(name)
	kebab := nonAlphanumRe.ReplaceAllString(lower, "-")
	return strings.Trim(kebab, "-")
}

// parseID splits a composite ID "{controller}.{suffix}" into its parts.
func parseID(id string) (controller, suffix string, ok bool) {
	dot := strings.IndexByte(id, '.')
	if dot <= 0 || dot == len(id)-1 {
		return "", "", false
	}
	return id[:dot], id[dot+1:], true
}

func normalizeMac(mac string) string {
	return strings.ToLower(mac)
}
