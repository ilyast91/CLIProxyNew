package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
)

type transactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// AdminUserRepository выполняет admin-операции с пользователями и audit log.
type AdminUserRepository struct {
	db    dbgen.DBTX
	users *UserRepository
}

// NewAdminUserRepository создаёт репозиторий admin-операций с пользователями.
func NewAdminUserRepository(db dbgen.DBTX) *AdminUserRepository {
	return &AdminUserRepository{db: db, users: NewUserRepository(db)}
}

// List возвращает список пользователей для администратора.
func (r *AdminUserRepository) List(ctx context.Context) ([]User, error) {
	if r == nil || r.users == nil {
		return nil, ErrInvalidInput
	}
	return r.users.List(ctx)
}

// SetStatusWithAudit меняет статус и пишет audit-запись в одной транзакции.
func (r *AdminUserRepository) SetStatusWithAudit(ctx context.Context, actorUserID, targetUserID int64, status string, actorIP *netip.Addr) error {
	if r == nil || actorUserID <= 0 || targetUserID <= 0 || (status != "active" && status != "blocked") {
		return ErrInvalidInput
	}
	beginner, ok := r.db.(transactionBeginner)
	if !ok {
		return fmt.Errorf("admin user repository requires transactional database")
	}
	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin admin user transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := NewUserRepository(tx).SetStatus(ctx, targetUserID, status); err != nil {
		return err
	}
	details, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return fmt.Errorf("marshal status audit details: %w", err)
	}
	if err := NewAdminAuditLogRepository(tx).Insert(ctx, AdminAuditLogEntry{
		ActorUserID: actorUserID,
		Action:      "user.status.changed", TargetType: "user", TargetID: strconv.FormatInt(targetUserID, 10),
		Details: details, ActorIP: actorIP,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit admin user transaction: %w", err)
	}
	return nil
}
