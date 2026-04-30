package system

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func loadGitHubReleaseFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/github-release-latest.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestFetchLatestRelease(t *testing.T) {
	fixture := loadGitHubReleaseFixture(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/dani-garcia/vaultwarden/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	overrideGitHubClient(t, srv)

	release, err := fetchLatestRelease("dani-garcia/vaultwarden")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if release.TagName != "1.35.8" {
		t.Errorf("expected tag_name 1.35.8, got %s", release.TagName)
	}
	if release.HTMLURL != "https://github.com/dani-garcia/vaultwarden/releases/tag/1.35.8" {
		t.Errorf("unexpected html_url: %s", release.HTMLURL)
	}
	if release.PublishedAt.IsZero() {
		t.Error("expected non-zero published_at")
	}
}

func TestFetchLatestRelease_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	overrideGitHubClient(t, srv)

	_, err := fetchLatestRelease("no-such/repo")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestFetchReleases_Deduplicates(t *testing.T) {
	callCount := 0
	fixture := loadGitHubReleaseFixture(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	overrideGitHubClient(t, srv)

	repos := map[string]struct{}{
		"dani-garcia/vaultwarden": {},
		"grafana/grafana":         {},
	}
	results := fetchReleases(repos, slog.Default())

	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for 2 unique repos, got %d", callCount)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestFetchLatestRelease_ParsesAllFields(t *testing.T) {
	fixture := loadGitHubReleaseFixture(t)

	// Verify the fixture deserialises into our struct correctly by comparing
	// against a full JSON decode of all fields.
	var full map[string]json.RawMessage
	if err := json.Unmarshal(fixture, &full); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal(fixture, &release); err != nil {
		t.Fatalf("unmarshal into GitHubRelease: %v", err)
	}

	// tag_name
	var expectedTag string
	json.Unmarshal(full["tag_name"], &expectedTag)
	if release.TagName != expectedTag {
		t.Errorf("tag_name: got %q, want %q", release.TagName, expectedTag)
	}

	// html_url
	var expectedURL string
	json.Unmarshal(full["html_url"], &expectedURL)
	if release.HTMLURL != expectedURL {
		t.Errorf("html_url: got %q, want %q", release.HTMLURL, expectedURL)
	}

	// published_at
	if release.PublishedAt.IsZero() {
		t.Error("published_at should not be zero")
	}
}
