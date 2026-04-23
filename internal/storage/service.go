package storage

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// StorageBackend defines the adapter interface for storage operations.
type StorageBackend interface {
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
}

type deviceBackend struct {
	device  string
	backend StorageBackend
}

// Service implements storage business logic.
type Service struct {
	backends []deviceBackend
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new storage service with one or more backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(backends map[string]StorageBackend, monitor ...adapters.AvailabilityChecker) *Service {
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

func (s *Service) findBackend(device string) (StorageBackend, error) {
	for _, db := range s.backends {
		if db.device == device {
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q", device)
}

// ListStorageVolumes returns all volumes with their associated disks from all backends.
func (s *Service) ListStorageVolumes(ctx context.Context, device *string) (VolumeList, error) {
	var volumes []Volume
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}

		resp, err := db.backend.GetStorageVolumes()
		if err != nil {
			return VolumeList{}, fmt.Errorf("list storage volumes from %s: %w", db.device, err)
		}

		volumes = append(volumes, mapVolumes(db.device, resp)...)
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

	backend, err := s.findBackend(device)
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
