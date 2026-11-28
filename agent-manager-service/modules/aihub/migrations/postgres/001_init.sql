-- Code generated from modules/aihub/schema/001_init.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_hub_subscription_plans (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    handle VARCHAR(40) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    display_name_folded VARCHAR(765) NOT NULL,
    description VARCHAR(1023),
    status VARCHAR(16) NOT NULL,
    expiry_time TIMESTAMPTZ,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    source_changed_at TIMESTAMPTZ,
    origin VARCHAR(255) NOT NULL,
    deprecated_at TIMESTAMPTZ,
    CONSTRAINT pk_mod_ai_hub_subscription_plans PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_subscription_plans_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_hub_subscription_plans_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_subscription_plan_conditions (
    org_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    condition_id VARCHAR(63) NOT NULL,
    condition_type VARCHAR(32) NOT NULL,
    params JSONB NOT NULL,
    sort_order INTEGER NOT NULL,
    CONSTRAINT pk_mod_ai_hub_subscription_plan_conditions PRIMARY KEY (plan_id, condition_id),
    CONSTRAINT fk_mod_ai_hub_subscription_plan_conditions_org_id_plan_id FOREIGN KEY (org_id, plan_id) REFERENCES mod_ai_hub_subscription_plans (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_ai_hub_subscriptions (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    source_id VARCHAR(64) NOT NULL,
    artifact_kind VARCHAR(32) NOT NULL,
    artifact_handle VARCHAR(255) NOT NULL,
    plan_id UUID NOT NULL,
    subscriber_id VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    source_changed_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_hub_subscriptions PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_source_id UNIQUE (org_id, source_id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_artifact_kind_artif_78ce39df UNIQUE (org_id, artifact_kind, artifact_handle, subscriber_id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_webhook_secrets (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    handle VARCHAR(40) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    org_ref VARCHAR(255),
    last_version INTEGER NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_hub_webhook_secrets PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_webhook_secrets_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_hub_webhook_secrets_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_webhook_secret_versions (
    org_id UUID NOT NULL,
    secret_id UUID NOT NULL,
    version INTEGER NOT NULL,
    secret_hash VARCHAR(64) NOT NULL,
    masked_secret VARCHAR(32) NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_hub_webhook_secret_versions PRIMARY KEY (secret_id, version),
    CONSTRAINT uq_mod_ai_hub_webhook_secret_versions_secret_hash UNIQUE (secret_hash),
    CONSTRAINT fk_mod_ai_hub_webhook_secret_versions_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES mod_ai_hub_webhook_secrets (org_id, id) ON DELETE CASCADE
);
