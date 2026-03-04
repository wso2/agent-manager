.PHONY: help setup setup-colima setup-k3d setup-openchoreo setup-platform setup-console-local setup-console-local-force dev-up dev-down dev-restart dev-rebuild dev-logs dev-migrate openchoreo-up openchoreo-down openchoreo-status teardown db-connect db-logs service-logs service-shell console-logs port-forward setup-kubeconfig-docker
.PHONY: helm-build helm-build-api helm-build-console helm-import helm-install helm-upgrade helm-sync helm-sync-api helm-sync-console helm-restart helm-status helm-logs helm-api-logs helm-console-logs helm-db-connect status api-logs dev-pause dev-resume

# Development mode: "compose" (default) or "helm"
DEV_MODE ?= compose

# AMP variables (keep in sync with deployments/scripts/env.sh)
AMP_NAMESPACE := wso2-amp
AMP_RELEASE_NAME := amp
AMP_IMAGE_TAG := 0.0.0-dev
CLUSTER_CONTEXT := k3d-openchoreo-local-v0.14.0

# Default target
help:
	@echo "Agent Manager Platform - Development Commands"
	@echo ""
	@echo "Current mode: DEV_MODE=$(DEV_MODE)"
	@echo "Switch modes: DEV_MODE=helm make <target>"
	@echo ""
	@echo "Setup (run once):"
	@echo "  make setup                   - Complete setup (Colima + k3d + OpenChoreo + Platform)"
	@echo "  make setup-colima            - Start Colima VM"
	@echo "  make setup-k3d              - Create k3d cluster"
	@echo "  make setup-thunder          - Setup Thunder"
	@echo "  make setup-openchoreo        - Install OpenChoreo on k3d"
	@echo "  make setup-platform          - Build images and start core platform services"
	@echo "  make setup-console-local     - Install console deps (only if changed)"
	@echo "  make setup-console-local-force - Force reinstall console deps"
	@echo ""
	@echo "Daily Development (mode-aware):"
	@echo "  make dev-up                  - Start platform services"
	@echo "  make dev-down                - Stop platform services"
	@echo "  make dev-restart             - Restart platform services"
	@echo "  make dev-rebuild             - Rebuild images and restart services"
	@echo "  make dev-logs                - Tail all platform logs"
	@echo "  make dev-migrate             - Run database migrations"
	@echo ""
	@echo "Helm Mode (DEV_MODE=helm):"
	@echo "  make helm-build              - Build all Docker images from source"
	@echo "  make helm-build-api          - Build API image only"
	@echo "  make helm-build-console      - Build Console image only"
	@echo "  make helm-import             - Import all images into k3d"
	@echo "  make helm-install            - First-time Helm install (build + import + deploy)"
	@echo "  make helm-upgrade            - Helm upgrade (redeploy with current values)"
	@echo "  make helm-sync               - Full sync: build all + import + restart pods"
	@echo "  make helm-sync-api           - Fast: build API + import + restart API pod"
	@echo "  make helm-sync-console       - Fast: build Console + import + restart Console pod"
	@echo "  make helm-restart            - Restart all AMP deployments"
	@echo "  make helm-status             - Show pods and services"
	@echo "  make helm-logs               - Tail all AMP logs"
	@echo "  make helm-api-logs           - Tail API logs"
	@echo "  make helm-console-logs       - Tail Console logs"
	@echo "  make helm-db-connect         - psql into PostgreSQL pod"
	@echo ""
	@echo "OpenChoreo Runtime:"
	@echo "  make openchoreo-up      - Start OpenChoreo cluster"
	@echo "  make openchoreo-down    - Stop OpenChoreo cluster (saves resources)"
	@echo "  make openchoreo-status  - Check OpenChoreo cluster status"
	@echo "  make port-forward       - Forward OpenChoreo services to localhost"
	@echo ""
	@echo "Database (Compose mode):"
	@echo "  make db-connect         - Connect to PostgreSQL"
	@echo "  make db-logs            - View database logs"
	@echo ""
	@echo "Service Debugging (Compose mode):"
	@echo "  make service-logs       - View service logs"
	@echo "  make service-shell      - Shell into service container"
	@echo "  make console-logs       - View console logs"
	@echo ""
	@echo "Pause / Resume (saves laptop resources):"
	@echo "  make dev-pause          - Stop k3d cluster and Colima VM"
	@echo "  make dev-resume         - Start Colima VM and k3d cluster"
	@echo ""
	@echo "Cleanup:"
	@echo "  make teardown           - Remove everything (cluster + platform)"
	@echo ""

# ============================================================================
# Setup
# ============================================================================

# Complete setup - dispatches based on DEV_MODE
ifeq ($(DEV_MODE),helm)
setup: setup-colima setup-k3d setup-openchoreo setup-thunder helm-install
	@echo ""
	@echo "Complete setup finished! (Helm mode)"
	@echo ""
	@echo "Access your services:"
	@echo "   Console:   http://localhost:3000"
	@echo "   API:       http://localhost:9000"
	@echo ""
	@echo "Useful commands:"
	@echo "   make helm-status        - Show pod status"
	@echo "   make helm-sync-api      - Rebuild and redeploy API"
	@echo "   make helm-api-logs      - Tail API logs"
else
setup: setup-colima setup-k3d setup-openchoreo setup-thunder setup-kubeconfig-docker setup-platform setup-console-local
	@echo ""
	@echo "Complete setup finished!"
	@echo ""
	@echo "Access your services:"
	@echo "   Console:   http://localhost:3000"
	@echo "   API:       http://localhost:8080"
	@echo "   Traces Observer Service: http://localhost:9098"
	@echo "   Database:  localhost:5432"
	@echo ""
	@echo "To access OpenChoreo services, run:"
	@echo "   make port-forward"
endif

# Setup individual components
setup-colima:
	@cd deployments/scripts && ./setup-colima.sh

setup-k3d:
	@cd deployments/scripts && DEV_MODE=$(DEV_MODE) ./setup-k3d.sh

setup-thunder:
	@cd deployments/scripts && ./setup-amp-thunder.sh

setup-openchoreo:
	@cd deployments/scripts && ./setup-openchoreo.sh $(CURDIR)

gen-keys:
	@echo "Generating JWT signing keys..."
	@cd agent-manager-service && make gen-keys
	@echo "JWT signing keys generated in agent-manager-service/keys/"

setup-platform: gen-keys
	@cd deployments/scripts && ./setup-platform.sh

# Console local setup with dependency tracking
# This will only rebuild when rush.json or pnpm-lock.yaml changes
.make:
	@mkdir -p .make

.make/console-deps-installed: console/rush.json console/common/config/rush/pnpm-lock.yaml | .make
	@echo "Installing console dependencies locally..."
	@if ! command -v rush &> /dev/null; then \
		echo "Rush not found. Installing Rush globally..."; \
		npm install -g @microsoft/rush@5.157.0; \
	fi
	@echo "Running rush update..."
	@cd console && rush update --full
	@touch .make/console-deps-installed

.make/console-built: .make/console-deps-installed
	@echo "Building monorepo packages..."
	@cd console && rush build
	@touch .make/console-built
	@echo "Console packages built"

setup-console-local: .make/console-built
	@echo "Console dependencies are up to date"

# Force rebuild of console dependencies (ignores timestamps)
setup-console-local-force:
	@rm -f .make/console-deps-installed .make/console-built
	@$(MAKE) setup-console-local

# Generate Docker-specific kubeconfig using k3d kubeconfig
# Always regenerates to ensure it matches the current cluster
setup-kubeconfig-docker:
	@echo "Generating Docker kubeconfig..."
	@cd deployments/scripts && ./generate-docker-kubeconfig.sh
	@echo "Docker kubeconfig is ready"

# ============================================================================
# Daily Development (mode-aware)
# ============================================================================

ifeq ($(DEV_MODE),helm)

dev-up:
	@cd deployments/scripts && ./helm-deploy-amp.sh

dev-down:
	@cd deployments/scripts && ./helm-deploy-amp.sh --uninstall

dev-restart: helm-restart

dev-rebuild: helm-sync

dev-logs: helm-logs

else

dev-up: setup-console-local setup-kubeconfig-docker gen-keys
	@echo "Starting Agent Manager platform..."
	@cd deployments && docker compose up -d
	@echo "Platform is running!"
	@echo "   Console: http://localhost:3000"
	@echo "   API:     http://localhost:8080"

dev-down:
	@echo "Stopping Agent Manager platform..."
	@cd deployments && docker compose down
	@echo "Platform stopped"

dev-restart:
	@echo "Restarting Agent Manager platform..."
	@cd deployments && docker compose restart
	@echo "Platform restarted"

dev-rebuild: setup-console-local
	@echo "Stopping services..."
	@cd deployments && docker compose down
	@echo "Removing console volumes (preserving database)..."
	@docker volume rm deployments_console_node_modules deployments_console_common_temp 2>/dev/null || true
	@echo "Cleaning Rush temp directory..."
	@rm -rf console/common/temp
	@echo "Rebuilding Docker images..."
	@cd deployments && docker compose build --no-cache
	@echo "Starting services..."
	@cd deployments && docker compose up -d
	@echo "Rebuild complete!"
	@echo "   Console: http://localhost:3000"
	@echo "   API:     http://localhost:8080"

dev-logs:
	@cd deployments && docker compose logs -f

endif

dev-migrate:
	@echo "Running database migrations..."
	@docker exec agent-manager-service sh -c "cd /go/src && make dev-migrate"
	@echo "Migrations completed"

# ============================================================================
# Helm Mode Targets
# ============================================================================

# Build all Docker images from source
helm-build:
	@cd deployments/scripts && ./build-and-import.sh api console traces-observer evaluation-job

# Build individual components
helm-build-api:
	@echo "Building API image..."
	@docker build -t amp-api:$(AMP_IMAGE_TAG) agent-manager-service/ --quiet
	@echo "API image built."

helm-build-console:
	@echo "Building Console image..."
	@docker build -t amp-console:$(AMP_IMAGE_TAG) console/ --quiet
	@echo "Console image built."

# Import all images into k3d (assumes images are already built)
helm-import:
	@echo "Importing images into k3d..."
	@k3d image import amp-api:$(AMP_IMAGE_TAG) amp-console:$(AMP_IMAGE_TAG) amp-traces-observer:$(AMP_IMAGE_TAG) amp-evaluation-job:$(AMP_IMAGE_TAG) -c openchoreo-local-v0.14.0
	@echo "Images imported."

# First-time Helm install: build + import + deploy
helm-install: helm-build
	@cd deployments/scripts && ./helm-deploy-amp.sh

# Helm upgrade (redeploy with current values, no image rebuild)
helm-upgrade:
	@cd deployments/scripts && ./helm-deploy-amp.sh

# Full sync: build all images, import, restart pods
helm-sync: helm-build
	@echo "Restarting AMP deployments..."
	@kubectl rollout restart deployment -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) 2>/dev/null || true
	@echo "Waiting for rollout..."
	@kubectl rollout status deployment -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --timeout=300s 2>/dev/null || true
	@echo "Sync complete."

# Fast sync: build API only, import, restart API pod
helm-sync-api: helm-build-api
	@echo "Importing API image into k3d..."
	@k3d image import amp-api:$(AMP_IMAGE_TAG) -c openchoreo-local-v0.14.0
	@echo "Restarting API deployment..."
	@kubectl rollout restart deployment/$(AMP_RELEASE_NAME)-agent-manager-service -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT)
	@kubectl rollout status deployment/$(AMP_RELEASE_NAME)-agent-manager-service -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --timeout=120s
	@echo "API synced."

# Fast sync: build Console only, import, restart Console pod
helm-sync-console: helm-build-console
	@echo "Importing Console image into k3d..."
	@k3d image import amp-console:$(AMP_IMAGE_TAG) -c openchoreo-local-v0.14.0
	@echo "Restarting Console deployment..."
	@kubectl rollout restart deployment/$(AMP_RELEASE_NAME)-console -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT)
	@kubectl rollout status deployment/$(AMP_RELEASE_NAME)-console -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --timeout=120s
	@echo "Console synced."

# Restart all AMP deployments (no rebuild)
helm-restart:
	@echo "Restarting all AMP deployments..."
	@kubectl rollout restart deployment -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT)
	@kubectl rollout status deployment -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --timeout=300s 2>/dev/null || true
	@echo "Restart complete."

# Show pod and service status
helm-status:
	@echo "=== Pods ==="
	@kubectl get pods -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) 2>/dev/null || echo "Namespace $(AMP_NAMESPACE) not found"
	@echo ""
	@echo "=== Services ==="
	@kubectl get svc -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) 2>/dev/null || echo "Namespace $(AMP_NAMESPACE) not found"

# Alias for helm-status
status: helm-status

# Tail all AMP logs
helm-logs:
	@kubectl logs -f -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --all-containers --max-log-requests=10 -l "app.kubernetes.io/instance=$(AMP_RELEASE_NAME)" --prefix 2>/dev/null || \
		kubectl logs -f -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) --all-containers --max-log-requests=10 --prefix

# Tail API logs
helm-api-logs:
	@kubectl logs -f -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) -l "app.kubernetes.io/component=agent-manager-service" --all-containers --prefix

# Alias for helm-api-logs
api-logs: helm-api-logs

# Tail Console logs
helm-console-logs:
	@kubectl logs -f -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) -l "app.kubernetes.io/component=console" --all-containers --prefix

# psql into PostgreSQL pod
helm-db-connect:
	@kubectl exec -it -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) \
		$$(kubectl get pod -n $(AMP_NAMESPACE) --context $(CLUSTER_CONTEXT) -l "app.kubernetes.io/name=postgresql" -o jsonpath='{.items[0].metadata.name}') \
		-- psql -U agentmanager -d agentmanager

# ============================================================================
# OpenChoreo lifecycle management
# ============================================================================

openchoreo-up:
	@echo "Starting OpenChoreo cluster..."
	@docker start openchoreo-local-control-plane openchoreo-local-worker 2>/dev/null || (echo "Cluster not found. Run 'make setup-k3d setup-openchoreo' first." && exit 1)
	@echo "Waiting for nodes to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12; do \
		kubectl get nodes --context kind-openchoreo-local >/dev/null 2>&1 && \
		kubectl wait --for=condition=Ready nodes --all --timeout=10s --context kind-openchoreo-local >/dev/null 2>&1 && break || sleep 10; \
	done
	@echo "Waiting for core system pods..."
	@kubectl wait --for=condition=Ready pods --all -n kube-system --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "Waiting for OpenChoreo control plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-control-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "Waiting for OpenChoreo data plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-data-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "Waiting for OpenChoreo observability plane..."
	@kubectl wait --for=condition=Ready pods --all -n openchoreo-observability-plane --timeout=90s --context kind-openchoreo-local 2>/dev/null || true
	@echo "OpenChoreo cluster is running"
	@echo ""
	@echo "Cluster status:"
	@kubectl get pods --all-namespaces --context kind-openchoreo-local | grep -v "Running\|Completed" | head -1 || echo "   All pods are running!"

openchoreo-down:
	@echo "Stopping OpenChoreo cluster..."
	@docker stop openchoreo-local-control-plane openchoreo-local-worker 2>/dev/null && echo "OpenChoreo cluster stopped (containers preserved)" || echo "Cluster not running"

openchoreo-status:
	@echo "OpenChoreo Cluster Status:"
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

# ============================================================================
# Database & Service Debugging (Compose mode)
# ============================================================================

db-connect:
	@docker exec -it agent-manager-db psql -U agentmanager -d agentmanager

db-logs:
	@docker logs -f agent-manager-db

service-logs:
	@docker logs -f agent-manager-service

service-shell:
	@docker exec -it agent-manager-service sh

console-logs:
	@docker logs -f agent-manager-console

# ============================================================================
# Pause / Resume (saves laptop resources)
# ============================================================================

CLUSTER_NAME := openchoreo-local-v0.14.0

dev-pause:
	@echo "Stopping k3d cluster..."
	@k3d cluster stop $(CLUSTER_NAME) 2>/dev/null || echo "Cluster not running"
	@echo "Stopping Colima..."
	@colima stop 2>/dev/null || echo "Colima not running"
	@echo "All stopped. CPU and memory freed."

dev-resume:
	@echo "Starting Colima..."
	@colima start
	@echo "Starting k3d cluster..."
	@k3d cluster start $(CLUSTER_NAME)
	@echo "Waiting for cluster to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		kubectl cluster-info --context $(CLUSTER_CONTEXT) &>/dev/null && break || sleep 3; \
	done
	@echo "Cluster is ready."

# ============================================================================
# Cleanup
# ============================================================================

teardown:
	@cd deployments/scripts && ./teardown.sh
