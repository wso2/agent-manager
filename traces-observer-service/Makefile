.PHONY: help build run stop clean test docker-build docker-run docker-stop docker-clean

# Variables
TAG=0.0.0-dev
APP_NAME=amp-traces-observer
DOCKER_IMAGE=$(APP_NAME):$(TAG)
CONTAINER_NAME=$(APP_NAME)
PORT=9098
KIND_CLUSTER_NAME=openchoreo-local
K3D_CLUSTER_NAME=openchoreo-local-v0.7

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Local development
build: ## Build the Go application locally
	@echo "Building $(APP_NAME)..."
	@go build -o $(APP_NAME) .

run: ## Run the application locally
	@echo "Running $(APP_NAME)..."
	@go run main.go

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(APP_NAME)
	@go clean

# Docker commands
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE) .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run -d \
		--name $(CONTAINER_NAME) \
		-p $(PORT):$(PORT) \
		-e OPENSEARCH_ADDRESS="http://host.docker.internal:9200" \
		-e OPENSEARCH_USERNAME="admin" \
		-e OPENSEARCH_PASSWORD="admin" \
		-e OPENSEARCH_TRACE_INDEX="custom-otel-span-index" \
		$(DOCKER_IMAGE)
	@echo "Container started. Logs:"
	@docker logs -f $(CONTAINER_NAME)

docker-stop: ## Stop Docker container
	@echo "Stopping Docker container..."
	@docker stop $(CONTAINER_NAME) || true
	@docker rm $(CONTAINER_NAME) || true

docker-logs: ## View Docker container logs
	@docker logs -f $(CONTAINER_NAME)

docker-shell: ## Access Docker container shell
	@docker exec -it $(CONTAINER_NAME) sh

docker-clean: docker-stop ## Clean Docker resources
	@echo "Cleaning Docker resources..."
	@docker rmi $(DOCKER_IMAGE) || true

# Add to kind cluster
docker-load-kind: docker-build ## Load Docker image into kind cluster
	@echo "Loading Docker image into kind cluster..."
	@kind load docker-image $(DOCKER_IMAGE) --name ${KIND_CLUSTER_NAME}

docker-load-k3d: docker-build ## Load Docker image into kind cluster
	@echo "Loading Docker image into k3d cluster..."
	@k3d image import $(DOCKER_IMAGE) --cluster ${K3D_CLUSTER_NAME}
# Combined commands
dev: build run ## Build and run locally

docker-dev: docker-build docker-run ## Build and run with Docker

docker-restart: docker-stop docker-dev ## Restart Docker container

all: clean build ## Clean and build
