# Testing guide

Which tests a change needs, where their files go, how to run them and which of them CI runs. The guides for each part of the repository hold the details; this page links to them.

## The test types

| Type | Covers | Files | Run locally | CI runs it on |
|---|---|---|---|---|
| API unit tests | Service logic with mocked repositories and clients, middleware, config, module logic | `*_test.go` next to the code in `agent-manager-service`, **no build tag** | `make test-unit` in `agent-manager-service`; one test with `make test-run RUN=<pattern> PKG=./services/` | PRs that touch `agent-manager-service/**` |
| Route and permission invariants | Every route is authorized and audited, every permission has an owner and reaches roles and ThunderID, module OpenAPI documents match their routes | `agent-manager-service/api/*_invariant_test.go`, `api/audit_coverage_test.go`, `rbac/*_test.go` | `make security-test-static` (repository root), or as part of `make test-unit` | Same as unit tests |
| API integration tests | Repositories, SQL, transactions, migrations, the module host, and requests through the real router | `*_test.go` with `//go:build integration` on line 1; API-level ones in `agent-manager-service/tests/` | `make test-integration` (SQLite); `DB_DRIVER=postgres make test-integration` (the server in `.env.test`) | PRs that touch `agent-manager-service/**`, on SQLite and PostgreSQL |
| End-to-end API tests | User flows over the public API of a running stack | `test/e2e/tests/<domain>/` | `make up` in ThunderID sign-in mode ([below](#running-the-suites-that-need-a-stack)), then `make e2e-test` (`SUITE=<domain>`, `FOCUS=<regex>`) | Every PR to `amp-2.x` except documentation-only ones; release. Not on PRs to `main` |
| End-to-end CLI tests | `amctl` as a user runs it: exit codes and `--json` envelopes | `test/e2e/tests/cli/<group>/` | `make e2e-test SUITE=cli` | Same as end-to-end API tests |
| Live security tests | The scope matrix, roles, tokens and agent identity against a running stack | `test/e2e/security/<area>/` | `make security-test-live` (`SUITE=`, `FOCUS=`) | Same as end-to-end API tests |
| Compose smoke test | Sign-in, the core API, which routes each distribution serves, the console files | `deployments/standalone/scripts/smoke.sh` | `make up` then `make smoke`; `make down`, `make up-core`, then `make smoke-core` (file sign-in) | PRs to `amp-2.x` that touch `agent-manager-service/**` or `deployments/standalone/**` (`amp`, `amp-core`, and `amp` with the gateway disabled) |
| File sign-in smoke | `auth.mode: file`: the generated user file, sign-in, the console's auth mode | `.github/workflows/file-auth-smoke.yaml` | The steps in that workflow | PRs that touch sign-in, config or `deployments/standalone/**` |
| CLI unit tests | Each `amctl` command's `runX` function, clients, config | `cli/pkg/**/*_test.go` | `make amctl-test`, or `go test ./...` in `cli` | PRs that touch `cli/**`, and OpenAPI changes (client drift check) |
| Console unit tests | Hooks, API clients, components, utilities (Vitest, jsdom) | `*.test.ts(x)` next to the code under `console/workspaces` | `pnpm build` once, then `pnpm test` in `console`, or `pnpm run test` in one package | PRs that touch `console/**` |
| Instrumentation library | `libs/amp-instrumentation` (pytest, ruff, mypy) | `libs/amp-instrumentation/tests/` | `pytest` in the library | PRs that touch the library |
| Evaluation library and job | `libs/amp-evaluation`, `evaluation-job` | `libs/amp-evaluation/tests/`, `evaluation-job/test_main.py` | `pytest` | By hand only, while evaluation is out of AMP Lite |
| Instrumentation matrix | Each provider, framework and Python version emits spans the observer can parse | `test/instrumentation-matrix/` | `nox -s emission -- --cell-id=<id>` ([README](../test/instrumentation-matrix/README.md)) | PRs that touch instrumentation, the observer or the matrix; nightly on `main` |
| Sample smoke tests | Each sample against a model stub, and as an external agent on a running stack | `test/sample-smoke/<sample>/` | [test/sample-smoke/README.md](../test/sample-smoke/README.md) | PRs that touch that sample; the manual-instrumentation sample also on `agent-manager-service/**` |
| Secret scan | New commits, with gitleaks | `.github/workflows/secret-scan.yml`, `.gitleaksignore` | `gitleaks git` | Every PR |

## Which tests does my change need?

Start from the row that matches the change. A change that spans rows needs each row's tests.

| Change | Tests to add | Where |
|---|---|---|
| Service logic | A unit test with mocked repositories and clients | `agent-manager-service/services/<service>_unit_test.go`, or the service's existing unit test file |
| Repository, SQL or migration | An integration test, passing on SQLite and PostgreSQL. Never unit-test a repository with a mocked repository | Next to the repository, `//go:build integration` |
| Core schema (`db_migrations/schema/lite.yaml`) | Run `make schema`; `TestBaselinesAreGenerated` fails if the rendered migrations are stale. Add an integration test when the change carries a constraint or data the code relies on | `agent-manager-service/db_migrations/` |
| A REST route in the core | A unit test for the service, an API-level integration test when the behaviour spans controller, service and repository, and an e2e spec when it changes a user flow. The spec and code generation steps are in the `add-api-resource` skill | `agent-manager-service/tests/<operation>_test.go`, `test/e2e/tests/<domain>/` |
| A new permission or scope | The invariants fail until the permission is granted to a role, listed in the ThunderID bootstrap (`ampinit/templates/thunder/bootstrap/50-amp-api-client.yaml` and `60-amp-resource-server.yaml`) and requested by the e2e client (`ampScopes` in `test/e2e/framework/auth.go`). Add the route to the scope matrix when it deletes, disables or archives something, or changes roles, credentials or membership | `test/e2e/security/authz/scope_matrix_test.go` |
| A role or role policy | A live security spec | `test/e2e/security/roles/` |
| A new module or module routes | Unit tests, an integration test in the style of `TestModuleContract`, the invariant registrations, and smoke checks for the module's routes | [Writing a module → Testing](amp-lite/modules.md#testing) |
| Anything on the line between the core and a module | Run all three smoke variants (`amp`, `amp-core`, `amp-off`) | `deployments/standalone/scripts/smoke.sh` |
| An `amctl` command | A unit test of `runX` built from a hand-made `*XxxOptions`; an e2e CLI spec for a user-visible flow | `cli/pkg/cmd/<group>/<command>_test.go`, `test/e2e/tests/cli/<group>/` |
| A console hook, API function or component | A Vitest test next to the file. API functions live in `apis/` and hooks in `hooks/` (`add-console-api-feature` skill) | `<name>.test.ts` or `<name>.test.tsx` |
| Sign-in or configuration | Unit tests, plus the file sign-in smoke (runs on its own) and the e2e suites (ThunderID sign-in) | — |
| The Compose stack | `make up` and `make smoke`; add smoke checks for new routes | `deployments/standalone/` |
| The instrumentation library or the observer | pytest or Go tests, and a matrix cell when the emitted spans change | [RUNBOOK](../test/instrumentation-matrix/RUNBOOK.md) |
| A sample | Its local smoke, and its platform smoke if it has one | `test/sample-smoke/<sample>/` |
| An evaluator | pytest | `libs/amp-evaluation/tests/` |

## Conventions

### API (`agent-manager-service`)

- The build tag on line 1 decides the tier: none is a unit test, `//go:build integration` is an integration test. The details, mocks and shared helpers are in [agent-manager-service/AGENTS.md → Testing](../agent-manager-service/AGENTS.md#testing).
- Mocks are generated with moq (`make mocks`). Don't hand-write a mock for an interface that has a `//go:generate moq` directive.
- CI lints test files with the same rules as other code.
- API-level tests in `tests/` build the router with `apitestutils.MakeAppClientWithDeps` and the dependencies they replace in `wiring.TestClients`; `tests/modules/testmod` is a complete example module.
- Route authorization invariants are named `TestSecurityInvariant*`; `make security-test-static` selects them by that prefix.
- Use `make test-integration`, not `make test`: `make test` runs against the database in `.env`.

### End-to-end and live security (`test/e2e`)

- Put the raw API call in `operations/<domain>/`, the spec in `tests/<domain>/`, and shared fixtures in `testsetup/`. [test/e2e/README.md](../test/e2e/README.md) covers the layout and [test/e2e/AGENTS.md](../test/e2e/AGENTS.md) the HTTP and CLI stacks.
- Suites run with several processes. Create a resource that several `Describe` blocks share in `SynchronizedBeforeSuite`, never in `BeforeSuite`.
- Name resources with a prefix constant from `test/e2e/framework/shared_agent.go` (`e2e-test-` for API specs), so leftovers are easy to find and the cleanup suite can remove them.
- Live security suites live in `test/e2e/security/`, outside `tests/`, so `make e2e-test` does not run them. They run with one process by default.
- A spec that needs a real model provider key skips itself when the key is not set, because pull requests from forks get no repository secrets.

### Smoke tests

- `smoke.sh` names the routes each distribution serves. A new module route needs a line there.
- The smoke test signs in as the admin, so the stack must run with file sign-in, the default. The e2e suites need ThunderID sign-in instead; see [Running the suites that need a stack](#running-the-suites-that-need-a-stack).

### CLI (`cli`)

- Test `runX(ctx, opts)`, not the cobra `RunE` path. Capture output with `iostreams.Test()`. [cli/AGENTS.md → Testing](../cli/AGENTS.md#testing).

### Console (`console`)

- Vitest with jsdom and `globals: true`; one `vitest.config.ts` per package. To add the first test to a package, copy the `test` script and `vitest.config.ts` from `workspaces/libs/shared-component`. Keep `vitest.jsdom-storage.ts` in `setupFiles`: Node 25 and later define their own `localStorage` and `sessionStorage`, which would otherwise shadow jsdom's.
- Tests import the other workspace packages from their built output: run `pnpm build` before `pnpm test`, and again after changing a package that a test imports.

### Python

- Each library has its own `AGENTS.md` with lint, type-check and test commands. Sample platform smokes need the stack in ThunderID sign-in mode (`AUTH_MODE=idp`).

## Running the suites that need a stack

The suites need different sign-in modes, so they can't all run against one stack. The stack's default is file sign-in (`auth.mode: file`), where the API accepts only tokens it issues itself.

| Suites | Sign-in mode | Stack |
|---|---|---|
| `make smoke`, `make smoke-core`, `smoke.sh amp-off` | file (the default) | `make up` or `make up-core` as shipped |
| `make e2e-test`, `make security-test-live`, sample platform smokes | ThunderID (`idp`): they sign in with client credentials against ThunderID | `make up` with these two lines in `deployments/standalone/.env` |

```bash
AUTH_MODE=idp
KEY_MANAGER_JWKS_URL=http://thunder:${THUNDER_PORT}/oauth2/jwks
```

`AUTH_MODE` alone isn't enough: without `KEY_MANAGER_JWKS_URL` the API still validates against its own keys and refuses ThunderID's tokens. Run `make up` again after changing `.env`. Remove both lines and run `make up` again to go back to file sign-in for the smoke test.

For the e2e and live security suites:

1. Start the stack in ThunderID mode as above (or `make dev-up` to run the API from source).
2. Copy `test/e2e/.env.example` to `test/e2e/.env`. Set `OPENAI_API_KEY` to run the specs that call a model; without it they skip. `make e2e-test` reads the stack's generated client secrets from the `amp-api` container.
3. Run `make e2e-test` and `make security-test-live`.

The full stack (`make up`) and the core stack (`make up-core`) use the same ports. Stop one with `make down` or `make down-core` before you start the other.

CI starts the same stack with `.github/actions/amp-dev-stack`, which fixes the client secrets and the admin password through `.github/scripts/standalone-ci-settings.sh`. Change both that script and `test/e2e/framework/config.go` if one of those values changes.

## What CI does not run on a pull request

- The evaluation library and job: their checks run by hand.
- The heavy tier of the instrumentation matrix: it is off until traces return as a module.
- The observability live security suite: it skips unless `AM_OBSERVER_BASE_URL` points at an observer, which the AMP Lite stack does not run.
- Sample platform smokes run only when their sample changes, apart from the manual-instrumentation sample.
- `standalone-smoke` builds the image without the console, so a change to how the API serves the console needs `make up` and `make smoke` locally.
- Scheduled workflows run on `main` only, and PRs to `main` don't run the end-to-end suites.

Run these yourself when your change touches them, and say so in the pull request's *Automation tests* section.
