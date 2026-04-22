# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go implementation of the Homelab API — a unified surface over heterogeneous homelab backends (UniFi, Synology, Docker, Immich, Hue, Sonos, UPS). The OpenAPI contract lives in a git submodule at `spec/` (source repo: [homelab-api-spec](https://github.com/bwilczynski/homelab-api-spec)). Server stubs are generated from it — never hand-edit `api.gen.go` files.

## Commands

```sh
make generate   # Bundle spec + regenerate server stubs (all domains)
make build      # Build the server binary to bin/server
make run        # Run the server locally on :8080
make test       # Run tests (go test ./...)
make lint       # go vet ./...
make tidy       # go mod tidy
```

### First time after cloning

```sh
git submodule update --init
```

`make generate` automatically bundles the spec submodule before generating stubs.

## Architecture

Code is split by API tag into self-contained domain packages. Each domain package contains its own generated server interface, handler, and service.

```
cmd/server/           HTTP server entry point — wires all domain handlers onto a chi router
internal/
  system/             tag: system  (health, info, utilization)
    api.gen.go        Generated StrictServerInterface + models — read-only
    handler.go        Implements StrictServerInterface, calls service layer
    service.go        Business logic
  containers/         tag: containers  (list, get, start, stop, restart)
    api.gen.go
    handler.go
    service.go
  storage/            tag: storage  (volumes)
    api.gen.go
    handler.go
    service.go
  backups/            tag: backups  (backup tasks)
    api.gen.go
    handler.go
    service.go
  adapters/           Backend clients (UniFi, Synology, Immich, Hue, Sonos, etc.)
```

**Key rules:**
- Business logic belongs in `service.go`, not in handlers.
- Handlers implement the generated `StrictServerInterface` and translate between request/response objects and service calls.
- Each backend service gets its own file under `internal/adapters/`.
- Adapters handle auth/credential exchange; handlers and service layer never see raw credentials.

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

GitHub Actions workflow (`.github/workflows/validate.yaml`) runs on push to main and PRs:
1. Checks out repo with submodules
2. Bundles the spec (`make -C spec bundle`)
3. Generates server stubs (`make generate`)
4. Builds (`make build`)
5. Runs tests (`make test`)
