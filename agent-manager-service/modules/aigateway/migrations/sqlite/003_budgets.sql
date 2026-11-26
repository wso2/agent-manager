-- Code generated from modules/aigateway/schema/003_budgets.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_budgets (
    id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    origin TEXT NOT NULL,
    gateway_id TEXT,
    configuration TEXT NOT NULL,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_budgets PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_budgets_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_mod_ai_gateway_budgets_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_budgets_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_budgets_1 CHECK (json_valid(configuration)),
    CONSTRAINT ck_mod_ai_gateway_budgets_2 CHECK (origin IN ('control_plane', 'gateway_api')),
    CONSTRAINT ck_mod_ai_gateway_budgets_3 CHECK ((origin = 'gateway_api') = (gateway_id IS NOT NULL))
);
CREATE INDEX idx_mod_ai_gateway_budgets_gateway_id ON mod_ai_gateway_budgets (gateway_id);

CREATE TABLE mod_ai_gateway_budget_usage (
    budget_id TEXT NOT NULL,
    budget_usage_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    budget_usage TEXT NOT NULL,
    period_start TIMESTAMP NOT NULL,
    spent TEXT NOT NULL,
    unpriced_requests INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_budget_usage PRIMARY KEY (budget_id, budget_usage_id, period_start),
    CONSTRAINT fk_mod_ai_gateway_budget_usage_org_id_budget_id FOREIGN KEY (org_id, budget_id) REFERENCES mod_ai_gateway_budgets (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_budget_usage_1 CHECK (json_valid(budget_usage))
);
CREATE INDEX idx_mod_ai_gateway_budget_usage_org_id_budget_id_period_start ON mod_ai_gateway_budget_usage (org_id, budget_id, period_start);

CREATE TABLE mod_ai_gateway_cost_usage (
    org_id TEXT NOT NULL,
    month TIMESTAMP NOT NULL,
    cost_key TEXT NOT NULL,
    project TEXT,
    agent TEXT,
    api_key TEXT,
    user_id TEXT,
    llm_provider TEXT NOT NULL,
    model TEXT,
    cost TEXT NOT NULL,
    missing_price_requests INTEGER NOT NULL,
    usage_unavailable_requests INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_cost_usage PRIMARY KEY (org_id, month, cost_key)
);
CREATE INDEX idx_mod_ai_gateway_cost_usage_month ON mod_ai_gateway_cost_usage (month);
