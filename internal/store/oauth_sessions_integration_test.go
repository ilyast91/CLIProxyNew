package store

import (
	"context"
	"encoding/json"
	"net/netip"
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

func TestIntegrationOAuthSessionRepositoryCancelWritesAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx := context.Background()
	pool := newTestPool(t)
	admin, err := NewUserRepository(pool).UpsertFromLDAP(ctx, UpsertUserParams{Username: "oauth-audit-admin", Role: "admin"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	repository := NewOAuthSessionRepository(pool)
	const state = "audit-session"
	if err := repository.Create(ctx, CreateOAuthSessionParams{State: state, Provider: "claude", FlowType: "callback", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	address := netip.MustParseAddr("192.0.2.31")
	if err := repository.CancelWithAudit(ctx, admin.ID, state, &address); err != nil {
		t.Fatalf("CancelWithAudit: %v", err)
	}

	session, err := repository.Get(ctx, state)
	if err != nil || session.Status != "cancelled" {
		t.Fatalf("session = %+v, %v", session, err)
	}
	var action, targetType, targetID string
	var details []byte
	if err := pool.QueryRow(ctx, "SELECT action, target_type, target_id, details FROM admin_audit_log").Scan(&action, &targetType, &targetID, &details); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	var detailValues map[string]string
	if err := json.Unmarshal(details, &detailValues); err != nil {
		t.Fatalf("decode audit details: %v", err)
	}
	if action != "oauth.session.cancelled" || targetType != "oauth_session" || targetID != state || detailValues["status"] != "cancelled" {
		t.Fatalf("audit = action %q, target %q/%q, details %v", action, targetType, targetID, detailValues)
	}
}
