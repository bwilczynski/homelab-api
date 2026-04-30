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
	githubClient  = &http.Client{Timeout: 10 * time.Second}
	githubBaseURL = "https://api.github.com"
)

// fetchReleases fetches the latest GitHub release for each unique repo concurrently.
// Returns a map from "owner/repo" to the release; repos that fail are omitted and logged.
func fetchReleases(repos map[string]struct{}, logger *slog.Logger) map[string]*GitHubRelease {
	results := make(map[string]*GitHubRelease, len(repos))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			release, err := fetchLatestRelease(repo)
			mu.Lock()
			if err == nil {
				results[repo] = release
			} else {
				logger.Warn("failed to fetch latest release", "repo", repo, "err", err)
			}
			mu.Unlock()
		}(repo)
	}
	wg.Wait()
	return results
}

// fetchLatestRelease calls the GitHub releases API for the given "owner/repo"
// and returns the latest release metadata.
func fetchLatestRelease(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubBaseURL, repo)
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
