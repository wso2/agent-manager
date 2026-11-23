---
name: add-tests
description: Use when adding, writing or choosing tests for a change in this repository — "what tests should I add", "add tests for this", finishing a feature or bug fix, or unsure whether something needs a unit, integration, e2e, live security, smoke, CLI, console or Python test, where the file goes, or whether CI will run it.
---

# Add the tests a change needs

**Source of truth:** [`docs/testing.md`](../../../docs/testing.md) — the test types, the change → tests table, and what CI runs. This skill is the checklist for applying it.

## Steps

1. **List what changed.** `git diff --name-only <base>...` against the PR's base branch (the remote for `wso2/agent-manager` may be `upstream` or `origin`), plus uncommitted files.
2. **Pick every row that applies** from the table below. A change usually hits several rows: a new route is service logic, a route, a permission and often a CLI command and a console hook.
3. **Write each test.** Where a leaf skill exists, use it; otherwise open the reference file.
4. **Run the narrowest command** for each test you wrote, then the tier it belongs to.
5. **Report** the tests you added, the commands you ran and their results, and any suite in "Not run on the PR" below that the change touches.

## Change → tests

| Change | Tests | How |
|---|---|---|
| Service logic (`agent-manager-service/services/`) | Unit, no build tag | `add-service-unit-test` skill |
| Repository, SQL, migration, schema | Integration, `//go:build integration`, on SQLite **and** PostgreSQL | [references/go-integration.md](references/go-integration.md) |
| A REST route in the core | Service unit + API-level integration in `agent-manager-service/tests/` + an e2e spec if a user flow changes (spec and codegen: `add-api-resource` skill) | [go-integration.md](references/go-integration.md), [e2e.md](references/e2e.md) |
| A new permission or scope | Wire it through roles, the ThunderID bootstrap and the e2e scope catalog; a scope-matrix entry when the route deletes, disables or archives something, or changes roles, credentials or membership | [references/security.md](references/security.md) |
| An audited or security-critical operation | Audit assertions | `add-audit-event` skill |
| A module or module routes | Unit, contract-style integration, invariant registrations, smoke lines | [references/modules-and-smoke.md](references/modules-and-smoke.md) |
| Core ↔ module boundary, the Compose stack | `smoke.sh amp`, `amp-core`, `amp-off` | [modules-and-smoke.md](references/modules-and-smoke.md) |
| An `amctl` command | CLI unit of `runX`; e2e CLI spec for a user-visible flow | [references/cli-console-python.md](references/cli-console-python.md), [e2e.md](references/e2e.md) |
| A console hook, API function or component | Vitest next to the file (`apis/` + `hooks/` pattern: `add-console-api-feature` skill) | [cli-console-python.md](references/cli-console-python.md) |
| Instrumentation library, observer, a sample, an evaluator | pytest / Go / matrix cell / sample smoke | [cli-console-python.md](references/cli-console-python.md); evaluator → `add-evaluator` skill |

## Rules that catch most mistakes

- **The build tag on line 1 is the tier.** No tag is a unit test and needs no database; `//go:build integration` needs one. Never mock a repository to test the repository.
- **A new permission breaks `make test-unit`** until it is granted to a role in `rbac/`, advertised in `agent-manager-service/ampinit/templates/thunder/bootstrap/50-amp-api-client.yaml` and `60-amp-resource-server.yaml`, and requested by `ampScopes` in `test/e2e/framework/auth.go`. Fix all four in the same change.
- **e2e specs** put the raw call in `test/e2e/operations/<domain>/`, the spec in `tests/<domain>/`, and shared fixtures in `testsetup/`; names use a prefix constant from `test/e2e/framework/shared_agent.go`; shared resources go in `SynchronizedBeforeSuite`.
- **Live security suites** live in `test/e2e/security/`, not `tests/`.
- **A spec that needs a model provider key** skips itself when the key is empty: pull requests from forks get no secrets.
- **Service unit tests** go in the service's existing unit test file when it has one, else `services/<service>_unit_test.go`.
- **e2e runs on PRs to `amp-2.x` only.** A PR to `main` gets none, so run the affected suites locally there.
- **Coverage is ratcheted.** CI fails when a package drops more than 0.5 points below its baseline. After adding tests, update the baseline in the same change (`make coverage-baseline` in `agent-manager-service`, `make amctl-coverage-baseline`, or the console command in `docs/testing.md#coverage`).
- **Console tests import built workspace packages.** Run `pnpm build` in `console` once before `pnpm test`.

## Run

| Tests | Command |
|---|---|
| One Go test | `cd agent-manager-service && make test-run RUN=<Test> PKG=./services/` |
| Unit tier (includes the invariants) | `make test-unit` in `agent-manager-service` |
| Integration tier | `make test-integration`; `DB_DRIVER=postgres make test-integration` |
| e2e / live security | `make up` with ThunderID sign-in ([references/e2e.md](references/e2e.md)), then `make e2e-test FOCUS="<spec>"`, `make security-test-live SUITE=<area>` |
| Smoke | `make up` with the default file sign-in, then `make smoke` ([references/modules-and-smoke.md](references/modules-and-smoke.md)) |
| CLI | `cd cli && go test -race ./pkg/cmd/<group>/...` |
| Console | `cd console && pnpm build && pnpm test` (one package: `pnpm run test` in it) |

A stack takes minutes to start. If none is running and the change needs e2e, smoke or live security, say which commands to run instead of skipping them silently.

## Not run on the PR

Say so in the report when the change touches one of these: the evaluation library and job (manual only), the instrumentation matrix's heavy tier (off), sample platform smokes other than manual-instrumentation (run only when their sample changes), the console as served by the API (`standalone-smoke` builds without it).
