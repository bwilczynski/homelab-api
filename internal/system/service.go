package system

import (
	"fmt"
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

// imageSource holds the resolved release source for a container image.
type imageSource struct {
	repo      string // "owner/repo"
	apiBase   string // API base URL, e.g. "https://api.github.com" or "https://codeberg.org/api/v1"
	sourceURL string // human-facing URL, e.g. "https://github.com/owner/repo"
}

// Service implements system domain business logic.
type Service struct {
	dsmBackends    []dsmEntry
	unifiBackends  []unifiEntry
	monitor        adapters.AvailabilityChecker // optional; nil means all backends available
	sources        map[string]imageSource       // image (without tag) → release source
	updateCacheTTL time.Duration
	logger         *slog.Logger
	warnedImages   map[string]bool // images already warned about missing source
	mu             sync.RWMutex
	ghCache        *githubReleasesCache
	refreshMu      sync.Mutex // serialises release fetches to prevent stampede
}

// NewService creates a new system service with one or more DSM and UniFi backends.
// sources maps container images (without tag) to their GitHub release repos for update checks.
// monitor may be nil; when non-nil, unreachable backends are skipped.
func NewService(dsmBackends map[string]DSMBackendConfig, unifiBackends map[string]UniFiBackend, updatesCfg config.UpdatesConfig, logger *slog.Logger, monitor adapters.AvailabilityChecker) *Service {
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

	srcMap := make(map[string]imageSource, len(updatesCfg.Sources))
	for _, s := range updatesCfg.Sources {
		apiBase := githubBaseURL
		sourceURL := fmt.Sprintf("https://github.com/%s", s.Source)
		if s.Type == "codeberg" {
			apiBase = codebergBaseURL
			sourceURL = fmt.Sprintf("https://codeberg.org/%s", s.Source)
		}
		srcMap[s.Image] = imageSource{repo: s.Source, apiBase: apiBase, sourceURL: sourceURL}
	}

	ttl := updatesCfg.CheckInterval.Duration
	if ttl <= 0 {
		ttl = time.Hour
	}

	return &Service{
		dsmBackends:    dsms,
		unifiBackends:  unifis,
		sources:        srcMap,
		updateCacheTTL: ttl,
		logger:         logger,
		warnedImages:   make(map[string]bool),
		monitor:        monitor,
	}
}
