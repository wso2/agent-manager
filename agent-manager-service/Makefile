export GOBIN := $(abspath ./.bin)
export PATH := $(GOBIN):$(PATH)

PROJECT_ROOT := $(shell pwd)
GO_VERSION := 1.25

.PHONY: help
help: ## Display available commands.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: gen-keys
gen-keys: ## Generate RSA keys for JWT signing.
	@bash scripts/gen_keys.sh

.PHONY: run
run: gen-keys ## Run development server with hot-reloading.
	air -c .air.toml

.PHONY: wire
wire: ## Run wire code gen.
	@echo "Running wire code gen"
	@(cd wiring && wire)
	@echo "Wire code gen done"

.PHONY: codegen
codegen: ## Run code gen.
	@echo "Running go generate"
	@go generate -tags=wireinject ./...
	@echo "Go generate done"

.PHONY: fmt
fmt: ## Format code.
	sh scripts/fmt.sh

.PHONY: format
format: ## Format code (alias for fmt).
	sh scripts/fmt.sh

.PHONY: test
test: gen-keys ## Run tests.
	bash scripts/run_tests.sh

.PHONY: lint
lint: ## Lint code.
	bash scripts/lint.sh

.PHONY: spec
spec: ## Code gen type/client for openapi spec.
	sh scripts/gen_client.sh
	sh scripts/fmt.sh

.PHONY: gen-oc-client
gen-oc-client: ## Generate OpenChoreo API client from local OpenAPI spec.
	@echo "Generating OpenChoreo client from local spec"
	@oapi-codegen -config clients/openchoreosvc/gen/oapi-codegen.yaml clients/openchoreosvc/openchoreo-api.yaml
	@oapi-codegen -config clients/openchoreosvc/gen/oapi-codegen-client.yaml clients/openchoreosvc/openchoreo-api.yaml
	@$(MAKE) fmt
	@echo "OpenChoreo client generated successfully"

.PHONY: gen-observer-client
gen-observer-client: ## Generate Observer API client from OpenChoreo GitHub repo.
	@echo "Fetching Observer OpenAPI spec from GitHub"
	@curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.14/cmd/observer/openapi.yaml -o clients/observabilitysvc/observer-api.yaml
	@echo "Generating Observer client"
	@oapi-codegen -config clients/observabilitysvc/gen/oapi-codegen.yaml clients/observabilitysvc/observer-api.yaml
	@oapi-codegen -config clients/observabilitysvc/gen/oapi-codegen-client.yaml clients/observabilitysvc/observer-api.yaml
	@echo "Cleaning up downloaded spec"
	@rm clients/observabilitysvc/observer-api.yaml
	@$(MAKE) fmt
	@echo "Observer client generated successfully"

.PHONY: gen-gw-client
gen-gw-client: ## Generate OpenChoreo API client from upstream OpenAPI spec.
	@echo "Generating OpenChoreo client"
	@curl -sL "https://raw.githubusercontent.com/wso2/api-platform/refs/heads/main/gateway/gateway-controller/api/openapi.yaml" -o /tmp/gw-api.yaml
	@oapi-codegen -config clients/gatewaysvc/gen/oapi-codegen.yaml /tmp/gw-api.yaml
	@oapi-codegen -config clients/gatewaysvc/gen/oapi-codegen-client.yaml /tmp/gw-api.yaml
	@rm /tmp/gw-api.yaml
	@$(MAKE) fmt
	@echo "Gateway client generated successfully"

.PHONY: codegenfmt-check
codegenfmt-check:
	@$(MAKE) wire
	@$(MAKE) spec
	@$(MAKE) codegen
	@$(MAKE) fmt
	@go mod tidy
	@echo ""
	@if [ ! -z "$$(git status --porcelain -- '*.go')" ]; then \
	  git diff --color -- '*.go' | cat; \
	  git status --porcelain -- '*.go'; \
      echo "There are new changes to .go files after code generation & formatting."; \
	  echo "Please run 'make codegenfmt-check' locally and commit the changes."; \
      exit 1; \
    fi
