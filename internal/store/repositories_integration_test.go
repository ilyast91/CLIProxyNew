package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIntegrationUserAPIKeyAndSessionRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()

	users := NewUserRepository(pool)
	apiKeys := NewAPIKeyRepository(pool)
	sessions := NewSessionRepository(pool)

	user, err := users.UpsertFromLDAP(ctx, UpsertUserParams{
		Username: "ivanov",
		Email:    "ivanov@example.com",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("UpsertFromLDAP() error = %v", err)
	}
	if user.Username != "ivanov" || user.Status != "active" {
		t.Fatalf("UpsertFromLDAP() user = %+v", user)
	}

	const plaintextKey = "cpn_live_0123456789abcdef"
	key, err := apiKeys.Create(ctx, CreateAPIKeyParams{
		UserID:    user.ID,
		Plaintext: plaintextKey,
		Name:      "integration",
	})
	if err != nil {
		t.Fatalf("APIKeyRepository.Create() error = %v", err)
	}
	if key.Prefix != plaintextKey[:APIKeyPrefixLength] {
		t.Fatalf("Create() prefix = %q", key.Prefix)
	}

	var storedHash string
	if err := pool.QueryRow(ctx, "SELECT key_hash FROM api_keys WHERE id = $1", key.ID).Scan(&storedHash); err != nil {
		t.Fatalf("прочитать key_hash: %v", err)
	}
	if storedHash == plaintextKey {
		t.Fatal("API-key сохранён в открытом виде")
	}

	principal, err := apiKeys.Authenticate(ctx, plaintextKey)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.UserID != user.ID || principal.APIKeyID != key.ID {
		t.Fatalf("Authenticate() principal = %+v", principal)
	}
	if _, err := apiKeys.Authenticate(ctx, "cpn_live_wrong-secret"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("Authenticate(wrong) error = %v, want ErrInvalidCredential", err)
	}

	const token = "opaque-session-token"
	session, err := sessions.Create(ctx, CreateSessionParams{
		UserID:    user.ID,
		Token:     token,
		Role:      "user",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("SessionRepository.Create() error = %v", err)
	}
	if session.TokenHash == token {
		t.Fatal("session token сохранён в открытом виде")
	}

	gotSession, err := sessions.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByToken() error = %v", err)
	}
	if gotSession.ID != session.ID || gotSession.UserID != user.ID {
		t.Fatalf("GetByToken() session = %+v", gotSession)
	}

	if err := users.SetStatus(ctx, user.ID, "blocked"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if _, err := apiKeys.Authenticate(ctx, plaintextKey); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("Authenticate(blocked) error = %v, want ErrInvalidCredential", err)
	}
	if _, err := sessions.GetByToken(ctx, token); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("GetByToken(blocked) error = %v, want ErrInvalidCredential", err)
	}
	if err := users.SetStatus(ctx, user.ID, "active"); err != nil {
		t.Fatalf("SetStatus(active) error = %v", err)
	}
	if _, err := sessions.GetByToken(ctx, token); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("GetByToken(after unblock) error = %v, want ErrInvalidCredential", err)
	}

	listedKeys, err := apiKeys.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(listedKeys) != 1 || listedKeys[0].ID != key.ID {
		t.Fatalf("ListByUser() = %+v", listedKeys)
	}
	if err := apiKeys.Revoke(ctx, user.ID, key.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := apiKeys.Authenticate(ctx, plaintextKey); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("Authenticate(revoked) error = %v, want ErrInvalidCredential", err)
	}
	if err := apiKeys.Revoke(ctx, user.ID, key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Revoke(second) error = %v, want ErrNotFound", err)
	}
}
