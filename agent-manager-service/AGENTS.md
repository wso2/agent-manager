# agent-manager-service — agent guide

Go control-plane API. Layered **controller → service → repository**, wired with `wire`. Request/response types and mocks are **generated** — never hand-edited.

## File map

| Need | Location |
|---|---|
| API contract (source of truth) | `docs/api_v1_openapi.yaml` |
| Generated request/response types | `spec/` (regenerated wholesale — do not edit) |
| HTTP handlers | `controllers/` |
| Business logic | `services/` |
| Persistence (SQL/GORM) | `repositories/` |
| Route + authz registration | `api/*_routes.go` |
| Permission constants | `rbac/permissions.go` |
| Audit trail (actions, redaction, sinks) | `audit/` |
| Role → permission map | `rbac/predefined_roles.go` |
| Sentinel errors | `utils/errors.go` |
| Generated mocks | `repositories/repomocks/`, `clients/clientmocks/` |
| DI wiring | `wiring/wire.go` |

## Golden path: add or change an API resource

Do these in order. Steps 1–2 are mandatory before writing any Go type for the request/response.

1. **Edit `docs/api_v1_openapi.yaml`** — add/modify the path, operation, and schemas.
2. **`make spec`** — regenerates `spec/` from the YAML (needs Docker). Never write a request/response struct by hand; the whole `spec/` dir is deleted and rebuilt, so edits there are lost.
3. **Add permission(s)** in `rbac/permissions.go` (`resource:verb`, see Permissions).
4. **Implement controller → service → repository** (see Layering). Add a repository/service *interface* if needed.
5. **Register routes with authz** in `api/<resource>_routes.go` (see Permissions).
6. **Add the permission(s) to roles** in `rbac/predefined_roles.go`.
7. **`make codegen`** — only if you added/changed an interface (regenerates wire DI + mocks).
8. **Test** — write a service unit test (see Testing). Run the done-checklist.

**Audit logging is automatic for step 5** — registering a route audits it. You only act when the build tells you to, or when the operation is security-critical. See Audit logging.

## Layering

Each layer depends on the **interface** of the layer below, injected via constructor. That seam is what makes services unit-testable.

- **Controller** (`controllers/`) — HTTP only: parse/validate request, map result → status code, translate sentinel errors → HTTP errors. No business logic.
- **Service** (`services/`) — business logic: validation gates, orchestration, error mapping. Depends on repo/client **interfaces**, never concrete types.
- **Repository** (`repositories/`) — persistence only. Interface + concrete impl.

**Documented exception — the identity surface.** `controllers/identity_controller.go` is constructed as `NewIdentityController(client thundersvc.IdentityClient)` and calls the Thunder client directly; there is no identity service. Its business logic, including fail-closed audit emits, therefore lives in the controller. This predates the audit trail and is not a pattern to copy: a new resource gets a service. Introducing an identity service is worthwhile but is its own change.

When you change an interface, update **all** implementations: concrete impl, generated mocks (`make codegen`), and any noop/static impls. Don't re-fetch a resource already loaded earlier in the request path — pass it down.

## Permissions (RBAC)

Permissions are typed OAuth2 scopes in `rbac/permissions.go` (e.g. `agent:create`), enforced **per route at registration time**, not in the handler.

```go
// api/<resource>_routes.go
rr.HandleFuncWithValidationAndAuthz(
    "POST /orgs/{orgName}/projects/{projName}/agents", rbac.AgentCreate, ctrl.CreateAgent)
rr.HandleFuncWithValidationAndAuthz(
    "GET /orgs/{orgName}/projects/{projName}/agents", rbac.AgentRead, ctrl.ListAgents)
```

`HandleFuncWithValidationAndAuthz` composes: path-param validation → `RequirePermission` (token scope via `jwtassertion.HasAllScopes`) → `RequireOrgMatch` (when the path has `{orgName}`). Variants: `...AndAnyAuthz` (any-of), `RequireDynamicPermission` (permission depends on the request).

Rules:

- **Every route declares its own permission.** 188 of 190 routes use an authz registrar; the 2 plain `HandleFuncWithValidation` routes are deliberate exceptions. Default to authz; do not add an unauthenticated route without a reason.
- **Name permissions `resource:verb`** — `:create`/`:read`/`:update`/`:delete`, or a coarser `:manage` only where existing resources do.
- **Every new permission must be granted by at least one role** in `PredefinedRolePermissions` (`RoleAdmin` / `RoleDeveloper` / `RoleAILead` / `RolePlatformEngineer`) — an ungranted permission is unreachable.
- Scope/audience is a first-pass filter; the per-route permission is enforcement. `RequireOrgMatch` checks org-vs-path, but the service must **also** validate org against the target resource it loads (defense in depth).

## Audit logging

Every route registered through `RouteRegistrar` is audited automatically: non-GET routes always, GET routes only if listed in `sensitiveReadPaths`. You do not add anything to get that. The record names the actor, org, action, resource, outcome and source.

**Three things can require action from you.**

**1. The build fails because an action cannot be derived.** The action defaults to the route's `rbac.Permission` (already `resource:verb`). A route registered with **no** permission has nothing to derive from, so `audit.NewRouteMeta` panics at startup and `api/audit_coverage_test.go` fails. Fix it by adding one line to `actionOverrides` in `audit/policy.go`. (A multi-permission route derives from the first permission's resource and does not panic — check the label is right.)

**2. The permission does not describe the effect.** Same fix, and this is the common case. Add an override when:

- one permission gates several operations — every API-key route is `*:api-key-manage`, so create/rotate/revoke would otherwise be indistinguishable;
- the permission names a different resource than the operation acts on (`publish-kind` is gated by `agent-kind:create` but acts on an agent);
- the permission is narrower than the effect (`deployments/state` is gated by `agent:suspend` but also resumes).

Queries stay exact regardless, because `requestPath` records the route pattern — the derived action is only the human-readable label.

**3. The operation is security-critical.** The envelope record says a route was called; it cannot say *what changed*. `POST .../permissions/add -> 200` does not name the permission granted. Add a semantic event when the operation touches credentials, privileges, membership, deployment or deletion — use the **`add-audit-event` skill**.

Rules:

- **Never read a request or response body into a record.** Bodies are out of reach by design; that is what makes auditing every route safe. Pass named fields via `audit.Detail` instead.
- **`audit.Detail` records only scalars, `[]string` and `fmt.Stringer`.** Anything else is replaced with a `[unsupported:<type>]` marker rather than serialised — so a struct cannot leak, but do not rely on that: pass the field you mean. Undeclared keys are dropped at write time and reported under `_droppedKeys`.
- **Never pass a secret.** Use `audit.SecretRef` (stores a fingerprint) or record the key *name* only.
- A new action needs a class, a severity and a detail schema, all declared together in `audit/actions_domain.go`. A test fails if a registered action has no schema.

## Code generation

| Command | Regenerates | From | Needs |
|---|---|---|---|
| `make spec` | `spec/` types | `docs/api_v1_openapi.yaml` | Docker |
| `make codegen` | wire DI + mocks (`repomocks/`, `clientmocks/`) | `//go:generate` directives | `moq` on PATH |

Generated files are checked in and **never hand-edited**. Regenerate and commit the output with your change. `make codegen` needs the `moq` binary (`go install github.com/matryer/moq@latest`) — it can't run via `go run` because the module is `-mod=vendor`. A new repository interface needs a `//go:generate moq ... -pkg repomocks -out repomocks/<file>_mock.go` directive above its declaration (copy an existing one).

## Engineering rules

- **Errors** — map the specific sentinel (`gorm.ErrRecordNotFound` → `utils.ErrXxxNotFound`); wrap everything else `fmt.Errorf("...: %w", err)`. Never flatten an unexpected error into not-found; never silently fall back to a default on a non-not-found error. Compare sentinels with `errors.Is`, never `==` or string match.
- **Context** — every I/O method takes `context.Context` first and propagates it. HTTP clients use `NewRequestWithContext`.
- **Org scoping** — always set `org_id` from the caller's token (Thunder OU ID / `ouID`), never from the request path or body. Missing tenant identity is an error, not a wildcard. Every org-scoped table maps the tenant column as `ou_id`.
- **Tenant isolation is DB-only** — org isolation happens at the DB (`ou_id`) layer alone. All OpenChoreo API calls resolve to a single default namespace from config (`OPEN_CHOREO_DEFAULT_NAMESPACE`, default `"default"`, `config.OpenChoreo.DefaultNamespace`), so there is no namespace-level tenant separation yet.
- **Concurrency** — never hold a lock across I/O. Atomic upserts (`ON CONFLICT`), not read-then-write. Serialize expensive side effects per-key, not globally.
- **Config** — validate at startup, not first use; check co-dependent values together.
- **Observability** — get the logger from the context (`logger.GetLogger(ctx)`), never the package-level `slog` functions: only the context logger carries the correlation ID, and CI rejects `slog.Info/Warn/Error/Debug` outside `app/`, `db/` and `server/`. Keys are snake_case and one concept has one name (`ou_id`, `error`, `duration_ms`); untrusted values go through `utils.SanitizeForLog`. `Error` = the service failed, `Warn` = handled, including every 4xx, `Info` = rare state change, `Debug` = hot path. Log a failure **once, at the boundary** — the controller's `log.Error` already prints the whole `%w` chain, so a service that logs and re-returns logs at `Warn`. See `docs/logging.md`.

# Testing

## Two tiers (decided by the build tag on line 1 of the file)

| Tier | Build tag | DB? | Run |
|---|---|---|---|
| **Unit** | none | no | `make test-unit` |
| **Integration** | `//go:build integration` | yes (isolated Postgres) | `make test-integration` |

`make test` = both. `make test-coverage` = merged HTML report. Unit coverage is reported per-package (no `-coverpkg=./...`).

## What goes where

- **Unit** — service logic with collaborators mocked: error mapping, validation gates, branching, transformation, fan-out. No DB, no network.
- **Integration** — repository SQL/GORM, transactions, constraints; anything whose code-under-test *is* the DB/external boundary.
- **Never unit-test the repository layer** (mocking a repo to test the repo is circular). For code with no mockable seam (goroutine/ticker loops calling `db.GetDB()`, concrete `*vault.Client`, live git remotes), unit-test the branches reachable *before* the boundary and leave the rest to integration.

## Write a service unit test

Reference: **`services/agent_kind_service_unit_test.go`** — copy its shape.

- `services/<service>_unit_test.go`, `package services`, **no build tag** (omit the `//go:build` line entirely; if present it would be an integration test).
- Inject mocks via the `NewXxx(...)` constructor.
- Assert the service's own logic. Use `assert.ErrorIs` / `assert.NotErrorIs` (testify) for sentinels. Explicitly check that real errors are **not** masked as not-found.

```go
package services

func TestAgentKindService_GetKind(t *testing.T) {
    repo := &repomocks.AgentKindRepositoryMock{
        GetKindFunc: func(_ context.Context, _, _ string) (*models.AgentKind, error) {
            return nil, gorm.ErrRecordNotFound
        },
    }
    svc := NewAgentKindService(repo, &clientmocks.OpenChoreoClientMock{})
    _, err := svc.GetKind(context.Background(), "acme", "chatbot")
    assert.ErrorIs(t, err, utils.ErrAgentKindNotFound)
}
```

## Testing a service that emits audit events

Operations that must not happen unrecorded refuse to proceed when no recorder is installed, so a bare `context.Background()` makes them fail by design — you will see `audit: recorder unavailable`.

Use `auditableCtx(t)` (`services/audit_testing_test.go`), which installs a discarding recorder:

```go
resp, err := svc.RotateAPIKey(auditableCtx(t), ouID, proj, agent, env, keyName, req)
```

To assert the refusal itself, pass a bare context and expect `audit.ErrRecorderUnavailable` — see `TestAgentTokenManager_GenerateToken_RefusesWithoutAuditRecorder`.

## Mocks in tests

- Repositories → `repomocks.<Iface>Mock`; clients → `clientmocks.<Iface>Mock` (both `moq`-generated).
- Fields follow `<Method>Func`. **An unconfigured method panics** — leave a func `nil` to assert that path must not be reached:

```go
oc := &clientmocks.OpenChoreoClientMock{
    ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
        return []*models.ProjectResponse{{Name: "proj-a"}}, nil
    },
    // DeleteProjectFunc nil → test fails loudly if delete is called.
}
```

- In-package interfaces (`MonitorExecutor`, `PublisherCredentialProvisioner`, `GitCredentialsService`) have **no** generated mock — hand-write a func-field stub in the test file, same `<Method>Func` shape.

## Shared test helpers — reuse, don't redeclare (duplicate = compile error)

- `strPtr(s string) *string` — in `llm_deployment_service_test.go`.
- `discardLogger() *slog.Logger` — in `evaluator_manager_unit_test.go`.

If your helper name also exists in an `integration`-tagged file, they collide only under `-tags=integration`; give the unit-tier copy a distinct name (e.g. `intPtrU`) rather than editing the integration file.

## Run one test (config env vars are required — they load at import time)

```bash
DB_HOST=localhost DB_PORT=5432 DB_USER=unit DB_PASSWORD=unit DB_NAME=unit \
OPEN_CHOREO_BASE_URL=http://localhost/api/v1 \
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
SERVER_PORT=8080 \
go test -run 'TestAgentKindService' ./services/
```

(Or just `make test-unit`, which sets these. Services that sign tokens need `make gen-keys` first.)

## Done checklist

- [ ] `make test-unit` passes.
- [ ] `go build -tags=integration ./...` compiles (catches helper-name collisions across tiers).
- [ ] **CI lint passes** — run the exact CI command (NOT just `make lint`/`gofmt`, which use a different config):
      `golangci-lint run --config .github/linters/.golangci.yaml ./...`
- [ ] `gofmt -l` clean on changed files.
- [ ] Regenerated `spec/`/mocks committed if the YAML or an interface changed.
- [ ] `go test ./api/ -run TestEveryMutatingRouteIsAudited` passes (also runs in `make test-unit`).

CI lints **test files too**, with a strict config (`.github/linters/.golangci.yaml`). Trip-ups that bite test authors:

- **`nilnil`** — never `return nil, nil`. Use an empty typed value (`return []*models.Foo{}, nil`). When `(nil, nil)` is the actual input under test, suppress with a reason: `//nolint:nilnil // exercising the (nil, nil) input the service must handle`.
- **`goheader`** — every `.go` file (tests included) needs the Apache license header; copy it from an existing file.
- **`errorlint` / `nilerr`** — compare errors with `errors.Is`, not `==`; don't `return nil` after a non-nil error check.
- **`nolintlint`** — a bare `//nolint` is itself an error: always `//nolint:<linter> // <reason>`.
- **`exhaustruct`** (every struct field must be set) is enabled, but an exclusion rule turns it off for `_test.go` files — so test fixtures can use partial struct literals freely. It still applies to production code you touch.
