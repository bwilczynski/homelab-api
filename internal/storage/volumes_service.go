package storage

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// StorageBackend defines the adapter interface for storage operations.
type StorageBackend interface {
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
}

func (s *Service) findStorageBackend(device string) (StorageBackend, error) {
	backend, ok := s.storageBackends.Find(device)
	if !ok {
		return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
	}
	return backend, nil
}

// ListStorageVolumes returns all volumes with their associated disks from all backends.
func (s *Service) ListStorageVolumes(ctx context.Context, device *string) (VolumeList, error) {
	var volumes []Volume
	for _, entry := range s.storageBackends {
		if device != nil && *device != entry.Name {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(entry.Name) {
			continue
		}

		resp, err := entry.Backend.GetStorageVolumes()
		if err != nil {
			return VolumeList{}, fmt.Errorf("list storage volumes from %s: %w", entry.Name, err)
		}
		volumes = append(volumes, mapVolumes(entry.Name, resp)...)
	}
	if volumes == nil {
		volumes = []Volume{}
	}
	return VolumeList{Items: volumes}, nil
}

// GetStorageVolume returns a single volume with extended detail by its composite ID (device.name).
func (s *Service) GetStorageVolume(ctx context.Context, volumeID string) (*VolumeDetail, error) {
	device, name, err := parseVolumeID(volumeID)
	if err != nil {
		return nil, err
	}

	backend, err := s.findStorageBackend(device)
	if err != nil {
		return nil, err
	}

	resp, err := backend.GetStorageVolumes()
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

	for _, vol := range mapVolumes(device, resp) {
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
	return apierrors.ParseCompositeID(id, "volume ID", "device.name")
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
		return Normal
	case "degraded":
		return Degraded
	case "repairing":
		return Repairing
	default:
		return Crashed
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
