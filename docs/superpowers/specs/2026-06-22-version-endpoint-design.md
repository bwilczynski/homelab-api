# Version Endpoint Design

**Date:** 2026-06-22  
**Status:** Approved

## Problem

Clients need a way to discover what version of the API server they are talking to before making domain requests. This lets clients detect incompatible deployments early rather than failing mid-operation.

## Decision

Add a `GET /version` endpoint outside the OpenAPI contract, following the same pattern as the existing `/.well-known/homelab` discovery endpoint. Neither DigitalOcean nor Microsoft Azure puts a version discovery endpoint inside their OpenAPI specs — it is infrastructure-level metadata, not domain API surface.

## Endpoint

```
GET /version
```

- **Auth:** None. Registered on the unprotected chi router, before the `protected` router group.
- **Path:** `/version` (root-level, not under `/api`)

### Response

```json
{
  "apiVersion": "0.1.0",
  "serverVersion": "v0.2.1"
}
```

| Field | Source | Description |
|---|---|---|
| `apiVersion` | `-ldflags "-X main.apiVersion=…"` | OpenAPI contract version from `spec/openapi/openapi.yaml` `info.version`. Extracted at build time. |
| `serverVersion` | `-ldflags "-X main.serverVersion=…"` | Git tag of the running binary (`git describe --tags --always`). Falls back to `"dev"` when unset. |

## Build Wiring

`make build` gains two `-ldflags` injections:

```makefile
API_VERSION := $(shell grep '^  version:' spec/openapi/openapi.yaml | awk '{print $$2}')
SERVER_VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)

LDFLAGS := -X main.apiVersion=$(API_VERSION) -X main.serverVersion=$(SERVER_VERSION)
```

`make run` (local dev) does not inject these; both variables default to `"dev"`.

## Implementation

All changes are in `cmd/server/main.go`:

1. Declare two package-level `var` strings (`apiVersion`, `serverVersion`) with default `"dev"`.
2. Register `GET /version` on the root `r` router (before `protected`) using an inline handler that writes the JSON response.

No new packages, no generated types, no tests needed (static response, no backend calls).

## Out of Scope

- No compatibility negotiation (client does not send its version; server does not return a `compatible` flag).
- No spec changes to `homelab-api-spec`.
