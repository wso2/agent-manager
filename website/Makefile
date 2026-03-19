.PHONY: help install start build serve clean version update-version

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
	@echo "  make version VERSION=v0.5.x DOCKER_TAG=v0.5.0         - Create new documentation version"
	@echo "  make update-version VERSION=v0.5.x DOCKER_TAG=v0.5.0  - Recreate existing documentation version"

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
	@echo "Error: VERSION and DOCKER_TAG are required"
	@echo "Usage: make version VERSION=v0.5.x DOCKER_TAG=v0.5.0"
	@exit 1
endif
ifndef DOCKER_TAG
	@echo "Error: VERSION and DOCKER_TAG are required"
	@echo "Usage: make version VERSION=v0.5.x DOCKER_TAG=v0.5.0"
	@exit 1
endif
	@echo "Creating documentation version $(VERSION)..."
	@if grep -q "\"$(VERSION)\"" versions.json 2>/dev/null; then \
		echo "⚠ Version $(VERSION) already exists"; \
		exit 1; \
	else \
		npm run docusaurus docs:version $(VERSION); \
		sed -i '' "s/latestVersion: '.*'/latestVersion: '$(VERSION)'/" docs/_constants.md; \
		sed -i '' "s/quickStartDockerTag: '.*'/quickStartDockerTag: '$(DOCKER_TAG)'/" docs/_constants.md; \
		echo "✓ Version $(VERSION) created successfully!"; \
		echo "✓ Updated docs/_constants.md with latestVersion: $(VERSION) and quickStartDockerTag: $(DOCKER_TAG)"; \
	fi

# Recreate an existing documentation version (deletes and re-snapshots from current docs)
update-version:
ifndef VERSION
	@echo "Error: VERSION and DOCKER_TAG are required"
	@echo "Usage: make update-version VERSION=v0.5.x DOCKER_TAG=v0.5.0"
	@exit 1
endif
ifndef DOCKER_TAG
	@echo "Error: VERSION and DOCKER_TAG are required"
	@echo "Usage: make update-version VERSION=v0.5.x DOCKER_TAG=v0.5.0"
	@exit 1
endif
	@if ! grep -q "\"$(VERSION)\"" versions.json 2>/dev/null; then \
		echo "⚠ Version $(VERSION) does not exist. Use 'make version' to create it."; \
		exit 1; \
	fi
	@echo "Removing existing version $(VERSION)..."
	@rm -rf versioned_docs/version-$(VERSION) versioned_sidebars/version-$(VERSION)-sidebars.json
	@node -e "const v=require('./versions.json');v.splice(v.indexOf('$(VERSION)'),1);require('fs').writeFileSync('versions.json',JSON.stringify(v,null,2)+'\n')"
	@echo "Recreating version $(VERSION)..."
	@npm run docusaurus docs:version $(VERSION)
	@sed -i '' "s/latestVersion: '.*'/latestVersion: '$(VERSION)'/" docs/_constants.md
	@sed -i '' "s/quickStartDockerTag: '.*'/quickStartDockerTag: '$(DOCKER_TAG)'/" docs/_constants.md
	@echo "✓ Version $(VERSION) updated successfully!"
