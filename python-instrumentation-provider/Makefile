.PHONY: help build docker-load-k3d

# Variables
TAG ?= dev-python3.11
APP_NAME = ghcr.io/wso2/amp-python-instrumentation-provider
DOCKER_IMAGE = $(APP_NAME):$(TAG)
OPEN_CHOREO_VERSION ?= 0.14.0
K3D_CLUSTER_NAME = openchoreo-local-v$(OPEN_CHOREO_VERSION)
KIND_CLUSTER_NAME = openchoreo-local

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build Docker image
	@echo "Building $(DOCKER_IMAGE)..."
	@docker build --platform linux/amd64 -t $(DOCKER_IMAGE) .

docker-load-k3d: build ## Build and load image into k3d cluster
	@echo "Loading $(DOCKER_IMAGE) into k3d cluster $(K3D_CLUSTER_NAME)..."
	@k3d image import $(DOCKER_IMAGE) --cluster $(K3D_CLUSTER_NAME)
