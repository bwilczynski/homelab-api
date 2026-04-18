package system

import (
	"context"
	"fmt"
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

// Service implements system domain business logic.
type Service struct {
	device string
	dsm    DSMBackend
	unifi  UniFiBackend
}

// NewService creates a new system service.
func NewService(device string, dsm DSMBackend, unifi UniFiBackend) *Service {
	return &Service{device: device, dsm: dsm, unifi: unifi}
}

// GetSystemHealth queries all backends for health and assembles an aggregate Health model.
// The top-level status is the worst status across all components.
func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	var components []ComponentHealth
	overall := Healthy

	// UniFi subsystems (gateway, wan, lan, wlan, www, vpn, …).
	subsystems, err := s.unifi.GetHealth()
	if err != nil {
		return Health{}, fmt.Errorf("get unifi health: %w", err)
	}
	for _, sub := range subsystems {
		status := mapUniFiStatus(sub.Status)
		components = append(components, ComponentHealth{
			Name:   sub.Subsystem,
			Status: status,
		})
		overall = worstStatus(overall, status)
	}

	// DSM storage volumes → single "storage" component.
	storageStatus, storageMsg, err := s.storageHealth()
	if err != nil {
		return Health{}, fmt.Errorf("get storage health: %w", err)
	}
	storageComponent := ComponentHealth{Name: "storage", Status: storageStatus}
	if storageMsg != "" {
		storageComponent.Message = &storageMsg
	}
	components = append(components, storageComponent)
	overall = worstStatus(overall, storageStatus)

	// DSM containers → single "containers" component.
	containersStatus, containersMsg, err := s.containersHealth()
	if err != nil {
		return Health{}, fmt.Errorf("get containers health: %w", err)
	}
	containersComponent := ComponentHealth{Name: "containers", Status: containersStatus}
	if containersMsg != "" {
		containersComponent.Message = &containersMsg
	}
	components = append(components, containersComponent)
	overall = worstStatus(overall, containersStatus)

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
func (s *Service) storageHealth() (HealthStatus, string, error) {
	resp, err := s.dsm.GetStorageVolumes()
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
func (s *Service) containersHealth() (HealthStatus, string, error) {
	resp, err := s.dsm.ListContainers()
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

// ListSystemInfo queries DSM for static system information.
func (s *Service) ListSystemInfo(ctx context.Context, device *string) (SystemInfoList, error) {
	if device != nil && *device != s.device {
		return SystemInfoList{Items: []SystemInfo{}}, nil
	}

	info, err := s.dsm.GetSystemInfo()
	if err != nil {
		return SystemInfoList{}, fmt.Errorf("get system info: %w", err)
	}

	uptimeSecs, err := parseUptime(info.UpTime)
	if err != nil {
		uptimeSecs = 0
	}

	return SystemInfoList{
		Items: []SystemInfo{
			{
				Device:        s.device,
				Model:         info.Model,
				Firmware:      info.FirmwareVer,
				RamMb:         info.RamSize,
				UptimeSeconds: uptimeSecs,
			},
		},
	}, nil
}

// ListSystemUtilization queries DSM for live utilization data.
func (s *Service) ListSystemUtilization(ctx context.Context, device *string) (SystemUtilizationList, error) {
	if device != nil && *device != s.device {
		return SystemUtilizationList{Items: []SystemUtilization{}}, nil
	}

	util, err := s.dsm.GetSystemUtilization()
	if err != nil {
		return SystemUtilizationList{}, fmt.Errorf("get system utilization: %w", err)
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

	return SystemUtilizationList{
		Items: []SystemUtilization{
			{
				Device:    s.device,
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
			},
		},
	}, nil
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
