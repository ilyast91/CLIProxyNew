package store

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationOAuthSessionRepositoryLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}
	r := NewOAuthSessionRepository(newTestPool(t))
	ctx := context.Background()
	if err := r.Create(ctx, CreateOAuthSessionParams{State: "state-1", Provider: "claude", FlowType: "callback", PKCEVerifier: "verifier", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	pending, err := r.ListPending(ctx)
	if err != nil || len(pending) != 1 || pending[0].PKCEVerifier != "verifier" {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if err := r.Cancel(ctx, "state-1"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "state-1")
	if err != nil || got.Status != "cancelled" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
