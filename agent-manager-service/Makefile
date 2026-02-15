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

.PHONY: dev-test
dev-test: gen-keys ## Run tests with isolated test database.
	bash scripts/run_tests_isolated.sh

.PHONY: test
test: gen-keys ## Run tests against main database (for development).
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
gen-observer-client: ## Generate Observer API client from local OpenAPI spec.
	@echo "Generating Observer client from local spec"
	@oapi-codegen -config clients/observabilitysvc/gen/oapi-codegen.yaml clients/observabilitysvc/observer-api.yaml
	@oapi-codegen -config clients/observabilitysvc/gen/oapi-codegen-client.yaml clients/observabilitysvc/observer-api.yaml
	@$(MAKE) fmt
	@echo "Observer client generated successfully"

.PHONY: gen-api-platform-client
gen-api-platform-client: ## Generate API Platform API client from upstream OpenAPI spec.
	@echo "Generating Gateway client"
	@curl -sL "https://raw.githubusercontent.com/wso2/api-platform/refs/heads/main/platform-api/src/resources/openapi.yaml" -o /tmp/gw-api.yaml
	@oapi-codegen -config clients/apiplatformsvc/gen/oapi-codegen.yaml /tmp/gw-api.yaml
	@oapi-codegen -config clients/apiplatformsvc/gen/oapi-codegen-client.yaml /tmp/gw-api.yaml
	@rm /tmp/gw-api.yaml
	@$(MAKE) fmt
	@echo "API Platform client generated successfully"

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

.PHONY: gen-builtin-evaluators-dev
gen-builtin-evaluators-dev: ## Generate builtin evaluators JSON using local amp-evaluation SDK
	@echo "Generating builtin evaluators (dev mode)..."
	@bash scripts/generate-builtin-evaluators.sh --dev --output $(PROJECT_ROOT)/data/builtin_evaluators.json
	@echo "✅ Generated: data/builtin_evaluators.json"

.PHONY: gen-builtin-evaluators
gen-builtin-evaluators: ## Generate builtin evaluators JSON using PyPI amp-evaluation SDK
	@echo "Generating builtin evaluators (prod mode)..."
	@if [ -z "$(AMP_EVALUATION_VERSION)" ]; then \
		echo "❌ AMP_EVALUATION_VERSION must be set"; \
		echo "   Usage: make gen-builtin-evaluators AMP_EVALUATION_VERSION=0.0.1"; \
		exit 1; \
	fi
	@bash scripts/generate-builtin-evaluators.sh --amp-eval-version $(AMP_EVALUATION_VERSION) --output $(PROJECT_ROOT)/data/builtin_evaluators.json
	@echo "✅ Generated: data/builtin_evaluators.json (using amp-evaluation $(AMP_EVALUATION_VERSION))"

.PHONY: dev-migrate
dev-migrate: gen-builtin-evaluators-dev ## Generate evaluators and run migrations
	@echo "🗄️  Running database migrations..."
	@go run -mod=readonly . -migrate -server=false
	@echo "✅ Migrations completed"
