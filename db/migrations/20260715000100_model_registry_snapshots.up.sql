CREATE TABLE model_registry_snapshots (
    provider text NOT NULL,
    client_id text NOT NULL,
    models jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, client_id),
    CHECK (jsonb_typeof(models) = 'array')
);
