package system

import (
	"context"
	"fmt"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// UtilizationDSMBackend is the narrow interface for utilization operations.
type UtilizationDSMBackend interface {
	GetSystemUtilization() (*adapters.DSMSystemUtilizationResponse, error)
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
