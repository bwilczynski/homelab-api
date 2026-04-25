# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go implementation of the Homelab API — a unified surface over heterogeneous homelab backends (UniFi, Synology, Docker, Immich, Hue, Sonos, UPS). The OpenAPI contract lives in a git submodule at `spec/` (source repo: [homelab-api-spec](https://github.com/bwilczynski/homelab-api-spec)). Server stubs are generated from it — never hand-edit `api.gen.go` files.

## Commands

```sh
make generate   # Bundle spec + regenerate server stubs (all domains)
make build      # Build the server binary to bin/server
make run        # Run the server locally on :8080 (loads .env if present)
make test       # Run tests (go test ./...)
make lint       # go vet ./...
make tidy       # go mod tidy
```

Run a single test:

```sh
go test ./internal/containers/ -run TestListContainers
```

### First time after cloning

```sh
git submodule update --init
```

`make generate` automatically bundles the spec submodule before generating stubs.

### Configuration

Copy `config.sample.yaml` to `config.yaml` and fill in backend credentials. Values support `${ENV_VAR}` expansion. Set `CONFIG_FILE` env var to override the default path.

## Architecture

Code is split by API tag into self-contained domain packages. Each domain package contains its own generated server interface, handler, and service.

```
cmd/server/           HTTP server entry point — wires all domain handlers onto a chi router
internal/
  system/             tag: system  (health, info, utilization)
    api.gen.go        Generated StrictServerInterface + models — read-only
    handler.go        Implements StrictServerInterface, calls service layer
    service.go        Business logic
    testdata/         JSON fixtures for tests
  containers/         tag: containers  (list, get, start, stop, restart)
  storage/            tag: storage  (volumes)
  backups/            tag: backups  (backup tasks)
  network/            tag: network  (clients, devices)
  adapters/           Backend clients (Synology, UniFi)
  apierrors/          Shared error sentinels and RFC 9457 problem+json constants
  health/             Background health monitor — probes backends periodically, services skip unreachable ones
  config/             YAML config loader with env-var expansion
```

**Key rules:**
- Business logic belongs in `service.go`, not in handlers.
- Handlers implement the generated `StrictServerInterface` and translate between request/response objects and service calls.
- Errors use RFC 9457 problem+json responses — see `apierrors` for shared constants and `handler.go` files for the pattern.
- Each domain `service.go` defines its own backend interface (e.g. `ContainerBackend`) that adapters satisfy. Services accept an optional `AvailabilityChecker` (the health monitor) to skip unreachable backends.
- Each backend service gets its own file under `internal/adapters/` (`synology.go`, `unifi.go`).
- Adapters handle auth/credential exchange; handlers and service layer never see raw credentials.
- Tests use JSON fixtures in `testdata/` directories, loaded via a generic `loadFixture[T]` helper that extracts the `.data` field from the Synology response envelope.

## Code generation

Each API tag has its own oapi-codegen config (`oapi-codegen-{tag}.yaml`) that generates a chi-server interface, strict-server wrappers, and models into the corresponding domain package. Models are included per-package (no shared models package) since the bundled spec has all `$ref`s resolved inline.

## Spec is read-only

The `spec/` submodule and all `api.gen.go` files are read-only in this repo. Never modify them here. If the spec seems wrong or incomplete, changes must go through the [homelab-api-spec](https://github.com/bwilczynski/homelab-api-spec) repo. This repo only implements the contract.

## Backend adapter rules

These rules apply any time you write or extend adapter code, regardless of how the task was initiated.

**Never fabricate API response structures.** Adapter structs and test fixtures must be derived from actual responses captured from the real backends. If a backend is unreachable, stop and report — do not proceed with assumed or invented response shapes.

**Capturing real responses:** Write minimal bash scripts in `scripts/` that authenticate and call the relevant endpoint (auth patterns: DSM session-based `_sid`, UniFi cookie-based). Save raw output to `scripts/responses/` — this directory is gitignored and must never be committed (raw responses contain real credentials and infrastructure details).

**Building fixtures from captured responses:** Create test fixtures by sanitizing raw responses. Preserve the exact JSON structure — keys, nesting, and types must match the real response. Only values are sanitized:
- Hostnames → `host-01`, `host-02`, …
- IPs → `192.168.1.10`, `192.168.1.11`, …
- MACs → `aa:bb:cc:dd:ee:01`, `aa:bb:cc:dd:ee:02`, …
- Passwords / tokens / session IDs → `REDACTED`
- Container names, software versions, disk models — keep as-is (not sensitive)

Before saving a fixture, verify its top-level key set matches the raw response: `diff <(jq 'keys' raw.json) <(jq '.data | keys' fixture.json)`.

## CI

GitHub Actions workflows:

- **validate.yaml** — runs on push to main and PRs: checks out repo with submodules, bundles the spec, generates stubs, builds, and runs tests.
- **image.yaml** — builds and publishes a Docker image to GHCR.
