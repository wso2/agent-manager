-- Code generated from tests/modules/testmod/schema/001_tags.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_testmod_agent_tags (
    agent_id UUID NOT NULL,
    org_id UUID NOT NULL,
    tag VARCHAR(64) NOT NULL,
    CONSTRAINT pk_mod_testmod_agent_tags PRIMARY KEY (agent_id)
);

CREATE TABLE mod_testmod_received (
    event_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    org_id UUID NOT NULL,
    subject VARCHAR(255),
    CONSTRAINT pk_mod_testmod_received PRIMARY KEY (event_id, event_type)
);
