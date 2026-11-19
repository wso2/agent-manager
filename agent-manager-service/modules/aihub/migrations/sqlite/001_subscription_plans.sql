-- Code generated from modules/aihub/schema/001_subscription_plans.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_aihub_subscription_plans (
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
    CONSTRAINT pk_mod_aihub_subscription_plans PRIMARY KEY (id),
    CONSTRAINT uq_mod_aihub_subscription_plans_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_aihub_subscription_plans_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_aihub_subscription_plan_conditions (
    org_id TEXT NOT NULL,
    plan_id TEXT NOT NULL,
    condition_id TEXT NOT NULL,
    condition_type TEXT NOT NULL,
    params TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    CONSTRAINT pk_mod_aihub_subscription_plan_conditions PRIMARY KEY (plan_id, condition_id),
    CONSTRAINT fk_mod_aihub_subscription_plan_conditions_org_id_plan_id FOREIGN KEY (org_id, plan_id) REFERENCES mod_aihub_subscription_plans (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_aihub_subscription_plan_conditions_1 CHECK (json_valid(params))
);

CREATE TABLE mod_aihub_deleted_plans (
    org_id TEXT NOT NULL,
    handle TEXT NOT NULL,
    deleted_at TIMESTAMP NOT NULL,
    recorded_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_aihub_deleted_plans PRIMARY KEY (org_id, handle)
);
