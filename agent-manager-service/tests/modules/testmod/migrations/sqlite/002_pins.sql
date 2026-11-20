-- Code generated from tests/modules/testmod/schema/002_pins.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_testmod_project_pins (
    project_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    CONSTRAINT pk_mod_testmod_project_pins PRIMARY KEY (project_id)
);
