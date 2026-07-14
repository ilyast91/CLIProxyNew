-- name: UpsertUserFromLDAP :one
INSERT INTO users (username, email, role, identity_source)
VALUES (sqlc.arg(username), sqlc.arg(email), sqlc.arg(role), 'ldap')
ON CONFLICT (username) DO UPDATE SET
    email = EXCLUDED.email,
    role = EXCLUDED.role,
    updated_at = now()
RETURNING id, username, email, role, status, identity_source, created_at, updated_at;

-- name: UpsertStaticUser :one
INSERT INTO users (username, email, role, identity_source)
VALUES (sqlc.arg(username), sqlc.arg(email), sqlc.arg(role), 'static')
ON CONFLICT (username) DO UPDATE SET
    email = EXCLUDED.email,
    role = EXCLUDED.role,
    updated_at = now()
RETURNING id, username, email, role, status, identity_source, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, username, email, role, status, identity_source, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT id, username, email, role, status, identity_source, created_at, updated_at
FROM users
WHERE username = $1;

-- name: ListUsers :many
SELECT id, username, email, role, status, identity_source, created_at, updated_at
FROM users
ORDER BY id DESC;

-- name: SetUserStatus :execrows
UPDATE users
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: BlockUserAndDeleteSessions :one
WITH blocked AS (
    UPDATE users
    SET status = 'blocked', updated_at = now()
    WHERE users.id = $1
    RETURNING users.id
), deleted_sessions AS (
    DELETE FROM sessions
    WHERE sessions.user_id IN (SELECT blocked.id FROM blocked)
    RETURNING sessions.id
)
SELECT count(*) FROM blocked;
