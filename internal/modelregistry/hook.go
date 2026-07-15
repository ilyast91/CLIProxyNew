package modelregistry

import (
	"context"
	"encoding/json"
	"log/slog"

	cliproxy "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
)

// SnapshotStore сохраняет снимок моделей одного upstream-клиента.
type SnapshotStore interface {
	Replace(ctx context.Context, provider, clientID string, models []byte) error
	Delete(ctx context.Context, provider, clientID string) error
}

// Hook зеркалирует публичный реестр моделей SDK в persistence-слой.
type Hook struct {
	store SnapshotStore
}

var _ cliproxy.ModelRegistryHook = (*Hook)(nil)

// New создаёт hook для зеркалирования моделей в store.
func New(store SnapshotStore) *Hook {
	return &Hook{store: store}
}

// OnModelsRegistered сохраняет полный snapshot моделей клиента.
func (h *Hook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*cliproxy.ModelInfo) {
	if !validIdentity(provider, clientID) || h.store == nil {
		return
	}

	snapshot, err := json.Marshal(models)
	if err != nil {
		slog.Error("marshal model registry snapshot", "provider", provider, "client_id", clientID, "error", err)
		return
	}
	if err := h.store.Replace(ctx, provider, clientID, snapshot); err != nil {
		slog.Error("store model registry snapshot", "provider", provider, "client_id", clientID, "error", err)
	}
}

// OnModelsUnregistered удаляет snapshot моделей отключённого клиента.
func (h *Hook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {
	if !validIdentity(provider, clientID) || h.store == nil {
		return
	}
	if err := h.store.Delete(ctx, provider, clientID); err != nil {
		slog.Error("delete model registry snapshot", "provider", provider, "client_id", clientID, "error", err)
	}
}

func validIdentity(provider, clientID string) bool {
	return provider != "" && clientID != ""
}
