-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE secret_jobs (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    secret_id UUID NOT NULL,
    owner_kind VARCHAR(32) NOT NULL,
    action VARCHAR(16) NOT NULL,
    provider VARCHAR(16),
    external_ref VARCHAR(512),
    staged_provider VARCHAR(16),
    staged_ref VARCHAR(512),
    version INTEGER,
    attempts INTEGER NOT NULL,
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL,
    claimed_by VARCHAR(255),
    claimed_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_secret_jobs PRIMARY KEY (id)
);
CREATE INDEX idx_secret_jobs_available_at_id ON secret_jobs (available_at, id);
CREATE INDEX idx_secret_jobs_secret_id ON secret_jobs (secret_id);
CREATE INDEX idx_secret_jobs_provider_external_ref ON secret_jobs (provider, external_ref);
