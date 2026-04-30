package system

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// DSMBackend defines the adapter interface for DSM system operations.
type DSMBackend interface {
	GetSystemInfo() (*adapters.DSMSystemInfoResponse, error)
	GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error)
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
	ListContainers() (*adapters.DSMContainerListResponse, error)
}

// UniFiBackend defines the adapter interface for UniFi system operations.
type UniFiBackend interface {
	GetHealth() ([]adapters.UniFiSubsystemHealth, error)
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

// updateCache holds the cached update results and when they were fetched.
type updateCache struct {
	items     []ContainerSystemUpdateDetail
	checkedAt time.Time
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
	cache          *updateCache
	refreshMu      sync.Mutex // serialises refreshUpdates calls to prevent stampede
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

// GetSystemHealth queries all backends for health and assembles an aggregate Health model.
// The top-level status is the worst status across all components.
func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	var components []ComponentHealth
	overall := Healthy

	// UniFi subsystems (gateway, wan, lan, wlan, www, vpn, …).
	for _, ue := range s.unifiBackends {
		if s.monitor != nil && !s.monitor.Available(ue.controller) {
			name := "network"
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":network"
			}
			msg := "offline"
			components = append(components, ComponentHealth{Name: name, Status: Unhealthy, Message: &msg})
			overall = Unhealthy
			continue
		}

		subsystems, err := ue.unifi.GetHealth()
		if err != nil {
			return Health{}, fmt.Errorf("get unifi health from %s: %w", ue.controller, err)
		}
		for _, sub := range subsystems {
			status := mapUniFiStatus(sub.Status)
			name := sub.Subsystem
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":" + name
			}
			components = append(components, ComponentHealth{
				Name:   name,
				Status: status,
			})
			overall = worstStatus(overall, status)
		}
	}

	// DSM storage volumes and containers per device.
	for _, de := range s.dsmBackends {
		prefix := ""
		if len(s.dsmBackends) > 1 {
			prefix = de.device + ":"
		}

		if s.monitor != nil && !s.monitor.Available(de.device) {
			msg := "offline"
			components = append(components, ComponentHealth{Name: prefix + "storage", Status: Unhealthy, Message: &msg})
			if de.dockerEnabled {
				components = append(components, ComponentHealth{Name: prefix + "containers", Status: Unhealthy, Message: &msg})
			}
			overall = Unhealthy
			continue
		}

		storageStatus, storageMsg, err := storageHealth(de.dsm)
		if err != nil {
			return Health{}, fmt.Errorf("get storage health from %s: %w", de.device, err)
		}
		storageComponent := ComponentHealth{Name: prefix + "storage", Status: storageStatus}
		if storageMsg != "" {
			storageComponent.Message = &storageMsg
		}
		components = append(components, storageComponent)
		overall = worstStatus(overall, storageStatus)

		if de.dockerEnabled {
			containersStatus, containersMsg, err := containersHealth(de.dsm)
			if err != nil {
				return Health{}, fmt.Errorf("get containers health from %s: %w", de.device, err)
			}
			containersComponent := ComponentHealth{Name: prefix + "containers", Status: containersStatus}
			if containersMsg != "" {
				containersComponent.Message = &containersMsg
			}
			components = append(components, containersComponent)
			overall = worstStatus(overall, containersStatus)
		}
	}

	if components == nil {
		components = []ComponentHealth{}
	}

	return Health{
		Status:     overall,
		CheckedAt:  time.Now().UTC(),
		Components: components,
	}, nil
}

// storageHealth derives a single HealthStatus from DSM volume statuses.
func storageHealth(dsm DSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.GetStorageVolumes()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
	}
	worst := Healthy
	var degraded, crashed []string
	for _, v := range resp.Volumes {
		st := mapVolumeStatus(v.Status)
		worst = worstStatus(worst, st)
		switch st {
		case Unhealthy:
			crashed = append(crashed, v.ID)
		case Degraded:
			degraded = append(degraded, v.ID)
		}
	}
	var msg string
	if len(crashed) > 0 {
		msg = fmt.Sprintf("crashed: %s", strings.Join(crashed, ", "))
	} else if len(degraded) > 0 {
		msg = fmt.Sprintf("degraded: %s", strings.Join(degraded, ", "))
	}
	return worst, msg, nil
}

// containersHealth derives a single HealthStatus from DSM container states.
func containersHealth(dsm DSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.ListContainers()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
	}
	notRunning := 0
	for _, c := range resp.Containers {
		if !c.State.Running {
			notRunning++
		}
	}
	if notRunning == 0 {
		return Healthy, "", nil
	}
	return Degraded, fmt.Sprintf("%d container(s) not running", notRunning), nil
}

// ListSystemInfo queries all DSM backends for static system information.
func (s *Service) ListSystemInfo(ctx context.Context, device *string) (SystemInfoList, error) {
	var items []SystemInfo
	for _, de := range s.dsmBackends {
		if device != nil && *device != de.device {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}

		info, err := de.dsm.GetSystemInfo()
		if err != nil {
			return SystemInfoList{}, fmt.Errorf("get system info from %s: %w", de.device, err)
		}

		uptimeSecs, err := parseUptime(info.UpTime)
		if err != nil {
			uptimeSecs = 0
		}

		items = append(items, SystemInfo{
			Device:        de.device,
			Model:         info.Model,
			Firmware:      info.FirmwareVer,
			RamMb:         info.RamSize,
			UptimeSeconds: uptimeSecs,
		})
	}
	if items == nil {
		items = []SystemInfo{}
	}
	return SystemInfoList{Items: items}, nil
}

// ListSystemUtilization queries all DSM backends for live utilization data.
func (s *Service) ListSystemUtilization(ctx context.Context, device *string) (SystemUtilizationList, error) {
	var items []SystemUtilization
	for _, de := range s.dsmBackends {
		if device != nil && *device != de.device {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}

		util, err := de.dsm.GetSystemUtilization()
		if err != nil {
			return SystemUtilizationList{}, fmt.Errorf("get system utilization from %s: %w", de.device, err)
		}

		// Memory: DSM reports in KB, API uses bytes.
		const kbToBytes = 1024
		totalBytes := int64(util.Memory.TotalReal) * kbToBytes
		availBytes := int64(util.Memory.AvailReal) * kbToBytes
		totalSwap := int64(util.Memory.TotalSwap) * kbToBytes
		usedSwap := (int64(util.Memory.TotalSwap) - int64(util.Memory.AvailSwap)) * kbToBytes

		// CPU: sum user + system for total.
		cpuTotal := util.CPU.UserLoad + util.CPU.SystemLoad + util.CPU.OtherLoad

		// Network: skip "total" aggregate device.
		network := make([]NetworkInterfaceUsage, 0, len(util.Network))
		for _, n := range util.Network {
			if n.Device == "total" {
				continue
			}
			network = append(network, NetworkInterfaceUsage{
				Name:          n.Device,
				RxBytesPerSec: n.Rx,
				TxBytesPerSec: n.Tx,
			})
		}

		// Disks: map each individual disk.
		disks := make([]DiskIo, 0, len(util.Disk.Disk))
		for _, d := range util.Disk.Disk {
			disks = append(disks, DiskIo{
				Name:           d.Device,
				ReadOpsPerSec:  d.ReadAccess,
				WriteOpsPerSec: d.WriteAccess,
			})
		}

		items = append(items, SystemUtilization{
			Device:    de.device,
			SampledAt: time.Now().UTC(),
			Cpu: CpuUsage{
				UserPercent:   util.CPU.UserLoad,
				SystemPercent: util.CPU.SystemLoad,
				TotalPercent:  cpuTotal,
			},
			Memory: MemoryUsage{
				TotalBytes:     totalBytes,
				AvailableBytes: availBytes,
				UsedPercent:    util.Memory.RealUsage,
				SwapTotalBytes: totalSwap,
				SwapUsedBytes:  usedSwap,
			},
			Network: network,
			Disks:   disks,
		})
	}
	if items == nil {
		items = []SystemUtilization{}
	}
	return SystemUtilizationList{Items: items}, nil
}

// worstStatus returns the more severe of two HealthStatus values.
func worstStatus(a, b HealthStatus) HealthStatus {
	if a == Unhealthy || b == Unhealthy {
		return Unhealthy
	}
	if a == Degraded || b == Degraded {
		return Degraded
	}
	return Healthy
}

// mapUniFiStatus converts a UniFi subsystem status string to HealthStatus.
func mapUniFiStatus(status string) HealthStatus {
	switch status {
	case "ok":
		return Healthy
	case "unknown":
		return Degraded
	default:
		return Unhealthy
	}
}

// mapVolumeStatus converts a DSM volume status string to HealthStatus.
func mapVolumeStatus(status string) HealthStatus {
	switch status {
	case "normal":
		return Healthy
	case "degraded", "repairing":
		return Degraded
	default:
		return Unhealthy
	}
}

// SeedUpdateCache pre-populates the update cache with the given items.
// Intended for test servers that cannot reach upstream sources.
func (s *Service) SeedUpdateCache(items []ContainerSystemUpdateDetail) {
	s.mu.Lock()
	s.cache = &updateCache{items: items, checkedAt: time.Now().UTC()}
	s.mu.Unlock()
}

// ListSystemUpdates returns tracked containers and their update status.
// Results are served from an in-memory cache; TTL is configured via check_interval (default 1h).
func (s *Service) ListSystemUpdates(ctx context.Context, status *SystemUpdateStatus, updateType *SystemUpdateType) (SystemUpdateList, error) {
	items, err := s.getUpdates(ctx)
	if err != nil {
		return SystemUpdateList{}, err
	}
	return s.toSystemUpdateList(items, status, updateType), nil
}

// GetSystemUpdate returns detailed update info for a single tracked component.
func (s *Service) GetSystemUpdate(ctx context.Context, id string) (*SystemUpdateDetail, error) {
	items, err := s.getUpdates(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Id == id {
			var detail SystemUpdateDetail
			if err := detail.FromContainerSystemUpdateDetail(item); err != nil {
				return nil, fmt.Errorf("marshal update detail for %s: %w", id, err)
			}
			return &detail, nil
		}
	}
	return nil, nil // not found
}

// CheckSystemUpdates forces a fresh upstream check and returns the full list.
func (s *Service) CheckSystemUpdates(ctx context.Context) (SystemUpdateList, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	items, err := s.refreshUpdates(ctx)
	if err != nil {
		return SystemUpdateList{}, err
	}
	s.mu.Lock()
	s.cache = &updateCache{items: items, checkedAt: time.Now().UTC()}
	s.mu.Unlock()
	return s.toSystemUpdateList(items, nil, nil), nil
}

// getUpdates returns cached update data, refreshing if the cache is stale.
// Uses refreshMu to ensure only one goroutine refreshes at a time.
func (s *Service) getUpdates(ctx context.Context) ([]ContainerSystemUpdateDetail, error) {
	s.mu.RLock()
	if s.cache != nil && time.Since(s.cache.checkedAt) < s.updateCacheTTL {
		items := s.cache.items
		s.mu.RUnlock()
		return items, nil
	}
	s.mu.RUnlock()

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	// Re-check after acquiring lock — another goroutine may have refreshed.
	s.mu.RLock()
	if s.cache != nil && time.Since(s.cache.checkedAt) < s.updateCacheTTL {
		items := s.cache.items
		s.mu.RUnlock()
		return items, nil
	}
	s.mu.RUnlock()

	items, err := s.refreshUpdates(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache = &updateCache{items: items, checkedAt: time.Now().UTC()}
	s.mu.Unlock()
	return items, nil
}

// containerCandidate holds pre-resolved data for a container before GitHub lookup.
type containerCandidate struct {
	device    string
	name      string
	image     string
	tag       string
	repo      string // GitHub "owner/repo"; empty if unresolved
	sourceURL string
}

// refreshUpdates scans all Docker-enabled DSM backends for containers with version
// tags, looks up the GitHub release source for each, and returns assembled details.
func (s *Service) refreshUpdates(_ context.Context) ([]ContainerSystemUpdateDetail, error) {
	checkedAt := time.Now().UTC()

	// Phase 1: collect candidates and unique repos to fetch.
	var candidates []containerCandidate
	repos := make(map[string]struct{})

	for _, de := range s.dsmBackends {
		if !de.dockerEnabled {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}
		resp, err := de.dsm.ListContainers()
		if err != nil {
			continue
		}
		for _, c := range resp.Containers {
			image, tag := splitImageTag(c.Image)
			if !isVersionTag(tag) {
				continue
			}

			githubRepo, sourceURL := s.resolveSource(image)
			if githubRepo == "" && !s.warnedImages[image] {
				s.warnedImages[image] = true
				s.logger.Warn("no GitHub source for container image; update status will be unknown",
					"container", c.Name,
					"image", image,
					"hint", "add an entry under updates.sources in config.yaml",
				)
			}
			if githubRepo != "" {
				repos[githubRepo] = struct{}{}
			}

			candidates = append(candidates, containerCandidate{
				device: de.device, name: c.Name,
				image: image, tag: tag,
				repo: githubRepo, sourceURL: sourceURL,
			})
		}
	}

	// Phase 2: fetch all unique repos concurrently.
	releases := fetchReleases(repos, s.logger)

	// Build a lookup from the previous cache so we can preserve release data
	// for repos where the GitHub API failed (e.g. rate-limited).
	prevByID := make(map[string]ContainerSystemUpdateDetail)
	s.mu.RLock()
	if s.cache != nil {
		for _, item := range s.cache.items {
			prevByID[item.Id] = item
		}
	}
	s.mu.RUnlock()

	// Phase 3: assemble results.
	items := make([]ContainerSystemUpdateDetail, 0, len(candidates))
	for _, cc := range candidates {
		item := ContainerSystemUpdateDetail{
			Id:             cc.device + "." + cc.name,
			Name:           cc.name,
			Type:           ContainerSystemUpdateDetailTypeContainer,
			Status:         Unknown,
			CurrentVersion: cc.tag,
			LatestVersion:  cc.tag,
			CheckedAt:      checkedAt,
			Image:          cc.image,
			Device:         cc.device,
			Source:         cc.sourceURL,
			ReleaseUrl:     cc.sourceURL + "/releases",
			PublishedAt:    checkedAt,
		}

		if release, ok := releases[cc.repo]; ok {
			item.LatestVersion = release.TagName
			item.ReleaseUrl = release.HTMLURL
			item.PublishedAt = release.PublishedAt
			if release.TagName == cc.tag {
				item.Status = UpToDate
			} else {
				item.Status = UpdateAvailable
			}
		} else if prev, ok := prevByID[item.Id]; ok && prev.Status != Unknown {
			// GitHub API failed — preserve previous release data instead of downgrading to unknown.
			item.Status = prev.Status
			item.LatestVersion = prev.LatestVersion
			item.ReleaseUrl = prev.ReleaseUrl
			item.PublishedAt = prev.PublishedAt
		}

		items = append(items, item)
	}

	return items, nil
}

// resolveSource returns the GitHub "owner/repo" and a source URL for the given image.
// It first checks the explicit sources map, then falls back to ghcr.io auto-derivation.
func (s *Service) resolveSource(image string) (githubRepo string, sourceURL string) {
	if repo, ok := s.sources[image]; ok {
		return repo, fmt.Sprintf("https://github.com/%s", repo)
	}
	if repo, ok := githubRepoFromGHCR(image); ok {
		return repo, fmt.Sprintf("https://github.com/%s", repo)
	}
	return "", "https://github.com"
}

// githubRepoFromGHCR derives a GitHub "owner/repo" from a ghcr.io image reference.
func githubRepoFromGHCR(image string) (string, bool) {
	const prefix = "ghcr.io/"
	if !strings.HasPrefix(image, prefix) {
		return "", false
	}
	rest := image[len(prefix):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// splitImageTag splits "registry/image:tag" into ("registry/image", "tag").
// Returns ("image", "") if there is no tag separator.
func splitImageTag(image string) (string, string) {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return image, ""
	}
	return image[:i], image[i+1:]
}

// isVersionTag returns true when the tag looks like a version (not empty, not "latest",
// not a SHA digest).
func isVersionTag(tag string) bool {
	if tag == "" || tag == "latest" {
		return false
	}
	if strings.HasPrefix(tag, "sha256:") {
		return false
	}
	return true
}

// toSystemUpdateList maps detail items to the base SystemUpdate list, applying optional filters.
func (s *Service) toSystemUpdateList(items []ContainerSystemUpdateDetail, status *SystemUpdateStatus, updateType *SystemUpdateType) SystemUpdateList {
	result := make([]SystemUpdate, 0, len(items))
	for _, item := range items {
		if status != nil && item.Status != *status {
			continue
		}
		if updateType != nil && SystemUpdateType(item.Type) != *updateType {
			continue
		}
		result = append(result, SystemUpdate{
			Id:             item.Id,
			Name:           item.Name,
			Type:           SystemUpdateType(item.Type),
			Status:         item.Status,
			Device:         item.Device,
			CurrentVersion: item.CurrentVersion,
			LatestVersion:  item.LatestVersion,
			CheckedAt:      item.CheckedAt,
		})
	}
	return SystemUpdateList{Items: result}
}

// parseUptime converts a DSM uptime string "H:M:S" to total seconds.
func parseUptime(s string) (int64, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("unexpected uptime format %q", s)
	}
	h, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	m, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, err
	}
	sec, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, err
	}
	return h*3600 + m*60 + sec, nil
}
