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
    id TEXT NOT NULL,
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
    CONSTRAINT pk_agent_identities PRIMARY KEY (id),
    CONSTRAINT uq_agent_identities_agent_id_environment_id UNIQUE (agent_id, environment_id),
    CONSTRAINT fk_agent_identities_org_id_agent_id FOREIGN KEY (org_id, agent_id) REFERENCES agents (org_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_identities_org_id_environment_id FOREIGN KEY (org_id, environment_id) REFERENCES environments (org_id, id),
    CONSTRAINT ck_agent_identities_1 CHECK (status IN ('pending', 'in_progress', 'completed', 'failed'))
);
CREATE INDEX idx_agent_identities_status_next_retry_at ON agent_identities (status, next_retry_at);

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

CREATE TABLE modules (
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_modules PRIMARY KEY (name),
    CONSTRAINT ck_modules_1 CHECK (state IN ('active', 'disabled', 'uninstalled'))
);

CREATE TABLE module_generation (
    id INTEGER NOT NULL,
    value INTEGER NOT NULL,
    CONSTRAINT pk_module_generation PRIMARY KEY (id),
    CONSTRAINT ck_module_generation_1 CHECK (id = 1)
);
INSERT INTO module_generation (id, value) VALUES (1, 0);

CREATE TABLE module_migrations (
    module TEXT NOT NULL,
    version TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_module_migrations PRIMARY KEY (module, version)
);

CREATE TABLE module_events (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    type TEXT NOT NULL,
    source TEXT NOT NULL,
    subject TEXT,
    data TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_module_events PRIMARY KEY (id),
    CONSTRAINT fk_module_events_org_id FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT ck_module_events_1 CHECK (json_valid(data))
);

CREATE TABLE module_event_deliveries (
    event_id TEXT NOT NULL,
    module TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    available_at TIMESTAMP NOT NULL,
    claimed_by TEXT,
    last_error TEXT,
    failed_at TIMESTAMP,
    CONSTRAINT pk_module_event_deliveries PRIMARY KEY (event_id, module),
    CONSTRAINT fk_module_event_deliveries_event_id FOREIGN KEY (event_id) REFERENCES module_events (id) ON DELETE CASCADE
);
CREATE INDEX idx_module_event_deliveries_available_at ON module_event_deliveries (available_at);

CREATE TABLE workload_credentials (
    id TEXT NOT NULL,
    module TEXT NOT NULL,
    workload TEXT NOT NULL,
    org_id TEXT,
    subject TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,
    CONSTRAINT pk_workload_credentials PRIMARY KEY (id),
    CONSTRAINT fk_workload_credentials_org_id FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
);
CREATE INDEX idx_workload_credentials_module_workload ON workload_credentials (module, workload);

CREATE TABLE workload_instances (
    credential_id TEXT NOT NULL,
    instance TEXT NOT NULL,
    module TEXT NOT NULL,
    workload TEXT NOT NULL,
    protocol TEXT NOT NULL,
    version TEXT NOT NULL,
    first_seen TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL,
    CONSTRAINT pk_workload_instances PRIMARY KEY (credential_id, instance),
    CONSTRAINT fk_workload_instances_credential_id FOREIGN KEY (credential_id) REFERENCES workload_credentials (id) ON DELETE CASCADE
);
CREATE INDEX idx_workload_instances_module_workload_last_seen ON workload_instances (module, workload, last_seen);
