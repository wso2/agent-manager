.PHONY: help tools setup setup-colima up-core down-core reset-core up-observability smoke smoke-core smoke-observability dev-up-observability setup-console-local setup-console-local-force up down logs reset dev-up dev-down dev-restart dev-rebuild dev-logs dev-migrate thunder-restart thunder-reset teardown db-connect service-logs service-shell console-logs console stop-console gen-instrumentation-contract check-contract-drift e2e-test security-test security-test-static security-test-live

# Default target
help:
	@echo "Agent Manager Platform - Development Commands"
	@echo ""
	@echo "The stack runs with Docker Compose from deployments/standalone (no Kubernetes)."
	@echo ""
	@echo "🚀 Setup (run once):"
	@echo "  make tools                   - Install pinned Go dev tools into .tools/bin (see tools.mk)"
	@echo "  make setup                   - Complete setup (tools + dev stack + console)"
	@echo "  make setup-colima            - Start Colima VM (macOS Docker host)"
	@echo "  make setup-console-local     - Install console deps (only if changed)"
	@echo "  make setup-console-local-force - Force reinstall console deps"
	@echo ""
	@echo "📦 Full stack from images (console served by the API on :9000):"
	@echo "  make up                      - Build images and start the whole stack"
	@echo "  make down                    - Stop the stack (data kept)"
	@echo "  make logs                    - Tail all stack logs"
	@echo "  make reset                   - Stop the stack and delete all its data"
	@echo "  make up-core                 - The same with the amp-core binary (no AI Gateway)"
	@echo "  make down-core / reset-core  - Stop / delete the amp-core stack"
	@echo "  make up-observability        - The full stack with ClickHouse and the OTel collector (traces)"
	@echo "  make smoke / smoke-core      - Smoke-test the running amp / amp-core stack"
	@echo "  make smoke-observability     - Smoke-test traces on a stack from up-observability"
	@echo "  DB=postgres                  - With up/dev-up/reset (and -core): use PostgreSQL instead of SQLite"
	@echo ""
	@echo "💻 Daily Development (API hot-reloads under air, console via vite):"
	@echo "  make dev-up                  - Start the stack and the console dev server"
	@echo "  make dev-up-observability    - The same with ClickHouse and the OTel collector (traces)"
	@echo "  make dev-down                - Stop the stack and the console"
	@echo "  make dev-restart             - Restart the stack and the console"
	@echo "  make dev-rebuild             - Rebuild images and restart"
	@echo "  make dev-logs                - Tail stack logs"
	@echo "  make console                 - Start (or restart) the console dev server on :3000 (BACKGROUND=true to detach)"
	@echo "  make stop-console            - Stop the console dev server"
	@echo "  make dev-migrate             - Run database migrations (air also migrates on every start)"
	@echo ""
	@echo "⚡ ThunderID:"
	@echo "  make thunder-restart    - Restart ThunderID"
	@echo "  make thunder-reset      - Wipe ThunderID data and bootstrap it again"
	@echo ""
	@echo "🗄️  Database:"
	@echo "  make db-connect         - Open the database (SQLite, or PostgreSQL with DB=postgres)"
	@echo ""
	@echo "🔧 Service Debugging:"
	@echo "  make service-logs       - View API logs"
	@echo "  make service-shell      - Shell into the API container"
	@echo "  make console-logs       - Tail console dev server logs"
	@echo ""
	@echo "amctl CLI:"
	@echo "  make amctl-build             - Build amctl for current platform"
	@echo "  make amctl-release-dry-run   - Cross-compile all targets without publishing"
	@echo "  make amctl-test              - Run amctl tests"
	@echo "  make amctl-coverage-check    - Run amctl tests and report coverage against cli/coverage-gate.json"
	@echo "🧪 E2E Tests:"
	@echo "  make e2e-test           - Run E2E tests (stack must be running)"
	@echo ""
	@echo "🔒 Security Tests:"
	@echo "  make security-test         - Run all security tests (static + live)"
	@echo "  make security-test-static  - Route-table authz invariants (no stack needed)"
	@echo "  make security-test-live    - Security suites against a running deployment"
	@echo ""
	@echo "🧹 Cleanup:"
	@echo "  make teardown           - Stop everything and delete all stack data"
	@echo ""

# Pinned Go-written dev tools (codegen, linters, test runners).
# Included after help so help stays the default goal.
include tools.mk

# Docker Compose stack (see deployments/standalone/README.md). The dev overlay
# swaps the API image for a hot-reloading source build and leaves the console
# to `make console`.
STACK_DIR   := deployments/standalone
# DB=postgres runs the postgres profile and points the API at it (SQLite otherwise).
COMPOSE     := docker compose --project-directory $(STACK_DIR) -f $(STACK_DIR)/docker-compose.yml -f $(STACK_DIR)/docker-compose.build.yml
ifeq ($(DB),postgres)
export COMPOSE_PROFILES := postgres
COMPOSE     += -f $(STACK_DIR)/docker-compose.postgres.yml
endif
DEV_COMPOSE := $(COMPOSE) -f $(STACK_DIR)/docker-compose.dev.yml
# amp-core: the core binary without modules (no AI Gateway), as its own
# Compose project so its data never mixes with the full stack's. It uses the
# same ports, so run one of the two at a time.
CORE_COMPOSE := $(COMPOSE) -f $(STACK_DIR)/docker-compose.core.yml -p amp-lite-core
# The observability profile: ClickHouse and the OTel collector, with amp-api
# pointed at them for this run (docs/amp-lite/observability.md).
OBS_ENV     := AMP_OBSERVABILITY_STORE_URL=http://clickhouse:8123
OBS_PROFILE := --profile observability $(if $(filter postgres,$(DB)),--profile postgres)
OBS_COMPOSE := $(OBS_ENV) $(COMPOSE) $(OBS_PROFILE)
DEV_OBS_COMPOSE := $(OBS_ENV) $(DEV_COMPOSE) $(OBS_PROFILE)
# Installs the observability module once; one an operator disabled stays
# disabled. $(1) is the compose command, $(2) the amp CLI in amp-api.
define install-observability
	@if $(1) exec -T amp-api $(2) module list | grep -Eq '^observability +[^ ]+ +available'; then \
		$(1) exec -T amp-api $(2) module install observability; fi
endef
# The stop targets name every profile, so they also stop PostgreSQL from a
# DB=postgres run and ClickHouse from up-observability.
ALL_PROFILES := --profile postgres --profile observability --profile observability-external

# Complete setup
setup: tools
	@$(MAKE) dev-up
	@echo ""
	@echo "Run 'make dev-down' to stop the stack and the console"

# Full stack from images
up:
	@echo "🚀 Building images and starting AMP..."
	@$(COMPOSE) up -d --build --wait --remove-orphans
	@echo "✅ AMP is running"
	@echo "   Console: http://localhost:9000 (sign in as admin; password: $(COMPOSE) exec amp-api amp secret show admin-password)"
	@echo "   API:     http://localhost:9000/api/v1"

down:
	@$(COMPOSE) $(ALL_PROFILES) down

logs:
	@$(COMPOSE) logs -f

reset:
	@$(COMPOSE) $(ALL_PROFILES) down -v

up-core:
	@echo "🚀 Building the amp-core image and starting AMP core (no AI Gateway)..."
	@$(CORE_COMPOSE) up -d --build --wait --remove-orphans
	@echo "✅ AMP core is running"
	@echo "   Console: http://localhost:9000 (sign in as admin; password: $(CORE_COMPOSE) exec amp-api amp secret show admin-password)"

down-core:
	@$(CORE_COMPOSE) $(ALL_PROFILES) down

reset-core:
	@$(CORE_COMPOSE) $(ALL_PROFILES) down -v

up-observability:
	@echo "🚀 Building images and starting AMP with ClickHouse and the OTel collector..."
	@$(OBS_COMPOSE) up -d --build --wait --remove-orphans
	$(call install-observability,$(OBS_COMPOSE),/go/bin/amp)
	@echo "✅ AMP is running with observability"
	@echo "   Console: http://localhost:9000 (sign in as admin; password: $(COMPOSE) exec amp-api amp secret show admin-password)"

# Smoke checks against a running stack (scripts/smoke.sh).
smoke:
	@$(STACK_DIR)/scripts/smoke.sh amp

smoke-core:
	@$(STACK_DIR)/scripts/smoke.sh amp-core

smoke-observability:
	@$(STACK_DIR)/scripts/smoke-observability.sh

# Setup individual components
setup-colima:
	@cd deployments/setup && ./setup-colima.sh

# Console local setup. The install is gated on console/pnpm-lock.yaml.
.make:
	@mkdir -p .make

.make/console-deps-installed: console/pnpm-lock.yaml | .make
	@echo "📦 Installing console dependencies locally..."
	@if ! command -v pnpm >/dev/null 2>&1; then \
		echo "⚠️  pnpm not found. Enabling via corepack..."; \
		command -v corepack >/dev/null 2>&1 || npm install -g corepack@0.36.0; \
		corepack enable; \
	fi
	@echo "📥 Running pnpm install..."
	@cd console && pnpm install --frozen-lockfile
	@touch .make/console-deps-installed

setup-console-local: .make/console-deps-installed

# Force reinstall of console dependencies (ignores timestamps)
setup-console-local-force:
	@rm -f .make/console-deps-installed
	@$(MAKE) setup-console-local

# Daily development commands
# --build: plain `up -d` reuses an image built from an older Dockerfile, silently
# running a stale toolchain. Cached layers keep the no-change case near-free.
dev-up:
	@echo "🚀 Starting Agent Manager (dev)..."
	@$(DEV_COMPOSE) up -d --build --wait --remove-orphans
	@$(MAKE) console BACKGROUND=true
	@echo "✅ Platform is running!"
	@echo "   Console: http://localhost:3000 (sign in as admin; password: $(DEV_COMPOSE) exec amp-api cat /var/lib/agent-manager/secrets/admin-password)"
	@echo "   API:     http://localhost:9000"

# The dev stack with the observability profile; amp-api's CLI is the binary
# air builds, run as the service user.
dev-up-observability:
	@echo "🚀 Starting Agent Manager (dev) with ClickHouse and the OTel collector..."
	@$(DEV_OBS_COMPOSE) up -d --build --wait --remove-orphans
	$(call install-observability,$(DEV_OBS_COMPOSE),su-exec 1000:1000 ./tmp/agent-manager-service)
	@$(MAKE) console BACKGROUND=true
	@echo "✅ Platform is running with observability!"
	@echo "   Console: http://localhost:3000 (sign in as admin; password: $(DEV_COMPOSE) exec amp-api cat /var/lib/agent-manager/secrets/admin-password)"
	@echo "   API:     http://localhost:9000"

dev-down:
	@echo "🛑 Stopping Agent Manager..."
	@$(DEV_COMPOSE) $(ALL_PROFILES) down
	@$(MAKE) stop-console
	@echo "✅ Platform stopped"

dev-restart:
	@echo "🔄 Restarting Agent Manager..."
	@$(DEV_COMPOSE) restart
	@$(MAKE) console BACKGROUND=true
	@echo "✅ Platform restarted"

dev-rebuild:
	@echo "🧹 Stopping services..."
	@$(DEV_COMPOSE) $(ALL_PROFILES) down
	@echo "🔨 Rebuilding Docker images..."
	@$(DEV_COMPOSE) build --no-cache
	@$(MAKE) dev-up

dev-logs:
	@$(DEV_COMPOSE) logs -f

dev-migrate:
	@cd agent-manager-service && make dev-migrate

# ThunderID
thunder-restart:
	@$(COMPOSE) restart thunder

# Deletes ThunderID's users, applications and keys; tokens it issued stop working.
# The configuration amp-init generates is kept and applied again.
thunder-reset:
	@echo "🧹 Resetting ThunderID (data wipe + bootstrap)..."
	@$(COMPOSE) rm -sf thunder thunder-setup amp-init
	@project=$$($(COMPOSE) config --format json | sed -n 's/^ *"name": "\(.*\)",$$/\1/p' | head -1); \
		docker volume rm $${project}_thunder-db $${project}_thunder-certs $${project}_thunder-secrets
	@$(COMPOSE) up -d --wait thunder
	@echo "✅ ThunderID reset complete!"

# Console dev server (pnpm dev on :3000)
console: setup-console-local
	@cd deployments/setup && ./console.sh $(if $(filter true,$(BACKGROUND)),--background)

stop-console:
	@cd deployments/setup && ./stop-console.sh

# Database commands
db-connect:
ifeq ($(DB),postgres)
	@$(COMPOSE) exec postgres sh -c 'psql -U "$$POSTGRES_USER" "$$POSTGRES_DB"'
else
	@$(COMPOSE) exec amp-api sqlite3 /var/lib/agent-manager/agent-manager.db
endif

# Service debugging
service-logs:
	@$(COMPOSE) logs -f amp-api

service-shell:
	@$(COMPOSE) exec amp-api sh

console-logs:
	@test -f .make/console.log || { echo "ℹ️  No console log: logs are only written by 'make console BACKGROUND=true'"; exit 1; }
	@tail -f .make/console.log

# amctl CLI client codegen (oapi-codegen against local OpenAPI spec)
# oapi-codegen comes from `make tools`, pinned to match .github/workflows/cli-codegen-check.yaml
amctl-gen-client:
	@oapi-codegen -config cli/pkg/clients/amsvc/gen/oapi-codegen.yaml agent-manager-service/docs/api_v1_openapi.yaml
	@oapi-codegen -config cli/pkg/clients/amsvc/gen/oapi-codegen-client.yaml agent-manager-service/docs/api_v1_openapi.yaml
	@oapi-codegen -config cli/pkg/clients/aigateway/gen/oapi-codegen.yaml agent-manager-service/modules/aigateway/api/openapi.yaml
	@oapi-codegen -config cli/pkg/clients/aigateway/gen/oapi-codegen-client.yaml agent-manager-service/modules/aigateway/api/openapi.yaml
	@echo "amctl client generated successfully"

# amctl CLI build targets
amctl-build:
	scripts/build-amctl.sh --single-target

amctl-release-dry-run:
	scripts/build-amctl.sh --output-dir dist/

amctl-test:
	cd cli && go test ./... -v

amctl-coverage-summary: | .make
	cd cli && go test -coverprofile=../.make/amctl-coverage.out ./...
	python3 scripts/coverage/go_summary.py .make/amctl-coverage.out --module github.com/wso2/agent-manager/cli \
		--exclude-file cli/coverage-excludes.txt --out .make/amctl-coverage.json

# BASE is the branch a pull request targets; changed lines are counted against it.
BASE ?= upstream/amp-2.x

amctl-coverage-check: amctl-coverage-summary
	python3 scripts/coverage/gate.py .make/amctl-coverage.json cli/coverage-gate.json --title "amctl coverage" \
		--go-profile .make/amctl-coverage.out --module github.com/wso2/agent-manager/cli --module-dir cli \
		--exclude-file cli/coverage-excludes.txt --base $(BASE) --report-only

# Code generation
gen-instrumentation-contract:
	@$(MAKE) -C agent-manager-observer gen-instrumentation-contract

check-contract-drift:
	@scripts/check-contract-drift.sh


# E2E tests
# Run all (parallel):    make e2e-test
# Run one suite:         make e2e-test SUITE=agent
# Run a suite group:     make e2e-test SUITE=cli            (recursive: all suites under tests/cli/)
# Run with focus filter to run a specific test case: make e2e-test FOCUS="Project Deletion Conflict"
# A focused run (FOCUS) runs serially (--procs=1); everything else uses -p.
GINKGO_PROCS := $(if $(FOCUS),--procs=1,-p)
# The running stack's generated client secrets, for the suites unless
# test/e2e/.env sets them.
STACK_SECRETS = api=$$(docker ps -q --filter label=com.docker.compose.service=amp-api | head -1); \
	secret() { [ -n "$$api" ] && docker exec "$$api" cat /var/lib/agent-manager/secrets/$$1 2>/dev/null; }; \
	: "$${IDP_CLIENT_SECRET:=$$(secret amp-api-client-secret)}" "$${THUNDER_SYSTEM_CLIENT_SECRET:=$$(secret amp-system-client-secret)}"; \
	export IDP_CLIENT_SECRET THUNDER_SYSTEM_CLIENT_SECRET

e2e-test:
	@echo "Running E2E tests..."
	@cd test/e2e && set -a && [ -f .env ] && . ./.env; set +a && $(STACK_SECRETS) && \
		go run github.com/onsi/ginkgo/v2/ginkgo -v $(GINKGO_PROCS) --timeout 45m --poll-progress-after=600s \
		--keep-going --junit-report=e2e-report.xml --output-dir=. \
		$(if $(FOCUS),--focus="$(FOCUS)") $(if $(SUITE),./tests/$(SUITE)/...,./tests/...)


# Security tests
#
# Two layers, both run by `make security-test`:
#
#   security-test-static  Go unit tests over the route table. No stack needed.
#   security-test-live    Ginkgo suites against a running deployment.
#
# The live suites live in test/e2e/security/ — deliberately NOT under
# test/e2e/tests/, because `make e2e-test` and the e2e CI job both glob
# ./tests/... and must not pick them up. They share the e2e framework/ and
# operations/ packages and run against the same stack `make e2e-test` needs.
#
# Run all:            make security-test
# Static only:        make security-test-static
# One live suite:     make security-test SUITE=authz
# One spec:           make security-test FOCUS="scope matrix"
# Parallel:           make security-test PROCS=4
#
# Serial by default, unlike e2e-test. These specs are individually cheap (one
# HTTP call each) but every one needs a scope-reduced token from the IDP.
# Ginkgo parallelism forks N processes with separate memory, so each rebuilds
# its own token cache — parallelism multiplies token requests against Thunder
# without buying much wall-clock. Override with PROCS= if that changes.
SECURITY_GINKGO_PROCS := $(if $(PROCS),--procs=$(PROCS),--procs=1)

security-test: security-test-static security-test-live

# Route-table invariants, all in agent-manager-service/api/route_authz_invariant_test.go:
#   every route carries a permission check; every declared permission gates
#   something; permissions are reachable through roles and IDP/E2E scope
#   catalogs; no handler reads the org from the URL path.
#
# ADDING AN INVARIANT? Name it TestSecurityInvariant<Whatever> and it runs here
# automatically. The suite selects by prefix rather than by a list of names,
# because a -run regex that matches nothing prints "no tests to run" and exits
# 0 — a broken list would report success while checking nothing. A self-check
# in the file enforces both halves of the convention.
#
# Env vars mirror scripts/run_unit_tests.sh: the api package loads config at
# init, so it needs them even though these tests touch no DB.
SECURITY_STATIC_TESTS := TestSecurityInvariant

security-test-static:
	@echo "Running static authorization invariants..."
	@cd agent-manager-service && \
		DB_HOST=localhost DB_PORT=5432 DB_USER=unit DB_PASSWORD=unit DB_NAME=unit \
		ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
		SERVER_PORT=8080 \
		go test -mod=readonly ./api/ -run '$(SECURITY_STATIC_TESTS)' -v

security-test-live:
	@echo "Running security tests (stack must be running)..."
	@cd test/e2e && set -a && [ -f .env ] && . ./.env; set +a && $(STACK_SECRETS) && \
		go run github.com/onsi/ginkgo/v2/ginkgo -v $(SECURITY_GINKGO_PROCS) --timeout 45m \
		--poll-progress-after=300s --keep-going --junit-report=security-report.xml --output-dir=. \
		$(if $(FOCUS),--focus="$(FOCUS)") $(if $(SUITE),./security/$(SUITE)/...,./security/...)


# Cleanup
teardown:
	@$(MAKE) stop-console
	@$(DEV_COMPOSE) $(ALL_PROFILES) down -v
