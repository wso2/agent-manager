# AMP Lite developer guide

How to run AMP Lite from source, change it and test it. To install and use a released version, see the [user guide](user-guide.md).

## Repository layout

| Folder | What it holds |
|---|---|
| [agent-manager-service](../agent-manager-service) | Agent Manager API, MCP server, module host and embedded console (Go). Builds `amp` (core and modules) and `amp-core` (core only); `amp init` generates the ThunderID and gateway configuration |
| [agent-manager-observer](../agent-manager-observer) | Trace query API over OpenSearch (Go); not part of the Lite stack until traces return as a module |
| [console](../console) | Web console (React, TypeScript, pnpm workspace) |
| [cli](../cli) | `amctl`, the command-line client (Go) |
| [libs/amp-instrumentation](../libs/amp-instrumentation) | Python auto-instrumentation for agents |
| [deployments/standalone](../deployments/standalone) | The Docker Compose stack, which is also the release bundle |
| [test/e2e](../test/e2e) | End-to-end and live security tests (Ginkgo) |
| [samples](../samples) | Example agents to run as external agents |
| [docs/amp-lite](amp-lite) | Architecture, database schema and data model, with their generators |

The [architecture diagram](amp-lite/architecture.png) shows how the services fit together. [deployments/standalone/README.md](../deployments/standalone/README.md) explains the startup order.

## Prerequisites

- Docker with Compose v2. On macOS, `make setup-colima` starts a Colima VM named `amp-lite` with 2 CPUs and 4 GB of memory (set `COLIMA_CPU` and `COLIMA_MEMORY` to change them). Docker Desktop also works.
- Go 1.26 (see `agent-manager-service/go.mod`).
- Node.js 22 or later and pnpm 9 for the console.
- Python 3.11 or later, only to work on the Python libraries or run the samples.
- Free ports 3000, 8080, 9000, 9243 and 19080.

## Start the stack

```bash
make setup     # once: installs the pinned Go tools into .tools/bin, then runs make dev-up
make dev-up    # every day after that
```

`make dev-up` starts the Compose stack with `docker-compose.dev.yml` on top:

- The API runs from your checkout under air and reloads when you save a Go file. It migrates the database on every start.
- The console runs as a Vite dev server on http://localhost:3000.
- Everything else (ThunderID, gateway) runs from images.

Sign in at http://localhost:3000 as `admin`. The password is `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`, which is created from `.env.example` on the first run.

Other ways to run it:

| Command | What it does |
|---|---|
| `make up` | Builds every image from this checkout and runs the stack as a release would, with the console served by the API on :9000 |
| `make up-core` | The same with the `amp-core` binary: no modules, no AI Gateway (`down-core`, `reset-core` to stop it) |
| `make smoke` / `make smoke-core` | Smoke-tests the running stack (`deployments/standalone/scripts/smoke.sh`) |
| `make up DB=postgres` (also `dev-up`, `reset`) | Uses PostgreSQL instead of SQLite |
| `make dev-down` / `make down` | Stops the stack and keeps its data |
| `make reset` / `make teardown` | Stops the stack and deletes all its data |
| `make dev-rebuild` | Rebuilds the images and restarts |
| `make thunder-reset` | Wipes ThunderID's data and sets it up again |
| `make help` | Lists everything else |

## Day-to-day commands

| Task | Command |
|---|---|
| API logs | `make service-logs` |
| All stack logs | `make dev-logs` |
| Shell in the API container | `make service-shell` |
| Open the database | `make db-connect` (add `DB=postgres` for PostgreSQL) |
| Restart the console | `make console` (`BACKGROUND=true` to detach) |
| Get an API token | `curl -X POST http://thunder.amp.localhost:8080/oauth2/token -u amp-api-client:<AMP_API_CLIENT_SECRET> -d grant_type=client_credentials -d "scope=<the amp:* scopes you need>"`. ThunderID only puts requested scopes in the token |

If the API looks up but serves old code, check `make service-logs`: air keeps the container running even when the build or the process fails.

## Changing the code

### Agent Manager API

Run these in `agent-manager-service`:

| When you change | Run |
|---|---|
| Any Go code | `make fmt` (gofumpt; CI rejects unformatted code) and `make lint` |
| The OpenAPI spec | `make spec` |
| Wiring in `wiring/wire.go` or `modules/aigateway/gwwire/wire.go` | `make wire` (regenerates both) |
| A repository or client interface | `make mocks` (needs moq v0.5.3 from `make tools`) |
| The database schema in `db_migrations/schema/lite.yaml`, or a module's in `modules/<name>/schema/` | `make schema`, which regenerates the SQLite and PostgreSQL migrations |
| Anything you want to run outside Compose | `make build` (`bin/amp`, `bin/amp-core`; run `make console-embed` first to include the console) |

CI fails if generated code is out of date (`make codegenfmt-check`). Commit the generated files with your change.

Every route needs an RBAC permission, and operations on credentials, permissions or deletions need an audit event. The [database schema](amp-lite/database-schema.md) explains the tables and the rules they follow.

Settings come from the environment, with an optional `amp.yaml` for the rest; [`amp.example.yaml`](../agent-manager-service/amp.example.yaml) lists the common ones.

### Modules

The core (organizations, projects, agents, identities, tokens, events, RBAC and audit) imports no module; everything else, including the AI Gateway, is a module under `agent-manager-service/modules/`. [Writing a module](amp-lite/modules.md) covers the split, the contract, testing and operation; to add one, follow [Add a module, step by step](amp-lite/modules.md#add-a-module-step-by-step). A module imports only `modkit`, the domain-free libraries under `agent-manager-service/pkg/` and its own packages; [Module boundaries](amp-lite/modules.md#module-boundaries) gives the rules, and core code that both sides need goes to `pkg/` only when it has no core imports, no global state and no domain. Run `make up-core` and `make smoke-core` as well as `make up` when a change touches the line between the core and a module.

### Console

Run these in `console`:

```bash
pnpm install
pnpm lint
pnpm build
```

[console/UX_GUIDELINES.md](../console/UX_GUIDELINES.md) covers page layout and wording. Components come from Oxygen UI. Pages belong to UI modules; which ones show is decided by the API (`GET /api/v1/ui-manifest`), so a new page is also listed in its backend module's manifest ([console/AGENTS.md](../console/AGENTS.md)).

### Observer and CLI

Both are plain Go modules. Run `go build ./... && go test ./...` in `agent-manager-observer` or `cli`. `make amctl-build` builds the CLI.

### Diagrams

The architecture and data-model diagrams are generated. Edit the generators in [docs/amp-lite/src](amp-lite/src) and run `docs/amp-lite/src/render.sh` (it needs Python 3 and Google Chrome). It also regenerates the ER diagram in `agent-manager-service/docs/database-er.md` from `lite.yaml`, so run it after a schema change.

## Tests

| Tests | Command | Needs |
|---|---|---|
| API unit tests | `make test-unit` in `agent-manager-service` | Nothing |
| API integration tests | `make test-integration` in `agent-manager-service` | Nothing; they run on a throwaway SQLite database |
| One test | `make test-run RUN=<pattern> PKG=./services/` | Nothing |
| Route authorization checks | `make security-test-static` | Nothing |
| End-to-end tests | `make e2e-test` (`SUITE=<name>` or `FOCUS=<regex>` to narrow) | A running stack; copy `test/e2e/.env.example` to `test/e2e/.env` first |
| Live security tests | `make security-test-live` | A running stack |

Tests that call real model providers need provider keys in `test/e2e/.env`. Without them those specs are skipped or fail at the first model call.

To try a change the way a user would, register one of the [samples](../samples) as an external agent, attach an LLM provider to it in the console, and run the sample with that provider's connection settings, so its model calls go through the AI Gateway. For the IT Helpdesk sample that is `USE_LLM_PROVIDER=true` with `LLM_PROVIDER_URL` and `LLM_PROVIDER_KEY` from the agent's LLM configuration.

## Commits and pull requests

- Write commit subjects in the imperative mood, under 50 characters, with a body that explains why. Don't use `type(scope):` prefixes.
- Fill in every section of the [pull request template](../pull_request_template.md).
- [CONTRIBUTING.md](../CONTRIBUTING.md) covers issues, discussions and design proposals.
