export GOBIN := $(abspath ./.bin)
export PATH := $(GOBIN):$(PATH)

PROJECT_ROOT := $(shell pwd)
GO_VERSION := 1.25

.PHONY: help
help: ## Display available commands.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: run
run: ## Run development server with hot-reloading.
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
test: ## Run tests.
	bash scripts/run_tests.sh

.PHONY: lint
lint: ## Lint code.
	bash scripts/lint.sh

.PHONY: spec
spec: ## Code gen type/client for openapi spec.
	sh scripts/gen_client.sh
	sh scripts/fmt.sh

.PHONY: codegenfmt-check
codegenfmt-check:
	@$(MAKE) wire
	@$(MAKE) spec
	@$(MAKE) codegen
	@$(MAKE) fmt
	@go mod tidy
	@echo ""
	@if [ ! -z "$$(git status --porcelain)" ]; then \
	  git diff --color | cat; \
	  git status --porcelain; \
      echo "There are new changes after the code generation & formatting."; \
	  echo "Please run 'codegenfmt-check' locally and commit the changes."; \
      exit 1; \
    fi
