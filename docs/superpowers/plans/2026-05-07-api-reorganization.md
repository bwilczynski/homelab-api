# API Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the spec's group/resource URL hierarchy (`/docker/containers/*`, `/storage/backups/*`), rename `internal/containers` → `internal/docker`, absorb `internal/backups` into `internal/storage`, and apply consistent per-resource-group file splitting across all domain packages.

**Architecture:** Each domain package has one `service.go` (Service struct + NewService), narrow backend interfaces and service methods split into `{resource}_service.go` files, one `handler.go`, and one `api.gen.go`. The `internal/containers` and `internal/backups` packages are deleted after migration.

**Tech Stack:** Go 1.22+, chi router, oapi-codegen v2, go test

**Spec:** `docs/superpowers/specs/2026-05-07-api-reorganization-design.md`

---

## File Map

**Created:**
- `oapi-codegen-docker.yaml`
- `internal/docker/service.go`
- `internal/docker/handler.go`
- `internal/docker/service_test.go`
- `internal/docker/testdata/` (copied from containers)
- `internal/storage/volumes_service.go`
- `internal/storage/backups_service.go`
- `internal/storage/backups_service_test.go`
- `internal/system/health_service.go`
- `internal/system/info_service.go`
- `internal/system/utilization_service.go`
- `internal/system/updates_service.go`
- `internal/network/devices_service.go`
- `internal/network/clients_service.go`

**Modified:**
- `Makefile`
- `spec` (submodule pointer)
- `internal/storage/service.go` (struct + NewService, removes all methods — moved to resource files)
- `internal/storage/handler.go` (adds backup handler methods)
- `internal/storage/service_test.go` → renamed to `internal/storage/volumes_service_test.go`
- `internal/system/service.go` (struct + NewService + combining interface only)
- `internal/network/service.go` (struct + NewService + combining interface + shared helpers)
- `cmd/server/main.go`
- `cmd/testserver/main.go`

**Deleted:**
- `oapi-codegen-containers.yaml`
- `oapi-codegen-backups.yaml`
- `internal/containers/` (entire directory)
- `internal/backups/` (entire directory)

---

## Task 1: Advance spec submodule pointer

**Files:**
- Modify: `spec` (submodule)

- [ ] **Step 1: Check the current spec commit**

```bash
git -C spec log --oneline -3
```

Expected output shows `f481090 Reorganize API into group/resource hierarchy` at HEAD.

- [ ] **Step 2: Stage the updated submodule pointer**

```bash
git add spec
```

- [ ] **Step 3: Commit**

```bash
git commit -m "chore: advance spec submodule to group/resource hierarchy"
```

---

## Task 2: Update codegen infrastructure

**Files:**
- Create: `oapi-codegen-docker.yaml`
- Modify: `Makefile`
- Delete: `oapi-codegen-containers.yaml`, `oapi-codegen-backups.yaml`

- [ ] **Step 1: Create `oapi-codegen-docker.yaml`**

```yaml
package: docker
generate:
  chi-server: true
  strict-server: true
  models: true
output: internal/docker/api.gen.go
output-options:
  include-tags:
    - docker
```

- [ ] **Step 2: Update the `generate` target in `Makefile`**

Replace the `generate` target body:

```makefile
generate: bundle ## Generate server stubs from the bundled spec
	@mkdir -p internal/system internal/docker internal/storage internal/network
	$(OAPI_CODEGEN) --config oapi-codegen-system.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-docker.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-storage.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-network.yaml $(SPEC_FILE)
```

- [ ] **Step 3: Delete the old codegen configs**

```bash
rm oapi-codegen-containers.yaml oapi-codegen-backups.yaml
```

- [ ] **Step 4: Commit**

```bash
git add oapi-codegen-docker.yaml Makefile
git rm oapi-codegen-containers.yaml oapi-codegen-backups.yaml
git commit -m "chore: update codegen configs for group/resource hierarchy"
```

---

## Task 3: Regenerate api.gen.go files

**Files:**
- Create: `internal/docker/api.gen.go`
- Modify: `internal/storage/api.gen.go` (adds backup operations)

- [ ] **Step 1: Bundle the spec and regenerate**

```bash
make generate
```

Expected: no errors. Two files updated/created:
- `internal/docker/api.gen.go` — docker tag, `/docker/containers/*` routes
- `internal/storage/api.gen.go` — storage tag, now includes `/storage/backups/*`

- [ ] **Step 2: Verify the generated interfaces**

```bash
grep -E "^func \(w \*StrictServerInterface\)" internal/docker/api.gen.go || \
grep "StrictServerInterface" internal/docker/api.gen.go | head -20
grep "ListBackups\|GetBackup" internal/storage/api.gen.go | head -10
```

Expected: `ListBackups` and `GetBackup` present in `internal/storage/api.gen.go`.

- [ ] **Step 3: Commit**

```bash
git add internal/docker/api.gen.go internal/storage/api.gen.go
git commit -m "chore: regenerate api.gen.go for docker and storage domains"
```

---

## Task 4: Create internal/docker package

**Files:**
- Create: `internal/docker/service.go`, `internal/docker/handler.go`, `internal/docker/service_test.go`
- Create: `internal/docker/testdata/` (copied from `internal/containers/testdata/`)

The logic is identical to `internal/containers/` — this is a package rename. The generated `api.gen.go` already exists from Task 3.

- [ ] **Step 1: Copy testdata fixtures**

```bash
cp -r internal/containers/testdata internal/docker/testdata
```

- [ ] **Step 2: Write `internal/docker/service_test.go`**

Copy from `internal/containers/service_test.go`, changing only the package declaration:

```go
package docker
```

All test code, mock struct, and `loadFixture` helper remain identical to `internal/containers/service_test.go`.

- [ ] **Step 3: Run tests — verify they fail to compile**

```bash
go test ./internal/docker/
```

Expected: compile error — `Service`, `ContainerBackend`, etc. not defined yet.

- [ ] **Step 4: Write `internal/docker/service.go`**

Copy from `internal/containers/service.go`, changing only the package declaration:

```go
package docker
```

All types (`ContainerBackend`, `Service`, `deviceBackend`), `NewService`, and all service methods (`ListContainers`, `GetContainer`, `StartContainer`, `StopContainer`, `RestartContainer`), and all helpers (`parseContainerID`, `mapRestartPolicy`, `mapStatus`, `mapContainer`, `mapContainerDetail`) remain identical to `internal/containers/service.go`.

- [ ] **Step 5: Write `internal/docker/handler.go`**

Copy from `internal/containers/handler.go`, changing only the package declaration:

```go
package docker
```

All handler code (`ServerHandler`, `NewHandler`, `ListContainers`, `GetContainer`, `StartContainer`, `StopContainer`, `RestartContainer`) remains identical.

- [ ] **Step 6: Run tests — verify they pass**

```bash
go test ./internal/docker/ -v
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/docker/
git commit -m "feat: add internal/docker package (rename from containers)"
```

---

## Task 5: Expand internal/storage — split files and add backups

**Files:**
- Create: `internal/storage/volumes_service.go`, `internal/storage/backups_service.go`, `internal/storage/backups_service_test.go`
- Modify: `internal/storage/service.go`, `internal/storage/handler.go`
- Rename: `internal/storage/service_test.go` → `internal/storage/volumes_service_test.go`
- Copy testdata from `internal/backups/testdata/` into `internal/storage/testdata/`

- [ ] **Step 1: Copy backup testdata fixtures**

```bash
cp internal/backups/testdata/backup_tasks.json internal/storage/testdata/
cp internal/backups/testdata/backup_task_detail.json internal/storage/testdata/
cp internal/backups/testdata/backup_task_status.json internal/storage/testdata/
cp internal/backups/testdata/backup_target.json internal/storage/testdata/
```

- [ ] **Step 2: Write `internal/storage/backups_service_test.go`**

Adapted from `internal/backups/service_test.go` — package changes to `storage`, types reference `storage` package. The `loadFixture` helper is defined in `volumes_service_test.go` (after rename in Step 3) so it is not duplicated here.

```go
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
	if task.LastResult != Warning {
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
	if detail.LastResult != Warning {
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
		{nil, Unknown},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "done", LastBkpErrorCode: 0}, Success},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "done", LastBkpErrorCode: 4401}, Warning},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "error"}, Failed},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "skip"}, Skipped},
		{&adapters.DSMBackupTaskStatusResponse{LastBkpResult: "other"}, Unknown},
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
```

- [ ] **Step 3: Run tests — verify compile failure**

```bash
go test ./internal/storage/ -run TestListBackupTasks
```

Expected: compile error — `BackupBackend`, `BackupTaskStatus`, etc. not defined in `storage` package.

- [ ] **Step 4: Rename `internal/storage/service_test.go` to `internal/storage/volumes_service_test.go`**

```bash
mv internal/storage/service_test.go internal/storage/volumes_service_test.go
```

No content changes needed — the package declaration is already `package storage`.

- [ ] **Step 5: Write `internal/storage/volumes_service.go`**

Extract volumes logic from current `internal/storage/service.go`:

```go
package storage

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// StorageBackend defines the adapter interface for volume operations.
type StorageBackend interface {
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
}

type storageDeviceBackend struct {
	device  string
	backend StorageBackend
}

func newStorageDeviceBackends(backends map[string]StorageBackend) []storageDeviceBackend {
	dbs := make([]storageDeviceBackend, 0, len(backends))
	for device, backend := range backends {
		dbs = append(dbs, storageDeviceBackend{device: device, backend: backend})
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].device < dbs[j].device })
	return dbs
}

func (s *Service) findStorageBackend(device string) (StorageBackend, error) {
	for _, db := range s.storageBackends {
		if db.device == device {
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
}

// ListStorageVolumes returns all volumes with their associated disks from all backends.
func (s *Service) ListStorageVolumes(ctx context.Context, device *string) (VolumeList, error) {
	var volumes []Volume
	for _, db := range s.storageBackends {
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
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid volume ID %q: expected format device.name: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}

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
```

- [ ] **Step 6: Write `internal/storage/backups_service.go`**

Migrate from `internal/backups/service.go`, changing `package backups` → `package storage`:

```go
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
	device, rawID, err := parseTaskID(taskID)
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
		if compositeID != taskID && fmt.Sprintf("%d", t.TaskID) != rawID {
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

func parseTaskID(id string) (device, taskID string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid task ID %q: expected format device.taskId: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}

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
```

- [ ] **Step 7: Replace `internal/storage/service.go`**

The current `service.go` holds everything. Replace it with the struct + NewService only:

```go
package storage

import (
	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// Service implements storage domain business logic for volumes and backups.
type Service struct {
	storageBackends []storageDeviceBackend
	backupBackends  []backupDeviceBackend
	monitor         adapters.AvailabilityChecker // optional; nil means all backends available
}

// NewService creates a new storage service.
// An optional AvailabilityChecker (e.g. a health.Monitor) may be passed to skip
// backends that are currently unreachable.
func NewService(storageBackends map[string]StorageBackend, backupBackends map[string]BackupBackend, monitor ...adapters.AvailabilityChecker) *Service {
	svc := &Service{
		storageBackends: newStorageDeviceBackends(storageBackends),
		backupBackends:  newBackupDeviceBackends(backupBackends),
	}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}
```

- [ ] **Step 8: Update `internal/storage/handler.go` to add backup methods**

Add `ListBackups` and `GetBackup` to the existing handler. The generated operation names from the spec are `listBackups` and `getBackup` (operationId), so the interface methods are `ListBackups` and `GetBackup`:

```go
package storage

import (
	"context"
	"errors"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements the generated StrictServerInterface for storage.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new storage handler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListStorageVolumes(ctx context.Context, request ListStorageVolumesRequestObject) (ListStorageVolumesResponseObject, error) {
	result, err := h.svc.ListStorageVolumes(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListStorageVolumes500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListStorageVolumes200JSONResponse(result), nil
}

func (h *ServerHandler) GetStorageVolume(ctx context.Context, request GetStorageVolumeRequestObject) (GetStorageVolumeResponseObject, error) {
	result, err := h.svc.GetStorageVolume(ctx, request.VolumeId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetStorageVolume404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetStorageVolume500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	if result == nil {
		detail := "volume not found"
		return GetStorageVolume404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetStorageVolume200JSONResponse(*result), nil
}

func (h *ServerHandler) ListBackups(ctx context.Context, request ListBackupsRequestObject) (ListBackupsResponseObject, error) {
	result, err := h.svc.ListBackupTasks(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListBackups500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListBackups200JSONResponse(result), nil
}

func (h *ServerHandler) GetBackup(ctx context.Context, request GetBackupRequestObject) (GetBackupResponseObject, error) {
	result, err := h.svc.GetBackupTask(ctx, request.BackupId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetBackup404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetBackup500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	if result == nil {
		detail := "backup not found"
		return GetBackup404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetBackup200JSONResponse(*result), nil
}
```

> **Note:** The exact generated type names (`ListBackupsRequestObject`, `GetBackupRequestObject`, `BackupId` field, etc.) must match what `make generate` produced in Task 3. Verify against `internal/storage/api.gen.go` before writing the handler.

- [ ] **Step 9: Run storage tests — verify all pass**

```bash
go test ./internal/storage/ -v
```

Expected: all volume tests and all backup tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/storage/
git commit -m "feat: split storage into volumes_service.go + backups_service.go, absorb backups domain"
```

---

## Task 6: Split internal/system/service.go

**Files:**
- Create: `internal/system/health_service.go`, `internal/system/info_service.go`, `internal/system/utilization_service.go`, `internal/system/updates_service.go`
- Modify: `internal/system/service.go`

The existing `service_test.go` requires no changes — tests reference the same exported types and method signatures.

- [ ] **Step 1: Create `internal/system/health_service.go`**

Extract from `service.go` all health-related interfaces, types, and methods:

```go
package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// HealthDSMBackend is the narrow interface for health checks on DSM backends.
type HealthDSMBackend interface {
	GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
	ListContainers() (*adapters.DSMContainerListResponse, error)
}

// HealthUniFiBackend is the narrow interface for health checks on UniFi backends.
type HealthUniFiBackend interface {
	GetHealth() ([]adapters.UniFiSubsystemHealth, error)
}

// GetSystemHealth queries all backends for health and assembles an aggregate Health model.
func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	var components []ComponentHealth
	overall := Healthy

	for _, ue := range s.unifiBackends {
		if s.monitor != nil && !s.monitor.Available(ue.controller) {
			name := "network"
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":network"
			}
			msg := "offline"
			components = append(components, ComponentHealth{Name: name, Status: Unhealthy, Message: &msg})
			overall = Unhealthy
			continue
		}
		subsystems, err := ue.unifi.GetHealth()
		if err != nil {
			return Health{}, fmt.Errorf("get unifi health from %s: %w", ue.controller, err)
		}
		for _, sub := range subsystems {
			status := mapUniFiStatus(sub.Status)
			name := sub.Subsystem
			if len(s.unifiBackends) > 1 {
				name = ue.controller + ":" + name
			}
			components = append(components, ComponentHealth{Name: name, Status: status})
			overall = worstStatus(overall, status)
		}
	}

	for _, de := range s.dsmBackends {
		prefix := ""
		if len(s.dsmBackends) > 1 {
			prefix = de.device + ":"
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			msg := "offline"
			components = append(components, ComponentHealth{Name: prefix + "storage", Status: Unhealthy, Message: &msg})
			if de.dockerEnabled {
				components = append(components, ComponentHealth{Name: prefix + "containers", Status: Unhealthy, Message: &msg})
			}
			overall = Unhealthy
			continue
		}
		storageStatus, storageMsg, err := storageHealth(de.dsm)
		if err != nil {
			return Health{}, fmt.Errorf("get storage health from %s: %w", de.device, err)
		}
		storageComponent := ComponentHealth{Name: prefix + "storage", Status: storageStatus}
		if storageMsg != "" {
			storageComponent.Message = &storageMsg
		}
		components = append(components, storageComponent)
		overall = worstStatus(overall, storageStatus)

		if de.dockerEnabled {
			containersStatus, containersMsg, err := containersHealth(de.dsm)
			if err != nil {
				return Health{}, fmt.Errorf("get containers health from %s: %w", de.device, err)
			}
			containersComponent := ComponentHealth{Name: prefix + "containers", Status: containersStatus}
			if containersMsg != "" {
				containersComponent.Message = &containersMsg
			}
			components = append(components, containersComponent)
			overall = worstStatus(overall, containersStatus)
		}
	}

	if components == nil {
		components = []ComponentHealth{}
	}
	return Health{Status: overall, CheckedAt: time.Now().UTC(), Components: components}, nil
}

func storageHealth(dsm HealthDSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.GetStorageVolumes()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
	}
	worst := Healthy
	var degraded, crashed []string
	for _, v := range resp.Volumes {
		st := mapVolumeStatus(v.Status)
		worst = worstStatus(worst, st)
		switch st {
		case Unhealthy:
			crashed = append(crashed, v.ID)
		case Degraded:
			degraded = append(degraded, v.ID)
		}
	}
	var msg string
	if len(crashed) > 0 {
		msg = fmt.Sprintf("crashed: %s", strings.Join(crashed, ", "))
	} else if len(degraded) > 0 {
		msg = fmt.Sprintf("degraded: %s", strings.Join(degraded, ", "))
	}
	return worst, msg, nil
}

func containersHealth(dsm HealthDSMBackend) (HealthStatus, string, error) {
	resp, err := dsm.ListContainers()
	if err != nil {
		return Unhealthy, err.Error(), nil //nolint:nilerr
	}
	notRunning := 0
	for _, c := range resp.Containers {
		if !c.State.Running {
			notRunning++
		}
	}
	if notRunning == 0 {
		return Healthy, "", nil
	}
	return Degraded, fmt.Sprintf("%d container(s) not running", notRunning), nil
}

func worstStatus(a, b HealthStatus) HealthStatus {
	if a == Unhealthy || b == Unhealthy {
		return Unhealthy
	}
	if a == Degraded || b == Degraded {
		return Degraded
	}
	return Healthy
}

func mapUniFiStatus(status string) HealthStatus {
	switch status {
	case "ok":
		return Healthy
	case "unknown":
		return Degraded
	default:
		return Unhealthy
	}
}

func mapVolumeStatus(status string) HealthStatus {
	switch status {
	case "normal":
		return Healthy
	case "degraded", "repairing":
		return Degraded
	default:
		return Unhealthy
	}
}
```


- [ ] **Step 2: Create `internal/system/info_service.go`**

```go
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
```

- [ ] **Step 3: Create `internal/system/utilization_service.go`**

```go
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

		const kbToBytes = 1024
		totalBytes := int64(util.Memory.TotalReal) * kbToBytes
		availBytes := int64(util.Memory.AvailReal) * kbToBytes
		totalSwap := int64(util.Memory.TotalSwap) * kbToBytes
		usedSwap := (int64(util.Memory.TotalSwap) - int64(util.Memory.AvailSwap)) * kbToBytes
		cpuTotal := util.CPU.UserLoad + util.CPU.SystemLoad + util.CPU.OtherLoad

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
```

- [ ] **Step 4: Create `internal/system/updates_service.go`**

Extract all updates-related code from `service.go` into this file. This includes `UpdatesDSMBackend`, `githubReleasesCache`, `containerCandidate`, `SeedGitHubReleases`, `ListSystemUpdates`, `GetSystemUpdate`, `CheckSystemUpdates`, `getUpdates`, `buildUpdateItems`, `getOrFetchReleases`, `resolveSource`, `githubRepoFromGHCR`, `normalizeVersion`, `splitImageTag`, `isVersionTag`, `toSystemUpdateList`:

```go
package system

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// UpdatesDSMBackend is the narrow interface for update checks on DSM backends.
type UpdatesDSMBackend interface {
	ListContainers() (*adapters.DSMContainerListResponse, error)
}

// githubReleasesCache holds cached GitHub release data indexed by "owner/repo".
type githubReleasesCache struct {
	releases  map[string]*GitHubRelease
	fetchedAt time.Time
}

// containerCandidate holds pre-resolved data for a container before GitHub lookup.
type containerCandidate struct {
	device    string
	name      string
	image     string
	tag       string
	repo      string
	sourceURL string
}

// SeedGitHubReleases pre-populates the GitHub releases cache.
func (s *Service) SeedGitHubReleases(releases map[string]*GitHubRelease) {
	s.mu.Lock()
	s.ghCache = &githubReleasesCache{releases: releases, fetchedAt: time.Now().UTC()}
	s.mu.Unlock()
}

// ListSystemUpdates returns tracked containers and their update status.
func (s *Service) ListSystemUpdates(ctx context.Context, status *SystemUpdateStatus, updateType *SystemUpdateType) (SystemUpdateList, error) {
	items, err := s.getUpdates(ctx)
	if err != nil {
		return SystemUpdateList{}, err
	}
	return s.toSystemUpdateList(items, status, updateType), nil
}

// GetSystemUpdate returns detailed update info for a single tracked component.
func (s *Service) GetSystemUpdate(ctx context.Context, id string) (*SystemUpdateDetail, error) {
	items, err := s.getUpdates(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Id == id {
			var detail SystemUpdateDetail
			if err := detail.FromContainerSystemUpdateDetail(item); err != nil {
				return nil, fmt.Errorf("marshal update detail for %s: %w", id, err)
			}
			return &detail, nil
		}
	}
	return nil, nil
}

// CheckSystemUpdates forces a fresh upstream check and returns the full list.
func (s *Service) CheckSystemUpdates(ctx context.Context) (SystemUpdateList, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	items, err := s.buildUpdateItems(ctx, true)
	if err != nil {
		return SystemUpdateList{}, err
	}
	return s.toSystemUpdateList(items, nil, nil), nil
}

func (s *Service) getUpdates(ctx context.Context) ([]ContainerSystemUpdateDetail, error) {
	return s.buildUpdateItems(ctx, false)
}

func (s *Service) buildUpdateItems(_ context.Context, forceGitHub bool) ([]ContainerSystemUpdateDetail, error) {
	checkedAt := time.Now().UTC()

	var candidates []containerCandidate
	repos := make(map[string]struct{})

	for _, de := range s.dsmBackends {
		if !de.dockerEnabled {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(de.device) {
			continue
		}
		resp, err := de.dsm.ListContainers()
		if err != nil {
			continue
		}
		for _, c := range resp.Containers {
			image, tag := splitImageTag(c.Image)
			if !isVersionTag(tag) {
				continue
			}
			githubRepo, sourceURL := s.resolveSource(image)
			if githubRepo == "" && !s.warnedImages[image] {
				s.warnedImages[image] = true
				s.logger.Warn("no GitHub source for container image; update status will be unknown",
					"container", c.Name, "image", image,
					"hint", "add an entry under updates.sources in config.yaml",
				)
			}
			if githubRepo != "" {
				repos[githubRepo] = struct{}{}
			}
			candidates = append(candidates, containerCandidate{
				device: de.device, name: c.Name,
				image: image, tag: tag,
				repo: githubRepo, sourceURL: sourceURL,
			})
		}
	}

	releases := s.getOrFetchReleases(repos, forceGitHub)

	items := make([]ContainerSystemUpdateDetail, 0, len(candidates))
	for _, cc := range candidates {
		item := ContainerSystemUpdateDetail{
			Id:             cc.device + "." + cc.name,
			Name:           cc.name,
			Type:           ContainerSystemUpdateDetailTypeContainer,
			Status:         Unknown,
			CurrentVersion: cc.tag,
			LatestVersion:  cc.tag,
			CheckedAt:      checkedAt,
			Image:          cc.image,
			Device:         cc.device,
			Source:         cc.sourceURL,
			ReleaseUrl:     cc.sourceURL + "/releases",
			PublishedAt:    checkedAt,
		}
		if release, ok := releases[cc.repo]; ok {
			item.LatestVersion = release.TagName
			item.ReleaseUrl = release.HTMLURL
			item.PublishedAt = release.PublishedAt
			if normalizeVersion(release.TagName) == normalizeVersion(cc.tag) {
				item.Status = UpToDate
			} else {
				item.Status = UpdateAvailable
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) getOrFetchReleases(repos map[string]struct{}, forceGitHub bool) map[string]*GitHubRelease {
	if !forceGitHub {
		s.mu.RLock()
		if s.ghCache != nil && time.Since(s.ghCache.fetchedAt) < s.updateCacheTTL {
			releases := s.ghCache.releases
			s.mu.RUnlock()
			return releases
		}
		s.mu.RUnlock()

		s.refreshMu.Lock()
		defer s.refreshMu.Unlock()

		s.mu.RLock()
		if s.ghCache != nil && time.Since(s.ghCache.fetchedAt) < s.updateCacheTTL {
			releases := s.ghCache.releases
			s.mu.RUnlock()
			return releases
		}
		s.mu.RUnlock()
	}

	fresh := fetchReleases(repos, s.logger)

	s.mu.Lock()
	prevLen := 0
	if s.ghCache != nil {
		prevLen = len(s.ghCache.releases)
	}
	merged := make(map[string]*GitHubRelease, max(len(fresh), prevLen))
	if s.ghCache != nil {
		maps.Copy(merged, s.ghCache.releases)
	}
	maps.Copy(merged, fresh)
	s.ghCache = &githubReleasesCache{releases: merged, fetchedAt: time.Now().UTC()}
	s.mu.Unlock()
	return merged
}

func (s *Service) resolveSource(image string) (githubRepo string, sourceURL string) {
	if repo, ok := s.sources[image]; ok {
		return repo, fmt.Sprintf("https://github.com/%s", repo)
	}
	if repo, ok := githubRepoFromGHCR(image); ok {
		return repo, fmt.Sprintf("https://github.com/%s", repo)
	}
	return "", "https://github.com"
}

func githubRepoFromGHCR(image string) (string, bool) {
	const prefix = "ghcr.io/"
	if !strings.HasPrefix(image, prefix) {
		return "", false
	}
	rest := image[len(prefix):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

func splitImageTag(image string) (string, string) {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return image, ""
	}
	return image[:i], image[i+1:]
}

func isVersionTag(tag string) bool {
	if tag == "" || tag == "latest" {
		return false
	}
	if strings.HasPrefix(tag, "sha256:") {
		return false
	}
	return true
}

func (s *Service) toSystemUpdateList(items []ContainerSystemUpdateDetail, status *SystemUpdateStatus, updateType *SystemUpdateType) SystemUpdateList {
	result := make([]SystemUpdate, 0, len(items))
	for _, item := range items {
		if status != nil && item.Status != *status {
			continue
		}
		if updateType != nil && SystemUpdateType(item.Type) != *updateType {
			continue
		}
		result = append(result, SystemUpdate{
			Id:             item.Id,
			Name:           item.Name,
			Type:           SystemUpdateType(item.Type),
			Status:         item.Status,
			Device:         item.Device,
			CurrentVersion: item.CurrentVersion,
			LatestVersion:  item.LatestVersion,
			CheckedAt:      item.CheckedAt,
		})
	}
	return SystemUpdateList{Items: result}
}
```

- [ ] **Step 5: Replace `internal/system/service.go`**

Replace the entire file with just the struct, NewService, and the combining DSMBackend interface:

```go
package system

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/config"
)

// DSMBackend is the combined interface satisfied by the Synology adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type DSMBackend interface {
	HealthDSMBackend
	InfoDSMBackend
	UtilizationDSMBackend
	UpdatesDSMBackend
}

// UniFiBackend is the combined interface satisfied by the UniFi adapter.
type UniFiBackend interface {
	HealthUniFiBackend
}

// DSMBackendConfig wraps a DSMBackend with feature flags.
type DSMBackendConfig struct {
	Backend       DSMBackend
	DockerEnabled bool
}

type dsmEntry struct {
	device        string
	dsm           DSMBackend
	dockerEnabled bool
}

type unifiEntry struct {
	controller string
	unifi      UniFiBackend
}

// Service implements system domain business logic.
type Service struct {
	dsmBackends    []dsmEntry
	unifiBackends  []unifiEntry
	monitor        adapters.AvailabilityChecker
	sources        map[string]string
	updateCacheTTL time.Duration
	logger         *slog.Logger
	warnedImages   map[string]bool
	mu             sync.RWMutex
	ghCache        *githubReleasesCache
	refreshMu      sync.Mutex
}

// NewService creates a new system service.
func NewService(dsmBackends map[string]DSMBackendConfig, unifiBackends map[string]UniFiBackend, updatesCfg config.UpdatesConfig, logger *slog.Logger, monitor ...adapters.AvailabilityChecker) *Service {
	dsms := make([]dsmEntry, 0, len(dsmBackends))
	for device, cfg := range dsmBackends {
		dsms = append(dsms, dsmEntry{device: device, dsm: cfg.Backend, dockerEnabled: cfg.DockerEnabled})
	}
	sort.Slice(dsms, func(i, j int) bool { return dsms[i].device < dsms[j].device })

	unifis := make([]unifiEntry, 0, len(unifiBackends))
	for controller, unifi := range unifiBackends {
		unifis = append(unifis, unifiEntry{controller: controller, unifi: unifi})
	}
	sort.Slice(unifis, func(i, j int) bool { return unifis[i].controller < unifis[j].controller })

	srcMap := make(map[string]string, len(updatesCfg.Sources))
	for _, s := range updatesCfg.Sources {
		srcMap[s.Image] = s.Source
	}

	ttl := updatesCfg.CheckInterval.Duration
	if ttl <= 0 {
		ttl = time.Hour
	}

	svc := &Service{
		dsmBackends:    dsms,
		unifiBackends:  unifis,
		sources:        srcMap,
		updateCacheTTL: ttl,
		logger:         logger,
		warnedImages:   make(map[string]bool),
	}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}
```

- [ ] **Step 6: Verify compilation**

```bash
go build ./internal/system/
```

Expected: no errors.

- [ ] **Step 7: Run system tests**

```bash
go test ./internal/system/ -v
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/system/
git commit -m "refactor: split system/service.go into per-resource-group files"
```

---

## Task 7: Split internal/network/service.go

**Files:**
- Create: `internal/network/devices_service.go`, `internal/network/clients_service.go`
- Modify: `internal/network/service.go`

- [ ] **Step 1: Create `internal/network/devices_service.go`**

```go
package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// DevicesBackend is the narrow interface for device operations.
type DevicesBackend interface {
	GetDevices() ([]adapters.UniFiDevice, error)
}

// ListDevices retrieves all managed network devices from all backends.
func (s *Service) ListDevices(ctx context.Context) (NetworkDeviceList, error) {
	var items []NetworkDevice
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		raw, err := cb.unifi.GetDevices()
		if err != nil {
			return NetworkDeviceList{}, fmt.Errorf("get unifi devices from %s: %w", cb.controller, err)
		}
		for _, d := range raw {
			items = append(items, deviceToList(cb.controller, d))
		}
	}
	if items == nil {
		items = []NetworkDevice{}
	}
	return NetworkDeviceList{Items: items}, nil
}

// GetDevice looks up a single device by composite ID and returns its detail.
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkDeviceDetail{}, false, nil
	}
	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkDeviceDetail{}, false, nil
	}
	raw, err := backend.GetDevices()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}
	for _, d := range raw {
		if toKebab(d.Name) == suffix {
			return deviceToDetail(controller, d), true, nil
		}
	}
	return NetworkDeviceDetail{}, false, nil
}

func deviceToList(controller string, d adapters.UniFiDevice) NetworkDevice {
	mac := normalizeMac(d.MAC)
	dev := NetworkDevice{
		Id:     fmt.Sprintf("%s.%s", controller, toKebab(d.Name)),
		Name:   d.Name,
		Mac:    mac,
		Ip:     d.IP,
		Type:   mapDeviceType(d.Type),
		Status: mapDeviceStatus(d.State),
	}
	if d.Type == "uap" {
		total := d.UserNumSta + d.GuestNumSta
		dev.NumClients = &total
	}
	return dev
}

func deviceToDetail(controller string, d adapters.UniFiDevice) NetworkDeviceDetail {
	mac := normalizeMac(d.MAC)
	det := NetworkDeviceDetail{
		Id:              fmt.Sprintf("%s.%s", controller, toKebab(d.Name)),
		Name:            d.Name,
		Mac:             mac,
		Ip:              d.IP,
		Type:            mapDeviceType(d.Type),
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
	}
	if d.Type == "uap" {
		total := d.UserNumSta + d.GuestNumSta
		det.NumClients = &total
	}
	return det
}

func mapDeviceType(t string) NetworkDeviceType {
	switch t {
	case "uap":
		return AccessPoint
	case "usw":
		return Switch
	case "ugw", "udm", "udm-pro":
		return Gateway
	default:
		return Unknown
	}
}

func mapDeviceStatus(state int) NetworkDeviceStatus {
	if state == 1 {
		return Connected
	}
	return Disconnected
}
```

- [ ] **Step 2: Create `internal/network/clients_service.go`**

```go
package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// ClientsBackend is the narrow interface for client operations.
type ClientsBackend interface {
	GetClients() ([]adapters.UniFiSta, error)
}

// ListClients retrieves all connected clients from all backends.
func (s *Service) ListClients(ctx context.Context) (NetworkClientList, error) {
	var items []NetworkClient
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		raw, err := cb.unifi.GetClients()
		if err != nil {
			return NetworkClientList{}, fmt.Errorf("get unifi clients from %s: %w", cb.controller, err)
		}
		for _, sta := range raw {
			items = append(items, clientToList(cb.controller, sta))
		}
	}
	if items == nil {
		items = []NetworkClient{}
	}
	return NetworkClientList{Items: items}, nil
}

// GetClient looks up a single client by composite ID and returns its typed detail.
func (s *Service) GetClient(ctx context.Context, id string) (NetworkClientDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkClientDetail{}, false, nil
	}
	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkClientDetail{}, false, nil
	}
	raw, err := backend.GetClients()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}
	for _, sta := range raw {
		if clientSuffix(sta) == suffix {
			detail, err := clientToDetail(controller, sta)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkClientDetail{}, false, nil
}

func clientToList(controller string, sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	client := NetworkClient{
		Id:             fmt.Sprintf("%s.%s", controller, clientSuffix(sta)),
		Name:           clientName(sta),
		Mac:            mac,
		ConnectionType: mapConnectionType(sta.IsWired),
	}
	if sta.IP != "" {
		ip := sta.IP
		client.Ip = &ip
	}
	return client
}

func clientToDetail(controller string, sta adapters.UniFiSta) (NetworkClientDetail, error) {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	name := clientName(sta)
	var ip *string
	if sta.IP != "" {
		v := sta.IP
		ip = &v
	}

	var detail NetworkClientDetail
	if sta.IsWired {
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			SwitchName:     sta.LastUplinkName,
			SwitchPort:     sta.SwPort,
			Uptime:         sta.Uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wired client detail: %w", err)
		}
	} else {
		ssid := ""
		if sta.ESSID != nil {
			ssid = *sta.ESSID
		}
		signal := 0
		if sta.Signal != nil {
			signal = *sta.Signal
		}
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Ssid:           ssid,
			SignalStrength:  signal,
			Uptime:         sta.Uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wireless client detail: %w", err)
		}
	}
	return detail, nil
}

func clientSuffix(sta adapters.UniFiSta) string {
	mac := normalizeMac(sta.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	return fmt.Sprintf("%s-%s", toKebab(clientName(sta)), prefix)
}

func clientName(sta adapters.UniFiSta) string {
	if sta.Name != nil && *sta.Name != "" {
		return *sta.Name
	}
	if sta.Hostname != nil && *sta.Hostname != "" {
		return *sta.Hostname
	}
	return sta.MAC
}

func mapConnectionType(isWired bool) NetworkClientConnectionType {
	if isWired {
		return NetworkClientConnectionTypeWired
	}
	return NetworkClientConnectionTypeWireless
}
```


- [ ] **Step 3: Replace `internal/network/service.go`**

Keep only the struct, NewService, combining UniFiBackend, shared helpers (`findBackend`, `parseID`, `toKebab`, `normalizeMac`):

```go
package network

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
}

type controllerBackend struct {
	controller string
	unifi      UniFiBackend
}

// Service implements network domain business logic.
type Service struct {
	backends []controllerBackend
	monitor  adapters.AvailabilityChecker
}

// NewService creates a new network service.
func NewService(backends map[string]UniFiBackend, monitor ...adapters.AvailabilityChecker) *Service {
	cbs := make([]controllerBackend, 0, len(backends))
	for controller, unifi := range backends {
		cbs = append(cbs, controllerBackend{controller: controller, unifi: unifi})
	}
	sort.Slice(cbs, func(i, j int) bool { return cbs[i].controller < cbs[j].controller })
	svc := &Service{backends: cbs}
	if len(monitor) > 0 {
		svc.monitor = monitor[0]
	}
	return svc
}

func (s *Service) findBackend(controller string) (UniFiBackend, error) {
	for _, cb := range s.backends {
		if cb.controller == controller {
			return cb.unifi, nil
		}
	}
	return nil, fmt.Errorf("unknown controller %q: %w", controller, apierrors.ErrNotFound)
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func toKebab(name string) string {
	lower := strings.ToLower(name)
	kebab := nonAlphanumRe.ReplaceAllString(lower, "-")
	return strings.Trim(kebab, "-")
}

func parseID(id string) (controller, suffix string, ok bool) {
	dot := strings.IndexByte(id, '.')
	if dot <= 0 || dot == len(id)-1 {
		return "", "", false
	}
	return id[:dot], id[dot+1:], true
}

func normalizeMac(mac string) string {
	return strings.ToLower(mac)
}
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./internal/network/
```

Expected: no errors.

- [ ] **Step 5: Run network tests**

```bash
go test ./internal/network/ -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/network/
git commit -m "refactor: split network/service.go into devices_service.go + clients_service.go"
```

---

## Task 8: Update cmd/server/main.go

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Update imports**

Replace `"github.com/bwilczynski/homelab-api/internal/backups"` and `"github.com/bwilczynski/homelab-api/internal/containers"` with `"github.com/bwilczynski/homelab-api/internal/docker"`.

- [ ] **Step 2: Replace containers + backups wiring with docker + updated storage**

Replace the container and backup wiring blocks:

```go
// Docker containers: all Synology backends; capability checked per-request.
dockerBackends := make(map[string]docker.ContainerBackend, len(synologyClients))
for name, client := range synologyClients {
    dockerBackends[name] = client
}
dockerSvc := docker.NewService(dockerBackends, monitor)
docker.HandlerWithOptions(docker.NewStrictHandler(docker.NewHandler(dockerSvc), nil), docker.ChiServerOptions{
    BaseRouter:       protected,
    Middlewares:      []docker.MiddlewareFunc{scopeMw},
    ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
})

// Storage: all Synology backends for volumes and backups.
storageBackends := make(map[string]storage.StorageBackend, len(synologyClients))
backupBackends := make(map[string]storage.BackupBackend, len(synologyClients))
for name, client := range synologyClients {
    storageBackends[name] = client
    backupBackends[name] = client
}
storageSvc := storage.NewService(storageBackends, backupBackends, monitor)
storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
    BaseRouter:       protected,
    Middlewares:      []storage.MiddlewareFunc{scopeMw},
    ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
})
```

Remove the old standalone backups wiring block entirely.

- [ ] **Step 3: Build**

```bash
go build ./cmd/server/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire docker and storage (with backups) in server router"
```

---

## Task 9: Update cmd/testserver/main.go

**Files:**
- Modify: `cmd/testserver/main.go`

- [ ] **Step 1: Update imports**

Replace `"github.com/bwilczynski/homelab-api/internal/backups"` and `"github.com/bwilczynski/homelab-api/internal/containers"` with `"github.com/bwilczynski/homelab-api/internal/docker"`.

- [ ] **Step 2: Rename `mockContainerBackend` struct and its interface comment**

Change the comment `// containers.ContainerBackend` → `// docker.ContainerBackend`. The struct itself stays `mockContainerBackend`.

- [ ] **Step 3: Replace containers + backups wiring with docker + updated storage**

In `main()`, replace the Containers block and Backups block:

```go
// Docker containers
containerList := new(loadFixture[adapters.DSMContainerListResponse](base + "/docker/testdata/container_list.json"))
cb := &mockContainerBackend{
    list:      containerList,
    detail:    new(loadFixture[adapters.DSMContainerDetailResponse](base + "/docker/testdata/container_detail.json")),
    resources: new(loadFixture[adapters.DSMContainerResourceResponse](base + "/docker/testdata/container_resources.json")),
}
dockerSvc := docker.NewService(map[string]docker.ContainerBackend{"nas-01": cb})
docker.HandlerWithOptions(docker.NewStrictHandler(docker.NewHandler(dockerSvc), nil), docker.ChiServerOptions{
    BaseRouter:       r,
    ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
})

// Storage (volumes + backups)
sb := &mockStorageBackend{
    volumes: new(loadFixture[adapters.DSMStorageVolumeResponse](base + "/storage/testdata/storage_volumes.json")),
}
bb := &mockBackupBackend{
    tasks:      new(loadFixture[adapters.DSMBackupTaskListResponse](base + "/storage/testdata/backup_tasks.json")),
    taskDetail: new(loadFixture[adapters.DSMBackupTaskDetailResponse](base + "/storage/testdata/backup_task_detail.json")),
    taskStatus: new(loadFixture[adapters.DSMBackupTaskStatusResponse](base + "/storage/testdata/backup_task_status.json")),
    target:     new(loadFixture[adapters.DSMBackupTargetResponse](base + "/storage/testdata/backup_target.json")),
}
storageSvc := storage.NewService(
    map[string]storage.StorageBackend{"nas-01": sb},
    map[string]storage.BackupBackend{"nas-01": bb},
)
storage.HandlerWithOptions(storage.NewStrictHandler(storage.NewHandler(storageSvc), nil), storage.ChiServerOptions{
    BaseRouter:       r,
    ErrorHandlerFunc: apierrors.ProblemBadRequestHandler,
})
```

Also update the mock struct comment for `mockBackupBackend`: `// storage.BackupBackend` (was `// backups.BackupBackend`).

- [ ] **Step 4: Build the testserver**

```bash
go build ./cmd/testserver/
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/testserver/main.go
git commit -m "feat: wire docker and storage in testserver"
```

---

## Task 10: Delete old packages and verify

**Files:**
- Delete: `internal/containers/`, `internal/backups/`

- [ ] **Step 1: Delete old packages**

```bash
git rm -r internal/containers/ internal/backups/
```

- [ ] **Step 2: Verify full build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 4: Run lint**

```bash
make lint
```

Expected: no issues.

- [ ] **Step 5: Commit**

```bash
git commit -m "chore: delete internal/containers and internal/backups — migrated to docker and storage"
```
