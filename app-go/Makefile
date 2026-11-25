# app-go development tasks. Run `make help` for the list.
#
# If Go is not on your PATH: make GO=/usr/local/go/bin/go GOFMT=/usr/local/go/bin/gofmt <target>

GO      ?= go
GOFMT   ?= gofmt
COMPOSE ?= docker compose

# The database `make db-up` starts; DB integration tests create their own schemas in it.
TMOCK_DB_PORT           ?= 5432
TMOCK_TEST_DATABASE_URL ?= postgres://tmock:tmock@localhost:$(TMOCK_DB_PORT)/tmock?sslmode=disable

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

## ── Go ──────────────────────────────────────────────────────────────────────

.PHONY: run
run: build ## Migrate, seed (ADMIN_* from .env) and serve on :3000 (PORT overrides)
	bin/tmock migrate
	bin/tmock seed
	bin/tmock serve

.PHONY: build
build: ## Build bin/tmock
	$(GO) build -o bin/tmock ./cmd/tmock

.PHONY: migrate
migrate: build ## Apply pending database migrations
	bin/tmock migrate

.PHONY: seed
seed: build ## Create the first admin from ADMIN_* if there are no users
	bin/tmock seed

.PHONY: test
test: ## Run unit and handler tests (DB tests are skipped)
	$(GO) test ./...

.PHONY: test-db
test-db: ## Run all tests, including DB integration tests (needs make db-up)
	TMOCK_TEST_DATABASE_URL='$(TMOCK_TEST_DATABASE_URL)' $(GO) test -count=1 ./...

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
