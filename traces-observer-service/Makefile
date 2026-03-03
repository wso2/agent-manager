.PHONY: help build run stop clean test docker-build docker-run docker-stop docker-clean mock-traces

# Variables
TAG=0.0.0-dev
APP_NAME=amp-traces-observer
DOCKER_IMAGE=$(APP_NAME):$(TAG)
CONTAINER_NAME=$(APP_NAME)
PORT=9098
KIND_CLUSTER_NAME=openchoreo-local
OPEN_CHOREO_VERSION=0.14.0
K3D_CLUSTER_NAME=openchoreo-local-v$(OPEN_CHOREO_VERSION)

# Mock trace generation configuration.
# Defaults are set here; override any value on the command line:
#   make mock-traces SPANS_PER_TRACE=5 TRACE_COUNT=20
OTLP_ENDPOINT ?= localhost:22893
OTLP_HTTP_PATH ?= /otel/v1/traces
TRACE_SERVICE_NAME ?= mock-trace-generator
TRACE_COUNT ?= 10
SPANS_PER_TRACE ?= 10
TRACE_WORKERS ?= 1
TRACE_RATE ?= 1
TRACE_DURATION ?= 0s
OTLP_INSECURE ?= true

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

mock-traces: ## Generate mock traces using Makefile-configured values
	@command -v telemetrygen >/dev/null 2>&1 || (echo "telemetrygen is required. Install with: go install github.com/open-telemetry/opentelemetry-collector-contrib/cmd/telemetrygen@latest" && exit 1)
	@[ -n "$(OTLP_ENDPOINT)" ] || (echo "OTLP_ENDPOINT is required (host:port)" && exit 1)
	@[ -n "$(OTLP_HTTP_PATH)" ] || (echo "OTLP_HTTP_PATH is required (for example: /otel/v1/traces)" && exit 1)
	@[ -n "$(AMP_AGENT_API_KEY)" ] || (echo "AMP_AGENT_API_KEY is required" && exit 1)
	@echo "$(SPANS_PER_TRACE)" | grep -Eq '^[1-9][0-9]*$$' || (echo "SPANS_PER_TRACE must be a positive integer" && exit 1)
	@echo "$(TRACE_COUNT)" | grep -Eq '^[1-9][0-9]*$$' || (echo "TRACE_COUNT must be a positive integer" && exit 1)
	@CHILD_SPANS=$$(( $(SPANS_PER_TRACE) - 1 )); \
	INSECURE_FLAG=""; \
	case "$(OTLP_INSECURE)" in true|TRUE|True|1|yes|YES|on|ON) INSECURE_FLAG="--otlp-insecure" ;; esac; \
	echo "Generating mock traces..."; \
	echo "  endpoint: $(OTLP_ENDPOINT)"; \
	echo "  path: $(OTLP_HTTP_PATH)"; \
	echo "  traces: $(TRACE_COUNT)"; \
	echo "  spans_per_trace: $(SPANS_PER_TRACE)"; \
	telemetrygen traces \
	  --otlp-http \
	  --otlp-endpoint "$(OTLP_ENDPOINT)" \
	  --otlp-http-url-path "$(OTLP_HTTP_PATH)" \
	  --service "$(TRACE_SERVICE_NAME)" \
	  --traces "$(TRACE_COUNT)" \
	  --child-spans "$$CHILD_SPANS" \
	  --workers "$(TRACE_WORKERS)" \
	  --rate "$(TRACE_RATE)" \
	  --duration "$(TRACE_DURATION)" \
	  $$INSECURE_FLAG \
	  --otlp-header "x-amp-api-key=\"$(AMP_AGENT_API_KEY)\""
