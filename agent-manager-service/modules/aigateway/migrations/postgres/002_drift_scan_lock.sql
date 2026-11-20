-- Code generated from modules/aigateway/schema/002_drift_scan_lock.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_locks (
    name VARCHAR(128) NOT NULL,
    owner VARCHAR(255),
    expires_at TIMESTAMPTZ,
    CONSTRAINT pk_mod_ai_gateway_locks PRIMARY KEY (name)
);
INSERT INTO mod_ai_gateway_locks (name) VALUES ('drift_scan');
