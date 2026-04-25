# homelab-api

Go implementation of the Homelab API — a unified surface over heterogeneous homelab backends (UniFi, Synology, Docker, Immich, Hue, Sonos, UPS).

The API contract is defined in [homelab-api-spec](https://github.com/bwilczynski/homelab-api-spec) and pulled in as a git submodule. Server stubs are generated with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

## Getting started

```sh
git clone --recurse-submodules https://github.com/bwilczynski/homelab-api.git
cd homelab-api
cp config.sample.yaml config.yaml  # fill in backend credentials
make generate
make run
```

The server starts on `http://localhost:8080`.

## Configuration

Backend connections are defined in `config.yaml`. Values support environment variable expansion (`${VAR_NAME}`). Set `CONFIG_FILE` to use a different path.

```yaml
backends:
  - name: nas-01
    type: synology
    host: 192.168.1.10:5001
    username: admin
    password: ${NAS01_PASS}

  - name: unifi
    type: unifi
    host: 192.168.1.1
    username: admin
    password: ${UNIFI_PASS}
```

Supported backend types: `synology`, `unifi`.

## Commands

| Command | Description |
|---------|-------------|
| `make generate` | Bundle spec + regenerate server stubs |
| `make build` | Build the server binary to `bin/server` |
| `make run` | Run the server locally on `:8080` |
| `make test` | Run tests |
| `make lint` | Run `go vet` |

## Project structure

Code is split by API tag into self-contained domain packages:

```
cmd/server/           Entry point — wires domain handlers onto a chi router
internal/
  system/             Health, system info, utilization
  containers/         Container lifecycle (list, get, start, stop, restart)
  storage/            Storage volumes
  backups/            Backup tasks
  network/            Network clients and devices
  adapters/           Backend clients (Synology, UniFi)
```

Each domain package contains:
- `api.gen.go` — generated `StrictServerInterface` and models (do not edit)
- `handler.go` — implements the generated interface
- `service.go` — business logic

## Docker

A multi-stage Dockerfile builds the server into a minimal [distroless](https://github.com/GoogleContainerTools/distroless) image.

```sh
docker build -t homelab-api .
docker run -p 8080:8080 homelab-api
```

A pre-built image is published to GHCR on every push to `main`:

```sh
docker pull ghcr.io/bwilczynski/homelab-api:latest
```

## Code generation

Each API tag has its own oapi-codegen config (`oapi-codegen-{tag}.yaml`). Running `make generate` bundles the spec submodule and regenerates all domain packages. Generated files are gitignored.
