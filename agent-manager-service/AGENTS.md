# agent-manager-service — agent guide

Go control-plane API. Layered **controller → service → repository**, wired with `wire`. Request/response types and mocks are **generated** — never hand-edited.

The core (orgs, projects, environments, agents, identity, tokens, events) lives in the top-level packages below and imports no module. The AI Gateway (gateways, LLM and MCP proxies, A2A, agent API keys, agent identities) is the `ai-gateway` module (installed by default on `cmd/amp`) under `modules/aigateway/`, with the same layers in `gwapi/`, `gwctl/`, `gwsvc/`, `gwrepo/` and `gwwire/`; it runs on `modkit.Runtime` like any module and reaches the core's agent lifecycle through the capabilities in `modkit/contracts/`. Routes this version does not serve answer 404, and no permission exists for them. `cmd/amp` links every module; `cmd/amp-core` links none.

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
| Generated mocks | `repositories/repomocks/`, `clients/clientmocks/`, `modules/aigateway/gwrepo/gwrepomocks/` |
| DI wiring | `wiring/wire.go` (core), `modules/aigateway/gwwire/wire.go` (gateway) |
| Configuration (env + `amp.yaml`) | `config/config_loader.go`, `config/file.go`; every key in `../docs/amp-lite/configuration.md` |
| Embedded console (`-tags console`) | `webconsole/` (`make console-embed` fills `webconsole/dist/`) |
| Console UI of the core / modules | `api.CoreUI` in `api/ui_manifest_routes.go`, `modkit.UI.Pages` in a module's manifest |

## Configuration, the console and the modules list

- **One binary, one image.** `amp serve [--migrate]`, `amp migrate`, `amp init thunder DIR ai-gateway DIR fileauth DIR`, `amp module …`; the old flags (`-migrate`, `-init-thunder=…`) still work. The image carries it as `/go/bin/amp` (and `/go/bin/agent-manager-service`). Compose's `amp-init` runs `amp init` from the `amp-api` image.
- **Settings** are environment variables, and an optional `amp.yaml` fills in what the environment leaves unset. The command line loads one file after parsing its flags: `--config`, else `$AMP_CONFIG`, else `./amp.yaml`. YAML keys are the variable names nested by `_` (`db: {driver: postgres}` is `DB_DRIVER`). A set variable always wins, even when empty (`FOO=` clears a file value). File values never enter the process environment: read settings with `config.Value`, and register keys a command reads outside the config package with `config.RegisterKeys` (keys nothing reads are logged as likely typos). Every key is listed in `../docs/amp-lite/configuration.md`.
- **Modules list.** `modules:` in `amp.yaml` (or `AMP_MODULES=ai-gateway,widgets=off`) is reconciled at boot: listed modules are installed with their providers and enabled or disabled as listed (a module listed disabled is installed only if it never was); other installed modules are disabled unless a listed one requires them. A change another replica already made counts as done. No list (or a `modules:` key whose entries are all commented out) means no reconcile, only the distribution's defaults (`cli.Main("amp", "ai-gateway")`) are installed if they never were; an empty list needs `modules: []` or `AMP_MODULES=none`. A module's `config:` block reaches it via `Runtime.Config("key")` (env `MODULE_<NAME>_<KEY>`).
- **Console.** The built console is embedded with `-tags console` and served at `/` on the API port (SPA fallback to `index.html`; hashed `assets/` cached a year; `index.html`, `config.js` not cached). Without the tag a placeholder page is served — no Node needed for backend work. `config.js` fills each `$NAME` of the console's `config.template.js` from `CONSOLE_<NAME>`, else a default the service derives (public URL, ThunderID, `AUTH_MODE`, instrumentation URL, sign-in scopes = every active permission). `CONSOLE_ENABLED=false` turns it off. Set `SERVER_PUBLIC_URL` behind a proxy or ingress; without it the origin comes from the request (`X-Forwarded-Proto`/`X-Forwarded-Host` when set). The image builds the console unless built with `--build-arg CONSOLE=none`. `make dev-up` still runs Vite on :3000 against the API.
- **Which console pages show** comes from `GET /api/v1/ui-manifest` (signed-in, filtered by the caller's permissions): the core's `api.CoreUI` plus every active module's `Manifest().UI.Pages` (`"<console module>/<page>"`). A console UI module with no listed page is hidden. A module adding console pages lists them there.

## Add a module

Follow [docs/amp-lite/modules.md](../docs/amp-lite/modules.md): what belongs in the core, the manifest, migrations, the `Runtime` surfaces (routes, hooks, events, capabilities), console pages, tests and operation. `tests/modules/testmod` is a small complete example. A module never imports core packages (depguard checks its non-test files) or touches core tables.

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
    "POST /agents", rbac.AgentCreate, ctrl.CreateAgent)
rr.HandleFuncWithValidationAndAuthz(
    "GET /agents", rbac.AgentRead, ctrl.ListAgents)
```

`HandleFuncWithValidationAndAuthz` composes: path-param validation → `RequirePermission` (token scope via `jwtassertion.HasAllScopes`) → `RequireOrgMatch` (when the path has `{orgName}`). Variants: `...AndAnyAuthz` (any-of), `RequireDynamicPermission` (permission depends on the request), `...AndPlatformAdminAuthz` (see below). `...AndAuthzAndOrgOverride` composes `middleware.ApplyOrgOverride` between `RequireOrgMatch` and the permission check, so a caller the deployment's `OrgOverrideResolver` trusts can act on the org named in `X-Impersonate-Org`; no core route uses it now. Module routes opt in with `modkit.AllowOrgOverride()` (the AI Gateway's gateway list and gateway identity-provider routes).

**The one non-org-scoped surface: `/platform/...`.** Every other route in the table takes its org from the token. Routes under `/platform` read across *all* organizations with no `ou_id` predicate, and are registered with `HandleFuncWithValidationAndPlatformAdminAuthz`, which panics at startup on a pattern containing `{orgName}`. They are guarded by `middleware.RequirePlatformAdminOU` — a check on `claims.OuId` against `config.PlatformAdminOUID` (`PLATFORM_ADMIN_OU_ID`) that is **unconditional and fail-closed**, because the permission such a route declares is an ordinary org-level grant every tenant's admin holds, so its scope check — which always runs — cannot separate tenants. Handlers on these routes must also call `middleware.IsPlatformAdminRequest(ctx)` and refuse when false, so a route re-registered through the ordinary registrar fails closed instead of leaking cross-tenant data. No core route uses it now; the AI Gateway's `GET /m/ai-gateway/platform/gateways/failure-summary` gets the same check from `modkit.PlatformAdmin()` (its permission, `ai-gateway:gateways.read`, is again an ordinary grant).

Rules:

- **Every route declares its own permission.** Almost every route uses an authz registrar; the few plain `HandleFuncWithValidation` routes are deliberate exceptions. Default to authz; do not add an unauthenticated route without a reason.
- **Name permissions `resource:verb`** — `:create`/`:read`/`:update`/`:delete`, or a coarser `:manage` only where existing resources do.
- **Every new permission must be granted by at least one role** in `PredefinedRolePermissions` (`RoleAdmin` / `RoleDeveloper` / `RoleAILead` / `RolePlatformEngineer`) — an ungranted permission is unreachable.
- Scope/audience is a first-pass filter; the per-route permission is enforcement. `RequireOrgMatch` checks org-vs-path, but the service must **also** validate org against the target resource it loads (defense in depth).

## Audit logging

Every route registered through `RouteRegistrar` is audited automatically: non-GET routes always, GET routes only if listed in `sensitiveReadPaths`. You do not add anything to get that. The record names the actor, org, action, resource, outcome and source.

**Three things can require action from you.**

**1. The build fails because an action cannot be derived.** The action defaults to the route's `rbac.Permission` (already `resource:verb`). A route registered with **no** permission has nothing to derive from, so `audit.NewRouteMeta` panics at startup and `api/audit_coverage_test.go` fails. Fix it by adding one line to `actionOverrides` in `audit/policy.go`. (A multi-permission route derives from the first permission's resource and does not panic — check the label is right.)

**2. The permission does not describe the effect.** Same fix, and this is the common case. Add an override when:

- one permission gates several operations — every API-key route is `*:api-key-manage`, so create/rotate/revoke would otherwise be indistinguishable;
- the permission names a different resource than the operation acts on;
- the permission is narrower than the effect.

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
| `make spec MODULE=<name>` | the module's types, `modules/<dir>/<package>/` | `modules/<dir>/api/openapi.yaml` | Docker |
| `make codegen` | wire DI + mocks (`repomocks/`, `clientmocks/`) | `//go:generate` directives | `moq` on PATH |
| `make mocks` | mocks only (`repomocks/`, `clientmocks/`), no wire. `PKG=./repositories/...` to scope it | `//go:generate moq` directives | `moq` **`v0.5.3`** installed |

Generated files are checked in and **never hand-edited**. Regenerate and commit the output with your change. `make codegen`/`make mocks` need the `moq` binary pinned at **`v0.5.3`** (`go install github.com/matryer/moq@v0.5.3`) (`make tools` installs it). `make mocks` checks the installed version and fails fast if it isn't `v0.5.3`, since a different `moq` version reformats import ordering across every mock file it touches even with no interface change; `make codegen` only checks that `moq` is present on PATH, so if `git status` shows unrelated mock diffs after running it, check `moq --version` before committing. A new repository interface needs a `//go:generate moq ... -pkg repomocks -out repomocks/<file>_mock.go` directive above its declaration (copy an existing one).

## Engineering rules

- **Errors** — map the specific sentinel (`gorm.ErrRecordNotFound` → `utils.ErrXxxNotFound`); wrap everything else `fmt.Errorf("...: %w", err)`. Never flatten an unexpected error into not-found; never silently fall back to a default on a non-not-found error. Compare sentinels with `errors.Is`, never `==` or string match.
- **Context** — every I/O method takes `context.Context` first and propagates it. HTTP clients use `NewRequestWithContext`.
- **Org scoping** — always set `org_id` from the caller's token (Thunder OU ID / `ouID`), never from the request path or body. Missing tenant identity is an error, not a wildcard. Every org-scoped table maps the tenant column as `ou_id`.
- **Tenant isolation is DB-only** — org isolation happens at the DB (`ou_id`) layer alone. Organizations are the token's (no table; an org exists through its `default` environment, created on first use by `repositories.EnsureOrg`); projects, that environment and agents live in `repositories.PlatformRepository`. ThunderID addressing uses one configured org name (`AMP_ORG_NAMESPACE`, default `"default"`, `config.OrgNamespace`).
- **Concurrency** — never hold a lock across I/O. Atomic upserts (`ON CONFLICT`), not read-then-write. Serialize expensive side effects per-key, not globally.
- **Config** — validate at startup, not first use; check co-dependent values together.
- **Observability** — log with correlation context (org, resource ID, request ID). Debug = hot paths, Info = rare events, Error = destructive ops.

# Testing

## Two tiers (decided by the build tag on line 1 of the file)

| Tier | Build tag | DB? | Run |
|---|---|---|---|
| **Unit** | none | no | `make test-unit` |
| **Integration** | `//go:build integration` | yes (a throwaway SQLite file, or Postgres with `DB_DRIVER=postgres`) | `make test-integration` |

`make test` = both. `make test-coverage` = merged HTML report. Unit coverage is reported per-package (no `-coverpkg=./...`).

## What goes where

- **Unit** — service logic with collaborators mocked: error mapping, validation gates, branching, transformation, fan-out. No DB, no network.
- **Integration** — repository SQL/GORM, transactions, constraints; anything whose code-under-test *is* the DB/external boundary.
- **Never unit-test the repository layer** (mocking a repo to test the repo is circular). For code with no mockable seam (goroutine/ticker loops calling `db.GetDB()`, concrete `*vault.Client`, live git remotes), unit-test the branches reachable *before* the boundary and leave the rest to integration.

## Write a service unit test

Reference: **`services/infra_resource_manager_unit_test.go`** — copy its shape.

- `services/<service>_unit_test.go`, `package services`, **no build tag** (omit the `//go:build` line entirely; if present it would be an integration test).
- Inject mocks via the `NewXxx(...)` constructor.
- Assert the service's own logic. Use `assert.ErrorIs` / `assert.NotErrorIs` (testify) for sentinels. Explicitly check that real errors are **not** masked as not-found.

```go
package services

func TestInfraResourceManager_GetProject(t *testing.T) {
    platform := &repomocks.PlatformRepositoryMock{
        GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
            return nil, utils.ErrProjectNotFound
        },
    }
    svc := NewInfraResourceManager(platform, slog.Default())
    _, err := svc.GetProject(context.Background(), "acme", "missing")
    assert.ErrorIs(t, err, utils.ErrProjectNotFound)
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
platform := &repomocks.PlatformRepositoryMock{
    ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
        return []*models.ProjectResponse{{Name: "proj-a"}}, nil
    },
    // DeleteProjectFunc nil → test fails loudly if delete is called.
}
```

- In-package interfaces without a `//go:generate moq` directive have **no** generated mock — hand-write a func-field stub in the test file, same `<Method>Func` shape.

## Shared test helpers — reuse, don't redeclare (duplicate = compile error)

- `strPtr(s string) *string` — in `services/helpers_test.go`.
- `discardLogger() *slog.Logger` — in `helpers_test.go`.

If your helper name also exists in an `integration`-tagged file, they collide only under `-tags=integration`; give the unit-tier copy a distinct name (e.g. `intPtrU`) rather than editing the integration file.

## Run one test (config env vars are required — they load at import time)

```bash
make test-run RUN=TestAgentKindService PKG=./services/
```

`PKG` defaults to `./...` if omitted. This sets the same config env var defaults as `make test-unit` (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `ENCRYPTION_KEY`, `SERVER_PORT`) and runs `make gen-keys` first, since services that sign tokens need the RSA keys present. Equivalent to running `go test -run <pattern> <pkg>` by hand with those env vars exported — see `scripts/run_single_test.sh` if you need the raw invocation.

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
