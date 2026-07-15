package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
)

// AdminModelRepository выполняет mutating model override операции с audit log.
type AdminModelRepository struct{ db dbgen.DBTX }

// NewAdminModelRepository создаёт admin репозиторий моделей.
func NewAdminModelRepository(db dbgen.DBTX) *AdminModelRepository {
	return &AdminModelRepository{db: db}
}

// List возвращает текущие model overrides.
func (r *AdminModelRepository) List(ctx context.Context) ([]ModelOverride, error) {
	return NewModelOverrideRepository(r.db).List(ctx)
}

// UpsertWithAudit сохраняет override и audit-запись в одной транзакции.
func (r *AdminModelRepository) UpsertWithAudit(ctx context.Context, actor int64, p UpsertModelOverrideParams, ip *netip.Addr) (ModelOverride, error) {
	if r == nil || actor <= 0 || !validModelOverrideParams(p) {
		return ModelOverride{}, ErrInvalidInput
	}
	b, ok := r.db.(transactionBeginner)
	if !ok {
		return ModelOverride{}, fmt.Errorf("admin model repository requires transactional database")
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return ModelOverride{}, err
	}
	defer tx.Rollback(ctx)
	v, err := NewModelOverrideRepository(tx).Upsert(ctx, p)
	if err != nil {
		return ModelOverride{}, err
	}
	d, _ := json.Marshal(map[string]string{"provider": p.Provider, "upstream_model": p.UpstreamModel})
	if err := NewAdminAuditLogRepository(tx).Insert(ctx, AdminAuditLogEntry{ActorUserID: actor, Action: "model_override.upserted", TargetType: "model_override", TargetID: p.ModelAlias, Details: d, ActorIP: ip}); err != nil {
		return ModelOverride{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ModelOverride{}, err
	}
	return v, nil
}
