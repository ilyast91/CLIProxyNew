CREATE TABLE runtime_revisions (
    name text PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO runtime_revisions (name) VALUES ('upstream_accounts');
