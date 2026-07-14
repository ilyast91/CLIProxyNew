-- name: InsertAdminAuditLog :exec
INSERT INTO admin_audit_log (actor_user_id, action, target_type, target_id, details, actor_ip)
VALUES (
    sqlc.arg(actor_user_id),
    sqlc.arg(action),
    sqlc.arg(target_type),
    sqlc.arg(target_id),
    sqlc.narg(details),
    sqlc.narg(actor_ip)
);
