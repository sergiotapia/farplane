# Farplane root Makefile
#
# Common targets for the Go control plane, Postgres, and migrations.

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND := $(ROOT)/farplane-backend
WEB := $(ROOT)/farplane-web

DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5432/farplane_dev?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable
APP_ENV ?= local
export DATABASE_URL
export TEST_DATABASE_URL
export APP_ENV

PGHOST ?= 127.0.0.1
PGPORT ?= 5432
PGUSER ?= postgres
PGPASSWORD ?= postgres
export PGPASSWORD

GO ?= go
PSQL ?= psql

.PHONY: help
.PHONY: db-create db-drop db-psql db-psql-test
.PHONY: migrate-up migrate-down migrate-reset migrate-status migrate-version migrate-create
.PHONY: migrate-up-test migrate-reset-test
.PHONY: test test-backend test-web test-short
.PHONY: backend web install-web
.PHONY: tidy fmt

## help: Show this help
help:
	@echo "Usage: make [target]"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## db-create: Create farplane_dev and farplane_test if missing
db-create:
	@$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 -tc \
		"SELECT 1 FROM pg_database WHERE datname = 'farplane_dev'" | grep -q 1 || \
		$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 \
			-c "CREATE DATABASE farplane_dev"
	@$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 -tc \
		"SELECT 1 FROM pg_database WHERE datname = 'farplane_test'" | grep -q 1 || \
		$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 \
			-c "CREATE DATABASE farplane_test"
	@echo "Databases ready: farplane_dev, farplane_test"

## db-drop: Drop farplane_dev and farplane_test (destructive)
db-drop:
	@$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 \
		-c "DROP DATABASE IF EXISTS farplane_dev WITH (FORCE)"
	@$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d postgres -v ON_ERROR_STOP=1 \
		-c "DROP DATABASE IF EXISTS farplane_test WITH (FORCE)"
	@echo "Dropped farplane_dev and farplane_test"

## db-psql: Open psql on farplane_dev
db-psql:
	$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d farplane_dev

## db-psql-test: Open psql on farplane_test
db-psql-test:
	$(PSQL) -h $(PGHOST) -p $(PGPORT) -U $(PGUSER) -d farplane_test

## migrate-up: Apply all pending migrations to farplane_dev
migrate-up:
	cd $(BACKEND) && $(GO) run ./cmd/migrate up

## migrate-down: Roll back one migration on farplane_dev
migrate-down:
	cd $(BACKEND) && $(GO) run ./cmd/migrate down

## migrate-reset: Roll back all migrations on farplane_dev
migrate-reset:
	cd $(BACKEND) && $(GO) run ./cmd/migrate reset

## migrate-status: Show migration status for farplane_dev
migrate-status:
	cd $(BACKEND) && $(GO) run ./cmd/migrate status

## migrate-version: Show current schema version for farplane_dev
migrate-version:
	cd $(BACKEND) && $(GO) run ./cmd/migrate version

## migrate-up-test: Apply migrations to farplane_test
migrate-up-test:
	cd $(BACKEND) && DATABASE_URL=$(TEST_DATABASE_URL) $(GO) run ./cmd/migrate up

## migrate-reset-test: Reset migrations on farplane_test
migrate-reset-test:
	cd $(BACKEND) && DATABASE_URL=$(TEST_DATABASE_URL) $(GO) run ./cmd/migrate reset

## migrate-create: Create a new SQL migration (NAME=add_users)
migrate-create:
ifndef NAME
	$(error NAME is required. Example: make migrate-create NAME=add_users)
endif
	cd $(BACKEND) && $(GO) run ./cmd/migrate create $(NAME)

## test: Run backend tests
test: test-backend

## test-backend: Run Go tests
test-backend:
	cd $(BACKEND) && $(GO) test ./...

## test-short: Run short Go tests only
test-short:
	cd $(BACKEND) && $(GO) test -short ./...

## test-web: Run SPA unit tests (when Vitest is wired)
test-web:
	cd $(WEB) && bun run test

## backend: Run the Go control plane
backend:
	cd $(BACKEND) && $(GO) run ./cmd/farplane

## web: Run the SPA dev server
web:
	cd $(WEB) && bun run dev

## install-web: Install SPA dependencies
install-web:
	cd $(WEB) && bun install

## tidy: Tidy Go modules
tidy:
	cd $(BACKEND) && $(GO) mod tidy

## fmt: Format Go code
fmt:
	cd $(BACKEND) && $(GO) fmt ./...
