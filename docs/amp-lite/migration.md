# Migration tracker

> **Contributors:** every change to `agent-manager-service/db_migrations/schema/lite.yaml` must update this file in the same pull request. Add a line to the [target changelog](#target-changelog) and fix every source mapping the change affects. A reviewer should reject a schema change that leaves this file stale.

This is the working record of how data moves into the new Agent Manager database from earlier releases. It is not the user-facing migration guide; that guide is tracked in [#2435](https://github.com/wso2/agent-manager/issues/2435) and will be built from this file. The target schema is described in [database-schema.md](database-schema.md).

Status: no migration tool exists yet. The mappings below are the design that tool and the guide will follow.

Contents:

- [Target changelog](#target-changelog)
- [Common rules for every source](#common-rules-for-every-source)
- [Source: Agent Manager 1.0.0](#source-agent-manager-100)
- [Source: 2026.11.0 (STS release)](#source-2026110-sts-release)
- [Open questions](#open-questions)

## Target changelog

One entry per target migration (`db_migrations/sql/<driver>/NNN_*.sql`). Before the first release, `001` is still edited in place; record each such change as a dated line under it. After the first release, `001` is frozen and every change gets a new number.

### 001 baseline (unreleased)

- 2026-11-13: The schema now covers all three tiers. Added `rest_apis`; the evaluation tables (`evaluators`, `monitors`, `monitor_evaluators`, `monitor_runs`, `monitor_run_evaluators`, `scores`, `monitor_alerts`, `monitor_alert_destinations`, `org_service_clients`); the agent runtime tables (`runtimes`, `git_credentials`, `agent_kinds`, `agent_kind_labels`, `agent_kind_versions`, `agent_workloads`, `builds`, `agent_releases`, `agent_deployments`, `agent_runtime_configs`, `agent_env_vars`, `agent_file_mounts`, `agent_memory_stores`); and the pipeline tables (`deployment_pipelines`, `deployment_pipeline_environments`, `deployment_pipeline_paths`). Added the nullable columns `projects.deployment_pipeline_id`, `environments.description`, `runtime_id`, `isolation_tier` and `dns_prefix`, `mcp_proxies.source_agent_id`, and `event_deliveries.monitor_alert_id`, `source_event_id` and `payload`. New lock rows: `monitor.scheduler`, `monitor.alerts`, `build.dispatcher`, `deployment.reconciler`. Every org now also gets a `default` deployment pipeline holding its default environment.
- Before 2026-11-13: The first Lite baseline covered orgs, projects, environments, agents, identities, gateway settings, secrets, gateways, artifacts, LLM, MCP, model configs, A2A, events, audit and usage, and locks.

## Common rules for every source

These rules apply to every source version unless its section says otherwise.

### Identifiers

| Old identifier | New column | Rule |
|---|---|---|
| `ou_id` (Thunder org unit) | `org_id` | The same value. An organization's id is its Thunder OU id, so `organizations.id = ou_id`. |
| `organization_name`, `org_name` | `organizations.handle` | Read once per org. The column is dropped from every other table. |
| OpenChoreo Project `metadata.uid` | `projects.id` | Kept as the id, so `llm_proxies.project_uuid` maps straight across. |
| OpenChoreo Component `metadata.uid` | `agents.id` | Kept as the id, so `monitors.agent_id` and `a2a_publications.agent_uuid` map straight across. Rows that hold the agent's **name** (`agent_name`, `agent_configurations.agent_id`, `ai_applications.agent_id`) are looked up by (org, project, name). |
| OpenChoreo Environment `metadata.uid` | `environments.id` | Kept as the id, so every `environment_uuid` maps straight across. Rows that hold the name (`environment_name`, `env_name`) are looked up by (org, name). |
| Old table UUIDs (`uuid`, `id`) | `id` | Kept. |
| `SERIAL` ids (`gateway_environment_mappings.id`, `env_agent_model_mapping.id` and so on) | none | Dropped. The new tables key on their natural columns. |

Kept ids are UUID v4 where the new code generates UUID v7. Ids are opaque, so this is safe. The only id-ordered reads (audit and usage paging) start empty.

### Values

- Empty strings that meant "not set" become NULL (schema rule 3), for example `last_error = ''`, `thunder_agent_id = ''`, `description = ''`.
- Soft-deleted rows (`deleted_at IS NOT NULL` on `gateways`, `custom_evaluators`) are not carried over (rule 4). The `deleted_at` columns go.
- Times move to UTC. Old `TIMESTAMP` columns without a time zone were written in the server's zone; convert them before loading.
- Every value that was encrypted inline, held at an OpenBao path, or held in an OpenChoreo Secret becomes a row in `secrets`, re-encrypted with the new master key. The owning row gets the `secret_id`.

### Environments in a target below Agent Runtime

AI Gateway and Agent Manager (Lite) targets run one hidden `default` environment per org. Pick one source environment per org (normally the one gateways and external agents use, or the first stage of the org's pipeline). Load it as the `default` environment row, keeping its id, and load only the per-environment rows that point at it. Rows for other environments can't be carried over until the target is Agent Runtime.

### Hosted agents in a target below Agent Runtime

AI Gateway and Agent Manager targets don't run hosted agents, and their hosted routes answer 501. Internal agents and everything under them (builds, releases, deployments, env vars, runtime configs) load only into an Agent Runtime target. Otherwise leave them out, and list them in the migration report.

### Order of loading

Load the tables in foreign-key order: `organizations`, `runtimes`, `environments`, `deployment_pipelines` and their stages and paths, `projects`, `secrets`, `agents`, then everything else. `db_migrations/schema` sorts tables this way already, so the generated baseline shows the order.

## Source: Agent Manager 1.0.0

Tag `amp/v1.0.0`. The service database is PostgreSQL with 43 migrations (`0001`-`0043` in `migration_history`) and 38 tables. Most product data was not in that database:

| Data | Where it lived |
|---|---|
| Organizations | Thunder org units. Every org shared one OpenChoreo namespace (`OPEN_CHOREO_DEFAULT_NAMESPACE`). |
| Projects | OpenChoreo `Project` |
| Agents, their labels, build settings and interface | OpenChoreo `Component` (labels, annotations, `spec.workflow.parameters`) |
| Images and endpoints | OpenChoreo `Workload` |
| Builds | OpenChoreo `WorkflowRun` |
| Deployments | OpenChoreo `ComponentRelease` and `ReleaseBinding` |
| Per-environment env vars, file mounts, replicas, resources, autoscaling, runtime class | `ReleaseBinding.spec.workloadOverrides` and `spec.componentTypeEnvironmentConfigs` |
| Environments, data planes, pipelines | OpenChoreo `Environment`, `ClusterDataPlane`, `DeploymentPipeline` |
| Secret values | Data-plane OpenBao, through the OpenChoreo Secret API and `SecretReference` |
| Git credentials | Workflow-plane OpenBao, through the OpenChoreo `GitSecret` API |
| Eval runs | OpenChoreo `WorkflowRun` (`monitor-evaluation-workflow`) |
| Agent identities | Thunder, one instance per environment (`env_thunder_urls`) |

### Service database tables

| 1.0.0 table | New table | Columns renamed | Columns dropped | New columns and how they're filled | Not carried over |
|---|---|---|---|---|---|
| `gateways` | `gateways` | `uuid` → `id`, `gateway_functionality_type` → `functionality_type` | `organization_name`, `deleted_at` | `hosting`: `managed` (1.0.0 had no self-hosted gateways). `is_critical`, `is_active`, `properties` become NOT NULL (NULL → `0`). | Soft-deleted gateways |
| `gateway_tokens` | `gateway_tokens` | `uuid` → `id`, `gateway_uuid` → `gateway_id` | | `org_id` from the gateway | Revoked tokens may be dropped |
| `gateway_environment_mappings` | `gateway_environments` | `gateway_uuid` → `gateway_id`, `environment_uuid` → `environment_id` | `id` | `org_id` | Mappings to environments not loaded |
| (rule: at most one ingress gateway per environment) | `environment_default_gateways` | | | Per environment: `ingress` = its `ingress` or `both` gateway; `egress` = its `egress` gateway, or else its `both` gateway | |
| `gateway_identity_providers` | `gateway_identity_providers` | `uuid` → `id`, `gateway_uuid` → `gateway_id` | | `org_id`; `''` → NULL for `issuer`, `jwks_uri`, `description` | |
| `eventhub_gateway_states`, `eventhub_events` | `gateway_event_states`, `gateway_events` | | | | All rows. They're a transient change feed; gateways resync when they reconnect. |
| `artifacts` | `artifacts` | `uuid` → `id` | `organization_name` | `kind`: `RestAPI` → `RestApi`. `in_catalog` NULL → `0`. | |
| `deployments` | `artifact_deployments` | `deployment_id` → `id`, `artifact_uuid` → `artifact_id`, `gateway_uuid` → `gateway_id` | `organization_name` | `metadata` text → JSON | Optional: the history can be dropped and rebuilt (see step 9) |
| `deployment_status` | `artifact_gateway_status` | `artifact_uuid` → `artifact_id`, `gateway_uuid` → `gateway_id` | `organization_name` | `status_desired` = old `status`; `performed_at`, `attempt_count = 0` | Recommended: don't load. Write `PENDING` rows for the drift scan to redeploy. |
| `association_mappings` | none | | | | All rows. The feature was removed. |
| `api_keys` | `api_keys` | `uuid` → `id`, `artifact_uuid` → `artifact_id`, `api_key_hash` → `key_hash`, `masked_api_key` → `masked_key` | `organization_name` | `org_id` | |
| `llm_provider_templates` | `llm_provider_templates` | `uuid` → `id` | `organization_name`, `is_system` | `configuration` text → JSON | Rows with `is_system = true` (built-ins ship in code) |
| `llm_providers` | `llm_providers` | `uuid` → `id` | | `org_id` from the artifact. The upstream API key moves out of `configuration` into `secrets`, and `upstream_secret_id` points at it. | |
| `llm_proxies` | `llm_proxies` | `uuid` → `id`, `project_uuid` → `project_id`, `provider_uuid` → `provider_id` | | `org_id`; upstream auth (including the key minted on the provider) moves to `secrets` | |
| `mcp_proxies` | `mcp_proxies` | `uuid` → `id` | | `project_id` NULL (1.0.0 proxies were org-level); `source_agent_id` NULL. Upstream auth in `configuration` moves to `secrets`. | `configuration.environments` (abandoned in 032) |
| `mcp_proxy_mappings` | `mcp_proxy_mappings` | `uuid` → `id`, `source_mcp_proxy_uuid` → `source_mcp_proxy_id` | | `org_id` | |
| `mcp_proxy_endpoints` | `mcp_proxy_endpoints` | `uuid` → `id`, `mcp_proxy_uuid` → `mcp_proxy_id` | | `org_id`; upstream auth → `secrets` (`upstream_secret_id`) | |
| `mcp_proxy_endpoint_environments` | `mcp_proxy_endpoint_environments` | `endpoint_uuid` → `endpoint_id`, `environment_uuid` → `environment_id`, `mcp_proxy_uuid` → `mcp_proxy_id`, `artifact_uuid` → `artifact_id` | `id` | `org_id` | Rows for environments not loaded |
| `mcp_proxy_scopes` | `mcp_proxy_scopes` + `mcp_proxy_scope_tools` | `uuid` → `id`, `mcp_proxy_uuid` → `mcp_proxy_id` | `tools` | One `mcp_proxy_scope_tools` row per entry in `tools`; `description` `''` → NULL | |
| `agent_configs` | `agent_gateway_settings` and `agent_runtime_configs` | `enable_api_key_security` → `api_key_security`, `enable_oauth_security` → `oauth_enabled`, `oauth_auth_header_prefix` → `oauth_header_prefix`, `resilience_timeout_seconds` → `timeout_seconds` | `id`, `org_name`, `project_name`, `agent_name`, `environment_name` | `agent_id` and `environment_id` by name lookup. For internal agents in an Agent Runtime target, `enable_auto_instrumentation` → `agent_runtime_configs.instrumentation_enabled` and `instrumentation_version` → `instrumentation_version`. `card_cors_*` NULL. | Instrumentation settings below Agent Runtime |
| `agent_configurations` | `agent_model_configs` | `uuid` → `id` | `organization_name`, `project_name`, `type_id` | `agent_id` = the agent's id (old column held its name); `type`: 1 → `llm`, 2 → `mcp`, 3 → `agent` | |
| `env_agent_model_mapping` | `agent_model_config_llm` | `config_uuid` → `config_id`, `environment_uuid` → `environment_id`, `llm_proxy_uuid` → `llm_proxy_id` | `id` | `org_id`; `policy_configuration` NULL → `[]` | |
| `agent_env_config_variables_mapping` | `agent_model_config_vars` | `config_uuid` → `config_id`, `environment_uuid` → `environment_id` | `id`, `secret_reference` | `org_id` | The proxy key behind `secret_reference`. External agents fetch a new key. |
| `env_agent_mcp_mapping` | `agent_model_config_mcp_envs` (+ `agent_model_config_mcp`) | `config_uuid` → `config_id`, `environment_uuid` → `environment_id`, `artifact_uuid` → `mapping_artifact_id`, `mcp_proxy_uuid` → `mcp_proxy_id` | `id` | `excluded = 0`. `agent_model_config_mcp` gets one row per config whose mappings agree on one proxy (as 2026.11.0 migration 049 did); `scope_recorded_at` NULL. | |
| `ai_applications` | `ai_applications` | `uuid` → `id` | `organization_name`, `project_name`, `environment_name` | `agent_id` by name lookup, `environment_id` by name lookup | |
| `agent_kinds` | `agent_kinds` + `agent_kind_labels` | | `org_name`, `project_name`, `agent_name`, `labels` | `source_agent_id` by name lookup (NULL if the agent is gone); one label row per `labels` entry | Below Agent Runtime: all rows |
| `agent_kind_versions` | `agent_kind_versions` | `agent_kind_id` → `kind_id`, `image_id` → `image_ref` | `build_name` | `org_id` from the kind; `build_id` from the loaded build with that WorkflowRun name, else NULL | Below Agent Runtime: all rows |
| `agent_thunder_clients` | `agent_identities` | | `org_name`, `project_name`, `agent_name`, `environment_name`, `provisioning_type`, `secret_ref_path`, `last_attempted_at`, `claimed_at` | `agent_id`, `environment_id` by name lookup; `''` → NULL for `thunder_agent_id`, `thunder_client_id`, `last_error`, `requested_by` | Identities issued by a per-environment Thunder that isn't the target's Thunder: set `status = 'pending'` so the reconciler provisions them again. Internal agents' stored client secrets. |
| `monitors` | `monitors` + `monitor_evaluators` | `next_run_time` → `next_run_at` | `org_name`, `project_name`, `agent_name`, `environment_name`, `evaluators`, `llm_provider_configs`, `sampling_rate` | `agent_id` = old `agent_id` (Component uid); `environment_id` = old `environment_id`; `sampling_percent = round(sampling_rate * 100)`; `enabled` = 1 unless the run schedule was suspended; one `monitor_evaluators` row per entry in `evaluators` (`identifier` → `evaluator_identifier`, `config`, `position` = list order); `judge_llm_provider_id` from the provider handle in `llm_provider_configs`; `description` `''` → NULL | Monitors of agents or environments not loaded |
| `monitor_llm_mapping` | `monitors.judge_llm_proxy_id`, `judge_policy_configuration`, `judge_api_key_secret_id` | `llm_proxy_uuid` → `judge_llm_proxy_id` | `id`, `secret_kv_path`, `secret_key` | The key value read from OpenBao at `secret_kv_path` goes into `secrets` | |
| `monitor_runs` | `monitor_runs` | `name` → `job_ref` | `llm_provider_configs` | `org_id`; `judge_config` = old `llm_provider_configs`; `attempt_count = 0`; `updated_at = completed_at` or `created_at` | Runs still `pending` or `running` at cut-over: load as `failed` with an explanatory `error_message` |
| `monitor_run_evaluators` | `monitor_run_evaluators` | `monitor_run_id` → `run_id`, `identifier` → `evaluator_identifier`, `count` → `item_count` | | `org_id` | |
| `scores` | `scores` | | `score` | `score_bp = round(score * 10000)`; `item_key` = `trace_id`, or `trace_id/span_id` when `span_id` is set; `span_label` `''` → NULL; `org_id` | |
| `custom_evaluators` | `evaluators` | | `org_name`, `deleted_at` | `org_id`; `description` `''` → NULL | Soft-deleted rows |
| `org_publisher_credentials` | `org_service_clients` (`purpose = 'eval_publisher'`) | | `id`, `org_name`, `org_uuid`, `secret_kv_path`, `secret_key`, `client_secret_encrypted` | `secret_id`: decrypt `client_secret_encrypted` with the old key (or read `secret_kv_path`) and store it in `secrets` | |
| `org_scheduler_credentials` | `org_service_clients` (`purpose = 'eval_scheduler'`) | | `id`, `org_uuid`, `secret_kv_path`, `secret_key`, `client_secret_encrypted` | as above | The OpenChoreo `ClusterAuthzRoleBinding` for the scheduler client |
| `env_thunder_system_clients`, `env_thunder_urls` | none | | | | All rows. One Thunder per install comes from config. |
| `migration_history` | `migration_history` | | | Written by the new service | Old entries |

### OpenChoreo resources and secret stores

| 1.0.0 source | New table | How |
|---|---|---|
| Thunder org unit (+ the shared namespace) | `organizations` | One row per OU that owns data: `id = ou_id`, `handle` = org handle, `thunder_ou_id = ou_id`. Each also gets its `default` environment and pipeline (step 4). |
| `Project` | `projects` | `id = metadata.uid`, `name`, display name and description annotations. `spec.deploymentPipelineRef` → `deployment_pipeline_id` (Agent Runtime only; otherwise NULL). |
| `Component` (external) | `agents` (`provisioning_type = 'external'`) + `agent_labels` | `id = metadata.uid`, `project_id` from `spec.owner.projectName`; user labels (reserved `openchoreo.dev/*` prefixes removed) → `agent_labels`. |
| `Component` (internal) | `agents` (`provisioning_type = 'internal'`) + `agent_labels` + `agent_workloads` | Agent Runtime only. `spec.workflow.parameters.repository` → `repo_url`, `repo_branch`, `repo_app_path`, `git_credential_id` (by `secretRef` name); the `build-source` label → `source_type` and `build_type`; language labels and `buildEnv` → `language`, `language_version`, `run_command`; `docker.filePath` → `dockerfile_path`; `endpoints[0]` → `interface_type`, `port`, `base_path`, `interface_schema_path`; kind labels → `agent_kind_version_id`. `workload_type = 'agent'`. |
| `Workload` | `agent_workloads.image_ref` (kind-sourced agents) | Image and endpoints. Source-built agents take the image from their release. |
| `WorkflowRun` (build) | `builds` | `id = metadata.uid`, `name`, status from conditions, `parameters` = workflow parameters, `commit_id`, `image_ref`, `started_at`; `steps` from conditions. Optional: old build history can be dropped. |
| `ComponentRelease` | `agent_releases` | One per release a `ReleaseBinding` points at: `image_ref`, `config` snapshot. `build_id` by image. |
| `ReleaseBinding` | `agent_deployments`, `agent_env_vars`, `agent_file_mounts`, `agent_runtime_configs` | `spec.state` `Active` → `desired_state = 'active'`, `Undeploy` → `undeployed`; `status = 'pending'` so the reconciler deploys it on the new runtime. `workloadOverrides.container.env[]` → `agent_env_vars` (`valueFrom.secretKeyRef` → value read from OpenBao into `secrets`; platform-injected variables such as `AMP_OTEL_ENDPOINT` and `AMP_AGENTID_*` are not loaded, because the new runtime injects them). `files[]` → `agent_file_mounts`. `componentTypeEnvironmentConfigs` → `replicas`, `cpu_*`, `memory_*`, `autoscaling_*`, and `runtimeClassName` → `isolation_tier`. |
| `Environment` | `environments` | `id = metadata.uid`, `name`, annotations → `display_name`, `description`, `isolation_tier`; `spec.isProduction`; `spec.dataPlaneRef` → `runtime_id`. `spec.gateway` isn't carried: gateway placement is in `gateway_environments`. Below Agent Runtime only the chosen environment is loaded, as `default`. |
| `ClusterDataPlane` | `runtimes` | Agent Runtime only. One row per org that uses it (`kind = 'kubernetes'`); connection details are re-entered, because they lived in OpenChoreo. |
| `DeploymentPipeline` | `deployment_pipelines`, `deployment_pipeline_environments`, `deployment_pipeline_paths` | Agent Runtime only. `promotionPaths` → one path row per source and target (drop the placeholder `" "` targets); `position` follows the promotion order. Below Agent Runtime each org has only its seeded `default` pipeline. |
| `GitSecret` + workflow-plane OpenBao | `git_credentials` + `secrets` | Agent Runtime only. `auth_type = 'basic'`, `username`, token read from OpenBao. |
| OpenChoreo `Secret` / `SecretReference` + data-plane OpenBao | `secrets` | Only the values a loaded row points at. Everything else is dropped. |
| Traits on `Component` and `ReleaseBinding` | `agent_gateway_settings`, `agent_runtime_configs` | Rendered from `agent_configs`, so nothing extra is read. `a2a-gateway-route` doesn't exist in 1.0.0. |
| `ProjectReleaseBinding`, `ComponentType`, `ClusterAuthzRoleBinding` | none | Platform plumbing, not product data. |
| `WorkflowRun` (eval) | `monitor_runs.job_ref` | History only. The name is kept for reference. |

### Data that can't be carried over

- Anything for environments other than the one chosen per org, when the target is below Agent Runtime.
- Hosted agents, builds, deployments and their configuration, when the target is below Agent Runtime.
- Agent identities issued by a per-environment Thunder the target doesn't use. They are provisioned again, and external agents need their new client secret.
- Proxy keys handed to agents through `agent_env_config_variables_mapping.secret_reference`. External agents must fetch new keys.
- Gateway change feeds, association mappings, built-in LLM templates stored as rows, and soft-deleted rows.
- Users, roles and role bindings: the target signs in from a config file.

### Steps

1. **Freeze.** Stop the old service and its workers. Let running builds and eval runs finish, or note them as failed.
2. **Export.** Take a `pg_dump` of the service database. Export the OpenChoreo resources listed above as YAML (Projects, Components, Workloads, WorkflowRuns, ComponentReleases, ReleaseBindings, Environments, ClusterDataPlanes, DeploymentPipelines). Read the secret values that loaded rows point at from the data-plane and workflow-plane OpenBao, and keep them encrypted at rest.
3. **Choose the target tier and environments.** For an AI Gateway or Agent Manager target, choose one environment per org (see [Common rules](#environments-in-a-target-below-agent-runtime)).
4. **Create the target database.** Start the new service once against an empty database. The migrations create every table and the lock rows. Stop it.
5. **Load orgs, environments and pipelines.** Insert `organizations`, then `environments` with their kept ids, then for each org the `default` deployment pipeline with its default environment at position 0. (The service creates these itself only for orgs it hasn't seen.) In an Agent Runtime target, also load `runtimes`, the other environments and the old pipelines.
6. **Load projects, secrets and agents.** Projects with kept ids. One `secrets` row per carried secret value, encrypted with the new master key. Agents with kept ids, their labels, and (Agent Runtime) their workloads.
7. **Load the service tables** in the order of the [table map](#service-database-tables), translating names to ids.
8. **Load runtime state** (Agent Runtime only): builds, releases, deployments with `status = 'pending'`, runtime configs, env vars, file mounts, agent kinds.
9. **Rebuild gateway state.** Skip `deployment_status`. For each artifact and gateway, write an `artifact_gateway_status` row with `status = 'PENDING'` and `status_desired = 'DEPLOYED'`. Point each gateway at the new control plane: tokens are carried as hashes, so gateways keep their registration token. The drift scan redeploys every artifact.
10. **Re-provision identities.** Identities from a Thunder the target doesn't use are left `pending`. Start the service; the identity reconciler provisions them. Send external agent owners their new client secrets.
11. **Verify.** Compare row counts per table against the export, open each org in the console, call one agent through each gateway, and (Agent Manager and up) check that monitors schedule runs.

## Source: 2026.11.0 (STS release)

Tag `amp/v2026.11.0`. Same storage model as 1.0.0, with 50 migrations (`0001`-`0050`) and 44 tables. Migrations `0001`-`0043` are identical to 1.0.0, so everything in the [1.0.0 section](#source-agent-manager-100) applies. This section lists only the differences.

### Service database tables added since 1.0.0

| 2026.11.0 table | New table | Columns renamed | Columns dropped | New columns and how they're filled | Not carried over |
|---|---|---|---|---|---|
| `a2a_publications` (044-048) | `agent_exposures` (`protocol = 'a2a'`) | `artifact_uuid` → `artifact_id`, `environment_uuid` → `environment_id` | `project_name`, `agent_name`, `environment_name`, `status`, `attempt_count`, `last_error`, `next_attempt_at`, `published_upstream_url`, `source` | `agent_id` = `agent_uuid`, or a name lookup when NULL | The publication queue state. The gateway render queue republishes it (step 9 above). |
| `a2a_agent_cards` (047) | `agent_cards` | `environment_uuid` → `environment_id` | `project_name`, `agent_name`, `environment_name`, `release_name`, `attempted_release_name` | `agent_id` by name lookup; `''` → NULL for `source_url`, `card_hash`, `last_error`; `claimed_by`, `claimed_until` NULL | Release names: they named OpenChoreo ComponentReleases. Set `status = 'pending'` to refetch cards. |
| `a2a_proxies` (048) | `a2a_proxies` | `environment_uuid` → `environment_id` | `project_name`, `agent_name`, `environment_name`, `upstream_auth_secret` | `agent_id` by name lookup; `upstream_auth_secret_id`: decrypt the old value and store it in `secrets`; `artifact_id` = the agent's A2A exposure artifact in that environment; `upstream_auth_header` `''` → NULL | |
| `agent_mcp_config_proxy` (049) | `agent_model_config_mcp` | `config_uuid` → `config_id`, `mcp_proxy_uuid` → `mcp_proxy_id` | `type_id`, `updated_at` | `org_id`; `scope_recorded_at` from `agent_mcp_config_scope` | Rows from the 1.0.0 backfill are already here, so don't derive them again from `env_agent_mcp_mapping` |
| `agent_mcp_config_env_exclusion` (050) | `agent_model_config_mcp_envs` with `excluded = 1` | `config_uuid` → `config_id`, `environment_uuid` → `environment_id` | `type_id` | `org_id`; `mapping_artifact_id` and `mcp_proxy_id` NULL | |
| `agent_mcp_config_scope` (050) | `agent_model_config_mcp.scope_recorded_at` | `recorded_at` → `scope_recorded_at` | `type_id` | | |

### Columns added to existing tables since 1.0.0

| Table | Columns | New home |
|---|---|---|
| `agent_configs` | `card_cors_enabled`, `card_cors_allow_origins`, `card_cors_allow_headers`, `card_cors_allow_credentials` (045) | `agent_gateway_settings`, same names |
| `agent_configurations` | unique `(uuid, type_id)` (049) | Not needed: `agent_model_configs.type` is a column and the child tables point at `id` |

### OpenChoreo differences

- `Component` gains A2A agent sub-types and the `a2a-gateway-route` trait. Both come from the A2A tables above and need nothing extra.
- `WorkflowRun` and the cell namespace carry a `cloud.wso2.com/orguuid` label. It confirms the org of an exported build when several orgs share the namespace.

### Steps

The [1.0.0 steps](#steps) apply, with two additions in step 7: load the A2A tables after `agents` and `artifacts`, and load the MCP connection tables (`agent_mcp_config_*`) before falling back to the 1.0.0 derivation from `env_agent_mcp_mapping`.

## Open questions

- **Sources between releases.** `main` has migrations `0051`-`0059` (gateway reconciliation, REST agent exposures, gateway hosting, project-scoped MCP proxies, event destinations, event outbox, monitor alerts) that no tagged release carries. Add a source section if a release ships them.
- **Capabilities without tables yet.** Shadow AI discovery, budgets and chargeback, and the kill switch have no tables of their own. Budgets and rate limits live in gateway policy configuration (JSON on proxies), and insights come from the observability store. Each needs a schema entry here once its design settles.
- **Migration tool.** Whether the steps above become a CLI (`amctl migrate`), a one-off job, or a documented manual procedure is not decided. #2435 tracks the user-facing form.
