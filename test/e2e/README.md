# AMP End-to-End Tests

End-to-end tests that drive the Agent Management Platform through its public
HTTP API. Written in Go with [Ginkgo](https://onsi.github.io/ginkgo/) /
[Gomega](https://onsi.github.io/gomega/).

## Quick start

```bash
cp .env.example .env          # set OPENAI_API_KEY; other defaults match quick-start install (for `make setup`, see below)
make e2e-test                          # run all suites (from repo root)
make e2e-test SUITE=monitors           # one suite
make e2e-test FOCUS="Project Deletion Conflict"   # one spec by description
```


## Layout

```
framework/   API client, config, shared types, assertion helpers
operations/  one function per API call, grouped by domain (no test logic)
testsetup/   shared, idempotent provisioning of reused fixtures
tests/       the suites — one package (Go test binary) per domain
```

A spec reads as `It` steps that delegate to `operations/` and `testsetup/`.

## Configuration

All config is environment variables with defaults matching the quick-start
install (see `.env.example`).

## Running against `make setup`

`make setup` runs agent-manager-service in docker-compose on `:9000` rather than
in the cluster, so the quick-start defaults point at the wrong places. In `.env`,
uncomment the four values in the "Local `make setup` cluster" block:

| Variable | Value | Without it |
|----------|-------|------------|
| `AMP_API_BASE_URL` | `http://localhost:9000` | every suite waits on `/healthz` (404) until `READINESS_TIMEOUT`, then aborts |
| `GATEWAY_CHART` | `deployments/helm-charts/wso2-amp-api-platform-gateway-extension` | `add-environment.sh` pulls the OCI chart, which has no `0.0.0-dev` tag |
| `AGENT_MANAGER_INTERNAL_BASE_URL` | `http://host.docker.internal:9000` | a new environment's gateway bootstrap Job can't reach the API; the Helm install times out |
| `AGENT_MANAGER_INTERNAL_CP` | `host.docker.internal:9243` | the new gateway can't reach the control plane |

`GATEWAY_CHART` may be repo-relative; the harness resolves it against the
checkout root.

Before a run, check the things that fail slowly and look like test bugs:

- **The OpenChoreo API port-forward is alive.**
  `curl -s -o /dev/null -w '%{http_code}' http://localhost:8195/api/v1/namespaces`
  should print `401`. If it can't connect, every `/orgs` call returns 500.
  Restart the forwards with `make stop-port-forward && make port-forward PLATFORM=true BACKGROUND=true`.
- **The evaluation image is in k3d.**
  `docker exec k3d-openchoreo-local-setup-server-0 crictl images | grep amp-evaluation-monitor`.
  The kubelet garbage-collects unused images when the node disk is nearly full,
  and monitor runs then sit in `ImagePullBackOff`. Reload it with
  `make -C evaluation-job docker-load-k3d`.
- **`OPENAI_API_KEY` has no space after `=`.** The file is sourced as shell, so
  `OPENAI_API_KEY= sk-...` leaves the key empty.

Shared fixtures are reused by name even when an earlier run left them broken
(for example a second environment whose gateway never installed). After an
aborted run, delete the `e2e-` environments and projects before running again.

## Shared fixtures

Building an agent is slow (the suite budgets up to 20 min per build), so
expensive fixtures are **built once and reused by name** across suites and
re-runs. They persist between runs and are reused as long as they exist — but the
root cleanup suite (`tests/e2e_suite_test.go`, label `cleanup`) deletes
`e2e-test-*` resources older than 1 hour, so a run started more than an hour after
the last rebuilds them fresh.

| Fixture | Provisioned by |
|---------|----------------|
| Shared project (`e2e-test-shared`) | `testsetup.EnsureProject` |
| Single-env IT helpdesk agent | `testsetup.SetupSharedITHelpdeskAgent` |
| Two-env promotion infra + agent | `testsetup.EnsurePromotableInfra` / `SetupSharedPromotableITHelpdeskAgent` |
| Catalog kind | published by the `catalog` suite |

The `agent` suite is the canonical builder; other suites reuse by name. All
`testsetup` helpers are idempotent (check-then-reuse).

## Parallel execution (important for setup code)

CI and `make e2e-test` run with `ginkgo --procs=2`. What that means:

- One suite runs at a time, but Ginkgo runs **that suite as N separate OS
  processes** and splits its specs (`Describe` blocks) across them. So specs in
  the same suite run at the same time, in different processes.
- Those processes **don't share memory.** Each has its own copy of `Client`,
  `Cfg`, etc., so each process must build its own.

**The trap:** a `BeforeSuite` runs on *every* one of the N processes. If it
*creates a shared resource* (e.g. the `e2e-test-shared` project), all N processes
try to create the same thing at once and collide (→ `409 Conflict`), which can
also leave the shared agent in a wedged state.

Where each hook runs, and what to use it for:

| Setup hook | Runs… | Use for |
|------------|-------|---------|
| `BeforeSuite` | on **every** process | `Client`/`Cfg` setup only — nothing shared |
| `BeforeAll` | once, on the single process that runs its `Describe` | a resource used by **one `Describe` only** (stays lazy — skipped when that block is filtered out) |
| `SynchronizedBeforeSuite` | phase 1 **once** (one process); phase 2 on **every** process | a resource shared across **multiple `Describe` blocks** — phase 1 provisions it once, phase 2 gives each process its own `Client`/`Cfg` |

`testsetup.SynchronizedSharedITHelpdeskAgent` is a ready-made phase pair for the
shared agent.

## Adding a test

1. Put the raw API call in `operations/<domain>/` (assert status there).
2. Add the spec to `tests/<domain>/`, reusing shared fixtures via `testsetup`.
3. Follow the setup convention above; guard creates for re-runs / parallelism.
4. Verify: `go vet ./...` and `make e2e-test FOCUS="<your spec>"`.
