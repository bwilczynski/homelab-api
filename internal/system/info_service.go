package system

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// InfoDSMBackend is the narrow interface for system info operations.
type InfoDSMBackend interface {
	GetSystemInfo() (*adapters.DSMSystemInfoResponse, error)
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
