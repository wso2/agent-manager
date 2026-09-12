# agent-manager — agent guide

WSO2 Agent Manager: an enterprise control plane to deploy, manage, govern, and observe AI agents at scale. Built on [OpenChoreo](https://github.com/openchoreo/openchoreo) for internal deployments and OpenTelemetry for observability.

This repo is a **multi-aspect monorepo**. Each aspect has its own `AGENTS.md` with the concrete patterns, commands, and gotchas for that stack — **read the aspect guide before working in it**. This root guide is the map and the cross-cutting rules.

## Aspect map

| Aspect | Dir | Stack | Guide |
|---|---|---|---|
| **Control-plane API** | `agent-manager-service/` | Go 1.25, net/http (stdlib), GORM/pgx, gRPC, wire, sqlc, oapi-codegen | [`agent-manager-service/AGENTS.md`](agent-manager-service/AGENTS.md) |
| **Web console** | `console/` | React 19, TS, Vite, pnpm workspaces, Oxygen UI (MUI 7), TanStack Query | [`console/AGENTS.md`](console/AGENTS.md) |
| **Oxygen UI library** | `console/.ai/oxygen-ui/` | WSO2 React component lib | [`console/.ai/oxygen-ui/AGENTS.md`](console/.ai/oxygen-ui/AGENTS.md) |
| **CLI (`amctl`)** | `cli/` | Go, cobra-style, factory-injected deps | [`cli/AGENTS.md`](cli/AGENTS.md) |
| **Trace/observability API** | `agent-manager-observer/` | Go, stdlib `net/http`, `slog` | [`agent-manager-observer/AGENTS.md`](agent-manager-observer/AGENTS.md) |
| **Instrumentation lib** | `libs/amp-instrumentation/` | Python, OpenTelemetry / Traceloop | [`libs/amp-instrumentation/AGENTS.md`](libs/amp-instrumentation/AGENTS.md) |
| **Evaluation lib** | `libs/amp-evaluation/` | Python, Pydantic v2 | [`libs/amp-evaluation/AGENTS.md`](libs/amp-evaluation/AGENTS.md) |
| **Evaluation job** | `evaluation-job/` | Python (Dockerized Argo job) | [`evaluation-job/AGENTS.md`](evaluation-job/AGENTS.md) |
| **Instrumentation init container** | `python-instrumentation-provider/` | Python + Docker (K8s init) | [`python-instrumentation-provider/AGENTS.md`](python-instrumentation-provider/AGENTS.md) |
| **E2E tests** | `test/e2e/` | Go, Ginkgo + Gomega | [`test/e2e/AGENTS.md`](test/e2e/AGENTS.md) |

Other dirs: `documentation/` (Docusaurus site), `deployments/` (Helm charts), `samples/` (example agents), `scripts/` (dev tooling). These have no dedicated guide yet.

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
console ──HTTP──▶ agent-manager-service ──▶ OpenChoreo / Postgres / Vault / NATS
   │                       │
   │                       └─ orchestrates agent deploys; agents are auto-instrumented by
   │                          python-instrumentation-provider (ships amp-instrumentation)
   │                                                     │
   └──HTTP──▶ agent-manager-observer ──▶ Observer ──▶ OTel traces from instrumented agents
                                                     │
                          evaluation-job ──uses amp-evaluation──▶ fetches those traces, scores them
cli (amctl) ──HTTP──▶ agent-manager-service   (same API as the console)
```

- **`agent-manager-service`** is the hub — the REST/gRPC/WS API the console and CLI both call.
- **Instrumented agents** emit OTel traces (via `amp-instrumentation`, injected by `python-instrumentation-provider`); `agent-manager-observer` serves them to the console; `evaluation-job` scores them via `amp-evaluation`.

## Local dev environment (from repo root)

```bash
make setup        # first-time: Colima + k3d + OpenChoreo + platform + console
make dev-up       # start all services
make dev-down     # stop
make dev-logs     # tail logs
make dev-rebuild  # full rebuild
make dev-migrate  # run DB migrations
make e2e-test     # run the e2e suite against the local stack
```

Run `make help` at the root for the full target list (gateway setup, port-forwarding, DB access, codegen helpers, etc.).

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

## Contributing

Design proposals and issue/PR workflow (feature lifecycle, proposal labels) are in [`CONTRIBUTING.md`](CONTRIBUTING.md). Security vulnerabilities → security@wso2.com (do not file a public issue).
