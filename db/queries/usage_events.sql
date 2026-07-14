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
