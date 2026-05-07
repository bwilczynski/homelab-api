package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

type mockBackupBackend struct {
	tasks      *adapters.DSMBackupTaskListResponse
	taskDetail *adapters.DSMBackupTaskDetailResponse
	taskStatus *adapters.DSMBackupTaskStatusResponse
	target     *adapters.DSMBackupTargetResponse
	tasksErr   error
	detailErr  error
	statusErr  error
	targetErr  error
}

func (m *mockBackupBackend) SupportsBackups() bool    { return true }
func (m *mockBackupBackend) Location() *time.Location { return time.UTC }

func (m *mockBackupBackend) ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error) {
	return m.tasks, m.tasksErr
}
func (m *mockBackupBackend) GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error) {
	return m.taskDetail, m.detailErr
}
func (m *mockBackupBackend) GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error) {
	return m.taskStatus, m.statusErr
}
func (m *mockBackupBackend) GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error) {
	return m.target, m.targetErr
}

func TestListBackupTasks(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	taskStatus := loadFixture[adapters.DSMBackupTaskStatusResponse](t, "testdata/backup_task_status.json")

	svc := NewService(
		map[string]StorageBackend{},
		map[string]BackupBackend{"nas-01": &mockBackupBackend{tasks: &tasks, taskStatus: &taskStatus}},
	)

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
		t.Errorf("expected status idle, got %v", task.Status)
	}
	if task.LastResult != BackupTaskResultWarning {
		t.Errorf("expected lastResult Warning, got %v", task.LastResult)
	}
	if task.Type != "hyperBackup" {
		t.Errorf("expected type hyperBackup, got %s", task.Type)
	}
}

func TestListBackupTasksWithDeviceFilter(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")

	svc := NewService(
		map[string]StorageBackend{},
		map[string]BackupBackend{"nas-01": &mockBackupBackend{tasks: &tasks}},
	)

	device := "nas-01"
	result, err := svc.ListBackupTasks(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 tasks for matching device, got %d", len(result.Items))
	}

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
	svc := NewService(
		map[string]StorageBackend{},
		map[string]BackupBackend{
			"nas-01": &mockBackupBackend{
				tasks: &adapters.DSMBackupTaskListResponse{TaskList: []adapters.DSMBackupTask{}},
			},
		},
	)

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
	taskDetail := loadFixture[adapters.DSMBackupTaskDetailResponse](t, "testdata/backup_task_detail.json")
	taskStatus := loadFixture[adapters.DSMBackupTaskStatusResponse](t, "testdata/backup_task_status.json")
	target := loadFixture[adapters.DSMBackupTargetResponse](t, "testdata/backup_target.json")

	svc := NewService(
		map[string]StorageBackend{},
		map[string]BackupBackend{
			"nas-01": &mockBackupBackend{
				tasks:      &tasks,
				taskDetail: &taskDetail,
				taskStatus: &taskStatus,
				target:     &target,
			},
		},
	)

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
		t.Errorf("expected status idle, got %v", detail.Status)
	}
	if detail.LastResult != BackupTaskResultWarning {
		t.Errorf("expected lastResult Warning, got %v", detail.LastResult)
	}
	if detail.Type != "hyperBackup" {
		t.Errorf("expected type hyperBackup, got %s", detail.Type)
	}
	if detail.LastRunAt == nil {
		t.Error("expected lastRunAt to be set")
	}
	if detail.NextRunAt == nil {
		t.Error("expected nextRunAt to be set")
	}
	if detail.Size == nil {
		t.Error("expected size to be set")
	}
	if detail.Size != nil && *detail.Size != 3206674163 {
		t.Errorf("expected size 3206674163, got %d", *detail.Size)
	}
	if detail.Folders == nil || len(*detail.Folders) == 0 {
		t.Error("expected folders to be non-empty")
	}
	if detail.Folders != nil && len(*detail.Folders) > 0 && (*detail.Folders)[0] != "/volume1/docker" {
		t.Errorf("expected first folder /volume1/docker, got %s", (*detail.Folders)[0])
	}
}

func TestGetBackupTaskNotFound(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")

	svc := NewService(
		map[string]StorageBackend{},
		map[string]BackupBackend{"nas-01": &mockBackupBackend{tasks: &tasks}},
	)

	detail, err := svc.GetBackupTask(context.Background(), "nas-01.999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for missing task, got %+v", detail)
	}
}

func TestGetBackupTaskInvalidID(t *testing.T) {
	svc := NewService(map[string]StorageBackend{}, map[string]BackupBackend{})

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
			t.Errorf("mapBackupStatus(%q) = %v, want %v", tt.state, got, tt.want)
		}
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

func TestParseBackupTime(t *testing.T) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	tests := []struct {
		input   string
		loc     *time.Location
		wantUTC string
		wantNil bool
	}{
		{"2026/04/24 02:30", time.UTC, "2026-04-24T02:30:00Z", false},
		{"2026/04/24 02:30", warsaw, "2026-04-24T00:30:00Z", false},
		{"", time.UTC, "", true},
		{"bad-format", time.UTC, "", true},
	}
	for _, tt := range tests {
		result := parseBackupTime(tt.input, tt.loc)
		if tt.wantNil {
			if result != nil {
				t.Errorf("parseBackupTime(%q) = %v, want nil", tt.input, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("parseBackupTime(%q, %s) = nil, want %s", tt.input, tt.loc, tt.wantUTC)
			continue
		}
		got := result.UTC().Format(time.RFC3339)
		if got != tt.wantUTC {
			t.Errorf("parseBackupTime(%q, %s) = %s, want %s", tt.input, tt.loc, got, tt.wantUTC)
		}
	}
}

func TestMapBackupResult(t *testing.T) {
	tests := []struct {
		status *adapters.DSMBackupTaskStatusResponse
		want   BackupTaskResult
	}{
		{nil, BackupTaskResultUnknown},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "done", LastBkpErrorCode: 0}, BackupTaskResultSuccess},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "done", LastBkpErrorCode: 4401}, BackupTaskResultWarning},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "error"}, BackupTaskResultFailed},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "skip"}, BackupTaskResultSkipped},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "other"}, BackupTaskResultUnknown},
	}
	for _, tt := range tests {
		got := mapBackupResult(tt.status)
		if got != tt.want {
			label := "<nil>"
			if tt.status != nil {
				label = fmt.Sprintf("result=%q code=%d", tt.status.LastBkpResult, tt.status.LastBkpErrorCode)
			}
			t.Errorf("mapBackupResult(%s) = %v, want %v", label, got, tt.want)
		}
	}
}
