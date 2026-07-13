CREATE TABLE model_overrides (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider text NOT NULL,
    model_alias text NOT NULL UNIQUE,
    upstream_model text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    config jsonb
);

