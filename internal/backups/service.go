package backups

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
	ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error)
	ListScheduledTasks() (*adapters.DSMTaskSchedulerListResponse, error)
	ListBackupLogs(taskID int) (*adapters.DSMBackupLogListResponse, error)
}

type deviceBackend struct {
	device  string
	backend BackupBackend
}

// Service implements backup business logic.
type Service struct {
	backends []deviceBackend
	monitor  adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new backup service with zero or more backends.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(backends map[string]BackupBackend, monitor ...adapters.AvailabilityChecker) *Service {
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

func (s *Service) findBackend(device string) (BackupBackend, error) {
	for _, db := range s.backends {
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
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsBackups() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}

		tasks, _, err := s.fetchBackupData(db.backend)
		if err != nil {
			return BackupTaskList{}, fmt.Errorf("list backup tasks from %s: %w", db.device, err)
		}

		for _, t := range tasks.TaskList {
			items = append(items, BackupTask{
				Device:     db.device,
				Id:         fmt.Sprintf("%s.%d", db.device, t.TaskID),
				Name:       t.Name,
				Status:     mapBackupStatus(t.State),
				LastResult: mapBackupResult(nil),
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
	device, rawID, err := parseTaskID(taskID)
	if err != nil {
		return nil, err
	}

	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}

	tasks, _, err := s.fetchBackupData(backend)
	if err != nil {
		return nil, fmt.Errorf("get backup task from %s: %w", device, err)
	}

	for _, t := range tasks.TaskList {
		compositeID := fmt.Sprintf("%s.%d", device, t.TaskID)
		if compositeID != taskID && fmt.Sprintf("%d", t.TaskID) != rawID {
			continue
		}

		return &BackupTaskDetail{
			Device:     device,
			Id:         compositeID,
			Name:       t.Name,
			Status:     mapBackupStatus(t.State),
			LastResult: mapBackupResult(nil),
			Type:       mapBackupType(t.Type),
			NextRunAt:  nil,
			LastRunAt:  nil,
		}, nil
	}
	return nil, nil
}

// fetchBackupData retrieves backup tasks and scheduled tasks from a backend.
func (s *Service) fetchBackupData(backend BackupBackend) (*adapters.DSMBackupTaskListResponse, *adapters.DSMTaskSchedulerListResponse, error) {
	tasks, err := backend.ListBackupTasks()
	if err != nil {
		return nil, nil, fmt.Errorf("list backup tasks: %w", err)
	}

	scheduledTasks, err := backend.ListScheduledTasks()
	if err != nil {
		return nil, nil, fmt.Errorf("list scheduled tasks: %w", err)
	}

	return tasks, scheduledTasks, nil
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
		return Unknown
	}
	switch status.LastBkpResult {
	case "done":
		if status.LastBkpErrorCode != 0 {
			return Warning
		}
		return Success
	case "error":
		return Failed
	case "skip":
		return Skipped
	default:
		return Unknown
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
// The "state" field from SYNO.Backup.Task represents whether the task can run.
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
// Examples: "image:image_local" → "hyperBackup", "glacier" → "glacierBackup".
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
