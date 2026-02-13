.PHONY: help install start build serve clean version 

# Default target
help:
	@echo "WSO2 Agent Manager Documentation - Available Commands"
	@echo ""
	@echo "Development:"
	@echo "  make install          - Install dependencies"
	@echo "  make start            - Start development server"
	@echo "  make build            - Build static site"
	@echo "  make serve            - Serve built site locally"
	@echo "  make clean            - Clean build artifacts"
	@echo ""
	@echo "Versioning:"
	@echo "  make version VERSION=v0.4.x        - Create new documentation version"

# Install dependencies
install:
	npm install

# Start development server
start:
	npm run start

# Build the site
build:
	npm run build

# Serve the built site
serve:
	npm run serve

# Clean build artifacts
clean:
	rm -rf build .docusaurus

# Create a new documentation version
version:
ifndef VERSION
	@echo "Error: VERSION is required"
	@echo "Usage: make version VERSION=v0.4.x"
	@exit 1
endif
	@echo "Creating documentation version $(VERSION)..."
	@if grep -q "\"$(VERSION)\"" versions.json 2>/dev/null; then \
		echo "⚠ Version $(VERSION) already exists"; \
		exit 1; \
	else \
		npm run docusaurus docs:version $(VERSION); \
		sed -i '' "s/latestVersion: '.*'/latestVersion: '$(VERSION)'/" docs/_constants.md; \
		echo "✓ Version $(VERSION) created successfully!"; \
		echo "✓ Updated docs/_constants.md with latestVersion: $(VERSION)"; \
	fi
