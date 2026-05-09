# Docker Networks & Images — Implementation Design

## Overview

Implement four new endpoints from the updated spec (`spec/` submodule commit `1bd1b54`):

- `GET /docker/networks` — list Docker networks across all backends
- `GET /docker/networks/{networkId}` — get one network by `{device}.{name}`
- `GET /docker/images` — list Docker images across all backends
- `GET /docker/images/{imageId}` — get one image by `{device}.{shortId}`

Both resources are backed exclusively by Synology (DSM) backends via `SYNO.Docker.Network list` and `SYNO.Docker.Image list`. DSM exposes no `get` method for either — detail endpoints are fulfilled server-side by filtering the list.

---

## Architecture

Follow the network domain pattern: `service.go` owns the combined backend interface and shared helpers; each resource type gets its own `*_service.go` with a narrow interface, business logic, and mapping functions.

### File layout after this change

```
internal/docker/
  api.gen.go              (regenerated — read-only)
  service.go              DockerBackend combined interface + Service + NewService + findBackend + parseDockerID
  containers_service.go   ContainersBackend (narrow) + all existing container logic (moved from service.go)
  networks_service.go     NetworksBackend (narrow) + ListNetworks + GetNetwork + mapping
  images_service.go       ImagesBackend (narrow) + ListImages + GetImage + mapping
  handler.go              ServerHandler — extended with 4 new methods
  containers_service_test.go  existing container tests (unchanged)
  networks_service_test.go  network list/get tests
  images_service_test.go    image list/get tests
  testdata/
    container_list.json     (existing)
    container_detail.json   (existing)
    container_resources.json (existing)
    network_list.json       (new — sanitized DSM fixture)
    image_list.json         (new — sanitized DSM fixture)
```

---

## service.go (refactored)

`ContainerBackend` is renamed to `DockerBackend`, composing three narrow interfaces:

```go
type DockerBackend interface {
    ContainersBackend
    NetworksBackend
    ImagesBackend
}
```

`Service` struct, `NewService`, `findBackend`, and `parseDockerID` live here. The `SupportsContainers()` capability check gates all three resource types — if Docker is not enabled on a NAS, networks and images are also unavailable.

`parseDockerID` replaces the existing `parseContainerID`; same logic (`strings.SplitN(id, ".", 2)`), generalised name.

`main.go` cast site changes from `docker.ContainerBackend` to `docker.DockerBackend`.

---

## containers_service.go

Current `service.go` body moves here verbatim. `ContainerBackend` becomes `ContainersBackend`:

```go
type ContainersBackend interface {
    SupportsContainers() bool
    ListContainers() (*adapters.DSMContainerListResponse, error)
    GetContainer(name string) (*adapters.DSMContainerDetailResponse, error)
    GetContainerResources() (*adapters.DSMContainerResourceResponse, error)
    StartContainer(name string) error
    StopContainer(name string) error
    RestartContainer(name string) error
}
```

All existing container methods (`ListContainers`, `GetContainer`, `StartContainer`, `StopContainer`, `RestartContainer`) and their mapping helpers stay here with no logic changes.

---

## networks_service.go

### Narrow interface

```go
type NetworksBackend interface {
    ListDockerNetworks() (*adapters.DSMDockerNetworkListResponse, error)
}
```

### Business logic

**`ListNetworks(ctx, device *string) (DockerNetworkList, error)`**
- Iterates `s.backends`; skips when `device` filter doesn't match, `!SupportsContainers()`, or monitor marks unavailable.
- Calls `ListDockerNetworks()`, maps each entry to `DockerNetwork`.
- `connectedContainers = len(entry.Containers)`.
- Returns empty slice (never nil) when no results.

**`GetNetwork(ctx, networkId string) (*DockerNetworkDetail, error)`**
- Calls `parseDockerID`; returns `ErrNotFound`-wrapped error on bad format.
- Calls `findBackend(device)`.
- Calls `ListDockerNetworks()` on that backend; scans for `entry.Name == name`.
- Returns `ErrNotFound` if no match.
- Maps to `DockerNetworkDetail`; omits `subnet`/`gateway`/`ipRange` when empty string.

### Mapping

`DockerNetwork` list item: `Id = "{device}.{name}"`, `ConnectedContainers = len(containers)`.

`DockerNetworkDetail` extends list item with `Driver`, `Containers []string`, and optional `Subnet`/`Gateway`/`IPRange` (omitted when DSM returns empty string).

---

## images_service.go

### Narrow interface

```go
type ImagesBackend interface {
    ListDockerImages() (*adapters.DSMDockerImageListResponse, error)
}
```

### Business logic

**`ListImages(ctx, device *string) (DockerImageList, error)`**
- Same backend iteration pattern as `ListNetworks`.
- `shortId` = first 12 characters of the digest after stripping the `sha256:` prefix from DSM's `id` field.
- `Id = "{device}.{shortId}"`.

**`GetImage(ctx, imageId string) (*DockerImageDetail, error)`**
- Parses `{device}.{shortId}` via `parseDockerID`.
- Calls `ListDockerImages()`; matches when `strings.HasPrefix(entry.ID, "sha256:"+shortId)`.
- Returns `ErrNotFound` if no match.
- `Created`: DSM `created` is a Unix timestamp (int64) → `time.Unix(created, 0).UTC()`.
- `Description`: omitted when empty string.

### Mapping

`DockerImage` list item: `Id`, `Device`, `Repository`, `Tags`, `Size`.

`DockerImageDetail` extends with `Created time.Time`, `VirtualSize`, and optional `Description`.

---

## Adapter layer (adapters/synology.go)

Two new response struct pairs (names avoid collision with the existing `DSMNetwork` used in container profile networks):

```go
type DSMDockerNetworkListResponse struct {
    Networks []DSMDockerNetworkItem `json:"network"`
}
type DSMDockerNetworkItem struct {
    ID         string   `json:"id"`
    Name       string   `json:"name"`
    Driver     string   `json:"driver"`
    Gateway    string   `json:"gateway"`
    Subnet     string   `json:"subnet"`
    IPRange    string   `json:"iprange"`
    Containers []string `json:"containers"`
}

type DSMDockerImageListResponse struct {
    Images []DSMDockerImageItem `json:"images"`
}
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

Two new methods on `SynologyClient`:

```go
func (c *SynologyClient) ListDockerNetworks() (*DSMDockerNetworkListResponse, error)
// calls SYNO.Docker.Network / list / version 1

func (c *SynologyClient) ListDockerImages() (*DSMDockerImageListResponse, error)
// calls SYNO.Docker.Image / list / version 1
```

---

## Handler (handler.go)

Four new methods on `ServerHandler`, following existing error-handling pattern:

| Handler method       | Service call        | 404 on                        |
|----------------------|---------------------|-------------------------------|
| `ListDockerNetworks` | `svc.ListNetworks`  | —                             |
| `GetDockerNetwork`   | `svc.GetNetwork`    | `ErrNotFound` or bad ID       |
| `ListDockerImages`   | `svc.ListImages`    | —                             |
| `GetDockerImage`     | `svc.GetImage`      | `ErrNotFound` or bad ID       |

No `anyOf`/discriminator schemas — standard `200JSONResponse` types, no hand-written wrapper needed.

---

## Code generation

Before implementing handlers, run `make generate` after updating the submodule pointer. This regenerates `internal/docker/api.gen.go` with the four new `StrictServerInterface` methods, request/response types, and chi routes.

---

## Tests

### Fixtures

Fixtures must be built from real DSM responses per CLAUDE.md — do not fabricate structures. Before writing any fixture:

1. Write `scripts/fetch_docker_networks.sh` — authenticate with `_sid` and call `SYNO.Docker.Network list`. Save raw output to `scripts/responses/docker_network_list.json`.
2. Write `scripts/fetch_docker_images.sh` — same auth, call `SYNO.Docker.Image list`. Save raw output to `scripts/responses/docker_image_list.json`.
3. Verify top-level key sets: `diff <(jq 'keys' raw.json) <(jq '.data | keys' fixture.json)`.
4. Sanitize into `testdata/network_list.json` and `testdata/image_list.json` — preserve exact JSON structure, sanitize only values (IPs → `192.168.1.x`, hostnames → `host-01`, tokens → `REDACTED`; container names, image names, drivers keep as-is).

Fixtures wrap responses in the Synology envelope format used by `loadFixture`:

```json
{ "data": { /* raw DSM response payload */ } }
```

The `scripts/responses/` directory is gitignored and must never be committed.

### Test coverage

**`networks_service_test.go`**
- `TestListNetworks` — maps correctly, `connectedContainers` count, IDs formed as `{device}.{name}`
- `TestListNetworksDeviceFilter` — matching device returns results; non-matching returns empty
- `TestGetNetwork` — fields mapped, optional fields omitted for host network (empty subnet/gateway)
- `TestGetNetworkNotFound` — unknown ID returns `ErrNotFound`
- `TestGetNetworkInvalidID` — malformed ID returns error

**`images_service_test.go`**
- `TestListImages` — IDs use first-12 shortId, `Created` converts Unix → UTC, empty description omitted
- `TestListImagesDeviceFilter`
- `TestGetImage` — full detail fields, Unix → `time.Time` conversion verified
- `TestGetImageNotFound`
- `TestGetImageInvalidID`

Mock: The existing `mockBackend` in `containers_service_test.go` currently implements `ContainerBackend` (soon `ContainersBackend`). After the refactor it must also implement `NetworksBackend` and `ImagesBackend` to satisfy `DockerBackend`. Add stub implementations of `ListDockerNetworks` and `ListDockerImages` to the existing `mockBackend` — no need for a separate mock type. Network and image test files reuse the same mock.
