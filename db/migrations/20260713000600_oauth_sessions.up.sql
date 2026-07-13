CREATE TABLE oauth_sessions (
    state text PRIMARY KEY,
    provider text NOT NULL,
    flow_type text NOT NULL CHECK (flow_type IN ('callback', 'device')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'error', 'cancelled')),
    auth_id text REFERENCES upstream_accounts(id) ON DELETE SET NULL,
    pkce_verifier text,
    device_code text,
    user_code text,
    error_message text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_sessions_status_expires_idx ON oauth_sessions (status, expires_at);

