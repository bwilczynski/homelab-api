# Backup Detail: Size, Folders, and Timezone Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `size` and `folders` to `BackupTaskDetail`, fix DSM timestamp parsing to use the NAS local timezone instead of UTC, and replace log/scheduler-based timing with the three DSM UI endpoints.

**Architecture:** Three new `SynologyClient` methods match the DSM browser requests (`SYNO.Backup.Task get`, `SYNO.Backup.Task status`, `SYNO.Backup.Target get`). The `BackupBackend` interface gains these three methods and a `Location() *time.Location` method (for timezone-aware parsing). Log-based and scheduler-based helpers are deleted.

**Tech Stack:** Go, oapi-codegen (generated `api.gen.go`), chi router, `time.ParseInLocation`.

---

## File Map

| File | Change |
|------|--------|
| `spec/` (submodule) | Update to new commit |
| `internal/backups/api.gen.go` | Regenerated — gains `Size *int64`, `Folders []string` on `BackupTaskDetail` |
| `internal/config/config.go` | Add `Timezone string` to `Backend` |
| `cmd/server/backends.go` | Load `time.Location` from config, pass to `NewSynologyClient` |
| `internal/adapters/synology.go` | Add `loc` field, three new DSM structs, three new methods, `Location()` |
| `internal/backups/service.go` | Update `BackupBackend` interface, replace helpers, update service methods |
| `internal/backups/service_test.go` | Update mock, add/update tests |
| `internal/backups/testdata/backup_task_detail.json` | New fixture |
| `internal/backups/testdata/backup_task_status.json` | New fixture |
| `internal/backups/testdata/backup_target.json` | New fixture |
| `scripts/dsm-backup-tasks.sh` | Add three new API capture calls |
| `config.sample.yaml` | Document optional `timezone` field |

---

### Task 1: Update spec submodule and regenerate stubs

**Files:**
- Modify: `spec/` (submodule pointer)
- Modify: `internal/backups/api.gen.go` (regenerated)

- [ ] **Step 1: Pull new spec commit into submodule**

```bash
git -C spec pull origin main
git add spec
```

- [ ] **Step 2: Regenerate server stubs**

```bash
make generate
```

Expected: no errors. `internal/backups/api.gen.go` is updated.

- [ ] **Step 3: Verify new fields exist in generated code**

```bash
grep -n "Size\|Folders" internal/backups/api.gen.go
```

Expected output (exact field names may vary slightly by codegen version):
```
    Size    *int64   `json:"size,omitempty"`
    Folders []string `json:"folders,omitempty"`
```

Note the exact Go types here — you will use them in Task 6. If types differ (e.g. `*[]string`), update Task 6 accordingly.

- [ ] **Step 4: Build to confirm no compilation errors**

```bash
make build
```

Expected: compilation fails because `BackupTaskDetail` in service.go doesn't yet set the new fields. That's fine — the build will succeed once the service is updated. If it fails for other reasons, investigate.

- [ ] **Step 5: Commit**

```bash
git add spec internal/backups/api.gen.go
git commit -m "chore: update spec submodule; regenerate backup stubs with size and folders"
```

---

### Task 2: Add timezone to config and SynologyClient

**Files:**
- Modify: `internal/config/config.go:20-28`
- Modify: `internal/adapters/synology.go:32-70`
- Modify: `cmd/server/backends.go:17-18`
- Modify: `config.sample.yaml`

- [ ] **Step 1: Add `Timezone` field to `Backend` in `internal/config/config.go`**

In the `Backend` struct (currently ends at `InsecureTLS`), add:

```go
// Backend describes a single backend target.
type Backend struct {
	Name        string      `yaml:"name"`
	Type        BackendType `yaml:"type"`
	Host        string      `yaml:"host"`
	Username    string      `yaml:"username"`
	Password    string      `yaml:"password"`
	AuthVersion string      `yaml:"auth_version"` // optional; Synology only — overrides the auto-discovered SYNO.API.Auth version
	InsecureTLS bool        `yaml:"insecure_tls"` // optional; skip TLS certificate verification (defaults to false)
	Timezone    string      `yaml:"timezone"`     // optional; IANA timezone name (e.g. "Europe/Warsaw"); defaults to server local TZ
}
```

- [ ] **Step 2: Add `loc` field and update `NewSynologyClient` in `internal/adapters/synology.go`**

Add `loc *time.Location` to the `SynologyClient` struct after `discoveryFailed`:

```go
type SynologyClient struct {
	name          string
	host          string
	user          string
	pass          string
	authVersion   string
	insecureTLS   bool
	authInfo      *dsmAPIInfo
	sid           string
	client        *http.Client
	logger        *slog.Logger
	mu              sync.RWMutex
	supportedAPIs   map[string]bool
	discoveryFailed bool
	loc             *time.Location // timezone for parsing DSM local timestamps
}
```

Update `NewSynologyClient` signature to accept `loc *time.Location`:

```go
func NewSynologyClient(name, host, user, pass, authVersion string, insecureTLS bool, logger *slog.Logger, loc *time.Location) *SynologyClient {
	if authVersion == "" {
		authVersion = "6"
	}
	if logger == nil {
		logger = slog.Default()
	}
	if loc == nil {
		loc = time.Local
	}
	return &SynologyClient{
		name:        name,
		host:        host,
		user:        user,
		pass:        pass,
		authVersion: authVersion,
		insecureTLS: insecureTLS,
		logger:      logger,
		loc:         loc,
		client: &http.Client{
			Transport: tlsTransport(insecureTLS),
		},
	}
}
```

Add a `Location()` method at the bottom of the Synology client section (before the backup types section):

```go
// Location returns the timezone location configured for this client,
// used to parse DSM timestamps that carry no UTC offset.
func (c *SynologyClient) Location() *time.Location {
	return c.loc
}
```

- [ ] **Step 3: Update `buildClients` in `cmd/server/backends.go` to load timezone and pass it**

```go
func buildClients(cfg *config.Config, logger *slog.Logger) (map[string]*adapters.SynologyClient, map[string]*adapters.UniFiClient) {
	synologyClients := make(map[string]*adapters.SynologyClient)
	unifiClients := make(map[string]*adapters.UniFiClient)
	for _, b := range cfg.Backends {
		switch b.Type {
		case config.BackendTypeSynology:
			loc := time.Local
			if b.Timezone != "" {
				if l, err := time.LoadLocation(b.Timezone); err != nil {
					logger.Warn("invalid timezone, falling back to server local TZ", "backend", b.Name, "timezone", b.Timezone, "error", err)
				} else {
					loc = l
				}
			}
			synologyClients[b.Name] = adapters.NewSynologyClient(b.Name, b.Host, b.Username, b.Password, b.AuthVersion, b.InsecureTLS, logger, loc)
		case config.BackendTypeUniFi:
			unifiClients[b.Name] = adapters.NewUniFiClient(b.Host, b.Username, b.Password, b.InsecureTLS)
		}
	}
	return synologyClients, unifiClients
}
```

Add `"time"` to the import if not already present.

- [ ] **Step 4: Document `timezone` in `config.sample.yaml`**

In the first Synology backend block, add the optional field as a comment:

```yaml
  - name: nas-01
    type: synology
    host: 192.168.1.10:5001
    username: admin
    password: ${NAS01_PASS}
    # timezone: Europe/Warsaw  # optional; IANA name — defaults to server local TZ
```

- [ ] **Step 5: Build**

```bash
make build
```

Expected: compilation errors because `NewSynologyClient` call sites now need a `loc` argument. You already fixed the one in `backends.go`; if there are others in test files, fix them the same way (pass `nil` to use server local TZ).

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/adapters/synology.go cmd/server/backends.go config.sample.yaml
git commit -m "feat: add timezone config to Synology backend; default to server local TZ"
```

---

### Task 3: Add new DSM structs and adapter methods

**Files:**
- Modify: `internal/adapters/synology.go` (backup types section, lines ~709–814)

- [ ] **Step 1: Add three new DSM response structs after `DSMBackupLogEntry`**

Add these structs between `DSMBackupLogEntry` and `ListBackupTasks`:

```go
// DSMBackupTaskDetailResponse is the data payload from SYNO.Backup.Task get
// with additional=["repository","schedule"]. Used to retrieve source folders.
type DSMBackupTaskDetailResponse struct {
	TaskID int              `json:"task_id"`
	Name   string           `json:"name"`
	State  string           `json:"state"`
	Source DSMBackupSource  `json:"source"`
}

// DSMBackupSource holds source folder information for a backup task.
type DSMBackupSource struct {
	FolderList []DSMBackupFolderEntry `json:"folder_list"`
}

// DSMBackupFolderEntry is a single source folder entry in a backup task.
type DSMBackupFolderEntry struct {
	FullPath string `json:"fullPath"`
}

// DSMBackupTaskStatusResponse is the data payload from SYNO.Backup.Task status
// with additional=["last_bkp_time","next_bkp_time","last_bkp_result","is_modified","last_bkp_progress","last_bkp_success_version"].
// Provides timing and result for the most recent backup run.
type DSMBackupTaskStatusResponse struct {
	TaskID             int    `json:"task_id"`
	State              string `json:"state"`
	Status             string `json:"status"`
	LastBkpResult      string `json:"last_bkp_result"`
	LastBkpErrorCode   int    `json:"last_bkp_error_code"`
	LastBkpSuccessTime string `json:"last_bkp_success_time"` // format: "2006/01/02 15:04", NAS local TZ
	LastBkpTime        string `json:"last_bkp_time"`         // format: "2006/01/02 15:04", NAS local TZ
	NextBkpTime        string `json:"next_bkp_time"`         // format: "2006/01/02 15:04", NAS local TZ
	IsModified         bool   `json:"is_modified"`
}

// DSMBackupTargetResponse is the data payload from SYNO.Backup.Target get
// with additional=["is_online","used_size","check_task_key","check_auth","account_meta"].
type DSMBackupTargetResponse struct {
	UsedSize int64 `json:"used_size"`
	IsOnline bool  `json:"is_online"`
}
```

- [ ] **Step 2: Add three new methods on `SynologyClient` after `ListBackupLogs`**

```go
// GetBackupTaskDetail retrieves detailed task info (source folders, schedule) from SYNO.Backup.Task get.
func (c *SynologyClient) GetBackupTaskDetail(taskID int) (*DSMBackupTaskDetailResponse, error) {
	data, err := c.Call("SYNO.Backup.Task", "get", "1", url.Values{
		"task_id":    {fmt.Sprintf("%d", taskID)},
		"additional": {`["repository","schedule"]`},
	})
	if err != nil {
		return nil, err
	}
	var result DSMBackupTaskDetailResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse backup task detail: %w", err)
	}
	return &result, nil
}

// GetBackupTaskStatus retrieves last/next run times and result from SYNO.Backup.Task status.
func (c *SynologyClient) GetBackupTaskStatus(taskID int) (*DSMBackupTaskStatusResponse, error) {
	data, err := c.Call("SYNO.Backup.Task", "status", "1", url.Values{
		"task_id":    {fmt.Sprintf("%d", taskID)},
		"blOnline":   {"false"},
		"additional": {`["last_bkp_time","next_bkp_time","last_bkp_result","is_modified","last_bkp_progress","last_bkp_success_version"]`},
	})
	if err != nil {
		return nil, err
	}
	var result DSMBackupTaskStatusResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse backup task status: %w", err)
	}
	return &result, nil
}

// GetBackupTarget retrieves backup target info (used size, online status) from SYNO.Backup.Target get.
func (c *SynologyClient) GetBackupTarget(taskID int) (*DSMBackupTargetResponse, error) {
	data, err := c.Call("SYNO.Backup.Target", "get", "1", url.Values{
		"task_id":    {fmt.Sprintf("%d", taskID)},
		"additional": {`["is_online","used_size","check_task_key","check_auth","account_meta"]`},
	})
	if err != nil {
		return nil, err
	}
	var result DSMBackupTargetResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse backup target: %w", err)
	}
	return &result, nil
}
```

- [ ] **Step 3: Build**

```bash
make build
```

Expected: compiles. No tests yet for the new methods (they talk to a real DSM).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/synology.go
git commit -m "feat: add GetBackupTaskDetail, GetBackupTaskStatus, GetBackupTarget adapter methods"
```

---

### Task 4: Create sanitized test fixtures

**Files:**
- Create: `internal/backups/testdata/backup_task_detail.json`
- Create: `internal/backups/testdata/backup_task_status.json`
- Create: `internal/backups/testdata/backup_target.json`

The timestamps in `backup_task_status.json` are chosen to align with the existing `backup_logs.json` fixture (last run: `2026/04/24 02:30`, next run: `2026/04/25 01:00`) so existing test assertions remain consistent.

- [ ] **Step 1: Verify key sets match raw responses**

```bash
diff <(jq 'keys' scripts/responses/dsm-backup-task-get-raw.json | jq -r '.[]' | sort) \
     <(jq '.data | keys' scripts/responses/dsm-backup-task-get-raw.json | jq -r '.[]' | sort)
```

(Both sides should print the same keys — this confirms the raw response envelope.)

- [ ] **Step 2: Create `internal/backups/testdata/backup_task_detail.json`**

Sanitized from `scripts/responses/dsm-backup-task-get-raw.json`. Hostname `buffalo` → `host-01`, target ID sanitized. Folder names kept as-is (not sensitive).

```json
{
  "data": {
    "data_enc": false,
    "data_type": "data",
    "ext3ShareList": [],
    "name": "Backup to LOCAL",
    "repo_id": 3,
    "repository": {
      "name": "",
      "repo_id": 3,
      "share": "backup",
      "target_type": "image",
      "transfer_type": "image_local"
    },
    "schedule": {
      "schedule": {
        "date": "2026/4/24",
        "date_type": 0,
        "hour": 1,
        "last_work_hour": 1,
        "min": 0,
        "next_trigger_time": "2026-04-25 01:00",
        "repeat": 0,
        "repeat_hour": 0,
        "repeat_hour_store_config": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23],
        "repeat_min": 0,
        "repeat_min_store_config": [],
        "week_name": "0,1,2,3,4,5,6"
      },
      "schedule_enable": true
    },
    "source": {
      "app_config": [],
      "app_list": [],
      "app_name_list": [],
      "backup_filter": {
        "exclude_list": [],
        "whitelist": []
      },
      "backup_volumes": [],
      "file_list": [
        {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "folderPath": "/docker",
          "fullPath": "/volume1/docker",
          "isValidSource": true,
          "missing": false
        },
        {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "folderPath": "/Documents",
          "fullPath": "/volume1/Documents",
          "isValidSource": true,
          "missing": false
        }
      ],
      "folder_list": [
        {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "folderPath": "/docker",
          "fullPath": "/volume1/docker",
          "isValidSource": true,
          "missing": false
        },
        {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "folderPath": "/Documents",
          "fullPath": "/volume1/Documents",
          "isValidSource": true,
          "missing": false
        }
      ],
      "share_list": {
        "Documents": {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "fullPath": "/volume1/Documents",
          "isValidSource": true
        },
        "docker": {
          "dataEncryped": false,
          "encryptedShare": false,
          "fileSystem": "BTRFS",
          "fileSystemType": "internal",
          "fullPath": "/volume1/docker",
          "isValidSource": true
        }
      }
    },
    "state": "backupable",
    "status": "none",
    "target_id": "host-01_1.hbk",
    "target_type": "image",
    "task_id": 3,
    "transfer_type": "image_local",
    "type": "image:image_local"
  },
  "success": true
}
```

- [ ] **Step 3: Create `internal/backups/testdata/backup_task_status.json`**

Sanitized from `scripts/responses/dsm-backup-task-status-raw.json`. Timestamps adjusted to align with existing log fixture (`2026/04/24`). `last_bkp_result: "done"` with `last_bkp_error_code: 4401` → this will map to `Warning` in tests.

```json
{
  "data": {
    "is_modified": false,
    "last_bkp_end_time": "2026/04/24 02:31",
    "last_bkp_error": "",
    "last_bkp_error_code": 4401,
    "last_bkp_result": "done",
    "last_bkp_success_time": "2026/04/24 02:30",
    "last_bkp_success_version": "2729",
    "last_bkp_time": "2026/04/24 02:31",
    "next_bkp_time": "2026/04/25 01:00",
    "schedule": {
      "schedule": {
        "date": "2026/4/24",
        "date_type": 0,
        "hour": 1,
        "last_work_hour": 1,
        "min": 0,
        "next_trigger_time": "2026-04-25 01:00",
        "repeat": 0,
        "repeat_hour": 0,
        "repeat_hour_store_config": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23],
        "repeat_min": 0,
        "repeat_min_store_config": [],
        "week_name": "0,1,2,3,4,5,6"
      },
      "schedule_enable": true
    },
    "state": "backupable",
    "status": "none",
    "task_id": 3
  },
  "success": true
}
```

- [ ] **Step 4: Create `internal/backups/testdata/backup_target.json`**

Sanitized from `scripts/responses/dsm-backup-target-get-raw.json`. `owner_name` and `uni_key` → REDACTED, `host_name` → `host-01`.

```json
{
  "data": {
    "capability": {
      "support_download": true,
      "support_filter": true,
      "support_statistics": true
    },
    "data_comp": true,
    "data_enc": false,
    "format_type": "image",
    "host_name": "host-01",
    "is_online": true,
    "last_detect_time": "2026/04/24 10:57",
    "owner_id": 1026,
    "owner_name": "REDACTED",
    "support_multi_version": true,
    "uni_key": "REDACTED",
    "used_size": 3206674163
  },
  "success": true
}
```

- [ ] **Step 5: Verify fixture key sets against raw responses**

```bash
diff <(jq '.data | keys' scripts/responses/dsm-backup-task-status-raw.json) \
     <(jq '.data | keys' internal/backups/testdata/backup_task_status.json)

diff <(jq '.data | keys' scripts/responses/dsm-backup-target-get-raw.json) \
     <(jq '.data | keys' internal/backups/testdata/backup_target.json)
```

Expected: no diff output.

- [ ] **Step 6: Commit**

```bash
git add internal/backups/testdata/
git commit -m "test: add backup_task_detail, backup_task_status, backup_target fixtures"
```

---

### Task 5: Write and pass tests for new helpers

**Files:**
- Modify: `internal/backups/service_test.go`
- Modify: `internal/backups/service.go`

This task introduces `parseBackupTime` and `mapBackupResult`, replacing `parseLogTime`, `parseSchedulerTime`, `findLastCompletion`, and `findNextRunAt`.

- [ ] **Step 1: Write failing tests for `parseBackupTime` and `mapBackupResult` in `service_test.go`**

Add these test functions at the bottom of `service_test.go`. Replace the existing `TestFindLastCompletion*` and `TestFindNextRunAt` tests with these:

```go
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
		// Europe/Warsaw in April is CEST = UTC+2; so 02:30 local = 00:30 UTC
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
			t.Errorf("mapBackupResult(%s) = %s, want %s", label, got, tt.want)
		}
	}
}
```

Also remove the now-superseded tests: `TestFindLastCompletion`, `TestFindLastCompletionEmpty`, `TestFindLastCompletionNil`, `TestFindNextRunAt`. Remove the `TestMapBackupStatus` test for `parseSchedulerTime` and `parseLogTime` if present.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/backups/ -run "TestParseBackupTime|TestMapBackupResult" -v
```

Expected: `FAIL` with "undefined: parseBackupTime" and "undefined: mapBackupResult".

- [ ] **Step 3: Implement `parseBackupTime` and `mapBackupResult` in `service.go`, remove old helpers**

In `service.go`, replace `parseSchedulerTime`, `parseLogTime`, `findNextRunAt`, and `findLastCompletion` with:

```go
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
```

Also remove the `strings` import if `findNextRunAt` was the last user of `strings.Contains` for integrity check detection. Check that `mapBackupStatus` and `mapBackupType` still use `strings` — if not, remove the import.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/backups/ -run "TestParseBackupTime|TestMapBackupResult" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/backups/service.go internal/backups/service_test.go
git commit -m "feat: add parseBackupTime (timezone-aware) and mapBackupResult; remove log/scheduler helpers"
```

---

### Task 6: Update BackupBackend interface and mock

**Files:**
- Modify: `internal/backups/service.go` (interface)
- Modify: `internal/backups/service_test.go` (mock)

- [ ] **Step 1: Update `BackupBackend` interface in `service.go`**

Replace the current interface with:

```go
// BackupBackend defines the adapter interface for backup operations.
type BackupBackend interface {
	SupportsBackups() bool
	Location() *time.Location
	ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error)
	GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error)
	GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error)
	GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error)
}
```

Remove `ListBackupLogs` and `ListScheduledTasks` from the interface.

- [ ] **Step 2: Update the mock in `service_test.go`**

Replace `mockBackupBackend` with:

```go
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

func (m *mockBackupBackend) SupportsBackups() bool { return true }
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
```

Also add `"time"` to the imports of `service_test.go` if not already present.

- [ ] **Step 3: Build**

```bash
make build
```

Expected: compilation errors because `service.go` still calls `ListBackupLogs` and `ListScheduledTasks`. These are fixed in Task 7.

- [ ] **Step 4: Commit**

```bash
git add internal/backups/service.go internal/backups/service_test.go
git commit -m "refactor: update BackupBackend interface — new DSM methods, remove log/scheduler"
```

---

### Task 7: Update service methods and tests

**Files:**
- Modify: `internal/backups/service.go`
- Modify: `internal/backups/service_test.go`

- [ ] **Step 1: Write the updated/new tests in `service_test.go`**

Replace `TestListBackupTasks`, `TestGetBackupTask`, `TestGetBackupTaskNotFound` with the following. Also update these two existing tests to remove the now-deleted `scheduled` field from the mock:

```go
func TestListBackupTasksWithDeviceFilter(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks},
	})
	// ... rest unchanged
}

func TestListBackupTasksEmpty(t *testing.T) {
	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{
			tasks: &adapters.DSMBackupTaskListResponse{TaskList: []adapters.DSMBackupTask{}},
		},
	})
	// ... rest unchanged
}
```

The tests `TestGetBackupTaskInvalidID`, `TestMapBackupStatus`, `TestParseTaskID` remain unchanged.

```go
func TestListBackupTasks(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	taskStatus := loadFixture[adapters.DSMBackupTaskStatusResponse](t, "testdata/backup_task_status.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks, taskStatus: &taskStatus},
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
	// task_status fixture: last_bkp_result="done", last_bkp_error_code=4401 → Warning
	if task.LastResult != Warning {
		t.Errorf("expected lastResult Warning, got %s", task.LastResult)
	}
	if task.Type != "hyperBackup" {
		t.Errorf("expected type hyperBackup, got %s", task.Type)
	}
}

func TestGetBackupTask(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")
	taskDetail := loadFixture[adapters.DSMBackupTaskDetailResponse](t, "testdata/backup_task_detail.json")
	taskStatus := loadFixture[adapters.DSMBackupTaskStatusResponse](t, "testdata/backup_task_status.json")
	target := loadFixture[adapters.DSMBackupTargetResponse](t, "testdata/backup_target.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{
			tasks:      &tasks,
			taskDetail: &taskDetail,
			taskStatus: &taskStatus,
			target:     &target,
		},
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
		t.Errorf("expected lastResult Warning, got %s", detail.LastResult)
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
	if len(detail.Folders) == 0 {
		t.Error("expected folders to be non-empty")
	}
	if len(detail.Folders) > 0 && detail.Folders[0] != "/volume1/docker" {
		t.Errorf("expected first folder /volume1/docker, got %s", detail.Folders[0])
	}
}

func TestGetBackupTaskNotFound(t *testing.T) {
	tasks := loadFixture[adapters.DSMBackupTaskListResponse](t, "testdata/backup_tasks.json")

	svc := NewService(map[string]BackupBackend{
		"nas-01": &mockBackupBackend{tasks: &tasks},
	})

	detail, err := svc.GetBackupTask(context.Background(), "nas-01.999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for missing task, got %+v", detail)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/backups/ -v 2>&1 | head -40
```

Expected: compilation errors because `service.go` still calls removed methods.

- [ ] **Step 3: Update `ListBackupTasks` in `service.go`**

Replace the inner loop in `ListBackupTasks` (the part that calls `ListBackupLogs`):

```go
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
```

Also remove the `tasks, _, err := s.fetchBackupData(db.backend)` call that fetched scheduled tasks — the list no longer needs scheduled tasks. Replace with:

```go
tasks, err := db.backend.ListBackupTasks()
if err != nil {
	return BackupTaskList{}, fmt.Errorf("list backup tasks from %s: %w", db.device, err)
}
```

- [ ] **Step 4: Update `GetBackupTask` in `service.go`**

Replace the body of the loop inside `GetBackupTask` (currently calls `findNextRunAt`, `ListBackupLogs`, `findLastCompletion`):

```go
for _, t := range tasks.TaskList {
	compositeID := fmt.Sprintf("%s.%d", device, t.TaskID)
	if compositeID != taskID && fmt.Sprintf("%d", t.TaskID) != rawID {
		continue
	}

	loc := backend.Location()

	status, _ := backend.GetBackupTaskStatus(t.TaskID)
	detail, _ := backend.GetBackupTaskDetail(t.TaskID)
	target, _ := backend.GetBackupTarget(t.TaskID)

	lastRunAt := parseBackupTime(status.LastBkpSuccessTime, loc)
	nextRunAt := parseBackupTime(status.NextBkpTime, loc)

	var size *int64
	if target != nil {
		v := target.UsedSize
		size = &v
	}

	var folders []string
	if detail != nil {
		for _, f := range detail.Source.FolderList {
			if f.FullPath != "" {
				folders = append(folders, f.FullPath)
			}
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
```

Also update `GetBackupTask` to use `backend.ListBackupTasks()` directly instead of `fetchBackupData`:

```go
tasks, err := backend.ListBackupTasks()
if err != nil {
	return nil, fmt.Errorf("get backup task from %s: %w", device, err)
}
```

Remove `fetchBackupData` entirely (it fetched both tasks and scheduled tasks; scheduled tasks are no longer needed).

Note: `status.LastBkpSuccessTime` access when `status == nil` would panic. Guard it:

```go
var lastBkpSuccessTime, nextBkpTime string
if status != nil {
	lastBkpSuccessTime = status.LastBkpSuccessTime
	nextBkpTime = status.NextBkpTime
}
lastRunAt := parseBackupTime(lastBkpSuccessTime, loc)
nextRunAt := parseBackupTime(nextBkpTime, loc)
```

- [ ] **Step 5: Verify `BackupTaskDetail` field names match generated code**

Check that `Size` and `Folders` fields exist in `api.gen.go` with the exact names you're using:

```bash
grep -n "Size\|Folders" internal/backups/api.gen.go
```

If the generated type for `Size` is `*int64`, use `v := target.UsedSize; size = &v` (already shown above). If `Folders` is `*[]string`, wrap accordingly.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/backups/ -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/backups/service.go internal/backups/service_test.go
git commit -m "feat: use status/detail/target APIs for backup task timing, size, folders"
```

---

### Task 8: Update capture script and run full test suite

**Files:**
- Modify: `scripts/dsm-backup-tasks.sh`

- [ ] **Step 1: Update `scripts/dsm-backup-tasks.sh` to capture all three new endpoints**

After the existing `SYNO.Core.TaskScheduler` block, add — using task_id from the first task in the captured response:

```bash
FIRST_TASK_ID=$(jq -r '.data.task_list[0].task_id' scripts/responses/dsm-backup-tasks-raw.json)
echo "--- SYNO.Backup.Task get (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Task&method=get&version=1&task_id=${FIRST_TASK_ID}&additional=%5B%22repository%22%2C%22schedule%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-task-get-raw.json | jq . >&2

echo "--- SYNO.Backup.Task status (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Task&method=status&version=1&task_id=${FIRST_TASK_ID}&blOnline=false&additional=%5B%22last_bkp_time%22%2C%22next_bkp_time%22%2C%22last_bkp_result%22%2C%22is_modified%22%2C%22last_bkp_progress%22%2C%22last_bkp_success_version%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-task-status-raw.json | jq . >&2

echo "--- SYNO.Backup.Target get (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Target&method=get&version=1&task_id=${FIRST_TASK_ID}&additional=%5B%22is_online%22%2C%22used_size%22%2C%22check_task_key%22%2C%22check_auth%22%2C%22account_meta%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-target-get-raw.json | jq . >&2
```

This block goes before the logout line.

- [ ] **Step 2: Run the full test suite**

```bash
make test
```

Expected: all tests PASS, no failures.

- [ ] **Step 3: Run lint**

```bash
make lint
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add scripts/dsm-backup-tasks.sh
git commit -m "chore: extend backup capture script with task detail, status, and target endpoints"
```
