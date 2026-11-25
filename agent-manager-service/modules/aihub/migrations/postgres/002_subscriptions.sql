-- Code generated from modules/aihub/schema/002_subscriptions.yaml by `make schema`. DO NOT EDIT.

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
