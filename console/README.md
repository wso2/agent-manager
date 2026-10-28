# Agent Manager Console

React/TypeScript web application for the Agent Manager platform, built as a pnpm workspaces monorepo.

## Tech Stack

- **React 19** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool and dev server
- **pnpm** - Package manager

## Prerequisites

Before you begin, ensure you have the following installed:

- **Node.js**: Version 20.19.0+ or 22.12.0+ (see engines in package.json)
- **pnpm**: Package manager, pinned via `packageManager` in package.json and activated through Corepack

### Installing pnpm

Enable Corepack (ships with Node.js):

```bash
corepack enable
```

Verify installation:
```bash
pnpm --version
```

Corepack activates the pinned pnpm 9.12.3 automatically based on the `packageManager` field in `console/package.json`.

## Getting Started

### 1. Install Dependencies

From the `console/` directory, install all dependencies for the monorepo:

```bash
cd console
make install
```

This command will:
- Install all dependencies for all projects in the monorepo via pnpm
- Create symlinks between local packages

### 2. Start Development Server

```bash
make dev   # or: pnpm dev
```

This will:
- Launch the Vite dev server at `http://localhost:3000`, serving the runtime `config.js`
  (see [Environment Configuration](#3-environment-configuration))
- Hot-reload on edits to any workspace package, not just the app

From the repo root, `make console` starts the same server (`make console BACKGROUND=true`
detaches it, logging to `.make/console.log`; stop it with `make stop-console`). `make setup`
starts it in the background for you.

No separate build step is needed for development — Vite resolves
`@agent-management-platform/*` imports straight to each package's `src/`, so there are
no library watchers to run. Run `make build` only to produce production output.

### 3. Environment Configuration

The dev server serves `config.js` by filling `apps/web-ui/public/config.template.js`
from `apps/web-ui/.env.development`, which holds the defaults for the local stack.
Override any value with an env var, or in a gitignored `.env.development.local`:

```bash
DISABLE_AUTH=true pnpm dev
```

Because the process env wins, a generic variable already exported in your shell
(`TLS_ENABLED`, `AMP_VERSION`, `API_BASE_URL`, `BASE_PATH`, ...) also changes the dev
config. If `config.js` shows a value you didn't expect, check `env` first.

Production builds don't use this: the Docker image renders `config.js` from the same
template when nginx starts.

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

# Remove node_modules and all caches
make purge

# Show all available commands
make help
```

### pnpm Commands

The `make` targets wrap these. Reach for them directly when you need a filter:

```bash
pnpm build                                        # all packages
pnpm build:core-ui                                # core-ui and its dependencies
pnpm -r --filter <package-name>... run build      # any package and its dependencies
pnpm lint                                         # eslint across packages
pnpm lint:fix                                     # eslint --fix across packages
```

### Project-Specific Commands

Navigate to any project directory and use `pnpm run`:

```bash
cd apps/web-ui

# Start development server
pnpm run dev

# Build for production
pnpm run build

# Run linting
pnpm run lint

# Fix linting issues
pnpm run lint:fix

# Preview production build
pnpm run preview
```

## Project Structure Details

See [`AGENTS.md`](AGENTS.md) for the package map — every workspace, its package name, and what it holds.

