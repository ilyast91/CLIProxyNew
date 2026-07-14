-- name: InsertUsageEvent :exec
INSERT INTO usage_events (
    user_id, api_key_id, upstream_account_id, provider, model,
    input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens,
    status_code, error, latency_ms, ttft_ms, failed
)
VALUES (
    sqlc.narg(user_id), sqlc.narg(api_key_id), sqlc.narg(upstream_account_id), sqlc.narg(provider), sqlc.narg(model),
    sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(reasoning_tokens), sqlc.arg(cached_tokens), sqlc.arg(total_tokens),
    sqlc.arg(status_code), sqlc.narg(error), sqlc.arg(latency_ms), sqlc.arg(ttft_ms), sqlc.arg(failed)
);

-- name: GetUsageSummaryByUser :one
SELECT
    count(*)::bigint AS request_count,
    count(*) FILTER (WHERE failed)::bigint AS failed_request_count,
    COALESCE(sum(input_tokens), 0)::bigint AS input_tokens,
    COALESCE(sum(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(sum(reasoning_tokens), 0)::bigint AS reasoning_tokens,
    COALESCE(sum(cached_tokens), 0)::bigint AS cached_tokens,
    COALESCE(sum(total_tokens), 0)::bigint AS total_tokens
FROM usage_events
WHERE user_id = $1
  AND created_at >= $2
  AND created_at < $3;

-- name: ListUsageByModelForUser :many
SELECT
    model,
    count(*)::bigint AS request_count,
    count(*) FILTER (WHERE failed)::bigint AS failed_request_count,
    COALESCE(sum(total_tokens), 0)::bigint AS total_tokens
FROM usage_events
WHERE user_id = $1
  AND created_at >= $2
  AND created_at < $3
  AND model IS NOT NULL
  AND model <> ''
GROUP BY model
ORDER BY total_tokens DESC, model ASC;

-- name: ListUsageByAPIKeyForUser :many
SELECT
    api_key_id,
    count(*)::bigint AS request_count,
    count(*) FILTER (WHERE failed)::bigint AS failed_request_count,
    COALESCE(sum(total_tokens), 0)::bigint AS total_tokens
FROM usage_events
WHERE user_id = $1
  AND created_at >= $2
  AND created_at < $3
  AND api_key_id IS NOT NULL
GROUP BY api_key_id
ORDER BY total_tokens DESC, api_key_id ASC;
