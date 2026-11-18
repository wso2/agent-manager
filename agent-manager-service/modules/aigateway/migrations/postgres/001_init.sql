-- Code generated from modules/aigateway/schema/001_init.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_agent_gateway_settings (
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    api_key_security BOOLEAN NOT NULL,
    oauth_enabled BOOLEAN NOT NULL,
    oauth_issuers JSONB NOT NULL,
    oauth_audiences JSONB NOT NULL,
    oauth_header_name VARCHAR(255) NOT NULL,
    oauth_header_prefix VARCHAR(64) NOT NULL,
    oauth_forward_token BOOLEAN NOT NULL,
    cors_enabled BOOLEAN NOT NULL,
    cors_allow_origins JSONB NOT NULL,
    cors_allow_methods JSONB NOT NULL,
    cors_allow_headers JSONB NOT NULL,
    cors_allow_credentials BOOLEAN NOT NULL,
    timeout_seconds INTEGER,
    card_cors_enabled BOOLEAN,
    card_cors_allow_origins JSONB,
    card_cors_allow_headers JSONB,
    card_cors_allow_credentials BOOLEAN,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_gateway_settings PRIMARY KEY (agent_id, environment_id)
);

CREATE TABLE mod_ai_gateway_agent_bindings (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    kind VARCHAR(8) NOT NULL,
    position INTEGER NOT NULL,
    target VARCHAR(255) NOT NULL,
    request JSONB NOT NULL,
    state VARCHAR(16) NOT NULL,
    last_error VARCHAR(2000),
    config_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_bindings PRIMARY KEY (id),
    CONSTRAINT ck_mod_ai_gateway_agent_bindings_1 CHECK (kind IN ('llm', 'mcp')),
    CONSTRAINT ck_mod_ai_gateway_agent_bindings_2 CHECK (state IN ('pending', 'ready', 'failed', 'removing'))
);
CREATE INDEX idx_mod_ai_gateway_agent_bindings_org_id_agent_id ON mod_ai_gateway_agent_bindings (org_id, agent_id);

CREATE TABLE mod_ai_gateway_agent_teardowns (
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_name VARCHAR(255) NOT NULL,
    agent_name VARCHAR(255) NOT NULL,
    last_error VARCHAR(2000),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_teardowns PRIMARY KEY (agent_id),
    CONSTRAINT uq_mod_ai_gateway_agent_teardowns_org_id_project_name__49fd1520 UNIQUE (org_id, project_name, agent_name)
);

CREATE TABLE mod_ai_gateway_gateways (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    functionality_type VARCHAR(16) NOT NULL,
    hosting VARCHAR(16) NOT NULL,
    vhost VARCHAR(255) NOT NULL,
    runtime_url VARCHAR(2048),
    is_critical BOOLEAN NOT NULL,
    is_active BOOLEAN NOT NULL,
    properties JSONB NOT NULL,
    manifest JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateways PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateways_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_mod_ai_gateway_gateways_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_gateways_1 CHECK (functionality_type IN ('ingress', 'egress', 'both')),
    CONSTRAINT ck_mod_ai_gateway_gateways_2 CHECK (hosting IN ('managed', 'self_hosted')),
    CONSTRAINT ck_mod_ai_gateway_gateways_3 CHECK (hosting <> 'self_hosted' OR functionality_type = 'egress')
);

CREATE TABLE mod_ai_gateway_gateway_environments (
    gateway_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_environments PRIMARY KEY (gateway_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_gateway_environments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_gateway_environments_environment_id ON mod_ai_gateway_gateway_environments (environment_id);

CREATE TABLE mod_ai_gateway_environment_default_gateways (
    environment_id UUID NOT NULL,
    role VARCHAR(16) NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_environment_default_gateways PRIMARY KEY (environment_id, role),
    CONSTRAINT fk_mod_ai_gateway_environment_default_gateways_gateway_a08bd891 FOREIGN KEY (gateway_id, environment_id) REFERENCES mod_ai_gateway_gateway_environments (gateway_id, environment_id),
    CONSTRAINT ck_mod_ai_gateway_environment_default_gateways_1 CHECK (role IN ('ingress', 'egress'))
);
CREATE INDEX idx_mod_ai_gateway_environment_default_gateways_gateway_id ON mod_ai_gateway_environment_default_gateways (gateway_id);

CREATE TABLE mod_ai_gateway_gateway_tokens (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    salt VARCHAR(255) NOT NULL,
    token_prefix VARCHAR(36) NOT NULL,
    status VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT pk_mod_ai_gateway_gateway_tokens PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateway_tokens_token_prefix UNIQUE (token_prefix),
    CONSTRAINT fk_mod_ai_gateway_gateway_tokens_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_gateway_tokens_1 CHECK (status IN ('active', 'revoked')),
    CONSTRAINT ck_mod_ai_gateway_gateway_tokens_2 CHECK (revoked_at IS NULL OR status = 'revoked')
);
CREATE INDEX idx_mod_ai_gateway_gateway_tokens_gateway_id_status ON mod_ai_gateway_gateway_tokens (gateway_id, status);

CREATE TABLE mod_ai_gateway_gateway_identity_providers (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    issuer VARCHAR(2048),
    jwks_uri VARCHAR(2048),
    description VARCHAR(1000),
    type VARCHAR(32) NOT NULL,
    jwks_skip_tls_verify BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_identity_providers PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_gateway_identity_providers_gateway_id_name UNIQUE (gateway_id, name),
    CONSTRAINT fk_mod_ai_gateway_gateway_identity_providers_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_gateway_event_states (
    gateway_id UUID NOT NULL,
    org_id UUID NOT NULL,
    version_id VARCHAR(64),
    seq BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_event_states PRIMARY KEY (gateway_id),
    CONSTRAINT fk_mod_ai_gateway_gateway_event_states_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_gateway_events (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    seq BIGINT NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL,
    entity_id VARCHAR(255) NOT NULL,
    event_data JSONB NOT NULL,
    originated_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_gateway_events PRIMARY KEY (id),
    CONSTRAINT fk_mod_ai_gateway_gateway_events_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_gateway_events_gateway_id_seq ON mod_ai_gateway_gateway_events (gateway_id, seq);
CREATE INDEX idx_mod_ai_gateway_gateway_events_gateway_id_processed_at ON mod_ai_gateway_gateway_events (gateway_id, processed_at);

CREATE TABLE mod_ai_gateway_artifacts (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    kind VARCHAR(20) NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(30) NOT NULL,
    in_catalog BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_artifacts PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_name_version UNIQUE (org_id, name, version),
    CONSTRAINT uq_mod_ai_gateway_artifacts_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_artifacts_1 CHECK (kind IN ('RestApi', 'WebSubAPI', 'LlmProvider', 'LlmProxy', 'Agent', 'McpProxy', 'McpProxyMapping'))
);

CREATE TABLE mod_ai_gateway_artifact_deployments (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    base_deployment_id UUID,
    name VARCHAR(255) NOT NULL,
    content BYTEA NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_artifact_deployments PRIMARY KEY (id),
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_artifact_deployments_base_deployment_id FOREIGN KEY (base_deployment_id) REFERENCES mod_ai_gateway_artifact_deployments (id)
);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_artifact_id_ga_7efb6b0f ON mod_ai_gateway_artifact_deployments (artifact_id, gateway_id, created_at);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_gateway_id ON mod_ai_gateway_artifact_deployments (gateway_id);
CREATE INDEX idx_mod_ai_gateway_artifact_deployments_base_deployment_id ON mod_ai_gateway_artifact_deployments (base_deployment_id);

CREATE TABLE mod_ai_gateway_artifact_gateway_status (
    artifact_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    org_id UUID NOT NULL,
    deployment_id UUID,
    status VARCHAR(20) NOT NULL,
    status_desired VARCHAR(20) NOT NULL,
    status_reason TEXT,
    render_reason TEXT,
    performed_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
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
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(253) NOT NULL,
    description TEXT,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_llm_provider_templates PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_provider_templates_org_id_handle UNIQUE (org_id, handle)
);

CREATE TABLE mod_ai_gateway_llm_providers (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    description TEXT,
    template_handle VARCHAR(255) NOT NULL,
    openapi_spec TEXT,
    model_list TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    upstream_secret_id UUID,
    created_by VARCHAR(255),
    CONSTRAINT pk_mod_ai_gateway_llm_providers PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_providers_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_llm_providers_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_llm_providers_template_handle ON mod_ai_gateway_llm_providers (template_handle);
CREATE INDEX idx_mod_ai_gateway_llm_providers_upstream_secret_id ON mod_ai_gateway_llm_providers (upstream_secret_id);

CREATE TABLE mod_ai_gateway_llm_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    provider_id UUID NOT NULL,
    description TEXT,
    openapi_spec TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    upstream_secret_id UUID,
    created_by VARCHAR(255),
    CONSTRAINT pk_mod_ai_gateway_llm_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_llm_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_llm_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_llm_proxies_org_id_provider_id FOREIGN KEY (org_id, provider_id) REFERENCES mod_ai_gateway_llm_providers (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_llm_proxies_org_id_project_id ON mod_ai_gateway_llm_proxies (org_id, project_id);
CREATE INDEX idx_mod_ai_gateway_llm_proxies_provider_id ON mod_ai_gateway_llm_proxies (provider_id);
CREATE INDEX idx_mod_ai_gateway_llm_proxies_upstream_secret_id ON mod_ai_gateway_llm_proxies (upstream_secret_id);

CREATE TABLE mod_ai_gateway_mcp_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    CONSTRAINT pk_mod_ai_gateway_mcp_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxies_org_id_project_id ON mod_ai_gateway_mcp_proxies (org_id, project_id);

CREATE TABLE mod_ai_gateway_mcp_proxy_mappings (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    source_mcp_proxy_id UUID NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_mappings PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_mappings_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_mappings_org_id_id FOREIGN KEY (org_id, id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_mappings_org_id_source_mcp_proxy_id FOREIGN KEY (org_id, source_mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxy_mappings_source_mcp_proxy_id ON mod_ai_gateway_mcp_proxy_mappings (source_mcp_proxy_id);

CREATE TABLE mod_ai_gateway_mcp_proxy_scopes (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    action VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_scopes PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_scopes_mcp_proxy_id_action UNIQUE (mcp_proxy_id, action),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_scopes_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_scopes_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_mcp_proxy_scope_tools (
    scope_id UUID NOT NULL,
    tool_name VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_scope_tools PRIMARY KEY (scope_id, tool_name),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_scope_tools_org_id_scope_id FOREIGN KEY (org_id, scope_id) REFERENCES mod_ai_gateway_mcp_proxy_scopes (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxy_scope_tools_org_id_tool_name ON mod_ai_gateway_mcp_proxy_scope_tools (org_id, tool_name);

CREATE TABLE mod_ai_gateway_mcp_proxy_endpoints (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    upstream_secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_endpoints PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoints_mcp_proxy_id_handle UNIQUE (mcp_proxy_id, handle),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoints_mcp_proxy_id_id UNIQUE (mcp_proxy_id, id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoints_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_mcp_proxy_endpoints_upstream_secret_id ON mod_ai_gateway_mcp_proxy_endpoints (upstream_secret_id);

CREATE TABLE mod_ai_gateway_mcp_proxy_endpoint_environments (
    endpoint_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_mcp_proxy_endpoint_environments PRIMARY KEY (endpoint_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoint_environments_mcp__167b5681 UNIQUE (mcp_proxy_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_mcp_proxy_endpoint_environments_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoint_environments_mcp__898cb1ee FOREIGN KEY (mcp_proxy_id, endpoint_id) REFERENCES mod_ai_gateway_mcp_proxy_endpoints (mcp_proxy_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_mcp_proxy_endpoint_environments_org__850f3890 FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id)
);

CREATE TABLE mod_ai_gateway_agent_model_configs (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_configs PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_model_configs_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_mod_ai_gateway_agent_model_configs_org_id_id UNIQUE (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_model_configs_1 CHECK (type IN ('llm', 'mcp', 'agent'))
);

CREATE TABLE mod_ai_gateway_agent_model_config_llm (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    llm_proxy_id UUID NOT NULL,
    policy_configuration JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_llm PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_llm_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_llm_org_id_llm_proxy_id FOREIGN KEY (org_id, llm_proxy_id) REFERENCES mod_ai_gateway_llm_proxies (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_llm_llm_proxy_id ON mod_ai_gateway_agent_model_config_llm (llm_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_mcp (
    config_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    scope_recorded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_mcp PRIMARY KEY (config_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_mcp_proxy_id ON mod_ai_gateway_agent_model_config_mcp (mcp_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_mcp_envs (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mapping_artifact_id UUID,
    mcp_proxy_id UUID,
    excluded BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_mcp_envs PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_m_72ed0dfb FOREIGN KEY (org_id, mapping_artifact_id) REFERENCES mod_ai_gateway_mcp_proxy_mappings (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_mcp_envs_org_id_m_3792f700 FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mod_ai_gateway_mcp_proxies (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_envs_mapping_96cf89a6 ON mod_ai_gateway_agent_model_config_mcp_envs (mapping_artifact_id);
CREATE INDEX idx_mod_ai_gateway_agent_model_config_mcp_envs_mcp_proxy_id ON mod_ai_gateway_agent_model_config_mcp_envs (mcp_proxy_id);

CREATE TABLE mod_ai_gateway_agent_model_config_vars (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    variable_key VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    variable_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_model_config_vars PRIMARY KEY (config_id, environment_id, variable_key),
    CONSTRAINT fk_mod_ai_gateway_agent_model_config_vars_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES mod_ai_gateway_agent_model_configs (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_ai_applications (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_ai_applications PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_ai_applications_agent_id_environment_id UNIQUE (agent_id, environment_id)
);

CREATE TABLE mod_ai_gateway_api_keys (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    masked_key VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL,
    purpose INTEGER NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_api_keys PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_api_keys_artifact_id_name UNIQUE (artifact_id, name),
    CONSTRAINT fk_mod_ai_gateway_api_keys_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_api_keys_key_hash ON mod_ai_gateway_api_keys (key_hash);

CREATE TABLE mod_ai_gateway_agent_exposures (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    protocol VARCHAR(16) NOT NULL,
    artifact_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_exposures PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_exposures_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_agent_exposures_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_agent_exposures_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id),
    CONSTRAINT ck_mod_ai_gateway_agent_exposures_1 CHECK (protocol IN ('a2a', 'rest'))
);

CREATE TABLE mod_ai_gateway_agent_cards (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    source VARCHAR(32) NOT NULL,
    source_url VARCHAR(2048),
    card JSONB,
    card_hash VARCHAR(64),
    fetched_at TIMESTAMPTZ,
    status VARCHAR(32) NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_error TEXT,
    next_attempt_at TIMESTAMPTZ,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_agent_cards PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_agent_cards_agent_id_environment_id UNIQUE (agent_id, environment_id)
);
CREATE INDEX idx_mod_ai_gateway_agent_cards_status_next_attempt_at ON mod_ai_gateway_agent_cards (status, next_attempt_at);

CREATE TABLE mod_ai_gateway_a2a_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    upstream_auth_header VARCHAR(255),
    upstream_auth_secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_a2a_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_a2a_proxies_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_mod_ai_gateway_a2a_proxies_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mod_ai_gateway_a2a_proxies_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES mod_ai_gateway_artifacts (org_id, id)
);
CREATE INDEX idx_mod_ai_gateway_a2a_proxies_upstream_auth_secret_id ON mod_ai_gateway_a2a_proxies (upstream_auth_secret_id);
