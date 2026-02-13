.PHONY: help docker-build docker-build-dev docker-load-k3d docker-clean

# Variables
TAG ?= 0.0.0-dev
AMP_EVALUATION_VERSION ?= 0.0.1
APP_NAME = amp-evaluation-monitor
DOCKER_IMAGE = $(APP_NAME):$(TAG)
OPEN_CHOREO_VERSION ?= 0.14.0
K3D_CLUSTER_NAME ?= openchoreo-local-v$(OPEN_CHOREO_VERSION)

# Project root for dev build context
PROJECT_ROOT = ..

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Docker commands
docker-build: ## Build production Docker image (installs amp-evaluation from PyPI)
	@echo "Building production Docker image $(DOCKER_IMAGE)..."
	@docker build -t $(DOCKER_IMAGE) \
		--build-arg AMP_EVALUATION_VERSION=$(AMP_EVALUATION_VERSION) \
		-f Dockerfile .

docker-build-dev: ## Build development Docker image (installs amp-evaluation from local libs)
	@echo "Building development Docker image $(DOCKER_IMAGE)..."
	@docker build -t $(DOCKER_IMAGE) -f Dockerfile.dev $(PROJECT_ROOT)

docker-load-k3d: docker-build-dev ## Build dev image and load into k3d cluster
	@echo "Loading Docker image into k3d cluster $(K3D_CLUSTER_NAME)..."
	@k3d image import $(DOCKER_IMAGE) --cluster $(K3D_CLUSTER_NAME)
	@echo "Image $(DOCKER_IMAGE) loaded into k3d cluster"

docker-clean: ## Clean Docker image
	@echo "Cleaning Docker image..."
	@docker rmi $(DOCKER_IMAGE) || true
