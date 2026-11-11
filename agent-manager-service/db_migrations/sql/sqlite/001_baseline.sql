-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE organizations (
    id TEXT NOT NULL,
    handle TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    thunder_ou_id TEXT,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_organizations PRIMARY KEY (id),
    CONSTRAINT uq_organizations_handle UNIQUE (handle)
);
CREATE INDEX idx_organizations_thunder_ou_id ON organizations (thunder_ou_id);

CREATE TABLE projects (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_projects PRIMARY KEY (id),
    CONSTRAINT uq_projects_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_projects_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_projects_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);

CREATE TABLE environments (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    is_production INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_environments PRIMARY KEY (id),
    CONSTRAINT uq_environments_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_environments_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_environments_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_environments_1 CHECK (is_production IN (0, 1))
);

CREATE TABLE agents (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    provisioning_type TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    sub_type TEXT,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agents PRIMARY KEY (id),
    CONSTRAINT uq_agents_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_agents_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agents_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT ck_agents_1 CHECK (provisioning_type IN ('external', 'internal'))
);
CREATE INDEX idx_agents_org_id_project_id ON agents (org_id, project_id);

CREATE TABLE agent_labels (
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    label_key TEXT NOT NULL,
    label_value TEXT,
    CONSTRAINT pk_agent_labels PRIMARY KEY (agent_id, label_key),
    CONSTRAINT fk_agent_labels_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_agent_labels_org_id_label_key_label_value ON agent_labels (org_id, label_key, label_value);

CREATE TABLE agent_identities (
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    thunder_agent_id TEXT,
    thunder_client_id TEXT,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_error TEXT,
    next_retry_at TIMESTAMP,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    requested_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_identities PRIMARY KEY (agent_id, environment_id),
    CONSTRAINT fk_agent_identities_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_identities_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_identities_1 CHECK (status IN ('pending', 'in_progress', 'completed', 'failed'))
);
CREATE INDEX idx_agent_identities_status_next_retry_at ON agent_identities (status, next_retry_at);

CREATE TABLE agent_gateway_settings (
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    api_key_security INTEGER NOT NULL,
    oauth_enabled INTEGER NOT NULL,
    oauth_issuers TEXT NOT NULL,
    oauth_audiences TEXT NOT NULL,
    oauth_header_name TEXT NOT NULL,
    oauth_header_prefix TEXT NOT NULL,
    oauth_forward_token INTEGER NOT NULL,
    cors_enabled INTEGER NOT NULL,
    cors_allow_origins TEXT NOT NULL,
    cors_allow_methods TEXT NOT NULL,
    cors_allow_headers TEXT NOT NULL,
    cors_allow_credentials INTEGER NOT NULL,
    timeout_seconds INTEGER,
    card_cors_enabled INTEGER,
    card_cors_allow_origins TEXT,
    card_cors_allow_headers TEXT,
    card_cors_allow_credentials INTEGER,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_gateway_settings PRIMARY KEY (agent_id, environment_id),
    CONSTRAINT fk_agent_gateway_settings_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_gateway_settings_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_gateway_settings_1 CHECK (api_key_security IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_2 CHECK (oauth_enabled IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_3 CHECK (json_valid(oauth_issuers)),
    CONSTRAINT ck_agent_gateway_settings_4 CHECK (json_valid(oauth_audiences)),
    CONSTRAINT ck_agent_gateway_settings_5 CHECK (oauth_forward_token IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_6 CHECK (cors_enabled IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_7 CHECK (json_valid(cors_allow_origins)),
    CONSTRAINT ck_agent_gateway_settings_8 CHECK (json_valid(cors_allow_methods)),
    CONSTRAINT ck_agent_gateway_settings_9 CHECK (json_valid(cors_allow_headers)),
    CONSTRAINT ck_agent_gateway_settings_10 CHECK (cors_allow_credentials IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_11 CHECK (card_cors_enabled IS NULL OR card_cors_enabled IN (0, 1)),
    CONSTRAINT ck_agent_gateway_settings_12 CHECK (card_cors_allow_origins IS NULL OR json_valid(card_cors_allow_origins)),
    CONSTRAINT ck_agent_gateway_settings_13 CHECK (card_cors_allow_headers IS NULL OR json_valid(card_cors_allow_headers)),
    CONSTRAINT ck_agent_gateway_settings_14 CHECK (card_cors_allow_credentials IS NULL OR card_cors_allow_credentials IN (0, 1))
);

CREATE TABLE secrets (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    owner_kind TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    ciphertext BLOB,
    key_id TEXT,
    external_ref TEXT,
    version INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_secrets PRIMARY KEY (id),
    CONSTRAINT uq_secrets_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_secrets_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);
CREATE INDEX idx_secrets_org_id_owner_kind_owner_id ON secrets (org_id, owner_kind, owner_id);

CREATE TABLE gateways (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    functionality_type TEXT NOT NULL,
    hosting TEXT NOT NULL,
    vhost TEXT NOT NULL,
    runtime_url TEXT,
    is_critical INTEGER NOT NULL,
    is_active INTEGER NOT NULL,
    properties TEXT NOT NULL,
    manifest TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateways PRIMARY KEY (id),
    CONSTRAINT uq_gateways_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_gateways_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_gateways_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_gateways_1 CHECK (is_critical IN (0, 1)),
    CONSTRAINT ck_gateways_2 CHECK (is_active IN (0, 1)),
    CONSTRAINT ck_gateways_3 CHECK (json_valid(properties)),
    CONSTRAINT ck_gateways_4 CHECK (manifest IS NULL OR json_valid(manifest)),
    CONSTRAINT ck_gateways_5 CHECK (functionality_type IN ('ingress', 'egress', 'both')),
    CONSTRAINT ck_gateways_6 CHECK (hosting IN ('managed', 'self_hosted')),
    CONSTRAINT ck_gateways_7 CHECK (hosting <> 'self_hosted' OR functionality_type = 'egress')
);

CREATE TABLE gateway_environments (
    gateway_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateway_environments PRIMARY KEY (gateway_id, environment_id),
    CONSTRAINT fk_gateway_environments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_gateway_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);
CREATE INDEX idx_gateway_environments_environment_id ON gateway_environments (environment_id);

CREATE TABLE environment_default_gateways (
    environment_id TEXT NOT NULL,
    role TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_environment_default_gateways PRIMARY KEY (environment_id, role),
    CONSTRAINT fk_environment_default_gateways_gateway_id_environment_id FOREIGN KEY (gateway_id, environment_id) REFERENCES gateway_environments (gateway_id, environment_id),
    CONSTRAINT fk_environment_default_gateways_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_environment_default_gateways_1 CHECK (role IN ('ingress', 'egress'))
);
CREATE INDEX idx_environment_default_gateways_gateway_id ON environment_default_gateways (gateway_id);

CREATE TABLE gateway_tokens (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    CONSTRAINT pk_gateway_tokens PRIMARY KEY (id),
    CONSTRAINT uq_gateway_tokens_token_prefix UNIQUE (token_prefix),
    CONSTRAINT fk_gateway_tokens_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_gateway_tokens_1 CHECK (status IN ('active', 'revoked')),
    CONSTRAINT ck_gateway_tokens_2 CHECK (revoked_at IS NULL OR status = 'revoked')
);
CREATE INDEX idx_gateway_tokens_gateway_id_status ON gateway_tokens (gateway_id, status);

CREATE TABLE gateway_identity_providers (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    name TEXT NOT NULL,
    issuer TEXT,
    jwks_uri TEXT,
    description TEXT,
    type TEXT NOT NULL,
    jwks_skip_tls_verify INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateway_identity_providers PRIMARY KEY (id),
    CONSTRAINT uq_gateway_identity_providers_gateway_id_name UNIQUE (gateway_id, name),
    CONSTRAINT fk_gateway_identity_providers_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_gateway_identity_providers_1 CHECK (jwks_skip_tls_verify IN (0, 1))
);

CREATE TABLE gateway_event_states (
    gateway_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    version_id TEXT,
    seq INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateway_event_states PRIMARY KEY (gateway_id),
    CONSTRAINT fk_gateway_event_states_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE gateway_events (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    entity_type TEXT NOT NULL,
    action TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    event_data TEXT NOT NULL,
    originated_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateway_events PRIMARY KEY (id),
    CONSTRAINT fk_gateway_events_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_gateway_events_1 CHECK (json_valid(event_data))
);
CREATE INDEX idx_gateway_events_gateway_id_seq ON gateway_events (gateway_id, seq);
CREATE INDEX idx_gateway_events_gateway_id_processed_at ON gateway_events (gateway_id, processed_at);

CREATE TABLE artifacts (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    in_catalog INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_artifacts PRIMARY KEY (id),
    CONSTRAINT uq_artifacts_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_artifacts_org_id_name_version UNIQUE (org_id, name, version),
    CONSTRAINT uq_artifacts_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_artifacts_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_artifacts_1 CHECK (in_catalog IN (0, 1)),
    CONSTRAINT ck_artifacts_2 CHECK (kind IN ('RestApi', 'WebSubAPI', 'LlmProvider', 'LlmProxy', 'Agent', 'McpProxy', 'McpProxyMapping'))
);

CREATE TABLE artifact_deployments (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    base_deployment_id TEXT,
    name TEXT NOT NULL,
    content BLOB NOT NULL,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_artifact_deployments PRIMARY KEY (id),
    CONSTRAINT fk_artifact_deployments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_artifact_deployments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id),
    CONSTRAINT fk_artifact_deployments_base_deployment_id FOREIGN KEY (base_deployment_id) REFERENCES artifact_deployments (id),
    CONSTRAINT ck_artifact_deployments_1 CHECK (metadata IS NULL OR json_valid(metadata))
);
CREATE INDEX idx_artifact_deployments_artifact_id_gateway_id_created_at ON artifact_deployments (artifact_id, gateway_id, created_at);
CREATE INDEX idx_artifact_deployments_gateway_id ON artifact_deployments (gateway_id);
CREATE INDEX idx_artifact_deployments_base_deployment_id ON artifact_deployments (base_deployment_id);

CREATE TABLE artifact_gateway_status (
    artifact_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    deployment_id TEXT,
    status TEXT NOT NULL,
    status_desired TEXT NOT NULL,
    status_reason TEXT,
    render_reason TEXT,
    performed_at TIMESTAMP NOT NULL,
    next_attempt_at TIMESTAMP,
    attempt_count INTEGER NOT NULL,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_artifact_gateway_status PRIMARY KEY (artifact_id, gateway_id),
    CONSTRAINT fk_artifact_gateway_status_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_artifact_gateway_status_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id),
    CONSTRAINT fk_artifact_gateway_status_deployment_id FOREIGN KEY (deployment_id) REFERENCES artifact_deployments (id),
    CONSTRAINT ck_artifact_gateway_status_1 CHECK (status IN ('PENDING', 'DEPLOYING', 'DEPLOYED', 'UNDEPLOYING', 'UNDEPLOYED', 'FAILED')),
    CONSTRAINT ck_artifact_gateway_status_2 CHECK (deployment_id IS NOT NULL OR status IN ('PENDING', 'UNDEPLOYED')),
    CONSTRAINT ck_artifact_gateway_status_3 CHECK ((status_desired = 'DEPLOYED' AND status IN ('PENDING', 'DEPLOYING', 'DEPLOYED', 'FAILED')) OR (status_desired = 'UNDEPLOYED' AND status IN ('UNDEPLOYING', 'UNDEPLOYED', 'FAILED')))
);
CREATE INDEX idx_artifact_gateway_status_gateway_id ON artifact_gateway_status (gateway_id);
CREATE INDEX idx_artifact_gateway_status_deployment_id ON artifact_gateway_status (deployment_id);
CREATE INDEX idx_artifact_gateway_status_next_attempt_at ON artifact_gateway_status (next_attempt_at);
CREATE INDEX idx_artifact_gateway_status_status ON artifact_gateway_status (status);

CREATE TABLE llm_provider_templates (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    configuration TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_llm_provider_templates PRIMARY KEY (id),
    CONSTRAINT uq_llm_provider_templates_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT fk_llm_provider_templates_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_llm_provider_templates_1 CHECK (json_valid(configuration))
);

CREATE TABLE llm_providers (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    description TEXT,
    template_handle TEXT NOT NULL,
    openapi_spec TEXT,
    model_list TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    upstream_secret_id TEXT,
    created_by TEXT,
    CONSTRAINT pk_llm_providers PRIMARY KEY (id),
    CONSTRAINT uq_llm_providers_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_llm_providers_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_llm_providers_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_llm_providers_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_llm_providers_template_handle ON llm_providers (template_handle);
CREATE INDEX idx_llm_providers_upstream_secret_id ON llm_providers (upstream_secret_id);

CREATE TABLE llm_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    description TEXT,
    openapi_spec TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    upstream_secret_id TEXT,
    created_by TEXT,
    CONSTRAINT pk_llm_proxies PRIMARY KEY (id),
    CONSTRAINT uq_llm_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_llm_proxies_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_provider_id FOREIGN KEY (org_id, provider_id) REFERENCES llm_providers (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_llm_proxies_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_llm_proxies_org_id_project_id ON llm_proxies (org_id, project_id);
CREATE INDEX idx_llm_proxies_provider_id ON llm_proxies (provider_id);
CREATE INDEX idx_llm_proxies_upstream_secret_id ON llm_proxies (upstream_secret_id);

CREATE TABLE mcp_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_id TEXT,
    description TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    created_by TEXT,
    CONSTRAINT pk_mcp_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxies_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT ck_mcp_proxies_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mcp_proxies_org_id_project_id ON mcp_proxies (org_id, project_id);

CREATE TABLE mcp_proxy_mappings (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    source_mcp_proxy_id TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    CONSTRAINT pk_mcp_proxy_mappings PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_mappings_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxy_mappings_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_mappings_org_id_source_mcp_proxy_id FOREIGN KEY (org_id, source_mcp_proxy_id) REFERENCES mcp_proxies (org_id, id),
    CONSTRAINT ck_mcp_proxy_mappings_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mcp_proxy_mappings_source_mcp_proxy_id ON mcp_proxy_mappings (source_mcp_proxy_id);

CREATE TABLE mcp_proxy_scopes (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    action TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mcp_proxy_scopes PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_scopes_mcp_proxy_id_action UNIQUE (mcp_proxy_id, action),
    CONSTRAINT uq_mcp_proxy_scopes_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxy_scopes_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mcp_proxy_scope_tools (
    scope_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_mcp_proxy_scope_tools PRIMARY KEY (scope_id, tool_name),
    CONSTRAINT fk_mcp_proxy_scope_tools_org_id_scope_id FOREIGN KEY (org_id, scope_id) REFERENCES mcp_proxy_scopes (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mcp_proxy_scope_tools_org_id_tool_name ON mcp_proxy_scope_tools (org_id, tool_name);

CREATE TABLE mcp_proxy_endpoints (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    upstream_secret_id TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mcp_proxy_endpoints PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_endpoints_mcp_proxy_id_handle UNIQUE (mcp_proxy_id, handle),
    CONSTRAINT uq_mcp_proxy_endpoints_mcp_proxy_id_id UNIQUE (mcp_proxy_id, id),
    CONSTRAINT fk_mcp_proxy_endpoints_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_endpoints_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_mcp_proxy_endpoints_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mcp_proxy_endpoints_upstream_secret_id ON mcp_proxy_endpoints (upstream_secret_id);

CREATE TABLE mcp_proxy_endpoint_environments (
    endpoint_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mcp_proxy_endpoint_environments PRIMARY KEY (endpoint_id, environment_id),
    CONSTRAINT uq_mcp_proxy_endpoint_environments_mcp_proxy_id_environment_id UNIQUE (mcp_proxy_id, environment_id),
    CONSTRAINT uq_mcp_proxy_endpoint_environments_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mcp_proxy_endpoint_environments_mcp_proxy_id_endpoint_id FOREIGN KEY (mcp_proxy_id, endpoint_id) REFERENCES mcp_proxy_endpoints (mcp_proxy_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_endpoint_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_mcp_proxy_endpoint_environments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id)
);

CREATE TABLE agent_model_configs (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_model_configs PRIMARY KEY (id),
    CONSTRAINT uq_agent_model_configs_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_agent_model_configs_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_model_configs_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_agent_model_configs_1 CHECK (type IN ('llm', 'mcp', 'other'))
);

CREATE TABLE agent_model_config_llm (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    llm_proxy_id TEXT NOT NULL,
    policy_configuration TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_model_config_llm PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_agent_model_config_llm_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_llm_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_model_config_llm_org_id_llm_proxy_id FOREIGN KEY (org_id, llm_proxy_id) REFERENCES llm_proxies (org_id, id),
    CONSTRAINT ck_agent_model_config_llm_1 CHECK (json_valid(policy_configuration))
);
CREATE INDEX idx_agent_model_config_llm_llm_proxy_id ON agent_model_config_llm (llm_proxy_id);

CREATE TABLE agent_model_config_mcp (
    config_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    scope_recorded_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_model_config_mcp PRIMARY KEY (config_id),
    CONSTRAINT fk_agent_model_config_mcp_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_mcp_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id)
);
CREATE INDEX idx_agent_model_config_mcp_mcp_proxy_id ON agent_model_config_mcp (mcp_proxy_id);

CREATE TABLE agent_model_config_mcp_envs (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mapping_artifact_id TEXT,
    excluded INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_model_config_mcp_envs PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_mapping_artifact_id FOREIGN KEY (org_id, mapping_artifact_id) REFERENCES mcp_proxy_mappings (org_id, id),
    CONSTRAINT ck_agent_model_config_mcp_envs_1 CHECK (excluded IN (0, 1))
);
CREATE INDEX idx_agent_model_config_mcp_envs_mapping_artifact_id ON agent_model_config_mcp_envs (mapping_artifact_id);

CREATE TABLE agent_model_config_vars (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    variable_key TEXT NOT NULL,
    org_id TEXT NOT NULL,
    variable_name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_model_config_vars PRIMARY KEY (config_id, environment_id, variable_key),
    CONSTRAINT fk_agent_model_config_vars_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_vars_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE ai_applications (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_ai_applications PRIMARY KEY (id),
    CONSTRAINT uq_ai_applications_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_ai_applications_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_ai_applications_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE api_keys (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    masked_key TEXT NOT NULL,
    status TEXT NOT NULL,
    purpose INTEGER NOT NULL,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_api_keys PRIMARY KEY (id),
    CONSTRAINT uq_api_keys_artifact_id_name UNIQUE (artifact_id, name),
    CONSTRAINT fk_api_keys_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_api_keys_key_hash ON api_keys (key_hash);

CREATE TABLE agent_exposures (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    protocol TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_exposures PRIMARY KEY (id),
    CONSTRAINT uq_agent_exposures_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_agent_exposures_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_agent_exposures_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_exposures_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_exposures_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id),
    CONSTRAINT ck_agent_exposures_1 CHECK (protocol IN ('a2a', 'rest'))
);

CREATE TABLE agent_cards (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    source TEXT NOT NULL,
    source_url TEXT,
    card TEXT,
    card_hash TEXT,
    fetched_at TIMESTAMP,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_error TEXT,
    next_attempt_at TIMESTAMP,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_agent_cards PRIMARY KEY (id),
    CONSTRAINT uq_agent_cards_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_agent_cards_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_cards_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_cards_1 CHECK (card IS NULL OR json_valid(card))
);
CREATE INDEX idx_agent_cards_status_next_attempt_at ON agent_cards (status, next_attempt_at);

CREATE TABLE a2a_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    upstream_auth_header TEXT,
    upstream_auth_secret_id TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_a2a_proxies PRIMARY KEY (id),
    CONSTRAINT uq_a2a_proxies_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_a2a_proxies_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_a2a_proxies_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_a2a_proxies_org_id_upstream_auth_secret_id FOREIGN KEY (org_id, upstream_auth_secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_a2a_proxies_upstream_auth_secret_id ON a2a_proxies (upstream_auth_secret_id);

CREATE TABLE event_destinations (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    kind TEXT NOT NULL,
    config TEXT NOT NULL,
    secret_id TEXT,
    availability TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_event_destinations PRIMARY KEY (id),
    CONSTRAINT uq_event_destinations_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_event_destinations_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_event_destinations_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_event_destinations_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_event_destinations_1 CHECK (json_valid(config)),
    CONSTRAINT ck_event_destinations_2 CHECK (availability IN ('all', 'org', 'projects'))
);
CREATE INDEX idx_event_destinations_secret_id ON event_destinations (secret_id);

CREATE TABLE event_destination_projects (
    destination_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_event_destination_projects PRIMARY KEY (destination_id, project_id),
    CONSTRAINT fk_event_destination_projects_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_destination_projects_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id)
);
CREATE INDEX idx_event_destination_projects_org_id_project_id ON event_destination_projects (org_id, project_id);

CREATE TABLE event_subscriptions (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    project_id TEXT,
    agent_id TEXT,
    name TEXT NOT NULL,
    description TEXT,
    destination_id TEXT NOT NULL,
    enabled INTEGER NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_event_subscriptions PRIMARY KEY (id),
    CONSTRAINT uq_event_subscriptions_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_event_subscriptions_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_subscriptions_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_event_subscriptions_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id),
    CONSTRAINT ck_event_subscriptions_1 CHECK (enabled IN (0, 1)),
    CONSTRAINT ck_event_subscriptions_2 CHECK (scope IN ('org', 'project', 'agent')),
    CONSTRAINT ck_event_subscriptions_3 CHECK ((scope = 'org' AND project_id IS NULL AND agent_id IS NULL) OR (scope = 'project' AND project_id IS NOT NULL AND agent_id IS NULL) OR (scope = 'agent' AND project_id IS NOT NULL AND agent_id IS NOT NULL))
);
CREATE INDEX idx_event_subscriptions_org_id_scope_project_id_agent_id ON event_subscriptions (org_id, scope, project_id, agent_id);
CREATE INDEX idx_event_subscriptions_destination_id ON event_subscriptions (destination_id);

CREATE TABLE event_subscription_types (
    subscription_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_event_subscription_types PRIMARY KEY (subscription_id, event_type),
    CONSTRAINT fk_event_subscription_types_org_id_subscription_id FOREIGN KEY (org_id, subscription_id) REFERENCES event_subscriptions (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_event_subscription_types_event_type ON event_subscription_types (event_type);

CREATE TABLE event_subscription_environments (
    subscription_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_event_subscription_environments PRIMARY KEY (subscription_id, environment_id),
    CONSTRAINT fk_event_subscription_environments_org_id_subscription_id FOREIGN KEY (org_id, subscription_id) REFERENCES event_subscriptions (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_subscription_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE event_outbox (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    body TEXT NOT NULL,
    available_at TIMESTAMP NOT NULL,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    CONSTRAINT pk_event_outbox PRIMARY KEY (id),
    CONSTRAINT fk_event_outbox_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_event_outbox_1 CHECK (json_valid(body))
);
CREATE INDEX idx_event_outbox_available_at_id ON event_outbox (available_at, id);

CREATE TABLE event_deliveries (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_scope TEXT,
    project_id TEXT,
    agent_id TEXT,
    environment_id TEXT,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    response_code INTEGER,
    last_error TEXT,
    delivered_at TIMESTAMP,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_event_deliveries PRIMARY KEY (id),
    CONSTRAINT uq_event_deliveries_destination_id_event_id UNIQUE (destination_id, event_id),
    CONSTRAINT fk_event_deliveries_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_event_deliveries_org_id_created_at ON event_deliveries (org_id, created_at);
CREATE INDEX idx_event_deliveries_destination_id_created_at ON event_deliveries (destination_id, created_at);
CREATE INDEX idx_event_deliveries_status_claimed_until ON event_deliveries (status, claimed_until);

CREATE TABLE event_delivery_subscriptions (
    delivery_id TEXT NOT NULL,
    subscription_id TEXT NOT NULL,
    CONSTRAINT pk_event_delivery_subscriptions PRIMARY KEY (delivery_id, subscription_id),
    CONSTRAINT fk_event_delivery_subscriptions_delivery_id FOREIGN KEY (delivery_id) REFERENCES event_deliveries (id) ON DELETE CASCADE
);
CREATE INDEX idx_event_delivery_subscriptions_subscription_id ON event_delivery_subscriptions (subscription_id);

CREATE TABLE audit_events (
    id TEXT NOT NULL,
    org_id TEXT,
    occurred_at TIMESTAMP NOT NULL,
    schema_version INTEGER NOT NULL,
    action TEXT NOT NULL,
    action_class TEXT NOT NULL,
    severity INTEGER NOT NULL,
    outcome TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    actor_display TEXT,
    actor_token_id TEXT,
    on_behalf_of TEXT,
    auth_method TEXT,
    surface TEXT NOT NULL,
    source_ip TEXT,
    user_agent TEXT,
    correlation_id TEXT,
    request_method TEXT,
    request_path TEXT,
    resource_type TEXT,
    resource_id TEXT,
    resource_name TEXT,
    project_name TEXT,
    environment_name TEXT,
    status_code INTEGER,
    error_code TEXT,
    error_message TEXT,
    required_permission TEXT,
    duration_ms INTEGER,
    details TEXT,
    CONSTRAINT pk_audit_events PRIMARY KEY (id),
    CONSTRAINT ck_audit_events_1 CHECK (details IS NULL OR json_valid(details)),
    CONSTRAINT ck_audit_events_2 CHECK (outcome IN ('success', 'failure', 'deny', 'unknown'))
);
CREATE INDEX idx_audit_events_org_id_occurred_at_id ON audit_events (org_id, occurred_at, id);
CREATE INDEX idx_audit_events_org_id_actor_id_occurred_at ON audit_events (org_id, actor_id, occurred_at);
CREATE INDEX idx_audit_events_org_id_resource_type_resource_id_occurred_at ON audit_events (org_id, resource_type, resource_id, occurred_at);
CREATE INDEX idx_audit_events_org_id_action_occurred_at ON audit_events (org_id, action, occurred_at);

CREATE TABLE usage_events (
    id TEXT NOT NULL,
    org_id TEXT,
    occurred_at TIMESTAMP NOT NULL,
    source TEXT NOT NULL,
    feature_code TEXT NOT NULL,
    username TEXT,
    session_id TEXT,
    status_code INTEGER,
    product_version TEXT NOT NULL,
    dimensions TEXT,
    CONSTRAINT pk_usage_events PRIMARY KEY (id),
    CONSTRAINT ck_usage_events_1 CHECK (dimensions IS NULL OR json_valid(dimensions))
);
CREATE INDEX idx_usage_events_org_id_occurred_at_id ON usage_events (org_id, occurred_at, id);
CREATE INDEX idx_usage_events_feature_code_occurred_at ON usage_events (feature_code, occurred_at);

CREATE TABLE locks (
    name TEXT NOT NULL,
    owner TEXT,
    expires_at TIMESTAMP,
    CONSTRAINT pk_locks PRIMARY KEY (name)
);
INSERT INTO locks (name) VALUES ('agent_identity.reconciler');
INSERT INTO locks (name) VALUES ('gateway.drift_scan');
