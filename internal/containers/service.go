package containers

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// ContainerBackend defines the adapter interface for container operations.
type ContainerBackend interface {
	ListContainers() (*adapters.DSMContainerListResponse, error)
	GetContainer(name string) (*adapters.DSMContainerDetailResponse, error)
	GetContainerResources() (*adapters.DSMContainerResourceResponse, error)
	StartContainer(name string) error
	StopContainer(name string) error
	RestartContainer(name string) error
}

// Service implements container business logic.
type Service struct {
	device  string
	backend ContainerBackend
}

// NewService creates a new container service.
func NewService(device string, backend ContainerBackend) *Service {
	return &Service{device: device, backend: backend}
}

// ListContainers returns all containers with their resource usage.
func (s *Service) ListContainers(ctx context.Context, device *string) (ContainerList, error) {
	if device != nil && *device != s.device {
		return ContainerList{Items: []Container{}}, nil
	}

	containers, err := s.backend.ListContainers()
	if err != nil {
		return ContainerList{}, fmt.Errorf("list containers: %w", err)
	}

	resources, err := s.backend.GetContainerResources()
	if err != nil {
		return ContainerList{}, fmt.Errorf("get container resources: %w", err)
	}

	resourceMap := make(map[string]adapters.DSMContainerResource, len(resources.Resources))
	for _, r := range resources.Resources {
		resourceMap[r.Name] = r
	}

	items := make([]Container, 0, len(containers.Containers))
	for _, c := range containers.Containers {
		res := resourceMap[c.Name]
		items = append(items, mapContainer(s.device, c, res, 0))
	}

	return ContainerList{Items: items}, nil
}

// GetContainer returns a single container by its composite ID (device:name).
func (s *Service) GetContainer(ctx context.Context, containerID string) (*Container, error) {
	_, name, err := parseContainerID(containerID)
	if err != nil {
		return nil, err
	}

	detail, err := s.backend.GetContainer(name)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}

	resources, err := s.backend.GetContainerResources()
	if err != nil {
		return nil, fmt.Errorf("get container resources: %w", err)
	}

	var res adapters.DSMContainerResource
	for _, r := range resources.Resources {
		if r.Name == name {
			res = r
			break
		}
	}

	c := mapContainerFromDetail(s.device, *detail, res)
	return &c, nil
}

// StartContainer starts a container by its composite ID.
func (s *Service) StartContainer(ctx context.Context, containerID string) error {
	_, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	return s.backend.StartContainer(name)
}

// StopContainer stops a container by its composite ID.
func (s *Service) StopContainer(ctx context.Context, containerID string) error {
	_, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	return s.backend.StopContainer(name)
}

// RestartContainer restarts a container by its composite ID.
func (s *Service) RestartContainer(ctx context.Context, containerID string) error {
	_, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	return s.backend.RestartContainer(name)
}

// parseContainerID splits a composite ID "device.name" into its parts.
func parseContainerID(id string) (device, name string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid container ID %q: expected format device.name", id)
	}
	return parts[0], parts[1], nil
}

// mapStatus converts a DSM container state to the API ContainerStatus enum.
func mapStatus(state adapters.DSMContainerState) ContainerStatus {
	if state.Dead {
		return Dead
	}
	if state.Restarting {
		return Restarting
	}
	if state.Paused {
		return Paused
	}
	if state.Running {
		return Running
	}
	return Stopped
}

// mapContainer converts a DSM container list entry to the API Container model.
func mapContainer(device string, c adapters.DSMContainer, res adapters.DSMContainerResource, restartCount int) Container {
	return Container{
		Id:           fmt.Sprintf("%s.%s", device, c.Name),
		Device:       device,
		Name:         c.Name,
		Image:        c.Image,
		Status:       mapStatus(c.State),
		RestartCount: restartCount,
		Resources: ContainerResources{
			CpuPercent:    res.CPU,
			MemoryBytes:   res.Memory,
			MemoryPercent: res.MemoryPercent,
		},
	}
}

// mapContainerFromDetail converts a DSM container detail to the API Container model.
func mapContainerFromDetail(device string, d adapters.DSMContainerDetailResponse, res adapters.DSMContainerResource) Container {
	return Container{
		Id:           fmt.Sprintf("%s.%s", device, d.Profile.Name),
		Device:       device,
		Name:         d.Profile.Name,
		Image:        d.Profile.Image,
		Status:       mapStatus(d.Details.State),
		RestartCount: d.Details.RestartCount,
		Resources: ContainerResources{
			CpuPercent:    res.CPU,
			MemoryBytes:   res.Memory,
			MemoryPercent: res.MemoryPercent,
		},
	}
}
