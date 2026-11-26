-- Code generated from modules/aigateway/schema/004_pricing.yaml by `make schema`. DO NOT EDIT.

CREATE TABLE mod_ai_gateway_model_prices (
    model VARCHAR(255) NOT NULL,
    provider VARCHAR(64),
    input_per_million VARCHAR(32),
    output_per_million VARCHAR(32),
    cached_input_per_million VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_mod_ai_gateway_model_prices PRIMARY KEY (model)
);
CREATE INDEX idx_mod_ai_gateway_model_prices_provider ON mod_ai_gateway_model_prices (provider);
