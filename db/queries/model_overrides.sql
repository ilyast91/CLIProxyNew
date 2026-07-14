-- name: UpsertModelOverride :one
INSERT INTO model_overrides (provider, model_alias, upstream_model, enabled, config)
VALUES (
    sqlc.arg(provider),
    sqlc.arg(model_alias),
    sqlc.arg(upstream_model),
    sqlc.arg(enabled),
    sqlc.narg(config)
)
ON CONFLICT (model_alias) DO UPDATE SET
    provider = EXCLUDED.provider,
    upstream_model = EXCLUDED.upstream_model,
    enabled = EXCLUDED.enabled,
    config = EXCLUDED.config
RETURNING id, provider, model_alias, upstream_model, enabled, config;

-- name: ListModelOverrides :many
SELECT id, provider, model_alias, upstream_model, enabled, config
FROM model_overrides
ORDER BY model_alias;

-- name: DeleteModelOverride :execrows
DELETE FROM model_overrides
WHERE model_alias = $1;
