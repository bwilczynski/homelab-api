package system

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// DSMBackend defines the adapter interface for DSM system operations.
type DSMBackend interface {
	GetSystemInfo() (*adapters.DSMSystemInfoResponse, error)
	GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error)
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
	ListContainers() (*adapters.DSMContainerListResponse, error)
}

// UniFiBackend defines the adapter interface for UniFi system operations.
type UniFiBackend interface {
	GetHealth() ([]adapters.UniFiSubsystemHealth, error)
}

// DSMBackendConfig wraps a DSMBackend with feature flags.
type DSMBackendConfig struct {
	Backend       DSMBackend
	DockerEnabled bool
}

type dsmEntry struct {
	device        string
	dsm           DSMBackend
	dockerEnabled bool
}

type unifiEntry struct {
	controller string
	unifi      UniFiBackend
}

// Service implements system domain business logic.
type Service struct {
	dsmBackends   []dsmEntry
	unifiBackends []unifiEntry
	monitor       adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new system service with one or more DSM and UniFi backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(dsmBackends map[string]DSMBackendConfig, unifiBackends map[string]UniFiBackend, monitor ...adapters.AvailabilityChecker) *Service {
	dsms := make([]dsmEntry, 0, len(dsmBackends))
	for device, cfg := range dsmBackends {
		dsms = append(dsms, dsmEntry{device: device, dsm: cfg.Backend, dockerEnabled: cfg.DockerEnabled})
	}
	sort.Slice(dsms, func(i, j int) bool { return dsms[i].device < dsms[j].device })

	unifis := make([]unifiEntry, 0, len(unifiBackends))
	for controller, unifi := range unifiBackends {
		unifis = append(unifis, unifiEntry{controller: controller, unifi: unifi})
	}
	sort.Slice(unifis, func(i, j int) bool { return unifis[i].controller < unifis[j].controller })

	svc := &Service{dsmBackends: dsms, unifiBackends: unifis}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
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

		subsystems, err := ue.unifi.GetHealth()
		if err != nil {
			return Health{}, fmt.Errorf("get unifi health from %s: %w", ue.controller, err)
		}
		for _, sub := range subsystems {
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

		storageStatus, storageMsg, err := storageHealth(de.dsm)
		if err != nil {
			return Health{}, fmt.Errorf("get storage health from %s: %w", de.device, err)
		}
		storageComponent := ComponentHealth{Name: prefix + "storage", Status: storageStatus}
		if storageMsg != "" {
			storageComponent.Message = &storageMsg
		}
		components = append(components, storageComponent)
		overall = worstStatus(overall, storageStatus)

		if de.dockerEnabled {
			containersStatus, containersMsg, err := containersHealth(de.dsm)
			if err != nil {
				return Health{}, fmt.Errorf("get containers health from %s: %w", de.device, err)
			}
			containersComponent := ComponentHealth{Name: prefix + "containers", Status: containersStatus}
			if containersMsg != "" {
				containersComponent.Message = &containersMsg
			}
			components = append(components, containersComponent)
			overall = worstStatus(overall, containersStatus)
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
func storageHealth(dsm DSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.GetStorageVolumes()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
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
func containersHealth(dsm DSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.ListContainers()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
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

// ListSystemInfo queries all DSM backends for static system information.
func (s *Service) ListSystemInfo(ctx context.Context, device *string) (SystemInfoList, error) {
	var items []SystemInfo
	for _, de := range s.dsmBackends {
		if device != nil && *device != de.device {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}

		info, err := de.dsm.GetSystemInfo()
		if err != nil {
			return SystemInfoList{}, fmt.Errorf("get system info from %s: %w", de.device, err)
		}

		uptimeSecs, err := parseUptime(info.UpTime)
		if err != nil {
			uptimeSecs = 0
		}

		items = append(items, SystemInfo{
			Device:        de.device,
			Model:         info.Model,
			Firmware:      info.FirmwareVer,
			RamMb:         info.RamSize,
			UptimeSeconds: uptimeSecs,
		})
	}
	if items == nil {
		items = []SystemInfo{}
	}
	return SystemInfoList{Items: items}, nil
}

// ListSystemUtilization queries all DSM backends for live utilization data.
func (s *Service) ListSystemUtilization(ctx context.Context, device *string) (SystemUtilizationList, error) {
	var items []SystemUtilization
	for _, de := range s.dsmBackends {
		if device != nil && *device != de.device {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}

		util, err := de.dsm.GetSystemUtilization()
		if err != nil {
			return SystemUtilizationList{}, fmt.Errorf("get system utilization from %s: %w", de.device, err)
		}

		// Memory: DSM reports in KB, API uses bytes.
		const kbToBytes = 1024
		totalBytes := int64(util.Memory.TotalReal) * kbToBytes
		availBytes := int64(util.Memory.AvailReal) * kbToBytes
		totalSwap := int64(util.Memory.TotalSwap) * kbToBytes
		usedSwap := (int64(util.Memory.TotalSwap) - int64(util.Memory.AvailSwap)) * kbToBytes

		// CPU: sum user + system for total.
		cpuTotal := util.CPU.UserLoad + util.CPU.SystemLoad + util.CPU.OtherLoad

		// Network: skip "total" aggregate device.
		network := make([]NetworkInterfaceUsage, 0, len(util.Network))
		for _, n := range util.Network {
			if n.Device == "total" {
				continue
			}
			network = append(network, NetworkInterfaceUsage{
				Name:          n.Device,
				RxBytesPerSec: n.Rx,
				TxBytesPerSec: n.Tx,
			})
		}

		// Disks: map each individual disk.
		disks := make([]DiskIo, 0, len(util.Disk.Disk))
		for _, d := range util.Disk.Disk {
			disks = append(disks, DiskIo{
				Name:           d.Device,
				ReadOpsPerSec:  d.ReadAccess,
				WriteOpsPerSec: d.WriteAccess,
			})
		}

		items = append(items, SystemUtilization{
			Device:    de.device,
			SampledAt: time.Now().UTC(),
			Cpu: CpuUsage{
				UserPercent:   util.CPU.UserLoad,
				SystemPercent: util.CPU.SystemLoad,
				TotalPercent:  cpuTotal,
			},
			Memory: MemoryUsage{
				TotalBytes:     totalBytes,
				AvailableBytes: availBytes,
				UsedPercent:    util.Memory.RealUsage,
				SwapTotalBytes: totalSwap,
				SwapUsedBytes:  usedSwap,
			},
			Network: network,
			Disks:   disks,
		})
	}
	if items == nil {
		items = []SystemUtilization{}
	}
	return SystemUtilizationList{Items: items}, nil
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

// parseUptime converts a DSM uptime string "H:M:S" to total seconds.
func parseUptime(s string) (int64, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("unexpected uptime format %q", s)
	}
	h, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	m, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, err
	}
	sec, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, err
	}
	return h*3600 + m*60 + sec, nil
}
