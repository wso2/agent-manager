# Writing a module

How AMP Lite is split into a core and modules, how to write a module, and how to run one. The contract lives in [`agent-manager-service/modkit`](../../agent-manager-service/modkit/modkit.go); the module host that runs it is `agent-manager-service/internal/modhost`. The design started as a public proof of concept, [a5anka/amp-modular-poc](https://github.com/a5anka/amp-modular-poc); this page describes what is built here, and [Beyond the proof of concept](#beyond-the-proof-of-concept) lists where it adds to or differs from it.

## What a module is

A module is a Go package compiled into the binary. It registers itself from `init()`, like a `database/sql` driver:

```go
func init() {
	modkit.Register(module{})
}
```

Being compiled in doesn't install it. Installing, enabling, disabling and uninstalling are changes to rows in the core's `modules` table. Every change bumps `module_generation`; each replica polls it every 2 seconds and reloads its set of active modules, so no restart is needed.

- **Every compiled-in module** starts as *available* and is installed by an operator (see [Operating modules](#operating-modules)), or by the server as a distribution default. `ai-gateway` (the AI Gateway) is one, and `observability` (traces, from ClickHouse, or from an in-memory store per replica when none is configured) another. `tests/modules/testmod` is linked into no binary: tests hand it to the host.
- **Distribution defaults.** A distribution's `main` passes its default modules to `cli.Main` (`cmd/amp`: `cli.Main("amp", "ai-gateway")`; `cmd/amp-core` none). With no modules list declared, the server installs each default at boot, with its providers, only if it was never installed (or was purged), and changes nothing else, so a default an operator disabled or uninstalled stays that way across restarts. A declared list replaces the defaults.

A distribution is a `main` package that chooses its modules with blank imports:

| Distribution | `main` | Modules |
|---|---|---|
| `amp` | `cmd/amp/main.go` imports `modules/all` | everything in `modules/all/all.go`: `ai-gateway` (installed by default), `observability` |
| `amp-core` | `cmd/amp-core/main.go` imports nothing | none |

Both call `cli.Main`, so they share every command. The Dockerfile builds one or the other with `--build-arg DISTRIBUTION=amp|amp-core`.

## Core or module?

**The rule:** the core holds what every distribution needs and what modules build on. Everything else is a module, and the core never imports one.

| Core | Modules |
|---|---|
| Organizations (including the default org created at startup), projects, environments, the agent registry | The AI Gateway (`ai-gateway`): gateways, LLM and MCP proxies, A2A, agent API keys, agent identity pages |
| Users, roles and RBAC, the permission registry | `observability`: traces, and the `traces.query/v1` capability (no trace data yet, see above) |
| Audit log, settings, secrets, agent tokens and JWKS | Later: evaluation, agent identity, A2A and other product areas as they are split out |
| HTTP edge and authentication, the module host, migrations, hooks and events, workload credentials, the embedded console shell | |

Before putting something in the core, ask:

1. Does every distribution need it, including `amp-core`?
2. Do other modules build on it as a primitive, the way they build on agents and projects?
3. Is it identity, tenancy or security?

If the answers are no, it belongs in a module. If a module needs something from another module, that is a capability, not a reason to move it into the core.

## Add a module, step by step

The sections below are the reference; this is the path through them. `tests/modules/testmod` exercises routes, hooks, events, capabilities, workloads, workers, MCP tools, secrets and core lookups; `modules/aigateway` is a large real one and shows the audit, webhook-data, org-override, `Track`, `UI`, `OnInstall` and `InitComponent` surfaces. What the contract adds to the proof of concept, and where it differs, is listed in [Beyond the proof of concept](#beyond-the-proof-of-concept).

1. **Decide it is a module** ([Core or module?](#core-or-module)), then create `modules/<name without dashes>/` (the directory `make spec MODULE=<name>` expects) with a Go file holding the module type and `init() { modkit.Register(module{}) }` ([Layout](#layout)). testmod leaves out the `init()`; a real module needs it.
2. **Write the manifest:** name, version, `CoreAPI`, what it `Provides` and `Requires`, its events and audit actions ([The manifest](#the-manifest)).
3. **Declare permissions and role grants** in the manifest, by short name (`widgets.read`); they become the permission `<name>:widgets.read` and the token scope `amp:<name>:widgets.read`. On the Compose stack, `make up` with the rebuilt image reruns `amp-init`, which writes ThunderID's bootstrap with the new scopes, and `thunder-setup`, which applies it to the existing ThunderID database (additively). Elsewhere, run `amp init thunder <dir>` into the directory your ThunderID setup reads and apply it with ThunderID's setup. File sign-in picks up `RoleGrants` with no step ([The manifest](#the-manifest)).
4. **Add tables** as YAML under `schema/`, run `make schema`, commit both `migrations/sqlite` and `migrations/postgres`, and embed them from `Migrations()`; tables are `mod_<name>_*` and never reference core tables. Add migration files; never edit one that has shipped ([Migrations](#migrations)).
5. **Write `Activate`:** register routes with `rt.Handle`, each with a permission and its route options (audit action, `Emits` for webhook events, `AlsoRequires`, `AllowOrgOverride`, ...), and annotate records and events from handlers with `AuditResource`/`AuditDetail` and `EventEnvironment`/`EventData`. `Activate` registers only: it must not write. The default organization and its environment already exist when it runs; a write route calls `rt.Core().EnsureOrg` before an organization's first rows ([Activate and the Runtime](#activate-and-the-runtime), [Routes](#routes)).
6. **React to the core** with hooks on agents, projects and environments, and subscribe to or publish events ([Hooks](#hooks), [Events](#events)).
7. **Add the rest it needs:** workers with `rt.Go`, MCP tools with `rt.MCPTool`, secrets with `rt.Secrets()`, settings with `rt.Config` ([Background workers](#background-workers), [MCP tools](#mcp-tools), [Data](#data)).
8. **Offer or use capabilities** for calls between modules, or into the core (`identity.directory/v1`) ([Capabilities](#capabilities)).
9. **Describe its API** in `api/openapi.yaml`, set `info.x-amp-module` and `servers: [{url: /m/<name>}]`, run `make spec MODULE=<name>` (`make codegenfmt-check` must stay clean), and add a `Test<Module>SpecMatchesRoutes` to `api/module_routes_invariant_test.go` that calls `checkModuleSpec` ([API document](#api-document)).
10. **Add console pages** in a UI module and list them in `UI.Pages` ([Console UI](#console-ui)).
11. **Add it to a distribution:** import it in `modules/all/all.go` so `cmd/amp` links it. It is then installed with `amp module install` or a `modules:` list; to install it by default, add it to the default list `cmd/amp` passes to `cli.Main` ([What a module is](#what-a-module-is), [Operating modules](#operating-modules)).
12. **If it ships a service** beside the server, declare it as a workload, and write its files at `amp init <name> DIR` with `modkit.InitComponent` ([Workloads](#workloads), [Optional interfaces](#optional-interfaces)).
13. **Test it** ([Testing](#testing)):
    - unit tests;
    - integration tests (`//go:build integration`, in the style of `tests/module_contract_test.go`) on SQLite and with `DB_DRIVER=postgres`; a module-local test helper package (like the gateway's `gwtest`) is exempt from `depguard` only once it is added to the linter's exclusions;
    - register the module in the `api/` invariant tests, which see only the modules they list (see [Testing](#testing));
    - add smoke checks for its routes to `deployments/standalone/scripts/smoke.sh` and run `make smoke` and `make smoke-core`.

    It must import only `modkit`, `pkg/` and its own packages; `depguard` checks the core part of that ([Module boundaries](#module-boundaries)).
14. **Document what you add to the contract**, if anything, in this page and in [Beyond the proof of concept](#beyond-the-proof-of-concept), in the same change.

## Layout

```text
modules/<name without dashes>/
├── <name>.go          # Manifest, Migrations, UI, Activate, init()
├── schema/            # 001_<description>.yaml ...  (what you edit)
├── migrations/        # sqlite/ and postgres/       (rendered by make schema)
└── api/openapi.yaml   # the module's own API document, if it has routes
```

The gateway splits its code into `gwapi`, `gwctl`, `gwsvc`, `gwrepo` and `gwwire`, like the core's layers; a small module doesn't need to.

## The manifest

`Manifest()` says what the module needs and offers. From `testmod`:

```go
return modkit.Manifest{
	Name:        Name,
	DisplayName: "Test Module",
	Version:     "1.0.0",
	CoreAPI:     modkit.CoreAPIVersion,
	Provides:    []string{Echo},
	Permissions: []modkit.Permission{
		{Name: "widget-read", DisplayName: "Read Widgets", Description: "Read widgets"},
		{Name: "widget-write", DisplayName: "Write Widgets", Description: "Write widgets"},
	},
	RoleGrants: map[string][]string{"Agent Manager Admin": {"widget-read", "widget-write"}},
	Events: []modkit.EventType{
		{Name: WidgetCreated, Description: "A widget was created."},
		{Name: WidgetArchived, Scope: "org", Description: "A widget was archived."},
	},
	AuditActions: []modkit.AuditAction{
		{Name: "widget-archive", Description: "Archive a widget"},
		{Name: "secret-read", Class: "credential", Description: "Read a secret"},
		...
	},
}
```

- **Name:** lowercase letters, digits and dashes, starting with a letter, at most 24 characters. Two modules can't have names whose table prefixes overlap (`a-b` and `a_b`, or `t` and `t-x`). An installable module can't be named after a core permission resource (`agent`, `project`, `environment`, ...), since its permissions would then read like the core's.
- **CoreAPI:** `modkit.CoreAPIVersion` (currently `1`). The core refuses a module written against another version.
- **Requires, Optional, Provides:** capability ids, versioned like `llm.proxy/v1`. Dependencies name capabilities, not modules, so a provider can be replaced without touching its consumers. `Requires` must be satisfied for the module to install or enable; `Optional` is used when present.
- **DisplayName:** how people see the module, such as the name of its resource in ThunderID; `Name` when empty.
- **Permissions:** an action (`widget-read`) or a resource and an action (`widgets.read`), each lowercase letters, digits and dashes. `widget-read` in module `testmod` becomes the permission `testmod:widget-read` and the token scope `amp:testmod:widget-read`; the AI Gateway's `llm-providers.read` becomes `ai-gateway:llm-providers.read` and `amp:ai-gateway:llm-providers.read`. A permission's `DisplayName` names its action in ThunderID (`Name` when empty).
- **RoleGrants:** predefined role name (`rbac.RoleAdmin` is `"Agent Manager Admin"`, also `Developer`, `AI Lead`, `Platform Engineer`) to the module's permission names. File sign-in grants them while the module is active. ThunderID roles are rendered by `amp init thunder` from every compiled-in module, so a new module's permissions reach ThunderID on the next `amp init`.
- **Events:** the event types the module publishes, named `<module>.<event>`, each with a description. An event with a `Scope` (`org`, `project` or `agent`, the narrowest resource it is about) is also a webhook event: while the module is active it is listed in the event catalog (`GET /api/v1/orgs/{org}/events/event-types`) and organizations can subscribe to it. Routes raise it with `modkit.Emits` (see [Route options](#route-options)). An event without a scope only reaches module subscribers.
- **AuditActions:** the audit actions the module's routes and tools name with `modkit.Audit`, each recorded as `<module>:<name>`, with a class (`credential`, `identity`, `deployment`, `config` or `read`; `config` when empty) and a severity (`info`, `notice`, `warning` or `critical`; `critical` for credential and identity actions and `info` otherwise when empty). An entry named after one of the module's permissions classifies the action that permission names. `Details` lists the detail keys the action's records may carry, each with its kind (see [Audit records](#audit-records)).
- **Workloads:** the parts of the module that run outside the binary. See [Workloads](#workloads).
- **Settings:** the settings the module reads with `rt.Config` (and `InitRequest.Config`): key, default, description, and whether it is a secret. They go into the [configuration reference](configuration.md) (`make config-docs`); a test in the module should check them against what it reads, as `modules/aigateway/settings_test.go` does.
- **UI:** console pages the module turns on, listed by `GET /api/v1/ui-manifest`. From the gateway:

  ```go
  var ui = modkit.UI{Pages: []modkit.Page{
  	{ID: "ai-gateway/gateways", Permission: "gateways.read"},
  	{ID: "ai-gateway/llm-providers", Permission: "llm-providers.read"},
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
- Queries you write in Go follow the [database rules](database-schema.md#rules): one SQL text for SQLite and PostgreSQL, no `ON CONFLICT`, no row-lock clauses, no database-specific functions. `db/portability_test.go` checks the repository for them; `pkg/dbx` has the portable helpers (claims, upserts, row locks).

### Stores outside the core database

A module that keeps data in its own store, such as ClickHouse, implements `modkit.ExternalMigrator`. `amp migrate` applies that store's schema files after the module's SQL migrations; install and enable only check that they were applied.

```go
func (module) ExternalSchema(config func(string) string, apply bool) (modkit.ExternalSchema, error)
```

- `ExternalSchema` returns the store's `Files()` (`NNN_description.<ext>`) and how to read what was applied (`Applied`), apply one file and record it (`Apply`), and settle config-driven settings such as a TTL after the files (`Settle`, on every run). `apply` is true for `amp migrate` and false for the install and enable check, so the module can log in with its migrator only in the migrate step.
- The core applies the same rules as for SQL migrations: files in order, each recorded with a sha256 checksum, and a refusal for an edited applied file, an applied version the binary doesn't have, and a new file that sorts before an applied one. Running `amp migrate` again applies nothing.
- The store keeps the record of what it applied, next to its data (the observability module's is `otel.amp_schema_migrations`), so a store that is recreated is migrated again from scratch.
- An error wrapping `modkit.ErrNotConfigured` means there is no store, or no login, to migrate: `amp migrate` logs the reason and carries on. This is how `amp serve --migrate` behaves on a server that has only the store's reading login.
- Installing or enabling a module whose store is configured but missing files is refused with a hint to run `amp migrate`; a store it can't reach is refused too. The check runs before the lifecycle transaction, so a slow store holds no lock.
- Apply never runs at install or enable, and `amp-core` has no modules, so neither changes for it.

## Activate and the Runtime

`Activate(ctx, rt)` registers everything the module adds. The host calls it on every replica each time the set of active modules changes, with a fresh `modkit.Runtime`, so it must be cheap and free of side effects. Installing a module also runs it inside the lifecycle change's transaction, to check it activates before the change commits, so it must not write: on SQLite a write on another connection waits on that transaction and the install fails. Write from a worker (`rt.Go`), `OnInstall` or a request instead. If it returns an error, or a registration fails (a write route without a permission, an undeclared capability, ...), the module stays inactive.

The core's default organization (`DEFAULT_ORG_OU_ID`) and its default environment exist by the time `Activate` runs: the core creates them before any module activates, so no module hook or event sees their creation.

### Data

- `rt.DB()` is the database handle. Read and write only the module's own `mod_<name>_` tables. Nothing stops a query on another table at run time; the rule is enforced by review and by the migration runner.
- `rt.Core()` reads the core: `GetOrg`, `ListProjects`, `GetProject`, `ListAgents`, `GetAgent`, `ListEnvironments`, and by the names the API uses, `GetProjectByName`, `GetAgentByName` (project and agent name) and `GetEnvironmentByName`. Called with the ctx of a request, hook or event, it answers only for that ctx's organization. It also offers two calls that are not reads:
  - `LockAgent(ctx, tx, orgID, agentID)` locks the agent's row inside `tx` (a hook's `req.Tx` or your own transaction) until it ends, so changes to that agent made meanwhile wait;
  - `EnsureOrg(ctx, orgID)` creates the caller's organization, with its default environment, if no core route has created it yet; call it before a route writes the first rows of an organization.
- `rt.Secrets()` keeps secret values in the core's `secrets` table, sealed by the core's configured vault (see [Modules and vaults](#modules-and-vaults)): `Put(ctx, orgID, path, value)`, `Get` (`modkit.ErrNotFound` when there is none) and `Delete`. Paths are the module's own (`providers/<id>/api-key`: segments of letters, digits, `.`, `_` and `-`), so a module never reaches another module's secrets or the core's. With a hook's `ctx` they go through the hook's transaction; `rt.Secrets().In(tx)` writes through your own.
- `rt.Config(key)` returns the module's own setting: `MODULE_<NAME>_<KEY>` from the environment, else the `config:` block of its `modules:` entry in `amp.yaml`, where a value may be an `${env:...}` or `${file:...}` reference.
- `rt.Logger()` is a logger tagged with the module.

### Routes

```go
rt.Handle(http.MethodGet, "/tags", "widget-read", func(w http.ResponseWriter, r *http.Request) { ... })
```

`Handle(method, path, perm, h, opts...)` mounts `/m/<name><path>` on the API port, beside the core's `/api/v1`, with the core's sign-in, permission check, organization from the token and audit trail. Methods are `GET`, `POST`, `PUT`, `PATCH` or `DELETE`; `AlsoRequires`, `AnyPermission`, `PlatformAdmin` and `AllowOrgOverride` apply only to these public routes. `perm` is the module's own permission (short name) or a core one. A `GET` may pass `""` (any signed-in user of the organization); every other method must name a permission, which also names its audit action unless an option names another. Paths under `/internal` belong to the workload routes below. `rt.Can(ctx, perm)` checks a permission inside a handler.

Inside a handler or MCP tool:

- `modkit.OrgID(ctx)` is the organization the request acts on (the caller's, or the one an [allowed org override](#route-options) switched to) and `modkit.PrincipalFrom(ctx)` the caller: `User` is the token subject (the signed-in user, or the client of a client-credentials token) and `ClientID` the OAuth client the token was issued to. The PoC's `Principal` also has a `Role`; Lite grants scopes rather than one role, so it is left out, and `rt.Can` answers permission questions.
- `rt.Track(ctx, event, props)` reports the request as feature usage named `event`, with `props` as its dimensions, after the response is written and with its status. A second call in the same request adds to `props`, later values winning, for dimensions a handler learns as it runs. It does nothing outside a request to one of the module's routes, or when the installation does not report feature usage (`MOESIF_ENABLED`). Feature names follow the product's feature-usage taxonomy, which `middleware/growthanalytics/taxonomy_test.go` checks for the gateway's routes; for a new module, add its packages to `routeDirs` there and declare its features in `middleware/growthanalytics/taxonomy.yaml`.

`HandleInternal(method, path, scope, h, opts...)` mounts `/m/<name>/internal<path>` on the internal server (:9243) for the module's own workloads. Callers present a workload token carrying `scope` as `Authorization: Bearer`; `modkit.WorkloadFrom(ctx)` returns who they are (module, workload, instance, subject, organization, scopes). While the module is inactive these routes answer `401 {"error":"module_inactive"}`. A workload gets its tokens from:

- a [heartbeat](#workloads), with a credential: the usual way for a service workload;
- `rt.IssueToken(ctx, modkit.TokenRequest{Subject, OrgID, Scopes, TTL, Claims})`: a short-lived token (15 minutes by default, at most an hour) the module mints itself, for a job it starts.

Tokens are RS256, signed with the core's agent-token key, with issuer `agent-manager-service/workload`, audience `amp-module:<name>` and type `amp-workload+jwt`. The public API refuses them.

#### Route options

Options declare what a route does besides needing its permission, in place of the core's tables keyed by its own route patterns:

```go
rt.Handle(http.MethodPost, "/widgets/{name}/archive", "widget-write", archive,
	modkit.Audit("widget-archive"), modkit.Emits(WidgetArchived))
rt.Handle(http.MethodGet, "/secrets/{name}", "widget-read", read, modkit.Audit("secret-read"))
```

- `modkit.Audit(action)` records the route under one of the manifest's `AuditActions` (or another of its permissions) instead of its permission, for a permission that gates several operations (create, rotate and revoke). On a read it also records the read, for reads that disclose credentials or security settings; other reads are not recorded. Every route that changes state is recorded either way.
- `modkit.AuditCoalesce(window)` records the route at most once per caller per window, for routes a machine polls.
- `modkit.AuditFailClosed()` makes the route refuse to run unrecorded: the core writes an intent record before the handler runs and answers 503 when it can't, and the route's record names that intent record (`attemptEventId`). Use it for routes whose effect lands outside the database, such as minting a credential a runtime receives.
- `modkit.Emits(event)` raises a webhook event of the manifest's (one with a `Scope`) after a 2xx response, with the response body as its data; `modkit.EmitsWithoutData(event)` leaves the body out, for responses that carry a secret.
- `modkit.AlsoRequires(perms...)` makes the route need these permissions too, all of them (an environment tier on top of the capability); the route's own permission still names its audit action.
- `modkit.AnyPermission(perms...)` lets in a caller holding any one of them, on a route registered with no permission of its own; a route that changes state also names its action with `modkit.Audit`.
- `modkit.PlatformAdmin()` admits only callers from the platform-admin organization, for a route that reads across every organization. Its path must not name an organization.
- `modkit.AllowOrgOverride()` lets a deployment's control plane drive an org-scoped route on a tenant's behalf. A request with the `X-Impersonate-Org` header acts on that organization when the deployment's override resolver (`app.Options.OrgOverrideResolver`) allows it, and is refused with 403 otherwise; the path must name the target organization, and `modkit.OrgID` returns it. Without a resolver, a request with the header is refused and one without it is unaffected. Only public routes whose path names `{orgName}` may use it.

A route naming an action or event its manifest doesn't declare keeps the module from activating.

#### Audit records

Every route that changes state gets one audit record, written by the core after the handler returns, with the caller, the outcome from the status code, and the project and environment from the path (`{projName}`, `{envName}`, `{envID}` or `{environmentId}`, else the `environment` query parameter). A handler, or a service it calls, adds to that record:

```go
modkit.AuditResource(ctx, "widget", w.ID, w.Name)
modkit.AuditDetail(ctx, "colour", w.Colour)
```

- `modkit.AuditResource(ctx, kind, id, name)` names what the request acted on.
- `modkit.AuditDetail(ctx, key, value)` adds a field. The key must be one the action declares in its manifest `Details`, with a kind: `identifier`, `name`, `email`, `name-list`, `count`, `flag`, `enum`, `fingerprint` or `url`. Undeclared keys are dropped when the record is written, a `url` loses its credentials and query, and values are strings, booleans, numbers or string lists, so a request payload cannot reach the trail. An unknown kind keeps the module from activating.
- A handler that refuses a request for a reason only the module can check answers 403 and sets `reason` (and `missingScope` for a missing scope); every action may carry those two. The record's outcome is then a denial.

The same calls work in an MCP tool, where they add to the tool call's record, and do nothing outside a request, so services stay testable without audit wiring.

#### Fixed internal paths (temporary)

`HandleInternalAt(method, absPath, h, opts...)` mounts a route at a fixed path below `/api/internal/v1` on the internal server, while the module is active (404 otherwise). The core doesn't authenticate its callers: the handler does, and a route that changes state names its audit action with `modkit.Audit`. It exists only so the AI Gateway can keep the paths its released controllers call (`/api/internal/v1/...`, with an `api-key` header) until they can register as workloads ([#2493](https://github.com/wso2/agent-manager/issues/2493)); it goes away then. `TestHandleInternalAtUsers` in `internal/modhost` lists the modules allowed to call it. Use `HandleInternal` instead.

Since the core doesn't know who called such a route, the handler tells it with `modkit.AuditInternalCaller(ctx, modkit.InternalCaller{...})` once it has authenticated the caller, so the route's record names it. With `Refused` set it records an authentication failure instead, at most once a minute per source and reason; the credential itself is never passed. It goes away with `HandleInternalAt`.

### Background workers

```go
rt.Go("reconciler", func(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(30 * time.Second):
			...
		}
	}
})
```

`rt.Go(name, fn)` runs `fn` on every replica where the module is active, from when the activation is in effect until the module is disabled or uninstalled, or the server stops. Then `fn`'s ctx is cancelled and it must return: a worker gets 10 seconds before it is abandoned. A module that stays active keeps its running workers when another module changes, so a reload never starts a second copy on a replica. A worker that returns an error or panics is restarted with backoff (1 second, doubling, at most a minute); one that returns `nil` is done. Commands such as `amp module install` run no workers. Work that must happen once across replicas needs its own claim, such as a row lock or a lease in the module's tables.

### MCP tools

```go
rt.MCPTool(modkit.MCPTool{
	Name: "testmod_count_tags", Description: "Counts the agents testmod has tagged.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"tag":{"type":"string"}}}`), ReadOnly: true,
}, "widget-read", func(ctx context.Context, args json.RawMessage) (any, error) { ... })
```

`rt.MCPTool(tool, perm, h, opts...)` adds a tool to the core's MCP server at `/mcp`. The server lists the active modules' tools on every request, so they appear and disappear with the module. A call needs `perm` (never `""`, unlike a `GET` route); tool names are lowercase letters, digits and underscores, starting with a letter, at most 64 characters. A call is audited under its name unless `modkit.Audit` names another (a read-only tool's calls only with `Audit`). The handler gets the call's arguments as sent and a ctx carrying the caller's organization; its result goes back as JSON. A `modkit.Error` is shown to the caller; any other error is reported as a failure without its text. Tool names are lowercase with underscores, unique across the core and every module: a module whose tool clashes with another module's doesn't activate, and one named like a core tool is never listed.

### Hooks

Hooks run synchronously inside the core's create, update or delete of an agent or environment, and the delete of a project:

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
| `Finalize` | Inside the transaction that deletes the resource, in reverse activation order | Agents and environments: no, errors are logged and the delete goes ahead. Projects: yes, an error rolls the delete back |

- A request addresses a module in an `extensions` object: `{"extensions": {"testmod": {"tag": "x"}}}` on agent create and update. The hook gets its own block in `req.Extension`. A block for a module that isn't active, or has no hook on that resource, is a 400.
- Write through `req.Tx`, never `rt.DB()`: each hook runs in its own savepoint, so its writes commit or roll back with the resource, and a failing or timed-out hook's writes are undone without breaking the core's transaction. `req.Tx` stops working when the hook returns.
- Each hook gets its own deep copy of `req.Object`; changing it affects nobody else.
- Each hook has 5 seconds (`modkit.HookTimeout`) and all hooks of one operation share 10 (`modkit.OperationTimeout`).
- Environments are only created with an organization, so today they get `Apply` hooks once, when an org is created; a failure there is logged and the org is still created.
- Projects take only `Finalize`, when a project is deleted, after the core has checked that no agent is left. It is how a module keeps a project its rows still belong to, as a foreign key would: return `&modkit.Error{Status: http.StatusConflict, ...}` while they exist, and delete what the project takes with it otherwise.

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
- A route that emits a webhook event (`modkit.Emits`) can add to it from its handler: `modkit.EventEnvironment(ctx, name)` sets the environment when the path does not name it (an id in the path, a name in the body), and `modkit.EventData(ctx, key, value)` adds a field to the event's data. Both do nothing outside such a route.

### Capabilities

A capability is a versioned Go interface in `modkit/contracts/<name>v<N>/`, with its id as a constant:

```go
package llmv1

const ID = "llm.proxy/v1"

type Proxy interface {
	ValidateProviders(ctx context.Context, orgID string, providers []string) error
}
```

- The provider calls `rt.Provide(llmv1.ID, impl)` for each id in its `Provides`.
- A consuming module lists the id in `Requires` or `Optional` and resolves it at call time with `modkit.Lookup[llmv1.Proxy](rt, llmv1.ID)`. The core does the same with `coreext.Lookup[llmv1.Proxy](llmv1.ID)`.
- Lookups always see the current set of active modules. `ok` is false when no active module provides it, so handle that, typically with a 400 naming the capability.
- A capability is a call between modules, not a way for the core to drive one. The core never calls `llm.proxy/v1` or `mcp.proxy/v1`; agent bindings reach the gateway through its extension block and hooks (see [Where the gateway stands](ai-gateway.md#where-the-gateway-stands)).
- A new major version is a new id (`llm.proxy/v2`) and package; providers can offer both while consumers move.
- **Core capabilities.** The core provides a few capabilities itself, listed in `modhost.CoreCapabilities` (today `identity.directory/v1`). A module lists one in `Requires` like any other; it is always satisfied, and no module may provide it (a manifest that does is refused at startup). The server supplies the implementations when it attaches the host; a CLI host has none, so a lookup there returns `ok` false.

### Optional interfaces

- `modkit.Installer`: `OnInstall(ctx, rt)` runs once when the module is installed, to catch up with core state that existed before it (agents created earlier, for example). If it fails, the module stays installed and the error is reported.
- `modkit.InitComponent`: `Init(ctx, req)` writes the module's own files at `amp init <module> DIR`, the way the core writes ThunderID's at `amp init thunder DIR`, for a service the module ships beside the server. It runs before any server, with no Runtime: `req.Config` reads the module's settings as `rt.Config` does, `req.Dir` is where its files go, and `req.CoreOwner` is the `uid:gid` for files the core reads back. It runs on every `amp init`, so it keeps what an earlier run generated. `pkg/initfiles` has the helpers.
- `modkit.HealthReporter`: `Health(ctx, rt) map[string]any` adds the module's own detail to `amp module health`. It runs within 5 seconds and must read only shared state (its tables), never one replica's memory, so every replica gives the same answer.
- `modkit.ExternalMigrator`: the schema of a store outside the core database (see [Stores outside the core database](#stores-outside-the-core-database)).

## Workloads

A workload is a part of the module that runs outside the binary: a gateway runtime, a worker pool, a job. Workloads never touch the database. They call the module's internal routes with tokens the core issues. The module declares them in its manifest; from `testmod`:

```go
Workloads: []modkit.WorkloadSpec{{
	Name: "pinger", Kind: modkit.WorkloadService, Trusted: true, Required: true,
	Scopes:    []string{"ping"},
	Protocols: []string{"testmod-ping/v2", "testmod-ping/v1"},
	Endpoints: []modkit.Endpoint{{Name: "events", Path: "/events", WebSocket: true}},
}},
```

- **Kind:** a `service` runs all the time and heartbeats; a `job` runs once per unit of work and gets a token from `rt.IssueToken`, not a credential.
- **Trusted:** whether it runs only first-party code. **Image:** the image this module version expects, if it ships one.
- **Scopes:** the module-local scopes its tokens carry.
- **Protocols:** what this module version accepts in its heartbeats, newest first. Accepting N and N-1 lets old and new instances overlap during a rollout. A heartbeat with any other protocol is refused.
- **Required:** the module is `degraded` while no instance of it is connected.
- **Endpoints:** internal routes the workload uses, by name. Each path must be one the module registers with `HandleInternal` (a `GET` route for a WebSocket), or the module doesn't activate. `rest`, the module's internal root, is always reported and can't be declared.

### Credentials

A service workload gets a credential from an operator, or from the module:

```bash
amp module credential <module> <workload> [--subject S] [--out FILE]
```

- It prints the credential once on stdout (the id and subject go to stderr). With `--out`, it writes the credential to FILE, readable by its owner only; if FILE already holds one it is kept and nothing is minted, so a bootstrap step can run on every start.
- `--subject` names who uses it (default: the workload's name). Tokens carry it as their subject.
- The module must be active. Only a hash of the secret is stored, in `workload_credentials`.
- A module mints its own with `rt.IssueCredential(ctx, modkit.CredentialRequest{Workload, Subject, OrgID})`, for example one per registered runtime, and revokes one with `rt.RevokeCredential`. Revoking refuses the credential's next heartbeat and drops its instances from health; tokens already issued stay valid until they expire. Purging a module (`amp module uninstall --purge`) revokes all its credentials.

### Heartbeat

A workload is configured with two things: the heartbeat URL and its credential. Every 10 seconds or so it sends:

```http
POST /api/internal/v1/workloads/heartbeat
{"credential": "<credential>", "instance": "gateway-0", "protocol": "amp-gateway/v1", "version": "1.4.0"}
```

`instance` names the process (default `default`) and `version` is shown in health. A heartbeat is answered with a 15-minute token and the workload's endpoints as absolute URLs:

```json
{
  "token": "eyJ...",
  "tokenType": "Bearer",
  "expiresAt": "2026-11-18T10:15:00Z",
  "module": "ai-gateway",
  "workload": "gateway-controller",
  "endpoints": {
    "rest": "https://amp-api:9243/m/ai-gateway/internal",
    "websocket": "wss://amp-api:9243/m/ai-gateway/internal/ws/gateways/connect"
  }
}
```

- **Endpoints.** A workload builds every URL from `endpoints`, re-reads them on every heartbeat, and reconnects when one changes, so moving a module's internal routes needs no change to the workload. `rest` is present when the module has internal routes; the others are the ones the workload declares. WebSocket endpoints use `wss` (or `ws` for `http`).
- **Base URL.** `INTERNAL_SERVER_PUBLIC_URL` if set (the internal server's counterpart of `SERVER_PUBLIC_URL`, for a proxy in front of it); otherwise the scheme and host the heartbeat arrived on, honouring the first `X-Forwarded-Proto` and `X-Forwarded-Host` when they are valid, as the console's `config.js` does. The core logs the endpoints it gives each new instance.
- **Registry.** Each heartbeat refreshes a `workload_instances` row (credential, instance, protocol, version, last seen). The first heartbeat of an instance is audited (`workload-token:exchange`) before its token is returned; refreshes are not.
- **Refusals.** The body is `{"error": "<code>", "message": "..."}`:

  | Status | `error` | Meaning | The workload |
  |---|---|---|---|
  | 400 | `invalid_request` | No credential or protocol, or a value with spaces | Fix its configuration |
  | 401 | `invalid_credential` | Unknown or revoked credential | Stands by and retries |
  | 401 | `module_inactive` | The module is disabled or not installed | Stands by and retries |
  | 409 | `unsupported_protocol` | The module doesn't accept this protocol; `protocols` lists what it does | Stops: deploy a matching version |

  Internal routes answer `401 module_inactive` too while the module is inactive, `401 invalid_token` for a missing or bad token, and `403 insufficient_scope` for a token of another module or without the route's scope.

### Health

```bash
amp module health [<module>]
```

reports each installed module (or one) as `ok`, `degraded` or `inactive`, with its workloads and their connected instances (those that heartbeated in the last 30 seconds), and the module's own `HealthReporter` detail. A module is `degraded` when a required service has no connected instance or it failed to activate. Health is read from the database, so every replica, and the command, agree.

## Connecting to the core

| You need to | Use |
|---|---|
| Refuse or extend a create, update or delete of an agent or environment, atomically | A hook |
| React after something happened, where a delay and a retry are fine | An event subscription |
| Offer a service other modules or the core call directly | A capability |
| Read orgs, projects, agents or environments | `rt.Core()` |
| Keep a secret value | `rt.Secrets()` |
| Serve your own API | `rt.Handle`, described in your own OpenAPI document |
| Let your own workloads call you | `rt.HandleInternal`, with the workload declared in the manifest |
| Run something in the background | `rt.Go` |
| Offer tools to MCP clients | `rt.MCPTool` |

A module must not:

- import the core's packages (`services`, `repositories`, `controllers`, `models`, ...): see [Module boundaries](#module-boundaries) for what it may import (`tests/modules/testmod` and `modules/aigateway` use only `modkit`, `pkg/` and their own packages);
- read or write core tables, or another module's tables; use `rt.Core()`, a capability or an event instead;
- be named after a core permission resource, or declare permissions outside its own namespace;
- do work in `Activate` beyond registering: it runs on every replica on every module change.

### The AI Gateway

The `ai-gateway` module runs on this contract alone. What it does as a module, its settings, and how the gateway that ships with Agent Manager registers itself are in [The AI Gateway module](ai-gateway.md).

### Modules and vaults

Modules and vaults are separate extension points. A module is a feature: it is installed, activated and uninstalled at run time through `modkit.Register`, and it reaches secrets only through `rt.Secrets()`. A vault is infrastructure: it decides where secret values are stored for the core and every module, is compiled in with `vault.Register`, and is chosen once at boot by `SECRET_VAULT_PROVIDER` (readers by `SECRET_VAULT_READERS`), reading its own settings from the environment. A module never picks or implements a vault, and switching vaults needs no module change.

## Module boundaries

A module talks to the core only through the contract. The import rules make that checkable: anything a module can import either is the contract or cannot reach the core's internals.

### What a module may import

| Import | Why it is allowed |
|---|---|
| `modkit` and `modkit/contracts/...` | The contract: the Runtime, the manifest types and the capability interfaces. |
| `pkg/...` | Leaf libraries with no domain and no state (below). |
| Its own packages (`modules/<name>/...`) | The module's code. |
| The standard library and third-party modules in `go.mod` | |

Everything else under `agent-manager-service/` is the core (`services`, `repositories`, `controllers`, `models`, `utils`, `config`, `db`, `middleware`, `audit`, `rbac`, ...) and is off limits. Another module's packages are off limits as well; use its capability. The `depguard` linter (`agent-manager-service/.github/linters/.golangci.yaml`) refuses a core import in a module's non-test files (it does not catch an import of another module's packages; review does); module tests and the gateway's test helpers (`gwtest`) are exempt.

### Leaf libraries (`pkg/`)

`agent-manager-service/pkg/` holds code that the core and modules both need and that has nothing to do with AMP's domain:

| Package | What it is |
|---|---|
| `pkg/httpx` | JSON success and error responses, `ValidationError`, the classification errors (`ErrInvalidInput`, `ErrNotFound`, ...), the generic error codes, and `ClientIP` (the caller's address, leftmost `X-Forwarded-For` hop). |
| `pkg/aesgcm` | `Encrypt` and `Decrypt` bytes with AES-256-GCM under a 32-byte key you pass in. |
| `pkg/dbx` | Portable SQL helpers on a `*gorm.DB` you pass in: `InsertIfAbsent`, `Upsert`, `ClaimRows`, `TryLock` (a lease row in a table you name, such as your module's own `mod_<module>_locks`), `LockRow(s)`, keyset pages (`After`), `Chunks`, `JSON`, `Now`. |
| `pkg/dberr` | Classifies driver errors (duplicate key, foreign key, ...) the same way on SQLite and PostgreSQL. |
| `pkg/sqlitedialect` | The GORM SQLite dialect the core opens databases with. |
| `pkg/ssrf` | Checks that an outbound URL points at a public host, and `ErrInvalidURL`. |
| `pkg/initfiles` | Writes `amp init` files: only when they change, generated once and kept, handed to a `uid:gid`. |
| `pkg/logger` | The request-scoped `slog` logger and the correlation id carried in the context. |

A package belongs in `pkg/` only if all of these hold:

- **It imports no core package.** Only the standard library, third-party modules and other `pkg/` packages.
- **It holds no global state.** No database handle, no configuration read, no registry: it works on what the caller passes in (`pkg/dbx` takes the `*gorm.DB`, it never opens one).
- **It knows nothing about AMP's domain.** No agents, projects, permissions or audit records. A helper that needs those is a core service, and a module reaches it through a capability.

The proof of concept allows modules to import only `modkit`. `pkg/` is a deliberate extension of that rule: its packages cannot reach into the core's data or services, so importing them cannot bypass the contract, while copying them into every module would let the copies drift and versioning them as contract would freeze helpers that change freely. If a helper fails one of the three tests above, it is not a `pkg/` library.

### Reaching the core

A module reaches core data and services in one of three ways:

| Use | For | Example |
|---|---|---|
| `rt.Core()` | Reading core resources (organizations, projects, agents, environments) and the few core writes a module needs inside a hook (`LockAgent`, `EnsureOrg`). It is org-scoped: every call names the organization, and another organization's resource is not found. | The gateway resolves an agent's name with `GetAgent`. |
| A capability | A core or module service with behaviour behind it, versioned so it can change without breaking its users. | The gateway's agent-identity pages manage roles and groups through `identity.directory/v1`. |
| `rt.DB()` | The module's own tables (`mod_<name>_*`), never the core's or another module's. | The gateway's repositories. |

Add a capability when a module needs the core (or another module) to *do* something, or to read something `Core()` does not offer. Add a `pkg/` library only for code with no domain and no state that both sides would otherwise copy. If you are unsure, it is a capability: a `pkg/` package can never reach core data, so it cannot replace one.

Capabilities today:

| Capability | Version | Provider | Methods | Consumers |
|---|---|---|---|---|
| `identity.directory/v1` (`modkit/contracts/identityv1`) | 1 | the core | `For(org, environment)` opens an environment's directory; the client lists, gets, creates, updates and deletes groups and roles, changes group members, role permissions and role assignments, and ensures or removes MCP resource servers. `ListProvisionedAgents(org, environment)` lists the agents the core provisions identities for there, from the core's own records. In Lite only the default environment has one (the platform ThunderID); any other answers `ErrNotProvisioned`. | `ai-gateway` (agent-identity pages, MCP proxy scopes) |
| `llm.proxy/v1` (`modkit/contracts/llmv1`) | 1 | `ai-gateway` | `ValidateProviders` | none in this repository |
| `mcp.proxy/v1` (`modkit/contracts/mcpv1`) | 1 | `ai-gateway` | `ValidateProxies` | none in this repository |
| `traces.query/v1` (`modkit/contracts/tracesv1`) | 1 | `observability` | `QueryTraces`, `QueryTraceSpans` and `GetSpanDetails`, for a `Scope` of IDs (organization required; project, agent and environment optional). Each refuses a `Scope` of another organization than the ctx's with `ErrForeignOrg`, and answers an unknown project, agent or environment with `ErrNotFound`. | none in this repository |

## Testing

- **Unit tests** for the module's own logic, with no database. The host's lifecycle (install, enable, disable, dependency resolution, reconcile) is covered in `internal/modhost`; you don't need to repeat it.
- **Integration tests** in the style of `tests/module_contract_test.go` (`TestModuleContract`): build a host with `modhost.NewWith(db, driver, log, []modkit.Module{...})` so the test binary stands in for a distribution, then check what your module changes over the real API: routes 404 before install and after disable, permissions 403 and 200, hooks refusing and writing, events delivered after commit, capabilities found. Run them on both databases: `make test-integration` uses SQLite, and `DB_DRIVER=postgres make test-integration` uses the server in `.env.test`.
- **Invariant tests** in `api/` walk every route and permission of the modules they list: every route is authorized (`route_authz_invariant_test.go`), every mutating route is audited (`audit_coverage_test.go`, and for module routes `module_routes_invariant_test.go`, which also checks that each action is declared), every permission has one owner and reaches roles and ThunderID (`permission_owner_invariant_test.go`), a module's OpenAPI document describes exactly the routes it mounts (`checkModuleSpec`), and the core's document holds no `/m/` path. For a new module, blank-import it in `api/route_authz_invariant_test.go` and `api/permission_owner_invariant_test.go`, add its route package to `routePackages` (and its permission package to `modulePermissionPackages`), and add `Test<Module>RoutesAreAudited` and `Test<Module>SpecMatchesRoutes` to `api/module_routes_invariant_test.go`. If it calls `rt.Track`, add its packages to `routeDirs` in `middleware/growthanalytics/taxonomy_test.go` and its features to `taxonomy.yaml`. They run in `make test-unit`; `make security-test-static` at the repository root runs the authorization ones alone.
- **Smoke tests** against a running stack: `make up` then `make smoke`, and `make up-core` then `make smoke-core` (`deployments/standalone/scripts/smoke.sh amp|amp-core`); `smoke.sh amp-off` checks the full stack with the gateway disabled. The checks name the gateway's routes, so add checks for a new module's routes. Run both whenever a change touches the line between the core and a module.

## API document

A module with routes describes them in its own OpenAPI 3 document, `api/openapi.yaml`, embedded and returned from the optional `modkit.APIDescriber` interface:

```go
//go:embed api/openapi.yaml
var openAPI []byte

func (module) OpenAPI() []byte { return openAPI }
```

- `servers` is `[{url: /m/<name>}]`, so paths are relative to the module (`/tags` for `/m/testmod/tags`), and `info.x-amp-module` is the module's name.
- It is self-contained: copy the common schemas it needs, such as `ErrorResponse`, rather than referring to the core's document. The core's document, `docs/api_v1_openapi.yaml`, holds only `/api/v1` paths.
- Its internal (workload) routes are not documented.
- `make spec MODULE=<name>` generates its Go types into `modules/<name without dashes>/<package>`, where the package is the document's `info.x-go-package` or `<name without dashes>spec` (`MODULE_DIR=<dir>` names a directory instead). `make spec-all` generates the core's and every module's, and `make codegenfmt-check` runs it.

## Console UI

The console's pages live in UI modules under `console/workspaces/ui-modules/<module>/`, one package per page under `pages/<feature>/`. UI modules are named for the product area and need not match the backend module (the gateway's pages are in `ai-gateway`). To add pages:

1. Create the page package and its `plugin.ts`, list it in the UI module's `src/index.ts`, and, for a new UI module, add it to `builtinModules` in `console/workspaces/core-ui/src/plugins/builtinModules.ts`. [console/AGENTS.md](../../console/AGENTS.md) has the recipe.
2. List each page in the backend module's `UI.Pages` as `<UI module>/<plugin id>`.

After sign-in the console reads `GET /api/v1/ui-manifest`. A UI module with no listed page is hidden, and so is every unlisted page; `acp-core` is always on. The manifest is read once per sign-in, so a module enabled later shows after the user signs in again.

## Operating modules

```bash
amp module list                       # NAME VERSION STATE PROVIDES REQUIRES
amp module install <name> [--with-deps]
amp module enable <name>
amp module disable <name>
amp module uninstall <name> [--purge]
amp module credential <name> <workload> [--subject S] [--out FILE]
amp module health [<name>]
```

In the Compose stack, run them in the API container (for example `docker compose exec amp-api /go/bin/amp module list`). Changes reach running servers within seconds. `install --with-deps` also installs the modules that provide what it `Requires`; disable and uninstall are refused while an active module requires it.

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
- No `modules:` key, or one whose entries are all commented out, means no reconcile: manage modules with `amp module` instead; the server only installs the distribution's defaults that were never installed. `modules: []` declares an empty set.
- `AMP_MODULES` overrides the list: `AMP_MODULES=widgets,reports=off`, or `AMP_MODULES=none` for the empty set.
- Listing a module the binary doesn't have stops the server at boot.

`CONSOLE_UI_MODULES_ENABLE` turns on extra console UI modules or pages whatever the manifest says (`agent-runtime agent-portal`, or `<module>/<page>`), for pages that need no backend module.

When a module is disabled:

- its routes answer 404 at once (its fixed internal paths too; its internal routes and its workloads' heartbeats answer `401 module_inactive`, so workloads stand by), and its hooks, event handlers and capabilities stop;
- its workers are stopped on every replica, its MCP tools leave `/mcp`, and its webhook events leave the event catalog;
- a request with an `extensions` block for it is a 400, and capability lookups fail (a core feature that needs it answers 400 naming the capability);
- its pages leave the UI manifest, and its permissions leave file sign-in roles and the console's sign-in scopes;
- its tables and data stay, and events queued for it wait up to 7 days. Uninstall keeps the data too, but drops queued events. Installing or enabling again picks up where it left off.

`amp module uninstall <name> --purge` (also on a module already uninstalled) empties its `mod_<name>_*` tables, deletes its secrets, revokes its workload credentials and forgets it, in one transaction. The tables themselves stay, as in the proof of concept, so installing it again runs no migration; reference data a migration seeded is restored by the module's `OnInstall` (the gateway re-creates its drift scan lease row). Purge does not touch a store outside the core database: the observability module's spans stay in ClickHouse until its TTL removes them.

## Beyond the proof of concept

The contract follows the public proof of concept, [a5anka/amp-modular-poc](https://github.com/a5anka/amp-modular-poc). The tables below list what is built here that the proof of concept does not have, or does differently, and why. Each entry is described in full in the section it links to. When you add to the contract or change a rule, add a row here in the same change.

### Additions to the contract

| Addition | Why |
| --- | --- |
| [`Manifest.DisplayName`, `Permission.DisplayName`](#the-manifest) | ThunderID shows permissions to administrators, and a readable name is needed there. |
| [`Manifest.AuditActions`](#route-options) and the route options `modkit.Audit`, `AuditCoalesce` | Every core route is audited under a declared action; module routes keep that rule, and machine-polled routes need coalescing. |
| [`AuditAction.Details`, `modkit.AuditResource`, `AuditDetail`](#audit-records) | A module's records say what was acted on and how, as the core's do; declaring each key with its kind keeps redaction working for module records. |
| [Route option `modkit.AuditFailClosed`](#route-options) | Routes that mint or revoke credentials a runtime receives must not run unrecorded; the core writes the intent record because only it holds the recorder. |
| [`modkit.AuditInternalCaller`](#fixed-internal-paths-temporary) | Time-boxed, with `HandleInternalAt`: those routes authenticate their callers themselves, so only the handler can name the caller or report a refused credential. |
| [Scoped `Manifest.Events`](#events) and the route options `modkit.Emits`, `EmitsWithoutData` | Module events can be offered as webhooks, which needs a scope and a description per event. |
| Route options [`AlsoRequires`, `AnyPermission`, `PlatformAdmin`](#route-options) | Some gateway routes need several permissions, one of several, or the platform-admin organization; one permission per route could not express them. |
| Route option [`AllowOrgOverride`](#route-options) | A deployment's control plane lists gateways and sets gateway identity providers on a tenant's behalf. The core already lets its own routes opt into the org override; the gateway did the same inside its handlers, which bypassed the core's checks (path must name the target, a refused override is a 403) and its audit record. |
| [`Runtime.Go`](#background-workers) | Modules need workers (reconcilers, sweepers) that start and stop with the module on every replica. |
| [`Runtime.MCPTool`](#mcp-tools) | Modules contribute tools to the core's `/mcp` server, authorized and audited like core tools. |
| [`Runtime.Secrets`](#data) | Modules keep secret values sealed by the core's vault without their own key handling; `uninstall --purge` deletes them with the module's tables. |
| [`Runtime.Config`](#data) | A module reads its own settings (`MODULE_<NAME>_<KEY>`, or its entry in `amp.yaml`) instead of the core's configuration. |
| [`modkit.EventEnvironment`, `EventData`](#events) | A route's webhook event takes its environment from the path; a module route that names it by id or in the body sets it, and adds fields the response does not carry (an API key's name, never the key), as core handlers do. |
| [`Runtime.PublishTx`](#events) | An event written in the module's own transaction is delivered only if that transaction commits. |
| [`Runtime.Track`](#routes) | The core reports feature usage per route; module routes report theirs the same way. It does nothing when reporting is off. |
| [`Runtime.IssueCredential`, `RevokeCredential`](#credentials) | A module mints workload credentials for the runtimes it registers itself, not only through `amp module credential`. |
| [`Runtime.HandleInternalAt`](#fixed-internal-paths-temporary) | Time-boxed: keeps the AI Gateway's released controllers working on their fixed `/api/internal/v1` paths until they can register as workloads (#2493). A test lists the modules allowed to call it. |
| [Wider `CoreAPI`](#data): organizations, projects and environments, lookups by name, `LockAgent`, `EnsureOrg` | The proof of concept reads agents only; the gateway addresses projects, agents and environments by name and must lock an agent or create an organization inside a hook. |
| [`modkit.OrgID(ctx)`](#routes); `OrgID` on `HookRequest`, `Event` and `TokenRequest` | Lite is multi-organization, so every call and event is org-scoped. |
| `Event` gains `ID` and `Subject`; the PoC's `AgentCreated` payload type is gone | Handlers dedupe on `ID`, since delivery is at least once; `Subject` names what the event is about. |
| [`modkit.CheckName`](#the-manifest) | Names are at most 24 characters and may not give overlapping `mod_` table prefixes; the PoC only rejects duplicates. |
| [`Apply` hook phase, `HookRequest.Tx`, project hooks](#hooks) | A module stores its part of a core resource (such as an agent's gateway bindings) in the core's transaction, and can refuse or clean up a project delete. |
| [`modkit.Reject`, `modkit.Error`](#hooks) | A hook refusal reaches the caller as a 400 or a chosen status with a message, not a 500. |
| [`UI.Pages`](#the-manifest) | The console lists a module's pages, filtered by permission, from the manifest. |
| [`WorkloadSpec.Endpoints`](#workloads) and endpoints in the heartbeat response | A workload learns every URL from its heartbeat, so it holds no module paths and follows a module's internal API when it moves. |
| [`modkit.APIDescriber`](#api-document) | Each module describes its routes in its own OpenAPI document, kept out of the core's. |
| [`modkit.InitComponent`](#optional-interfaces) | A module writes its own `amp init` files (the gateway's controller configuration). |
| [`modkit.ExternalMigrator`](#stores-outside-the-core-database) | A module that keeps its data outside the core database (the observability module's ClickHouse) has its schema applied and checked by `amp migrate`, under the same rules as SQL migrations. |
| [The default organization exists at `Activate`](#activate-and-the-runtime) | Lite is multi-organization and creates organizations on first use; a module that seeds rows for the default organization (the gateway's bundled gateway) needs it to exist before its routes are mounted. |
| [Distribution default modules](#what-a-module-is) | A full install should run the AI Gateway without an operator step, as before it was a module, while `amp module disable` or `uninstall` still sticks across restarts; a reconciled default list would undo those on every boot. |
| [Core capabilities](#capabilities) (`identity.directory/v1`) | A module reaches a core service through a versioned contract instead of a core import. The proof of concept has only module-to-module capabilities. |

### Differences in shape or rules

| Difference | Why |
| --- | --- |
| [`modkit.Principal`](#routes) has `User` and `ClientID` and no `Role` | Lite grants scopes, not one role; `rt.Can` answers permission questions. |
| [`WorkloadSpec`](#workloads) has no `Isolation` or `Egress` | No Lite workload needs them yet. |
| The PoC's `Workload`/`Service`/`Job` are `WorkloadSpec`/`WorkloadService`/`WorkloadJob`; its `Claims`/`ClaimsFrom` are `modkit.Workload`/`WorkloadFrom` (`Name`, `Instance`, `OrgID`) | The workload a token belongs to is what handlers need; scopes are checked by the host. |
| [`Runtime.DB()`](#data) is a `*gorm.DB` on SQLite and PostgreSQL, with migrations written as YAML and rendered per database ([Migrations](#migrations)) | Lite runs on both databases; the PoC is PostgreSQL only (`*pgxpool.Pool`). |
| `modkit.Agent` adds `OrgID`, `ProjectID`, `ProjectName` and `Provisioning`, and drops `Description` | Hooks and lookups need the agent's place and how it is provisioned. |
| The PoC's `DevWorkloads`/`DevEnv` and `WriteJSON`/`WriteError` are not carried over | There is no dev workload runner; handlers use `pkg/httpx`. |
| [Workload tokens](#heartbeat) are RS256 with the core's agent-token key, not EdDSA | The runtimes already trust that key. |
| [Modules may import `pkg/`](#leaf-libraries-pkg) as well as `modkit` | `pkg/` holds domain-free libraries with no core imports and no state, so they cannot bypass the contract; copying them into every module would let them drift. |
| [The import rule](#what-a-module-may-import) applies to non-test files only | Module tests may use the core's test harnesses (`db/dbtest`, `internal/modhost`). |
| [The AI Gateway offers no environment attach](ai-gateway.md#where-the-gateway-stands) on `environment.created` | Lite has one environment per organization, created with it, and a gateway serves one environment, so there is nothing to offer; revisit with more environments. |
| Module permissions and audit actions may be `<resource>.<action>` (`ai-gateway:llm-providers.read`) | A module with many resources groups its permissions by resource; ThunderID accepts the dots. |
| [A refusal only the module can check](#audit-records) (the gateway's environment tier) is recorded on the route's own record, as a denial with `reason` and `missingScope`, not as a separate `authz:deny` record | The route's record already carries the caller and the outcome; a second record would need the core's audit package in the module. |
| [Module records take the project and environment from the path](#audit-records) | Handlers name only what they acted on; the path already says where. |
| Module routes never treat a permission as an "axis": the route's own permission names its action, and `AlsoRequires` permissions never do | The core's axis-permission registry existed only for the gateway's environment tier, which `AlsoRequires` now expresses. |
| Module secrets use owner kind `mod:<module>` in the core's `secrets` table | No schema change is needed. |
| Internal routes answer 404 for a wrong method, not 405 | Module internal routes are mounted per path and method, and only while the module is active. |
| No REST endpoint mints workload credentials | Credentials come from `amp module credential` (operators) or `rt.IssueCredential` (the module itself); nothing else needs one yet. |
