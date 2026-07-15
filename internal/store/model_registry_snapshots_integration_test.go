package store

import (
	"context"
	"errors"
	"testing"
)

func TestIntegrationModelRegistrySnapshotRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()
	repository := NewModelRegistrySnapshotRepository(pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM model_registry_snapshots WHERE provider = $1 AND client_id = $2", "openai", "account-1"); err != nil {
			t.Errorf("удалить test model registry snapshot: %v", err)
		}
	})

	if err := repository.Replace(ctx, "openai", "account-1", []byte("not-json")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Replace(invalid JSON) error = %v, want ErrInvalidInput", err)
	}
	if err := repository.Replace(ctx, "openai", "account-1", []byte(`[{"id":"gpt-5"}]`)); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if err := repository.Replace(ctx, "openai", "account-1", []byte(`[{"id":"gpt-5-mini"}]`)); err != nil {
		t.Fatalf("Replace(update) error = %v", err)
	}

	snapshots, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Provider != "openai" || snapshots[0].ClientID != "account-1" || string(snapshots[0].Models) != `[{"id": "gpt-5-mini"}]` {
		t.Fatalf("List() = %+v", snapshots)
	}

	if err := repository.Delete(ctx, "openai", "account-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := repository.Delete(ctx, "openai", "account-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(second) error = %v, want ErrNotFound", err)
	}
}
