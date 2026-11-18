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

CREATE TABLE modules (
    name VARCHAR(63) NOT NULL,
    version VARCHAR(64) NOT NULL,
    state VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_modules PRIMARY KEY (name),
    CONSTRAINT ck_modules_1 CHECK (state IN ('active', 'disabled', 'uninstalled'))
);

CREATE TABLE module_generation (
    id INTEGER NOT NULL,
    value BIGINT NOT NULL,
    CONSTRAINT pk_module_generation PRIMARY KEY (id),
    CONSTRAINT ck_module_generation_1 CHECK (id = 1)
);
INSERT INTO module_generation (id, value) VALUES (1, 0);

CREATE TABLE module_migrations (
    module VARCHAR(63) NOT NULL,
    version VARCHAR(255) NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_module_migrations PRIMARY KEY (module, version)
);

CREATE TABLE module_events (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    type VARCHAR(128) NOT NULL,
    source VARCHAR(63) NOT NULL,
    subject VARCHAR(255),
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_module_events PRIMARY KEY (id),
    CONSTRAINT fk_module_events_org_id FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE module_event_deliveries (
    event_id UUID NOT NULL,
    module VARCHAR(63) NOT NULL,
    attempts INTEGER NOT NULL,
    available_at TIMESTAMPTZ NOT NULL,
    claimed_by VARCHAR(64),
    last_error VARCHAR(1024),
    failed_at TIMESTAMPTZ,
    CONSTRAINT pk_module_event_deliveries PRIMARY KEY (event_id, module),
    CONSTRAINT fk_module_event_deliveries_event_id FOREIGN KEY (event_id) REFERENCES module_events (id) ON DELETE CASCADE
);
CREATE INDEX idx_module_event_deliveries_available_at ON module_event_deliveries (available_at);

CREATE TABLE workload_credentials (
    id UUID NOT NULL,
    module VARCHAR(63) NOT NULL,
    workload VARCHAR(63) NOT NULL,
    org_id UUID,
    subject VARCHAR(255) NOT NULL,
    secret_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT pk_workload_credentials PRIMARY KEY (id),
    CONSTRAINT fk_workload_credentials_org_id FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
);
CREATE INDEX idx_workload_credentials_module_workload ON workload_credentials (module, workload);

CREATE TABLE workload_instances (
    credential_id UUID NOT NULL,
    instance VARCHAR(255) NOT NULL,
    module VARCHAR(63) NOT NULL,
    workload VARCHAR(63) NOT NULL,
    protocol VARCHAR(63) NOT NULL,
    version VARCHAR(63) NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_workload_instances PRIMARY KEY (credential_id, instance),
    CONSTRAINT fk_workload_instances_credential_id FOREIGN KEY (credential_id) REFERENCES workload_credentials (id) ON DELETE CASCADE
);
CREATE INDEX idx_workload_instances_module_workload_last_seen ON workload_instances (module, workload, last_seen);
