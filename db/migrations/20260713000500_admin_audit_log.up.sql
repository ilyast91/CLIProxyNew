CREATE TABLE admin_audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id bigint REFERENCES users(id),
    action text NOT NULL,
    target_type text,
    target_id text,
    details jsonb,
    actor_ip inet,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_log_actor_created_idx
    ON admin_audit_log (actor_user_id, created_at DESC);
CREATE INDEX admin_audit_log_target_idx
    ON admin_audit_log (target_type, target_id);

