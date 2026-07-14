package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5/pgtype"
)

// AdminAuditLogRepository записывает append-only аудит действий администратора.
type AdminAuditLogRepository struct {
	queries *dbgen.Queries
}

// NewAdminAuditLogRepository создаёт репозиторий audit log.
func NewAdminAuditLogRepository(db dbgen.DBTX) *AdminAuditLogRepository {
	return &AdminAuditLogRepository{queries: dbgen.New(db)}
}

// Insert добавляет одну audit-запись без секретных данных.
func (r *AdminAuditLogRepository) Insert(ctx context.Context, entry AdminAuditLogEntry) error {
	if r == nil || r.queries == nil || entry.ActorUserID <= 0 || entry.Action == "" || entry.TargetType == "" || entry.TargetID == "" || (len(entry.Details) > 0 && !json.Valid(entry.Details)) {
		return ErrInvalidInput
	}
	if err := r.queries.InsertAdminAuditLog(ctx, dbgen.InsertAdminAuditLogParams{
		ActorUserID: pgtype.Int8{Int64: entry.ActorUserID, Valid: true},
		Action:      entry.Action,
		TargetType:  pgtype.Text{String: entry.TargetType, Valid: true},
		TargetID:    pgtype.Text{String: entry.TargetID, Valid: true},
		Details:     append([]byte(nil), entry.Details...),
		ActorIp:     entry.ActorIP,
	}); err != nil {
		return fmt.Errorf("insert admin audit log: %w", err)
	}
	return nil
}
