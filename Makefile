.PHONY: help setup setup-colima setup-k3d setup-openchoreo setup-default-env-thunder setup-sandbox setup-gvisor setup-kata setup-platform setup-gateway setup-console-local setup-console-local-force setup-amp teardown-amp reset-amp dev-up dev-down dev-restart dev-rebuild dev-logs dev-migrate openchoreo-up openchoreo-down openchoreo-status thunder-up thunder-down thunder-restart thunder-reset teardown db-connect db-logs service-logs service-shell console-logs port-forward stop-port-forward gen-eval-artifacts gen-instrumentation-contract check-contract-drift check-matrix-manifest e2e-test security-test security-test-static security-test-live

# Absolute path to the console directory on the host. Passed to docker-compose
# so the container mounts and builds at the same path, keeping pnpm
# symlinks valid on both the host and inside the container.
export CONSOLE_HOST_PATH := $(realpath $(CURDIR)/console)

# Default target
help:
	@echo "Agent Manager Platform - Development Commands"
	@echo ""
	@echo "🚀 Setup (run once):"
	@echo "  make setup                   - Complete setup (Colima + k3d + OpenChoreo + Platform)"
	@echo "  make setup-colima            - Start Colima VM"
	@echo "  make setup-k3d              - Create k3d cluster"
	@echo "  make setup-openchoreo        - Install OpenChoreo on k3d"
	@echo "  make setup-sandbox           - Install Agent Sandbox module (run after setup-openchoreo)"
	@echo "  make setup-gvisor            - Add a gVisor (runsc) isolation node to the cluster (run after make setup)"
	@echo "  make setup-kata              - Join a real (non-Docker) node to the cluster and install Kata there (needs nested virt; run after make setup)"
	@echo "  make setup-platform          - Build images and start core platform services"
	@echo "  make setup-gateway           - Install API Platform Gateway (run via make setup)"
	@echo "  make setup-console-local     - Install console deps (only if changed), then build"
	@echo "  make setup-console-local-force - Force reinstall console deps"
	@echo ""
	@echo "💻 Daily Development:"
	@echo "  make dev-up                  - Start platform services (console, service, db)"
	@echo "  make dev-down                - Stop platform services"
	@echo "  make dev-restart             - Restart platform services"
	@echo "  make dev-rebuild             - Rebuild images and restart services"
	@echo "  make dev-logs                - Tail all platform logs"
	@echo "  make dev-migrate             - Generate evaluators and run database migrations (run after dev-up/setup)"
	@echo ""
	@echo "☸️  OpenChoreo Runtime:"
	@echo "  make openchoreo-up      - Start OpenChoreo cluster"
	@echo "  make openchoreo-down    - Stop OpenChoreo cluster (saves resources)"
	@echo "  make openchoreo-status  - Check OpenChoreo cluster status"
	@echo "  make port-forward       - Stop and restart all port-forwards (interactive)"
	@echo "  make stop-port-forward  - Stop all active port-forwards"
	@echo ""
	@echo "⚡ Platform Thunder (Control Plane IDP):"
	@echo "  make thunder-up         - Install or upgrade Platform Thunder"
	@echo "  make thunder-down       - Uninstall Platform Thunder"
	@echo "  make thunder-restart    - Restart Platform Thunder pods"
	@echo "  make thunder-reset      - Reset Platform Thunder (uninstall + PVC wipe + reinstall)"
	@echo ""
	@echo "🗄️  Database:"
	@echo "  make db-connect         - Connect to PostgreSQL"
	@echo "  make db-logs            - View database logs"
	@echo ""
	@echo "🔧 Service Debugging:"
	@echo "  make service-logs       - View service logs"
	@echo "  make service-shell      - Shell into service container"
	@echo "  make console-logs       - View console logs"
	@echo ""
	@echo "🔧 Code Generation:"
	@echo "  make gen-eval-artifacts - Regenerate evaluator Go catalog + console TS models"
	@echo ""
	@echo "amctl CLI:"
	@echo "  make amctl-build             - Build amctl for current platform"
	@echo "  make amctl-release-dry-run   - Cross-compile all targets without publishing"
	@echo "  make amctl-test              - Run amctl tests"
	@echo "🧪 E2E Tests:"
	@echo "  make setup-ai-gateway   - Install AI Gateway (needed for LLM proxy tests)"
	@echo "  make e2e-test           - Run E2E tests (cluster must be running)"
	@echo ""
	@echo "🔒 Security Tests:"
	@echo "  make security-test         - Run all security tests (static + live)"
	@echo "  make security-test-static  - Route-table authz invariants (no cluster needed)"
	@echo "  make security-test-live    - Security suites against a running deployment"
	@echo ""
	@echo "🧹 Cleanup:"
	@echo "  make reset-amp          - Fast reset: teardown-amp + setup-amp (OpenChoreo base preserved)"
	@echo "  make teardown-amp       - Remove AMP layer only (extensions, platform, gateways, env-Thunders)"
	@echo "  make setup-amp          - Reinstall AMP layer atop the existing OpenChoreo base"
	@echo "  make teardown           - Remove everything (Kind cluster + platform)"
	@echo ""

# Complete setup
setup: setup-colima setup-k3d setup-openchoreo setup-platform setup-sandbox setup-console-local
	@$(MAKE) dev-migrate
	@cd deployments/setup && ./port-forward.sh --platform --background
	@$(MAKE) setup-default-env-thunder
	@$(MAKE) setup-gateway
	@cd deployments/setup && ./port-forward.sh --background
	@echo ""
	@echo "✅ Setup complete!"
	@echo ""
	@echo "   Console:                 http://localhost:3000"
	@echo "   API:                     http://localhost:8080"
	@bash deployments/scripts/print-admin-credentials.sh http://localhost:3000 amp-thunder || true
	@echo ""
	@echo "Run 'make stop-port-forward' to stop port-forwards"
	@echo "Run 'make port-forward' to restart in a dedicated terminal"

# AMP-layer lifecycle: rebuild everything above the OpenChoreo base (colima,
# k3d, prerequisites, planes, gateway-operator, agent-sandbox stay untouched).
# Mirrors the tail of `make setup` exactly, minus the base steps.
reset-amp:
	@$(MAKE) teardown-amp
	@$(MAKE) setup-amp

teardown-amp:
	@cd deployments/setup && ./teardown-amp.sh

setup-amp: setup-console-local
	@cd deployments/setup && ./setup-amp-extensions.sh $(CURDIR)
	@$(MAKE) setup-platform
	@$(MAKE) dev-migrate
	@cd deployments/setup && ./port-forward.sh --platform --background
	@$(MAKE) wait-ams-healthy
	@$(MAKE) setup-default-env-thunder
	@$(MAKE) setup-gateway
	@cd deployments/setup && ./port-forward.sh --background
	@echo ""
	@echo "✅ AMP layer ready!"
	@echo "   Console: http://localhost:3000"
	@echo "   API:     http://localhost:9000"
	@bash deployments/scripts/print-admin-credentials.sh http://localhost:3000 amp-thunder || true

# Air recompiles the Go service on a fresh container start, so :9000 lags
# `docker compose up` by up to a couple of minutes. Steps that call the AMS API
# (setup-default-env-thunder) must gate on this or they fail with HTTP 000.
wait-ams-healthy:
	@echo "⏳ Waiting for agent-manager-service on :9000..."
	@for i in $$(seq 1 60); do \
		curl -sf http://localhost:9000/healthz >/dev/null 2>&1 && exit 0; \
		sleep 3; \
	done; \
	echo "❌ agent-manager-service not healthy on :9000 after 3 minutes"; exit 1

# Setup individual components
setup-colima:
	@cd deployments/setup && ./setup-colima.sh

setup-k3d:
	@cd deployments/setup && ./setup-k3d.sh && ./setup-prerequisites.sh

setup-openchoreo:
	@cd deployments/setup && ./setup-openchoreo.sh $(CURDIR)

# Provisions the default env-Thunder instance. Must run after AMS is up +
# migrated (store_via_ams needs it) — hence after dev-migrate, not in setup-openchoreo.sh.
setup-default-env-thunder:
	@ENV_NAME=default DISPLAY_NAME="Default" ORG_NAME=default \
	  THUNDER_HANDLE=default-idp \
	  WAIT_TIMEOUT=300s \
	  AMP_API_URL="http://localhost:9000/api/v1" \
	  bash deployments/scripts/add-environment-thunder.sh \
	  && echo "✅ Default environment Thunder ID instance provisioned" \
	  || { \
	    echo "⚠️  Default-env Thunder provisioning failed — continuing with remaining setup steps."; \
	    echo "    Re-run manually: ENV_NAME=default DISPLAY_NAME=Default ORG_NAME=default THUNDER_HANDLE=default-idp \\"; \
	    echo "      AMP_API_URL=http://localhost:9000/api/v1 \\"; \
	    echo "      bash deployments/scripts/add-environment-thunder.sh"; \
	  }

setup-sandbox:
	@cd deployments/scripts && ./setup-sandbox.sh

setup-gvisor:
	@cd deployments/setup && ./setup-gvisor-node.sh

setup-kata:
	@cd deployments/setup && ./setup-kata-node.sh

gen-keys:
	@echo "🔑 Generating JWT signing keys..."
	@cd agent-manager-service && make gen-keys
	@echo "✅ JWT signing keys generated in agent-manager-service/keys/"

setup-platform: gen-keys
	@cd deployments/setup && ./setup-platform.sh

setup-gateway:
	@cd deployments/setup && ./setup-gateway.sh

# Console local setup. The install is gated on console/pnpm-lock.yaml.
.make:
	@mkdir -p .make

.make/console-deps-installed: console/pnpm-lock.yaml | .make
	@echo "📦 Installing console dependencies locally..."
	@if ! command -v pnpm >/dev/null 2>&1; then \
		echo "⚠️  pnpm not found. Enabling via corepack..."; \
		corepack enable; \
	fi
	@echo "📥 Running pnpm install..."
	@cd console && pnpm install --frozen-lockfile
	@touch .make/console-deps-installed

setup-console-local: .make/console-deps-installed
	@echo "🔨 Building monorepo packages..."
	@cd console && pnpm build
	@echo "✅ Console packages built"

# Force reinstall of console dependencies (ignores timestamps)
setup-console-local-force:
	@rm -f .make/console-deps-installed
	@$(MAKE) setup-console-local

# Daily development commands
dev-up: setup-console-local gen-keys
	@echo "🚀 Starting Agent Manager platform..."
	@cd deployments && docker compose up -d
	@echo "✅ Platform is running!"
	@echo "   Console: http://localhost:3000"
	@echo "   API:     http://localhost:8080"

dev-down:
	@echo "🛑 Stopping Agent Manager platform..."
	@cd deployments && docker compose down
	@echo "✅ Platform stopped"

dev-restart:
	@echo "🔄 Restarting Agent Manager platform..."
	@cd deployments && docker compose restart
	@echo "✅ Platform restarted"

dev-rebuild: setup-console-local
	@echo "🧹 Stopping services..."
	@cd deployments && docker compose down
	@echo "🧹 Removing console volumes (preserving database)..."
	@docker volume rm deployments_console_node_modules 2>/dev/null || true
	@echo "🔨 Rebuilding Docker images..."
	@cd deployments && docker compose build --no-cache
	@echo "🔄 Starting services..."
	@cd deployments && docker compose up -d
	@echo "✅ Rebuild complete!"
	@echo "   Console: http://localhost:3000"
	@echo "   API:     http://localhost:8080"

dev-logs:
	@cd deployments && docker compose logs -f

dev-migrate:
	@cd agent-manager-service && make dev-migrate

# OpenChoreo lifecycle management
openchoreo-up:
	@echo "🚀 Starting OpenChoreo cluster..."
	@docker start openchoreo-local-control-plane openchoreo-local-worker 2>/dev/null || (echo "⚠️  Cluster not found. Run 'make setup-k3d setup-openchoreo' first." && exit 1)
	@echo "⏳ Waiting for nodes to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12; do \
		kubectl get nodes --context kind-openchoreo-local >/dev/null 2>&1 && \
		kubectl wait --for=condition=Ready nodes --all --timeout=10s --context kind-openchoreo-local >/dev/null 2>&1 && break || sleep 10; \
	done
	@echo "⏳ Waiting for core system pods..."
	@kubectl wait --for=condition=Ready pods --all -n kube-system --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "⏳ Waiting for OpenChoreo control plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-control-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "⏳ Waiting for OpenChoreo data plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-data-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "⏳ Waiting for OpenChoreo observability plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-observability-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "✅ OpenChoreo cluster is running"
	@echo ""
	@echo "📊 Cluster status:"
	@kubectl get pods --all-namespaces --context kind-openchoreo-local | grep -v "Running\|Completed" | head -1 || echo "   All pods are running!"

openchoreo-down:
	@echo "🛑 Stopping OpenChoreo cluster..."
	@docker stop openchoreo-local-control-plane openchoreo-local-worker 2>/dev/null && echo "✅ OpenChoreo cluster stopped (containers preserved)" || echo "⚠️  Cluster not running"

openchoreo-status:
	@echo "📊 OpenChoreo Cluster Status:"
	@echo ""
	@echo "Docker Containers:"
	@docker ps -a --filter name=openchoreo-local --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "No containers found"
	@echo ""
	@echo "Kubernetes Nodes:"
	@kubectl get nodes --context kind-openchoreo-local 2>/dev/null || echo "Cluster not accessible (may be stopped)"
	@echo ""
	@echo "OpenChoreo Pods:"
	@kubectl get pods -n openchoreo-system --context kind-openchoreo-local 2>/dev/null || echo "Cluster not accessible"

# Platform Thunder management
thunder-up:
	@echo "📦 Installing/Upgrading Platform Thunder (amp-thunder-extension)..."
	@helm dependency update deployments/helm-charts/wso2-amp-thunder-extension
	@helm upgrade --install amp-thunder-extension deployments/helm-charts/wso2-amp-thunder-extension \
		--namespace amp-thunder --create-namespace \
		--set thunder.bootstrap.agentManagerMcpBaseUrl=http://localhost:9000 \
		--set thunder.setup.admin.password=admin
	@echo "⏳ Waiting for Platform Thunder deployment..."
	@kubectl rollout status deployment -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension --timeout=120s 2>/dev/null || true
	@echo "✅ Platform Thunder is up and running!"

thunder-down:
	@echo "🛑 Uninstalling Platform Thunder (amp-thunder-extension)..."
	@helm uninstall amp-thunder-extension -n amp-thunder 2>/dev/null || true
	@kubectl delete job -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension --wait --timeout=60s 2>/dev/null || true
	@echo "✅ Platform Thunder uninstalled"

thunder-restart:
	@echo "🔄 Restarting Platform Thunder deployment..."
	@kubectl rollout restart deployment -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension 2>/dev/null || true
	@kubectl rollout status deployment -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension --timeout=120s 2>/dev/null || true
	@echo "✅ Platform Thunder restarted"

thunder-reset:
	@echo "🧹 Resetting Platform Thunder (uninstall + job/PVC delete + reinstall)..."
	@helm uninstall amp-thunder-extension -n amp-thunder --wait --timeout=2m 2>/dev/null || true
	@kubectl delete job -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension --wait --timeout=60s 2>/dev/null || true
	@kubectl delete pvc -n amp-thunder -l app.kubernetes.io/instance=amp-thunder-extension --wait --timeout=60s 2>/dev/null || true
	@$(MAKE) thunder-up
	@echo "✅ Platform Thunder reset complete!"

# Port forwarding for OpenChoreo
PLATFORM   ?=
BACKGROUND ?=

port-forward:
	@cd deployments/setup && ./port-forward.sh \
		$(if $(filter true,$(PLATFORM)),--platform) \
		$(if $(filter true,$(BACKGROUND)),--background)

stop-port-forward:
	@cd deployments/setup && ./stop-port-forward.sh

# Database commands
db-connect:
	@docker exec -it agent-manager-db psql -U agentmanager -d agentmanager

db-logs:
	@docker logs -f agent-manager-db

# Service debugging
service-logs:
	@docker logs -f agent-manager-service

service-shell:
	@docker exec -it agent-manager-service sh

console-logs:
	@docker logs -f agent-manager-console

# amctl CLI client codegen (oapi-codegen against local OpenAPI spec)
# Pinned to the same version used in .github/workflows/cli-codegen-check.yaml
OAPI_CODEGEN_VERSION := v2.6.0

amctl-gen-client:
	@if ! command -v oapi-codegen >/dev/null || ! oapi-codegen -version 2>&1 | grep -qx '$(OAPI_CODEGEN_VERSION)'; then \
		echo "Installing oapi-codegen $(OAPI_CODEGEN_VERSION)..."; \
		go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION); \
	fi
	@oapi-codegen -config cli/pkg/clients/amsvc/gen/oapi-codegen.yaml agent-manager-service/docs/api_v1_openapi.yaml
	@oapi-codegen -config cli/pkg/clients/amsvc/gen/oapi-codegen-client.yaml agent-manager-service/docs/api_v1_openapi.yaml
	@echo "amctl client generated successfully"

# amctl CLI build targets
amctl-build:
	scripts/build-amctl.sh --single-target

amctl-release-dry-run:
	scripts/build-amctl.sh --output-dir dist/

amctl-test:
	cd cli && go test ./... -v

# Code generation
gen-eval-artifacts:
	@echo "Generating evaluator artifacts..."
	@cd agent-manager-service && make gen-evaluators-dev
	@bash console/workspaces/pages/eval/scripts/generate-evaluator-models.sh --dev
	@echo "All evaluator artifacts generated"

gen-instrumentation-contract:
	@$(MAKE) -C agent-manager-observer gen-instrumentation-contract

check-contract-drift:
	@scripts/check-contract-drift.sh

check-matrix-manifest:
	@scripts/check-matrix-manifest.py

# AI Gateway setup (required for LLM proxy/guardrail tests)
setup-ai-gateway: dev-migrate
	@echo "🚀 Installing AI Gateway extension..."
	@helm upgrade --install amp-ai-gateway deployments/helm-charts/wso2-amp-api-platform-gateway-extension \
		--namespace openchoreo-data-plane \
		--set agentManager.apiUrl="http://host.k3d.internal:9000/api/v1" \
		--set agentManager.idp.tokenUrl="http://amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090/oauth2/token" \
		--set agentManager.idp.clientId="amp-api-client" \
		--set agentManager.idp.clientSecret="amp-api-client-secret" \
		--set gateway.name="default" \
		--set gateway.displayName="Default AI Gateway" \
		--set gateway.vhost="http://ai-gateway.amp.localhost:8084" \
		--set gateway.type="EGRESS" \
		--set apiGateway.controlPlane.host="host.k3d.internal:9243" \
		--set developmentMode=true
	@echo "⏳ Waiting for gateway bootstrap job..."
	@kubectl wait --for=condition=complete job/amp-ai-gateway-bootstrap \
		-n openchoreo-data-plane --timeout=300s 2>/dev/null || true
	@echo "✅ AI Gateway installed"

# E2E tests
# Run all (parallel):    make e2e-test
# Run one suite:         make e2e-test SUITE=monitors
# Run a suite group:     make e2e-test SUITE=cli            (recursive: all suites under tests/cli/)
# Run with focus filter to run a specific test case: make e2e-test FOCUS="Project Deletion Conflict"
# A focused run (FOCUS) runs serially (--procs=1); everything else uses -p.
GINKGO_PROCS := $(if $(FOCUS),--procs=1,-p)
e2e-test:
	@echo "Running E2E tests..."
	@cd test/e2e && set -a && [ -f .env ] && . ./.env; set +a && \
		go run github.com/onsi/ginkgo/v2/ginkgo -v $(GINKGO_PROCS) --timeout 45m --poll-progress-after=600s \
		--keep-going --junit-report=e2e-report.xml --output-dir=. \
		$(if $(FOCUS),--focus="$(FOCUS)") $(if $(SUITE),./tests/$(SUITE)/...,./tests/...)


# Security tests
#
# Two layers, both run by `make security-test`:
#
#   security-test-static  Go unit tests over the route table. No cluster needed.
#   security-test-live    Ginkgo suites against a running deployment.
#
# The live suites live in test/e2e/security/ — deliberately NOT under
# test/e2e/tests/, because `make e2e-test` and the e2e CI job both glob
# ./tests/... and must not pick them up. They share the e2e framework/ and
# operations/ packages and run against the same cluster `make e2e-test` needs.
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
# init, so it needs them even though these tests touch neither DB nor OpenChoreo.
SECURITY_STATIC_TESTS := TestSecurityInvariant

security-test-static:
	@echo "Running static authorization invariants..."
	@cd agent-manager-service && \
		DB_HOST=localhost DB_PORT=5432 DB_USER=unit DB_PASSWORD=unit DB_NAME=unit \
		OPEN_CHOREO_BASE_URL=http://localhost/api/v1 \
		ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
		SERVER_PORT=8080 \
		go test -mod=readonly ./api/ -run '$(SECURITY_STATIC_TESTS)' -v

security-test-live:
	@echo "Running security tests (cluster must be running)..."
	@cd test/e2e && set -a && [ -f .env ] && . ./.env; set +a && \
		go run github.com/onsi/ginkgo/v2/ginkgo -v $(SECURITY_GINKGO_PROCS) --timeout 45m \
		--poll-progress-after=300s --keep-going --junit-report=security-report.xml --output-dir=. \
		$(if $(FOCUS),--focus="$(FOCUS)") $(if $(SUITE),./security/$(SUITE)/...,./security/...)


# Cleanup
teardown:
	@cd deployments/setup && ./teardown.sh
