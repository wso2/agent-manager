.PHONY: help build docker-load-k3d lock check-locks

# Variables
PYTHON_VERSION ?= 3.11
# Default must be a version with a committed lock (see locks/); the image installs
# locks/traceloop-$(TRACELOOP_VERSION)-python$(PYTHON_VERSION).txt.
TRACELOOP_VERSION ?= 0.63.0
TAG ?= dev-python$(PYTHON_VERSION)
REGISTRY_ENDPOINT ?= ghcr.io/wso2
APP_NAME = $(REGISTRY_ENDPOINT)/amp-python-instrumentation-provider
DOCKER_IMAGE = $(APP_NAME):$(TAG)
K3D_CLUSTER_NAME = amp-dev
KIND_CLUSTER_NAME = amp-dev

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build Docker image
	@echo "Building $(DOCKER_IMAGE)..."
	@docker build --platform linux/amd64 \
		--build-arg PYTHON_VERSION=$(PYTHON_VERSION) \
		--build-arg TRACELOOP_VERSION=$(TRACELOOP_VERSION) \
		-t $(DOCKER_IMAGE) .

docker-load-k3d: build ## Build and load image into k3d cluster
	@echo "Loading $(DOCKER_IMAGE) into k3d cluster $(K3D_CLUSTER_NAME)..."
	@k3d image import $(DOCKER_IMAGE) --cluster $(K3D_CLUSTER_NAME)

lock: ## Regenerate per-(traceloop x python) dependency locks from release-config.json
	@bash scripts/gen-locks.sh

check-locks: ## Verify committed locks cover the release-config matrix (hermetic; no PyPI/Docker)
	@bash scripts/check-locks.sh
