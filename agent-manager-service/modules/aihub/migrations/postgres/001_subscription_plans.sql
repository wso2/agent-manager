-- Code generated from modules/aihub/schema/001_subscription_plans.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_aihub_subscription_plans (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    handle VARCHAR(40) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    display_name_folded VARCHAR(255) NOT NULL,
    description VARCHAR(1023),
    status VARCHAR(16) NOT NULL,
    expiry_time TIMESTAMPTZ,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    source_changed_at TIMESTAMPTZ,
    CONSTRAINT pk_mod_aihub_subscription_plans PRIMARY KEY (id),
    CONSTRAINT uq_mod_aihub_subscription_plans_org_id_handle UNIQUE (org_id, handle),
    CONSTRAINT uq_mod_aihub_subscription_plans_org_id_id UNIQUE (org_id, id)
);

CREATE TABLE mod_aihub_subscription_plan_conditions (
    org_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    condition_id VARCHAR(63) NOT NULL,
    condition_type VARCHAR(32) NOT NULL,
    params JSONB NOT NULL,
    sort_order INTEGER NOT NULL,
    CONSTRAINT pk_mod_aihub_subscription_plan_conditions PRIMARY KEY (plan_id, condition_id),
    CONSTRAINT fk_mod_aihub_subscription_plan_conditions_org_id_plan_id FOREIGN KEY (org_id, plan_id) REFERENCES mod_aihub_subscription_plans (org_id, id) ON DELETE CASCADE
);

CREATE TABLE mod_aihub_deleted_plans (
    org_id UUID NOT NULL,
    handle VARCHAR(40) NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_aihub_deleted_plans PRIMARY KEY (org_id, handle)
);
