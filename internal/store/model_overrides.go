package store

import (
	"context"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
)

// ModelOverrideRepository хранит allow-list и desired mapping клиентских моделей.
type ModelOverrideRepository struct {
	queries *dbgen.Queries
}

// NewModelOverrideRepository создаёт репозиторий model overrides.
func NewModelOverrideRepository(db dbgen.DBTX) *ModelOverrideRepository {
	return &ModelOverrideRepository{queries: dbgen.New(db)}
}

// Upsert создаёт override либо обновляет существующий по model alias.
func (r *ModelOverrideRepository) Upsert(ctx context.Context, params UpsertModelOverrideParams) (ModelOverride, error) {
	if !validModelOverrideParams(params) {
		return ModelOverride{}, ErrInvalidInput
	}

	row, err := r.queries.UpsertModelOverride(ctx, dbgen.UpsertModelOverrideParams{
		Provider:      params.Provider,
		ModelAlias:    params.ModelAlias,
		UpstreamModel: params.UpstreamModel,
		Enabled:       params.Enabled,
		Config:        params.Config,
	})
	if err != nil {
		return ModelOverride{}, fmt.Errorf("upsert model override: %w", err)
	}
	return modelOverrideFromDB(row), nil
}

// List возвращает все model overrides в стабильном порядке alias.
func (r *ModelOverrideRepository) List(ctx context.Context) ([]ModelOverride, error) {
	rows, err := r.queries.ListModelOverrides(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model overrides: %w", err)
	}

	overrides := make([]ModelOverride, 0, len(rows))
	for _, row := range rows {
		overrides = append(overrides, modelOverrideFromDB(row))
	}
	return overrides, nil
}

// Delete удаляет override по model alias.
func (r *ModelOverrideRepository) Delete(ctx context.Context, modelAlias string) error {
	if modelAlias == "" {
		return ErrInvalidInput
	}

	deleted, err := r.queries.DeleteModelOverride(ctx, modelAlias)
	if err != nil {
		return fmt.Errorf("delete model override: %w", err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

func modelOverrideFromDB(row dbgen.ModelOverride) ModelOverride {
	return ModelOverride{
		ID:            row.ID,
		Provider:      row.Provider,
		ModelAlias:    row.ModelAlias,
		UpstreamModel: row.UpstreamModel,
		Enabled:       row.Enabled,
		Config:        append([]byte(nil), row.Config...),
	}
}
