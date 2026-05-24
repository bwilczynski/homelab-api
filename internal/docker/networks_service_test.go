package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
	"github.com/bwilczynski/homelab-api/internal/testhelpers"
)

func TestListNetworks(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")

	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})
	result, err := svc.ListNetworks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != len(resp.Networks) {
		t.Fatalf("expected %d networks, got %d", len(resp.Networks), len(result.Items))
	}

	first := result.Items[0]
	if first.Id != "nas-01."+resp.Networks[0].Name {
		t.Errorf("expected id nas-01.%s, got %s", resp.Networks[0].Name, first.Id)
	}
	if first.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", first.Device)
	}
	if first.Name != resp.Networks[0].Name {
		t.Errorf("expected name %s, got %s", resp.Networks[0].Name, first.Name)
	}
	if first.ConnectedContainers != len(resp.Networks[0].Containers) {
		t.Errorf("expected connectedContainers %d, got %d", len(resp.Networks[0].Containers), first.ConnectedContainers)
	}
}

func TestListNetworksDeviceFilter(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	device := "nas-01"
	result, err := svc.ListNetworks(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != len(resp.Networks) {
		t.Fatalf("expected %d networks for matching device, got %d", len(resp.Networks), len(result.Items))
	}

	other := "nas-02"
	result, err = svc.ListNetworks(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 networks for non-matching device, got %d", len(result.Items))
	}
}

func TestListNetworksEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		networksResp: &adapters.DSMDockerNetworkListResponse{Networks: []adapters.DSMDockerNetworkItem{}},
	}})
	result, err := svc.ListNetworks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestGetNetwork(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	id := "nas-01." + resp.Networks[0].Name
	detail, err := svc.GetNetwork(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Id != id {
		t.Errorf("expected id %s, got %s", id, detail.Id)
	}
	if detail.Driver != resp.Networks[0].Driver {
		t.Errorf("expected driver %s, got %s", resp.Networks[0].Driver, detail.Driver)
	}
	if len(detail.Containers) != len(resp.Networks[0].Containers) {
		t.Errorf("expected %d containers, got %d", len(resp.Networks[0].Containers), len(detail.Containers))
	}
}

func TestGetNetworkHostNetworkOptionalFields(t *testing.T) {
	// Host network has empty subnet and gateway — optional fields must be nil
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		networksResp: &adapters.DSMDockerNetworkListResponse{
			Networks: []adapters.DSMDockerNetworkItem{
				{ID: "abc123", Name: "host", Driver: "host", Gateway: "", Subnet: "", IPRange: "", Containers: []string{}},
			},
		},
	}})

	detail, err := svc.GetNetwork(context.Background(), "nas-01.host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Subnet != nil {
		t.Errorf("expected nil Subnet for host network, got %v", detail.Subnet)
	}
	if detail.Gateway != nil {
		t.Errorf("expected nil Gateway for host network, got %v", detail.Gateway)
	}
	if detail.IpRange != nil {
		t.Errorf("expected nil IpRange for host network, got %v", detail.IpRange)
	}
}

func TestGetNetworkNotFound(t *testing.T) {
	resp := testhelpers.LoadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	_, err := svc.GetNetwork(context.Background(), "nas-01.does_not_exist")
	if err == nil {
		t.Fatal("expected error for unknown network")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetNetworkInvalidID(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{}})

	_, err := svc.GetNetwork(context.Background(), "invalid-no-dot")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
