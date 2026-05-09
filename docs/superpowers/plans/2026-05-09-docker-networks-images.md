# Docker Networks & Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `GET /docker/networks`, `GET /docker/networks/{networkId}`, `GET /docker/images`, and `GET /docker/images/{imageId}` backed by real Synology DSM API responses.

**Architecture:** Split the existing monolithic `service.go` into `service.go` (combined `DockerBackend` interface + shared helpers) and `containers_service.go` (existing container logic unchanged), then add `networks_service.go` and `images_service.go` following the same narrow-interface pattern used in the network domain. DSM exposes only `list` for both resources; detail endpoints filter the list server-side.

**Tech Stack:** Go, chi router, oapi-codegen strict server, `SYNO.Docker.Network` and `SYNO.Docker.Image` DSM APIs.

---

### Task 1: Capture real DSM responses

**Files:**
- Create: `scripts/dsm-docker-networks.sh`
- Create: `scripts/dsm-docker-images.sh`

> These scripts must be run against your real NAS before any fixture or test can be written. Output goes to `scripts/responses/` (gitignored). Do not proceed to later tasks without completing this step.

- [ ] **Step 1: Write the networks capture script**

```bash
#!/usr/bin/env bash
# Probe SYNO.Docker.Network API
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../.env"
source "$SCRIPT_DIR/dsm-auth-discover.sh"

BASE="https://${DSM_HOST}/webapi/entry.cgi"
AUTH_BASE="https://${DSM_HOST}/webapi/${AUTH_PATH}"

SID=$(curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')
echo "=== Logged in, SID=${SID:0:8}..."

echo ""
echo "=== SYNO.Docker.Network list ==="
curl -s ${INSECURE_TLS:+-k} \
  "${BASE}?api=SYNO.Docker.Network&method=list&version=1&_sid=${SID}" \
  | tee "$SCRIPT_DIR/responses/docker_network_list.json" | jq .

curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=logout&version=${AUTH_VER}&_sid=${SID}" > /dev/null
echo ""
echo "=== Logged out ==="
```

Save to `scripts/dsm-docker-networks.sh` and `chmod +x scripts/dsm-docker-networks.sh`.

- [ ] **Step 2: Write the images capture script**

```bash
#!/usr/bin/env bash
# Probe SYNO.Docker.Image API
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../.env"
source "$SCRIPT_DIR/dsm-auth-discover.sh"

BASE="https://${DSM_HOST}/webapi/entry.cgi"
AUTH_BASE="https://${DSM_HOST}/webapi/${AUTH_PATH}"

SID=$(curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')
echo "=== Logged in, SID=${SID:0:8}..."

echo ""
echo "=== SYNO.Docker.Image list ==="
curl -s ${INSECURE_TLS:+-k} \
  "${BASE}?api=SYNO.Docker.Image&method=list&version=1&_sid=${SID}" \
  | tee "$SCRIPT_DIR/responses/docker_image_list.json" | jq .

curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=logout&version=${AUTH_VER}&_sid=${SID}" > /dev/null
echo ""
echo "=== Logged out ==="
```

Save to `scripts/dsm-docker-images.sh` and `chmod +x scripts/dsm-docker-images.sh`.

- [ ] **Step 3: Run both scripts**

```bash
bash scripts/dsm-docker-networks.sh
bash scripts/dsm-docker-images.sh
```

Confirm `scripts/responses/docker_network_list.json` and `scripts/responses/docker_image_list.json` exist and contain `"success": true`.

- [ ] **Step 4: Verify response key sets**

```bash
# For networks — note the key inside .data (e.g. "network")
jq '.data | keys' scripts/responses/docker_network_list.json

# For images — note the key inside .data (e.g. "images")
jq '.data | keys' scripts/responses/docker_image_list.json
```

Record the exact top-level keys inside `.data` for both responses — you will need them when writing Go structs in Task 2.

- [ ] **Step 5: Commit the scripts**

```bash
git add scripts/dsm-docker-networks.sh scripts/dsm-docker-images.sh
git commit -m "chore: add DSM docker networks and images capture scripts"
```

---

### Task 2: Add adapter structs and methods

**Files:**
- Modify: `internal/adapters/synology.go` (after the existing container structs, around line 337)

- [ ] **Step 1: Add DSM network structs**

Add after the `DSMContainerResourceResponse` block (around line 505 in `synology.go`):

```go
// DSMDockerNetworkListResponse is the data payload from SYNO.Docker.Network list.
type DSMDockerNetworkListResponse struct {
	Networks []DSMDockerNetworkItem `json:"network"`
}

// DSMDockerNetworkItem represents a Docker network from the DSM API.
// Named DSMDockerNetworkItem to avoid collision with DSMNetwork (container profile network).
type DSMDockerNetworkItem struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Driver     string   `json:"driver"`
	Gateway    string   `json:"gateway"`
	Subnet     string   `json:"subnet"`
	IPRange    string   `json:"iprange"`
	Containers []string `json:"containers"`
}

// DSMDockerImageListResponse is the data payload from SYNO.Docker.Image list.
type DSMDockerImageListResponse struct {
	Images []DSMDockerImageItem `json:"images"`
}

// DSMDockerImageItem represents a Docker image from the DSM API.
type DSMDockerImageItem struct {
	ID          string   `json:"id"`
	Repository  string   `json:"repository"`
	Tags        []string `json:"tags"`
	Size        int64    `json:"size"`
	VirtualSize int64    `json:"virtual_size"`
	Created     int64    `json:"created"`
	Description string   `json:"description"`
}
```

> If `jq '.data | keys'` from Task 1 shows a different key than `"network"` or `"images"`, update the `json:` tags accordingly.

- [ ] **Step 2: Add ListDockerNetworks method**

Add after `RestartContainer` method (around line 710):

```go
// ListDockerNetworks retrieves all Docker networks from the DSM API.
func (c *SynologyClient) ListDockerNetworks() (*DSMDockerNetworkListResponse, error) {
	data, err := c.Call("SYNO.Docker.Network", "list", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMDockerNetworkListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse docker network list: %w", err)
	}
	return &result, nil
}
```

- [ ] **Step 3: Add ListDockerImages method**

```go
// ListDockerImages retrieves all Docker images from the DSM API.
func (c *SynologyClient) ListDockerImages() (*DSMDockerImageListResponse, error) {
	data, err := c.Call("SYNO.Docker.Image", "list", "1", nil)
	if err != nil {
		return nil, err
	}
	var result DSMDockerImageListResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse docker image list: %w", err)
	}
	return &result, nil
}
```

- [ ] **Step 4: Verify it compiles**

```bash
go build ./internal/adapters/...
```

Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/synology.go
git commit -m "feat: add DSM docker network and image adapter structs and methods"
```

---

### Task 3: Regenerate API stubs

**Files:**
- Modify: `internal/docker/api.gen.go` (generated — never hand-edit)

- [ ] **Step 1: Run code generation**

```bash
make generate
```

Expected: exits 0, no errors.

- [ ] **Step 2: Verify new types exist**

```bash
grep -n "DockerNetwork\|DockerImage\|ListDockerNetworks\|GetDockerNetwork\|ListDockerImages\|GetDockerImage" internal/docker/api.gen.go | head -40
```

Confirm you see:
- `type DockerNetwork struct`
- `type DockerNetworkDetail struct`
- `type DockerNetworkList struct`
- `type DockerImage struct`
- `type DockerImageDetail struct`
- `type DockerImageList struct`
- `ListDockerNetworksRequestObject`, `ListDockerNetworks200JSONResponse`
- `GetDockerNetworkRequestObject`, `GetDockerNetwork200JSONResponse`, `GetDockerNetwork404...`, `GetDockerNetwork500...`
- Same patterns for images

- [ ] **Step 3: Check DockerNetworkDetail field names**

```bash
grep -A 30 "^type DockerNetworkDetail struct" internal/docker/api.gen.go
```

Note the exact field names — you will use them in `networks_service.go`. Expected (may vary):
`ConnectedContainers`, `Containers`, `Device`, `Driver`, `Gateway`, `Id`, `IpRange`, `Name`, `Subnet`.

- [ ] **Step 4: Check DockerImageDetail field names**

```bash
grep -A 30 "^type DockerImageDetail struct" internal/docker/api.gen.go
```

Note exact field names. Expected: `Created`, `Description`, `Device`, `Id`, `Repository`, `Size`, `Tags`, `VirtualSize`. Confirm `Created` type is `time.Time`.

- [ ] **Step 5: Verify it still builds**

```bash
go build ./...
```

Expected: fails with "does not implement StrictServerInterface" (the handler is missing 4 methods). That is fine — it confirms generation worked. We address it in Task 9.

- [ ] **Step 6: Commit generated file**

```bash
git add internal/docker/api.gen.go
git commit -m "chore: regenerate docker stubs for networks and images endpoints"
```

---

### Task 4: Refactor service.go → service.go + containers_service.go

**Files:**
- Modify: `internal/docker/service.go` (keep only shared scaffolding)
- Create: `internal/docker/containers_service.go` (all existing container logic)

The goal is to mirror the network domain structure: `service.go` holds the combined interface and `Service` struct; each resource type has its own file.

- [ ] **Step 1: Create containers_service.go**

Create `internal/docker/containers_service.go` with the following content (the container-specific parts extracted from the current `service.go`):

```go
package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ContainersBackend is the narrow interface for container operations.
type ContainersBackend interface {
	SupportsContainers() bool
	ListContainers() (*adapters.DSMContainerListResponse, error)
	GetContainer(name string) (*adapters.DSMContainerDetailResponse, error)
	GetContainerResources() (*adapters.DSMContainerResourceResponse, error)
	StartContainer(name string) error
	StopContainer(name string) error
	RestartContainer(name string) error
}

// ListContainers returns all containers with their resource usage from all backends.
func (s *Service) ListContainers(ctx context.Context, device *string) (ContainerList, error) {
	var items []Container
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsContainers() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}

		containers, err := db.backend.ListContainers()
		if err != nil {
			return ContainerList{}, fmt.Errorf("list containers from %s: %w", db.device, err)
		}

		resources, err := db.backend.GetContainerResources()
		if err != nil {
			return ContainerList{}, fmt.Errorf("get container resources from %s: %w", db.device, err)
		}

		resourceMap := make(map[string]adapters.DSMContainerResource, len(resources.Resources))
		for _, r := range resources.Resources {
			resourceMap[r.Name] = r
		}

		for _, c := range containers.Containers {
			items = append(items, mapContainer(db.device, c, resourceMap[c.Name], 0))
		}
	}
	if items == nil {
		items = []Container{}
	}
	return ContainerList{Items: items}, nil
}

// GetContainer returns a single container by its composite ID (device.name).
func (s *Service) GetContainer(ctx context.Context, containerID string) (*ContainerDetail, error) {
	device, name, err := parseDockerID(containerID)
	if err != nil {
		return nil, err
	}

	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}

	detail, err := backend.GetContainer(name)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}

	resources, err := backend.GetContainerResources()
	if err != nil {
		return nil, fmt.Errorf("get container resources: %w", err)
	}

	var res adapters.DSMContainerResource
	for _, r := range resources.Resources {
		if r.Name == name {
			res = r
			break
		}
	}

	c := mapContainerDetail(device, *detail, res)
	return &c, nil
}

// StartContainer starts a container by its composite ID.
func (s *Service) StartContainer(ctx context.Context, containerID string) error {
	device, name, err := parseDockerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.StartContainer(name)
}

// StopContainer stops a container by its composite ID.
func (s *Service) StopContainer(ctx context.Context, containerID string) error {
	device, name, err := parseDockerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.StopContainer(name)
}

// RestartContainer restarts a container by its composite ID.
func (s *Service) RestartContainer(ctx context.Context, containerID string) error {
	device, name, err := parseDockerID(containerID)
	if err != nil {
		return err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return err
	}
	return backend.RestartContainer(name)
}

func mapRestartPolicy(name string) ContainerDetailRestartPolicy {
	switch ContainerDetailRestartPolicy(name) {
	case Always, No, OnFailure, UnlessStopped:
		return ContainerDetailRestartPolicy(name)
	default:
		return No
	}
}

func mapStatus(state adapters.DSMContainerState) ContainerStatus {
	if state.Dead {
		return Dead
	}
	if state.Restarting {
		return Restarting
	}
	if state.Paused {
		return Paused
	}
	if state.Running {
		return Running
	}
	return Stopped
}

func mapContainer(device string, c adapters.DSMContainer, res adapters.DSMContainerResource, restartCount int) Container {
	return Container{
		Id:           fmt.Sprintf("%s.%s", device, c.Name),
		Device:       device,
		Name:         c.Name,
		Image:        c.Image,
		Status:       mapStatus(c.State),
		RestartCount: restartCount,
		Resources: ContainerResources{
			CpuPercent:    res.CPU,
			MemoryBytes:   res.Memory,
			MemoryPercent: res.MemoryPercent,
		},
	}
}

func mapContainerDetail(device string, d adapters.DSMContainerDetailResponse, res adapters.DSMContainerResource) ContainerDetail {
	startedAt, _ := time.Parse(time.RFC3339Nano, d.Details.State.StartedAt)

	var finishedAt *time.Time
	if !d.Details.State.Running {
		if t, err := time.Parse(time.RFC3339Nano, d.Details.State.FinishedAt); err == nil && !t.IsZero() {
			finishedAt = &t
		}
	}

	envVars := make([]EnvVariable, len(d.Profile.EnvVariables))
	for i, e := range d.Profile.EnvVariables {
		envVars[i] = EnvVariable{Key: e.Key, Value: e.Value}
	}

	networks := make([]ContainerNetwork, len(d.Profile.Networks))
	for i, n := range d.Profile.Networks {
		networks[i] = ContainerNetwork{Name: n.Name, Driver: n.Driver}
	}

	portBindings := make([]PortBinding, len(d.Profile.PortBindings))
	for i, p := range d.Profile.PortBindings {
		portBindings[i] = PortBinding{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			Protocol:      PortBindingProtocol(p.Type),
		}
	}

	volumeBindings := make([]VolumeMount, len(d.Profile.VolumeBindings))
	for i, v := range d.Profile.VolumeBindings {
		mode := Rw
		if v.Type == "ro" {
			mode = Ro
		}
		volumeBindings[i] = VolumeMount{
			Source:      v.HostPath(),
			Destination: v.MountPath,
			Mode:        mode,
		}
	}

	restartPolicy := mapRestartPolicy(d.Details.HostConfig.RestartPolicy.Name)

	labels := d.Details.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	return ContainerDetail{
		Id:             fmt.Sprintf("%s.%s", device, d.Profile.Name),
		Device:         device,
		Name:           d.Profile.Name,
		Image:          d.Profile.Image,
		Status:         mapStatus(d.Details.State),
		RestartCount:   d.Details.RestartCount,
		Resources: ContainerResources{
			CpuPercent:    res.CPU,
			MemoryBytes:   res.Memory,
			MemoryPercent: res.MemoryPercent,
		},
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		ExitCode:       d.Details.State.ExitCode,
		OomKilled:      d.Details.State.OOMKilled,
		RestartPolicy:  restartPolicy,
		Privileged:     d.Profile.Privileged,
		MemoryLimit:    Bytes(d.Profile.MemoryLimit),
		PortBindings:   portBindings,
		Networks:       networks,
		VolumeBindings: volumeBindings,
		EnvVariables:   envVars,
		Entrypoint:     d.Details.Config.Entrypoint,
		Cmd:            d.Details.Config.Cmd,
		Labels:         &labels,
	}
}
```

- [ ] **Step 2: Replace service.go**

Replace the entire contents of `internal/docker/service.go` with:

```go
package docker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// DockerBackend is the combined interface satisfied by the Synology adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type DockerBackend interface {
	ContainersBackend
	NetworksBackend
	ImagesBackend
}

type deviceBackend struct {
	device  string
	backend DockerBackend
}

// Service implements Docker domain business logic.
type Service struct {
	backends []deviceBackend
	monitor  adapters.AvailabilityChecker
}

// NewService creates a new Docker service with one or more backends.
func NewService(backends map[string]DockerBackend, monitor ...adapters.AvailabilityChecker) *Service {
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

func (s *Service) findBackend(device string) (DockerBackend, error) {
	for _, db := range s.backends {
		if db.device == device {
			if !db.backend.SupportsContainers() {
				return nil, fmt.Errorf("device %q does not support docker: %w", device, apierrors.ErrNotFound)
			}
			return db.backend, nil
		}
	}
	return nil, fmt.Errorf("unknown device %q: %w", device, apierrors.ErrNotFound)
}

// parseDockerID splits a composite ID "device.suffix" into its parts.
func parseDockerID(id string) (device, suffix string, err error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid ID %q: expected format device.name: %w", id, apierrors.ErrNotFound)
	}
	return parts[0], parts[1], nil
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/docker/...
```

Expected: fails with missing interface methods (handler not yet updated). That is expected — `NetworksBackend` and `ImagesBackend` are not yet defined, causing the `DockerBackend` composite to fail. We add them in Tasks 7 and 8.

Actually, the build will fail because `NetworksBackend` and `ImagesBackend` don't exist yet. To make it compile temporarily while we work, we can define empty placeholder interfaces. **Do not do this.** Instead, continue straight to Task 5 where the mock is updated — then Tasks 7 and 8 complete the picture. The build will be green after Task 8.

- [ ] **Step 4: Commit**

```bash
git add internal/docker/service.go internal/docker/containers_service.go
git commit -m "refactor: split docker service.go into service.go + containers_service.go"
```

---

### Task 5: Update mock and main.go cast

**Files:**
- Modify: `internal/docker/containers_service_test.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Update mockBackend in containers_service_test.go**

Replace the `mockBackend` struct and its methods with the version below. The struct gains `networksResp`/`networksErr` and `imagesResp`/`imagesErr` fields; two new methods are added to satisfy `DockerBackend`. The map type in `NewService` calls changes from `ContainerBackend` to `DockerBackend`.

Replace the `mockBackend` struct definition and all its methods:

```go
type mockBackend struct {
	listResp      *adapters.DSMContainerListResponse
	detailResp    *adapters.DSMContainerDetailResponse
	resourcesResp *adapters.DSMContainerResourceResponse
	networksResp  *adapters.DSMDockerNetworkListResponse
	imagesResp    *adapters.DSMDockerImageListResponse
	startErr      error
	stopErr       error
	restartErr    error
}

func (m *mockBackend) ListContainers() (*adapters.DSMContainerListResponse, error) {
	return m.listResp, nil
}
func (m *mockBackend) GetContainer(name string) (*adapters.DSMContainerDetailResponse, error) {
	return m.detailResp, nil
}
func (m *mockBackend) GetContainerResources() (*adapters.DSMContainerResourceResponse, error) {
	return m.resourcesResp, nil
}
func (m *mockBackend) SupportsContainers() bool          { return true }
func (m *mockBackend) StartContainer(name string) error   { return m.startErr }
func (m *mockBackend) StopContainer(name string) error    { return m.stopErr }
func (m *mockBackend) RestartContainer(name string) error { return m.restartErr }
func (m *mockBackend) ListDockerNetworks() (*adapters.DSMDockerNetworkListResponse, error) {
	return m.networksResp, nil
}
func (m *mockBackend) ListDockerImages() (*adapters.DSMDockerImageListResponse, error) {
	return m.imagesResp, nil
}
```

Also update every `NewService(map[string]ContainerBackend{...})` call in the file to `NewService(map[string]DockerBackend{...})`. There are multiple — use find-and-replace.

- [ ] **Step 2: Update main.go cast**

In `cmd/server/main.go`, find:

```go
dockerBackends := make(map[string]docker.ContainerBackend, len(synologyClients))
```

Replace with:

```go
dockerBackends := make(map[string]docker.DockerBackend, len(synologyClients))
```

- [ ] **Step 3: Verify containers tests still pass (will fail to compile until Tasks 7+8 define the narrow interfaces)**

We will verify compilation at the end of Task 8. For now, commit what we have.

- [ ] **Step 4: Commit**

```bash
git add internal/docker/containers_service_test.go cmd/server/main.go
git commit -m "refactor: update mock and main.go for DockerBackend composite interface"
```

---

### Task 6: Build test fixtures from real responses

**Files:**
- Create: `internal/docker/testdata/network_list.json`
- Create: `internal/docker/testdata/image_list.json`

- [ ] **Step 1: Inspect raw network response structure**

```bash
jq '.data | keys' scripts/responses/docker_network_list.json
jq '.data.network[0] | keys' scripts/responses/docker_network_list.json
```

Confirm the JSON key for the array matches the `json:"network"` tag in `DSMDockerNetworkListResponse`. If different, update the struct tag in `synology.go`.

- [ ] **Step 2: Inspect raw image response structure**

```bash
jq '.data | keys' scripts/responses/docker_image_list.json
jq '.data.images[0] | keys' scripts/responses/docker_image_list.json
```

Confirm `images` key and field names match `DSMDockerImageItem`. If different, update struct tags.

- [ ] **Step 3: Create testdata/network_list.json**

Sanitize the raw response into a fixture. Keep at least two entries — one `bridge` network with containers and one `host` network (no subnet/gateway) to exercise the optional-field path. Rules:
- Preserve exact JSON structure and all keys
- Keep container names, network names, driver names as-is (not sensitive)
- Replace real IPs with `192.168.1.10`, `192.168.1.11`, …
- Tokens/SIDs → `REDACTED`
- Wrap in the Synology envelope:

```json
{
  "data": {
    "network": [ /* sanitized entries here */ ]
  }
}
```

Verify key set matches raw response:
```bash
diff \
  <(jq '.data | keys' scripts/responses/docker_network_list.json) \
  <(jq '.data | keys' internal/docker/testdata/network_list.json)
```

Expected: no diff output.

- [ ] **Step 4: Create testdata/image_list.json**

Sanitize into at least two image entries. Keep repository names, tags, and drivers. Rules same as above. Wrap:

```json
{
  "data": {
    "images": [ /* sanitized entries here */ ]
  }
}
```

Verify:
```bash
diff \
  <(jq '.data | keys' scripts/responses/docker_image_list.json) \
  <(jq '.data | keys' internal/docker/testdata/image_list.json)
```

Expected: no diff output.

- [ ] **Step 5: Commit fixtures**

```bash
git add internal/docker/testdata/network_list.json internal/docker/testdata/image_list.json
git commit -m "test: add sanitized DSM docker network and image fixtures"
```

---

### Task 7: Implement networks_service.go (TDD)

**Files:**
- Create: `internal/docker/networks_service.go`
- Create: `internal/docker/networks_service_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/docker/networks_service_test.go`. Replace `FIRST_NETWORK_NAME` and `FIRST_NETWORK_CONTAINER_COUNT` with the actual values from your `testdata/network_list.json` fixture (first entry's `name` and `len(containers)`). Replace `HOST_NETWORK_NAME` with the name of the host network entry (the one with empty subnet/gateway).

```go
package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

func TestListNetworks(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")

	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})
	result, err := svc.ListNetworks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != len(resp.Networks) {
		t.Fatalf("expected %d networks, got %d", len(resp.Networks), len(result.Items))
	}

	first := result.Items[0]
	if first.Id != "nas-01."+resp.Networks[0].Name {
		t.Errorf("expected id nas-01.%s, got %s", resp.Networks[0].Name, first.Id)
	}
	if first.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", first.Device)
	}
	if first.Name != resp.Networks[0].Name {
		t.Errorf("expected name %s, got %s", resp.Networks[0].Name, first.Name)
	}
	if first.ConnectedContainers != len(resp.Networks[0].Containers) {
		t.Errorf("expected connectedContainers %d, got %d", len(resp.Networks[0].Containers), first.ConnectedContainers)
	}
}

func TestListNetworksDeviceFilter(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	device := "nas-01"
	result, err := svc.ListNetworks(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != len(resp.Networks) {
		t.Fatalf("expected %d networks for matching device, got %d", len(resp.Networks), len(result.Items))
	}

	other := "nas-02"
	result, err = svc.ListNetworks(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 networks for non-matching device, got %d", len(result.Items))
	}
}

func TestListNetworksEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		networksResp: &adapters.DSMDockerNetworkListResponse{Networks: []adapters.DSMDockerNetworkItem{}},
	}})
	result, err := svc.ListNetworks(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestGetNetwork(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	// Get the first network by composite ID
	id := "nas-01." + resp.Networks[0].Name
	detail, err := svc.GetNetwork(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Id != id {
		t.Errorf("expected id %s, got %s", id, detail.Id)
	}
	if detail.Driver != resp.Networks[0].Driver {
		t.Errorf("expected driver %s, got %s", resp.Networks[0].Driver, detail.Driver)
	}
	if len(detail.Containers) != len(resp.Networks[0].Containers) {
		t.Errorf("expected %d containers, got %d", len(resp.Networks[0].Containers), len(detail.Containers))
	}
}

func TestGetNetworkHostNetworkOptionalFields(t *testing.T) {
	// Host network has empty subnet and gateway — optional fields must be nil
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		networksResp: &adapters.DSMDockerNetworkListResponse{
			Networks: []adapters.DSMDockerNetworkItem{
				{ID: "abc123", Name: "host", Driver: "host", Gateway: "", Subnet: "", IPRange: "", Containers: []string{}},
			},
		},
	}})

	detail, err := svc.GetNetwork(context.Background(), "nas-01.host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Subnet != nil {
		t.Errorf("expected nil Subnet for host network, got %v", detail.Subnet)
	}
	if detail.Gateway != nil {
		t.Errorf("expected nil Gateway for host network, got %v", detail.Gateway)
	}
	if detail.IpRange != nil {
		t.Errorf("expected nil IpRange for host network, got %v", detail.IpRange)
	}
}

func TestGetNetworkNotFound(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerNetworkListResponse](t, "testdata/network_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{networksResp: &resp}})

	_, err := svc.GetNetwork(context.Background(), "nas-01.does_not_exist")
	if err == nil {
		t.Fatal("expected error for unknown network")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetNetworkInvalidID(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{}})

	_, err := svc.GetNetwork(context.Background(), "invalid-no-dot")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails to compile**

```bash
go test ./internal/docker/... 2>&1 | head -20
```

Expected: compile error — `NetworksBackend` undefined, `svc.ListNetworks` undefined.

- [ ] **Step 3: Implement networks_service.go**

Create `internal/docker/networks_service.go`:

```go
package docker

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// NetworksBackend is the narrow interface for Docker network operations.
type NetworksBackend interface {
	ListDockerNetworks() (*adapters.DSMDockerNetworkListResponse, error)
}

// ListNetworks returns all Docker networks from all backends.
func (s *Service) ListNetworks(ctx context.Context, device *string) (DockerNetworkList, error) {
	var items []DockerNetwork
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsContainers() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}
		raw, err := db.backend.ListDockerNetworks()
		if err != nil {
			return DockerNetworkList{}, fmt.Errorf("list docker networks from %s: %w", db.device, err)
		}
		for _, n := range raw.Networks {
			items = append(items, mapDockerNetwork(db.device, n))
		}
	}
	if items == nil {
		items = []DockerNetwork{}
	}
	return DockerNetworkList{Items: items}, nil
}

// GetNetwork returns a single Docker network by composite ID "{device}.{name}".
func (s *Service) GetNetwork(ctx context.Context, networkID string) (*DockerNetworkDetail, error) {
	device, name, err := parseDockerID(networkID)
	if err != nil {
		return nil, err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}
	raw, err := backend.ListDockerNetworks()
	if err != nil {
		return nil, fmt.Errorf("list docker networks: %w", err)
	}
	for _, n := range raw.Networks {
		if n.Name == name {
			detail := mapDockerNetworkDetail(device, n)
			return &detail, nil
		}
	}
	return nil, fmt.Errorf("network %q not found: %w", networkID, apierrors.ErrNotFound)
}

func mapDockerNetwork(device string, n adapters.DSMDockerNetworkItem) DockerNetwork {
	containers := n.Containers
	if containers == nil {
		containers = []string{}
	}
	return DockerNetwork{
		Id:                  fmt.Sprintf("%s.%s", device, n.Name),
		Name:                n.Name,
		Device:              device,
		ConnectedContainers: len(containers),
	}
}

func mapDockerNetworkDetail(device string, n adapters.DSMDockerNetworkItem) DockerNetworkDetail {
	containers := n.Containers
	if containers == nil {
		containers = []string{}
	}
	detail := DockerNetworkDetail{
		Id:                  fmt.Sprintf("%s.%s", device, n.Name),
		Name:                n.Name,
		Device:              device,
		ConnectedContainers: len(containers),
		Driver:              n.Driver,
		Containers:          containers,
	}
	if n.Subnet != "" {
		s := n.Subnet
		detail.Subnet = &s
	}
	if n.Gateway != "" {
		g := n.Gateway
		detail.Gateway = &g
	}
	if n.IPRange != "" {
		r := n.IPRange
		detail.IpRange = &r
	}
	return detail
}

```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/docker/... -run TestListNetworks -v
go test ./internal/docker/... -run TestGetNetwork -v
```

Expected: all pass.

- [ ] **Step 5: Run all docker tests to check no regressions**

```bash
go test ./internal/docker/... -v
```

Expected: all existing container tests still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/docker/networks_service.go internal/docker/networks_service_test.go
git commit -m "feat: implement docker networks service with list and get"
```

---

### Task 8: Implement images_service.go (TDD)

**Files:**
- Create: `internal/docker/images_service.go`
- Create: `internal/docker/images_service_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/docker/images_service_test.go`. Values are derived from `testdata/image_list.json` at runtime via `loadFixture` — no hardcoded fixture values needed for most assertions.

```go
package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

func TestListImages(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")

	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})
	result, err := svc.ListImages(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != len(resp.Images) {
		t.Fatalf("expected %d images, got %d", len(resp.Images), len(result.Items))
	}

	first := result.Items[0]
	wantShortID := strings.TrimPrefix(resp.Images[0].ID, "sha256:")[:12]
	wantID := "nas-01." + wantShortID
	if first.Id != wantID {
		t.Errorf("expected id %s, got %s", wantID, first.Id)
	}
	if first.Device != "nas-01" {
		t.Errorf("expected device nas-01, got %s", first.Device)
	}
	if first.Repository != resp.Images[0].Repository {
		t.Errorf("expected repository %s, got %s", resp.Images[0].Repository, first.Repository)
	}
	if first.Size != resp.Images[0].Size {
		t.Errorf("expected size %d, got %d", resp.Images[0].Size, first.Size)
	}
}

func TestListImagesDeviceFilter(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	device := "nas-01"
	result, err := svc.ListImages(context.Background(), &device)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != len(resp.Images) {
		t.Fatalf("expected %d images for matching device, got %d", len(resp.Images), len(result.Items))
	}

	other := "nas-02"
	result, err = svc.ListImages(context.Background(), &other)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 images for non-matching device, got %d", len(result.Items))
	}
}

func TestListImagesEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		imagesResp: &adapters.DSMDockerImageListResponse{Images: []adapters.DSMDockerImageItem{}},
	}})
	result, err := svc.ListImages(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
}

func TestGetImage(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	shortID := strings.TrimPrefix(resp.Images[0].ID, "sha256:")[:12]
	id := "nas-01." + shortID
	detail, err := svc.GetImage(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Id != id {
		t.Errorf("expected id %s, got %s", id, detail.Id)
	}
	if detail.Repository != resp.Images[0].Repository {
		t.Errorf("expected repository %s, got %s", resp.Images[0].Repository, detail.Repository)
	}
	if detail.VirtualSize != resp.Images[0].VirtualSize {
		t.Errorf("expected virtualSize %d, got %d", resp.Images[0].VirtualSize, detail.VirtualSize)
	}

	// Created: DSM Unix timestamp must be converted to UTC time.Time
	wantCreated := time.Unix(resp.Images[0].Created, 0).UTC()
	if !detail.Created.Equal(wantCreated) {
		t.Errorf("expected created %v, got %v", wantCreated, detail.Created)
	}
}

func TestGetImageDescriptionOmittedWhenEmpty(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{
		imagesResp: &adapters.DSMDockerImageListResponse{
			Images: []adapters.DSMDockerImageItem{
				{ID: "sha256:925ff61909ae1234", Repository: "caddy", Tags: []string{"latest"}, Size: 100, VirtualSize: 200, Created: 1700000000, Description: ""},
			},
		},
	}})

	detail, err := svc.GetImage(context.Background(), "nas-01.925ff61909ae")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Description != nil {
		t.Errorf("expected nil Description for empty string, got %v", detail.Description)
	}
}

func TestGetImageNotFound(t *testing.T) {
	resp := loadFixture[adapters.DSMDockerImageListResponse](t, "testdata/image_list.json")
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{imagesResp: &resp}})

	_, err := svc.GetImage(context.Background(), "nas-01.000000000000")
	if err == nil {
		t.Fatal("expected error for unknown image")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetImageInvalidID(t *testing.T) {
	svc := NewService(map[string]DockerBackend{"nas-01": &mockBackend{}})

	_, err := svc.GetImage(context.Background(), "invalid")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !errors.Is(err, apierrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails to compile**

```bash
go test ./internal/docker/... 2>&1 | head -20
```

Expected: compile error — `ImagesBackend` undefined, `svc.ListImages` undefined.

- [ ] **Step 3: Implement images_service.go**

Create `internal/docker/images_service.go`:

```go
package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ImagesBackend is the narrow interface for Docker image operations.
type ImagesBackend interface {
	ListDockerImages() (*adapters.DSMDockerImageListResponse, error)
}

// ListImages returns all Docker images from all backends.
func (s *Service) ListImages(ctx context.Context, device *string) (DockerImageList, error) {
	var items []DockerImage
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsContainers() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}
		raw, err := db.backend.ListDockerImages()
		if err != nil {
			return DockerImageList{}, fmt.Errorf("list docker images from %s: %w", db.device, err)
		}
		for _, img := range raw.Images {
			items = append(items, mapDockerImage(db.device, img))
		}
	}
	if items == nil {
		items = []DockerImage{}
	}
	return DockerImageList{Items: items}, nil
}

// GetImage returns a single Docker image by composite ID "{device}.{shortId}".
func (s *Service) GetImage(ctx context.Context, imageID string) (*DockerImageDetail, error) {
	device, shortID, err := parseDockerID(imageID)
	if err != nil {
		return nil, err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}
	raw, err := backend.ListDockerImages()
	if err != nil {
		return nil, fmt.Errorf("list docker images: %w", err)
	}
	for _, img := range raw.Images {
		if strings.HasPrefix(img.ID, "sha256:"+shortID) {
			detail := mapDockerImageDetail(device, img)
			return &detail, nil
		}
	}
	return nil, fmt.Errorf("image %q not found: %w", imageID, apierrors.ErrNotFound)
}

func imageShortID(dsmID string) string {
	id := strings.TrimPrefix(dsmID, "sha256:")
	if len(id) >= 12 {
		return id[:12]
	}
	return id
}

func mapDockerImage(device string, img adapters.DSMDockerImageItem) DockerImage {
	tags := img.Tags
	if tags == nil {
		tags = []string{}
	}
	return DockerImage{
		Id:         fmt.Sprintf("%s.%s", device, imageShortID(img.ID)),
		Device:     device,
		Repository: img.Repository,
		Tags:       tags,
		Size:       img.Size,
	}
}

func mapDockerImageDetail(device string, img adapters.DSMDockerImageItem) DockerImageDetail {
	tags := img.Tags
	if tags == nil {
		tags = []string{}
	}
	detail := DockerImageDetail{
		Id:          fmt.Sprintf("%s.%s", device, imageShortID(img.ID)),
		Device:      device,
		Repository:  img.Repository,
		Tags:        tags,
		Size:        img.Size,
		Created:     time.Unix(img.Created, 0).UTC(),
		VirtualSize: img.VirtualSize,
	}
	if img.Description != "" {
		d := img.Description
		detail.Description = &d
	}
	return detail
}
```

> **Field name check:** `DockerImageDetail.Created` must be `time.Time`. `DockerImageDetail.VirtualSize` must match exactly the field name from `api.gen.go` (checked in Task 3 Step 4). `DockerImage.Size` is `int64` (Bytes is a type alias for int64, no cast needed). Adjust field names if `grep` in Task 3 showed different names.

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/docker/... -run TestListImages -v
go test ./internal/docker/... -run TestGetImage -v
```

Expected: all pass.

- [ ] **Step 5: Run full docker test suite**

```bash
go test ./internal/docker/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/docker/images_service.go internal/docker/images_service_test.go
git commit -m "feat: implement docker images service with list and get"
```

---

### Task 9: Extend handler.go

**Files:**
- Modify: `internal/docker/handler.go`

The `ServerHandler` must now implement four additional methods from `StrictServerInterface`. Add them to the bottom of `handler.go`.

- [ ] **Step 1: Add the four handler methods**

Append to `internal/docker/handler.go`:

```go
func (h *ServerHandler) ListDockerNetworks(ctx context.Context, request ListDockerNetworksRequestObject) (ListDockerNetworksResponseObject, error) {
	result, err := h.svc.ListNetworks(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListDockerNetworks500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListDockerNetworks200JSONResponse(result), nil
}

func (h *ServerHandler) GetDockerNetwork(ctx context.Context, request GetDockerNetworkRequestObject) (GetDockerNetworkResponseObject, error) {
	result, err := h.svc.GetNetwork(ctx, request.NetworkId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetDockerNetwork404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetDockerNetwork500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetDockerNetwork200JSONResponse(*result), nil
}

func (h *ServerHandler) ListDockerImages(ctx context.Context, request ListDockerImagesRequestObject) (ListDockerImagesResponseObject, error) {
	result, err := h.svc.ListImages(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListDockerImages500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListDockerImages200JSONResponse(result), nil
}

func (h *ServerHandler) GetDockerImage(ctx context.Context, request GetDockerImageRequestObject) (GetDockerImageResponseObject, error) {
	result, err := h.svc.GetImage(ctx, request.ImageId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetDockerImage404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetDockerImage500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetDockerImage200JSONResponse(*result), nil
}
```

> **Exact type names** are derived from the generated `api.gen.go`. If the generated names differ from what is shown (e.g. `GetDockerNetwork400ApplicationProblemPlusJSONResponse` exists), add `400` handling for bad composite IDs — but this is typically handled by the strict-server wrapper before reaching your handler. Cross-check with the `GetContainer*` types in `api.gen.go` for the exact pattern.

- [ ] **Step 2: Verify the full build**

```bash
go build ./...
```

Expected: exits 0 with no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/docker/handler.go
git commit -m "feat: add handler methods for docker networks and images endpoints"
```

---

### Task 10: Final verification

- [ ] **Step 1: Run full test suite with race detector**

```bash
go test -race ./...
```

Expected: all pass, no data races.

- [ ] **Step 2: Run lint**

```bash
make lint
```

Expected: no issues.

- [ ] **Step 3: Build production binary**

```bash
make build
```

Expected: exits 0, `bin/server` created.

- [ ] **Step 4: Smoke test (optional but recommended)**

```bash
make run &
sleep 2
curl -s http://localhost:8080/docker/networks | jq .
curl -s http://localhost:8080/docker/images | jq .
kill %1
```

Expected: JSON responses with `items` array (may be empty if running without real config).
