# app-go development tasks. Run `make help` for the list.
#
# If Go is not on your PATH: make GO=/usr/local/go/bin/go GOFMT=/usr/local/go/bin/gofmt <target>

GO      ?= go
GOFMT   ?= gofmt
COMPOSE ?= docker compose

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

## ── Go ──────────────────────────────────────────────────────────────────────

.PHONY: run
run: ## Run the server on :3000 (PORT overrides)
	$(GO) run ./cmd/tmock

.PHONY: build
build: ## Build bin/tmock
	$(GO) build -o bin/tmock ./cmd/tmock

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format all Go files
	$(GOFMT) -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go file is not gofmt-formatted
	@out="$$($(GOFMT) -l .)"; if [ -n "$$out" ]; then echo "Not formatted:"; echo "$$out"; exit 1; fi

.PHONY: check
check: fmt-check vet test ## fmt-check + vet + test (the Definition of Done basics)

.PHONY: vendor-js
vendor-js: ## Re-download vendored JS and verify it against static/vendor/VERSIONS
	$(GO) run ./dev/vendorjs

## ── Local services (Docker) ─────────────────────────────────────────────────

.PHONY: db-up
db-up: ## Start PostgreSQL and wait until it is ready
	$(COMPOSE) up -d --wait postgres

.PHONY: db-down
db-down: ## Stop all local services (data is kept)
	$(COMPOSE) --profile otel down

.PHONY: db-reset
db-reset: ## Stop all local services and delete the database volume
	$(COMPOSE) --profile otel down -v

.PHONY: db-psql
db-psql: ## Open psql in the PostgreSQL container
	$(COMPOSE) exec postgres psql -U tmock -d tmock

.PHONY: otel-up
otel-up: ## Start the local OpenTelemetry Collector on :4318
	$(COMPOSE) --profile otel up -d otel-collector

.PHONY: otel-logs
otel-logs: ## Follow what the collector receives
	$(COMPOSE) --profile otel logs -f otel-collector
