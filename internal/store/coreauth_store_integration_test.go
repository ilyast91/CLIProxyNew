package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/jackc/pgx/v5"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var _ coreauth.Store = (*CoreAuthStore)(nil)

func TestIntegrationCoreAuthStoreContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()
	keyV1 := bytes.Repeat([]byte{1}, security.AES256KeySize)
	keyV2 := bytes.Repeat([]byte{2}, security.AES256KeySize)

	keyringV1, err := security.NewKeyring(1, map[int][]byte{1: keyV1})
	if err != nil {
		t.Fatalf("NewKeyring(v1) error = %v", err)
	}
	authStore := NewCoreAuthStore(pool, keyringV1)
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT revision FROM runtime_revisions WHERE name = $1", UpstreamAccountsRevision).Scan(&revision); err != nil {
		t.Fatalf("прочитать initial runtime revision: %v", err)
	}

	auth := &coreauth.Auth{
		ID:        "auth-codex-user",
		Provider:  "codex",
		Label:     "Main account",
		Status:    coreauth.StatusActive,
		ProxyURL:  "socks5://temporary-quota-proxy:1080",
		CreatedAt: time.Now().Add(-time.Hour).UTC(),
		UpdatedAt: time.Now().UTC(),
		Attributes: map[string]string{
			"provider_secret": "must-not-leak",
			"base_url":        "https://api.example.com",
			"compat_name":     "example",
		},
		Metadata: map[string]any{},
		Storage: &testTokenStorage{
			Type:         "codex",
			Email:        "user@example.com",
			AccessToken:  "access-token-secret",
			RefreshToken: "refresh-token-v1",
		},
	}

	id, err := authStore.Save(ctx, auth)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if id != auth.ID {
		t.Fatalf("Save() id = %q, want %q", id, auth.ID)
	}
	assertRuntimeRevision(t, ctx, pool, revision+1)

	var (
		ciphertext []byte
		keyVersion int
		attributes []byte
		authType   string
		email      string
	)
	if err := pool.QueryRow(ctx, `
		SELECT credentials_enc, enc_key_version, attributes, auth_type, email
		FROM upstream_accounts WHERE id = $1`, auth.ID,
	).Scan(&ciphertext, &keyVersion, &attributes, &authType, &email); err != nil {
		t.Fatalf("прочитать upstream_accounts: %v", err)
	}
	if authType != "oauth" {
		t.Fatalf("auth_type = %q, want oauth", authType)
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q, want user@example.com", email)
	}
	if keyVersion != 1 {
		t.Fatalf("enc_key_version = %d, want 1", keyVersion)
	}
	if bytes.Contains(ciphertext, []byte("access-token-secret")) || bytes.Contains(ciphertext, []byte("refresh-token-v1")) {
		t.Fatal("credentials_enc содержит plaintext token")
	}
	if bytes.Contains(attributes, []byte("must-not-leak")) || bytes.Contains(attributes, []byte("provider_secret")) {
		t.Fatalf("attributes содержит credential: %s", attributes)
	}
	if !bytes.Contains(attributes, []byte("base_url")) {
		t.Fatalf("attributes не содержит routing metadata: %s", attributes)
	}
	persistedJSON, err := keyringV1.Decrypt(security.EncryptedValue{KeyVersion: keyVersion, Ciphertext: ciphertext})
	if err != nil {
		t.Fatalf("decrypt persisted credential: %v", err)
	}
	var persistedAuth coreauth.Auth
	if err := json.Unmarshal(persistedJSON, &persistedAuth); err != nil {
		t.Fatalf("decode persisted credential: %v", err)
	}
	if persistedAuth.ProxyURL != "" {
		t.Fatalf("persisted ProxyURL = %q, want empty system-proxy mode", persistedAuth.ProxyURL)
	}
	loaded, err := authStore.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("List() count = %d, want 1", len(loaded))
	}
	if loaded[0].ProxyURL != "" {
		t.Fatalf("List() ProxyURL = %q, want empty system-proxy mode", loaded[0].ProxyURL)
	}
	if loaded[0].Metadata["refresh_token"] != "refresh-token-v1" {
		t.Fatalf("List() metadata = %+v", loaded[0].Metadata)
	}

	keyringV2, err := security.NewKeyring(2, map[int][]byte{1: keyV1, 2: keyV2})
	if err != nil {
		t.Fatalf("NewKeyring(v2) error = %v", err)
	}
	rotatedStore := NewCoreAuthStore(pool, keyringV2)
	if _, err := rotatedStore.List(ctx); err != nil {
		t.Fatalf("List(v1 through rotated keyring) error = %v", err)
	}

	auth.Metadata["refresh_token"] = "refresh-token-v2"
	auth.LastRefreshedAt = time.Now().UTC()
	if _, err := rotatedStore.Save(ctx, auth); err != nil {
		t.Fatalf("Save(refresh) error = %v", err)
	}
	assertRuntimeRevision(t, ctx, pool, revision+2)

	var rowCount int
	if err := pool.QueryRow(ctx, "SELECT count(*), max(enc_key_version) FROM upstream_accounts WHERE id = $1", auth.ID).Scan(&rowCount, &keyVersion); err != nil {
		t.Fatalf("проверить upsert/key rotation: %v", err)
	}
	if rowCount != 1 || keyVersion != 2 {
		t.Fatalf("upstream row count/version = %d/%d, want 1/2", rowCount, keyVersion)
	}

	loaded, err = rotatedStore.List(ctx)
	if err != nil {
		t.Fatalf("List(after refresh) error = %v", err)
	}
	if got := loaded[0].Metadata["refresh_token"]; got != "refresh-token-v2" {
		t.Fatalf("refresh token = %v, want refresh-token-v2", got)
	}

	if err := rotatedStore.Delete(ctx, auth.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	assertRuntimeRevision(t, ctx, pool, revision+3)
	if err := rotatedStore.Delete(ctx, auth.ID); err != nil {
		t.Fatalf("Delete(second) error = %v", err)
	}
	assertRuntimeRevision(t, ctx, pool, revision+4)
	loaded, err = rotatedStore.List(ctx)
	if err != nil {
		t.Fatalf("List(after delete) error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("List(after delete) = %d records, want 0", len(loaded))
	}

	if _, err := rotatedStore.Save(ctx, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Save(nil) error = %v, want ErrInvalidInput", err)
	}

	overflowVersion := int(math.MaxInt32) + 1
	overflowKeyring, err := security.NewKeyring(overflowVersion, map[int][]byte{
		overflowVersion: keyV1,
	})
	if err != nil {
		t.Fatalf("NewKeyring(overflow) error = %v", err)
	}
	overflowStore := NewCoreAuthStore(pool, overflowKeyring)
	if _, err := overflowStore.Save(ctx, auth); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Save(key version overflow) error = %v, want ErrInvalidInput", err)
	}
}

func assertRuntimeRevision(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, "SELECT revision FROM runtime_revisions WHERE name = $1", UpstreamAccountsRevision).Scan(&got); err != nil || got != want {
		t.Fatalf("runtime revision = %d, %v; want %d", got, err, want)
	}
}

func TestCoreAuthStoreRejectsNilDB(t *testing.T) {
	keyring, err := security.NewKeyring(1, map[int][]byte{
		1: bytes.Repeat([]byte{1}, security.AES256KeySize),
	})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}

	authStore := NewCoreAuthStore(nil, keyring)
	if _, err := authStore.List(context.Background()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List(nil DB) error = %v, want ErrInvalidInput", err)
	}
}

func TestIntegrationCoreAuthStoreSaveWritesUpstreamAccountAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx := context.Background()
	pool := newTestPool(t)
	admin, err := NewUserRepository(pool).UpsertFromLDAP(ctx, UpsertUserParams{Username: "audit-admin", Role: "admin"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	keyring, err := security.NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{1}, security.AES256KeySize)})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	authStore := NewCoreAuthStore(pool, keyring)
	details := []byte(`{"provider":"openai-compatibility","label":"primary","base_url":"https://example.com/v1"}`)
	auditCtx := WithUpstreamAccountAudit(ctx, AdminAuditLogEntry{
		ActorUserID: admin.ID,
		Action:      "upstream_api_key.created",
		TargetType:  "upstream_account",
		TargetID:    "pending",
		Details:     details,
	})
	auth := &coreauth.Auth{
		ID:       "sdk-assigned-id",
		Provider: "openai-compatibility",
		Label:    "primary",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
			coreauth.AttributeAPIKey:   "upstream-secret",
			"base_url":                 "https://example.com/v1",
		},
	}
	if _, err := authStore.Save(auditCtx, auth); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var action, targetType, targetID string
	var gotDetails []byte
	if err := pool.QueryRow(ctx, "SELECT action, target_type, target_id, details FROM admin_audit_log").Scan(&action, &targetType, &targetID, &gotDetails); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var gotDetailValues, wantDetailValues map[string]string
	if err := json.Unmarshal(gotDetails, &gotDetailValues); err != nil {
		t.Fatalf("decode audit details: %v", err)
	}
	if err := json.Unmarshal(details, &wantDetailValues); err != nil {
		t.Fatalf("decode expected audit details: %v", err)
	}
	if action != "upstream_api_key.created" || targetType != "upstream_account" || targetID != auth.ID || !mapsEqual(gotDetailValues, wantDetailValues) {
		t.Fatalf("audit = action=%q target=%q/%q details=%s", action, targetType, targetID, gotDetails)
	}
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}

type testTokenStorage struct {
	Type         string `json:"type"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (s *testTokenStorage) SaveTokenToFile(string) error {
	return errors.New("SaveTokenToFile не должен вызываться")
}

func TestTokenStorageMetadataRejectsHiddenState(t *testing.T) {
	storage := &hiddenTokenStorage{secret: "must-not-touch-filesystem"}
	if _, err := tokenStorageMetadata(storage); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("tokenStorageMetadata(hidden) error = %v, want ErrInvalidInput", err)
	}

	nested := &nestedHiddenTokenStorage{
		Credentials: nestedCredentials{
			AccessToken:  "visible-token",
			refreshToken: "hidden-token",
		},
	}
	if _, err := tokenStorageMetadata(nested); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("tokenStorageMetadata(nested hidden) error = %v, want ErrInvalidInput", err)
	}
}

type hiddenTokenStorage struct {
	secret string
}

func (*hiddenTokenStorage) SaveTokenToFile(string) error {
	return errors.New("SaveTokenToFile не должен вызываться")
}

type nestedHiddenTokenStorage struct {
	Credentials nestedCredentials `json:"credentials"`
}

type nestedCredentials struct {
	AccessToken  string `json:"access_token"`
	refreshToken string
}

func (*nestedHiddenTokenStorage) SaveTokenToFile(string) error {
	return errors.New("SaveTokenToFile не должен вызываться")
}
