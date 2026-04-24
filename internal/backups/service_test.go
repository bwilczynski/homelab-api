package backups

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// mockBackupBackend implements BackupBackend for testing.
type mockBackupBackend struct {
	tasks     *adapters.DSMBackupTaskListResponse
	scheduled *adapters.DSMTaskSchedulerListResponse
	logs      *adapters.DSMBackupLogListResponse
	tasksErr  error
	schedErr  error
	logsErr   error
}

func (m *mockBackupBackend) ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error) {
	return m.tasks, m.tasksErr
}

func (m *mockBackupBackend) ListScheduledTasks() (*adapters.DSMTaskSchedulerListResponse, error) {
	return m.scheduled, m.schedErr
}

func (m *mockBackupBackend) ListBackupLogs(taskID int) (*adapters.DSMBackupLogListResponse, error) {
	return m.logs, m.logsErr
}

func loadFixture[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return envelope.Data
}

func TestListBackupTasks(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	scheduled := loadFixture[adapters.DSMTaskSchedulerListResponse](t, "testdata/task_scheduler.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks, scheduled: &scheduled},
	})

	result, err := svc.ListBackupTasks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(result.Items))
	}

	task := result.Items[0]
	if task.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", task.Device)
	}
	if task.Id != "nas-01.3" {
		t.Errorf("expected id nas-01.3, got %s", task.Id)
	}
	if task.Name != "Backup to LOCAL" {
		t.Errorf("expected name 'Backup to LOCAL', got %s", task.Name)
	}
	if task.Status != Idle {
		t.Errorf("expected status idle, got %s", task.Status)
	}
	if task.LastResult != Unknown {
		t.Errorf("expected lastResult unknown, got %s", task.LastResult)
	}
	if task.Type != "hyperBackup" {
		t.Errorf("expected type hyperBackup, got %s", task.Type)
	}
}

func TestListBackupTasksWithDeviceFilter(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	scheduled := loadFixture[adapters.DSMTaskSchedulerListResponse](t, "testdata/task_scheduler.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks, scheduled: &scheduled},
	})

	// Matching device
	device := "nas-01"
	result, err := svc.ListBackupTasks(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 tasks for matching device, got %d", len(result.Items))
	}

	// Non-matching device
	other := "nas-02"
	result, err = svc.ListBackupTasks(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 tasks for non-matching device, got %d", len(result.Items))
	}
}

func TestListBackupTasksEmpty(t *testing.T) {
	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{
			tasks:     &adapters.DSMBackupTaskListResponse{TaskList: []adapters.DSMBackupTask{}},
			scheduled: &adapters.DSMTaskSchedulerListResponse{Tasks: []adapters.DSMScheduledTask{}},
		},
	})

	result, err := svc.ListBackupTasks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(result.Items))
	}
}

func TestGetBackupTask(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	scheduled := loadFixture[adapters.DSMTaskSchedulerListResponse](t, "testdata/task_scheduler.json")
	logs := loadFixture[adapters.DSMBackupLogListResponse](t, "testdata/backup_logs.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks, scheduled: &scheduled, logs: &logs},
	})

	detail, err := svc.GetBackupTask(context.Background(), "nas-01.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected task detail, got nil")
	}

	if detail.Id != "nas-01.3" {
		t.Errorf("expected id nas-01.3, got %s", detail.Id)
	}
	if detail.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", detail.Device)
	}
	if detail.Name != "Backup to LOCAL" {
		t.Errorf("expected name 'Backup to LOCAL', got %s", detail.Name)
	}
	if detail.Status != Idle {
		t.Errorf("expected status idle, got %s", detail.Status)
	}
	if detail.LastResult != Warning {
		t.Errorf("expected lastResult warning, got %s", detail.LastResult)
	}
	if detail.Type != "hyperBackup" {
		t.Errorf("expected type hyperBackup, got %s", detail.Type)
	}
	if detail.NextRunAt == nil {
		t.Error("expected nextRunAt to be set")
	}
	if detail.LastRunAt == nil {
		t.Error("expected lastRunAt to be set from logs")
	}
}

func TestGetBackupTaskNotFound(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	scheduled := loadFixture[adapters.DSMTaskSchedulerListResponse](t, "testdata/task_scheduler.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks, scheduled: &scheduled},
	})

	detail, err := svc.GetBackupTask(context.Background(), "nas-01.999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for missing task, got %+v", detail)
	}
}

func TestGetBackupTaskInvalidID(t *testing.T) {
	svc := NewService(map[string]BackupBackend{})

	_, err := svc.GetBackupTask(context.Background(), "invalid-id")
	if err == nil {
		t.Fatal("expected error for invalid task ID")
	}
}

func TestMapBackupStatus(t *testing.T) {
	tests := []struct {
		state string
		want  BackupTaskStatus
	}{
		{"backupable", Idle},
		{"running", Running},
		{"suspend", Suspended},
		{"error", Error},
		{"unknown_state", Idle},
	}

	for _, tt := range tests {
		got := mapBackupStatus(tt.state)
		if got != tt.want {
			t.Errorf("mapBackupStatus(%q) = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestFindLastCompletion(t *testing.T) {
	logs := loadFixture[adapters.DSMBackupLogListResponse](t, "testdata/backup_logs.json")

	lastRun, result := findLastCompletion(&logs)
	if lastRun == nil {
		t.Error("expected lastRunAt from logs, got nil")
	}
	// Fixture has a warn-level entry within the run, so result should be Warning.
	if result != Warning {
		t.Errorf("expected Warning, got %s", result)
	}
}

func TestFindLastCompletionEmpty(t *testing.T) {
	lastRun, result := findLastCompletion(&adapters.DSMBackupLogListResponse{})
	if lastRun != nil {
		t.Errorf("expected nil time for empty logs, got %v", lastRun)
	}
	if result != Unknown {
		t.Errorf("expected Unknown for empty logs, got %s", result)
	}
}

func TestFindLastCompletionNil(t *testing.T) {
	lastRun, result := findLastCompletion(nil)
	if lastRun != nil {
		t.Errorf("expected nil time for nil logs, got %v", lastRun)
	}
	if result != Unknown {
		t.Errorf("expected Unknown for nil logs, got %s", result)
	}
}

func TestMapBackupType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"image:image_local", "hyperBackup"},
		{"image:image_remote", "hyperBackup"},
		{"glacier_backup", "glacierBackup"},
		{"custom_type", "custom_type"},
	}

	for _, tt := range tests {
		got := mapBackupType(tt.input)
		if got != tt.want {
			t.Errorf("mapBackupType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseTaskID(t *testing.T) {
	tests := []struct {
		id         string
		wantDevice string
		wantTaskID string
		wantErr    bool
	}{
		{"nas-01.3", "nas-01", "3", false},
		{"device.123", "device", "123", false},
		{"invalid", "", "", true},
		{".taskId", "", "", true},
		{"device.", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		device, taskID, err := parseTaskID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseTaskID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			continue
		}
		if device != tt.wantDevice {
			t.Errorf("parseTaskID(%q) device = %q, want %q", tt.id, device, tt.wantDevice)
		}
		if taskID != tt.wantTaskID {
			t.Errorf("parseTaskID(%q) taskID = %q, want %q", tt.id, taskID, tt.wantTaskID)
		}
	}
}

func TestFindNextRunAt(t *testing.T) {
	scheduled := loadFixture[adapters.DSMTaskSchedulerListResponse](t, "testdata/task_scheduler.json")

	// Task that has a matching scheduler entry
	nextRun := findNextRunAt("Backup to LOCAL", &scheduled)
	if nextRun == nil {
		t.Error("expected nextRunAt for 'Backup to LOCAL', got nil")
	}

	// Task with no matching scheduler entry
	nextRun = findNextRunAt("Nonexistent Task", &scheduled)
	if nextRun != nil {
		t.Errorf("expected nil for unknown task, got %v", nextRun)
	}
}

