# Agent Manager Console

React/TypeScript web application for the Agent Manager platform, built as a Rush monorepo.

## Tech Stack

- **React 19** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool and dev server
- **Rush** - Monorepo management
- **pnpm** - Package manager

## Prerequisites

Before you begin, ensure you have the following installed:

- **Node.js**: Version 18.20.3+ or 20.14.0+ (see supported versions in rush.json)
- **Rush**: The monorepo management tool
- **pnpm**: Package manager (installed automatically by Rush)

### Installing Rush

Install Rush globally:

```bash
npm install -g @microsoft/rush
```

Verify installation:
```bash
rush --version
```

## Getting Started

### 1. Install Dependencies

From the `console/` directory, install all dependencies for the monorepo:

```bash
cd console
make install
```

This command will:
- Install Rush's local copy of pnpm
- Install all dependencies for all projects in the monorepo
- Create symlinks between local packages

### 2. Build Libraries

Build all shared libraries first:

```bash
make build-webapp
```

Or build all projects:
```bash
make build
```

### 3. Start Development Server

```bash
make dev
```

This will:
- Start all library dependencies in watch mode
- Launch the Vite dev server at `http://localhost:3000`
- Automatically rebuild dependencies when you make changes
- Hot-reload the webapp when dependencies update

Press `Ctrl+C` to stop all processes.

### 4. Environment Configuration

Copy the configuration template and customize it:

```bash
cp apps/webapp/public/config.js.template apps/webapp/public/config.js
```

Edit `apps/webapp/public/config.js` to set your API URL:

```javascript
window.APP_CONFIG = {
  API_URL: 'http://localhost:8080'
};
```

## Available Commands

### Make Commands (Recommended)

```bash
# Start development mode with hot-reload
make dev

# Install dependencies
make install

# Build all projects
make build

# Clean build outputs
make clean

# Purge Rush cache
make purge

# Show all available commands
make help
```

### Rush Commands

```bash
# Install dependencies
rush install

# Build all projects
rush build

# Build specific project and its dependencies
rush build --to @agent-management-platform/webapp

# Run linting for all projects
rush lint

# Run tests for all projects
rush test

# Clean all build outputs
rush purge

# Update dependencies
rush update

```

### Project-Specific Commands

Navigate to any project directory and use `rushx`:

```bash
cd apps/webapp

# Start development server
rushx dev

# Build for production
rushx build

# Run linting
rushx lint

# Fix linting issues
rushx lint:fix

# Preview production build
rushx preview
```

## Project Structure Details

### Apps
- **webapp**: Main React application with Vite build system

### Libraries
- **auth**: Authentication provider and hooks
- **types**: Shared TypeScript type definitions
- **eslint-config**: Shared ESLint configuration
- **views**: Shared UI components and themes
- **api-client**: API client utilities

### Pages
- **AgentsListPage**: Example page component (use as reference)

