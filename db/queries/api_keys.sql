-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, key_hash, key_prefix, name, expires_at, scope)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(key_hash),
    sqlc.arg(key_prefix),
    sqlc.narg(name),
    sqlc.narg(expires_at),
    sqlc.narg(scope)
)
RETURNING id, user_id, key_prefix, name, status, expires_at, scope, last_used_at, created_at;

-- name: FindAPIKeyCandidates :many
SELECT
    api_keys.id,
    api_keys.user_id,
    api_keys.key_hash,
    api_keys.status,
    api_keys.expires_at,
    users.status AS user_status
FROM api_keys
JOIN users ON users.id = api_keys.user_id
WHERE api_keys.key_prefix = $1
  AND api_keys.status = 'active'
  AND (api_keys.expires_at IS NULL OR api_keys.expires_at >= CURRENT_DATE);

-- name: FindAPIKeyCandidatesForSource :many
SELECT
    api_keys.id,
    api_keys.user_id,
    api_keys.key_hash,
    api_keys.status,
    api_keys.expires_at,
    users.status AS user_status
FROM api_keys
JOIN users ON users.id = api_keys.user_id
WHERE api_keys.key_prefix = sqlc.arg(key_prefix)
  AND users.identity_source = sqlc.arg(identity_source)
  AND api_keys.status = 'active'
  AND (api_keys.expires_at IS NULL OR api_keys.expires_at >= CURRENT_DATE);

-- name: RevokeAPIKey :execrows
UPDATE api_keys
SET status = 'revoked'
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND status = 'active';

-- name: ListAPIKeysByUser :many
SELECT id, user_id, key_prefix, name, status, expires_at, scope, last_used_at, created_at
FROM api_keys
WHERE user_id = $1
ORDER BY id DESC;

-- name: ListAllAPIKeys :many
SELECT
    api_keys.id,
    api_keys.user_id,
    api_keys.key_prefix,
    api_keys.name,
    api_keys.status,
    api_keys.expires_at,
    api_keys.scope,
    api_keys.last_used_at,
    api_keys.created_at,
    users.username AS owner_username,
    users.identity_source AS owner_identity_source,
    users.status AS owner_status
FROM api_keys
JOIN users ON users.id = api_keys.user_id
ORDER BY api_keys.id DESC;
