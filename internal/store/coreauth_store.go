package store

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var _ coreauth.Store = (*CoreAuthStore)(nil)

var persistedAttributeAllowlist = map[string]struct{}{
	"base_url":        {},
	"compat_name":     {},
	"excluded_models": {},
}

// CoreAuthStore реализует SDK-контракт coreauth.Store поверх PostgreSQL.
type CoreAuthStore struct {
	db      dbgen.DBTX
	queries *dbgen.Queries
	keyring *security.Keyring
}

// NewCoreAuthStore создаёт encrypted credential store с system-proxy режимом.
func NewCoreAuthStore(db dbgen.DBTX, keyring *security.Keyring) *CoreAuthStore {
	store := &CoreAuthStore{
		db:      db,
		keyring: keyring,
	}
	if !nilDBTX(db) {
		store.queries = dbgen.New(db)
	}
	return store
}

// List загружает и прозрачно расшифровывает upstream credentials.
func (s *CoreAuthStore) List(ctx context.Context) ([]*coreauth.Auth, error) {
	if s == nil || s.queries == nil || s.keyring == nil {
		return nil, fmt.Errorf("list auths: %w", ErrInvalidInput)
	}

	rows, err := s.queries.ListUpstreamAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list upstream accounts: %w", err)
	}

	auths := make([]*coreauth.Auth, 0, len(rows))
	for _, row := range rows {
		plaintext, err := s.keyring.Decrypt(security.EncryptedValue{
			KeyVersion: int(row.EncKeyVersion),
			Ciphertext: row.CredentialsEnc,
		})
		if err != nil {
			return nil, fmt.Errorf("decrypt upstream account %q: %w", row.ID, err)
		}

		var auth coreauth.Auth
		if err := json.Unmarshal(plaintext, &auth); err != nil {
			return nil, fmt.Errorf("decode upstream account %q: %w", row.ID, ErrCorruptCredential)
		}
		if auth.ID != row.ID || auth.Provider != row.Provider {
			return nil, fmt.Errorf("validate upstream account %q: %w", row.ID, ErrCorruptCredential)
		}

		// Per-account proxy из legacy credentials не должен обходить proxy процесса.
		auth.ProxyURL = ""
		ensurePostgresSource(&auth)
		auths = append(auths, &auth)
	}
	return auths, nil
}

// Save шифрует и сохраняет запись Auth, заменяя существующую с тем же ID.
func (s *CoreAuthStore) Save(ctx context.Context, auth *coreauth.Auth) (string, error) {
	if s == nil || s.queries == nil || s.keyring == nil || auth == nil {
		return "", ErrInvalidInput
	}

	id := strings.TrimSpace(auth.ID)
	provider := strings.TrimSpace(auth.Provider)
	if id == "" || provider == "" {
		return "", ErrInvalidInput
	}

	persisted := auth.Clone()
	persisted.ID = id
	persisted.Provider = provider
	// Все HTTP-клиенты используют HTTP_PROXY/HTTPS_PROXY/NO_PROXY процесса.
	persisted.ProxyURL = ""
	ensurePostgresSource(persisted)
	metadata, err := persistedCredentialMetadata(persisted)
	if err != nil {
		return "", fmt.Errorf("encode token storage for %q: %w", id, err)
	}
	persisted.Metadata = metadata
	authType, err := databaseAuthType(persisted)
	if err != nil {
		return "", err
	}

	plaintext, err := json.Marshal(persisted)
	if err != nil {
		return "", fmt.Errorf("encode upstream account %q: %w", id, err)
	}
	encrypted, err := s.keyring.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt upstream account %q: %w", id, err)
	}
	if encrypted.KeyVersion <= 0 || encrypted.KeyVersion > math.MaxInt32 {
		return "", fmt.Errorf("%w: версия мастер-ключа не помещается в PostgreSQL integer", ErrInvalidInput)
	}

	attributes, err := publicAttributesJSON(persisted.Attributes)
	if err != nil {
		return "", fmt.Errorf("encode public attributes for %q: %w", id, err)
	}

	params := dbgen.UpsertUpstreamAccountParams{
		ID:               id,
		Provider:         provider,
		Email:            accountEmail(persisted),
		AuthType:         authType,
		Label:            nullableText(persisted.Label),
		CredentialsEnc:   encrypted.Ciphertext,
		EncKeyVersion:    int32(encrypted.KeyVersion),
		Attributes:       attributes,
		Status:           databaseAuthStatus(persisted),
		LastRefreshedAt:  nullableTimestamptz(persisted.LastRefreshedAt),
		NextRefreshAfter: nullableTimestamptz(persisted.NextRefreshAfter),
	}
	if starter, ok := s.db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	}); ok {
		tx, err := starter.Begin(ctx)
		if err != nil {
			return "", fmt.Errorf("begin save upstream account: %w", err)
		}
		defer tx.Rollback(ctx)
		queries := s.queries.WithTx(tx)
		savedID, err := queries.UpsertUpstreamAccount(ctx, params)
		if err != nil {
			return "", fmt.Errorf("save upstream account %q: %w", id, err)
		}
		if _, err := queries.IncrementRuntimeRevision(ctx, UpstreamAccountsRevision); err != nil {
			return "", fmt.Errorf("increment upstream accounts revision: %w", err)
		}
		if audit, ok := upstreamAccountAuditFromContext(ctx); ok {
			audit.TargetID = id
			if err := NewAdminAuditLogRepository(tx).Insert(ctx, audit); err != nil {
				return "", fmt.Errorf("write upstream account audit: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit save upstream account: %w", err)
		}
		return savedID, nil
	}
	if _, ok := upstreamAccountAuditFromContext(ctx); ok {
		return "", fmt.Errorf("save upstream account audit requires transactional store: %w", ErrInvalidInput)
	}
	savedID, err := s.queries.UpsertUpstreamAccount(ctx, params)
	if err != nil {
		return "", fmt.Errorf("save upstream account %q: %w", id, err)
	}
	return savedID, nil
}

// Delete идемпотентно удаляет upstream account по SDK-owned ID.
func (s *CoreAuthStore) Delete(ctx context.Context, id string) error {
	if s == nil || s.queries == nil || strings.TrimSpace(id) == "" {
		return ErrInvalidInput
	}
	if starter, ok := s.db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	}); ok {
		tx, err := starter.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin delete upstream account: %w", err)
		}
		defer tx.Rollback(ctx)
		queries := s.queries.WithTx(tx)
		if err := queries.DeleteUpstreamAccount(ctx, strings.TrimSpace(id)); err != nil {
			return fmt.Errorf("delete upstream account: %w", err)
		}
		if _, err := queries.IncrementRuntimeRevision(ctx, UpstreamAccountsRevision); err != nil {
			return fmt.Errorf("increment upstream accounts revision: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit delete upstream account: %w", err)
		}
		return nil
	}
	if err := s.queries.DeleteUpstreamAccount(ctx, strings.TrimSpace(id)); err != nil {
		return fmt.Errorf("delete upstream account: %w", err)
	}
	if _, err := s.queries.IncrementRuntimeRevision(ctx, UpstreamAccountsRevision); err != nil {
		return fmt.Errorf("increment upstream accounts revision: %w", err)
	}
	return nil
}

func databaseAuthType(auth *coreauth.Auth) (string, error) {
	switch auth.AuthKind() {
	case coreauth.AuthKindOAuth:
		return "oauth", nil
	case coreauth.AuthKindAPIKey:
		return "api-key", nil
	default:
		return "", fmt.Errorf("%w: неизвестный auth kind", ErrInvalidInput)
	}
}

func databaseAuthStatus(auth *coreauth.Auth) string {
	switch {
	case auth.Disabled || auth.Status == coreauth.StatusDisabled:
		return "disabled"
	case auth.Unavailable || auth.Status == coreauth.StatusError:
		return "unavailable"
	default:
		return "active"
	}
}

func accountEmail(auth *coreauth.Auth) string {
	if auth.Metadata != nil {
		if email, ok := auth.Metadata["email"].(string); ok {
			if email = strings.TrimSpace(email); email != "" {
				return email
			}
		}
	}
	return auth.ID
}

func publicAttributesJSON(attributes map[string]string) ([]byte, error) {
	public := make(map[string]string)
	for key, value := range attributes {
		if _, ok := persistedAttributeAllowlist[key]; ok {
			public[key] = value
		}
	}
	if len(public) == 0 {
		return nil, nil
	}
	return json.Marshal(public)
}

func ensurePostgresSource(auth *coreauth.Auth) {
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes[coreauth.AttributeSourceBackend] = coreauth.AuthSourcePostgres
}

func nullableTimestamptz(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

type tokenStorage interface {
	SaveTokenToFile(string) error
}

type rawJSONStorage interface {
	RawJSON() []byte
}

func persistedCredentialMetadata(auth *coreauth.Auth) (map[string]any, error) {
	metadata := make(map[string]any, len(auth.Metadata)+4)
	if auth.Storage != nil {
		storageMetadata, err := tokenStorageMetadata(auth.Storage)
		if err != nil {
			return nil, err
		}
		for key, value := range storageMetadata {
			metadata[key] = value
		}
	}
	for key, value := range auth.Metadata {
		metadata[key] = value
	}
	if providerType, ok := metadata["type"].(string); !ok || strings.TrimSpace(providerType) == "" {
		metadata["type"] = auth.Provider
	}
	metadata["disabled"] = auth.Disabled
	return metadata, nil
}

func tokenStorageMetadata(storage tokenStorage) (map[string]any, error) {
	if rawProvider, ok := storage.(rawJSONStorage); ok {
		if metadata, valid, err := decodeStorageMetadata(rawProvider.RawJSON()); err != nil {
			return nil, err
		} else if valid {
			return metadata, nil
		}
	}

	if tokenStorageHasHiddenState(storage) {
		return nil, fmt.Errorf("%w: token storage содержит неэкспортируемое состояние без RawJSON", ErrInvalidInput)
	}
	raw, err := json.Marshal(storage)
	if err != nil {
		return nil, fmt.Errorf("encode token storage: %w", err)
	}
	metadata, valid, err := decodeStorageMetadata(raw)
	if err != nil {
		return nil, err
	}
	if !valid || len(metadata) == 0 {
		return nil, fmt.Errorf("%w: token storage не предоставляет JSON credentials", ErrInvalidInput)
	}
	return metadata, nil
}

func tokenStorageHasHiddenState(storage tokenStorage) bool {
	return jsonValueHasHiddenState(reflect.ValueOf(storage), make(map[reflectionVisit]struct{}))
}

type reflectionVisit struct {
	typeOfValue reflect.Type
	pointer     uintptr
}

func jsonValueHasHiddenState(value reflect.Value, visited map[reflectionVisit]struct{}) bool {
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		return jsonValueHasHiddenState(value.Elem(), visited)
	}
	if valueSupportsJSON(value) {
		return false
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() || reflectionValueVisited(value, visited) {
			return false
		}
		return jsonValueHasHiddenState(value.Elem(), visited)
	case reflect.Struct:
		typeOfValue := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := typeOfValue.Field(i)
			fieldValue := value.Field(i)
			ignored := field.PkgPath != "" || strings.Split(field.Tag.Get("json"), ",")[0] == "-"
			if ignored {
				if !fieldValue.IsZero() {
					return true
				}
				continue
			}
			if jsonValueHasHiddenState(fieldValue, visited) {
				return true
			}
		}
	case reflect.Map:
		if value.IsNil() || reflectionValueVisited(value, visited) {
			return false
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if jsonValueHasHiddenState(iterator.Value(), visited) {
				return true
			}
		}
	case reflect.Slice:
		if value.IsNil() || reflectionValueVisited(value, visited) {
			return false
		}
		fallthrough
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if jsonValueHasHiddenState(value.Index(i), visited) {
				return true
			}
		}
	}
	return false
}

func valueSupportsJSON(value reflect.Value) bool {
	if value.CanInterface() {
		if _, ok := value.Interface().(json.Marshaler); ok {
			return true
		}
		if _, ok := value.Interface().(encoding.TextMarshaler); ok {
			return true
		}
	}
	if value.CanAddr() && value.Addr().CanInterface() {
		if _, ok := value.Addr().Interface().(json.Marshaler); ok {
			return true
		}
		if _, ok := value.Addr().Interface().(encoding.TextMarshaler); ok {
			return true
		}
	}
	return false
}

func reflectionValueVisited(value reflect.Value, visited map[reflectionVisit]struct{}) bool {
	visit := reflectionVisit{typeOfValue: value.Type(), pointer: value.Pointer()}
	if _, ok := visited[visit]; ok {
		return true
	}
	visited[visit] = struct{}{}
	return false
}

func decodeStorageMetadata(raw []byte) (map[string]any, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, false, fmt.Errorf("decode token storage: %w", err)
	}
	return metadata, metadata != nil, nil
}

func nilDBTX(db dbgen.DBTX) bool {
	if db == nil {
		return true
	}
	value := reflect.ValueOf(db)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
