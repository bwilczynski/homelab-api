SPEC_REPO   := spec
SPEC_FILE   := $(SPEC_REPO)/dist/openapi.bundled.yaml
BINARY      := bin/server
TESTSERVER  := bin/testserver
OAPI_CODEGEN := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

.PHONY: help build run generate bundle lint test tidy build-testserver contract-test

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the server binary
	go build -o $(BINARY) ./cmd/server

run: ## Run the server locally (loads .env if present)
	$(if $(wildcard .env),set -a && . ./.env && set +a &&) go run ./cmd/server

generate: bundle ## Generate server stubs from the bundled spec
	@mkdir -p internal/system internal/docker internal/storage internal/network
	$(OAPI_CODEGEN) --config oapi-codegen-system.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-docker.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-storage.yaml $(SPEC_FILE)
	$(OAPI_CODEGEN) --config oapi-codegen-network.yaml $(SPEC_FILE)

bundle: ## Bundle the OpenAPI spec from the submodule
	$(MAKE) -C $(SPEC_REPO) bundle

lint: ## Run go vet and staticcheck
	go vet ./...

test: ## Run tests
	go test ./...

tidy: ## Tidy go.mod
	go mod tidy

build-testserver: generate ## Build the fixture-backed test server
	go build -o $(TESTSERVER) ./cmd/testserver

contract-test: build-testserver ## Run contract tests (Schemathesis vs test server)
	@$(TESTSERVER) & TSPID=$$!; \
	trap "kill $$TSPID 2>/dev/null" EXIT; \
	READY=0; for i in 1 2 3 4 5; do curl -sf http://localhost:8081/system/health >/dev/null && READY=1 && break || sleep 1; done; \
	[ $$READY -eq 1 ] || { echo "test server failed to start"; exit 1; }; \
	schemathesis run $(SPEC_FILE) --url http://localhost:8081 --checks all --exclude-checks unsupported_method
