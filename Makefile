.PHONY: help setup setup-colima setup-k3d setup-openchoreo setup-platform setup-console-local setup-console-local-force dev-up dev-down dev-restart dev-rebuild dev-logs dev-migrate openchoreo-up openchoreo-down openchoreo-status teardown db-connect db-logs service-logs service-shell console-logs port-forward setup-kubeconfig-docker gen-eval-artifacts

# Default target
help:
	@echo "Agent Manager Platform - Development Commands"
	@echo ""
	@echo "🚀 Setup (run once):"
	@echo "  make setup                   - Complete setup (Colima + k3d + OpenChoreo + Platform)"
	@echo "  make setup-colima            - Start Colima VM"
	@echo "  make setup-k3d              - Create k3d cluster"
	@echo "  make setup-openchoreo        - Install OpenChoreo on k3d"
	@echo "  make setup-platform          - Build images and start core platform services"
	@echo "  make setup-console-local     - Install console deps (only if changed)"
	@echo "  make setup-console-local-force - Force reinstall console deps"
	@echo ""
	@echo "💻 Daily Development:"
	@echo "  make dev-up                  - Start platform services (console, service, db)"
	@echo "  make dev-down                - Stop platform services"
	@echo "  make dev-restart             - Restart platform services"
	@echo "  make dev-rebuild             - Rebuild images and restart services"
	@echo "  make dev-logs                - Tail all platform logs"
	@echo "  make dev-migrate             - Generate evaluators and run database migrations"
	@echo ""
	@echo "☸️  OpenChoreo Runtime:"
	@echo "  make openchoreo-up      - Start OpenChoreo cluster"
	@echo "  make openchoreo-down    - Stop OpenChoreo cluster (saves resources)"
	@echo "  make openchoreo-status  - Check OpenChoreo cluster status"
	@echo "  make port-forward       - Forward OpenChoreo services to localhost"
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
	@echo "🧹 Cleanup:"
	@echo "  make teardown           - Remove everything (Kind cluster + platform)"
	@echo ""

# Complete setup
setup: setup-colima setup-k3d setup-openchoreo setup-platform setup-console-local
	@echo ""
	@echo "✅ Complete setup finished!"
	@echo ""
	@echo "🌐 Access your services:"
	@echo "   Console:   http://localhost:3000"
	@echo "   API:       http://localhost:8080"
	@echo "   Traces Observer Service: http://localhost:9098"
	@echo "   Database:  localhost:5432"
	@echo ""
	@echo "📊 To access OpenChoreo services, run:"
	@echo "   make port-forward"

# Setup individual components
setup-colima:
	@cd deployments/scripts && ./setup-colima.sh

setup-k3d:
	@cd deployments/scripts && ./setup-k3d.sh && ./setup-prerequisites.sh

setup-openchoreo:
	@cd deployments/scripts && ./setup-openchoreo.sh $(CURDIR)

gen-keys:
	@echo "🔑 Generating JWT signing keys..."
	@cd agent-manager-service && make gen-keys
	@echo "✅ JWT signing keys generated in agent-manager-service/keys/"

setup-platform: gen-keys
	@cd deployments/scripts && ./setup-platform.sh

# Console local setup with dependency tracking
# This will only rebuild when rush.json or pnpm-lock.yaml changes
.make:
	@mkdir -p .make

.make/console-deps-installed: console/rush.json console/common/config/rush/pnpm-lock.yaml | .make
	@echo "📦 Installing console dependencies locally..."
	@if ! command -v rush &> /dev/null; then \
		echo "⚠️  Rush not found. Installing Rush globally..."; \
		npm install -g @microsoft/rush@5.157.0; \
	fi
	@echo "📥 Running rush update..."
	@cd console && rush update --full
	@touch .make/console-deps-installed

.make/console-built: .make/console-deps-installed
	@echo "🔨 Building monorepo packages..."
	@cd console && rush build
	@touch .make/console-built
	@echo "✅ Console packages built"

setup-console-local: .make/console-built
	@echo "✅ Console dependencies are up to date"

# Force rebuild of console dependencies (ignores timestamps)
setup-console-local-force:
	@rm -f .make/console-deps-installed .make/console-built
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
	@docker volume rm deployments_console_node_modules deployments_console_common_temp 2>/dev/null || true
	@echo "🧹 Cleaning Rush temp directory..."
	@rm -rf console/common/temp
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
	@echo "🗄️  Running database migrations..."
	@docker exec agent-manager-service sh -c "cd /go/src && make dev-migrate"
	@echo "✅ Migrations completed"

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

# Port forwarding for OpenChoreo
port-forward:
	@cd deployments/scripts && ./port-forward.sh

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

# Code generation
gen-eval-artifacts:
	@echo "Generating evaluator artifacts..."
	@cd agent-manager-service && make gen-evaluators-dev
	@bash console/workspaces/pages/eval/scripts/generate-evaluator-models.sh --dev
	@echo "All evaluator artifacts generated"

# Cleanup
teardown:
	@cd deployments/scripts && ./teardown.sh
