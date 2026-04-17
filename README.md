# homelab-api

Go implementation of the Homelab API — a unified surface over heterogeneous homelab backends (UniFi, Synology, Docker, Immich, Hue, Sonos, UPS).

The API contract is defined in [homelab-api-spec](https://github.com/bwilczynski/homelab-api-spec) and pulled in as a git submodule. Server stubs are generated with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

## Getting started

```sh
git clone --recurse-submodules https://github.com/bwilczynski/homelab-api.git
cd homelab-api
make generate
make run
```

The server starts on `http://localhost:8080`.

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
  adapters/           Backend clients (UniFi, Synology, Immich, Hue, Sonos, …)
```

Each domain package contains:
- `api.gen.go` — generated `StrictServerInterface` and models (do not edit)
- `handler.go` — implements the generated interface
- `service.go` — business logic

## Code generation

Each API tag has its own oapi-codegen config (`oapi-codegen-{tag}.yaml`). Running `make generate` bundles the spec submodule and regenerates all domain packages. Generated files are gitignored.
