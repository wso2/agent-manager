-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE organizations (
    id UUID NOT NULL,
    handle VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    thunder_ou_id VARCHAR(64),
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_organizations PRIMARY KEY (id),
    CONSTRAINT uq_organizations_handle UNIQUE (handle)
);
CREATE INDEX idx_organizations_thunder_ou_id ON organizations (thunder_ou_id);

CREATE TABLE projects (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_projects PRIMARY KEY (id),
    CONSTRAINT uq_projects_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_projects_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_projects_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);

CREATE TABLE environments (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    is_production BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_environments PRIMARY KEY (id),
    CONSTRAINT uq_environments_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_environments_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_environments_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);

CREATE TABLE agents (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    provisioning_type VARCHAR(16) NOT NULL,
    agent_type VARCHAR(32) NOT NULL,
    sub_type VARCHAR(32),
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agents PRIMARY KEY (id),
    CONSTRAINT uq_agents_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_agents_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agents_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT ck_agents_1 CHECK (provisioning_type IN ('external', 'internal'))
);
CREATE INDEX idx_agents_org_id_project_id ON agents (org_id, project_id);

CREATE TABLE agent_labels (
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    label_key VARCHAR(63) NOT NULL,
    label_value VARCHAR(63),
    CONSTRAINT pk_agent_labels PRIMARY KEY (agent_id, label_key),
    CONSTRAINT fk_agent_labels_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_agent_labels_org_id_label_key_label_value ON agent_labels (org_id, label_key, label_value);

CREATE TABLE agent_identities (
    id UUID NOT NULL,
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    thunder_agent_id VARCHAR(64),
    thunder_client_id VARCHAR(255),
    status VARCHAR(16) NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    requested_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_identities PRIMARY KEY (id),
    CONSTRAINT uq_agent_identities_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_agent_identities_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_identities_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_identities_1 CHECK (status IN ('pending', 'in_progress', 'completed', 'failed'))
);
CREATE INDEX idx_agent_identities_status_next_retry_at ON agent_identities (status, next_retry_at);

CREATE TABLE agent_gateway_settings (
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
    CONSTRAINT pk_agent_gateway_settings PRIMARY KEY (agent_id, environment_id),
    CONSTRAINT fk_agent_gateway_settings_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_gateway_settings_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE secrets (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    owner_kind VARCHAR(32) NOT NULL,
    owner_id UUID NOT NULL,
    provider VARCHAR(16) NOT NULL,
    ciphertext BYTEA,
    key_id VARCHAR(64),
    external_ref VARCHAR(512),
    version INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_secrets PRIMARY KEY (id),
    CONSTRAINT uq_secrets_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_secrets_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);
CREATE INDEX idx_secrets_org_id_owner_kind_owner_id ON secrets (org_id, owner_kind, owner_id);

CREATE TABLE gateways (
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
    CONSTRAINT pk_gateways PRIMARY KEY (id),
    CONSTRAINT uq_gateways_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_gateways_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_gateways_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_gateways_1 CHECK (functionality_type IN ('ingress', 'egress', 'both')),
    CONSTRAINT ck_gateways_2 CHECK (hosting IN ('managed', 'self_hosted')),
    CONSTRAINT ck_gateways_3 CHECK (hosting <> 'self_hosted' OR functionality_type = 'egress')
);

CREATE TABLE gateway_environments (
    gateway_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_gateway_environments PRIMARY KEY (gateway_id, environment_id),
    CONSTRAINT fk_gateway_environments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_gateway_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);
CREATE INDEX idx_gateway_environments_environment_id ON gateway_environments (environment_id);

CREATE TABLE environment_default_gateways (
    environment_id UUID NOT NULL,
    role VARCHAR(16) NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_environment_default_gateways PRIMARY KEY (environment_id, role),
    CONSTRAINT fk_environment_default_gateways_gateway_id_environment_id FOREIGN KEY (gateway_id, environment_id) REFERENCES gateway_environments (gateway_id, environment_id),
    CONSTRAINT fk_environment_default_gateways_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_environment_default_gateways_1 CHECK (role IN ('ingress', 'egress'))
);
CREATE INDEX idx_environment_default_gateways_gateway_id ON environment_default_gateways (gateway_id);

CREATE TABLE gateway_tokens (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    salt VARCHAR(255) NOT NULL,
    token_prefix VARCHAR(36) NOT NULL,
    status VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT pk_gateway_tokens PRIMARY KEY (id),
    CONSTRAINT uq_gateway_tokens_token_prefix UNIQUE (token_prefix),
    CONSTRAINT fk_gateway_tokens_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_gateway_tokens_1 CHECK (status IN ('active', 'revoked')),
    CONSTRAINT ck_gateway_tokens_2 CHECK (revoked_at IS NULL OR status = 'revoked')
);
CREATE INDEX idx_gateway_tokens_gateway_id_status ON gateway_tokens (gateway_id, status);

CREATE TABLE gateway_identity_providers (
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
    CONSTRAINT pk_gateway_identity_providers PRIMARY KEY (id),
    CONSTRAINT uq_gateway_identity_providers_gateway_id_name UNIQUE (gateway_id, name),
    CONSTRAINT fk_gateway_identity_providers_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE gateway_event_states (
    gateway_id UUID NOT NULL,
    org_id UUID NOT NULL,
    version_id VARCHAR(64),
    seq BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_gateway_event_states PRIMARY KEY (gateway_id),
    CONSTRAINT fk_gateway_event_states_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE
);

CREATE TABLE gateway_events (
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
    CONSTRAINT pk_gateway_events PRIMARY KEY (id),
    CONSTRAINT fk_gateway_events_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_gateway_events_gateway_id_seq ON gateway_events (gateway_id, seq);
CREATE INDEX idx_gateway_events_gateway_id_processed_at ON gateway_events (gateway_id, processed_at);

CREATE TABLE artifacts (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    kind VARCHAR(20) NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(30) NOT NULL,
    in_catalog BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_artifacts PRIMARY KEY (id),
    CONSTRAINT uq_artifacts_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_artifacts_org_id_name_version UNIQUE (org_id, name, version),
    CONSTRAINT uq_artifacts_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_artifacts_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_artifacts_1 CHECK (kind IN ('RestApi', 'WebSubAPI', 'LlmProvider', 'LlmProxy', 'Agent', 'McpProxy', 'McpProxyMapping'))
);

CREATE TABLE artifact_deployments (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    gateway_id UUID NOT NULL,
    base_deployment_id UUID,
    name VARCHAR(255) NOT NULL,
    content BYTEA NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_artifact_deployments PRIMARY KEY (id),
    CONSTRAINT fk_artifact_deployments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_artifact_deployments_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES gateways (org_id, id),
    CONSTRAINT fk_artifact_deployments_base_deployment_id FOREIGN KEY (base_deployment_id) REFERENCES artifact_deployments (id)
);
CREATE INDEX idx_artifact_deployments_artifact_id_gateway_id_created_at ON artifact_deployments (artifact_id, gateway_id, created_at);
CREATE INDEX idx_artifact_deployments_gateway_id ON artifact_deployments (gateway_id);
CREATE INDEX idx_artifact_deployments_base_deployment_id ON artifact_deployments (base_deployment_id);

CREATE TABLE artifact_gateway_status (
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
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(253) NOT NULL,
    description TEXT,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_llm_provider_templates PRIMARY KEY (id),
    CONSTRAINT uq_llm_provider_templates_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT fk_llm_provider_templates_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);

CREATE TABLE llm_providers (
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
    CONSTRAINT pk_llm_providers PRIMARY KEY (id),
    CONSTRAINT uq_llm_providers_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_llm_providers_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_llm_providers_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_llm_providers_template_handle ON llm_providers (template_handle);
CREATE INDEX idx_llm_providers_upstream_secret_id ON llm_providers (upstream_secret_id);

CREATE TABLE llm_proxies (
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
    CONSTRAINT pk_llm_proxies PRIMARY KEY (id),
    CONSTRAINT uq_llm_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_llm_proxies_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_provider_id FOREIGN KEY (org_id, provider_id) REFERENCES llm_providers (org_id, id),
    CONSTRAINT fk_llm_proxies_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_llm_proxies_org_id_project_id ON llm_proxies (org_id, project_id);
CREATE INDEX idx_llm_proxies_provider_id ON llm_proxies (provider_id);
CREATE INDEX idx_llm_proxies_upstream_secret_id ON llm_proxies (upstream_secret_id);

CREATE TABLE mcp_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    CONSTRAINT pk_mcp_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxies_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id)
);
CREATE INDEX idx_mcp_proxies_org_id_project_id ON mcp_proxies (org_id, project_id);

CREATE TABLE mcp_proxy_mappings (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    source_mcp_proxy_id UUID NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    CONSTRAINT pk_mcp_proxy_mappings PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_mappings_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxy_mappings_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_mappings_org_id_source_mcp_proxy_id FOREIGN KEY (org_id, source_mcp_proxy_id) REFERENCES mcp_proxies (org_id, id)
);
CREATE INDEX idx_mcp_proxy_mappings_source_mcp_proxy_id ON mcp_proxy_mappings (source_mcp_proxy_id);

CREATE TABLE mcp_proxy_scopes (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    action VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mcp_proxy_scopes PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_scopes_mcp_proxy_id_action UNIQUE (mcp_proxy_id, action),
    CONSTRAINT uq_mcp_proxy_scopes_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxy_scopes_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mcp_proxy_scope_tools (
    scope_id UUID NOT NULL,
    tool_name VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_mcp_proxy_scope_tools PRIMARY KEY (scope_id, tool_name),
    CONSTRAINT fk_mcp_proxy_scope_tools_org_id_scope_id FOREIGN KEY (org_id, scope_id) REFERENCES mcp_proxy_scopes (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mcp_proxy_scope_tools_org_id_tool_name ON mcp_proxy_scope_tools (org_id, tool_name);

CREATE TABLE mcp_proxy_endpoints (
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
    CONSTRAINT pk_mcp_proxy_endpoints PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxy_endpoints_mcp_proxy_id_handle UNIQUE (mcp_proxy_id, handle),
    CONSTRAINT uq_mcp_proxy_endpoints_mcp_proxy_id_id UNIQUE (mcp_proxy_id, id),
    CONSTRAINT fk_mcp_proxy_endpoints_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_endpoints_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_mcp_proxy_endpoints_upstream_secret_id ON mcp_proxy_endpoints (upstream_secret_id);

CREATE TABLE mcp_proxy_endpoint_environments (
    endpoint_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mcp_proxy_endpoint_environments PRIMARY KEY (endpoint_id, environment_id),
    CONSTRAINT uq_mcp_proxy_endpoint_environments_mcp_proxy_id_environment_id UNIQUE (mcp_proxy_id, environment_id),
    CONSTRAINT uq_mcp_proxy_endpoint_environments_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_mcp_proxy_endpoint_environments_mcp_proxy_id_endpoint_id FOREIGN KEY (mcp_proxy_id, endpoint_id) REFERENCES mcp_proxy_endpoints (mcp_proxy_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxy_endpoint_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_mcp_proxy_endpoint_environments_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id)
);

CREATE TABLE agent_model_configs (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_model_configs PRIMARY KEY (id),
    CONSTRAINT uq_agent_model_configs_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_agent_model_configs_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_model_configs_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_agent_model_configs_1 CHECK (type IN ('llm', 'mcp', 'agent'))
);

CREATE TABLE agent_model_config_llm (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    llm_proxy_id UUID NOT NULL,
    policy_configuration JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_model_config_llm PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_agent_model_config_llm_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_llm_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_model_config_llm_org_id_llm_proxy_id FOREIGN KEY (org_id, llm_proxy_id) REFERENCES llm_proxies (org_id, id)
);
CREATE INDEX idx_agent_model_config_llm_llm_proxy_id ON agent_model_config_llm (llm_proxy_id);

CREATE TABLE agent_model_config_mcp (
    config_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mcp_proxy_id UUID NOT NULL,
    scope_recorded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_model_config_mcp PRIMARY KEY (config_id),
    CONSTRAINT fk_agent_model_config_mcp_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_mcp_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id)
);
CREATE INDEX idx_agent_model_config_mcp_mcp_proxy_id ON agent_model_config_mcp (mcp_proxy_id);

CREATE TABLE agent_model_config_mcp_envs (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    mapping_artifact_id UUID,
    mcp_proxy_id UUID,
    excluded BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_model_config_mcp_envs PRIMARY KEY (config_id, environment_id),
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_mapping_artifact_id FOREIGN KEY (org_id, mapping_artifact_id) REFERENCES mcp_proxy_mappings (org_id, id),
    CONSTRAINT fk_agent_model_config_mcp_envs_org_id_mcp_proxy_id FOREIGN KEY (org_id, mcp_proxy_id) REFERENCES mcp_proxies (org_id, id)
);
CREATE INDEX idx_agent_model_config_mcp_envs_mapping_artifact_id ON agent_model_config_mcp_envs (mapping_artifact_id);
CREATE INDEX idx_agent_model_config_mcp_envs_mcp_proxy_id ON agent_model_config_mcp_envs (mcp_proxy_id);

CREATE TABLE agent_model_config_vars (
    config_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    variable_key VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    variable_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_model_config_vars PRIMARY KEY (config_id, environment_id, variable_key),
    CONSTRAINT fk_agent_model_config_vars_org_id_config_id FOREIGN KEY (org_id, config_id) REFERENCES agent_model_configs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_model_config_vars_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE ai_applications (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    handle VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_ai_applications PRIMARY KEY (id),
    CONSTRAINT uq_ai_applications_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_ai_applications_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_ai_applications_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE api_keys (
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
    CONSTRAINT pk_api_keys PRIMARY KEY (id),
    CONSTRAINT uq_api_keys_artifact_id_name UNIQUE (artifact_id, name),
    CONSTRAINT fk_api_keys_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_api_keys_key_hash ON api_keys (key_hash);

CREATE TABLE agent_exposures (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    protocol VARCHAR(16) NOT NULL,
    artifact_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_exposures PRIMARY KEY (id),
    CONSTRAINT uq_agent_exposures_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_agent_exposures_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_agent_exposures_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_exposures_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_exposures_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id),
    CONSTRAINT ck_agent_exposures_1 CHECK (protocol IN ('a2a', 'rest'))
);

CREATE TABLE agent_cards (
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
    CONSTRAINT pk_agent_cards PRIMARY KEY (id),
    CONSTRAINT uq_agent_cards_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_agent_cards_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_cards_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);
CREATE INDEX idx_agent_cards_status_next_attempt_at ON agent_cards (status, next_attempt_at);

CREATE TABLE a2a_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    upstream_auth_header VARCHAR(255),
    upstream_auth_secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_a2a_proxies PRIMARY KEY (id),
    CONSTRAINT uq_a2a_proxies_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_a2a_proxies_artifact_id UNIQUE (artifact_id),
    CONSTRAINT fk_a2a_proxies_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_a2a_proxies_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_a2a_proxies_org_id_upstream_auth_secret_id FOREIGN KEY (org_id, upstream_auth_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT fk_a2a_proxies_org_id_artifact_id FOREIGN KEY (org_id, artifact_id) REFERENCES artifacts (org_id, id)
);
CREATE INDEX idx_a2a_proxies_upstream_auth_secret_id ON a2a_proxies (upstream_auth_secret_id);

CREATE TABLE event_destinations (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    kind VARCHAR(32) NOT NULL,
    config JSONB NOT NULL,
    secret_id UUID,
    availability VARCHAR(16) NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_event_destinations PRIMARY KEY (id),
    CONSTRAINT uq_event_destinations_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_event_destinations_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_event_destinations_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_event_destinations_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_event_destinations_1 CHECK (availability IN ('all', 'org', 'projects'))
);
CREATE INDEX idx_event_destinations_secret_id ON event_destinations (secret_id);

CREATE TABLE event_destination_projects (
    destination_id UUID NOT NULL,
    project_id UUID NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_event_destination_projects PRIMARY KEY (destination_id, project_id),
    CONSTRAINT fk_event_destination_projects_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_destination_projects_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id)
);
CREATE INDEX idx_event_destination_projects_org_id_project_id ON event_destination_projects (org_id, project_id);

CREATE TABLE event_subscriptions (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    scope VARCHAR(16) NOT NULL,
    project_id UUID,
    agent_id UUID,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    destination_id UUID NOT NULL,
    enabled BOOLEAN NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_event_subscriptions PRIMARY KEY (id),
    CONSTRAINT uq_event_subscriptions_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_event_subscriptions_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_subscriptions_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_event_subscriptions_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id),
    CONSTRAINT ck_event_subscriptions_1 CHECK (scope IN ('org', 'project', 'agent')),
    CONSTRAINT ck_event_subscriptions_2 CHECK ((scope = 'org' AND project_id IS NULL AND agent_id IS NULL) OR (scope = 'project' AND project_id IS NOT NULL AND agent_id IS NULL) OR (scope = 'agent' AND project_id IS NOT NULL AND agent_id IS NOT NULL))
);
CREATE INDEX idx_event_subscriptions_org_id_scope_project_id_agent_id ON event_subscriptions (org_id, scope, project_id, agent_id);
CREATE INDEX idx_event_subscriptions_destination_id ON event_subscriptions (destination_id);

CREATE TABLE event_subscription_types (
    subscription_id UUID NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_event_subscription_types PRIMARY KEY (subscription_id, event_type),
    CONSTRAINT fk_event_subscription_types_org_id_subscription_id FOREIGN KEY (org_id, subscription_id) REFERENCES event_subscriptions (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_event_subscription_types_event_type ON event_subscription_types (event_type);

CREATE TABLE event_subscription_environments (
    subscription_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_event_subscription_environments PRIMARY KEY (subscription_id, environment_id),
    CONSTRAINT fk_event_subscription_environments_org_id_subscription_id FOREIGN KEY (org_id, subscription_id) REFERENCES event_subscriptions (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_event_subscription_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id)
);

CREATE TABLE event_outbox (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    body JSONB NOT NULL,
    available_at TIMESTAMPTZ NOT NULL,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    CONSTRAINT pk_event_outbox PRIMARY KEY (id),
    CONSTRAINT fk_event_outbox_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
);
CREATE INDEX idx_event_outbox_available_at_id ON event_outbox (available_at, id);

CREATE TABLE event_deliveries (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    destination_id UUID NOT NULL,
    event_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    event_scope VARCHAR(16),
    project_id UUID,
    agent_id UUID,
    environment_id UUID,
    status VARCHAR(16) NOT NULL,
    attempts INTEGER NOT NULL,
    response_code INTEGER,
    last_error TEXT,
    delivered_at TIMESTAMPTZ,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_event_deliveries PRIMARY KEY (id),
    CONSTRAINT uq_event_deliveries_destination_id_event_id UNIQUE (destination_id, event_id),
    CONSTRAINT fk_event_deliveries_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_event_deliveries_org_id_created_at ON event_deliveries (org_id, created_at);
CREATE INDEX idx_event_deliveries_destination_id_created_at ON event_deliveries (destination_id, created_at);
CREATE INDEX idx_event_deliveries_status_claimed_until ON event_deliveries (status, claimed_until);

CREATE TABLE event_delivery_subscriptions (
    delivery_id UUID NOT NULL,
    subscription_id UUID NOT NULL,
    CONSTRAINT pk_event_delivery_subscriptions PRIMARY KEY (delivery_id, subscription_id),
    CONSTRAINT fk_event_delivery_subscriptions_delivery_id FOREIGN KEY (delivery_id) REFERENCES event_deliveries (id) ON DELETE CASCADE
);
CREATE INDEX idx_event_delivery_subscriptions_subscription_id ON event_delivery_subscriptions (subscription_id);

CREATE TABLE audit_events (
    id UUID NOT NULL,
    org_id UUID,
    occurred_at TIMESTAMPTZ NOT NULL,
    schema_version INTEGER NOT NULL,
    action VARCHAR(128) NOT NULL,
    action_class VARCHAR(32) NOT NULL,
    severity INTEGER NOT NULL,
    outcome VARCHAR(16) NOT NULL,
    actor_type VARCHAR(16) NOT NULL,
    actor_id VARCHAR(255),
    actor_display VARCHAR(255),
    actor_token_id VARCHAR(255),
    on_behalf_of VARCHAR(255),
    auth_method VARCHAR(32),
    surface VARCHAR(16) NOT NULL,
    source_ip VARCHAR(45),
    user_agent VARCHAR(512),
    correlation_id VARCHAR(64),
    request_method VARCHAR(8),
    request_path TEXT,
    resource_type VARCHAR(64),
    resource_id VARCHAR(255),
    resource_name VARCHAR(255),
    project_name VARCHAR(63),
    environment_name VARCHAR(63),
    status_code INTEGER,
    error_code VARCHAR(64),
    error_message TEXT,
    required_permission VARCHAR(128),
    duration_ms INTEGER,
    details JSONB,
    CONSTRAINT pk_audit_events PRIMARY KEY (id),
    CONSTRAINT ck_audit_events_1 CHECK (outcome IN ('success', 'failure', 'deny', 'unknown'))
);
CREATE INDEX idx_audit_events_org_id_occurred_at_id ON audit_events (org_id, occurred_at, id);
CREATE INDEX idx_audit_events_org_id_actor_id_occurred_at ON audit_events (org_id, actor_id, occurred_at);
CREATE INDEX idx_audit_events_org_id_resource_type_resource_id_occurred_at ON audit_events (org_id, resource_type, resource_id, occurred_at);
CREATE INDEX idx_audit_events_org_id_action_occurred_at ON audit_events (org_id, action, occurred_at);

CREATE TABLE usage_events (
    id UUID NOT NULL,
    org_id UUID,
    occurred_at TIMESTAMPTZ NOT NULL,
    source VARCHAR(16) NOT NULL,
    feature_code VARCHAR(128) NOT NULL,
    username VARCHAR(255),
    session_id VARCHAR(64),
    status_code INTEGER,
    product_version VARCHAR(32) NOT NULL,
    dimensions JSONB,
    CONSTRAINT pk_usage_events PRIMARY KEY (id)
);
CREATE INDEX idx_usage_events_org_id_occurred_at_id ON usage_events (org_id, occurred_at, id);
CREATE INDEX idx_usage_events_feature_code_occurred_at ON usage_events (feature_code, occurred_at);

CREATE TABLE locks (
    name VARCHAR(128) NOT NULL,
    owner VARCHAR(255),
    expires_at TIMESTAMPTZ,
    CONSTRAINT pk_locks PRIMARY KEY (name)
);
INSERT INTO locks (name) VALUES ('agent_identity.reconciler');
INSERT INTO locks (name) VALUES ('gateway.drift_scan');
