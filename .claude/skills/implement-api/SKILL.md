---
name: implement-api
description: |
  Implement a homelab API domain end-to-end: adapter, service, handler, tests.
  Creates a branch and opens a PR.
  Invoke: /implement-api <domain> (e.g., containers, storage, system, backups)
disable-model-invocation: true
user-invocable: true
allowed-tools: Bash Read Write Edit Glob Grep
argument-hint: <domain>
context: fork
---

# Implement API Domain

Implement the `$ARGUMENTS` domain for homelab-api. This creates the full backend: adapter, service, handler, and tests — then opens a PR.

## Pre-flight

Read the project's `CLAUDE.md` for architecture rules and conventions.

## Step 1: Validate arguments

`$ARGUMENTS` must be one of: `system`, `containers`, `storage`, `backups`. If invalid, report the error and stop.

Set `DOMAIN=$ARGUMENTS` for the rest of this workflow.

## Step 2: Create branch

```bash
git checkout -b feature/${DOMAIN}
```

## Step 3: Read the generated interface

Read `internal/${DOMAIN}/api.gen.go` to understand:
- The `StrictServerInterface` methods you need to implement
- Request/response object types (e.g., `ListContainersRequestObject`, `ListContainers200JSONResponse`)
- Generated model structs (e.g., `Container`, `Health`)

This file is read-only — never edit it.

## Step 4: Consult domain mapping

Read `${CLAUDE_SKILL_DIR}/domains.md` for:
- Which backend(s) serve this domain (DSM, UniFi, or both)
- Which API endpoints to call
- How endpoints map to StrictServerInterface methods

## Step 5: Explore backend APIs

Using the API details from `domains.md`, write scripts in `scripts/` within this project to call the real backend APIs and capture responses:
- Write minimal bash scripts that authenticate and call the relevant endpoints (follow auth patterns from `domains.md`)
- Output JSON only — no text/table formatting
- If you need to discover additional APIs beyond what `domains.md` covers, use the DSM discover endpoint (`SYNO.API.Info` query=all) or UniFi API exploration
- Credentials are in env vars: `DSM_HOST`, `DSM_USER`, `DSM_PASS`, `UNIFI_HOST`, `UNIFI_USER`, `UNIFI_PASS` (source from `.env` — copy `env.sample` if it doesn't exist yet)

Run the scripts to capture real API responses. These require network access to the homelab.

## Step 6: Sanitize responses

Create sanitized versions of the captured responses for use as test fixtures. Replace:
- Hostnames → `host-01`, `host-02`
- IPs → `192.168.1.10`, `192.168.1.11`, ...
- MACs → `aa:bb:cc:dd:ee:01`, `aa:bb:cc:dd:ee:02`
- Container names → keep as-is (service names, not sensitive)
- Disk models → use real vendor names with different variants (e.g., "WD Red Plus 4TB", "Seagate IronWolf 8TB")
- Software versions → keep as-is (public info)

Save sanitized fixtures as `internal/${DOMAIN}/testdata/*.json`.

## Step 7: Implement adapter

Create the adapter in `internal/adapters/`:
- One file per backend: `synology.go`, `unifi.go`, etc.
- Follow the auth patterns from `domains.md`:
  - **DSM:** Session-based auth — POST to `SYNO.API.Auth` login, get `_sid`, pass as query param
  - **UniFi:** Cookie-based auth — POST to `/api/login`, store cookie jar
- Credentials from env vars: `DSM_HOST`, `DSM_USER`, `DSM_PASS`, `UNIFI_HOST`, `UNIFI_USER`, `UNIFI_PASS`
- Use `net/http` client with JSON parsing
- Define response structs that match the real API response shapes
- If an adapter file already exists (from another domain's implementation), extend it rather than duplicating

## Step 8: Define interface and implement service

In `internal/${DOMAIN}/service.go`:
- Define the adapter interface that the service needs (consumer-defined, Go idiom)
- Accept the interface via `NewService(backend BackendInterface)` constructor
- Map backend response structs → generated API model structs
- Keep business logic here (aggregation, filtering, transformation)

In `internal/${DOMAIN}/handler.go`:
- Each StrictServerInterface method calls the corresponding service method
- Wrap the result in the typed response: e.g., `ListContainers200JSONResponse(result)`
- Handle errors appropriately

## Step 9: Write tests

Create `internal/${DOMAIN}/service_test.go`:
- Load sanitized JSON fixtures from `testdata/`
- Create a mock implementation of the adapter interface
- Test the mapping from backend response → API model
- Test edge cases (empty lists, missing fields)
- Use standard `testing` package (no external test frameworks)

## Step 10: Wire, verify, and PR

1. **Wire up** — Update `cmd/server/main.go`:
   - Create adapter instance with config from env vars
   - Pass adapter to `NewService(adapter)`
   - The rest of the wiring (handler, strict handler, router) stays the same

2. **Verify**:
   ```bash
   make build && make test && make lint
   ```
   Fix any issues before proceeding.

3. **Commit and PR**:
   ```bash
   git add -A
   git commit -m "Implement ${DOMAIN} API with adapter, service, and tests"
   git push -u origin feature/${DOMAIN}
   gh pr create --title "Implement ${DOMAIN} API" --body "$(cat <<'EOF'
   ## Summary
   - Implemented ${DOMAIN} domain with real backend adapter
   - Added service layer mapping backend responses to API models
   - Added tests with sanitized response fixtures

   ## Test plan
   - [ ] `make build` passes
   - [ ] `make test` passes
   - [ ] `make lint` passes
   - [ ] Manual test against live backend with `make run`
   EOF
   )"
   ```

## Important rules

- Never edit `api.gen.go` files — they are generated
- Never edit files in `spec/` — it's a read-only submodule
- Business logic belongs in `service.go`, not handlers
- Handlers only translate between HTTP request/response objects and service calls
- Adapters handle all authentication — service layer never sees raw credentials
- Keep adapter interfaces minimal — only the methods this domain actually needs
