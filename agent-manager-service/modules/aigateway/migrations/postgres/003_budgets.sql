-- Code generated from modules/aigateway/schema/003_budgets.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_budgets (
    id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    origin VARCHAR(16) NOT NULL,
    gateway_id UUID,
    project_id UUID,
    configuration JSONB NOT NULL,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_budgets PRIMARY KEY (id),
    CONSTRAINT uq_mod_ai_gateway_budgets_org_id_name UNIQUE (org_id, name),
    CONSTRAINT uq_mod_ai_gateway_budgets_org_id_id UNIQUE (org_id, id),
    CONSTRAINT fk_mod_ai_gateway_budgets_org_id_gateway_id FOREIGN KEY (org_id, gateway_id) REFERENCES mod_ai_gateway_gateways (org_id, id) ON DELETE CASCADE,
    CONSTRAINT ck_mod_ai_gateway_budgets_1 CHECK (origin IN ('control_plane', 'gateway_api')),
    CONSTRAINT ck_mod_ai_gateway_budgets_2 CHECK ((origin = 'gateway_api') = (gateway_id IS NOT NULL))
);
CREATE INDEX idx_mod_ai_gateway_budgets_gateway_id ON mod_ai_gateway_budgets (gateway_id);
CREATE INDEX idx_mod_ai_gateway_budgets_project_id ON mod_ai_gateway_budgets (project_id);

CREATE TABLE mod_ai_gateway_budget_usage (
    budget_id UUID NOT NULL,
    allowance_id UUID NOT NULL,
    org_id UUID NOT NULL,
    allowance JSONB NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    spent VARCHAR(32) NOT NULL,
    unpriced_requests BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_budget_usage PRIMARY KEY (budget_id, allowance_id, period_start),
    CONSTRAINT fk_mod_ai_gateway_budget_usage_org_id_budget_id FOREIGN KEY (org_id, budget_id) REFERENCES mod_ai_gateway_budgets (org_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_mod_ai_gateway_budget_usage_org_id_budget_id_period_start ON mod_ai_gateway_budget_usage (org_id, budget_id, period_start);

CREATE TABLE mod_ai_gateway_budget_warnings (
    budget_id UUID NOT NULL,
    allowance_id UUID NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    amount VARCHAR(32) NOT NULL,
    org_id UUID NOT NULL,
    threshold INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_budget_warnings PRIMARY KEY (budget_id, allowance_id, period_start, amount),
    CONSTRAINT fk_mod_ai_gateway_budget_warnings_budget_id_allowance__be5ce6bb FOREIGN KEY (budget_id, allowance_id, period_start) REFERENCES mod_ai_gateway_budget_usage (budget_id, allowance_id, period_start) ON DELETE CASCADE
);

CREATE TABLE mod_ai_gateway_cost_usage (
    org_id UUID NOT NULL,
    month TIMESTAMPTZ NOT NULL,
    cost_key UUID NOT NULL,
    project VARCHAR(63),
    agent VARCHAR(63),
    api_key VARCHAR(255),
    user_id VARCHAR(255),
    llm_provider VARCHAR(255) NOT NULL,
    model VARCHAR(255),
    cost VARCHAR(32) NOT NULL,
    missing_price_requests BIGINT NOT NULL,
    usage_unavailable_requests BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_cost_usage PRIMARY KEY (org_id, month, cost_key)
);
CREATE INDEX idx_mod_ai_gateway_cost_usage_month ON mod_ai_gateway_cost_usage (month);
