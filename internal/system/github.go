package system

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// GitHubRelease holds the fields we need from the GitHub releases API.
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

var (
	githubClient    = &http.Client{Timeout: 10 * time.Second}
	githubBaseURL   = "https://api.github.com"
	codebergBaseURL = "https://codeberg.org/api/v1"
)

// fetchReleases fetches the latest release for each unique repo concurrently.
// repos maps "owner/repo" to the API base URL for that repo's host.
// Returns a map from "owner/repo" to the release; repos that fail are omitted and logged.
func fetchReleases(repos map[string]string, logger *slog.Logger) map[string]*GitHubRelease {
	results := make(map[string]*GitHubRelease, len(repos))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repo, apiBase := range repos {
		wg.Add(1)
		go func(repo, apiBase string) {
			defer wg.Done()
			release, err := fetchLatestRelease(repo, apiBase)
			mu.Lock()
			if err == nil {
				results[repo] = release
			} else {
				logger.Warn("failed to fetch latest release", "repo", repo, "err", err)
			}
			mu.Unlock()
		}(repo, apiBase)
	}
	wg.Wait()
	return results
}

// fetchLatestRelease calls the releases API for the given "owner/repo" at the given apiBase
// and returns the latest release metadata.
func fetchLatestRelease(repo, apiBase string) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	resp, err := githubClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch release for %s: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, repo)
	}
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release for %s: %w", repo, err)
	}
	return &release, nil
}
