# agent-manager — agent guide

WSO2 Agent Manager: an enterprise control plane to register, govern and observe AI agents. This branch is AMP Lite: no OpenChoreo, external agents only, run with Docker Compose. The API is one binary with a module host, built as `amp` (core and modules, including the AI Gateway) or `amp-core` (core only).

This repo is a **multi-aspect monorepo**. Each aspect has its own `AGENTS.md` with the concrete patterns, commands, and gotchas for that stack — **read the aspect guide before working in it**. This root guide is the map and the cross-cutting rules.

## Aspect map

| Aspect | Dir | Stack | Guide |
|---|---|---|---|
| **Control-plane API** | `agent-manager-service/` | Go 1.26, net/http (stdlib), GORM (SQLite or PostgreSQL), wire, oapi-codegen; modules on `modkit` | [`agent-manager-service/AGENTS.md`](agent-manager-service/AGENTS.md) |
| **Web console** | `console/` | React 19, TS, Vite, pnpm workspaces, Oxygen UI (MUI 7), TanStack Query | [`console/AGENTS.md`](console/AGENTS.md) |
| **Oxygen UI library** | `console/.ai/oxygen-ui/` | WSO2 React component lib | [`console/.ai/oxygen-ui/AGENTS.md`](console/.ai/oxygen-ui/AGENTS.md) |
| **CLI (`amctl`)** | `cli/` | Go, cobra-style, factory-injected deps | [`cli/AGENTS.md`](cli/AGENTS.md) |
| **Trace/observability API** (not in the Lite stack) | `agent-manager-observer/` | Go, stdlib `net/http`, `slog` | [`agent-manager-observer/AGENTS.md`](agent-manager-observer/AGENTS.md) |
| **Instrumentation lib** | `libs/amp-instrumentation/` | Python, OpenTelemetry / Traceloop | [`libs/amp-instrumentation/AGENTS.md`](libs/amp-instrumentation/AGENTS.md) |
| **Evaluation lib** | `libs/amp-evaluation/` | Python, Pydantic v2 | [`libs/amp-evaluation/AGENTS.md`](libs/amp-evaluation/AGENTS.md) |
| **Evaluation job** | `evaluation-job/` | Python (Dockerized Argo job) | [`evaluation-job/AGENTS.md`](evaluation-job/AGENTS.md) |
| **E2E tests** | `test/e2e/` | Go, Ginkgo + Gomega | [`test/e2e/AGENTS.md`](test/e2e/AGENTS.md) |

Other dirs: `deployments/` (Docker Compose stack in `deployments/standalone`), `samples/` (example agents), `scripts/` (dev tooling). These have no dedicated guide yet. The documentation site lives in a separate repository, [wso2/docs-agent-platform](https://github.com/wso2/docs-agent-platform).

## Task skills

Repeatable, error-prone procedures are captured as Claude Code skills in `.claude/skills/` (committed, team-shared). They encode the step-by-step recipe and link back to the relevant aspect guide for the "why":

| Skill | Use when |
|---|---|
| `add-api-resource` | Adding/changing a REST endpoint in `agent-manager-service` (spec-first → codegen → RBAC → layers) |
| `add-service-unit-test` | Writing a Go service unit test (mocks, CI lint traps) |
| `add-audit-event` | Recording a security-critical operation in the audit trail, or fixing a route the audit policy cannot label |
| `add-console-api-feature` | Wiring a new backend call into the console (two-file `apis/`+`hooks/` pattern) |
| `add-evaluator` | Adding an evaluator to `amp-evaluation` (type-hint-driven level/mode) |

## How the aspects fit together

```text
console ──HTTP──▶ agent-manager-service ──▶ SQLite or PostgreSQL / ThunderID / AI Gateway
                           │
                           └─ registers external agents; their LLM, MCP and A2A calls
                              go through the AI Gateway
cli (amctl) ──HTTP──▶ agent-manager-service   (same API as the console)
```

- **`agent-manager-service`** is the hub — the REST and MCP API the console and CLI both call. It also serves the built console on its own port. Its core imports no module; the AI Gateway is the `ai-gateway` module, which `amp` installs by default (see [Writing a module](docs/amp-lite/modules.md)).
- **Not in the Lite stack:** the stack stores no traces, so `agent-manager-observer` does not run and agents instrumented with `libs/amp-instrumentation` have nowhere to send spans; `evaluation-job` and `amp-evaluation` are neither built nor run. Their source stays for the observability and evaluation modules planned to replace them.

## Local dev environment (from repo root)

```bash
make tools        # pinned Go dev tools (codegen, linters, ginkgo) into .tools/bin; versions in tools.mk
make setup        # first-time: tools, then dev-up
make dev-up       # start the Compose stack with the API under air, and the console (Vite, background, :3000)
make dev-down     # stop them
make dev-logs     # tail the stack's logs
make up           # the stack as a release runs it; console on the API port :9000
make up-core      # the same with amp-core (no modules, no AI Gateway)
make dev-rebuild  # full image rebuild
make console      # (re)start the console dev server; BACKGROUND=true to detach
make stop-console # stop the console dev server
make console-logs # tail the background console's log (.make/console.log)
make dev-migrate  # run DB migrations
make e2e-test     # run the e2e suite against the local stack
```

Run `make help` at the root for the full target list (DB access, smoke tests, codegen helpers, etc.).

## Cross-cutting engineering rules

These apply everywhere; each aspect guide restates the specifics for its stack.

- **Security** — never hardcode secrets (empty defaults that fail at startup). Every route/operation enforces its own authorization; validate the caller's org/tenant against the target resource (defense in depth), don't rely on a shared middleware wildcard.
- **Error handling** — distinguish "not found" from real errors; never flatten an unexpected error into not-found and never silently fall back to a default on a real error. Wrap with context (`%w` in Go).
- **Context propagation** — every I/O method takes and propagates `context.Context` (Go) / passes cancellation through (Python jobs).
- **Concurrency** — never hold a lock across I/O; use atomic upserts, not read-then-write; serialize expensive per-key side effects, not globally.
- **Config** — validate at startup, not first use; check co-dependent values together.
- **Observability** — log with correlation context (org, resource ID, request ID); hot paths at Debug, rare events at Info, destructive ops at Error.
- **Audit trail** — in `agent-manager-service`, every mutating route and MCP tool is audited automatically and a test fails the build if one is not. Operations touching credentials, privileges, membership, deployment or deletion additionally record *what changed* — see the `add-audit-event` skill. Never put a request or response body, or any secret value, into a record. Design and open questions: [discussion #1500](https://github.com/wso2/agent-manager/discussions/1500).
- **Generated code is never hand-edited** — regenerate and commit (OpenAPI types + wire + mocks in the Go service; `dist/` in the console). See the aspect guides for the exact commands.

## Releases

- **Platform** — `.github/workflows/release.yml` (and `release-promote.yml` to promote an existing release): images and the Compose bundle, tagged `amp/v<version>`
- **amctl** — its own version line via `.github/workflows/amctl_release.yaml`, tagged `amctl/v<version>`. The platform release does not ship amctl; see [`cli/AGENTS.md`](cli/AGENTS.md#releases-and-updates)
- **Python libraries** — `amp_instrumentation_release.yaml`

## Contributing

Design proposals and issue/PR workflow (feature lifecycle, proposal labels) are in [`CONTRIBUTING.md`](CONTRIBUTING.md). Security vulnerabilities → security@wso2.com (do not file a public issue).
