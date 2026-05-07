package system

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// DSMBackend is the combined interface satisfied by the Synology adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type DSMBackend interface {
	HealthDSMBackend
	InfoDSMBackend
	UtilizationDSMBackend
	UpdatesDSMBackend
}

// UniFiBackend is the combined interface satisfied by the UniFi adapter.
type UniFiBackend interface {
	HealthUniFiBackend
}

// DSMBackendConfig wraps a DSMBackend with feature flags.
type DSMBackendConfig struct {
	Backend       DSMBackend
	DockerEnabled bool
}

type dsmEntry struct {
	device        string
	dsm           DSMBackend
	dockerEnabled bool
}

type unifiEntry struct {
	controller string
	unifi      UniFiBackend
}

// Service implements system domain business logic.
type Service struct {
	dsmBackends    []dsmEntry
	unifiBackends  []unifiEntry
	monitor        adapters.AvailabilityChecker // optional; nil means all backends available
	sources        map[string]string            // image (without tag) → GitHub "owner/repo"
	updateCacheTTL time.Duration
	logger         *slog.Logger
	warnedImages   map[string]bool // images already warned about missing source
	mu             sync.RWMutex
	ghCache        *githubReleasesCache
	refreshMu      sync.Mutex // serialises GitHub fetches to prevent stampede
}

// NewService creates a new system service with one or more DSM and UniFi backends.
// sources maps container images (without tag) to their GitHub release repos for update checks.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(dsmBackends map[string]DSMBackendConfig, unifiBackends map[string]UniFiBackend, updatesCfg config.UpdatesConfig, logger *slog.Logger, monitor ...adapters.AvailabilityChecker) *Service {
	dsms := make([]dsmEntry, 0, len(dsmBackends))
	for device, cfg := range dsmBackends {
		dsms = append(dsms, dsmEntry{device: device, dsm: cfg.Backend, dockerEnabled: cfg.DockerEnabled})
	}
	sort.Slice(dsms, func(i, j int) bool { return dsms[i].device < dsms[j].device })

	unifis := make([]unifiEntry, 0, len(unifiBackends))
	for controller, unifi := range unifiBackends {
		unifis = append(unifis, unifiEntry{controller: controller, unifi: unifi})
	}
	sort.Slice(unifis, func(i, j int) bool { return unifis[i].controller < unifis[j].controller })

	srcMap := make(map[string]string, len(updatesCfg.Sources))
	for _, s := range updatesCfg.Sources {
		srcMap[s.Image] = s.Source
	}

	ttl := updatesCfg.CheckInterval.Duration
	if ttl <= 0 {
		ttl = time.Hour
	}

	svc := &Service{
		dsmBackends:    dsms,
		unifiBackends:  unifis,
		sources:        srcMap,
		updateCacheTTL: ttl,
		logger:         logger,
		warnedImages:   make(map[string]bool),
	}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}
