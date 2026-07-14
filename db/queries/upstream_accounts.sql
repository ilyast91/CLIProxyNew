-- name: ListUpstreamAccounts :many
SELECT
    id,
    provider,
    email,
    auth_type,
    label,
    credentials_enc,
    enc_key_version,
    attributes,
    status,
    last_refreshed_at,
    next_refresh_after,
    created_at
FROM upstream_accounts
ORDER BY id;

-- name: UpsertUpstreamAccount :one
INSERT INTO upstream_accounts (
    id,
    provider,
    email,
    auth_type,
    label,
    credentials_enc,
    enc_key_version,
    attributes,
    status,
    last_refreshed_at,
    next_refresh_after
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(provider),
    sqlc.arg(email),
    sqlc.arg(auth_type),
    sqlc.narg(label),
    sqlc.arg(credentials_enc),
    sqlc.arg(enc_key_version),
    sqlc.narg(attributes),
    sqlc.arg(status),
    sqlc.narg(last_refreshed_at),
    sqlc.narg(next_refresh_after)
)
ON CONFLICT (id) DO UPDATE SET
    provider = EXCLUDED.provider,
    email = EXCLUDED.email,
    auth_type = EXCLUDED.auth_type,
    label = EXCLUDED.label,
    credentials_enc = EXCLUDED.credentials_enc,
    enc_key_version = EXCLUDED.enc_key_version,
    attributes = EXCLUDED.attributes,
    status = EXCLUDED.status,
    last_refreshed_at = EXCLUDED.last_refreshed_at,
    next_refresh_after = EXCLUDED.next_refresh_after
RETURNING id;

-- name: DeleteUpstreamAccount :exec
DELETE FROM upstream_accounts
WHERE id = $1;
