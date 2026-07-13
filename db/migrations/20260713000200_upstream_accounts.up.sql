CREATE TABLE upstream_accounts (
    id text PRIMARY KEY,
    provider text NOT NULL,
    email text NOT NULL,
    auth_type text NOT NULL CHECK (auth_type IN ('oauth', 'api-key')),
    label text,
    credentials_enc bytea NOT NULL,
    enc_key_version integer NOT NULL CHECK (enc_key_version > 0),
    attributes jsonb,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'unavailable')),
    last_refreshed_at timestamptz,
    next_refresh_after timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, email)
);

CREATE INDEX upstream_accounts_status_idx ON upstream_accounts (status);

