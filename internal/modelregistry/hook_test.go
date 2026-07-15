package modelregistry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cliproxy "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
)

type snapshotStoreStub struct {
	provider string
	clientID string
	models   []byte

	deletedProvider string
	deletedClientID string
	replaceErr      error
	deleteErr       error
}

func (s *snapshotStoreStub) Replace(ctx context.Context, provider, clientID string, models []byte) error {
	s.provider = provider
	s.clientID = clientID
	s.models = append([]byte(nil), models...)
	return s.replaceErr
}

func (s *snapshotStoreStub) Delete(ctx context.Context, provider, clientID string) error {
	s.deletedProvider = provider
	s.deletedClientID = clientID
	return s.deleteErr
}

func TestHookStoresCompleteModelSnapshot(t *testing.T) {
	store := &snapshotStoreStub{}
	hook := New(store)

	hook.OnModelsRegistered(context.Background(), "openai", "account-1", []*cliproxy.ModelInfo{{
		ID: "gpt-5", DisplayName: "GPT-5", ContextLength: 400000,
	}})

	if store.provider != "openai" || store.clientID != "account-1" {
		t.Fatalf("Replace() identity = %q/%q, want openai/account-1", store.provider, store.clientID)
	}
	var models []map[string]any
	if err := json.Unmarshal(store.models, &models); err != nil {
		t.Fatalf("snapshot is not JSON: %v", err)
	}
	if len(models) != 1 || models[0]["id"] != "gpt-5" || models[0]["context_length"] != float64(400000) {
		t.Fatalf("snapshot = %s", store.models)
	}
}

func TestHookDeletesSnapshotWhenModelsAreUnregistered(t *testing.T) {
	store := &snapshotStoreStub{}
	hook := New(store)

	hook.OnModelsUnregistered(context.Background(), "anthropic", "account-2")

	if store.deletedProvider != "anthropic" || store.deletedClientID != "account-2" {
		t.Fatalf("Delete() identity = %q/%q, want anthropic/account-2", store.deletedProvider, store.deletedClientID)
	}
}

func TestHookSkipsInvalidIdentityAndStoreErrors(t *testing.T) {
	store := &snapshotStoreStub{replaceErr: errors.New("database unavailable"), deleteErr: errors.New("database unavailable")}
	hook := New(store)

	hook.OnModelsRegistered(context.Background(), "", "account-1", nil)
	hook.OnModelsUnregistered(context.Background(), "openai", "")
	hook.OnModelsRegistered(context.Background(), "openai", "account-1", nil)
	hook.OnModelsUnregistered(context.Background(), "openai", "account-1")

	if store.provider != "openai" || store.clientID != "account-1" || store.deletedProvider != "openai" || store.deletedClientID != "account-1" {
		t.Fatalf("hook did not continue after store errors: %+v", store)
	}
}
