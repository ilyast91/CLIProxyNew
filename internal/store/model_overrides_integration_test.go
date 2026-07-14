package store

import (
	"context"
	"errors"
	"testing"
)

func TestIntegrationModelOverrideRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()
	repository := NewModelOverrideRepository(pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM model_overrides WHERE model_alias = $1", "business-gpt"); err != nil {
			t.Errorf("удалить test model override: %v", err)
		}
	})
	if _, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider: "openai", ModelAlias: "invalid-config", UpstreamModel: "gpt-5", Config: []byte("not-json"),
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Upsert(invalid config) error = %v, want ErrInvalidInput", err)
	}

	override, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider:      "openai",
		ModelAlias:    "business-gpt",
		UpstreamModel: "gpt-5",
		Enabled:       true,
		Config:        []byte(`{"reasoning_effort":"high"}`),
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if override.Provider != "openai" || override.ModelAlias != "business-gpt" || override.UpstreamModel != "gpt-5" || !override.Enabled {
		t.Fatalf("Upsert() = %+v", override)
	}

	updated, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider:      "openai",
		ModelAlias:    "business-gpt",
		UpstreamModel: "gpt-5-mini",
		Enabled:       false,
	})
	if err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}
	if updated.ID != override.ID || updated.UpstreamModel != "gpt-5-mini" || updated.Enabled {
		t.Fatalf("Upsert(update) = %+v", updated)
	}

	overrides, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(overrides) != 1 || overrides[0].ID != override.ID {
		t.Fatalf("List() = %+v", overrides)
	}

	if err := repository.Delete(ctx, override.ModelAlias); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := repository.Delete(ctx, override.ModelAlias); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(second) error = %v, want ErrNotFound", err)
	}
}
