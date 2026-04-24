package backups

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// BackupBackend defines the adapter interface for backup operations.
type BackupBackend interface {
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
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q", device)
}

// ListBackupTasks returns backup tasks from all (or a filtered) backends.
func (s *Service) ListBackupTasks(ctx context.Context, device *string) (BackupTaskList, error) {
	var items []BackupTask
	for _, db := range s.backends {
		if device != nil && *device != db.device {
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
			logs, _ := db.backend.ListBackupLogs(t.TaskID)
			_, lastResult := findLastCompletion(logs)
			items = append(items, BackupTask{
				Device:     db.device,
				Id:         fmt.Sprintf("%s.%d", db.device, t.TaskID),
				Name:       t.Name,
				Status:     mapBackupStatus(t.State),
				LastResult: lastResult,
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

	tasks, scheduledTasks, err := s.fetchBackupData(backend)
	if err != nil {
		return nil, fmt.Errorf("get backup task from %s: %w", device, err)
	}

	for _, t := range tasks.TaskList {
		compositeID := fmt.Sprintf("%s.%d", device, t.TaskID)
		if compositeID != taskID && fmt.Sprintf("%d", t.TaskID) != rawID {
			continue
		}

		nextRunAt := findNextRunAt(t.Name, scheduledTasks)

		logs, _ := backend.ListBackupLogs(t.TaskID)
		lastRunAt, lastResult := findLastCompletion(logs)

		return &BackupTaskDetail{
			Device:     device,
			Id:         compositeID,
			Name:       t.Name,
			Status:     mapBackupStatus(t.State),
			LastResult: lastResult,
			Type:       mapBackupType(t.Type),
			NextRunAt:  nextRunAt,
			LastRunAt:  lastRunAt,
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

// findNextRunAt finds the next scheduled trigger time for a backup task by name.
// It looks for the primary backup action (not integrity checks).
func findNextRunAt(taskName string, scheduled *adapters.DSMTaskSchedulerListResponse) *time.Time {
	for _, st := range scheduled.Tasks {
		if st.Name != taskName {
			continue
		}
		// Skip integrity check tasks — only use primary backup schedule.
		if strings.Contains(st.Action, "Integrity Check") {
			continue
		}
		return parseSchedulerTime(st.NextTriggerTime)
	}
	return nil
}

// findLastCompletion scans the log list (newest-first) for the most recent task
// completion event and returns the run time and result.
// It detects warnings by checking for warn-level entries between the completion
// and the preceding "backup task started" entry.
func findLastCompletion(logs *adapters.DSMBackupLogListResponse) (*time.Time, BackupTaskResult) {
	if logs == nil {
		return nil, Unknown
	}
	for i, entry := range logs.LogList {
		lower := strings.ToLower(entry.Event)
		if strings.Contains(lower, "backup task failed") {
			return parseLogTime(entry.Time), Failed
		}
		if strings.Contains(lower, "backup task finished") {
			t := parseLogTime(entry.Time)
			for _, older := range logs.LogList[i+1:] {
				if strings.Contains(strings.ToLower(older.Event), "backup task started") {
					break
				}
				if older.Level == "warn" {
					return t, Warning
				}
			}
			return t, Success
		}
	}
	return nil, Unknown
}

// parseSchedulerTime parses the DSM task scheduler time format "2006-01-02 15:04".
func parseSchedulerTime(s string) *time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		return nil
	}
	return &t
}

// parseLogTime parses the DSM backup log time format "2006/01/02 15:04:05".
func parseLogTime(s string) *time.Time {
	t, err := time.Parse("2006/01/02 15:04:05", s)
	if err != nil {
		return nil
	}
	return &t
}

// parseTaskID splits a composite ID "device.taskId" into its parts.
func parseTaskID(id string) (device, taskID string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid task ID %q: expected format device.taskId", id)
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
