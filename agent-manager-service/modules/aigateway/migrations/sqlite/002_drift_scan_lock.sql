-- Code generated from modules/aigateway/schema/002_drift_scan_lock.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_locks (
    name TEXT NOT NULL,
    owner TEXT,
    expires_at TIMESTAMP,
    CONSTRAINT pk_mod_ai_gateway_locks PRIMARY KEY (name)
);
INSERT INTO mod_ai_gateway_locks (name) VALUES ('drift_scan');
