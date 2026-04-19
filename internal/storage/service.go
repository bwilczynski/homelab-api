package storage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// StorageBackend defines the adapter interface for storage operations.
type StorageBackend interface {
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
}

// Service implements storage business logic.
type Service struct {
	device  string
	backend StorageBackend
}

// NewService creates a new storage service.
func NewService(device string, backend StorageBackend) *Service {
	return &Service{device: device, backend: backend}
}

// ListStorageVolumes returns all volumes with their associated disks.
func (s *Service) ListStorageVolumes(ctx context.Context, device *string) (VolumeList, error) {
	if device != nil && *device != s.device {
		return VolumeList{Items: []Volume{}}, nil
	}

	resp, err := s.backend.GetStorageVolumes()
	if err != nil {
		return VolumeList{}, fmt.Errorf("list storage volumes: %w", err)
	}

	volumes := mapVolumes(s.device, resp)
	return VolumeList{Items: volumes}, nil
}

// GetStorageVolume returns a single volume with extended detail by its composite ID (device.name).
func (s *Service) GetStorageVolume(ctx context.Context, volumeID string) (*VolumeDetail, error) {
	_, name, err := parseVolumeID(volumeID)
	if err != nil {
		return nil, err
	}

	resp, err := s.backend.GetStorageVolumes()
	if err != nil {
		return nil, fmt.Errorf("get storage volume: %w", err)
	}

	poolByID := make(map[string]adapters.DSMStoragePool, len(resp.StoragePools))
	for _, p := range resp.StoragePools {
		poolByID[p.ID] = p
	}

	rawByName := make(map[string]adapters.DSMStorageVolume, len(resp.Volumes))
	for _, v := range resp.Volumes {
		rawByName[v.ID] = v
	}

	disksByID := make(map[string]adapters.DSMStorageDisk, len(resp.Disks))
	for _, d := range resp.Disks {
		disksByID[d.ID] = d
	}

	for _, vol := range mapVolumes(s.device, resp) {
		if vol.Name != name {
			continue
		}
		raw := rawByName[vol.Name]
		pool := poolByID[raw.PoolPath]
		return &VolumeDetail{
			Device:     vol.Device,
			Disks:      mapDisks(pool, disksByID),
			FileSystem: vol.FileSystem,
			Id:         vol.Id,
			Name:       vol.Name,
			RaidType:   vol.RaidType,
			Status:     vol.Status,
			TotalBytes: vol.TotalBytes,
			UsedBytes:  vol.UsedBytes,
			MountPath:  raw.VolPath,
			PoolStatus: mapVolumeStatus(pool.Status),
		}, nil
	}
	return nil, nil
}

// parseVolumeID splits a composite ID "device.name" into its parts.
func parseVolumeID(id string) (device, name string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid volume ID %q: expected format device.name", id)
	}
	return parts[0], parts[1], nil
}

// mapVolumes converts a DSM storage response to API Volume models.
func mapVolumes(device string, resp *adapters.DSMStorageVolumeResponse) []Volume {
	volumes := make([]Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		totalBytes, _ := strconv.ParseInt(v.Size.Total, 10, 64)
		usedBytes, _ := strconv.ParseInt(v.Size.Used, 10, 64)

		volumes = append(volumes, Volume{
			Id:         fmt.Sprintf("%s.%s", device, v.ID),
			Device:     device,
			Name:       v.ID,
			FileSystem: v.FsType,
			RaidType:   v.RaidType,
			Status:     mapVolumeStatus(v.Status),
			TotalBytes: totalBytes,
			UsedBytes:  usedBytes,
		})
	}
	return volumes
}

// mapDisks converts the DSM pool's disk list to API VolumeDisk models.
func mapDisks(pool adapters.DSMStoragePool, disksByID map[string]adapters.DSMStorageDisk) []VolumeDisk {
	disks := make([]VolumeDisk, 0, len(pool.Disks))
	for _, diskID := range pool.Disks {
		if d, ok := disksByID[diskID]; ok {
			totalBytes, _ := strconv.ParseInt(d.SizeTotal, 10, 64)
			disks = append(disks, VolumeDisk{
				Id:                 d.ID,
				Model:              d.Model,
				Status:             mapDiskStatus(d.Status),
				TemperatureCelsius: d.Temp,
				TotalBytes:         totalBytes,
			})
		}
	}
	return disks
}

// mapVolumeStatus converts a DSM volume status string to VolumeStatus.
func mapVolumeStatus(status string) VolumeStatus {
	switch status {
	case "normal":
		return VolumeStatusNormal
	case "degraded":
		return VolumeStatusDegraded
	case "repairing":
		return VolumeStatusRepairing
	default:
		return VolumeStatusCrashed
	}
}

// mapDiskStatus converts a DSM disk status string to DiskStatus.
func mapDiskStatus(status string) DiskStatus {
	switch status {
	case "normal":
		return DiskStatusNormal
	case "warning":
		return DiskStatusWarning
	case "failing":
		return DiskStatusFailing
	default:
		return DiskStatusCritical
	}
}
