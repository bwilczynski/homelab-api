# API Reorganization: Group/Resource Hierarchy + Domain Refactor

**Date:** 2026-05-07
**Spec commit:** f481090 — "Reorganize API into group/resource hierarchy"

## Context

The spec submodule was updated to restructure the API into a group/resource hierarchy. This requires two categories of work:

1. **Spec-driven changes** — implement the new URL structure, renamed tags, and scope format the spec now declares.
2. **Domain refactor** — apply consistent file-per-resource-group structure across all domain packages, and split backend interfaces by resource group (interface segregation).

## Spec-Driven Changes

### URL restructuring

| Old path | New path |
|----------|----------|
| `/containers/*` | `/docker/containers/*` |
| `/backup/tasks/*` | `/storage/backups/*` |

### Tag changes

| Old tag | New tag | Effect |
|---------|---------|--------|
| `containers` | `docker` | Package rename |
| `backups` (standalone) | `storage` | Absorbed into storage package |

### Scope format

All scopes change from `<access>:<resource>` to `<access>:<group>`:

| Old scope | New scope |
|-----------|-----------|
| `read:containers` | `read:docker` |
| `write:containers` | `write:docker` |
| `read:backup` | `read:storage` |
| `write:backup` | `write:storage` |

`ScopeMiddleware` does pure string comparison — no code change needed. Existing tokens and `config.sample.yaml` documentation must reflect the new scope strings.

### Param rename

`taskId` path parameter renamed to `backupId` in the get-backup endpoint. Handled entirely by regenerated code.

## Package Restructuring

### Renames and deletions

- `internal/containers/` → `internal/docker/` (directory + package rename)
- `internal/backups/` — deleted; code absorbed into `internal/storage/`

### Code generation

The Makefile already reflects the new structure (four codegen configs: system, docker, storage, network). `oapi-codegen-docker.yaml` targets `internal/docker/`, `oapi-codegen-storage.yaml` generates both volumes and backups into `internal/storage/`. The two deleted configs (`oapi-codegen-containers.yaml`, `oapi-codegen-backups.yaml`) are already removed from the working tree.

## Domain File Layout

The consistent pattern across all packages:

- One `Service` struct per package, defined in `service.go`
- Backend interfaces and service methods split into `{resource}_service.go` files
- One `handler.go` per package (thin delegation, unchanged structure)
- `api.gen.go` is always generated — never hand-edited

### `internal/docker/`

Single resource group — no split needed.

```
api.gen.go             regenerated (docker tag, /docker/containers/* routes)
service.go             Service struct, NewService, ContainerBackend interface, container methods
handler.go             unchanged pattern
testdata/              existing fixtures carry over
```

### `internal/storage/`

Volumes and backups merged under one generated interface.

```
api.gen.go             regenerated (storage tag — /storage/volumes/* + /storage/backups/*)
service.go             Service struct, NewService
volumes_service.go     StorageBackend interface, volume methods
backups_service.go     BackupBackend interface, backup methods
handler.go             delegates to svc for both resource groups
testdata/              fixtures from both old storage and old backups packages
```

### `internal/system/`

685-line monolith split by resource group.

```
api.gen.go             unchanged
service.go             Service struct, NewService, shared cache/config state
health_service.go      HealthDSMBackend + HealthUniFiBackend interfaces, health methods
utilization_service.go UtilizationBackend interface, utilization methods
updates_service.go     UpdatesBackend interface, updates methods + GitHub release cache logic
handler.go             unchanged
testdata/              unchanged
```

### `internal/network/`

Devices and clients split.

```
api.gen.go             unchanged
service.go             Service struct, NewService
devices_service.go     DevicesBackend interface, device methods
clients_service.go     ClientsBackend interface, client methods
handler.go             unchanged
testdata/              unchanged
```

## Backend Interface Segregation

Each `*_service.go` defines its own narrow interface containing only the methods that resource group needs. Go's implicit interface satisfaction means the Synology and UniFi adapter types in `internal/adapters/` satisfy multiple small interfaces without modification.

Example for storage:

```go
// volumes_service.go
type StorageBackend interface {
    GetStorageVolumes() (*adapters.DSMStorageVolumeResponse, error)
    GetStorageVolumeDetail(volumeID string) (*adapters.DSMStorageVolumeDetailResponse, error)
}

// backups_service.go
type BackupBackend interface {
    SupportsBackups() bool
    Location() *time.Location
    ListBackupTasks() (*adapters.DSMBackupTaskListResponse, error)
    GetBackupTaskDetail(taskID int) (*adapters.DSMBackupTaskDetailResponse, error)
    GetBackupTaskStatus(taskID int) (*adapters.DSMBackupTaskStatusResponse, error)
    GetBackupTarget(taskID int) (*adapters.DSMBackupTargetResponse, error)
}
```

## Router Wiring (`cmd/server/main.go`)

- Replace `containers` import with `docker`
- Remove the standalone `backups` wiring block
- `storage.NewService` accepts both storage and backup backends (both remain Synology clients)
- `cmd/testserver/main.go` receives the same import/wiring updates

## Error Handling

No changes to error patterns. All handlers continue to return RFC 9457 problem+json responses using the shared `apierrors` constants.

## Testing

- `internal/docker/` — existing container tests carry over (fixtures unchanged, package rename only)
- `internal/storage/` — existing volume tests carry over; backup tests migrated from `internal/backups/`
- `internal/system/` and `internal/network/` — existing tests carry over; only file organization changes, not logic

## Out of Scope

- Adapter code changes (`internal/adapters/`)
- Auth middleware logic
- Config loader
- Any endpoint behavior changes
