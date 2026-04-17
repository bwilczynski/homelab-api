# Domain → Backend Mapping

## Authentication Patterns

### Synology DSM

```
# Login — returns session ID
GET https://{DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=login&version=6&account={user}&passwd={pass}&format=sid

# Response: { "success": true, "data": { "sid": "SESSION_ID" } }

# API call — pass _sid as query param
GET https://{DSM_HOST}/webapi/entry.cgi?api={API_NAME}&method={METHOD}&version={VERSION}&_sid={SID}&{extra_params}

# Response: { "success": true, "data": { ... } }

# Logout
GET https://{DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6&_sid={SID}
```

Env vars: `DSM_HOST`, `DSM_USER`, `DSM_PASS`

### UniFi Controller

```
# Login — sets session cookie
POST https://{UNIFI_HOST}/api/login
Body: { "username": "{user}", "password": "{pass}" }
# Store cookies from response

# API call — send cookies
GET https://{UNIFI_HOST}/api/s/default/{endpoint}
# Response: { "meta": { "rc": "ok" }, "data": [...] }

# Logout
POST https://{UNIFI_HOST}/api/logout
```

Env vars: `UNIFI_HOST`, `UNIFI_USER`, `UNIFI_PASS`

Note: Both use self-signed certs — skip TLS verification.

### API Discovery (DSM)

To find additional DSM APIs beyond what is documented here:
```
GET /webapi/entry.cgi?api=SYNO.API.Info&method=query&version=1&query=all&_sid={SID}
```
Returns all available API names with their maxVersion and paths.

---

## system

**Backends:** DSM + UniFi

### DSM APIs

**SYNO.Core.System** method=`info` version=1
- Returns: model, firmware_ver, up_time ("H:M:S"), ram_size (MB)
- Maps to: `SystemInfo` model

**SYNO.Core.System.Utilization** method=`get` version=1
- Returns:
  - `cpu`: `{ user_load, system_load }` (percentages)
  - `memory`: `{ memory_size, real_usage, avail_real, total_swap, used_swap }` (KB)
  - `network[]`: `{ device, rx, tx }` (bytes/sec)
  - `disk.disk[]`: `{ device, read_access, write_access }` (ops/sec)
- Maps to: `SystemUtilization` model (CpuUsage, MemoryUsage, NetworkInterfaceUsage, DiskIo)

### UniFi APIs

**GET /api/s/default/stat/health**
- Returns array of subsystem health objects:
  - `{ subsystem: "wan"|"lan"|"wlan", status: "ok"|..., wan_ip, latency, xput_down, xput_up, num_adopted, num_sta }`
- Maps to: `Health` model with `ComponentHealth` entries

### Interface methods

- `GetSystemHealth` → UniFi health subsystems → `Health` with components
- `ListSystemInfo` → DSM system info → `SystemInfoList` (one entry per device)
- `ListSystemUtilization` → DSM utilization → `SystemUtilizationList`

---

## containers

**Backend:** DSM

### DSM APIs

**SYNO.Docker.Container** method=`list` version=1 params=`limit=0&offset=0`
- Returns: `{ containers: [{ name, status, image }] }`
- Maps to: `ListContainers` → list of `Container` models

**SYNO.Docker.Container** method=`get` version=1 params=`name={container_name}`
- Returns: `{ details: { Name, status, up_time, RestartCount, Config: { Image } } }`
- Maps to: `GetContainer` → single `Container` model

**SYNO.Docker.Container.Resource** method=`get` version=1
- Returns: `{ resources: [{ name, cpu, memory, memoryPercent }] }`
- Use to enrich container list with resource usage

**SYNO.Docker.Container** method=`start`/`stop`/`restart` version=1 params=`name={container_name}`
- Maps to: `StartContainer`, `StopContainer`, `RestartContainer`
- These are action endpoints (POST), check DSM API for exact method names

### Interface methods

- `ListContainers` → container list + resource data → `ContainerList`
- `GetContainer` → container detail + resources → `Container`
- `StartContainer` / `StopContainer` / `RestartContainer` → container actions

---

## storage

**Backend:** DSM

### DSM APIs

**SYNO.Storage.CGI.Storage** method=`load_info` version=1
- Returns:
  - `volumes[]`: `{ id, fs_type, status, size: { total, used } }` (bytes)
  - `disks[]`: `{ id, model, size_total, status, temp }` (bytes, Celsius)
- Maps to: `StorageVolume` models

### Interface methods

- `ListStorageVolumes` → volumes + associated disks → `StorageVolumeList`
- `GetStorageVolume` → single volume by ID → `StorageVolume`

---

## backups

**Backend:** DSM

### DSM APIs

**SYNO.Core.TaskScheduler** method=`list` version=2 params=`offset=0&limit=50`
- Returns: `{ tasks: [{ name, enable, next_trigger_time, action }] }`

**SYNO.Backup.Task** method=`list` version=1
- Returns: `{ task_list: [{ task_id, name, state }] }`

**SYNO.SDS.Backup.Client.Common.Log** method=`list` version=1 params=`task_id={id}&offset=0&limit=100`
- Returns: `{ log_list: [{ event, time, task_id }] }`

### Interface methods

- `ListBackupTasks` → scheduled tasks + backup tasks → `BackupTaskList`
- `GetBackupTask` → single backup task with logs → `BackupTask`
