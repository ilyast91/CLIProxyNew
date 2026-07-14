-- name: GetRuntimeRevision :one
SELECT revision
FROM runtime_revisions
WHERE name = $1;

-- name: IncrementRuntimeRevision :one
UPDATE runtime_revisions
SET revision = revision + 1, updated_at = now()
WHERE name = $1
RETURNING revision;
