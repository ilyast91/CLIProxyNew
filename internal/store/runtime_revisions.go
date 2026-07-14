package store

import (
	"context"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
)

const UpstreamAccountsRevision = "upstream_accounts"

// RuntimeRevisionRepository синхронизирует controlled restart между репликами.
type RuntimeRevisionRepository struct{ queries *dbgen.Queries }

// NewRuntimeRevisionRepository создаёт репозиторий runtime revisions.
func NewRuntimeRevisionRepository(db dbgen.DBTX) *RuntimeRevisionRepository {
	return &RuntimeRevisionRepository{queries: dbgen.New(db)}
}

// Get возвращает текущую ревизию named runtime state.
func (r *RuntimeRevisionRepository) Get(ctx context.Context, name string) (int64, error) {
	value, err := r.queries.GetRuntimeRevision(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("get runtime revision: %w", err)
	}
	return value, nil
}

// Increment помечает изменение runtime state.
func (r *RuntimeRevisionRepository) Increment(ctx context.Context, name string) (int64, error) {
	value, err := r.queries.IncrementRuntimeRevision(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("increment runtime revision: %w", err)
	}
	return value, nil
}
