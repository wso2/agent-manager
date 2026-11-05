.PHONY: help test lint docker-build docker-load-k3d

# Pinned dev tools (`make tools` at the repo root); puts ../../../.tools/bin first on PATH.
include $(dir $(lastword $(MAKEFILE_LIST)))../../../tools.mk

IMAGE ?= amp-observer-lite:dev
K3D_CLUSTER_NAME ?= openchoreo-local-setup

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests
	@go test ./...

lint: ## Lint code (golangci-lint from `make tools`)
	@golangci-lint run ./...

docker-build: ## Build the observer-lite image
	@docker build --build-context amp-observer=../../../agent-manager-observer -t $(IMAGE) .

docker-load-k3d: docker-build ## Import the image into the k3d cluster
	@k3d image import $(IMAGE) --cluster $(K3D_CLUSTER_NAME)
