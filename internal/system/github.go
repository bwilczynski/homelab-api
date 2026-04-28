package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// githubRelease holds the fields we need from the GitHub releases API.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

// fetchLatestRelease calls the GitHub releases API for the given "owner/repo"
// and returns the latest release metadata.
func fetchLatestRelease(repo string) (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url) //nolint:noctx
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
