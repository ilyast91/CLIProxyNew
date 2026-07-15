package store

import "context"

type upstreamAccountAuditContextKey struct{}

// WithUpstreamAccountAudit добавляет audit-запись для атомарного сохранения credential.
func WithUpstreamAccountAudit(ctx context.Context, entry AdminAuditLogEntry) context.Context {
	return context.WithValue(ctx, upstreamAccountAuditContextKey{}, entry)
}

func upstreamAccountAuditFromContext(ctx context.Context) (AdminAuditLogEntry, bool) {
	entry, ok := ctx.Value(upstreamAccountAuditContextKey{}).(AdminAuditLogEntry)
	return entry, ok
}
