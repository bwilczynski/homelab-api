package meta_test

import (
	"testing"

	"github.com/bwilczynski/homelab-api/internal/meta"
)

func TestGetVersion(t *testing.T) {
	svc := meta.NewService("0.1.0", "v1.2.3", false, "")

	apiVersion, serverVersion := svc.GetVersion()

	if apiVersion != "0.1.0" {
		t.Errorf("apiVersion: got %q, want %q", apiVersion, "0.1.0")
	}
	if serverVersion != "v1.2.3" {
		t.Errorf("serverVersion: got %q, want %q", serverVersion, "v1.2.3")
	}
}

func TestGetAuth_Enabled(t *testing.T) {
	svc := meta.NewService("0.1.0", "dev", true, "https://dex.example.com")

	enabled, issuer := svc.GetAuth()

	if !enabled {
		t.Error("expected enabled=true")
	}
	if issuer != "https://dex.example.com" {
		t.Errorf("issuer: got %q, want %q", issuer, "https://dex.example.com")
	}
}

func TestGetAuth_Disabled(t *testing.T) {
	svc := meta.NewService("0.1.0", "dev", false, "")

	enabled, issuer := svc.GetAuth()

	if enabled {
		t.Error("expected enabled=false")
	}
	if issuer != "" {
		t.Errorf("issuer: got %q, want empty", issuer)
	}
}
