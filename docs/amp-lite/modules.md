# Writing a module

How AMP Lite is split into a core and modules, how to write a module, and how to run one. The contract lives in [`agent-manager-service/modkit`](../../agent-manager-service/modkit/modkit.go); the module host that runs it is `agent-manager-service/internal/modhost`. The design started as a public proof of concept, [a5anka/amp-modular-poc](https://github.com/a5anka/amp-modular-poc); this page describes what is built here.

## What a module is

A module is a Go package compiled into the binary. It registers itself from `init()`, like a `database/sql` driver:

```go
func init() {
	modkit.Register(module{})
}
```

Being compiled in doesn't install it. Installing, enabling, disabling and uninstalling are changes to rows in the core's `modules` table. Every change bumps `module_generation`; each replica polls it every 2 seconds and reloads its set of active modules, so no restart is needed.

- **Built-in modules** set `Builtin: true` in their manifest. They are always active and can't be disabled or uninstalled. Today these are `gateway` (the AI Gateway) and `legacy` (the routes Lite doesn't serve yet, which answer `501`).
- **Installable modules** start as *available* and are installed by an operator (see [Operating modules](#operating-modules)). `aihub` (the AI Hub: subscription plans) is one; the standalone stack installs it by default through its modules list (`AMP_MODULES=aihub`). `tests/modules/testmod` is another, used only by tests.

A distribution is a `main` package that chooses its modules with blank imports:

| Distribution | `main` | Modules |
|---|---|---|
| `amp` | `cmd/amp/main.go` imports `modules/all` | everything in `modules/all/all.go`: `gateway`, `legacy`, `aihub` |
| `amp-core` | `cmd/amp-core/main.go` imports nothing | none |

Both call `cli.Main`, so they share every command. The Dockerfile builds one or the other with `--build-arg DISTRIBUTION=amp|amp-core`.

## Core or module?

**The rule:** the core holds what every distribution needs and what modules build on. Everything else is a module, and the core never imports one.

| Core | Modules |
|---|---|
| Organizations (including the default org created at startup), projects, environments, the agent registry | The AI Gateway (`gateway`): gateways, LLM and MCP proxies, A2A, agent API keys, agent identity pages |
| Users, roles and RBAC, the permission registry | `legacy`: placeholder routes; the AI Hub (`aihub`): subscription plans, their sync from the developer portal and to the gateways |
| Audit log, settings, secrets, agent tokens and JWKS | Later: observability, evaluation, agent identity, A2A and other product areas as they are split out |
| HTTP edge and authentication, the module host, migrations, hooks and events, workload credentials, the embedded console shell | |

Before putting something in the core, ask:

1. Does every distribution need it, including `amp-core`?
2. Do other modules build on it as a primitive, the way they build on agents and projects?
3. Is it identity, tenancy or security?

If the answers are no, it belongs in a module. If a module needs something from another module, that is a capability, not a reason to move it into the core.

## Layout

```text
modules/<name>/
├── <name>.go          # Manifest, Migrations, UI, Activate, init()
├── schema/            # 001_<description>.yaml ...  (what you edit)
└── migrations/        # sqlite/ and postgres/       (rendered by make schema)
```

The gateway splits its code into `gwapi`, `gwctl`, `gwsvc`, `gwrepo` and `gwwire`, like the core's layers; a small module doesn't need to.

## The manifest

`Manifest()` says what the module needs and offers. From `testmod`:

```go
return modkit.Manifest{
	Name:     Name,
	Version:  "1.0.0",
	CoreAPI:  modkit.CoreAPIVersion,
	Provides: []string{Echo},
	Permissions: []modkit.Permission{
		{Name: "widget-read", Description: "Read widgets"},
		{Name: "widget-write", Description: "Write widgets"},
	},
	RoleGrants: map[string][]string{"Agent Manager Admin": {"widget-read", "widget-write"}},
	Events:     []string{WidgetCreated},
}
```

- **Name:** lowercase letters, digits and dashes, starting with a letter, at most 24 characters. Two modules can't have names whose table prefixes overlap (`a-b` and `a_b`, or `t` and `t-x`). An installable module can't be named after a core permission resource (`agent`, `project`, `environment`, ...), since its permissions would then read like the core's.
- **CoreAPI:** `modkit.CoreAPIVersion` (currently `1`). The core refuses a module written against another version.
- **Requires, Optional, Provides:** capability ids, versioned like `llm.proxy/v1`. Dependencies name capabilities, not modules, so a provider can be replaced without touching its consumers. `Requires` must be satisfied for the module to install or enable; `Optional` is used when present.
- **Permissions:** one-segment names. `widget-read` in module `testmod` becomes the permission `testmod:widget-read` and the token scope `amp:testmod:widget-read`. Built-in modules may also declare older full names (`llm-provider:read`), which are used as written.
- **RoleGrants:** predefined role name (`rbac.RoleAdmin` is `"Agent Manager Admin"`, also `Developer`, `AI Lead`, `Platform Engineer`) to the module's permission names. File sign-in grants them while the module is active. ThunderID roles are rendered by `amp init thunder` from every compiled-in module, so a new module's permissions reach ThunderID on the next `amp init`.
- **Events:** the event types the module publishes, named `<module>.<event>`.
- **UI:** console pages the module turns on, listed by `GET /api/v1/ui-manifest`. From the gateway:

  ```go
  var ui = modkit.UI{Pages: []modkit.Page{
  	{ID: "ai-gateway/gateways", Permission: "read"},
  	{ID: "ai-gateway/llm-providers", Permission: string(rbac.LLMProviderRead)},
  	...
  }}
  ```

  A page ID is `<console module>/<page plugin id>`. `Permission` is one of the module's own (by its short name) or a core permission; the page is listed only to callers who hold it. `UI.Nav`, `UI.Slots`, `UI.OwnSlots` and `Module.UI()` (a prebuilt bundle) are part of the contract and appear in the manifest response, but the console doesn't render module-supplied bundles yet: only `Pages` has an effect today.

## Migrations

Tables are declared in `modules/<dir>/schema/NNN_<description>.yaml`, in the same logical types as the core's [`lite.yaml`](../../agent-manager-service/db_migrations/schema/lite.yaml), with the module named at the top:

```yaml
module: testmod
tables:
  - name: mod_testmod_agent_tags
    columns:
      - {name: agent_id, type: id}
      - {name: org_id, type: id}
      - {name: tag, type: text, size: 64}
    primary_key: [agent_id]
```

- Every table is named `mod_<name>_<table>` (a dash in the name becomes `_`). The core's tables can never start with `mod_`. A table may reference earlier tables of the same module only.
- `make schema` (in `agent-manager-service`) renders `migrations/sqlite/` and `migrations/postgres/`. Embed that folder and return it from `Migrations()`:

  ```go
  //go:embed migrations
  var migrations embed.FS

  func (module) Migrations() fs.FS {
  	sub, err := fs.Sub(migrations, "migrations")
  	...
  }
  ```

- `amp migrate` (or `amp serve --migrate`) applies the core's migrations, then every compiled-in module's in name order, whether the module is installed or not. Each applied file is recorded in `module_migrations` with a sha256 checksum. The runner refuses an applied file that was edited, a recorded file the binary doesn't have (a database newer than the binary), a new file that sorts before an applied one, and any statement that touches a table outside the module's prefix. Add a new file; never edit one. A server won't start while migrations are pending.
- Queries you write in Go follow the [database rules](database-schema.md#rules): one SQL text for SQLite and PostgreSQL, no `ON CONFLICT`, no row-lock clauses, no database-specific functions. `db/dbx/portability_test.go` checks the repository for them.

## Activate and the Runtime

`Activate(ctx, rt)` registers everything the module adds. The host calls it on every replica each time the set of active modules changes, with a fresh `modkit.Runtime`, so it must be cheap and free of side effects. If it returns an error, or a registration fails (a write route without a permission, an undeclared capability, ...), the module stays inactive.

### Data

- `rt.DB()` is the database handle. Read and write only the module's own `mod_<name>_` tables. Nothing stops a query on another table at run time; the rule is enforced by review and by the migration runner.
- `rt.Core()` reads the core: `GetOrg`, `ListProjects`, `GetProject`, `ListAgents`, `GetAgent`, `ListEnvironments`. Inside a request or hook it refuses an organization other than the caller's.
- `rt.Config(key)` returns the module's own setting: `MODULE_<NAME>_<KEY>` from the environment, else the `config:` block of its `modules:` entry in `amp.yaml`.
- `rt.Logger()` is a logger tagged with the module.

### Routes

```go
rt.Handle(http.MethodGet, "/tags", "widget-read", func(w http.ResponseWriter, r *http.Request) { ... })
```

`Handle(method, path, perm, h)` mounts `/api/v1/m/<name><path>` on the API port with the core's sign-in, permission check, organization from the token and audit trail. `perm` is the module's own permission (short name) or a core one. A `GET` may pass `""` (any signed-in user of the organization); every other method must name a permission, which also names its audit action. `rt.Can(ctx, perm)` checks a permission inside a handler.

`HandleInternal(method, path, scope, h)` mounts `/api/internal/v1/m/<name><path>` on the internal server (:9243) for the module's own workloads. Callers present a workload token carrying `scope`; `modkit.WorkloadFrom(ctx)` returns who they are. A module gets those tokens by:

- `rt.IssueToken(ctx, modkit.TokenRequest{Subject, OrgID, Scopes, TTL, Claims})`: a short-lived token (15 minutes by default, at most an hour), signed with the core's key, audience `amp-module:<name>`. The public API refuses it.
- `rt.IssueCredential(ctx, modkit.CredentialRequest{...})`: an id and secret (only a hash is stored in `workload_credentials`) that a workload exchanges for a token at `POST /api/internal/v1/workload-tokens`. `rt.RevokeCredential` revokes one.

### Hooks

Hooks run synchronously inside the core's create, update or delete of an agent or environment:

```go
rt.Hook(modkit.ResourceAgent, modkit.Apply, func(ctx context.Context, req modkit.HookRequest) error {
	...
	return req.Tx.Exec(`INSERT INTO mod_testmod_agent_tags (agent_id, org_id, tag) VALUES (?, ?, ?)`,
		req.ID, req.OrgID, ext.Tag).Error
})
```

| Phase | When | Can refuse? |
|---|---|---|
| `Validate` | Create and update: inside the transaction, before the write. Delete: before the core tears anything down, outside any transaction (`req.Tx` is nil), read only | Yes. `modkit.Reject(...)` is a 400; `&modkit.Error{Status, Message}` sets the status (409 vetoes a delete); any other error is a 500 |
| `Apply` | Inside the transaction, after a create or update is written | An error rolls the change back |
| `Finalize` | Inside the transaction that deletes the resource, in reverse activation order | No: errors are logged and the delete goes ahead |

- A request addresses a module in an `extensions` object: `{"extensions": {"testmod": {"tag": "x"}}}` on agent create and update. The hook gets its own block in `req.Extension`. A block for a module that isn't active, or has no hook on that resource, is a 400.
- Write through `req.Tx`, never `rt.DB()`: each hook runs in its own savepoint, so its writes commit or roll back with the resource, and a failing or timed-out hook's writes are undone without breaking the core's transaction. `req.Tx` stops working when the hook returns.
- Each hook gets its own deep copy of `req.Object`; changing it affects nobody else.
- Each hook has 5 seconds (`modkit.HookTimeout`) and all hooks of one operation share 10 (`modkit.OperationTimeout`).
- Environments are only created with an organization, so today they get `Apply` hooks once, when an org is created; a failure there is logged and the org is still created.

### Events

```go
rt.Subscribe(modkit.EventAgentCreated, record)
...
return rt.PublishTx(r.Context(), tx, WidgetCreated, map[string]string{"name": body.Name})
```

- Core events: `agent.created`, `agent.deleted`, `environment.created`. A module can also subscribe to any event a compiled-in module declares.
- `PublishTx` writes the event in your transaction, so it is delivered only if the transaction commits. `Publish` writes it on its own, or through the hook's transaction when called with a hook's `ctx`. Both refuse an event type the manifest doesn't declare.
- Delivery is asynchronous and **at least once**: one `module_event_deliveries` row per subscribing module, claimed by a replica that runs the module, with a 30-second deadline per delivery. A failed delivery is retried with backoff (2 seconds, doubling, at most 5 minutes) up to 10 attempts, then kept as failed. A handler that overruns may still be running when the retry starts, so handlers must be idempotent and safe to run concurrently; `testmod` checks for the event id before inserting.
- Deliveries are kept for 7 days. A disabled module's queued deliveries wait for it within that window; uninstalling drops them.

### Capabilities

A capability is a versioned Go interface in `modkit/contracts/<name>v<N>/`, with its id as a constant:

```go
package llmv1

const ID = "llm.proxy/v1"

type Proxy interface {
	ValidateProviders(ctx context.Context, orgID string, providers []string) error
	BindAgent(ctx context.Context, agent contracts.AgentRef, environment string, configs []spec.ModelConfigRequest) error
}
```

- The provider calls `rt.Provide(llmv1.ID, impl)` for each id in its `Provides`.
- A consuming module lists the id in `Requires` or `Optional` and resolves it at call time with `modkit.Lookup[llmv1.Proxy](rt, llmv1.ID)`. The core does the same with `coreext.Lookup[llmv1.Proxy](llmv1.ID)`.
- Lookups always see the current set of active modules. `ok` is false when no active module provides it, so handle that: the core answers a `modelConfig` with no `llm.proxy/v1` provider with a 400 naming the capability.
- A new major version is a new id (`llm.proxy/v2`) and package; providers can offer both while consumers move.

### Optional interfaces

- `modkit.Installer`: `OnInstall(ctx, rt)` runs once when the module is installed, to catch up with core state that existed before it (agents created earlier, for example). If it fails, the module stays installed and the error is reported.
- `modkit.HealthReporter`: `Health(ctx, rt) map[string]any` is part of the contract, but nothing calls it yet.

## Connecting to the core

| You need to | Use |
|---|---|
| Refuse or extend a create, update or delete of an agent or environment, atomically | A hook |
| React after something happened, where a delay and a retry are fine | An event subscription |
| Offer a service other modules or the core call directly | A capability |
| Read orgs, projects, agents or environments | `rt.Core()` |
| Serve your own API | `rt.Handle` |
| Let your own workloads call you | `rt.HandleInternal` with workload tokens |

A module must not:

- import the core's packages (`services`, `repositories`, `controllers`, `models`, ...): only `modkit` and `modkit/contracts`;
- read or write core tables, or another module's tables; use `rt.Core()`, a capability or an event instead;
- be named after a core permission resource, or declare permissions outside its own namespace (only built-in modules keep older full names);
- do work in `Activate` beyond registering: it runs on every replica on every module change.

### Where the gateway stands

The `gateway` module predates the contract and doesn't follow it yet. Its `Activate` only provides `llm.proxy/v1`, `mcp.proxy/v1` and `agent.gateway/v1`; everything else goes through an escape hatch for built-in modules, `builtin.Bundle`. Its `Build(ctx, *wiring.AppParams)` builds the gateway on the core's Go services and repositories and returns `builtin.Parts`: routes under `/api/v1/orgs/...`, internal routes (gateway control plane and WebSocket), MCP tools, environment readers, and an `Init`, `Start` and `Stop` for its workers. Its packages import core `services`, `repositories`, `controllers`, `models` and `spec`, its tables are core tables without the `mod_gateway_` prefix (listed in [database-schema.md](database-schema.md#gateway-module-tables)), and its contracts take `spec` types. Don't copy this for a new module.

To run it on `modkit.Runtime` alone, the core needs:

- a `Core()` read API wide enough to replace its use of `PlatformRepository` and the exported repository helpers;
- its routes on `Handle`, either by moving the console to `/api/v1/m/gateway/...` or by letting built-in modules mount absolute paths;
- a contract for background workers, which `Runtime` doesn't have (built-ins use `Parts.Start` and `Stop`);
- its tables renamed to `mod_gateway_*` with their own migrations, its settings read through `rt.Config`, its ThunderID resource rendered from its manifest rather than the core's bootstrap template, its share of the OpenAPI spec split out, and contracts free of `spec` types.

## Testing

- **Unit tests** for the module's own logic, with no database. The host's lifecycle (install, enable, disable, dependency resolution, reconcile) is covered in `internal/modhost`; you don't need to repeat it.
- **Integration tests** in the style of `tests/module_contract_test.go` (`TestModuleContract`): build a host with `modhost.NewWith(db, driver, log, []modkit.Module{...})` so the test binary stands in for a distribution, then check what your module changes over the real API: routes 404 before install and after disable, permissions 403 and 200, hooks refusing and writing, events delivered after commit, capabilities found. Run them on both databases: `make test-integration` uses SQLite, and `DB_DRIVER=postgres make test-integration` uses the server in `.env.test`.
- **Invariant tests** in `api/` walk every route and permission, modules included: every route is authorized (`route_authz_invariant_test.go`), every mutating route is audited (`audit_coverage_test.go`), and every permission has one owner and reaches roles and ThunderID (`permission_owner_invariant_test.go`). They run in `make test-unit`; `make security-test-static` at the repository root runs the authorization ones alone.
- **Smoke tests** against a running stack: `make up` then `make smoke`, and `make up-core` then `make smoke-core` (`deployments/standalone/scripts/smoke.sh amp|amp-core`). Run both whenever a change touches the line between the core and a module.

## Console UI

The console's pages live in UI modules under `console/workspaces/ui-modules/<module>/`, one package per page under `pages/<feature>/`. UI modules are named for the product area and need not match the backend module (the gateway's pages are in `ai-gateway`). To add pages:

1. Create the page package and its `plugin.ts`, list it in the UI module's `src/index.ts`, and, for a new UI module, add it to `builtinModules` in `console/workspaces/core-ui/src/plugins/builtinModules.ts`. [console/AGENTS.md](../../console/AGENTS.md) has the recipe.
2. List each page in the backend module's `UI.Pages` as `<UI module>/<plugin id>`.

After sign-in the console reads `GET /api/v1/ui-manifest`. A UI module with no listed page is hidden, and so is every unlisted page; `acp-core` is always on. The manifest is read once per sign-in, so a module enabled later shows after the user signs in again.

## Operating modules

```bash
amp module list                       # NAME VERSION STATE BUILTIN PROVIDES REQUIRES
amp module install <name> [--with-deps]
amp module enable <name>
amp module disable <name>
amp module uninstall <name>
```

In the Compose stack, run them in the API container (for example `docker compose exec amp-api /go/bin/amp module list`). Changes reach running servers within seconds. `install --with-deps` also installs the modules that provide what it `Requires`; disable and uninstall are refused while an active module requires it. Built-in modules can't be changed.

To declare the set instead, list it in `amp.yaml`; the server reconciles it at every boot:

```yaml
modules:
  - widgets
  - name: reports
    enabled: false
    config:
      retention_days: 30   # rt.Config("retention_days"), or MODULE_REPORTS_RETENTION_DAYS
```

- Listed modules are installed with their providers and enabled or disabled as listed. A module listed disabled is installed only if it never was.
- Installed modules that aren't listed are disabled, unless a listed one requires them.
- No `modules:` key, or one whose entries are all commented out, means no reconcile: manage modules with `amp module` instead. `modules: []` declares an empty set.
- `AMP_MODULES` overrides the list: `AMP_MODULES=widgets,reports=off`, or `AMP_MODULES=none` for the empty set.
- Listing a built-in module as disabled, or a module the binary doesn't have, stops the server at boot.

`CONSOLE_UI_MODULES_ENABLE` turns on extra console UI modules or pages whatever the manifest says (`agent-runtime agent-portal`, or `<module>/<page>`), for pages that need no backend module.

When a module is disabled:

- its routes answer 404 at once, and its hooks, event handlers and capabilities stop;
- a request with an `extensions` block for it is a 400, and capability lookups fail (a core feature that needs it answers 400 naming the capability);
- its pages leave the UI manifest, and its permissions leave file sign-in roles and the console's sign-in scopes;
- its tables and data stay, and events queued for it wait up to 7 days. Uninstall keeps the data too, but drops queued events. Installing or enabling again picks up where it left off.
