# Agent Manager database schema

Status: in progress, development preview. Areas are added as they are agreed. Every schema change is also recorded in the [migration tracker](migration.md).

Diagrams: [data model](data-model.png) ([HTML](data-model.html)) and [architecture](architecture.png) ([draw.io](architecture.drawio)). See [README](README.md) for how to edit them.

## Intent

AMP Lite is a version of Agent Manager that runs anywhere, with no OpenChoreo. Its own database is the only source of truth for everything the control plane manages.

The product ships as three cumulative tiers, and one schema serves all of them:

| Tier | Adds | Tables it starts to fill |
|---|---|---|
| AI Gateway | Model, MCP and agent (A2A) governance, guardrails, budgets, imported agent ID, insights, kill switch | Orgs and projects, gateways, artifacts, LLM, MCP, REST APIs, A2A, agent identities, secrets, events, audit |
| Agent Manager (today's Lite) | Managed agent ID, trace and debug, evaluation, alerts, portal, shadow AI discovery | Evaluation and alerts |
| Agent Runtime | Hosted agents and MCP servers, environments and pipelines, sandboxing, agent memory | Agent runtime, more environments and pipelines |

The first start creates every table, for all three tiers, and seeds what each tier needs (see [Seed data](#seed-data)). Tables of features a customer doesn't use stay empty, so moving up a tier needs no schema change. Environments are internal below the Agent Runtime tier: each org has one `default` environment that users never see.

## Decisions that shape the schema

- Lite only connects external agents, and the hosted routes answer 501. The agent runtime tables (builds, releases, deployments, env vars, resources, git credentials, agent kinds, sandbox settings, memory stores) and the evaluation tables (evaluators, monitors, runs, scores, alerts) are in the schema anyway, so enabling those tiers later needs no migration. Their design keeps no OpenChoreo concept: a `runtimes` row replaces the data plane, and releases and deployments replace component releases and release bindings.
- Multi-tenant. `organizations` is the root.
- Environments are hidden below the Agent Runtime tier. Every org gets one `default` environment and one `default` deployment pipeline holding it. Lite's API and console never show or accept an environment; the service always uses the org's default. Per-environment tables carry an `environment_id`, so the Agent Runtime tier adds environment rows and pipeline stages without table changes.
- Console login is file-based to start with: a default username and password belong to a default org; a file maps scopes to roles (admin, developer). No users, roles or scopes tables. Tables refer to users by username.
- Thunder ships with Lite and gives every external agent its agent ID. Thunder keeps its own separate database; ours stores only IDs returned by Thunder's API.
- Gateways belong to an org, with no limit per org. The default org's bundled gateway is running out of the box; users add more (managed or self-hosted).
- Secrets live in our database, encrypted. Production can plug in an external vault.
- Audit log (console actions) and console usage tracking are stored in our database.
- Traces, runtime logs, build logs and metrics are stored elsewhere.
- Databases: SQLite ships with Lite and holds everything. Production uses PostgreSQL 18, SQL Server 2025 or Oracle 19c.
- Lite has not been released, so the full-product tables were added to the `001` baseline rather than to later migrations. From the first release on, every change is a new numbered migration (rule 11).

## Database rules

### Column types

| Logical type | SQLite | PostgreSQL 18 | SQL Server 2025 | Oracle 19c |
|---|---|---|---|---|
| ID (UUID v7 from the app) | TEXT | UUID | CHAR(36) | CHAR(36) |
| Short text | TEXT | VARCHAR(n) | NVARCHAR(n) | VARCHAR2(n CHAR) |
| Long text | TEXT | TEXT | NVARCHAR(MAX) | CLOB |
| JSON | TEXT + json_valid check | JSONB | JSON | CLOB + IS JSON check |
| Yes/no | INTEGER 0/1 | BOOLEAN | BIT | NUMBER(1) 0/1 |
| Whole number | INTEGER | INTEGER / BIGINT | INT / BIGINT | NUMBER(10) / NUMBER(19) |
| Time (UTC) | TIMESTAMP (stored as ISO 8601 text) | TIMESTAMPTZ | DATETIME2(6) | TIMESTAMP(6) |
| Binary | BLOB | BYTEA | VARBINARY(MAX) | BLOB |

### Rules

1. JSON columns are validated by the database and never filtered, sorted or joined on. Anything queried gets real columns or a child table.
2. The app generates IDs (time-ordered UUID v7) and sets `created_at` / `updated_at`. No database UUID functions, auto-increment columns or triggers.
3. "Not set" is always NULL. No empty-string defaults (Oracle stores `''` as NULL).
4. Hard deletes. No soft-delete columns. The audit log keeps history.
5. Names of tables, columns, indexes and constraints: lowercase snake_case, at most 63 characters. Text that is unique or indexed stays within 4,000 bytes; long text and JSON are never unique or indexed.
6. Every tenant-owned table has `org_id`. A child points at its parent through a composite key that includes `org_id` (for example (`org_id`, `project_id`) → `projects(org_id, id)`), so a row can never point at another org's data.
7. One cascade path per table (SQL Server rejects more). Cascades follow org › project › agent; anything else is deleted by the service.
8. No unique keys on nullable columns (SQL Server allows only one NULL in a unique key).
9. Comparisons are case-sensitive on all four databases. On SQL Server, key text columns use a case-sensitive collation.
10. No database-specific SQL in queries: every query is written once and runs unchanged on all four databases, with no per-database repositories. Upserts are an insert in a savepoint, then an update (or nothing) on a duplicate-key error. Job claims are lease updates: select candidate ids, then `UPDATE … SET claimed_by, claimed_until WHERE id IN (…)` and the lease is free or expired, then read back the claimed rows. A row is locked for a transaction by updating it first. Singleton workers take a lease row in `locks`. Only connection setup, DDL (rule 11) and the mapping of Oracle errors differ per database. SQLite needs foreign keys switched on per connection and every transaction started as `BEGIN IMMEDIATE`. GORM's own upsert and row-lock clauses are not used: its SQL Server dialect renders `FOR UPDATE` (rejected) and `MERGE`, SQLite silently drops row locks, and the Oracle dialect has no `FOR SHARE` and doesn't translate duplicate-key errors.
11. One migration folder per database, same version numbers across all four. SQLite column changes rebuild the table.
12. No decimal or floating-point columns. A fraction is stored as a scaled whole number with the scale in the name: `sampling_percent` (1-100), `score_bp` (basis points, 0-10000 for 0.0-1.0). Averages are computed in the app or with an explicit cast, since `AVG` of a whole-number column returns a whole number on SQL Server.
13. Time on SQLite: columns are declared `TIMESTAMP` because the pure-Go driver only parses stored text back into a time for columns declared DATE, DATETIME or TIMESTAMP (a TEXT column fails to scan). Values stay ISO 8601 text, always UTC, in the driver's layout `2006-01-02 15:04:05.999999999-07:00` (pinned with `_time_format=sqlite` in the connection string). UTC values end in `+00:00`, and because `+` sorts before `.`, text order is time order even with mixed fractional seconds; a `Z` suffix would break that. Times computed in SQL (`MAX(created_at)`, `COALESCE(...)`) lose the declared type on SQLite and come back as strings: compute them in the app or scan them as text. Reading a TIMESTAMP column into a Go string does not return the stored text (the driver parses it and database/sql formats it as RFC 3339), so never compare times as strings in Go; use `CAST(col AS TEXT)` when the raw stored text is needed.

### locks

Replaces Postgres advisory locks for the reconcilers, the drift scan and gateway serialization.

| Column | Type | Null | Notes |
|---|---|---|---|
| name | short text (128) | no | PK, e.g. `reconciler.gateway` |
| owner | short text (255) | yes | Instance holding it. NULL = free. |
| expires_at | time | yes | Lease end; an expired lease can be taken over. |

Taken with the same lease update as job claims (rule 10). Rows are seeded by migrations, so taking a lock never inserts: `agent_identity.reconciler`, `gateway.drift_scan` (AI Gateway), `monitor.scheduler`, `monitor.alerts` (Agent Manager), `build.dispatcher`, `deployment.reconciler` (Agent Runtime). A lock row for a worker a tier doesn't run is never taken.

### Queue tables

Every table a worker claims rows from carries `claimed_by` (short text (255), nullable) and `claimed_until` (time, nullable): `agent_identities`, `agent_cards`, `artifact_gateway_status` (the render queue), `event_deliveries`, `event_outbox`, `monitors` (the scheduler), `monitor_runs`, `builds` and `agent_deployments`.

### Seed data

| What | Where | Tier |
|---|---|---|
| Lock rows (above) | Migration | All; each tier takes only its own |
| Default org, its Thunder org unit, a default project, the bundled gateway and its token | First start | AI Gateway and up |
| Per org: the `default` environment and the `default` deployment pipeline with that environment as its only stage | Org creation, one transaction | All; only Agent Runtime reads the pipeline |
| Built-in LLM provider templates and built-in evaluators | Code, never stored | AI Gateway (templates), Agent Manager (evaluators) |

Nothing else is seeded: evaluation and runtime tables start empty and fill when those features are used.

## Orgs and projects

### organizations

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| handle | short text (63) | no | Unique. URL name. `^[a-z0-9]+(-[a-z0-9]+)*$`, checked by the app. |
| display_name | short text (255) | no | |
| description | short text (1000) | yes | |
| thunder_ou_id | short text (64) | yes | Thunder org unit holding this org's agent identities. Not unique (rule 8); Thunder guarantees it. |
| created_by | short text (255) | yes | Username. NULL for system-created rows. |
| created_at, updated_at | time | no | |

### projects

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK. Returned as the API's project `uuid`. |
| org_id | ID | no | FK → organizations(id), no cascade. |
| name | short text (63) | no | Same pattern as `handle`. Immutable. |
| display_name | short text (255) | no | |
| description | short text (1000) | yes | |
| deployment_pipeline_id | ID | yes | FK (org_id, deployment_pipeline_id) → deployment_pipelines, no action. NULL = the org's `default` pipeline. Agent Runtime only. |
| created_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

Unique: (org_id, name); (org_id, id) for child composite keys.

### Behaviour

- First start seeds the default org (handle from config), its `default` environment, its Thunder org unit, a default project in it, and the configured default gateway with its registration token, attached to the default environment. This runs on every start: it creates the org and its default environment if missing and keeps an existing gateway and token.
- Orgs are never renamed or deleted. Creating an org creates its `default` environment in the same transaction, and its Thunder org unit; if Thunder fails, the org is not saved.
- Deleting a project is refused while it still has agents, LLM proxies or MCP servers; the conflict names what is left. Its event subscriptions and its entries in event destinations' project lists are removed in the same transaction, before the project row.
- Dropped from today's API: org `namespace`. Project `deploymentPipeline` comes back with the Agent Runtime tier as `deployment_pipeline_id`.

## Environments

### environments

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id | ID | no | FK → organizations(id), no cascade. |
| name | short text (63) | no | `default` in Lite. Same pattern as project `name`. Immutable. |
| display_name | short text (255) | no | |
| description | short text (1000) | yes | |
| is_production | yes/no | no | `0` for the seeded default. |
| runtime_id | ID | yes | FK (org_id, runtime_id) → runtimes, no action. Where hosted agents in this environment run. NULL below Agent Runtime. |
| isolation_tier | short text (16) | yes | Sandbox for hosted agents: `gvisor` or `kata`. NULL = the runtime's default (runc). |
| dns_prefix | short text (63) | yes | Prefix of hosted agents' URLs in this environment. |
| created_at, updated_at | time | no | |

Unique: (org_id, name); (org_id, id) for child composite keys.

- Lite never creates, renames or deletes environments after seeding. The service looks up the org's `default` environment (cached per org) and uses it wherever a per-environment row is read or written.
- Per-environment tables point at it through (org_id, environment_id) → environments(org_id, id), no cascade. Their rows are deleted through their owner (agent, gateway, MCP proxy), which keeps one cascade path per table (rule 7).
- OpenChoreo's data-plane reference becomes `runtime_id`; its DNS prefix and isolation tier keep their meaning.
- Agent Runtime adds more rows here, and stages to pipelines, without changing the per-environment tables.

### Deployment pipelines

The order an org's environments are promoted in (was the OpenChoreo DeploymentPipeline). Every org gets a `default` pipeline holding its default environment at position 0, created in the same transaction as the org.

- `deployment_pipelines`: id, org_id, name (unique per org), display_name, description, created_by, timestamps.
- `deployment_pipeline_environments`: PK (pipeline_id, environment_id), org_id, position (unique per pipeline, 0 = first stage). Cascade from the pipeline; the environment FK has no action, so an environment in a pipeline can't be deleted.
- `deployment_pipeline_paths`: PK (pipeline_id, source_environment_id, target_environment_id). Cascade from the pipeline; both environment FKs have no action. CHECK source <> target. A pipeline with one environment has no paths. The service checks that both ends are stages of the same pipeline.

## Agents and secrets

Lite has external agents only. The tables hosted agents need are under [Agent runtime](#agent-runtime).

### agents

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id, project_id | ID | no | FK (org_id, project_id) → projects(org_id, id), no cascade: a project with agents can't be deleted. |
| name | short text (63) | no | Unique per org (today's behaviour; gateway artifacts key on it). Immutable. |
| display_name | short text (255) | no | |
| description | short text (1000) | yes | |
| provisioning_type | short text (16) | no | CHECK `external` or `internal`. `external` in Lite; `internal` reserved for the full product. The API's `agentType.type` is derived from it (`external` → `external-agent-api`, `internal` → `agent-api`) rather than stored. |
| created_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

Unique: (org_id, name); (org_id, id) for child composite keys.

### agent_labels

| Column | Type | Null | Notes |
|---|---|---|---|
| agent_id | ID | no | FK (org_id, agent_id) → agents(org_id, id) ON DELETE CASCADE |
| org_id | ID | no | |
| label_key | short text (63) | no | |
| label_value | short text (63) | yes | Empty value stored as NULL, returned as "". |

PK (agent_id, label_key). Index (org_id, label_key, label_value). At most 10 per agent (app).

### agent_identities

One row per agent per environment (one in Lite): its Thunder identity and the provisioning queue. Replaces `agent_thunder_clients`. PK (agent_id, environment_id).

| Column | Type | Null | Notes |
|---|---|---|---|
| agent_id | ID | no | FK (org_id, agent_id) → agents CASCADE. |
| org_id | ID | no | |
| environment_id | ID | no | FK (org_id, environment_id) → environments, no cascade. |
| thunder_agent_id | short text (64) | yes | Set once Thunder returns it. |
| thunder_client_id | short text (255) | yes | |
| status | short text (16) | no | pending, in_progress, completed, failed |
| attempt_count | whole number | no | |
| last_error | long text | yes | |
| next_retry_at | time | yes | |
| claimed_by, claimed_until | short text (255), time | yes | Lease (rule 10). |
| requested_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

External agents' client secrets are never stored, as today: regenerate creates a new secret in Thunder and returns it once.

### agent_gateway_settings

One row per agent per environment. Replaces `agent_configs`. PK (agent_id, environment_id). FK (org_id, agent_id) → agents CASCADE; FK (org_id, environment_id) → environments.

- Yes/no: api_key_security, oauth_enabled, oauth_forward_token, cors_enabled, cors_allow_credentials
- JSON: oauth_issuers, oauth_audiences, cors_allow_origins, cors_allow_methods, cors_allow_headers
- Short text: oauth_header_name, oauth_header_prefix
- Whole number: timeout_seconds (NULL = gateway default)
- card_cors_enabled, card_cors_allow_origins, card_cors_allow_headers, card_cors_allow_credentials: NULL = inherit from the agent

### secrets

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id | ID | no | FK → organizations, no cascade |
| owner_kind | short text (32) | no | What uses it (llm_provider, event_destination, a2a_proxy, ...). For cleanup; not an FK. |
| owner_id | ID | no | |
| provider | short text (16) | no | `db`, or an external vault's name |
| ciphertext | binary | yes | `db` only. AES-256-GCM, nonce stored with it. |
| key_id | short text (64) | yes | Master key that encrypted it, for rotation. |
| external_ref | short text (512) | yes | Path in the external vault. |
| version | whole number | no | |
| created_at, updated_at | time | no | |

Index (org_id, owner_kind, owner_id). Owning tables reference a secret by `secret_id` (FK, no action); the service deletes the secret after the owner. Every value encrypted inline today moves here: LLM provider and LLM proxy upstream auth, MCP endpoint upstream auth, A2A upstream auth, event destination secrets.

## Gateways, LLM, MCP and A2A

Common changes to every existing table here: `ou_id` becomes `org_id` (FK); leftover `organization_name` / `org_name` / `project_name` / `agent_name` columns and `deleted_at` go (rows point at the owner's id instead); `environment_name` + `environment_uuid` become one `environment_id` FK. Columns not mentioned keep today's meaning and get the portable types.

### Gateways

- `gateways`: org_id, name (unique per org), display_name, description, functionality_type (ingress | egress | both), hosting (managed | self_hosted), vhost, runtime_url, is_critical, is_active, properties (JSON), manifest (JSON). No limit per org: first start seeds one; users add more (managed or self-hosted).
- `gateway_environments` (was `gateway_environment_mappings`): PK (gateway_id, environment_id), org_id. Cascade from the gateway. A new gateway is attached to the org's default environment automatically; Lite never asks.
- `environment_default_gateways` (replaces the `org_default_gateways` draft): PK (environment_id, role), org_id, role ingress | egress, gateway_id. FK (gateway_id, environment_id) → gateway_environments, no action: the default must be attached to that environment, and the gateway can't be detached or deleted while it is a default. First start points both roles at the bundled gateway. Anything placed automatically (agent model-config proxies, A2A routes, MCP mappings) goes to the matching default; users can change it or pick a gateway explicitly. A gateway can't be deleted while it is a default. This also replaces today's "at most one ingress per environment" migration checks with a plain key.
- `gateway_tokens`: + org_id. `token_prefix` plain unique (was unique among active only).
- `gateway_identity_providers`: + org_id. Unique (gateway_id, name).
- `gateway_event_states`, `gateway_events` (were `eventhub_*`): gateway_id becomes a real FK, cascade. event_data is JSON. Index (gateway_id, seq).
- Gateway deployments don't need an environment column: the gateway sets it.

### Artifacts and gateway deployments

- `artifacts`: base row for every gateway-deployable item (kind: RestApi, WebSubAPI, LlmProvider, LlmProxy, Agent, McpProxy, McpProxyMapping, OtelIngest). Unique (org_id, handle), (org_id, name, version).
- `artifact_deployments` (was `deployments`): artifact_id cascade; gateway_id and base_deployment_id no action (SQL Server rejects multiple cascade paths and self-referencing SET NULL; the service clears them). content binary, metadata JSON.
- `artifact_gateway_status` (was `deployment_status`): PK (artifact_id, gateway_id), org_id. Cascade from artifact only; deleting a gateway clears its rows in the service. Status CHECK constraints kept. claimed_by, claimed_until for the render queue.
- Typed rows share the artifact's id (`llm_providers`, `llm_proxies`, `mcp_proxies`, `mcp_proxy_mappings`): FK id → artifacts(id) ON DELETE CASCADE, so deleting the artifact deletes the typed row and everything under it. Their project FK has no action, so a project that still has LLM proxies or MCP servers can't be deleted.
- `OtelIngest`: the built-in trace-ingest API (`/otel`) that every gateway serves. One artifact per org (handle `amp-otel-ingest`), created when a gateway first connects and deployed to each gateway on connect unless it already serves the current version. Gateways receive it as a RestApi. It has no typed row.

### LLM

- `llm_provider_templates`: org templates only (built-ins ship in code, `resources/builtin_llm_templates.go`). org_id NOT NULL, unique (org_id, handle). `is_system` dropped.
- `llm_providers`: id = artifact id. Upstream API key moves out of `configuration` JSON into `secrets` (`upstream_secret_id`).
- `llm_proxies`: id = artifact id. (org_id, project_id) FK → projects. provider_id → llm_providers, restrict (a provider in use can't be deleted). Upstream auth moves out of `configuration` JSON into `secrets` (`upstream_secret_id`); this includes the key minted on the provider for a proxy.

### Agent model and MCP configs

- `agent_model_configs` (was `agent_configurations`): agent_id (cascade), name unique per agent, description, type text (llm | mcp | other; was type_id).
- `agent_model_config_llm` (was `env_agent_model_mapping`): PK (config_id, environment_id), llm_proxy_id, policy_configuration JSON.
- `agent_model_config_mcp`: config_id PK, mcp_proxy_id, scope_recorded_at (folds `agent_mcp_config_proxy` and `agent_mcp_config_scope`).
- `agent_model_config_mcp_envs` (was `env_agent_mcp_mapping` + `agent_mcp_config_env_exclusion`): PK (config_id, environment_id), mapping_artifact_id (nullable), excluded yes/no. Same meaning as today: a connection covers every environment unless excluded, so a missing row means "included, not mapped yet" (an environment added later is picked up), and `excluded = 1` records a deliberate removal.
- `agent_model_config_vars` (was `agent_env_config_variables_mapping`): PK (config_id, environment_id, variable_key), variable_name. `secret_reference` dropped (only fed hosted workloads).
- `ai_applications`: one per agent per environment, unique (agent_id, environment_id), cascade from the agent; handle, name.
- All of these cascade from the config or agent; the environment FK has no action.
- `api_keys`: org_id, artifact_id (cascade), name unique per artifact, display_name, key_hash (indexed), masked_key, status, purpose, expires_at. An external agent's proxy key is returned once in the response; only its hash is kept, and no copy goes to `secrets`.

### MCP

- `mcp_proxies`: id = artifact id. project_id NULL = org-level (composite FK skips the check when NULL on all four databases).
- `mcp_proxy_mappings`: unchanged apart from common changes.
- `mcp_proxy_scopes`: unique (mcp_proxy_id, action), description.
- `mcp_proxy_scope_tools`: (scope_id, tool_name) PK; replaces the JSON tools list because agent-identity screens look scopes up by tool.
- `mcp_proxy_endpoints`: kept as today (cascade from the proxy): upstream, capabilities, security and policies per endpoint. Upstream auth moves into `secrets` (`upstream_secret_id`). Unique (mcp_proxy_id, id) for the child key below. Lite creates one endpoint per proxy.
- `mcp_proxy_endpoint_environments`: PK (endpoint_id, environment_id), mcp_proxy_id, artifact_id (not null, unique, no action), status. One FK (mcp_proxy_id, endpoint_id) → mcp_proxy_endpoints(mcp_proxy_id, id) cascade, so there is one cascade path; deleting the proxy deletes these artifacts in the service. Unique (mcp_proxy_id, environment_id) keeps "one endpoint per environment per proxy". Lite has one row: the single endpoint in the default environment.
- Project MCP proxies that expose a hosted MCP server point at it through `source_agent_id` (see [Hosted MCP servers](#hosted-mcp-servers)). Lite answers their routes with 501.

### A2A

One row per agent per environment each, unique (agent_id, environment_id), cascade from the agent:

- `agent_exposures` (was `platform_agent_exposures`): protocol (a2a | rest), artifact_id unique.
- `agent_cards`: source, source_url, card JSON, card_hash, fetched_at, status, attempt_count, last_error, next_attempt_at, claimed_by, claimed_until. OpenChoreo release-name columns dropped.
- `a2a_proxies`: upstream_auth_header; the value moves to `secrets` (`upstream_auth_secret_id`).

### Per-environment Thunder

- `env_thunder_urls` and `env_thunder_system_clients` are not created in Lite: one Thunder ships per install, so its URL and system client come from config. If the full product gives each environment its own Thunder, these come back keyed by (org_id, environment_id).

### REST APIs

- `rest_apis`: typed row of a `RestApi` artifact (id = artifact id, cascade). (org_id, project_id) FK → projects, no action. description, lifecycle_status, transport (JSON list), configuration (JSON: context, upstream, operations, policies), upstream_secret_id → secrets, created_by. Upstream credentials move out of `configuration` into `secrets`, as for LLM providers.

### Hosted MCP servers

- `mcp_proxies.source_agent_id`: the hosted workload (an `agents` row whose `agent_workloads.workload_type` is `mcp_server`) a project MCP proxy fronts. FK (org_id, source_agent_id) → agents, no action. CHECK: set only on project-level proxies. NULL for proxies to external MCP servers.

## Events, audit log and console usage

### Events (webhooks and other destinations)

Today: `event_destinations`, `event_subscriptions`, `event_deliveries`, `event_outbox` (migrations 056-058). Lists stored as JSON today become child tables because the dispatcher and screens filter on them.

- `event_destinations`: id, org_id, name (unique per org), description, kind, config JSON, `secret_id` → secrets (was `secret_encrypted`), availability (all | org | projects, CHECK), created_by, timestamps.
- `event_destination_projects` (was the `projects` JSON list): PK (destination_id, project_id), org_id. Cascade from the destination; the project FK has no action, so deleting a project removes its rows in the service.
- `event_subscriptions`: id, org_id, scope (org | project | agent, CHECK), project_id and agent_id (nullable, composite FKs; set to match the scope, CHECK), name, description, destination_id (restrict: a destination in use can't be deleted), enabled, created_by, timestamps. Cascade from the agent; project FK has no action. Index (org_id, scope, project_id, agent_id).
- `event_subscription_types` (was `event_types` JSON): PK (subscription_id, event_type). Index (event_type).
- `event_subscription_environments` (was `environments` JSON): PK (subscription_id, environment_id). No rows = all environments. Lite never writes rows here.
- `event_outbox`: id, org_id, body JSON, claimed_by (nullable; was `lease` with `''` for unset), claimed_until (nullable), available_at. Index (available_at, id). Claimed through the per-database repository (rule 10).
- `event_deliveries`: id, org_id, destination_id (cascade), event_id, event_type, event_scope, project_id, agent_id, environment_id (all nullable, no FK: delivery history outlives deleted projects and agents), status, attempts, response_code, last_error, delivered_at, claimed_by, claimed_until, timestamps. Unique (destination_id, event_id). Indexes (org_id, created_at), (destination_id, created_at).
- `event_deliveries` also carries monitor alert notifications: `monitor_alert_id` (nullable, no FK, like the other history columns), `source_event_id` (nullable; index, to find an alert's earlier deliveries) and `payload` (JSON, nullable). Unused below the Agent Manager tier.
- `event_delivery_subscriptions` (was `subscription_ids` JSON + GIN index): PK (delivery_id, subscription_id). Cascade from the delivery; no FK to the subscription.
- Deliveries older than a configured age are deleted by a cleanup job.

### Audit log

Today audit events (`audit.Event`, schema version 1) go only to stdout through `audit.Sink`. Lite adds a database sink and a read API.

`audit_events`: one row per event, insert only, never updated.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK. The event's `eventId` (UUID v7, so it sorts by time). |
| org_id | ID | yes | No FK. NULL for events outside an org (failed sign-in, system). |
| occurred_at | time | no | |
| schema_version | whole number | no | |
| action | short text (128) | no | e.g. `agent.delete` |
| action_class | short text (32) | no | |
| severity | whole number | no | 1 info … 4 critical |
| outcome | short text (16) | no | success, failure, deny, unknown |
| actor_type | short text (16) | no | user, service, agent, gateway, system, anonymous |
| actor_id | short text (255) | yes | Username for console users. |
| actor_display, actor_token_id, on_behalf_of | short text (255) | yes | |
| auth_method | short text (32) | yes | |
| surface | short text (16) | no | api, mcp, internal, publisher, system |
| source_ip | short text (45) | yes | |
| user_agent | short text (512) | yes | |
| correlation_id | short text (64) | yes | |
| request_method | short text (8) | yes | |
| request_path | long text | yes | Not indexed. |
| resource_type | short text (64) | yes | |
| resource_id | short text (255) | yes | |
| resource_name | short text (255) | yes | Kept as text so the entry survives the resource's deletion. |
| project_name, environment_name | short text (63) | yes | Same reason; no FKs. |
| status_code | whole number | yes | |
| error_code | short text (64) | yes | |
| error_message | long text | yes | |
| required_permission | short text (128) | yes | |
| duration_ms | whole number | yes | |
| details | JSON | yes | Already redacted by `audit/redact.go`. |

- No foreign keys at all: the log must outlive whatever it describes, and writes must never fail on a missing parent. This is the one exception to rule 6.
- Indexes: (org_id, occurred_at, id); (org_id, actor_id, occurred_at); (org_id, resource_type, resource_id, occurred_at); (org_id, action, occurred_at).
- Writes are batched by the sink. Actions that must not run unrecorded (today's fail-closed policy) wait for their row to commit.
- Retention: a cleanup job deletes rows older than a configured number of days, in small batches.
- Read API: `GET /orgs/{org}/audit-events`, admin role only. Filters: time range, actor, action, resource type and id, outcome. Newest first, keyset paging on (occurred_at, id).

### Console usage

Today usage events go to Moesif (`middleware/growthanalytics`): server-side feature codes from `Track` wrappers and a fixed allowlist of console events (`amp.console.*`, about 30 codes), each with a few named dimensions plus shared metadata. Lite stores them instead.

`usage_events`: insert only.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK, UUID v7 |
| org_id | ID | yes | No FK. |
| occurred_at | time | no | |
| source | short text (16) | no | api, console |
| feature_code | short text (128) | no | Taxonomy code or `amp.console.*` action. |
| username | short text (255) | yes | |
| session_id | short text (64) | yes | Console session. |
| status_code | whole number | yes | API events only. |
| product_version | short text (32) | no | |
| dimensions | JSON | yes | The event's allowlisted dimensions. |

- No foreign keys, same reason as the audit log.
- Indexes: (org_id, occurred_at); (feature_code, occurred_at).
- Dimensions stay JSON: they differ per event code and are only read in bulk. If a screen ever needs to group by one, it becomes a column.
- Retention: a cleanup job, like the audit log.
- Collection can be switched off in config. Moesif stays an optional extra sink.
- Store and export only: the console never shows usage, so there are no summary tables. An admin-only `GET /orgs/{org}/usage-events/export` streams a time range as NDJSON (one event per line), oldest first, keyset paging on (occurred_at, id). Analysis happens outside the product.

## Evaluation and alerts

Agent Manager tier. Empty on AI Gateway. Today's tables (migrations 005-013, 039, 059) keyed rows by org name, project name, agent name and OpenChoreo UIDs; here every row points at ids, and lists that were JSON become child tables where something filters on them.

### evaluators

The org's own evaluators (was `custom_evaluators`). Built-in evaluators ship in code and are never stored.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id | ID | no | FK → organizations, no cascade |
| identifier | short text (128) | no | Unique per org. Monitors refer to evaluators by identifier. |
| display_name | short text (128) | no | |
| description | short text (512) | yes | |
| version | short text (20) | no | |
| type | short text (16) | no | CHECK `code`, `llm_judge` |
| level | short text (16) | no | CHECK `trace`, `agent`, `llm` |
| source | long text | no | Python source or prompt template |
| config_schema | JSON | no | Parameter list |
| tags | JSON | no | Read in bulk only |
| created_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

Soft delete (`deleted_at`) is dropped (rule 4). Deleting an evaluator a monitor still uses is refused by the service, through the `monitor_evaluators` index.

### monitors

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id, agent_id | ID | no | FK → agents CASCADE |
| environment_id | ID | no | FK → environments, no action. Always the default below Agent Runtime. |
| name | short text (63) | no | Unique per agent |
| display_name | short text (128) | no | |
| description | short text (512) | yes | |
| type | short text (16) | no | CHECK `future` (scheduled) or `past` (one window) |
| enabled | yes/no | no | Start/stop. Replaces the scheduler's suspended state. |
| interval_minutes | whole number | yes | Required for `future` (CHECK) |
| next_run_at | time | yes | Scheduler queue key |
| trace_start, trace_end | time | yes | Required for `past` (CHECK) |
| sampling_percent | whole number | no | 1-100 (was `sampling_rate` 0.01-1.00, rule 12) |
| judge_llm_provider_id | ID | yes | FK → llm_providers, no action. The org provider an LLM judge uses. |
| judge_llm_proxy_id | ID | yes | FK → llm_proxies, no action. The proxy created for the judge (was `monitor_llm_mapping`). |
| judge_api_key_secret_id | ID | yes | FK → secrets. The judge's key on that proxy (was an OpenBao path). |
| judge_policy_configuration | JSON | yes | |
| claimed_by, claimed_until | | yes | Scheduler lease (rule 10) |
| created_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

Index (enabled, next_run_at). Status (`Active`, `Suspended`, `Failed`) is derived from `enabled` and the latest run.

- `monitor_evaluators` (was the `evaluators` JSON): PK (monitor_id, evaluator_identifier), display_name, position, config JSON. Cascade from the monitor. Index (org_id, evaluator_identifier) finds monitors using an evaluator. No FK to `evaluators`, because built-ins have no row.
- `monitor_runs`: id, monitor_id (cascade), evaluators and judge_config (JSON snapshots taken when the run is created), trace_start, trace_end, status (pending, running, success, failed), job_ref (the runner's id for the job; was the OpenChoreo WorkflowRun name), started_at, completed_at, error_message, event_queued_at, attempt_count, claimed_by, claimed_until, timestamps.
- `monitor_run_evaluators`: id, run_id (cascade), monitor_id (no FK, rule 7), evaluator_identifier, evaluator_name (unique per run), level, aggregations JSON, item_count, skipped_count.
- `scores`: id, run_evaluator_id (cascade), monitor_id (no FK), item_key, trace_id, span_id, span_label, score_bp (0-10000, rule 12; NULL when skipped), explanation, skip_reason, trace_start_time. Unique (run_evaluator_id, item_key): `item_key` is the trace id, or `trace_id/span_id` for span-level scores, because the old unique key over nullable `span_id` breaks rule 8. Indexes (monitor_id, trace_start_time), (trace_id, span_id), (monitor_id, span_label).
- `monitor_alerts`: one per monitor (unique monitor_id, cascade). notify_on_failure, notify_on_success, thresholds JSON, breached JSON (keys of breached thresholds, read in bulk), last_evaluated_at.
- `monitor_alert_destinations` (was `destination_ids` JSON): PK (alert_id, destination_id). Cascade from the alert; the destination FK has no action, so a destination an alert uses can't be deleted.
- `org_service_clients` (was `org_publisher_credentials` and `org_scheduler_credentials`): PK (org_id, purpose), purpose `eval_publisher` or `eval_scheduler`, client_id, secret_id → secrets (was an OpenBao path or inline ciphertext).

## Agent runtime

Agent Runtime tier: hosted agents and MCP servers. Empty below it. In 1.0.0 none of this was in the service database: it lived in OpenChoreo resources (Component, Workload, WorkflowRun, ComponentRelease, ReleaseBinding, DataPlane, GitSecret). The tables below hold the same facts with no OpenChoreo concept; a runtime driver turns them into whatever the target platform needs.

| Table | Holds | Keys and cascades |
|---|---|---|
| `runtimes` | Where hosted agents run (was the DataPlane): name, kind (`kubernetes`, `docker`), endpoint, config JSON, credential_secret_id → secrets, is_active | Unique (org_id, name) |
| `git_credentials` | Named repository credentials (was the GitSecret): auth_type (`basic`, `ssh`), username, secret_id → secrets | Unique (org_id, name) |
| `agent_workloads` | One per hosted agent: workload_type (`agent`, `mcp_server`), source_type (`repository`, `image`, `agent_kind`), repository URL, branch, app path, git_credential_id, build_type (`buildpack`, `docker`), language and version, run command, Dockerfile path, image_ref, agent_kind_version_id, interface (type, port, base path, schema path), health_checks JSON | PK agent_id, cascade from the agent. CHECK: the columns the source type needs are set. |
| `builds` | One build: name (unique per agent), status (pending, running, succeeded, failed, cancelled), parameters JSON snapshot, commit_id, image_ref (required once succeeded), interface, health_checks and steps JSON, job_ref, attempt_count, last_error, claim lease, requested_by, started_at, finished_at | Cascade from the agent. Index (status, claimed_until) for the build dispatcher. |
| `agent_kinds`, `agent_kind_labels`, `agent_kind_versions` | Published reusable agents: kind name (unique per org), source_agent_id (no action; the service clears it before deleting the agent); labels as rows (were JSON + GIN); versions with version (unique per kind), build_id (no action), image_ref, config_schema JSON, metadata JSON | Labels and versions cascade from the kind |
| `agent_releases` | Immutable snapshot of what runs: image_ref, build_id (no action), config JSON (env var names and non-secret values, file mounts, resources, instrumentation) | Cascade from the agent |
| `agent_deployments` | Which release runs in each environment (was the ReleaseBinding): release_id (no action), desired_state (`active`, `suspended`, `undeployed`), status, status_reason, promoted_from_environment_id, deployed_by, deployed_at, retry and claim columns | Unique (agent_id, environment_id), cascade from the agent |
| `agent_runtime_configs` | Per agent per environment: CPU and memory requests and limits, replicas, autoscaling (min, max), isolation_tier override (sandbox), warm_pool_size, instrumentation enabled and version | PK (agent_id, environment_id), cascade from the agent |
| `agent_env_vars`, `agent_file_mounts` | Per agent per environment: env var name and value, or mount path and content; a sensitive value is a secret_id instead (CHECK: never both). `is_system` marks platform-injected variables. | PK (agent_id, environment_id, name / mount_path), cascade from the agent |
| `agent_memory_stores` | Memory a hosted agent keeps: name (unique per agent and environment), memory_type (`short_term`, `long_term`), backend, config JSON, secret_id, retention_days, status | Cascade from the agent. The memory itself lives in the backend. |

- Promotion copies a release to the target environment's `agent_deployments` row along a `deployment_pipeline_paths` edge, with that environment's runtime config and env vars.
- Sandboxing: an environment's `isolation_tier` applies to every hosted agent in it; `agent_runtime_configs.isolation_tier` can tighten it per agent.
- Build and runtime logs stay in the observability store.

## Known differences from the code

The schema above is the target. The migrations currently differ in these places:

- `agent_identities` uses its own `id` as the primary key, with (agent_id, environment_id) unique.
- `agent_model_configs.type` allows `llm | mcp | agent` rather than `llm | mcp | other`.
- `agent_model_config_mcp_envs` has an extra `mcp_proxy_id` column, and `a2a_proxies` an extra `artifact_id`.
- `gateways` also enforces that a self-hosted gateway is egress only.
- `audit_events` and `usage_events` exist, but nothing writes or reads them yet.
- The evaluation, REST API and agent runtime tables exist but nothing in Lite writes them: their routes answer 501. The evaluation models and repositories in `models/` and `repositories/` still use the old column names (`ou_id`, `custom_evaluators`, `sampling_rate`, `monitor_llm_mapping`) and need porting before evaluation is switched on.
