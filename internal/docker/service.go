package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ContainerBackend defines the adapter interface for container operations.
type ContainerBackend interface {
	SupportsContainers() bool
	ListContainers() (*adapters.DSMContainerListResponse, error)
	GetContainer(name string) (*adapters.DSMContainerDetailResponse, error)
	GetContainerResources() (*adapters.DSMContainerResourceResponse, error)
	StartContainer(name string) error
	StopContainer(name string) error
	RestartContainer(name string) error
}

type deviceBackend struct {
	device  string
	backend ContainerBackend
}

// Service implements container business logic.
type Service struct {
	backends []deviceBackend
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new container service with one or more backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(backends map[string]ContainerBackend, monitor ...adapters.AvailabilityChecker) *Service {
	dbs := make([]deviceBackend, 0, len(backends))
	for device, backend := range backends {
		dbs = append(dbs, deviceBackend{device: device, backend: backend})
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].device < dbs[j].device })
	svc := &Service{backends: dbs}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}

func (s *Service) findBackend(device string) (ContainerBackend, error) {
	for _, db := range s.backends {
		if db.device == device {
			if !db.backend.SupportsContainers() {
				return nil, fmt.Errorf("device %q does not support containers: %w", device, apierrors.ErrNotFound)
			}
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
}

// ListContainers returns all containers with their resource usage from all backends.
func (s *Service) ListContainers(ctx context.Context, device *string) (ContainerList, error) {
	var items []Container
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

		containers, err := db.backend.ListContainers()
		if err != nil {
			return ContainerList{}, fmt.Errorf("list containers from %s: %w", db.device, err)
		}

		resources, err := db.backend.GetContainerResources()
		if err != nil {
			return ContainerList{}, fmt.Errorf("get container resources from %s: %w", db.device, err)
		}

		resourceMap := make(map[string]adapters.DSMContainerResource, len(resources.Resources))
		for _, r := range resources.Resources {
			resourceMap[r.Name] = r
		}

		for _, c := range containers.Containers {
			items = append(items, mapContainer(db.device, c, resourceMap[c.Name], 0))
		}
	}
	if items == nil {
		items = []Container{}
	}
	return ContainerList{Items: items}, nil
}

// GetContainer returns a single container by its composite ID (device.name).
func (s *Service) GetContainer(ctx context.Context, containerID string) (*ContainerDetail, error) {
	device, name, err := parseContainerID(containerID)
	if err != nil {
		return nil, err
	}

	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}

	detail, err := backend.GetContainer(name)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}

	resources, err := backend.GetContainerResources()
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

	c := mapContainerDetail(device, *detail, res)
	return &c, nil
}

// StartContainer starts a container by its composite ID.
func (s *Service) StartContainer(ctx context.Context, containerID string) error {
	device, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.StartContainer(name)
}

// StopContainer stops a container by its composite ID.
func (s *Service) StopContainer(ctx context.Context, containerID string) error {
	device, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.StopContainer(name)
}

// RestartContainer restarts a container by its composite ID.
func (s *Service) RestartContainer(ctx context.Context, containerID string) error {
	device, name, err := parseContainerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.RestartContainer(name)
}

// parseContainerID splits a composite ID "device.name" into its parts.
func parseContainerID(id string) (device, name string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid container ID %q: expected format device.name: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}

// mapRestartPolicy converts a Docker restart policy name to the API enum.
func mapRestartPolicy(name string) ContainerDetailRestartPolicy {
	switch ContainerDetailRestartPolicy(name) {
	case Always, No, OnFailure, UnlessStopped:
		return ContainerDetailRestartPolicy(name)
	default:
		return No
	}
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

// mapContainerDetail converts a DSM container detail to the API ContainerDetail model.
func mapContainerDetail(device string, d adapters.DSMContainerDetailResponse, res adapters.DSMContainerResource) ContainerDetail {
	startedAt, _ := time.Parse(time.RFC3339Nano, d.Details.State.StartedAt)

	var finishedAt *time.Time
	if !d.Details.State.Running {
		if t, err := time.Parse(time.RFC3339Nano, d.Details.State.FinishedAt); err == nil && !t.IsZero() {
			finishedAt = &t
		}
	}

	envVars := make([]EnvVariable, len(d.Profile.EnvVariables))
	for i, e := range d.Profile.EnvVariables {
		envVars[i] = EnvVariable{Key: e.Key, Value: e.Value}
	}

	networks := make([]ContainerNetwork, len(d.Profile.Networks))
	for i, n := range d.Profile.Networks {
		networks[i] = ContainerNetwork{Name: n.Name, Driver: n.Driver}
	}

	portBindings := make([]PortBinding, len(d.Profile.PortBindings))
	for i, p := range d.Profile.PortBindings {
		portBindings[i] = PortBinding{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			Protocol:      PortBindingProtocol(p.Type),
		}
	}

	volumeBindings := make([]VolumeMount, len(d.Profile.VolumeBindings))
	for i, v := range d.Profile.VolumeBindings {
		mode := Rw
		if v.Type == "ro" {
			mode = Ro
		}
		volumeBindings[i] = VolumeMount{
			Source:      v.HostPath(),
			Destination: v.MountPath,
			Mode:        mode,
		}
	}

	restartPolicy := mapRestartPolicy(d.Details.HostConfig.RestartPolicy.Name)

	labels := d.Details.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	return ContainerDetail{
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
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		ExitCode:       d.Details.State.ExitCode,
		OomKilled:      d.Details.State.OOMKilled,
		RestartPolicy:  restartPolicy,
		Privileged:     d.Profile.Privileged,
		MemoryLimit:    Bytes(d.Profile.MemoryLimit),
		PortBindings:   portBindings,
		Networks:       networks,
		VolumeBindings: volumeBindings,
		EnvVariables:   envVars,
		Entrypoint:     d.Details.Config.Entrypoint,
		Cmd:            d.Details.Config.Cmd,
		Labels:         &labels,
	}
}
