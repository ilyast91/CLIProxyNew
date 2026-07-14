package store

import (
	"context"
	"encoding/json"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

func TestIntegrationAdminUserRepositorySetStatusWithAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUserRepository(pool)
	admin, err := users.UpsertFromLDAP(ctx, UpsertUserParams{Username: "admin-user", Role: "admin"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	target, err := users.UpsertFromLDAP(ctx, UpsertUserParams{Username: "target-user", Role: "user"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if _, err := NewSessionRepository(pool).Create(ctx, CreateSessionParams{
		UserID: target.ID, Token: "target-session", Role: "user", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create target session: %v", err)
	}

	repository := NewAdminUserRepository(pool)
	listed, err := repository.List(ctx)
	if err != nil || len(listed) != 2 {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	address := netip.MustParseAddr("192.0.2.10")
	if err := repository.SetStatusWithAudit(ctx, admin.ID, target.ID, "blocked", &address); err != nil {
		t.Fatalf("SetStatusWithAudit() error = %v", err)
	}

	got, err := users.GetByID(ctx, target.ID)
	if err != nil || got.Status != "blocked" {
		t.Fatalf("target = %+v, %v", got, err)
	}
	if _, err := NewSessionRepository(pool).GetByToken(ctx, "target-session"); err != ErrInvalidCredential {
		t.Fatalf("blocked session lookup error = %v, want ErrInvalidCredential", err)
	}
	var action, targetType, targetID string
	var details []byte
	if err := pool.QueryRow(ctx, "SELECT action, target_type, target_id, details FROM admin_audit_log").Scan(&action, &targetType, &targetID, &details); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	var auditDetails map[string]string
	if err := json.Unmarshal(details, &auditDetails); err != nil {
		t.Fatalf("decode audit details: %v", err)
	}
	if action != "user.status.changed" || targetType != "user" || targetID != strconv.FormatInt(target.ID, 10) || auditDetails["status"] != "blocked" {
		t.Fatalf("audit = action %q, target %q/%q, details %s", action, targetType, targetID, details)
	}
	if err := repository.SetStatusWithAudit(ctx, admin.ID, target.ID, "invalid", &address); err != ErrInvalidInput {
		t.Fatalf("SetStatusWithAudit(invalid) error = %v, want ErrInvalidInput", err)
	}
}
