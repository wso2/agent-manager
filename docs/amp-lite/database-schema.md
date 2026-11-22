# AMP Lite database schema

Status: in progress, development preview. Areas are added as they are agreed.

Diagrams: [data model](data-model.png) ([HTML](data-model.html)) and [architecture](architecture.png) ([draw.io](architecture.drawio)). See [README](README.md) for how to edit them.

## Intent

AMP Lite is a version of Agent Manager that runs anywhere, with no OpenChoreo. Environments stay in the schema but are internal: each org has one `default` environment that users never see. Its own database is the only source of truth for everything the control plane manages.

## Decisions that shape the schema

- Lite only connects external agents. Agent runtime (builds, deployments, env vars, resources, git credentials, agent kinds) are deferred, and evaluation (evaluators, monitors, runs, scores, alerts) is removed from Lite; if they come back, they come as modules with their own tables.
- The schema is split between the core and modules (see [Modules](#modules)). The core owns tenancy, agents, identity, secrets, events, audit and the module host's own tables. A module's tables are named `mod_<module>_<table>` and come from the module's own migrations. The AI Gateway's 33 tables are listed under [Gateway module tables](#gateway-module-tables); `amp-core` (no modules) doesn't have them.
- Multi-tenant. `organizations` is the root.
- Environments are kept for a later version but hidden in Lite. Every org gets one `default` environment. Lite's API and console never show or accept an environment; the service always uses the org's default. Tables that are per-environment today keep an `environment_id`, so adding more environments later needs no table changes. No deployment pipeline or promotion.
- Console login is file-based to start with: a default username and password belong to a default org; a file maps scopes to roles (admin, developer). No users, roles or scopes tables. Tables refer to users by username.
- Thunder ships with Lite and gives every external agent its agent ID. Thunder keeps its own separate database; ours stores only IDs returned by Thunder's API.
- Gateways belong to an org, with no limit per org, and are managed by the `ai-gateway` module. With `amp`, the default org's bundled gateway is running out of the box; users add more (managed or self-hosted).
- Secrets live in our database, encrypted. Production can plug in an external vault.
- Audit log (console actions) and console usage tracking are stored in our database.
- Traces, runtime logs, build logs and metrics are not stored in Lite; they come back with an observability module and its own store.
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

Replaces Postgres advisory locks for the core's workers. A module that needs a lease keeps its own table with the same columns (the `ai-gateway` module's drift scan uses `mod_ai_gateway_locks`).

| Column | Type | Null | Notes |
|---|---|---|---|
| name | short text (128) | no | PK, e.g. `agent_identity.reconciler` |
| owner | short text (255) | yes | Instance holding it. NULL = free. |
| expires_at | time | yes | Lease end; an expired lease can be taken over. |

Taken with the same lease update as job claims (rule 10). Rows are seeded by migrations, so taking a lock never inserts.

### Queue tables

Every table a worker claims rows from carries `claimed_by` (short text (255), nullable) and `claimed_until` (time, nullable): `agent_identities`, `agent_cards`, `artifact_gateway_status` (the render queue), `event_deliveries` and `event_outbox`. `module_event_deliveries` uses `claimed_by` with `available_at` as the lease end.

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

- Every start makes sure the default org exists (`DEFAULT_ORG_HANDLE`, `DEFAULT_ORG_OU_ID`) with its `default` environment; the core does this before modules activate, so it works with or without modules and modules can rely on it. With `amp`, the gateway module then seeds the configured default gateway with its registration token, attached to the default environment, and keeps an existing gateway and token.
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

One row per agent per environment, in the `ai-gateway` module (`mod_ai_gateway_agent_gateway_settings`). Replaces `agent_configs`. PK (agent_id, environment_id). Removed with its agent by the module's agent finalize hook.

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
| owner_kind | short text (32) | no | What uses it: a core kind (`event_destination`, ...), or `mod:<module>` for a module's secrets (`mod:ai-gateway`). For cleanup; not an FK. `named_secret` rows are addressed by name through the secret management client and are their own owner. |
| owner_id | ID | no | For a module secret, derived from the module and its path (`llm-providers/<id>/upstream-auth`), so a module only reaches its own. |
| provider | short text (16) | no | The vault that protects the value: `local`, or an external vault's name |
| ciphertext | binary | yes | `local` only. AES-256-GCM, nonce stored with it, bound to (org_id, id) so it does not open in another row. |
| key_id | short text (64) | yes | Master key that encrypted it, for rotation. NULL on rows written before the vault. |
| external_ref | short text (512) | yes | Path in the external vault. |
| version | whole number | no | |
| created_at, updated_at | time | no | |

Index (org_id, owner_kind, owner_id). Core tables reference a secret by `secret_id` (FK, no action) and the service deletes it after the owner (event destination secrets). Modules reach theirs only through `rt.Secrets()` by path: the AI Gateway keeps its LLM provider and LLM proxy upstream auth, MCP endpoint upstream auth and A2A upstream auth there, each written and deleted in its row's transaction, and the row's configuration carries the path as its `secretRef`; its rows hold no secret columns. `amp module uninstall --purge` deletes a module's secrets. Module secrets go through the same vault and jobs as core ones.

Values go through a vault (`agent-manager-service/vault`), selected by `SECRET_VAULT_PROVIDER`. The built-in `local` vault encrypts into the row. An external vault is a Go package that implements `vault.ExternalVault` and registers itself with `vault.Register`. It reserves a version-specific reference from the promotion job ID before writing. Retrying the job writes the same value under the same reference. Destroying the reference prevents an interrupted attempt from recreating it. Rows are opened by the vault named in `provider`. The local vault stays loaded, and `SECRET_VAULT_READERS` lists previous external providers still needed for reads and cleanup. `ENCRYPTION_KEY_PREVIOUS` keeps values sealed before a key rotation readable. The standalone Compose file sets it from `AMP_ENCRYPTION_KEY_PREVIOUS`.

During an upgrade or rollback involving a pre-vault binary, keep the local vault, use the same `ENCRYPTION_KEY` on all replicas, and leave `SECRET_VAULT_BIND_LOCAL_ROWS=false` (the default). The local vault writes the legacy AES-GCM format in this mode. After all replicas support the vault and the rollback window has closed, set `SECRET_VAULT_BIND_LOCAL_ROWS=true` to bind newly written local ciphertext to its organization and secret ID. Existing values remain readable and are resealed on update. Enabling row binding, switching to an external vault, or rotating the encryption key can make newly written credentials unreadable by pre-vault replicas. Turning row binding off changes future writes only.

### Standalone OpenBao

Run OpenBao independently of AMP and enable a KV v2 secrets engine. Configure AMP with `SECRET_VAULT_PROVIDER=openbao`, `OPENBAO_ADDR` (the server origin), `OPENBAO_TOKEN` (a scoped token), and optionally `OPENBAO_KV_MOUNT` (defaults to `secret`). Standalone Compose forwards these settings to `amp-api`. For a server on the host, `OPENBAO_ADDR` must be reachable from inside the API container. Use HTTPS and a trusted CA outside local development. Provide the token through the API process environment and use a separate mount per AMP deployment.

For a mount named `secret`, the API token needs this OpenBao policy (kept in `agent-manager-service/vault/openbao/policy.hcl`; CI runs the OpenBao test with a token limited to it):

```hcl
path "secret/data/amp/*" {
  capabilities = ["create", "read"]
}
path "secret/destroy/amp/*" {
  capabilities = ["update"]
}
path "secret/metadata/amp/*" {
  capabilities = ["delete"]
}
```

The API writes each credential to its own immutable OpenBao path and commits only its reference in SQL. The `ciphertext` and `key_id` columns are null for OpenBao rows, and no promotion job stages the value in SQL. Old locally encrypted credentials stay readable and move to OpenBao when rewritten. Keep `OPENBAO_ADDR` and `OPENBAO_KV_MOUNT` pointed at the original mount while it holds referenced values. If you switch back to local writes, set `SECRET_VAULT_READERS=openbao` to keep existing references readable and allow cleanup.

The API reserves a cleanup job before staging a credential in OpenBao, outside the owning SQL transaction. Committing the reference cancels the cleanup job atomically. If the write fails, SQL rolls back, or the API exits before commit, the previous committed credential remains readable and the worker permanently destroys the abandoned value after the staging window. OpenBao retains a metadata tombstone to prevent delayed writes from recreating the value. Replaced and deleted committed credentials are reclaimed by the normal job worker. `/healthz` reports 503 when OpenBao is unreachable, sealed or refuses the token.

Module code writes through `rt.Secrets()`. Under OpenBao, a `Put` inside a transaction (`In(tx)`) needs the context returned by `Prepare` for the same path and value, called before the transaction opens. Hooks run inside the core's transaction, so they can read and delete module secrets but cannot write them under OpenBao.

To test against a disposable standalone server, start `bao server -dev -dev-listen-address=127.0.0.1:8200` in a separate process. Export `OPENBAO_ADDR=http://127.0.0.1:8200` and the dev token as `OPENBAO_TOKEN`, then run `make test-integration` from `agent-manager-service`. The suite runs `TestStandaloneOpenBaoReferenceOnly` when the address is set.


An external vault is never written inside a request's transaction. A write seals the value into the row with `local`, so it is readable at once, and queues a `promote` job; a delete, or a write that replaces an external value, queues a `destroy` job for it. Both commit or roll back with the change. After the commit a worker writes the value to the external vault as a new entry or version, points the row at it, and removes the value it replaced. A rolled-back change has therefore written nothing to the external vault, and the value a committed row points to stays readable until a later commit replaces it.

### secret_jobs

Created by migration `002_secret_jobs`, not the baseline, so a database the baseline already ran on gets it too.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| org_id, secret_id, owner_kind | ID, ID, short text (32) | no | The secret the job is for, kept so the job outlives it. No foreign keys. |
| action | short text (16) | no | `promote` or `destroy` |
| provider, external_ref | short text (16), short text (512) | yes | `destroy`: the value to remove. `promote`: the external value the promoted version replaces, removed once the row points at the new one. Cleared on a promotion a later write or delete took over. |
| staged_provider, staged_ref | short text (16), short text (512) | yes | `promote`: reserved provider and reference persisted before the external write. Retries reuse the reference. A superseded promotion queues it for cleanup. |
| version | whole number | yes | `promote`: the secrets.version it promotes. NULL once superseded. |
| attempts, last_error | whole number, long text | no, yes | Failures so far; the job is retried with backoff (15 s doubling to an hour) until it succeeds. |
| available_at | time | no | Not run before this. |
| claimed_by, claimed_until | short text (255), time | yes | Renewed during vault calls. Publication and cleanup handoff verify the current claim in the same transaction. |
| created_at | time | no | |

Indexes (available_at, id), (secret_id), (provider, external_ref). A `destroy` job removes nothing a committed row, or a pending promotion, still refers to; it is dropped instead. Every step is safe to repeat.

## Gateways, LLM, MCP and A2A

Everything in this section, and `agent_gateway_settings` above, belongs to the `ai-gateway` module: each table is created by the module's migrations as `mod_ai_gateway_<name>` (see [Gateway module tables](#gateway-module-tables)), and its references to core rows (organizations, projects, agents, environments, secrets) have no foreign keys. The descriptions below use the short names.

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

- `artifacts`: base row for every gateway-deployable item (kind: RestApi, WebSubAPI, LlmProvider, LlmProxy, Agent, McpProxy, McpProxyMapping), in_catalog. Unique (org_id, handle), (org_id, name, version).
- `artifact_deployments` (was `deployments`): artifact_id cascade; gateway_id and base_deployment_id no action (SQL Server rejects multiple cascade paths and self-referencing SET NULL; the service clears them). content binary, metadata JSON.
- `artifact_gateway_status` (was `deployment_status`): PK (artifact_id, gateway_id), org_id. Cascade from artifact only; deleting a gateway clears its rows in the service. Status CHECK constraints kept. claimed_by, claimed_until for the render queue.
- Typed rows share the artifact's id (`llm_providers`, `llm_proxies`, `mcp_proxies`, `mcp_proxy_mappings`): FK (org_id, id) → artifacts(org_id, id) ON DELETE CASCADE, so deleting the artifact deletes the typed row and everything under it. They have no key into `projects`; the module's project finalize hook refuses (409) to delete a project that still has LLM proxies or MCP servers.

### LLM

- `llm_provider_templates`: org templates only (built-ins ship in code, `modules/aigateway/resources/builtin_llm_templates.go`). org_id NOT NULL, unique (org_id, handle). `is_system` dropped.
- `llm_providers`: id = artifact id. The upstream API key is in the module's secret store; `configuration` carries its path as `secretRef` ([modules.md](modules.md#where-the-gateway-stands)).
- `llm_proxies`: id = artifact id. project_id (no key into `projects`). provider_id → llm_providers, restrict (a provider in use can't be deleted). Upstream auth, including the key minted on the provider for a proxy, is in the module's secret store; `configuration` carries its path as `secretRef`.

### Agent model and MCP configs

- `agent_model_configs` (was `agent_configurations`): agent_id (no key into `agents`), name unique per agent, description, type text (llm | mcp | agent; was type_id).
- `agent_model_config_llm` (was `env_agent_model_mapping`): PK (config_id, environment_id), llm_proxy_id, policy_configuration JSON.
- `agent_model_config_mcp`: config_id PK, mcp_proxy_id, scope_recorded_at (folds `agent_mcp_config_proxy` and `agent_mcp_config_scope`).
- `agent_model_config_mcp_envs` (was `env_agent_mcp_mapping` + `agent_mcp_config_env_exclusion`): PK (config_id, environment_id), mapping_artifact_id (nullable), excluded yes/no. Same meaning as today: a connection covers every environment unless excluded, so a missing row means "included, not mapped yet" (an environment added later is picked up), and `excluded = 1` records a deliberate removal.
- `agent_model_config_vars` (was `agent_env_config_variables_mapping`): PK (config_id, environment_id, variable_key), variable_name. `secret_reference` dropped (only fed hosted workloads).
- `ai_applications`: one per agent per environment, unique (agent_id, environment_id); handle, name.
- The per-config tables cascade from their config. None of these tables has a key into `agents` or `environments`: the agent's rows go when the `agent.deleted` handler finishes its teardown, and an environment's rows when its finalize hook runs (see [Gateway module tables](#gateway-module-tables)).
- `api_keys`: org_id, artifact_id (cascade), name unique per artifact, display_name, key_hash (indexed), masked_key, status, purpose, expires_at. An external agent's proxy key is returned once in the response; only its hash is kept, and no copy goes to `secrets`.

### MCP

- `mcp_proxies`: id = artifact id. project_id NULL = org-level (no key into `projects`).
- `mcp_proxy_mappings`: unchanged apart from common changes.
- `mcp_proxy_scopes`: unique (mcp_proxy_id, action), description.
- `mcp_proxy_scope_tools`: (scope_id, tool_name) PK; replaces the JSON tools list because agent-identity screens look scopes up by tool.
- `mcp_proxy_endpoints`: kept as today (cascade from the proxy): upstream, capabilities, security and policies per endpoint. Upstream auth is in the module's secret store, under the proxy and the endpoint's handle. Unique (mcp_proxy_id, id) for the child key below. Lite creates one endpoint per proxy.
- `mcp_proxy_endpoint_environments`: PK (endpoint_id, environment_id), mcp_proxy_id, artifact_id (not null, unique, no action), status. One FK (mcp_proxy_id, endpoint_id) → mcp_proxy_endpoints(mcp_proxy_id, id) cascade, so there is one cascade path; deleting the proxy deletes these artifacts in the service. Unique (mcp_proxy_id, environment_id) keeps "one endpoint per environment per proxy". Lite has one row: the single endpoint in the default environment.
- Project MCP proxies (expose a hosted agent's MCP endpoint) are deferred.

### A2A

One row per agent per environment each, unique (agent_id, environment_id), removed after the `agent.deleted` handler's teardown:

- `agent_exposures` (was `platform_agent_exposures`): protocol (a2a | rest), artifact_id unique.
- `agent_cards`: source, source_url, card JSON, card_hash, fetched_at, status, attempt_count, last_error, next_attempt_at, claimed_by, claimed_until. OpenChoreo release-name columns dropped.
- `a2a_proxies`: upstream_auth_header; the value is in the module's secret store while the header is set.

### Per-environment Thunder

- `env_thunder_urls` and `env_thunder_system_clients` are not created in Lite: one Thunder ships per install, so its URL and system client come from config. If the full product gives each environment its own Thunder, these come back keyed by (org_id, environment_id).

### Deferred

- Evaluation is not part of Lite. A future evaluation module brings its own tables: its monitors pick up `environment_id`, and its publisher and scheduler credentials live in `secrets`.

## Events, audit log and console usage

### Events (webhooks and other destinations)

Today: `event_destinations`, `event_subscriptions`, `event_deliveries`, `event_outbox` (migrations 056-058). Lists stored as JSON today become child tables because the dispatcher and screens filter on them.

- `event_destinations`: id, org_id, name (unique per org), description, kind, config JSON, `secret_id` → secrets (was `secret_encrypted`), availability (all | org | projects, CHECK), created_by, timestamps.
- `event_destination_projects` (was the `projects` JSON list): PK (destination_id, project_id), org_id. Cascade from the destination; the project FK has no action, so deleting a project removes its rows in the service.
- `event_subscriptions`: id, org_id, scope (org | project | agent, CHECK), project_id and agent_id (nullable, composite FKs; set to match the scope, CHECK), name, description, destination_id (restrict: a destination in use can't be deleted), enabled, created_by, timestamps. Cascade from the agent; project FK has no action. Index (org_id, scope, project_id, agent_id).
- `event_subscription_types` (was `event_types` JSON): PK (subscription_id, event_type). Index (event_type).
- `event_subscription_environments` (was `environments` JSON): PK (subscription_id, environment_id). No rows = all environments. Lite never writes rows here.
- `event_outbox`: id, org_id, body JSON, claimed_by (nullable; was `lease` with `''` for unset), claimed_until (nullable), available_at. Index (available_at, id). Claimed with the same lease update as job claims (rule 10).
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
| surface | short text (16) | no | api, mcp, internal, system |
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

## Modules

Modules are compiled into the binary (`cmd/amp` links them, `cmd/amp-core` links none) and installed, enabled and disabled at runtime. The module host keeps their state in core tables. Each module keeps its schema in `modules/<name>/schema/*.yaml`, rendered per database into `modules/<name>/migrations/{sqlite,postgres}` like the core's; every table it creates must be named `mod_<module>_<table>`, and core tables may not start with `mod_`.

### modules

| Column | Type | Null | Notes |
|---|---|---|---|
| name | short text (63) | no | PK. Module name. |
| version | short text (64) | no | Version that was installed. |
| state | short text (16) | no | CHECK `active`, `disabled` or `uninstalled`. |
| created_at, updated_at | time | no | |

A compiled-in module with no row is available but not installed; `amp` installs `ai-gateway` at boot while it has no row and no modules list is declared. `amp module uninstall --purge` empties the module's `mod_<name>_*` tables, deletes its rows in `secrets` (owner kind `mod:<name>`), revokes its `workload_credentials` and deletes its row, in one transaction.

### module_generation

One row (`id = 1`, CHECK). Every install, enable, disable or uninstall bumps `value` in the same transaction; each replica polls it and reloads its set of active modules when it changes. Module migrations lock this row so that only one instance migrates at a time.

### module_migrations

| Column | Type | Null | Notes |
|---|---|---|---|
| module | short text (63) | no | PK part 1 |
| version | short text (255) | no | PK part 2. Migration file name, e.g. `001_widgets`. |
| checksum | short text (64) | no | sha256 of the statements. An applied migration that was edited is refused. |
| applied_at | time | no | |

Module migrations run after the core's, module by module in name order. Startup is refused while any compiled-in module has pending migrations.

### module_events and module_event_deliveries

Events for modules: core events (`agent.created`, `agent.deleted`, `environment.created`) that a module subscribes to, and events modules publish. They are kept apart from `event_outbox`, which feeds external destinations.

- `module_events`: id, org_id (FK → organizations, cascade), type (short text (128)), source (core or a module name), subject (nullable), data JSON, created_at. Written in the same transaction as the change it describes.
- `module_event_deliveries`: PK (event_id, module), event_id FK → module_events cascade. One row per subscribing module, until that module has handled it. attempts, available_at, claimed_by, last_error, failed_at. A failed delivery is retried with backoff; after the last attempt `failed_at` is set and the row is kept. Index (available_at).
- Events and deliveries are deleted after a retention period (7 days by default); uninstalling a module deletes its deliveries.

### workload_credentials

Credentials a module's service workloads present in their heartbeats (`POST /api/internal/v1/workloads/heartbeat`), which return short-lived workload tokens. Tokens carry the scopes the workload declares in the module's manifest.

| Column | Type | Null | Notes |
|---|---|---|---|
| id | ID | no | PK |
| module | short text (63) | no | Indexed with workload. |
| workload | short text (63) | no | A service workload the module declares. |
| org_id | ID | yes | FK → organizations, cascade. |
| subject | short text (255) | no | Who uses it, e.g. `default` or `gateway:<id>`. |
| secret_hash | short text (64) | no | sha256 of the secret; the credential (`<id>.<secret>`) is shown once. |
| created_at | time | no | |
| revoked_at | time | yes | |

### workload_instances

Workload instances that heartbeated. A row is refreshed on every heartbeat; an instance seen in the last 30 seconds counts as connected for module health.

| Column | Type | Null | Notes |
|---|---|---|---|
| credential_id | ID | no | PK part 1. FK → workload_credentials, cascade. Revoking the credential deletes its rows. |
| instance | short text (255) | no | PK part 2. The name the instance reports. |
| module | short text (63) | no | Indexed with workload and last_seen. |
| workload | short text (63) | no | |
| protocol | short text (63) | no | From the last heartbeat. |
| version | short text (63) | no | From the last heartbeat. |
| first_seen | time | no | |
| last_seen | time | no | Rows not seen for 24 hours are deleted on the credential's next new instance. |

### Gateway module tables

The `ai-gateway` module (`modules/aigateway`) creates its tables with its own migrations, rendered from `modules/aigateway/schema/*.yaml` by `make schema`: `mod_ai_gateway_` followed by `agent_gateway_settings`, `gateways`, `gateway_environments`, `environment_default_gateways`, `gateway_tokens`, `gateway_identity_providers`, `gateway_event_states`, `gateway_events`, `artifacts`, `artifact_deployments`, `artifact_gateway_status`, `llm_provider_templates`, `llm_providers`, `llm_proxies`, `mcp_proxies`, `mcp_proxy_mappings`, `mcp_proxy_scopes`, `mcp_proxy_scope_tools`, `mcp_proxy_endpoints`, `mcp_proxy_endpoint_environments`, `agent_model_configs`, `agent_model_config_llm`, `agent_model_config_mcp`, `agent_model_config_mcp_envs`, `agent_model_config_vars`, `ai_applications`, `api_keys`, `agent_exposures`, `agent_cards`, `a2a_proxies`, `agent_bindings`, `agent_teardowns` and `locks`. Foreign keys between them stay; there are none into core tables.

Its finalize hooks do what the dropped keys did, in the core's delete transaction:

- **Agent deleted:** its `agent_bindings` and `agent_gateway_settings` rows are deleted, and an `agent_teardowns` row records the agent's organization, project and name. Its `agent_model_configs` (and, through their keys, the per-config tables), `ai_applications`, `agent_exposures`, `agent_cards` and `a2a_proxies` rows stay until the `agent.deleted` handler has revoked their credentials and withdrawn them from the gateways; it then deletes them and the teardown row. While the teardown row exists, an agent with the same name in that project cannot be created (409).
- **Project deleted:** refused (409) while the project has LLM proxies or MCP servers.
- **Environment deleted:** refused (409) while a gateway mapped to it serves artifacts; otherwise its rows in the module's per-environment tables are deleted.

`locks` (from `002_drift_scan_lock.yaml`) holds the drift scan's lease, row `drift_scan`, with the columns of the core's [locks](#locks) table, so only one replica scans at a time.

Two of its tables track agent bindings:

- **`agent_bindings`:** one row per LLM provider or MCP proxy an agent's `extensions.ai-gateway` block asked for: `kind` (`llm | mcp`), `position` in the request list, `target` (the provider or proxy handle), the stored `request`, `state` (`pending | ready | failed | removing`), `last_error` and the `config_id` it provisioned. The apply hook writes them in the agent's transaction; the background handler moves them to `ready` or `failed`.
- **`agent_teardowns`:** one row per deleted agent whose gateway side effects are not yet undone (PK agent_id; unique org, project and name), with the last error.

Organizations are not deleted in Lite.


## Known differences from the code

The schema above is the target. The migrations currently differ in these places:

- `agent_identities` uses its own `id` as the primary key, with (agent_id, environment_id) unique.
- `agent_model_configs.type` allows `llm | mcp | agent` rather than `llm | mcp | other`.
- `agent_model_config_mcp_envs` has an extra `mcp_proxy_id` column, and `a2a_proxies` an extra `artifact_id`.
- `gateways` also enforces that a self-hosted gateway is egress only.
- `audit_events` and `usage_events` exist, but nothing writes or reads them yet.
- The gateway's `gwmodels` (`modules/aigateway/gwmodels/rest_api.go`) still defines a table that is in no schema: `rest_apis`.
