# Backup Detail: Size, Folders, and Timezone Fix

**Date:** 2026-05-02

## Summary

Two changes in scope:

1. **Spec update** — `BackupTaskDetail` gains two optional fields: `size` (int64 bytes) and `folders` (string array). Implementation pulls these from the DSM UI API calls (`SYNO.Backup.Task get`, `SYNO.Backup.Task status`, `SYNO.Backup.Target get`).
2. **Timezone fix** — DSM backup timestamps have no TZ offset. They are in the NAS local timezone, not UTC. Parsing them with `time.Parse` produces wrong UTC values. Fix: treat them as the configured timezone (or server local if unconfigured).

---

## DSM API Changes

The current implementation uses `ListBackupLogs` (log parsing) to derive `lastRunAt` and `lastResult`, and `ListScheduledTasks` (task scheduler list) to derive `nextRunAt`. These are replaced with three direct DSM UI API calls, matching exactly what the Hyper Backup UI uses.

### req1 — `SYNO.Backup.Task get` with `additional=["repository","schedule"]`
- **Used for**: source `folders` (`source.folder_list[].fullPath`) and schedule info
- **Replaces**: nothing currently (new data)

### req2 — `SYNO.Backup.Task status` with `additional=["last_bkp_time","next_bkp_time","last_bkp_result","is_modified","last_bkp_progress","last_bkp_success_version"]`
- **Used for**: `lastRunAt` (from `last_bkp_success_time`), `nextRunAt` (from `next_bkp_time`), `lastResult` (from `last_bkp_result` + `last_bkp_error_code`)
- **Replaces**: `ListBackupLogs` (for `lastRunAt`/`lastResult`) and `ListScheduledTasks` (for `nextRunAt`)

### req3 — `SYNO.Backup.Target get` with `additional=["is_online","used_size","check_task_key","check_auth","account_meta"]`
- **Used for**: `size` (from `used_size`)
- **Replaces**: nothing currently (new data)

---

## `lastResult` Mapping

From `SYNO.Backup.Task status` response:

| `last_bkp_result` | `last_bkp_error_code` | API result  |
|-------------------|-----------------------|-------------|
| `"done"`          | `0`                   | `success`   |
| `"done"`          | non-zero              | `warning`   |
| `"error"`         | any                   | `failed`    |
| `"skip"`          | any                   | `skipped`   |
| anything else     | any                   | `unknown`   |

This replaces log-based warning detection. Verified: task 3 returns `last_bkp_result: "done"`, `last_bkp_error_code: 4401` → `warning`, matching the existing test expectation.

---

## Timestamp Parsing

All DSM backup timestamps are in the NAS local timezone with no UTC offset. Since logs and scheduler are replaced by the status API, only one format is in use:
- Status format: `"2006/01/02 15:04"` (e.g. `"2026/05/02 02:54"`)

Fix: replace `time.Parse` with `time.ParseInLocation(layout, s, loc)` using a configured location. `parseLogTime` and `parseSchedulerTime` are deleted; replaced by `parseBackupTime`.

**Timezone source (priority order):**
1. `timezone` field on the Synology backend config entry (IANA name, e.g. `Europe/Warsaw`)
2. Server local time (`time.Local`) if not specified

Config example:
```yaml
backends:
  - name: nas-01
    type: synology
    host: 192.168.1.10:5001
    username: admin
    password: ${NAS01_PASS}
    timezone: Europe/Warsaw   # optional; defaults to server local TZ
```

---

## Adapter Changes (`internal/adapters/synology.go`)

### New structs

```
DSMBackupTaskDetailResponse   — response for SYNO.Backup.Task get (fields: task_id, name, state, source.folder_list[].fullPath, schedule, ...)
DSMBackupTaskStatusResponse   — response for SYNO.Backup.Task status (fields: task_id, state, last_bkp_result, last_bkp_error_code, last_bkp_success_time, next_bkp_time, ...)
DSMBackupTargetResponse       — response for SYNO.Backup.Target get (fields: used_size, is_online, ...)
DSMBackupFolderEntry          — item within source.folder_list (fields: fullPath, folderPath, ...)
```

### New methods on `SynologyClient`

```
GetBackupTaskDetail(taskID int) (*DSMBackupTaskDetailResponse, error)
GetBackupTaskStatus(taskID int) (*DSMBackupTaskStatusResponse, error)
GetBackupTarget(taskID int) (*DSMBackupTargetResponse, error)
```

### Removed methods (no longer called)

`ListScheduledTasks` — was used only for `nextRunAt`; replaced by `next_bkp_time` in status. Remove from `BackupBackend` interface and `service.go`. Keep the method on `SynologyClient` for now (non-breaking), just remove from the interface.

### Timezone field

`SynologyClient` gains a `loc *time.Location` field, set from config. All `parseSchedulerTime`, `parseLogTime` equivalents use `time.ParseInLocation(..., loc)`.

---

## Service Changes (`internal/backups/service.go`)

### `BackupBackend` interface

Remove `ListScheduledTasks`. Add three new methods:
```go
GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error)
GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error)
GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error)
```

`ListBackupLogs` is also removed from the interface (no longer needed for result/timing); keep the method on `SynologyClient` but drop from `BackupBackend`.

### `GetBackupTask` (detail endpoint)

Replace current flow (logs + scheduler) with:
1. Call `GetBackupTaskDetail` → extract `folders`
2. Call `GetBackupTaskStatus` → extract `lastRunAt`, `nextRunAt`, `lastResult`
3. Call `GetBackupTarget` → extract `size`

### `ListBackupTasks` (list endpoint)

Per task: replace `ListBackupLogs` call with `GetBackupTaskStatus` for `lastResult`.

### Helpers removed

- `findNextRunAt` — replaced by `next_bkp_time` from status
- `findLastCompletion` — replaced by status-based result mapping
- `parseSchedulerTime`, `parseLogTime` — replaced by a single `parseBackupTime(s, loc)` function that handles both formats (`"2006/01/02 15:04"`, `"2006/01/02 15:04:05"`, `"2006-01-02 15:04"`)

### New helpers

```go
parseBackupTime(s string, loc *time.Location) *time.Time  // DSM status format "2006/01/02 15:04", parsed in configured location
mapBackupResult(result string, errorCode int) BackupTaskResult
```

---

## Config Changes (`internal/config/config.go`)

Add `Timezone string` to `BackendConfig`. When wiring backends in `cmd/server/`, load the location:
```go
loc := time.Local
if cfg.Timezone != "" {
    loc, err = time.LoadLocation(cfg.Timezone)
    // error → fatal
}
```
Pass `loc` when creating `SynologyClient`.

---

## Script Update (`scripts/dsm-backup-tasks.sh`)

Add all three new API calls after the existing task list call. Save raw responses to:
- `scripts/responses/dsm-backup-task-get-raw.json`
- `scripts/responses/dsm-backup-task-status-raw.json`
- `scripts/responses/dsm-backup-target-get-raw.json`

---

## Fixtures

Three new fixture files (sanitized from captured responses):
- `internal/backups/testdata/backup_task_detail.json`
- `internal/backups/testdata/backup_task_status.json`
- `internal/backups/testdata/backup_target.json`

Top-level key set of each fixture verified against raw response.

---

## `api.gen.go` Regeneration

Pull new spec commit into submodule, then run `make generate`. `BackupTaskDetail` will gain:
- `Size *int64 \`json:"size,omitempty"\``
- `Folders []string \`json:"folders,omitempty"\``

Do not edit `api.gen.go` manually.

---

## Test Changes (`internal/backups/service_test.go`)

- Update `mockBackupBackend` to add the three new interface methods and remove `ListScheduledTasks`/`ListBackupLogs`
- Add `TestGetBackupTask` assertions for `Size` and `Folders`
- Add `TestParseBackupTime` covering all three formats and timezone shift (verify a known DSM local time parses to expected UTC)
- Add `TestMapBackupResult` for all four result states
- Update `TestFindLastCompletion` → remove (helper deleted); add `TestMapBackupResult` instead
- Update `TestFindNextRunAt` → remove (helper deleted)
