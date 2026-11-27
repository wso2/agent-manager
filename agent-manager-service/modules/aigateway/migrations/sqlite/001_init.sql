-- Code generated from modules/aigateway/schema/001_init.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_agent_gateway_settings (
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
    CONSTRAINT pk_mod_ai_gateway_agent_gateway_settings PRIMARY KEY (agent_id, environment_id),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_1 CHECK (api_key_security IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_2 CHECK (oauth_enabled IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_3 CHECK (json_valid(oauth_issuers)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_4 CHECK (json_valid(oauth_audiences)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_5 CHECK (oauth_forward_token IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_6 CHECK (cors_enabled IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_7 CHECK (json_valid(cors_allow_origins)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_8 CHECK (json_valid(cors_allow_methods)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_9 CHECK (json_valid(cors_allow_headers)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_10 CHECK (cors_allow_credentials IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_11 CHECK (card_cors_enabled IS NULL OR card_cors_enabled IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_12 CHECK (card_cors_allow_origins IS NULL OR json_valid(card_cors_allow_origins)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_13 CHECK (card_cors_allow_headers IS NULL OR json_valid(card_cors_allow_headers)),
    CONSTRAINT ck_mod_ai_gateway_agent_gateway_settings_14 CHECK (card_cors_allow_credentials IS NULL OR card_cors_allow_credentials IN (0, 1))
);

CREATE TABLE mod_ai_gateway_agent_bindings (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    position INTEGER NOT NULL,
    target TEXT NOT NULL,
    request TEXT NOT NULL,
    state TEXT NOT NULL,
    last_error TEXT,
    config_id TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_bindings PRIMARY KEY (id),
    CONSTRAINT ck_mod_ai_gateway_agent_bindings_1 CHECK (json_valid(request)),
    CONSTRAINT ck_mod_ai_gateway_agent_bindings_2 CHECK (kind IN ('llm', 'mcp')),
    CONSTRAINT ck_mod_ai_gateway_agent_bindings_3 CHECK (state IN ('pending', 'ready', 'failed', 'removing'))
);
CREATE INDEX idx_mod_ai_gateway_agent_bindings_org_id_agent_id ON mod_ai_gateway_agent_bindings (org_id, agent_id);

CREATE TABLE mod_ai_gateway_agent_teardowns (
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_name TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    last_error TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_teardowns PRIMARY KEY (agent_id),
    CONSTRAINT uq_mod_ai_gateway_agent_teardowns_org_id_project_name__49fd1520 UNIQUE (org_id, project_name, agent_name)
);

CREATE TABLE mod_ai_gateway_gateways (
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
    CONSTRAINT pk_mod_ai_gateway_gateways PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateways_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_mod_ai_gateway_gateways_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_gateways_1 CHECK (is_critical IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_gateways_2 CHECK (is_active IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_gateways_3 CHECK (json_valid(properties)),
    CONSTRAINT ck_mod_ai_gateway_gateways_4 CHECK (manifest IS NULL OR json_valid(manifest)),
    CONSTRAINT ck_mod_ai_gateway_gateways_5 CHECK (functionality_type IN ('ingress', 'egress', 'both')),
    CONSTRAINT ck_mod_ai_gateway_gateways_6 CHECK (hosting IN ('managed', 'self_hosted')),
    CONSTRAINT ck_mod_ai_gateway_gateways_7 CHECK (hosting <> 'self_hosted' OR functionality_type = 'egress')
);

CREATE TABLE mod_ai_gateway_gateway_environments (
    gateway_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_environments PRIMARY KEY (gateway_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_gateway_environments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_gateway_environments_environment_id ON mod_ai_gateway_gateway_environments (environment_id);

CREATE TABLE mod_ai_gateway_environment_default_gateways (
    environment_id TEXT NOT NULL,
    role TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_environment_default_gateways PRIMARY KEY (environment_id, role),
    CONSTRAINT fk_mod_ai_gateway_environment_default_gateways_gateway_a08bd891 FOREIGN KEY (gateway_id, environment_id) REFERENCES mod_ai_gateway_gateway_environments (gateway_id, environment_id),
    CONSTRAINT ck_mod_ai_gateway_environment_default_gateways_1 CHECK (role IN ('ingress', 'egress'))
);
CREATE INDEX idx_mod_ai_gateway_environment_default_gateways_gateway_id ON mod_ai_gateway_environment_default_gateways (gateway_id);

CREATE TABLE mod_ai_gateway_gateway_tokens (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    CONSTRAINT pk_mod_ai_gateway_gateway_tokens PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateway_tokens_token_prefix UNIQUE (token_prefix),
    CONSTRAINT fk_mod_ai_gateway_gateway_tokens_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_gateway_tokens_1 CHECK (status IN ('active', 'revoked')),
    CONSTRAINT ck_mod_ai_gateway_gateway_tokens_2 CHECK (revoked_at IS NULL OR status = 'revoked')
);
CREATE INDEX idx_mod_ai_gateway_gateway_tokens_gateway_id_status ON mod_ai_gateway_gateway_tokens (gateway_id, status);

CREATE TABLE mod_ai_gateway_gateway_identity_providers (
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
    CONSTRAINT pk_mod_ai_gateway_gateway_identity_providers PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateway_identity_providers_gateway_id_name UNIQUE (gateway_id, name),
    CONSTRAINT fk_mod_ai_gateway_gateway_identity_providers_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_gateway_identity_providers_1 CHECK (jwks_skip_tls_verify IN (0, 1))
);

CREATE TABLE mod_ai_gateway_gateway_event_states (
    gateway_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    version_id TEXT,
    seq INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_event_states PRIMARY KEY (gateway_id),
    CONSTRAINT fk_mod_ai_gateway_gateway_event_states_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_gateway_events (
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
    CONSTRAINT pk_mod_ai_gateway_gateway_events PRIMARY KEY (id),
    CONSTRAINT fk_mod_ai_gateway_gateway_events_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_gateway_events_1 CHECK (json_valid(event_data))
);
CREATE INDEX idx_mod_ai_gateway_gateway_events_gateway_id_seq ON mod_ai_gateway_gateway_events (gateway_id, seq);
CREATE INDEX idx_mod_ai_gateway_gateway_events_gateway_id_processed_at ON mod_ai_gateway_gateway_events (gateway_id, processed_at);

CREATE TABLE mod_ai_gateway_artifacts (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    in_catalog INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_artifacts PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_name_version UNIQUE (org_id, name, version),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_artifacts_1 CHECK (in_catalog IN (0, 1)),
    CONSTRAINT ck_mod_ai_gateway_artifacts_2 CHECK (kind IN ('RestApi', 'WebSubAPI', 'LlmProvider', 'LlmProxy', 'Agent', 'McpProxy', 'McpProxyMapping'))
);

CREATE TABLE mod_ai_gateway_artifact_deployments (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    gateway_id TEXT NOT NULL,
    base_deployment_id TEXT,
    name TEXT NOT NULL,
    content BLOB NOT NULL,
    metadata TEXT,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_artifact_deployments PRIMARY KEY (id),
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_base_deployment_id FOREIGN KEY (base_deployment_id) REFERENCES mod_ai_gateway_artifact_deployments (id),
    CONSTRAINT ck_mod_ai_gateway_artifact_deployments_1 CHECK (metadata IS NULL OR json_valid(metadata))
);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_artifact_id_ga_7efb6b0f ON mod_ai_gateway_artifact_deployments (artifact_id, gateway_id, created_at);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_gateway_id ON mod_ai_gateway_artifact_deployments (gateway_id);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_base_deployment_id ON mod_ai_gateway_artifact_deployments (base_deployment_id);

CREATE TABLE mod_ai_gateway_artifact_gateway_status (
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
    CONSTRAINT pk_mod_ai_gateway_artifact_gateway_status PRIMARY KEY (artifact_id, gateway_id),
    CONSTRAINT fk_mod_ai_gateway_artifact_gateway_status_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_artifact_gateway_status_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_artifact_gateway_status_deployment_id FOREIGN KEY (deployment_id) REFERENCES mod_ai_gateway_artifact_deployments (id),
    CONSTRAINT ck_mod_ai_gateway_artifact_gateway_status_1 CHECK (status IN ('PENDING', 'DEPLOYING', 'DEPLOYED', 'UNDEPLOYING', 'UNDEPLOYED', 'FAILED')),
    CONSTRAINT ck_mod_ai_gateway_artifact_gateway_status_2 CHECK (deployment_id IS NOT NULL OR status IN ('PENDING', 'UNDEPLOYED')),
    CONSTRAINT ck_mod_ai_gateway_artifact_gateway_status_3 CHECK ((status_desired = 'DEPLOYED' AND status IN ('PENDING', 'DEPLOYING', 'DEPLOYED', 'FAILED')) OR (status_desired = 'UNDEPLOYED' AND status IN ('UNDEPLOYING', 'UNDEPLOYED', 'FAILED')))
);
CREATE INDEX idx_mod_ai_gateway_artifact_gateway_status_gateway_id ON mod_ai_gateway_artifact_gateway_status (gateway_id);
CREATE INDEX idx_mod_ai_gateway_artifact_gateway_status_deployment_id ON mod_ai_gateway_artifact_gateway_status (deployment_id);
CREATE INDEX idx_mod_ai_gateway_artifact_gateway_status_next_attempt_at ON mod_ai_gateway_artifact_gateway_status (next_attempt_at);
CREATE INDEX idx_mod_ai_gateway_artifact_gateway_status_status ON mod_ai_gateway_artifact_gateway_status (status);

CREATE TABLE mod_ai_gateway_llm_provider_templates (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    configuration TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_llm_provider_templates PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_provider_templates_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT ck_mod_ai_gateway_llm_provider_templates_1 CHECK (json_valid(configuration))
);

CREATE TABLE mod_ai_gateway_llm_providers (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    description TEXT,
    template_handle TEXT NOT NULL,
    openapi_spec TEXT,
    model_list TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    created_by TEXT,
    CONSTRAINT pk_mod_ai_gateway_llm_providers PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_providers_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_llm_providers_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_llm_providers_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mod_ai_gateway_llm_providers_template_handle ON mod_ai_gateway_llm_providers (template_handle);

CREATE TABLE mod_ai_gateway_llm_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    description TEXT,
    openapi_spec TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    created_by TEXT,
    CONSTRAINT pk_mod_ai_gateway_llm_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_llm_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_llm_proxies_org_id_provider_id FOREIGN KEY (org_id, provider_id) REFERENCES mod_ai_gateway_llm_providers (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_llm_proxies_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mod_ai_gateway_llm_proxies_org_id_project_id ON mod_ai_gateway_llm_proxies (org_id, project_id);
CREATE INDEX idx_mod_ai_gateway_llm_proxies_provider_id ON mod_ai_gateway_llm_proxies (provider_id);

CREATE TABLE mod_ai_gateway_mcp_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    project_id TEXT,
    description TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    created_by TEXT,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_mcp_proxies_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxies_org_id_project_id ON mod_ai_gateway_mcp_proxies (org_id, project_id);

CREATE TABLE mod_ai_gateway_mcp_proxy_mappings (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    source_mcp_proxy_id TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_mappings PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_mappings_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_mappings_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_mappings_org_id_source_mcp_proxy_id FOREIGN KEY (org_id, source_mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_mcp_proxy_mappings_1 CHECK (json_valid(configuration))
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxy_mappings_source_mcp_proxy_id ON mod_ai_gateway_mcp_proxy_mappings (source_mcp_proxy_id);

CREATE TABLE mod_ai_gateway_mcp_proxy_scopes (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    action TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_scopes PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_scopes_mcp_proxy_id_action UNIQUE (mcp_proxy_id, action),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_scopes_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_scopes_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_mcp_proxy_scope_tools (
    scope_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_scope_tools PRIMARY KEY (scope_id, tool_name),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_scope_tools_org_id_scope_id FOREIGN KEY (org_id, scope_id) REFERENCES mod_ai_gateway_mcp_proxy_scopes (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxy_scope_tools_org_id_tool_name ON mod_ai_gateway_mcp_proxy_scope_tools (org_id, tool_name);

CREATE TABLE mod_ai_gateway_mcp_proxy_endpoints (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT,
    status TEXT NOT NULL,
    configuration TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_endpoints PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoints_mcp_proxy_id_handle UNIQUE (mcp_proxy_id, handle),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoints_mcp_proxy_id_id UNIQUE (mcp_proxy_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoints_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_mcp_proxy_endpoints_1 CHECK (json_valid(configuration))
);

CREATE TABLE mod_ai_gateway_mcp_proxy_endpoint_environments (
    endpoint_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_endpoint_environments PRIMARY KEY (endpoint_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoint_environments_mcp__167b5681 UNIQUE (mcp_proxy_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoint_environments_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoint_environments_mcp__898cb1ee FOREIGN KEY (mcp_proxy_id, endpoint_id) REFERENCES mod_ai_gateway_mcp_proxy_endpoints (mcp_proxy_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoint_environments_org__850f3890 FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id)
);

CREATE TABLE mod_ai_gateway_agent_model_configs (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_configs PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_model_configs_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_mod_ai_gateway_agent_model_configs_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_model_configs_1 CHECK (type IN ('llm', 'mcp', 'agent'))
);

CREATE TABLE mod_ai_gateway_agent_model_config_llm (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    llm_proxy_id TEXT NOT NULL,
    policy_configuration TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_llm PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_llm_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_llm_org_id_llm_proxy_id FOREIGN KEY (org_id, llm_proxy_id) REFERENCES mod_ai_gateway_llm_proxies (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_model_config_llm_1 CHECK (json_valid(policy_configuration))
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_llm_llm_proxy_id ON mod_ai_gateway_agent_model_config_llm (llm_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_mcp (
    config_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mcp_proxy_id TEXT NOT NULL,
    scope_recorded_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_mcp PRIMARY KEY (config_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_mcp_proxy_id ON mod_ai_gateway_agent_model_config_mcp (mcp_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_mcp_envs (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    mapping_artifact_id TEXT,
    mcp_proxy_id TEXT,
    excluded INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_mcp_envs PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_m_72ed0dfb FOREIGN KEY (org_id, mapping_artifact_id) REFERENCES mod_ai_gateway_mcp_proxy_mappings (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_m_3792f700 FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_model_config_mcp_envs_1 CHECK (excluded IN (0, 1))
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_envs_mapping_96cf89a6 ON mod_ai_gateway_agent_model_config_mcp_envs (mapping_artifact_id);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_envs_mcp_proxy_id ON mod_ai_gateway_agent_model_config_mcp_envs (mcp_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_vars (
    config_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    variable_key TEXT NOT NULL,
    org_id TEXT NOT NULL,
    variable_name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_vars PRIMARY KEY (config_id, environment_id, variable_key),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_vars_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_ai_applications (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_ai_applications PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_ai_applications_agent_id_environment_id UNIQUE (agent_id, environment_id)
);

CREATE TABLE mod_ai_gateway_api_keys (
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
    CONSTRAINT pk_mod_ai_gateway_api_keys PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_api_keys_artifact_id_name UNIQUE (artifact_id, name),
    CONSTRAINT fk_mod_ai_gateway_api_keys_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_api_keys_key_hash ON mod_ai_gateway_api_keys (key_hash);

CREATE TABLE mod_ai_gateway_agent_exposures (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    protocol TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_exposures PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_exposures_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_agent_exposures_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_agent_exposures_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_exposures_1 CHECK (protocol IN ('a2a', 'rest'))
);

CREATE TABLE mod_ai_gateway_agent_cards (
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
    CONSTRAINT pk_mod_ai_gateway_agent_cards PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_cards_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT ck_mod_ai_gateway_agent_cards_1 CHECK (card IS NULL OR json_valid(card))
);
CREATE INDEX idx_mod_ai_gateway_agent_cards_status_next_attempt_at ON mod_ai_gateway_agent_cards (status, next_attempt_at);

CREATE TABLE mod_ai_gateway_a2a_proxies (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    upstream_auth_header TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_a2a_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_a2a_proxies_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_a2a_proxies_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_a2a_proxies_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id)
);

CREATE TABLE mod_ai_gateway_locks (
    name TEXT NOT NULL,
    owner TEXT,
    expires_at TIMESTAMP,
    CONSTRAINT pk_mod_ai_gateway_locks PRIMARY KEY (name)
);
INSERT INTO mod_ai_gateway_locks (name) VALUES ('drift_scan');
