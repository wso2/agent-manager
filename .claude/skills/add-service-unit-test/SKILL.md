---
name: add-service-unit-test
description: Write a service-layer unit test in agent-manager-service (the Go control plane). Use when the user asks to add or fix a unit test for a service, mock a repository/client, or when CI lint fails on a test file (nilnil, goheader, exhaustruct, errorlint). Enforces the no-build-tag unit tier, moq-generated mocks, and the strict CI lint config that also lints test files.
---

# Write a service unit test (agent-manager-service)

Fast, DB-free tests of service logic with collaborators mocked. To decide which tests a change needs (integration, e2e, security, smoke, CLI, console), use the `add-tests` skill.

**Read first:** `agent-manager-service/AGENTS.md` → the "Testing" section (two tiers, what goes where, mocks, CI lint traps). This skill is the executable checklist.

## Rules that decide correctness

- **File:** `services/<service>_unit_test.go`, `package services`, **no build tag** — omit the `//go:build` line entirely. A `//go:build integration` line makes it an integration (DB) test.
- **Only test the service layer.** Never unit-test the repository (mocking a repo to test the repo is circular) — that's the integration tier.
- **Inject mocks via the `NewXxx(...)` constructor**, not by reaching into fields.
- **Assert the service's own logic** — error mapping, validation gates, branching, fan-out. Use `assert.ErrorIs` / `assert.NotErrorIs` for sentinels, and explicitly check that a real error is **not** masked as not-found.

## Mocks

- Repositories → `repomocks.<Iface>Mock`; clients → `clientmocks.<Iface>Mock` (both `moq`-generated; run `make codegen` if the interface is new).
- Configure `<Method>Func` fields. **Leaving a func `nil` and having it called panics** — use that deliberately to assert a path must not be reached.
- In-package interfaces with no generated mock (e.g. `MonitorExecutor`, `GitCredentialsService`) → hand-write a func-field stub in the test file, same `<Method>Func` shape.
- Reuse shared helpers, don't redeclare (duplicate = compile error): `strPtr` (in `llm_deployment_service_test.go`), `discardLogger` (in `evaluator_manager_unit_test.go`).

## Reference

Copy the shape of **`services/agent_kind_service_unit_test.go`**.

## CI lint traps (CI lints test files too, with `.github/linters/.golangci.yaml`)

- **`nilnil`** — never `return nil, nil`. Return an empty typed value (`return []*models.Foo{}, nil`). If `(nil, nil)` is genuinely under test: `//nolint:nilnil // <reason>`.
- **`goheader`** — every `.go` file (tests included) needs the Apache license header; copy from an existing file.
- **`errorlint` / `nilerr`** — compare with `errors.Is`, not `==`; don't `return nil` after a non-nil error check.
- **`nolintlint`** — a bare `//nolint` is itself an error; always `//nolint:<linter> // <reason>`.
- **`exhaustruct`** is off for `_test.go` (partial struct literals fine in fixtures) but still applies to production code you touch.

## Run

```bash
make test-unit                 # runs unit tier + sets required config env vars
# single test (env vars load at import time):
go test -run 'TestAgentKindService' ./services/    # needs DB_*/OPEN_CHOREO_BASE_URL/ENCRYPTION_KEY/SERVER_PORT set
```
Services that sign tokens need `make gen-keys` first.

## Services that emit audit events

Operations that must not happen unrecorded — minting a token, rotating a key, deploying, deleting — refuse to proceed when no audit recorder is installed. A bare `context.Background()` therefore makes them fail with `audit: recorder unavailable`.

Pass `auditableCtx(t)` (declared in `services/audit_testing_test.go`, do not redeclare it):

```go
resp, err := svc.RotateAPIKey(auditableCtx(t), ouID, proj, agent, env, keyName, req)
```

To assert the refusal itself, pass a bare context and expect `audit.ErrRecorderUnavailable`.

## Done checklist

- [ ] `make test-unit` passes.
- [ ] `go build -tags=integration ./...` compiles (catches helper-name collisions across tiers).
- [ ] CI lint clean: `golangci-lint run --config .github/linters/.golangci.yaml ./...`
- [ ] `gofmt -l` clean on changed files.
