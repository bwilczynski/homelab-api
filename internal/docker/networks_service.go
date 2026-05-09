package docker

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// NetworksBackend is the narrow interface for Docker network operations.
type NetworksBackend interface {
	ListDockerNetworks() (*adapters.DSMDockerNetworkListResponse, error)
}

// ListNetworks returns all Docker networks from all backends.
func (s *Service) ListNetworks(ctx context.Context, device *string) (DockerNetworkList, error) {
	var items []DockerNetwork
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsContainers() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}
		raw, err := db.backend.ListDockerNetworks()
		if err != nil {
			return DockerNetworkList{}, fmt.Errorf("list docker networks from %s: %w", db.device, err)
		}
		for _, n := range raw.Networks {
			items = append(items, mapDockerNetwork(db.device, n))
		}
	}
	if items == nil {
		items = []DockerNetwork{}
	}
	return DockerNetworkList{Items: items}, nil
}

// GetNetwork returns a single Docker network by composite ID "{device}.{name}".
func (s *Service) GetNetwork(ctx context.Context, networkID string) (*DockerNetworkDetail, error) {
	device, name, err := parseDockerID(networkID)
	if err != nil {
		return nil, err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}
	raw, err := backend.ListDockerNetworks()
	if err != nil {
		return nil, fmt.Errorf("list docker networks: %w", err)
	}
	for _, n := range raw.Networks {
		if n.Name == name {
			detail := mapDockerNetworkDetail(device, n)
			return &detail, nil
		}
	}
	return nil, fmt.Errorf("network %q not found: %w", networkID, apierrors.ErrNotFound)
}

func mapDockerNetwork(device string, n adapters.DSMDockerNetworkItem) DockerNetwork {
	containers := n.Containers
	if containers == nil {
		containers = []string{}
	}
	return DockerNetwork{
		Id:                  fmt.Sprintf("%s.%s", device, n.Name),
		Name:                n.Name,
		Device:              device,
		ConnectedContainers: len(containers),
	}
}

func mapDockerNetworkDetail(device string, n adapters.DSMDockerNetworkItem) DockerNetworkDetail {
	containers := n.Containers
	if containers == nil {
		containers = []string{}
	}
	detail := DockerNetworkDetail{
		Id:                  fmt.Sprintf("%s.%s", device, n.Name),
		Name:                n.Name,
		Device:              device,
		ConnectedContainers: len(containers),
		Driver:              n.Driver,
		Containers:          containers,
	}
	if n.Subnet != "" {
		s := n.Subnet
		detail.Subnet = &s
	}
	if n.Gateway != "" {
		g := n.Gateway
		detail.Gateway = &g
	}
	if n.IPRange != "" {
		r := n.IPRange
		detail.IpRange = &r
	}
	return detail
}
