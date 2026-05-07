package system

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// UpdatesDSMBackend is the narrow interface for updates operations.
type UpdatesDSMBackend interface {
	ListContainers() (*adapters.DSMContainerListResponse, error)
}

// githubReleasesCache holds cached GitHub release data indexed by "owner/repo".
type githubReleasesCache struct {
	releases  map[string]*GitHubRelease
	fetchedAt time.Time
}

// SeedGitHubReleases pre-populates the GitHub releases cache.
// Intended for test servers that cannot reach upstream sources.
func (s *Service) SeedGitHubReleases(releases map[string]*GitHubRelease) {
	s.mu.Lock()
	s.ghCache = &githubReleasesCache{releases: releases, fetchedAt: time.Now().UTC()}
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

	items, err := s.buildUpdateItems(ctx, true)
	if err != nil {
		return SystemUpdateList{}, err
	}
	return s.toSystemUpdateList(items, nil, nil), nil
}

// getUpdates always scans DSM live for current container versions and returns assembled
// update details. GitHub release data is served from cache when still within the TTL.
func (s *Service) getUpdates(ctx context.Context) ([]ContainerSystemUpdateDetail, error) {
	return s.buildUpdateItems(ctx, false)
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

// buildUpdateItems scans all Docker-enabled DSM backends live for current container
// versions and assembles update details using GitHub release data.
// When forceGitHub is true the GitHub cache is bypassed; otherwise it is used if fresh.
func (s *Service) buildUpdateItems(_ context.Context, forceGitHub bool) ([]ContainerSystemUpdateDetail, error) {
	checkedAt := time.Now().UTC()

	// Phase 1: always scan DSM live so CurrentVersion reflects the running container.
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

	// Phase 2: get GitHub releases — cached or fresh depending on TTL and forceGitHub.
	releases := s.getOrFetchReleases(repos, forceGitHub)

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
			if normalizeVersion(release.TagName) == normalizeVersion(cc.tag) {
				item.Status = UpToDate
			} else {
				item.Status = UpdateAvailable
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// getOrFetchReleases returns GitHub release data for the given repos.
// When forceGitHub is false it serves from the in-memory cache if still within the TTL.
// Failed fetches preserve the previously cached release for the affected repo so that
// a rate-limit or network error does not downgrade a known status to unknown.
func (s *Service) getOrFetchReleases(repos map[string]struct{}, forceGitHub bool) map[string]*GitHubRelease {
	if !forceGitHub {
		s.mu.RLock()
		if s.ghCache != nil && time.Since(s.ghCache.fetchedAt) < s.updateCacheTTL {
			releases := s.ghCache.releases
			s.mu.RUnlock()
			return releases
		}
		s.mu.RUnlock()

		// Cache stale: acquire refreshMu to prevent a fetch stampede.
		s.refreshMu.Lock()
		defer s.refreshMu.Unlock()

		// Double-check after acquiring lock — another goroutine may have refreshed.
		s.mu.RLock()
		if s.ghCache != nil && time.Since(s.ghCache.fetchedAt) < s.updateCacheTTL {
			releases := s.ghCache.releases
			s.mu.RUnlock()
			return releases
		}
		s.mu.RUnlock()
	}

	// Fetch fresh releases from GitHub.
	fresh := fetchReleases(repos, s.logger)

	// Merge into the previous cache: start with old entries so repos whose fetch
	// failed (e.g. rate-limited) retain their last known release.
	s.mu.Lock()
	prevLen := 0
	if s.ghCache != nil {
		prevLen = len(s.ghCache.releases)
	}
	merged := make(map[string]*GitHubRelease, max(len(fresh), prevLen))
	if s.ghCache != nil {
		maps.Copy(merged, s.ghCache.releases)
	}
	// overwrite with successfully fetched releases
	maps.Copy(merged, fresh)
	s.ghCache = &githubReleasesCache{releases: merged, fetchedAt: time.Now().UTC()}
	s.mu.Unlock()

	return merged
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

// normalizeVersion strips a leading "v" so that "v1.2.3" and "1.2.3" compare equal.
// Some GitHub release tags include the prefix while the corresponding Docker image tag does not.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
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
