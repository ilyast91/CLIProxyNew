package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
)

// ModelRegistrySnapshotRepository хранит снимки публичного реестра моделей SDK.
type ModelRegistrySnapshotRepository struct {
	queries *dbgen.Queries
}

// NewModelRegistrySnapshotRepository создаёт репозиторий снимков реестра моделей.
func NewModelRegistrySnapshotRepository(db dbgen.DBTX) *ModelRegistrySnapshotRepository {
	return &ModelRegistrySnapshotRepository{queries: dbgen.New(db)}
}

// Replace атомарно заменяет полный snapshot моделей одного upstream-клиента.
func (r *ModelRegistrySnapshotRepository) Replace(ctx context.Context, provider, clientID string, models []byte) error {
	if provider == "" || clientID == "" || !json.Valid(models) {
		return ErrInvalidInput
	}
	if err := r.queries.UpsertModelRegistrySnapshot(ctx, dbgen.UpsertModelRegistrySnapshotParams{
		Provider: provider,
		ClientID: clientID,
		Models:   models,
	}); err != nil {
		return fmt.Errorf("upsert model registry snapshot: %w", err)
	}
	return nil
}

// List возвращает снимки моделей в стабильном порядке provider/client.
func (r *ModelRegistrySnapshotRepository) List(ctx context.Context) ([]ModelRegistrySnapshot, error) {
	rows, err := r.queries.ListModelRegistrySnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model registry snapshots: %w", err)
	}

	snapshots := make([]ModelRegistrySnapshot, 0, len(rows))
	for _, row := range rows {
		snapshots = append(snapshots, ModelRegistrySnapshot{
			Provider:  row.Provider,
			ClientID:  row.ClientID,
			Models:    append([]byte(nil), row.Models...),
			UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return snapshots, nil
}

// Delete удаляет snapshot моделей отключённого upstream-клиента.
func (r *ModelRegistrySnapshotRepository) Delete(ctx context.Context, provider, clientID string) error {
	if provider == "" || clientID == "" {
		return ErrInvalidInput
	}
	deleted, err := r.queries.DeleteModelRegistrySnapshot(ctx, dbgen.DeleteModelRegistrySnapshotParams{
		Provider: provider,
		ClientID: clientID,
	})
	if err != nil {
		return fmt.Errorf("delete model registry snapshot: %w", err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}
