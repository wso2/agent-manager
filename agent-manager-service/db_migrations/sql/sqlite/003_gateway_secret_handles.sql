-- Code generated from db_migrations/schema/lite.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE gateway_secret_handles (
    handle TEXT NOT NULL,
    org_id TEXT NOT NULL,
    secret_id TEXT NOT NULL,
    retired_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_gateway_secret_handles PRIMARY KEY (handle),
    CONSTRAINT fk_gateway_secret_handles_org_id_secret_id FOREIGN KEY (org_id, secret_id) REFERENCES secrets (org_id, id)
);
CREATE INDEX idx_gateway_secret_handles_secret_id ON gateway_secret_handles (secret_id);
CREATE INDEX idx_gateway_secret_handles_org_id_retired_at ON gateway_secret_handles (org_id, retired_at);
