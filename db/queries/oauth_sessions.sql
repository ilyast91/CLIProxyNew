-- name: CreateOAuthSession :exec
INSERT INTO oauth_sessions (state, provider, flow_type, pkce_verifier, device_code, user_code, expires_at)
VALUES ($1, $2, $3, sqlc.narg(pkce_verifier), sqlc.narg(device_code), sqlc.narg(user_code), $4);

-- name: GetOAuthSession :one
SELECT state, provider, flow_type, status, auth_id, pkce_verifier, device_code, user_code, error_message, expires_at, created_at, updated_at
FROM oauth_sessions WHERE state = $1;

-- name: ListPendingOAuthSessions :many
SELECT state, provider, flow_type, status, auth_id, pkce_verifier, device_code, user_code, error_message, expires_at, created_at, updated_at
FROM oauth_sessions WHERE status = 'pending' ORDER BY created_at DESC;

-- name: CompleteOAuthSession :execrows
UPDATE oauth_sessions SET status = 'completed', auth_id = $2, error_message = NULL, updated_at = now()
WHERE state = $1 AND status = 'pending';

-- name: FailOAuthSession :execrows
UPDATE oauth_sessions SET status = 'error', error_message = $2, updated_at = now()
WHERE state = $1 AND status = 'pending';

-- name: CancelOAuthSession :execrows
UPDATE oauth_sessions SET status = 'cancelled', updated_at = now()
WHERE state = $1 AND status = 'pending';
