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

CREATE TABLE deployment_pipelines (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_deployment_pipelines PRIMARY KEY (id),
    CONSTRAINT uq_deployment_pipelines_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_deployment_pipelines_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_deployment_pipelines_org_id FOREIGN KEY (org_id) REFERENCES organizations (id)
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
    CONSTRAINT ck_artifacts_1 CHECK (kind IN ('RestApi', 'WebSubAPI', 'LlmProvider', 'LlmProxy', 'Agent', 'McpProxy', 'McpProxyMapping', 'OtelIngest'))
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
    monitor_alert_id UUID,
    source_event_id VARCHAR(128),
    payload JSONB,
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
CREATE INDEX idx_event_deliveries_source_event_id ON event_deliveries (source_event_id);

CREATE TABLE event_delivery_subscriptions (
    delivery_id UUID NOT NULL,
    subscription_id UUID NOT NULL,
    CONSTRAINT pk_event_delivery_subscriptions PRIMARY KEY (delivery_id, subscription_id),
    CONSTRAINT fk_event_delivery_subscriptions_delivery_id FOREIGN KEY (delivery_id) REFERENCES event_deliveries (id) ON DELETE CASCADE
);
CREATE INDEX idx_event_delivery_subscriptions_subscription_id ON event_delivery_subscriptions (subscription_id);

CREATE TABLE evaluators (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    identifier VARCHAR(128) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    description VARCHAR(512),
    version VARCHAR(20) NOT NULL,
    type VARCHAR(16) NOT NULL,
    level VARCHAR(16) NOT NULL,
    source TEXT NOT NULL,
    config_schema JSONB NOT NULL,
    tags JSONB NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_evaluators PRIMARY KEY (id),
    CONSTRAINT uq_evaluators_org_id_identifier UNIQUE (org_id, identifier),
    CONSTRAINT uq_evaluators_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_evaluators_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT ck_evaluators_1 CHECK (type IN ('code', 'llm_judge')),
    CONSTRAINT ck_evaluators_2 CHECK (level IN ('trace', 'agent', 'llm'))
);

CREATE TABLE org_service_clients (
    org_id UUID NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_org_service_clients PRIMARY KEY (org_id, purpose),
    CONSTRAINT fk_org_service_clients_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_org_service_clients_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_org_service_clients_1 CHECK (purpose IN ('eval_publisher', 'eval_scheduler'))
);
CREATE INDEX idx_org_service_clients_secret_id ON org_service_clients (secret_id);

CREATE TABLE runtimes (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    kind VARCHAR(16) NOT NULL,
    endpoint VARCHAR(2048),
    config JSONB NOT NULL,
    credential_secret_id UUID,
    is_active BOOLEAN NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_runtimes PRIMARY KEY (id),
    CONSTRAINT uq_runtimes_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_runtimes_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_runtimes_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_runtimes_org_id_credential_secret_id FOREIGN KEY (org_id, credential_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_runtimes_1 CHECK (kind IN ('kubernetes', 'docker'))
);
CREATE INDEX idx_runtimes_credential_secret_id ON runtimes (credential_secret_id);

CREATE TABLE git_credentials (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    auth_type VARCHAR(16) NOT NULL,
    username VARCHAR(255),
    secret_id UUID NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_git_credentials PRIMARY KEY (id),
    CONSTRAINT uq_git_credentials_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_git_credentials_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_git_credentials_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_git_credentials_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_git_credentials_1 CHECK (auth_type IN ('basic', 'ssh'))
);
CREATE INDEX idx_git_credentials_secret_id ON git_credentials (secret_id);

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
INSERT INTO locks (name) VALUES ('monitor.scheduler');
INSERT INTO locks (name) VALUES ('monitor.alerts');
INSERT INTO locks (name) VALUES ('build.dispatcher');
INSERT INTO locks (name) VALUES ('deployment.reconciler');

CREATE TABLE projects (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    deployment_pipeline_id UUID,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_projects PRIMARY KEY (id),
    CONSTRAINT uq_projects_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_projects_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_projects_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_projects_org_id_deployment_pipeline_id FOREIGN KEY (org_id, deployment_pipeline_id) REFERENCES deployment_pipelines (org_id, id)
);
CREATE INDEX idx_projects_deployment_pipeline_id ON projects (deployment_pipeline_id);

CREATE TABLE environments (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    is_production BOOLEAN NOT NULL,
    runtime_id UUID,
    isolation_tier VARCHAR(16),
    dns_prefix VARCHAR(63),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_environments PRIMARY KEY (id),
    CONSTRAINT uq_environments_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_environments_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_environments_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_environments_org_id_runtime_id FOREIGN KEY (org_id, runtime_id) REFERENCES runtimes (org_id, id),
    CONSTRAINT ck_environments_1 CHECK (isolation_tier IN ('gvisor', 'kata'))
);
CREATE INDEX idx_environments_runtime_id ON environments (runtime_id);

CREATE TABLE deployment_pipeline_environments (
    pipeline_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    position INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_deployment_pipeline_environments PRIMARY KEY (pipeline_id, environment_id),
    CONSTRAINT uq_deployment_pipeline_environments_pipeline_id_position UNIQUE (pipeline_id, position),
    CONSTRAINT fk_deployment_pipeline_environments_org_id_pipeline_id FOREIGN KEY (org_id, pipeline_id) REFERENCES deployment_pipelines (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_deployment_pipeline_environments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_deployment_pipeline_environments_1 CHECK (position >= 0)
);
CREATE INDEX idx_deployment_pipeline_environments_org_id_environment_id ON deployment_pipeline_environments (org_id, environment_id);

CREATE TABLE deployment_pipeline_paths (
    pipeline_id UUID NOT NULL,
    source_environment_id UUID NOT NULL,
    target_environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_deployment_pipeline_paths PRIMARY KEY (pipeline_id, source_environment_id, target_environment_id),
    CONSTRAINT fk_deployment_pipeline_paths_org_id_pipeline_id FOREIGN KEY (org_id, pipeline_id) REFERENCES deployment_pipelines (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_deployment_pipeline_paths_org_id_source_environment_id FOREIGN KEY (org_id, source_environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_deployment_pipeline_paths_org_id_target_environment_id FOREIGN KEY (org_id, target_environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_deployment_pipeline_paths_1 CHECK (source_environment_id <> target_environment_id)
);
CREATE INDEX idx_deployment_pipeline_paths_org_id_target_environment_id ON deployment_pipeline_paths (org_id, target_environment_id);

CREATE TABLE agents (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(1000),
    provisioning_type VARCHAR(16) NOT NULL,
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

CREATE TABLE rest_apis (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    description TEXT,
    lifecycle_status VARCHAR(20) NOT NULL,
    transport JSONB NOT NULL,
    configuration JSONB NOT NULL,
    upstream_secret_id UUID,
    created_by VARCHAR(255),
    CONSTRAINT pk_rest_apis PRIMARY KEY (id),
    CONSTRAINT uq_rest_apis_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_rest_apis_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_rest_apis_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_rest_apis_org_id_upstream_secret_id FOREIGN KEY (org_id, upstream_secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_rest_apis_org_id_project_id ON rest_apis (org_id, project_id);
CREATE INDEX idx_rest_apis_upstream_secret_id ON rest_apis (upstream_secret_id);

CREATE TABLE mcp_proxies (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID,
    source_agent_id UUID,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    CONSTRAINT pk_mcp_proxies PRIMARY KEY (id),
    CONSTRAINT uq_mcp_proxies_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mcp_proxies_org_id_id FOREIGN KEY (org_id, id) REFERENCES artifacts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_mcp_proxies_org_id_project_id FOREIGN KEY (org_id, project_id) REFERENCES projects (org_id, id),
    CONSTRAINT fk_mcp_proxies_org_id_source_agent_id FOREIGN KEY (org_id, source_agent_id) REFERENCES agents (org_id, id),
    CONSTRAINT ck_mcp_proxies_1 CHECK (source_agent_id IS NULL OR project_id IS NOT NULL)
);
CREATE INDEX idx_mcp_proxies_org_id_project_id ON mcp_proxies (org_id, project_id);
CREATE INDEX idx_mcp_proxies_source_agent_id ON mcp_proxies (source_agent_id);

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

CREATE TABLE monitors (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    description VARCHAR(512),
    type VARCHAR(16) NOT NULL,
    enabled BOOLEAN NOT NULL,
    interval_minutes INTEGER,
    next_run_at TIMESTAMPTZ,
    trace_start TIMESTAMPTZ,
    trace_end TIMESTAMPTZ,
    sampling_percent INTEGER NOT NULL,
    judge_llm_provider_id UUID,
    judge_llm_proxy_id UUID,
    judge_api_key_secret_id UUID,
    judge_policy_configuration JSONB,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_monitors PRIMARY KEY (id),
    CONSTRAINT uq_monitors_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_monitors_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_monitors_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_monitors_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_monitors_org_id_judge_llm_provider_id FOREIGN KEY (org_id, judge_llm_provider_id) REFERENCES llm_providers (org_id, id),
    CONSTRAINT fk_monitors_org_id_judge_llm_proxy_id FOREIGN KEY (org_id, judge_llm_proxy_id) REFERENCES llm_proxies (org_id, id),
    CONSTRAINT fk_monitors_org_id_judge_api_key_secret_id FOREIGN KEY (org_id, judge_api_key_secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_monitors_1 CHECK (type IN ('future', 'past')),
    CONSTRAINT ck_monitors_2 CHECK ((type = 'future' AND interval_minutes IS NOT NULL) OR (type = 'past' AND trace_start IS NOT NULL AND trace_end IS NOT NULL)),
    CONSTRAINT ck_monitors_3 CHECK (sampling_percent BETWEEN 1 AND 100)
);
CREATE INDEX idx_monitors_enabled_next_run_at ON monitors (enabled, next_run_at);
CREATE INDEX idx_monitors_judge_llm_provider_id ON monitors (judge_llm_provider_id);
CREATE INDEX idx_monitors_judge_llm_proxy_id ON monitors (judge_llm_proxy_id);
CREATE INDEX idx_monitors_judge_api_key_secret_id ON monitors (judge_api_key_secret_id);

CREATE TABLE monitor_evaluators (
    monitor_id UUID NOT NULL,
    evaluator_identifier VARCHAR(128) NOT NULL,
    org_id UUID NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    position INTEGER NOT NULL,
    config JSONB,
    CONSTRAINT pk_monitor_evaluators PRIMARY KEY (monitor_id, evaluator_identifier),
    CONSTRAINT fk_monitor_evaluators_org_id_monitor_id FOREIGN KEY (org_id, monitor_id) REFERENCES monitors (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_monitor_evaluators_org_id_evaluator_identifier ON monitor_evaluators (org_id, evaluator_identifier);

CREATE TABLE monitor_runs (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    monitor_id UUID NOT NULL,
    evaluators JSONB NOT NULL,
    judge_config JSONB,
    trace_start TIMESTAMPTZ NOT NULL,
    trace_end TIMESTAMPTZ NOT NULL,
    status VARCHAR(16) NOT NULL,
    job_ref VARCHAR(255),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    event_queued_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_monitor_runs PRIMARY KEY (id),
    CONSTRAINT uq_monitor_runs_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_monitor_runs_org_id_monitor_id FOREIGN KEY (org_id, monitor_id) REFERENCES monitors (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_monitor_runs_1 CHECK (status IN ('pending', 'running', 'success', 'failed'))
);
CREATE INDEX idx_monitor_runs_monitor_id_created_at ON monitor_runs (monitor_id, created_at);
CREATE INDEX idx_monitor_runs_monitor_id_trace_start_trace_end ON monitor_runs (monitor_id, trace_start, trace_end);
CREATE INDEX idx_monitor_runs_status_claimed_until ON monitor_runs (status, claimed_until);

CREATE TABLE monitor_run_evaluators (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    run_id UUID NOT NULL,
    monitor_id UUID NOT NULL,
    evaluator_identifier VARCHAR(128) NOT NULL,
    evaluator_name VARCHAR(255) NOT NULL,
    level VARCHAR(16) NOT NULL,
    aggregations JSONB NOT NULL,
    item_count INTEGER NOT NULL,
    skipped_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_monitor_run_evaluators PRIMARY KEY (id),
    CONSTRAINT uq_monitor_run_evaluators_run_id_evaluator_name UNIQUE (run_id, evaluator_name),
    CONSTRAINT uq_monitor_run_evaluators_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_monitor_run_evaluators_org_id_run_id FOREIGN KEY (org_id, run_id) REFERENCES monitor_runs (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_monitor_run_evaluators_1 CHECK (level IN ('trace', 'agent', 'llm'))
);
CREATE INDEX idx_monitor_run_evaluators_monitor_id_evaluator_name ON monitor_run_evaluators (monitor_id, evaluator_name);

CREATE TABLE scores (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    run_evaluator_id UUID NOT NULL,
    monitor_id UUID NOT NULL,
    item_key VARCHAR(512) NOT NULL,
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255),
    span_label VARCHAR(255),
    score_bp INTEGER,
    explanation TEXT,
    skip_reason TEXT,
    trace_start_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_scores PRIMARY KEY (id),
    CONSTRAINT uq_scores_run_evaluator_id_item_key UNIQUE (run_evaluator_id, item_key),
    CONSTRAINT fk_scores_org_id_run_evaluator_id FOREIGN KEY (org_id, run_evaluator_id) REFERENCES monitor_run_evaluators (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_scores_1 CHECK (score_bp BETWEEN 0 AND 10000)
);
CREATE INDEX idx_scores_monitor_id_trace_start_time ON scores (monitor_id, trace_start_time);
CREATE INDEX idx_scores_trace_id_span_id ON scores (trace_id, span_id);
CREATE INDEX idx_scores_monitor_id_span_label ON scores (monitor_id, span_label);

CREATE TABLE monitor_alerts (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    monitor_id UUID NOT NULL,
    notify_on_failure BOOLEAN NOT NULL,
    notify_on_success BOOLEAN NOT NULL,
    thresholds JSONB NOT NULL,
    breached JSONB NOT NULL,
    last_evaluated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_monitor_alerts PRIMARY KEY (id),
    CONSTRAINT uq_monitor_alerts_monitor_id UNIQUE (monitor_id),
    CONSTRAINT uq_monitor_alerts_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_monitor_alerts_org_id_monitor_id FOREIGN KEY (org_id, monitor_id) REFERENCES monitors (org_id, id) ON DELETE CASCADE
);

CREATE TABLE monitor_alert_destinations (
    alert_id UUID NOT NULL,
    destination_id UUID NOT NULL,
    org_id UUID NOT NULL,
    CONSTRAINT pk_monitor_alert_destinations PRIMARY KEY (alert_id, destination_id),
    CONSTRAINT fk_monitor_alert_destinations_org_id_alert_id FOREIGN KEY (org_id, alert_id) REFERENCES monitor_alerts (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_monitor_alert_destinations_org_id_destination_id FOREIGN KEY (org_id, destination_id) REFERENCES event_destinations (org_id, id)
);
CREATE INDEX idx_monitor_alert_destinations_org_id_destination_id ON monitor_alert_destinations (org_id, destination_id);

CREATE TABLE agent_kinds (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    source_agent_id UUID,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_kinds PRIMARY KEY (id),
    CONSTRAINT uq_agent_kinds_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_agent_kinds_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_kinds_org_id FOREIGN KEY (org_id) REFERENCES organizations (id),
    CONSTRAINT fk_agent_kinds_org_id_source_agent_id FOREIGN KEY (org_id, source_agent_id) REFERENCES agents (org_id, id)
);
CREATE INDEX idx_agent_kinds_source_agent_id ON agent_kinds (source_agent_id);

CREATE TABLE agent_kind_labels (
    kind_id UUID NOT NULL,
    label_key VARCHAR(63) NOT NULL,
    org_id UUID NOT NULL,
    label_value VARCHAR(63),
    CONSTRAINT pk_agent_kind_labels PRIMARY KEY (kind_id, label_key),
    CONSTRAINT fk_agent_kind_labels_org_id_kind_id FOREIGN KEY (org_id, kind_id) REFERENCES agent_kinds (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_agent_kind_labels_org_id_label_key_label_value ON agent_kind_labels (org_id, label_key, label_value);

CREATE TABLE builds (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL,
    parameters JSONB NOT NULL,
    commit_id VARCHAR(64),
    image_ref VARCHAR(1024),
    interface JSONB,
    health_checks JSONB,
    steps JSONB,
    job_ref VARCHAR(255),
    attempt_count INTEGER NOT NULL,
    last_error TEXT,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    requested_by VARCHAR(255),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_builds PRIMARY KEY (id),
    CONSTRAINT uq_builds_agent_id_name UNIQUE (agent_id, name),
    CONSTRAINT uq_builds_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_builds_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_builds_1 CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT ck_builds_2 CHECK (status <> 'succeeded' OR image_ref IS NOT NULL)
);
CREATE INDEX idx_builds_agent_id_created_at ON builds (agent_id, created_at);
CREATE INDEX idx_builds_status_claimed_until ON builds (status, claimed_until);

CREATE TABLE agent_kind_versions (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    kind_id UUID NOT NULL,
    version VARCHAR(30) NOT NULL,
    build_id UUID,
    image_ref VARCHAR(1024) NOT NULL,
    config_schema JSONB NOT NULL,
    metadata JSONB,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_kind_versions PRIMARY KEY (id),
    CONSTRAINT uq_agent_kind_versions_kind_id_version UNIQUE (kind_id, version),
    CONSTRAINT uq_agent_kind_versions_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_kind_versions_org_id_kind_id FOREIGN KEY (org_id, kind_id) REFERENCES agent_kinds (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_kind_versions_org_id_build_id FOREIGN KEY (org_id, build_id) REFERENCES builds (org_id, id)
);
CREATE INDEX idx_agent_kind_versions_build_id ON agent_kind_versions (build_id);

CREATE TABLE agent_workloads (
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    workload_type VARCHAR(16) NOT NULL,
    source_type VARCHAR(16) NOT NULL,
    repo_url VARCHAR(2048),
    repo_branch VARCHAR(255),
    repo_app_path VARCHAR(1024),
    git_credential_id UUID,
    build_type VARCHAR(16),
    language VARCHAR(32),
    language_version VARCHAR(32),
    run_command VARCHAR(1024),
    dockerfile_path VARCHAR(1024),
    image_ref VARCHAR(1024),
    agent_kind_version_id UUID,
    interface_type VARCHAR(16),
    port INTEGER,
    base_path VARCHAR(255),
    interface_schema_path VARCHAR(1024),
    health_checks JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_workloads PRIMARY KEY (agent_id),
    CONSTRAINT fk_agent_workloads_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_workloads_org_id_git_credential_id FOREIGN KEY (org_id, git_credential_id) REFERENCES git_credentials (org_id, id),
    CONSTRAINT fk_agent_workloads_org_id_agent_kind_version_id FOREIGN KEY (org_id, agent_kind_version_id) REFERENCES agent_kind_versions (org_id, id),
    CONSTRAINT ck_agent_workloads_1 CHECK (workload_type IN ('agent', 'mcp_server')),
    CONSTRAINT ck_agent_workloads_2 CHECK (source_type IN ('repository', 'image', 'agent_kind')),
    CONSTRAINT ck_agent_workloads_3 CHECK (build_type IN ('buildpack', 'docker')),
    CONSTRAINT ck_agent_workloads_4 CHECK ((source_type = 'repository' AND repo_url IS NOT NULL AND build_type IS NOT NULL) OR (source_type = 'image' AND image_ref IS NOT NULL) OR (source_type = 'agent_kind' AND agent_kind_version_id IS NOT NULL))
);
CREATE INDEX idx_agent_workloads_git_credential_id ON agent_workloads (git_credential_id);
CREATE INDEX idx_agent_workloads_agent_kind_version_id ON agent_workloads (agent_kind_version_id);

CREATE TABLE agent_releases (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    build_id UUID,
    image_ref VARCHAR(1024) NOT NULL,
    config JSONB NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_releases PRIMARY KEY (id),
    CONSTRAINT uq_agent_releases_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_releases_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_releases_org_id_build_id FOREIGN KEY (org_id, build_id) REFERENCES builds (org_id, id)
);
CREATE INDEX idx_agent_releases_agent_id_created_at ON agent_releases (agent_id, created_at);
CREATE INDEX idx_agent_releases_build_id ON agent_releases (build_id);

CREATE TABLE agent_deployments (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    release_id UUID NOT NULL,
    desired_state VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL,
    status_reason TEXT,
    promoted_from_environment_id UUID,
    deployed_by VARCHAR(255),
    deployed_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL,
    next_attempt_at TIMESTAMPTZ,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_deployments PRIMARY KEY (id),
    CONSTRAINT uq_agent_deployments_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT uq_agent_deployments_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_deployments_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_deployments_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_deployments_org_id_release_id FOREIGN KEY (org_id, release_id) REFERENCES agent_releases (org_id, id),
    CONSTRAINT fk_agent_deployments_org_id_promoted_from_environment_id FOREIGN KEY (org_id, promoted_from_environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_deployments_1 CHECK (desired_state IN ('active', 'suspended', 'undeployed')),
    CONSTRAINT ck_agent_deployments_2 CHECK (status IN ('pending', 'deploying', 'active', 'suspended', 'undeploying', 'undeployed', 'failed'))
);
CREATE INDEX idx_agent_deployments_org_id_environment_id ON agent_deployments (org_id, environment_id);
CREATE INDEX idx_agent_deployments_release_id ON agent_deployments (release_id);
CREATE INDEX idx_agent_deployments_status_next_attempt_at ON agent_deployments (status, next_attempt_at);

CREATE TABLE agent_runtime_configs (
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    org_id UUID NOT NULL,
    cpu_request VARCHAR(16),
    cpu_limit VARCHAR(16),
    memory_request VARCHAR(16),
    memory_limit VARCHAR(16),
    replicas INTEGER,
    autoscaling_enabled BOOLEAN NOT NULL,
    min_replicas INTEGER,
    max_replicas INTEGER,
    isolation_tier VARCHAR(16),
    warm_pool_size INTEGER,
    instrumentation_enabled BOOLEAN NOT NULL,
    instrumentation_version VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_runtime_configs PRIMARY KEY (agent_id, environment_id),
    CONSTRAINT fk_agent_runtime_configs_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_runtime_configs_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_runtime_configs_1 CHECK (isolation_tier IN ('gvisor', 'kata')),
    CONSTRAINT ck_agent_runtime_configs_2 CHECK (min_replicas IS NULL OR max_replicas IS NULL OR min_replicas <= max_replicas)
);

CREATE TABLE agent_env_vars (
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    value TEXT,
    secret_id UUID,
    is_system BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_env_vars PRIMARY KEY (agent_id, environment_id, name),
    CONSTRAINT fk_agent_env_vars_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_env_vars_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_env_vars_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_agent_env_vars_1 CHECK (value IS NULL OR secret_id IS NULL)
);
CREATE INDEX idx_agent_env_vars_secret_id ON agent_env_vars (secret_id);

CREATE TABLE agent_file_mounts (
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    mount_path VARCHAR(255) NOT NULL,
    org_id UUID NOT NULL,
    file_key VARCHAR(255) NOT NULL,
    content TEXT,
    secret_id UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_file_mounts PRIMARY KEY (agent_id, environment_id, mount_path),
    CONSTRAINT fk_agent_file_mounts_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_file_mounts_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_file_mounts_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_agent_file_mounts_1 CHECK (content IS NULL OR secret_id IS NULL)
);
CREATE INDEX idx_agent_file_mounts_secret_id ON agent_file_mounts (secret_id);

CREATE TABLE agent_memory_stores (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    name VARCHAR(63) NOT NULL,
    memory_type VARCHAR(16) NOT NULL,
    backend VARCHAR(32) NOT NULL,
    config JSONB NOT NULL,
    secret_id UUID,
    retention_days INTEGER,
    status VARCHAR(16) NOT NULL,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_agent_memory_stores PRIMARY KEY (id),
    CONSTRAINT uq_agent_memory_stores_agent_id_environment_id_name UNIQUE (agent_id, environment_id, name),
    CONSTRAINT uq_agent_memory_stores_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_agent_memory_stores_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_memory_stores_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT fk_agent_memory_stores_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id),
    CONSTRAINT ck_agent_memory_stores_1 CHECK (memory_type IN ('short_term', 'long_term')),
    CONSTRAINT ck_agent_memory_stores_2 CHECK (status IN ('provisioning', 'ready', 'failed', 'deleting'))
);
CREATE INDEX idx_agent_memory_stores_secret_id ON agent_memory_stores (secret_id);
