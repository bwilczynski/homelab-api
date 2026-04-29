package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// githubRelease holds the fields we need from the GitHub releases API.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

var (
	githubClient  = &http.Client{Timeout: 10 * time.Second}
	githubBaseURL = "https://api.github.com"
)

// fetchReleases fetches the latest GitHub release for each unique repo concurrently.
// Returns a map from "owner/repo" to the release (nil on error).
func fetchReleases(repos map[string]struct{}) map[string]*githubRelease {
	results := make(map[string]*githubRelease, len(repos))
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
			}
			mu.Unlock()
		}(repo)
	}
	wg.Wait()
	return results
}

// fetchLatestRelease calls the GitHub releases API for the given "owner/repo"
// and returns the latest release metadata.
func fetchLatestRelease(repo string) (*githubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubBaseURL, repo)
	resp, err := githubClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch release for %s: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, repo)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release for %s: %w", repo, err)
	}
	return &release, nil
}
