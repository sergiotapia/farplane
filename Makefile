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

# Backend quality-gate knobs (local only; agent-loop owns make check/gauntlet).
COVERPROFILE ?= coverage.out
COVERPROFILE_CHANGED ?= coverage.changed.out
# Default base is master (this repo has no main branch).
GIT_BASE ?= master
CHANGED_SINCE ?= $(GIT_BASE)
BACKEND_PKGS = $$($(GO) list ./... | grep -v '/features$$')

.PHONY: help
.PHONY: db-create db-drop db-psql db-psql-test
.PHONY: migrate-up migrate-down migrate-reset migrate-status migrate-version migrate-create
.PHONY: migrate-up-test migrate-reset-test
.PHONY: test test-backend test-web test-web-e2e test-short
.PHONY: lint lint-backend fmt cover-backend govulncheck gitleaks gomutants go-arch-lint acceptance-backend
.PHONY: lint-web format-web typecheck-web knip-web deps-web audit-web mutate-web
.PHONY: check gauntlet
.PHONY: backend web install-web
.PHONY: tidy

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

## test-backend: Run Go tests with race, shuffle, and coverage profile
## (features/ is separate — it resets farplane_test and must not race httpapi tests)
test-backend:
	cd $(BACKEND) && $(GO) test $(BACKEND_PKGS) \
		-race -shuffle=on -count=1 \
		-coverprofile=$(COVERPROFILE) -covermode=atomic

## test-short: Run short Go tests only
test-short:
	cd $(BACKEND) && $(GO) test -short $(BACKEND_PKGS)

## lint: Alias for lint-backend
lint: lint-backend

## lint-backend: Run golangci-lint on the Go control plane
lint-backend:
	cd $(BACKEND) && golangci-lint run ./...

## fmt: Format Go code with gofumpt
fmt:
	cd $(BACKEND) && gofumpt -extra -l -w .

## cover-backend: Patch coverage vs GIT_BASE (substantive diffs; floors in .go-covercheck.yml)
cover-backend: test-backend
	$(BACKEND)/scripts/filter-coverprofile.sh \
		$(BACKEND)/$(COVERPROFILE) $(GIT_BASE) $(BACKEND)/$(COVERPROFILE_CHANGED) $(ROOT)
	@if [ "$$(wc -l < $(BACKEND)/$(COVERPROFILE_CHANGED))" -le 1 ]; then \
		echo "cover-backend: no changed production Go files vs $(GIT_BASE); pass"; \
	else \
		cd $(BACKEND) && go-covercheck $(COVERPROFILE_CHANGED) -c .go-covercheck.yml; \
	fi

## govulncheck: Scan Go modules for known vulnerabilities
govulncheck:
	cd $(BACKEND) && govulncheck ./...

## gitleaks: Scan the repo for leaked secrets
gitleaks:
	gitleaks detect --source $(ROOT) --config $(ROOT)/.gitleaks.toml --verbose

## gomutants: Mutation test lines changed since CHANGED_SINCE
## Scoped to unit packages — DB integration packages race on farplane_test under
## gomutants' coverage collection. Expand GOMUTANTS_PKGS when DB tests are isolated.
GOMUTANTS_PKGS ?= \
	github.com/farplane/farplane/farplane-backend/internal/secretbox \
	github.com/farplane/farplane/farplane-backend/internal/dockerlint \
	github.com/farplane/farplane/farplane-backend/internal/envgen \
	github.com/farplane/farplane/farplane-backend/internal/lanehub \
	github.com/farplane/farplane/farplane-backend/internal/auth \
	github.com/farplane/farplane/farplane-backend/internal/config \
	github.com/farplane/farplane/farplane-backend/internal/agents \
	github.com/farplane/farplane/farplane-backend/internal/githubapp \
	github.com/farplane/farplane/farplane-backend/internal/lanetemplate
gomutants:
	cd $(BACKEND) && gomutants --config .gomutants.yml \
		--changed-since $(CHANGED_SINCE) $(GOMUTANTS_PKGS)

## go-arch-lint: Enforce internal package import boundaries
go-arch-lint:
	cd $(BACKEND) && go-arch-lint check

## acceptance-backend: Run godog feature suite against the test API/DB
acceptance-backend:
	cd $(BACKEND) && $(GO) test ./features -count=1 -race

## test-web: Run SPA unit tests with coverage thresholds
test-web:
	cd $(WEB) && bun run test:coverage

## test-web-e2e: Run Playwright BDD journeys (API must be up; see farplane-web/QUALITY.md)
test-web-e2e:
	cd $(WEB) && bun run test:e2e

## lint-web: Lint and format-check the SPA with Biome
lint-web:
	cd $(WEB) && bun run lint

## format-web: Auto-format the SPA with Biome
format-web:
	cd $(WEB) && bun run format

## typecheck-web: Typecheck the SPA with tsc --noEmit
typecheck-web:
	cd $(WEB) && bun run typecheck

## knip-web: Find unused files, exports, and dependencies
knip-web:
	cd $(WEB) && bun run knip

## deps-web: Check SPA import architecture with dependency-cruiser
deps-web:
	cd $(WEB) && mise exec -- bun run deps

## audit-web: Audit SPA dependencies with bun audit (fails on moderate+)
audit-web:
	cd $(WEB) && bun run audit

## mutate-web: Run StrykerJS mutation tests on SPA lib helpers
mutate-web:
	cd $(WEB) && bun run mutate

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

# check order: format → lint → types → tests → cover → security → arch.
# Uses GIT_BASE/CHANGED_SINCE (default: master). Needs Postgres for Go tests.
## check: Fast/medium local quality pipeline (agent Definition of Done)
check: \
	fmt format-web \
	lint-backend lint-web \
	typecheck-web \
	cover-backend test-web \
	govulncheck gitleaks audit-web \
	go-arch-lint knip-web deps-web

# gauntlet = check + mutation + acceptance + e2e.
# Extra prereqs: Postgres + migrate-up-test; API on :8080 (make backend);
# optional E2E_EMAIL / E2E_PASSWORD; once: bunx playwright install chromium.
# Mutation uses CHANGED_SINCE (default: GIT_BASE/master). See AGENTS.md.
## gauntlet: Full local pipeline (check + mutation + acceptance + e2e)
gauntlet: check gomutants mutate-web acceptance-backend test-web-e2e
