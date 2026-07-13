-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, role, expires_at, created_ip)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(token_hash),
    sqlc.arg(role),
    sqlc.narg(expires_at),
    sqlc.narg(created_ip)
)
RETURNING id, user_id, token_hash, role, expires_at, created_ip, created_at;

-- name: GetSessionByTokenHash :one
SELECT
    sessions.id,
    sessions.user_id,
    sessions.token_hash,
    sessions.role,
    sessions.expires_at,
    sessions.created_ip,
    sessions.created_at,
    users.status AS user_status
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = $1
  AND sessions.expires_at > now();

-- name: DeleteSessionsByUser :exec
DELETE FROM sessions
WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= now();

