.PHONY: all help install-dev lint format-check format type-check test test-engine lock check-lock check-deps licenses check-wheels docker-build docker-smoke docker-buildx clean

# Variables
VERSION ?= 0.0.0-dev
APP_NAME = amp-shadow-runner
DOCKER_IMAGE = $(APP_NAME):$(VERSION)
PLATFORM ?= linux/amd64
PYTHON ?= python
LOCK = locks/requirements-linux-py312.txt
WHEEL_DIR = build/wheels
# uv venvs have no pip; use uv when it is installed.
PIP ?= $(shell command -v uv >/dev/null 2>&1 && echo "uv pip" || echo "$(PYTHON) -m pip")

all: help ## Default target

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-25s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install-dev: ## Install the hash-checked lock, then the package and dev tools, into the active venv
	@$(PIP) install --require-hashes --no-deps -r $(LOCK)
	@$(PIP) install -e '.[dev]'

lint: ## Run ruff linter
	@echo "Running ruff linter..."
	@ruff check .
	@echo "Lint passed."

format-check: ## Check code formatting with ruff
	@echo "Checking formatting..."
	@ruff format --check .
	@echo "Format check passed."

format: ## Format code and apply safe lint fixes
	@ruff format .
	@ruff check --fix .

type-check: ## Run mypy (strict) on src, tests and scripts
	@echo "Running mypy..."
	@mypy
	@echo "Type check passed."

test: ## Run all unit tests
	@echo "Running tests..."
	@$(PYTHON) -m pytest
	@echo "Tests passed."

test-engine: ## Run the engine tests only (grammars, purity)
	@$(PYTHON) -m pytest tests/engine

lock: ## Regenerate the hash-pinned lock from requirements.in (needs uv and PyPI)
	@uv pip compile --quiet requirements.in --universal --python-version 3.12 --generate-hashes \
		--custom-compile-command "make lock" --output-file $(LOCK)
	@echo "Lock written to $(LOCK). Run 'make install-dev licenses' to refresh the notice file."

check-lock: ## Verify the lock matches requirements.in and pyproject.toml (hermetic; no PyPI)
	@$(PYTHON) scripts/check_lock.py

check-deps: ## Check runtime licences, the denylist and the notice file (install the lock first)
	@$(PYTHON) scripts/check_deps.py

licenses: ## Regenerate licenses/THIRD_PARTY_LICENSES.md from the installed lock
	@$(PYTHON) scripts/check_deps.py --write

check-wheels: ## Check every locked package has a binary wheel for linux amd64 and arm64 (needs pip and PyPI)
	@for platform in manylinux2014_x86_64 manylinux2014_aarch64; do \
		echo "Checking wheels for $$platform..."; \
		$(PYTHON) -m pip download --quiet --no-deps --only-binary=:all: --implementation cp \
			--python-version 3.12 --platform $$platform -r $(LOCK) -d $(WHEEL_DIR)/$$platform || exit 1; \
	done
	@echo "Wheels available for both architectures."

docker-build: ## Build the image for PLATFORM (default linux/amd64)
	@docker build --platform $(PLATFORM) --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE) .
	@echo "Built: $(DOCKER_IMAGE)"

docker-smoke: ## Smoke-test a built image (version, help, grammars, scan checks, read-only, non-root)
	@bash scripts/smoke-image.sh $(DOCKER_IMAGE) $(VERSION)

docker-buildx: ## Build (without loading) for linux/amd64 and linux/arm64
	@docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE) .

clean: ## Remove build and test artifacts
	@rm -rf build dist htmlcov .coverage .pytest_cache .mypy_cache .ruff_cache
