.PHONY: help docker-build docker-build-dev docker-push docker-load-k3d docker-clean

# Variables
TAG ?= 0.0.0-dev
AMP_EVALUATION_VERSION ?= $(TAG)
APP_NAME = amp-evaluation-monitor
DOCKER_IMAGE = $(APP_NAME):$(TAG)
OPEN_CHOREO_VERSION ?= 0.14.0
K3D_CLUSTER_NAME ?= openchoreo-local-v$(OPEN_CHOREO_VERSION)
REGISTRY_ENDPOINT ?= ghcr.io/wso2

# Project root for dev build context
PROJECT_ROOT = ..

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-25s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Docker commands
docker-build: ## Build production Docker image (installs amp-evaluation from PyPI)
	@echo "Building production Docker image $(DOCKER_IMAGE)..."
	@echo "  Using AMP_EVALUATION_VERSION=$(AMP_EVALUATION_VERSION)"
	@docker build -t $(DOCKER_IMAGE) \
		--build-arg AMP_EVALUATION_VERSION=$(AMP_EVALUATION_VERSION) \
		-f Dockerfile .
	@echo "✅ Built: $(DOCKER_IMAGE)"

docker-build-dev: ## Build development Docker image (installs amp-evaluation from local libs)
	@echo "Building development Docker image $(DOCKER_IMAGE)..."
	@echo "  Using local amp-evaluation SDK from libs/"
	@docker build -t $(DOCKER_IMAGE) -f Dockerfile.dev $(PROJECT_ROOT)
	@echo "✅ Built: $(DOCKER_IMAGE)"

docker-push: docker-build ## Build and push production image to registry
	@echo "Tagging image: $(REGISTRY_ENDPOINT)/$(DOCKER_IMAGE)..."
	@docker tag $(DOCKER_IMAGE) $(REGISTRY_ENDPOINT)/$(DOCKER_IMAGE)
	@echo "Pushing to registry..."
	@docker push $(REGISTRY_ENDPOINT)/$(DOCKER_IMAGE)
	@echo "✅ Pushed: $(REGISTRY_ENDPOINT)/$(DOCKER_IMAGE)"

docker-load-k3d: docker-build-dev ## Build dev image and load into k3d cluster
	@echo "Loading Docker image into k3d cluster $(K3D_CLUSTER_NAME)..."
	@k3d image import $(DOCKER_IMAGE) --cluster $(K3D_CLUSTER_NAME)
	@echo "✅ Image $(DOCKER_IMAGE) loaded into k3d cluster"

docker-clean: ## Clean Docker image
	@echo "Cleaning Docker image..."
	@docker rmi $(DOCKER_IMAGE) || true
	@docker rmi $(REGISTRY_ENDPOINT)/$(DOCKER_IMAGE) || true
