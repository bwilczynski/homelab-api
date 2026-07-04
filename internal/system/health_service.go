package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// HealthDSMBackend is the narrow interface for health checks on DSM backends.
type HealthDSMBackend interface {
	GetStorageVolumes(ctx context.Context) (*adapters.DSMStorageVolumeResponse, error)
	ListContainers(ctx context.Context) (*adapters.DSMContainerListResponse, error)
}

// HealthUniFiBackend is the narrow interface for health checks on UniFi backends.
type HealthUniFiBackend interface {
	GetHealth(ctx context.Context) ([]adapters.UniFiSubsystemHealth, error)
}

// GetSystemHealth queries all backends for health and assembles an aggregate Health model.
// The top-level status is the worst status across all components.
func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	var components []ComponentHealth
	overall := Healthy

	// UniFi subsystems (gateway, wan, lan, wlan, www, vpn, …).
	for _, ue := range s.unifiBackends {
		if s.monitor != nil && !s.monitor.Available(ue.controller) {
			name := "network"
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":network"
			}
			msg := "offline"
			components = append(components, ComponentHealth{Name: name, Status: Unhealthy, Message: &msg})
			overall = Unhealthy
			continue
		}

		subsystems, err := ue.unifi.GetHealth(ctx)
		if err != nil {
			name := "network"
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":network"
			}
			msg := err.Error()
			components = append(components, ComponentHealth{Name: name, Status: Unhealthy, Message: &msg})
			overall = Unhealthy
			continue
		}
		for _, sub := range subsystems {
			if sub.Status == "unknown" {
				s.logger.Info("skipping unifi subsystem with unknown status", "subsystem", sub.Subsystem, "controller", ue.controller)
				continue
			}
			status := mapUniFiStatus(sub.Status)
			name := sub.Subsystem
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":" + name
			}
			components = append(components, ComponentHealth{
				Name:   name,
				Status: status,
			})
			overall = worstStatus(overall, status)
		}
	}

	// DSM storage volumes and containers per device.
	for _, de := range s.dsmBackends {
		prefix := ""
		if len(s.dsmBackends) > 1 {
			prefix = de.device + ":"
		}

		if s.monitor != nil && !s.monitor.Available(de.device) {
			msg := "offline"
			components = append(components, ComponentHealth{Name: prefix + "storage", Status: Unhealthy, Message: &msg})
			if de.dockerEnabled {
				components = append(components, ComponentHealth{Name: prefix + "containers", Status: Unhealthy, Message: &msg})
			}
			overall = Unhealthy
			continue
		}

		storageStatus, storageMsg, err := storageHealth(ctx, de.dsm)
		if err != nil {
			// On error, mark as unhealthy with the error message
			storageComponent := ComponentHealth{Name: prefix + "storage", Status: Unhealthy}
			storageComponent.Message = &storageMsg
			components = append(components, storageComponent)
			overall = Unhealthy
		} else {
			storageComponent := ComponentHealth{Name: prefix + "storage", Status: storageStatus}
			if storageMsg != "" {
				storageComponent.Message = &storageMsg
			}
			components = append(components, storageComponent)
			overall = worstStatus(overall, storageStatus)
		}

		if de.dockerEnabled {
			containersStatus, containersMsg, err := containersHealth(ctx, de.dsm)
			if err != nil {
				// On error, mark as unhealthy with the error message
				containersComponent := ComponentHealth{Name: prefix + "containers", Status: Unhealthy}
				containersComponent.Message = &containersMsg
				components = append(components, containersComponent)
				overall = Unhealthy
			} else {
				containersComponent := ComponentHealth{Name: prefix + "containers", Status: containersStatus}
				if containersMsg != "" {
					containersComponent.Message = &containersMsg
				}
				components = append(components, containersComponent)
				overall = worstStatus(overall, containersStatus)
			}
		}
	}

	if components == nil {
		components = []ComponentHealth{}
	}

	return Health{
		Status:     overall,
		CheckedAt:  time.Now().UTC(),
		Components: components,
	}, nil
}

// storageHealth derives a single HealthStatus from DSM volume statuses.
func storageHealth(ctx context.Context, dsm HealthDSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.GetStorageVolumes(ctx)
	if err != nil {
		return Unhealthy, err.Error(), err
	}
	worst := Healthy
	var degraded, crashed []string
	for _, v := range resp.Volumes {
		st := mapVolumeStatus(v.Status)
		worst = worstStatus(worst, st)
		switch st {
		case Unhealthy:
			crashed = append(crashed, v.ID)
		case Degraded:
			degraded = append(degraded, v.ID)
		}
	}
	var msg string
	if len(crashed) > 0 {
		msg = fmt.Sprintf("crashed: %s", strings.Join(crashed, ", "))
	} else if len(degraded) > 0 {
		msg = fmt.Sprintf("degraded: %s", strings.Join(degraded, ", "))
	}
	return worst, msg, nil
}

// containersHealth derives a single HealthStatus from DSM container states.
func containersHealth(ctx context.Context, dsm HealthDSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.ListContainers(ctx)
	if err != nil {
		return Unhealthy, err.Error(), err
	}
	notRunning := 0
	for _, c := range resp.Containers {
		if !c.State.Running {
			notRunning++
		}
	}
	if notRunning == 0 {
		return Healthy, "", nil
	}
	return Degraded, fmt.Sprintf("%d container(s) not running", notRunning), nil
}

// worstStatus returns the more severe of two HealthStatus values.
func worstStatus(a, b HealthStatus) HealthStatus {
	if a == Unhealthy || b == Unhealthy {
		return Unhealthy
	}
	if a == Degraded || b == Degraded {
		return Degraded
	}
	return Healthy
}

// mapUniFiStatus converts a UniFi subsystem status string to HealthStatus.
func mapUniFiStatus(status string) HealthStatus {
	switch status {
	case "ok":
		return Healthy
	case "unknown":
		return Degraded
	default:
		return Unhealthy
	}
}

// mapVolumeStatus converts a DSM volume status string to HealthStatus.
func mapVolumeStatus(status string) HealthStatus {
	switch status {
	case "normal":
		return Healthy
	case "degraded", "repairing":
		return Degraded
	default:
		return Unhealthy
	}
}
