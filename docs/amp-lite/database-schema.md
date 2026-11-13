# AMP Lite database schema

Status: in progress, development preview. Areas are added as they are agreed.

Diagrams: [data model](data-model.png) ([HTML](data-model.html)) and [architecture](architecture.png) ([draw.io](architecture.drawio)). See [README](README.md) for how to edit them.

## Intent

AMP Lite is a version of Agent Manager that runs anywhere, with no OpenChoreo. Environments stay in the schema but are internal: each org has one `default` environment that users never see. Its own database is the only source of truth for everything the control plane manages.

## Decisions that shape the schema

- Lite only connects external agents. Agent runtime (builds, deployments, env vars, resources, git credentials, agent kinds) and evaluation (evaluators, monitors, runs, scores, alerts) are deferred until the modularization design is settled.
- Multi-tenant. `organizations` is the root.
- Environments are kept for a later version but hidden in Lite. Every org gets one `default` environment. Lite's API and console never show or accept an environment; the service always uses the org's default. Tables that are per-environment today keep an `environment_id`, so adding more environments later needs no table changes. No deployment pipeline or promotion.
- Console login is file-based to start with: a default username and password belong to a default org; a file maps scopes to roles (admin, developer). No users, roles or scopes tables. Tables refer to users by username.
- Thunder ships with Lite and gives every external agent its agent ID. Thunder keeps its own separate database; ours stores only IDs returned by Thunder's API.
- Gateways belong to an org, with no limit per org. The default org's bundled gateway is running out of the box; users add more (managed or self-hosted).
- Secrets live in our database, encrypted. Production can plug in an external vault.
- Audit log (console actions) and console usage tracking are stored in our database.
- Traces, runtime logs, build logs and metrics are stored elsewhere.
- Databases: SQLite ships with Lite and holds everything. Production uses PostgreSQL 18, SQL Server 2025 or Oracle 19c.

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
12. Time on SQLite: columns are declared `TIMESTAMP` because the pure-Go driver only parses stored text back into a time for columns declared DATE, DATETIME or TIMESTAMP (a TEXT column fails to scan). Values stay ISO 8601 text, always UTC, in the driver's layout `2006-01-02 15:04:05.999999999-07:00` (pinned with `_time_format=sqlite` in the connection string). UTC values end in `+00:00`, and because `+` sorts before `.`, text order is time order even with mixed fractional seconds; a `Z` suffix would break that. Times computed in SQL (`MAX(created_at)`, `COALESCE(...)`) lose the declared type on SQLite and come back as strings: compute them in the app or scan them as text. Reading a TIMESTAMP column into a Go string does not return the stored text (the driver parses it and database/sql formats it as RFC 3339), so never compare times as strings in Go; use `CAST(col AS TEXT)` when the raw stored text is needed.

### locks

Replaces Postgres advisory locks for the reconcilers, the drift scan and gateway serialization.

| Column | Type | Null | Notes |
|---|---|---|---|
| name | short text (128) | no | PK, e.g. `reconciler.gateway` |
| owner | short text (255) | yes | Instance holding it. NULL = free. |
| expires_at | time | yes | Lease end; an expired lease can be taken over. |

Taken with the same lease update as job claims (rule 10). Rows are seeded by migrations, so taking a lock never inserts.

### Queue tables

Every table a worker claims rows from carries `claimed_by` (short text (255), nullable) and `claimed_until` (time, nullable): `agent_identities`, `agent_cards`, `artifact_gateway_status` (the render queue), `event_deliveries` and `event_outbox`.

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
| created_by | short text (255) | yes | |
| created_at, updated_at | time | no | |

Unique: (org_id, name); (org_id, id) for child composite keys.

### Behaviour

- First start seeds the default org (handle from config), its `default` environment, its Thunder org unit, a default project in it, and the configured default gateway with its registration token, attached to the default environment. This runs on every start: it creates the org and its default environment if missing and keeps an existing gateway and token.
- Orgs are never renamed or deleted. Creating an org creates its `default` environment in the same transaction, and its Thunder org unit; if Thunder fails, the org is not saved.
- Deleting a project is refused while it still has agents, LLM proxies or MCP servers; the conflict names what is left. Its event subscriptions and its entries in event destinations' project lists are removed in the same transaction, before the project row.
- Dropped from today's API: org `namespace`, project `deploymentPipeline`. There is no pipeline table.

## Environments

### environments

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id | ID | no | FK → organizations(id), no cascade. |
| name | short text (63) | no | `default` in Lite. Same pattern as project `name`. Immutable. |
| display_name | short text (255) | no | |
| is_production | yes/no | no | `0` for the seeded default. Kept for the full product. |
| created_at, updated_at | time | no | |

Unique: (org_id, name); (org_id, id) for child composite keys.

- Lite never creates, renames or deletes environments after seeding. The service looks up the org's `default` environment (cached per org) and uses it wherever a per-environment row is read or written.
- Per-environment tables point at it through (org_id, environment_id) → environments(org_id, id), no cascade. Their rows are deleted through their owner (agent, gateway, MCP proxy), which keeps one cascade path per table (rule 7).
- OpenChoreo's data-plane reference, DNS prefix and pipeline fields are not carried over.
- The full product adds more rows here, plus a pipeline table, without changing the per-environment tables.

## Agents and secrets

Lite has external agents only. Runtime tables (build settings, builds, deployments, env vars, resources, health checks, instrumentation) are added later if hosted agents land in Lite.

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
- Project MCP proxies (expose a hosted agent's MCP endpoint) are deferred until the modularization design is settled.

### A2A

One row per agent per environment each, unique (agent_id, environment_id), cascade from the agent:

- `agent_exposures` (was `platform_agent_exposures`): protocol (a2a | rest), artifact_id unique.
- `agent_cards`: source, source_url, card JSON, card_hash, fetched_at, status, attempt_count, last_error, next_attempt_at, claimed_by, claimed_until. OpenChoreo release-name columns dropped.
- `a2a_proxies`: upstream_auth_header; the value moves to `secrets` (`upstream_auth_secret_id`).

### Per-environment Thunder

- `env_thunder_urls` and `env_thunder_system_clients` are not created in Lite: one Thunder ships per install, so its URL and system client come from config. If the full product gives each environment its own Thunder, these come back keyed by (org_id, environment_id).

### Deferred

- Evaluation monitors (per environment today) and the eval publisher and scheduler credentials (`org_publisher_credentials`, `org_scheduler_credentials`, encrypted today) move to `secrets` and pick up `environment_id` if evaluation lands in Lite.

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

## Known differences from the code

The schema above is the target. The migrations currently differ in these places:

- `agent_identities` uses its own `id` as the primary key, with (agent_id, environment_id) unique.
- `agent_model_configs.type` allows `llm | mcp | agent` rather than `llm | mcp | other`.
- `agent_model_config_mcp_envs` has an extra `mcp_proxy_id` column, and `a2a_proxies` an extra `artifact_id`.
- `gateways` also enforces that a self-hosted gateway is egress only.
- `audit_events` and `usage_events` exist, but nothing writes or reads them yet.
- `models/` still defines tables that are not in this schema: evaluation (monitors, monitor alerts, scores and evaluators) and `rest_apis`.
