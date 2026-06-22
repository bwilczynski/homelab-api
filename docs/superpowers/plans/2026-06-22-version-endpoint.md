# Version Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `GET /version` endpoint that returns the API spec version and server binary version as a static JSON response, with no authentication required.

**Architecture:** Two package-level variables (`apiVersion`, `serverVersion`) in `cmd/server/main.go` are set to `"dev"` by default and overridden at build time via `-ldflags`. The handler is registered directly on the unprotected chi router before the protected route group, matching the existing `/.well-known/homelab` pattern.

**Tech Stack:** Go standard library (`encoding/json`, `net/http`), chi router, GNU Make.

---

### Task 1: Inject version variables and register the endpoint

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Add package-level version variables to `main.go`**

  After the `package main` declaration and before the `import` block, add:

  ```go
  // Injected at build time via -ldflags; defaults to "dev" for local runs.
  var (
  	apiVersion    = "dev"
  	serverVersion = "dev"
  )
  ```

  Place the `var` block after the `import` block (Go requires `import` before top-level declarations). The top of `main.go` currently ends the import block with a `)`; add the `var` block immediately after it:

  ```go
  // ... existing imports ...
  )

  // Injected at build time via -ldflags; defaults to "dev" for local runs.
  var (
  	apiVersion    = "dev"
  	serverVersion = "dev"
  )

  const shutdownTimeout = 10 * time.Second
  ```

- [ ] **Step 2: Register `GET /version` on the unprotected router**

  In `main.go`, find the existing `/.well-known/homelab` handler (around line 92). Immediately after it, add the `/version` handler:

  ```go
  r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
  	w.Header().Set("Content-Type", "application/json")
  	type response struct {
  		APIVersion    string `json:"apiVersion"`
  		ServerVersion string `json:"serverVersion"`
  	}
  	_ = json.NewEncoder(w).Encode(response{
  		APIVersion:    apiVersion,
  		ServerVersion: serverVersion,
  	})
  })
  ```

- [ ] **Step 3: Build and verify the endpoint responds**

  ```bash
  make build && ./bin/server &
  sleep 1
  curl -s http://localhost:8080/version
  ```

  Expected output (both fields `"dev"` since no ldflags injected):

  ```json
  {"apiVersion":"dev","serverVersion":"dev"}
  ```

  Kill the server: `kill %1`

- [ ] **Step 4: Update `make build` in the Makefile to inject versions via ldflags**

  Replace the current `build` target:

  ```makefile
  build: ## Build the server binary
  	go build -o $(BINARY) ./cmd/server
  ```

  With:

  ```makefile
  API_VERSION    := $(shell grep '^  version:' spec/openapi/openapi.yaml | awk '{print $$2}')
  SERVER_VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)

  build: ## Build the server binary
  	go build -ldflags "-X main.apiVersion=$(API_VERSION) -X main.serverVersion=$(SERVER_VERSION)" -o $(BINARY) ./cmd/server
  ```

  Place the two `API_VERSION` / `SERVER_VERSION` variable lines near the top of the Makefile, after the existing variable block (`SPEC_REPO`, `BINARY`, etc.).

- [ ] **Step 5: Build with ldflags and verify injected values**

  ```bash
  make build && ./bin/server &
  sleep 1
  curl -s http://localhost:8080/version
  ```

  Expected output (exact `serverVersion` will vary by your local git state):

  ```json
  {"apiVersion":"0.1.0","serverVersion":"9c03bcf"}
  ```

  `apiVersion` must be `"0.1.0"` (from the spec). `serverVersion` will be the most recent git tag or short SHA.

  Kill the server: `kill %1`

- [ ] **Step 6: Verify existing tests still pass**

  ```bash
  make test
  ```

  Expected: all tests pass, no compilation errors.

- [ ] **Step 7: Commit**

  ```bash
  git add cmd/server/main.go Makefile
  git commit -m "feat: add GET /version endpoint with build-time version injection"
  ```
