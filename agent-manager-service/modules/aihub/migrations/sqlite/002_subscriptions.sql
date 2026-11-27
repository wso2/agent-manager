-- Code generated from modules/aihub/schema/002_subscriptions.yaml by `make schema`. DO NOT EDIT.

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
