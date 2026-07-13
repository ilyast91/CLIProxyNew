CREATE TABLE usage_events (
    id bigint GENERATED ALWAYS AS IDENTITY,
    user_id bigint REFERENCES users(id),
    api_key_id bigint REFERENCES api_keys(id) ON DELETE SET NULL,
    upstream_account_id text REFERENCES upstream_accounts(id) ON DELETE SET NULL,
    provider text,
    model text,
    input_tokens integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    reasoning_tokens integer NOT NULL DEFAULT 0,
    cached_tokens integer NOT NULL DEFAULT 0,
    total_tokens integer NOT NULL DEFAULT 0,
    status_code integer NOT NULL,
    error text,
    latency_ms integer NOT NULL DEFAULT 0,
    ttft_ms integer NOT NULL DEFAULT 0,
    failed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE usage_events_20260713
    PARTITION OF usage_events
    FOR VALUES FROM ('2026-07-13 00:00:00+03') TO ('2026-07-14 00:00:00+03');

CREATE INDEX usage_events_user_created_idx ON usage_events (user_id, created_at DESC);
CREATE INDEX usage_events_provider_created_idx ON usage_events (provider, created_at DESC);
CREATE INDEX usage_events_model_created_idx ON usage_events (model, created_at DESC);

CREATE MATERIALIZED VIEW usage_aggregates AS
SELECT
    created_at::date AS period_start,
    'user'::text AS dimension,
    user_id::text AS dimension_value,
    count(*)::bigint AS request_count,
    sum(input_tokens)::bigint AS input_tokens_sum,
    sum(output_tokens)::bigint AS output_tokens_sum,
    sum(total_tokens)::bigint AS total_tokens_sum,
    count(*) FILTER (WHERE failed)::bigint AS failure_count
FROM usage_events
WHERE user_id IS NOT NULL
GROUP BY created_at::date, user_id
UNION ALL
SELECT
    created_at::date,
    'model'::text,
    model,
    count(*)::bigint,
    sum(input_tokens)::bigint,
    sum(output_tokens)::bigint,
    sum(total_tokens)::bigint,
    count(*) FILTER (WHERE failed)::bigint
FROM usage_events
WHERE model IS NOT NULL
GROUP BY created_at::date, model
UNION ALL
SELECT
    created_at::date,
    'provider'::text,
    provider,
    count(*)::bigint,
    sum(input_tokens)::bigint,
    sum(output_tokens)::bigint,
    sum(total_tokens)::bigint,
    count(*) FILTER (WHERE failed)::bigint
FROM usage_events
WHERE provider IS NOT NULL
GROUP BY created_at::date, provider
UNION ALL
SELECT
    created_at::date,
    'api_key'::text,
    api_key_id::text,
    count(*)::bigint,
    sum(input_tokens)::bigint,
    sum(output_tokens)::bigint,
    sum(total_tokens)::bigint,
    count(*) FILTER (WHERE failed)::bigint
FROM usage_events
WHERE api_key_id IS NOT NULL
GROUP BY created_at::date, api_key_id
WITH NO DATA;

CREATE UNIQUE INDEX usage_aggregates_identity_idx
    ON usage_aggregates (period_start, dimension, dimension_value);

