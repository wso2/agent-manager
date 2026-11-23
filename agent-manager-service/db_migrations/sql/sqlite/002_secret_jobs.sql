-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE secret_jobs (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    secret_id TEXT NOT NULL,
    owner_kind TEXT NOT NULL,
    action TEXT NOT NULL,
    provider TEXT,
    external_ref TEXT,
    staged_provider TEXT,
    staged_ref TEXT,
    version INTEGER,
    attempts INTEGER NOT NULL,
    last_error TEXT,
    available_at TIMESTAMP NOT NULL,
    claimed_by TEXT,
    claimed_until TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_secret_jobs PRIMARY KEY (id)
);
CREATE INDEX idx_secret_jobs_available_at_id ON secret_jobs (available_at, id);
CREATE INDEX idx_secret_jobs_secret_id ON secret_jobs (secret_id);
CREATE INDEX idx_secret_jobs_provider_external_ref ON secret_jobs (provider, external_ref);
