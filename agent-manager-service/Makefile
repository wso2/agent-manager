export GOBIN := $(abspath ./.bin)
export PATH := $(GOBIN):$(PATH)

# Pinned dev tools (`make tools` at the repo root); puts ../.tools/bin first on PATH.
include $(dir $(lastword $(MAKEFILE_LIST)))../tools.mk

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
	@# Requires the `moq` binary on PATH (mock generation for client + repository
	@# interfaces, see //go:generate directives). Install with (version pinned in README):
	@#   go install github.com/matryer/moq@v0.5.3
	@command -v moq >/dev/null 2>&1 || echo "WARNING: 'moq' not found on PATH; go generate will still run but mock (re)generation will fail. Install: go install github.com/matryer/moq@v0.5.3"
	@go generate -tags=wireinject ./...
	@echo "Go generate done"

MOQ_REQUIRED_VERSION := v0.5.3

.PHONY: mocks
mocks: ## Regenerate moq mocks only (runs every //go:generate moq directive; codegen also runs this plus wire). Usage: make mocks [PKG=./repositories/...]
	@command -v moq >/dev/null 2>&1 || { echo "ERROR: 'moq' not found on PATH. Install: make tools (repo root)"; exit 1; }
	@# `moq --version` prints "dev" for go-installed builds; read the module version instead.
	@moq_version="$$(go version -m "$$(command -v moq)" | awk '$$1 == "mod" {print $$3}')"; \
	if [ "$$moq_version" != "$(MOQ_REQUIRED_VERSION)" ]; then \
		echo "ERROR: 'moq' version $$moq_version found, but $(MOQ_REQUIRED_VERSION) is required (a different version reformats import ordering across every mock file it touches). Install: make tools (repo root)"; \
		exit 1; \
	fi
	@echo "Regenerating moq mocks in $(if $(PKG),$(PKG),$$(go list ./... | grep -v '/wiring$$'))"
	@go generate -tags=wireinject $(if $(PKG),$(PKG),$$(go list ./... | grep -v '/wiring$$'))
	@echo "Mock generation done"

.PHONY: fmt
fmt: ## Format code.
	sh scripts/fmt.sh

.PHONY: format
format: ## Format code (alias for fmt).
	sh scripts/fmt.sh

.PHONY: test-unit
test-unit: gen-keys ## Run fast unit tests (no database required).
	bash scripts/run_unit_tests.sh

.PHONY: test-integration
test-integration: gen-keys ## Run integration tests with an isolated test database (-tags=integration).
	bash scripts/run_tests_isolated.sh

.PHONY: test-run
test-run: gen-keys ## Run one test/pattern with the unit-tier config env vars set. Usage: make test-run RUN=TestAgentKindService PKG=./services/
	@bash scripts/run_single_test.sh "$(RUN)" "$(PKG)"

.PHONY: dev-test
dev-test: test-integration ## Alias for test-integration (isolated DB). Kept for backwards compatibility.

.PHONY: test
test: gen-keys ## Run all tests (unit + integration) against the main database. For development.
	bash scripts/run_tests.sh

.PHONY: test-coverage
test-coverage: gen-keys ## Run unit + integration tests and produce a merged HTML coverage report.
	bash scripts/run_coverage.sh

.PHONY: lint
lint: ## Lint code.
	bash scripts/lint.sh

.PHONY: spec
spec: ## Code gen type/client for openapi spec.
	sh scripts/gen_client.sh
	sh scripts/fmt.sh

# Pin the oapi-codegen CLI version. Must stay in lockstep with the
# `github.com/oapi-codegen/runtime` version in go.mod — mismatched versions
# emit code that drifts from what is already checked in (interface{} vs any,
# subtle param-handling changes, etc.). Bump both together.
OAPI_CODEGEN := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1

.PHONY: gen-oc-client
gen-oc-client: ## Generate OpenChoreo API client from upstream OpenAPI spec.
	@echo "Generating OpenChoreo client from upstream spec"
	@curl -sL "https://raw.githubusercontent.com/openchoreo/openchoreo/v1.3.1/openapi/openchoreo-api.yaml" -o /tmp/openchoreo-api.yaml
	@$(OAPI_CODEGEN) -config clients/openchoreosvc/gen/oapi-codegen.yaml /tmp/openchoreo-api.yaml
	@$(OAPI_CODEGEN) -config clients/openchoreosvc/gen/oapi-codegen-client.yaml /tmp/openchoreo-api.yaml
	@rm /tmp/openchoreo-api.yaml
	@$(MAKE) fmt
	@echo "OpenChoreo client generated successfully"

.PHONY: gen-api-platform-client
gen-api-platform-client: ## Generate API Platform API client from upstream OpenAPI spec.
	@echo "Generating Gateway client"
	@curl -sL "https://raw.githubusercontent.com/wso2/api-platform/refs/heads/main/platform-api/src/resources/openapi.yaml" -o /tmp/gw-api.yaml
	@$(OAPI_CODEGEN) -config clients/apiplatformsvc/gen/oapi-codegen.yaml /tmp/gw-api.yaml
	@$(OAPI_CODEGEN) -config clients/apiplatformsvc/gen/oapi-codegen-client.yaml /tmp/gw-api.yaml
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

.PHONY: gen-instrumentation-baseline
gen-instrumentation-baseline: ## Regenerate instrumentation/baseline.json from .github/release-config.json.
	GOFLAGS=-mod=mod go run ./scripts/gen_instrumentation_baseline.go

.PHONY: gen-evaluators-dev
gen-evaluators-dev: ## Generate builtin evaluators using local amp-evaluation SDK
	@echo "Generating builtin evaluators (dev mode)..."
	@bash scripts/generate-builtin-evaluators.sh --dev --output $(PROJECT_ROOT)/catalog/builtin_evaluators.go
	@echo "✅ Generated: catalog/builtin_evaluators.go"

.PHONY: gen-evaluators
gen-evaluators: ## Generate builtin evaluators using PyPI amp-evaluation SDK
	@echo "Generating builtin evaluators (prod mode)..."
	@if [ -z "$(AMP_EVALUATION_VERSION)" ]; then \
		echo "❌ AMP_EVALUATION_VERSION must be set"; \
		echo "   Usage: make gen-evaluators AMP_EVALUATION_VERSION=0.0.1"; \
		exit 1; \
	fi
	@bash scripts/generate-builtin-evaluators.sh --amp-eval-version $(AMP_EVALUATION_VERSION) --output $(PROJECT_ROOT)/catalog/builtin_evaluators.go
	@echo "✅ Generated: catalog/builtin_evaluators.go (using amp-evaluation $(AMP_EVALUATION_VERSION))"

.PHONY: dev-migrate
dev-migrate: ## Run database migrations
	@echo "🗄️  Running database migrations..."
	@ENV_FILE_PATH=.env go run -mod=readonly . -migrate -server=false
	@echo "✅ Migrations completed"
