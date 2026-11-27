-- Code generated from modules/aihub/schema/001_init.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_hub_subscription_plans (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    display_name TEXT NOT NULL,
    display_name_folded TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    expiry_time TIMESTAMP,
    created_by TEXT,
    updated_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    source_changed_at TIMESTAMP,
    origin TEXT NOT NULL,
    deprecated_at TIMESTAMP,
    CONSTRAINT pk_mod_ai_hub_subscription_plans PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_subscription_plans_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_hub_subscription_plans_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_subscription_plan_conditions (
    org_id TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    condition_id TEXT NOT NULL,
    condition_type TEXT NOT NULL,
    params TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    CONSTRAINT pk_mod_ai_hub_subscription_plan_conditions PRIMARY KEY (plan_id, condition_id),
    CONSTRAINT fk_mod_ai_hub_subscription_plan_conditions_org_id_plan_id FOREIGN KEY (org_id, plan_id) REFERENCES mod_ai_hub_subscription_plans (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_hub_subscription_plan_conditions_1 CHECK (json_valid(params))
);

CREATE TABLE mod_ai_hub_subscriptions (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    artifact_kind TEXT NOT NULL,
    artifact_handle TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    subscriber_id TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    source_changed_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_hub_subscriptions PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_source_id UNIQUE (org_id, source_id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_artifact_kind_artif_78ce39df UNIQUE (org_id, artifact_kind, artifact_handle, subscriber_id),
    CONSTRAINT uq_mod_ai_hub_subscriptions_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_webhook_secrets (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    display_name TEXT NOT NULL,
    org_ref TEXT,
    last_version INTEGER NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_hub_webhook_secrets PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_hub_webhook_secrets_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_ai_hub_webhook_secrets_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_ai_hub_webhook_secret_versions (
    org_id TEXT NOT NULL,
    secret_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    secret_hash TEXT NOT NULL,
    masked_secret TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_hub_webhook_secret_versions PRIMARY KEY (secret_id, version),
    CONSTRAINT uq_mod_ai_hub_webhook_secret_versions_secret_hash UNIQUE (secret_hash),
    CONSTRAINT fk_mod_ai_hub_webhook_secret_versions_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES mod_ai_hub_webhook_secrets (org_id, id) ON DELETE CASCADE
);
