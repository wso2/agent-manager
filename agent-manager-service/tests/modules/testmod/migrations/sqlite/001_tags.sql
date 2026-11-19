-- Code generated from tests/modules/testmod/schema/001_tags.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_testmod_agent_tags (
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    tag TEXT NOT NULL,
    CONSTRAINT pk_mod_testmod_agent_tags PRIMARY KEY (agent_id)
);

CREATE TABLE mod_testmod_received (
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    org_id TEXT NOT NULL,
    subject TEXT,
    CONSTRAINT pk_mod_testmod_received PRIMARY KEY (event_id, event_type)
);
