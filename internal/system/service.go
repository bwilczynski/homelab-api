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

// GetSystemHealth queries UniFi subsystem health and maps it to the Health model.
func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	subsystems, err := s.unifi.GetHealth()
	if err != nil {
		return Health{}, fmt.Errorf("get health: %w", err)
	}

	components := make([]ComponentHealth, 0, len(subsystems))
	overall := Healthy

	for _, sub := range subsystems {
		status := mapUniFiStatus(sub.Status)
		if status == Unhealthy && overall != Unhealthy {
			overall = Unhealthy
		} else if status == Degraded && overall == Healthy {
			overall = Degraded
		}
		components = append(components, ComponentHealth{
			Name:   sub.Subsystem,
			Status: status,
		})
	}

	return Health{
		Status:     overall,
		CheckedAt:  time.Now().UTC(),
		Components: components,
	}, nil
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
