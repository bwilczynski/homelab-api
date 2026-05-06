package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValid(t *testing.T) {
	cfg := writeTemp(t, `
backends:
  - name: nas-01
    type: synology
    host: 192.168.1.10
    username: admin
    password: secret
  - name: unifi
    type: unifi
    host: 192.168.1.1
    username: admin
    password: secret2
`)

	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(c.Backends))
	}
	if c.Backends[0].Name != "nas-01" {
		t.Errorf("expected name nas-01, got %s", c.Backends[0].Name)
	}
	if c.Backends[0].Type != BackendTypeSynology {
		t.Errorf("expected type synology, got %s", c.Backends[0].Type)
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	t.Setenv("TEST_HOST", "10.0.0.1")
	t.Setenv("TEST_PASS", "s3cret")

	cfg := writeTemp(t, `
backends:
  - name: nas
    type: synology
    host: ${TEST_HOST}
    username: admin
    password: ${TEST_PASS}
`)

	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Backends[0].Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", c.Backends[0].Host)
	}
	if c.Backends[0].Password != "s3cret" {
		t.Errorf("expected password s3cret, got %s", c.Backends[0].Password)
	}
}

func TestLoadDuplicateName(t *testing.T) {
	cfg := writeTemp(t, `
backends:
  - name: nas-01
    type: synology
    host: a
    username: u
    password: p
  - name: nas-01
    type: synology
    host: b
    username: u
    password: p
`)

	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestLoadMissingName(t *testing.T) {
	cfg := writeTemp(t, `
backends:
  - type: synology
    host: a
    username: u
    password: p
`)

	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadUnknownType(t *testing.T) {
	cfg := writeTemp(t, `
backends:
  - name: x
    type: docker
    host: a
    username: u
    password: p
`)

	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestLoadNoBackends(t *testing.T) {
	cfg := writeTemp(t, `backends: []`)

	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error for empty backends")
	}
}

func TestByType(t *testing.T) {
	c := &Config{
		Backends: []Backend{
			{Name: "a", Type: BackendTypeSynology},
			{Name: "b", Type: BackendTypeUniFi},
			{Name: "c", Type: BackendTypeSynology},
		},
	}

	syn := c.ByType(BackendTypeSynology)
	if len(syn) != 2 {
		t.Fatalf("expected 2 synology backends, got %d", len(syn))
	}

	uni := c.ByType(BackendTypeUniFi)
	if len(uni) != 1 {
		t.Fatalf("expected 1 unifi backend, got %d", len(uni))
	}
}

func TestLoadAuthScopesEnabled(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  scopes_enabled: true
  issuer: https://test-issuer
dex:
  url: http://dex:5556
backends:
  - name: nas
    type: synology
    host: a
    username: u
    password: p
`)
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Auth.ScopesEnabled {
		t.Error("expected ScopesEnabled to be true")
	}
}

func TestLoadDexURL(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  issuer: http://localhost:8080/dex
dex:
  url: http://dex:5556
backends:
  - name: nas
    type: synology
    host: a
    username: u
    password: p
`)
	c, err := Load(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Dex.URL != "http://dex:5556" {
		t.Errorf("expected dex.url http://dex:5556, got %s", c.Dex.URL)
	}
}

func TestLoadAuthEnabled_RequiresDexURL(t *testing.T) {
	cfg := writeTemp(t, `
auth:
  enabled: true
  issuer: http://localhost:8080/dex
backends:
  - name: nas
    type: synology
    host: a
    username: u
    password: p
`)
	_, err := Load(cfg)
	if err == nil {
		t.Fatal("expected error when auth enabled but dex.url missing")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
