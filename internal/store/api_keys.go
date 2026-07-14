package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5/pgtype"
)

// APIKeyRepository хранит bcrypt-хэши клиентских API-ключей.
type APIKeyRepository struct {
	queries *dbgen.Queries
}

// NewAPIKeyRepository создаёт репозиторий API-ключей.
func NewAPIKeyRepository(db dbgen.DBTX) *APIKeyRepository {
	return &APIKeyRepository{queries: dbgen.New(db)}
}

// Create хэширует и сохраняет API-ключ, не сохраняя plaintext.
func (r *APIKeyRepository) Create(ctx context.Context, params CreateAPIKeyParams) (APIKey, error) {
	if params.UserID <= 0 || len(params.Plaintext) < APIKeyPrefixLength {
		return APIKey{}, ErrInvalidInput
	}

	hash, err := security.HashSecret(params.Plaintext)
	if err != nil {
		return APIKey{}, fmt.Errorf("hash API-key: %w", err)
	}

	row, err := r.queries.CreateAPIKey(ctx, dbgen.CreateAPIKeyParams{
		UserID:    params.UserID,
		KeyHash:   hash,
		KeyPrefix: params.Plaintext[:APIKeyPrefixLength],
		Name:      nullableText(params.Name),
		ExpiresAt: nullableDate(params.ExpiresAt),
		Scope:     append([]byte(nil), params.Scope...),
	})
	if err != nil {
		return APIKey{}, fmt.Errorf("create API-key: %w", err)
	}
	return apiKeyFromDB(row), nil
}

// Authenticate находит кандидатов по префиксу и проверяет bcrypt-хэш.
func (r *APIKeyRepository) Authenticate(ctx context.Context, plaintext string) (APIKeyPrincipal, error) {
	if len(plaintext) < APIKeyPrefixLength {
		return APIKeyPrincipal{}, ErrInvalidCredential
	}

	candidates, err := r.queries.FindAPIKeyCandidates(ctx, plaintext[:APIKeyPrefixLength])
	if err != nil {
		return APIKeyPrincipal{}, fmt.Errorf("find API-key candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.UserStatus == "active" && security.VerifySecret(candidate.KeyHash, plaintext) {
			return APIKeyPrincipal{UserID: candidate.UserID, APIKeyID: candidate.ID}, nil
		}
	}
	return APIKeyPrincipal{}, ErrInvalidCredential
}

// AuthenticateForSource проверяет API-key только для активного identity source.
func (r *APIKeyRepository) AuthenticateForSource(ctx context.Context, plaintext, identitySource string) (APIKeyPrincipal, error) {
	if len(plaintext) < APIKeyPrefixLength || !validIdentitySource(identitySource) {
		return APIKeyPrincipal{}, ErrInvalidCredential
	}

	candidates, err := r.queries.FindAPIKeyCandidatesForSource(ctx, dbgen.FindAPIKeyCandidatesForSourceParams{
		KeyPrefix:      plaintext[:APIKeyPrefixLength],
		IdentitySource: identitySource,
	})
	if err != nil {
		return APIKeyPrincipal{}, fmt.Errorf("find API-key candidates for source: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.UserStatus == "active" && security.VerifySecret(candidate.KeyHash, plaintext) {
			return APIKeyPrincipal{UserID: candidate.UserID, APIKeyID: candidate.ID}, nil
		}
	}
	return APIKeyPrincipal{}, ErrInvalidCredential
}

// ListByUser возвращает безопасные метаданные ключей без bcrypt-хэшей.
func (r *APIKeyRepository) ListByUser(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := r.queries.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list API-keys: %w", err)
	}

	keys := make([]APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, apiKeyFromListRow(row))
	}
	return keys, nil
}

// Revoke отзывает активный ключ пользователя.
func (r *APIKeyRepository) Revoke(ctx context.Context, userID, keyID int64) error {
	revoked, err := r.queries.RevokeAPIKey(ctx, dbgen.RevokeAPIKeyParams{ID: keyID, UserID: userID})
	if err != nil {
		return fmt.Errorf("revoke API-key: %w", err)
	}
	if revoked == 0 {
		return ErrNotFound
	}
	return nil
}

func apiKeyFromDB(row dbgen.CreateAPIKeyRow) APIKey {
	return APIKey{
		ID:         row.ID,
		UserID:     row.UserID,
		Prefix:     row.KeyPrefix,
		Name:       row.Name.String,
		Status:     row.Status,
		ExpiresAt:  datePointer(row.ExpiresAt),
		Scope:      append([]byte(nil), row.Scope...),
		LastUsedAt: timestamptzPointer(row.LastUsedAt),
		CreatedAt:  row.CreatedAt.Time,
	}
}

func apiKeyFromListRow(row dbgen.ListAPIKeysByUserRow) APIKey {
	return APIKey{
		ID:         row.ID,
		UserID:     row.UserID,
		Prefix:     row.KeyPrefix,
		Name:       row.Name.String,
		Status:     row.Status,
		ExpiresAt:  datePointer(row.ExpiresAt),
		Scope:      append([]byte(nil), row.Scope...),
		LastUsedAt: timestamptzPointer(row.LastUsedAt),
		CreatedAt:  row.CreatedAt.Time,
	}
}

func nullableDate(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *value, Valid: true}
}

func datePointer(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func timestamptzPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
