-- Code generated from modules/aigateway/schema/004_pricing.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_model_prices (
    model TEXT NOT NULL,
    provider TEXT,
    input_per_million TEXT,
    output_per_million TEXT,
    cached_input_per_million TEXT,
    created_at TIMESTAMP NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_model_prices PRIMARY KEY (model)
);
CREATE INDEX idx_mod_ai_gateway_model_prices_provider ON mod_ai_gateway_model_prices (provider);
