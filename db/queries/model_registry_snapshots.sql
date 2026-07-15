-- name: UpsertModelRegistrySnapshot :exec
INSERT INTO model_registry_snapshots (provider, client_id, models)
VALUES (sqlc.arg(provider), sqlc.arg(client_id), sqlc.arg(models))
ON CONFLICT (provider, client_id) DO UPDATE SET
    models = EXCLUDED.models,
    updated_at = now();

-- name: ListModelRegistrySnapshots :many
SELECT provider, client_id, models, updated_at
FROM model_registry_snapshots
ORDER BY provider, client_id;

-- name: DeleteModelRegistrySnapshot :execrows
DELETE FROM model_registry_snapshots
WHERE provider = $1 AND client_id = $2;
