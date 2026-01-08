.PHONY: help install build dev clean purge

# Default target
help:
	@echo "Console Development Commands:"
	@echo ""
	@echo "  make install     - Install all dependencies"
	@echo "  make build       - Build all projects"
	@echo "  make dev         - Start development mode with hot-reload"
	@echo "  make clean       - Clean build outputs"
	@echo "  make purge       - Purge all Rush build cache"
	@echo ""

# Install dependencies
install:
	@echo "Installing dependencies..."
	@rush install

# Build all projects
build:
	@echo "Building all projects..."
	@rush build

build-webapp:
	@echo "Building webapp..."
	@rush build --to @agent-management-platform/webapp

# Start development with local-dev.sh script
dev:
	@echo "Starting development mode..."
	@./local-dev.sh

# Clean build outputs
clean:
	@echo "Cleaning build outputs..."
	@find workspaces -name "dist" -type d -exec rm -rf {} + 2>/dev/null || true
	@find apps -name "dist" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "Done!"

# Purge Rush cache and build outputs
purge:
	@echo "Purging Rush cache..."
	@rush purge
