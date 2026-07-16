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
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE username = $1", "static:debug"); err != nil {
			t.Errorf("удалить static test user: %v", err)
		}
	})

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
	if user.Username != "ivanov" || user.Status != "active" || user.IdentitySource != "ldap" {
		t.Fatalf("UpsertFromLDAP() user = %+v", user)
	}
	if _, err := users.UpsertFromLDAP(ctx, UpsertUserParams{Username: "static:debug", Role: "user"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpsertFromLDAP(static namespace) error = %v, want ErrInvalidInput", err)
	}

	staticUser, err := users.UpsertStatic(ctx, UpsertUserParams{
		Username: "static:debug",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("UpsertStatic() error = %v", err)
	}
	if staticUser.IdentitySource != "static" || staticUser.Username != "static:debug" {
		t.Fatalf("UpsertStatic() user = %+v", staticUser)
	}
	if _, err := users.UpsertStatic(ctx, UpsertUserParams{Username: "debug", Role: "user"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpsertStatic(non-static namespace) error = %v, want ErrInvalidInput", err)
	}
	const staticKey = "cpn_test_static-key"
	if _, err := apiKeys.Create(ctx, CreateAPIKeyParams{UserID: staticUser.ID, Plaintext: staticKey}); err != nil {
		t.Fatalf("Create(static API key) error = %v", err)
	}
	if _, err := apiKeys.AuthenticateForSource(ctx, staticKey, "ldap"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("AuthenticateForSource(static key, ldap) error = %v, want ErrInvalidCredential", err)
	}
	if _, err := apiKeys.AuthenticateForSource(ctx, staticKey, "static"); err != nil {
		t.Fatalf("AuthenticateForSource(static key, static) error = %v", err)
	}

	const staticToken = "static-opaque-session-token"
	if _, err := sessions.Create(ctx, CreateSessionParams{
		UserID:    staticUser.ID,
		Token:     staticToken,
		Role:      "admin",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Create(static session) error = %v", err)
	}
	if _, err := sessions.GetByTokenForSource(ctx, staticToken, "ldap"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("GetByTokenForSource(static token, ldap) error = %v, want ErrInvalidCredential", err)
	}
	if _, err := sessions.GetByTokenForSource(ctx, staticToken, "static"); err != nil {
		t.Fatalf("GetByTokenForSource(static token, static) error = %v", err)
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
	apiKeys.InvalidateUser(user.ID)
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
	allKeys, err := apiKeys.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	foundKey := false
	for _, listed := range allKeys {
		if listed.ID == key.ID && listed.OwnerUsername == user.Username && listed.OwnerIdentitySource == "ldap" && listed.OwnerStatus == "active" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Fatalf("ListAll() does not contain expected owner metadata: %+v", allKeys)
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
