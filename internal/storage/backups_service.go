package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// BackupBackend defines the adapter interface for backup operations.
type BackupBackend interface {
	SupportsBackups() bool
	Location() *time.Location
	ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error)
	GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error)
	GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error)
	GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error)
}

type backupDeviceBackend struct {
	device  string
	backend BackupBackend
}

func newBackupDeviceBackends(backends map[string]BackupBackend) []backupDeviceBackend {
	dbs := make([]backupDeviceBackend, 0, len(backends))
	for device, backend := range backends {
		dbs = append(dbs, backupDeviceBackend{device: device, backend: backend})
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].device < dbs[j].device })
	return dbs
}

func (s *Service) findBackupBackend(device string) (BackupBackend, error) {
	for _, db := range s.backupBackends {
		if db.device == device {
			if !db.backend.SupportsBackups() {
				return nil, fmt.Errorf("device %q does not support backups: %w", device, apierrors.ErrNotFound)
			}
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
}

// ListBackupTasks returns backup tasks from all (or a filtered) backends.
func (s *Service) ListBackupTasks(ctx context.Context, device *string) (BackupTaskList, error) {
	var items []BackupTask
	for _, db := range s.backupBackends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsBackups() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}

		tasks, err := db.backend.ListBackupTasks()
		if err != nil {
			return BackupTaskList{}, fmt.Errorf("list backup tasks from %s: %w", db.device, err)
		}
		for _, t := range tasks.TaskList {
			status, _ := db.backend.GetBackupTaskStatus(t.TaskID)
			items = append(items, BackupTask{
				Device:     db.device,
				Id:         fmt.Sprintf("%s.%d", db.device, t.TaskID),
				Name:       t.Name,
				Status:     mapBackupStatus(t.State),
				LastResult: mapBackupResult(status),
				Type:       mapBackupType(t.Type),
			})
		}
	}
	if items == nil {
		items = []BackupTask{}
	}
	return BackupTaskList{Items: items}, nil
}

// GetBackupTask returns a single backup task by composite ID (device.taskId).
func (s *Service) GetBackupTask(ctx context.Context, taskID string) (*BackupTaskDetail, error) {
	device, _, err := parseTaskID(taskID)
	if err != nil {
		return nil, err
	}

	backend, err := s.findBackupBackend(device)
	if err != nil {
		return nil, err
	}

	tasks, err := backend.ListBackupTasks()
	if err != nil {
		return nil, fmt.Errorf("get backup task from %s: %w", device, err)
	}

	for _, t := range tasks.TaskList {
		compositeID := fmt.Sprintf("%s.%d", device, t.TaskID)
		if compositeID != taskID {
			continue
		}

		loc := backend.Location()
		status, _ := backend.GetBackupTaskStatus(t.TaskID)
		detail, _ := backend.GetBackupTaskDetail(t.TaskID)
		target, _ := backend.GetBackupTarget(t.TaskID)

		var lastBkpSuccessTime, nextBkpTime string
		if status != nil {
			lastBkpSuccessTime = status.LastBkpSuccessTime
			nextBkpTime = status.NextBkpTime
		}
		lastRunAt := parseBackupTime(lastBkpSuccessTime, loc)
		nextRunAt := parseBackupTime(nextBkpTime, loc)

		var size *int64
		if target != nil {
			v := target.UsedSize
			size = &v
		}

		var folders *[]string
		if detail != nil {
			var fl []string
			for _, f := range detail.Source.FolderList {
				if f.FullPath != "" {
					fl = append(fl, f.FullPath)
				}
			}
			if len(fl) > 0 {
				folders = &fl
			}
		}

		return &BackupTaskDetail{
			Device:     device,
			Id:         compositeID,
			Name:       t.Name,
			Status:     mapBackupStatus(t.State),
			LastResult: mapBackupResult(status),
			Type:       mapBackupType(t.Type),
			LastRunAt:  lastRunAt,
			NextRunAt:  nextRunAt,
			Size:       size,
			Folders:    folders,
		}, nil
	}
	return nil, nil
}

// parseBackupTime parses a DSM backup timestamp in the format "2006/01/02 15:04"
// using the given location. Returns nil if s is empty or unparseable.
// The returned time is in UTC.
func parseBackupTime(s string, loc *time.Location) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006/01/02 15:04", s, loc)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// mapBackupResult converts a DSMBackupTaskStatusResponse to a BackupTaskResult.
// "done" with a non-zero error code indicates a backup that completed with warnings.
func mapBackupResult(status *adapters.DSMBackupTaskStatusResponse) BackupTaskResult {
	if status == nil {
		return BackupTaskResultUnknown
	}
	switch status.LastBkpResult {
	case "done":
		if status.LastBkpErrorCode != 0 {
			return BackupTaskResultWarning
		}
		return BackupTaskResultSuccess
	case "error":
		return BackupTaskResultFailed
	case "skip":
		return BackupTaskResultSkipped
	default:
		return BackupTaskResultUnknown
	}
}

// parseTaskID splits a composite ID "device.taskId" into its parts.
func parseTaskID(id string) (device, taskID string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid task ID %q: expected format device.taskId: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}

// mapBackupStatus converts a DSM backup task state string to BackupTaskStatus.
func mapBackupStatus(state string) BackupTaskStatus {
	switch state {
	case "backupable":
		return Idle
	case "running":
		return Running
	case "suspend":
		return Suspended
	case "error":
		return Error
	default:
		return Idle
	}
}

// mapBackupType converts a DSM backup type string to a human-readable type.
func mapBackupType(t string) string {
	switch {
	case strings.HasPrefix(t, "image:"):
		return "hyperBackup"
	case strings.Contains(t, "glacier"):
		return "glacierBackup"
	default:
		return t
	}
}
