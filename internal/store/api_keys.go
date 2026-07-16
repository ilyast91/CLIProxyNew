package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/cache"
	"github.com/ilyast91/CLIProxyNew/internal/security"
	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// APIKeyRepository хранит bcrypt-хэши клиентских API-ключей.
type APIKeyRepository struct {
	queries         *dbgen.Queries
	candidatesCache *cache.TTL[apiKeyCacheKey, []apiKeyCandidate]
	verifiedCache   *cache.TTL[verifiedAPIKeyCacheKey, APIKeyPrincipal]
}

const apiKeyCacheTTL = 10 * time.Second

type apiKeyCacheKey struct {
	prefix         string
	identitySource string
}

type apiKeyCandidate struct {
	id      int64
	userID  int64
	keyHash string
}

type verifiedAPIKeyCacheKey struct {
	digest         [sha256.Size]byte
	identitySource string
}

// NewAPIKeyRepository создаёт репозиторий API-ключей.
func NewAPIKeyRepository(db dbgen.DBTX) *APIKeyRepository {
	return &APIKeyRepository{
		queries:         dbgen.New(db),
		candidatesCache: cache.NewTTL[apiKeyCacheKey, []apiKeyCandidate](apiKeyCacheTTL, nil),
		verifiedCache:   cache.NewTTL[verifiedAPIKeyCacheKey, APIKeyPrincipal](apiKeyCacheTTL, nil),
	}
}

// CacheStats возвращает snapshot verified authentication cache для observability.
func (r *APIKeyRepository) CacheStats() cache.Stats {
	if r == nil || r.verifiedCache == nil {
		return cache.Stats{}
	}
	return r.verifiedCache.Stats()
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
	r.invalidateCandidates(params.Plaintext[:APIKeyPrefixLength])
	return apiKeyFromDB(row), nil
}

// Authenticate находит кандидатов по префиксу и проверяет bcrypt-хэш.
func (r *APIKeyRepository) Authenticate(ctx context.Context, plaintext string) (APIKeyPrincipal, error) {
	if len(plaintext) < APIKeyPrefixLength {
		return APIKeyPrincipal{}, ErrInvalidCredential
	}
	return r.authenticate(ctx, plaintext, "")
}

// AuthenticateForSource проверяет API-key только для активного identity source.
func (r *APIKeyRepository) AuthenticateForSource(ctx context.Context, plaintext, identitySource string) (APIKeyPrincipal, error) {
	if len(plaintext) < APIKeyPrefixLength || !validIdentitySource(identitySource) {
		return APIKeyPrincipal{}, ErrInvalidCredential
	}
	return r.authenticate(ctx, plaintext, identitySource)
}

func (r *APIKeyRepository) authenticate(ctx context.Context, plaintext, identitySource string) (APIKeyPrincipal, error) {
	verifiedKey := verifiedAPIKeyCacheKey{digest: sha256.Sum256([]byte(plaintext)), identitySource: identitySource}
	if principal, ok := r.verifiedCache.Get(verifiedKey); ok {
		return principal, nil
	}

	candidates, err := r.findCandidates(ctx, apiKeyCacheKey{
		prefix:         plaintext[:APIKeyPrefixLength],
		identitySource: identitySource,
	})
	if err != nil {
		return APIKeyPrincipal{}, err
	}
	for _, candidate := range candidates {
		if !security.VerifySecret(candidate.keyHash, plaintext) {
			continue
		}
		active, err := r.userIsActive(ctx, candidate.userID)
		if err != nil {
			return APIKeyPrincipal{}, err
		}
		if active {
			principal := APIKeyPrincipal{UserID: candidate.userID, APIKeyID: candidate.id}
			r.verifiedCache.Set(verifiedKey, principal)
			return principal, nil
		}
	}
	return APIKeyPrincipal{}, ErrInvalidCredential
}

// InvalidateUser удаляет локально кэшированный verified principal пользователя.
func (r *APIKeyRepository) InvalidateUser(userID int64) {
	if r == nil || userID <= 0 {
		return
	}
	r.verifiedCache.DeleteWhere(func(_ verifiedAPIKeyCacheKey, principal APIKeyPrincipal) bool {
		return principal.UserID == userID
	})
}

func (r *APIKeyRepository) findCandidates(ctx context.Context, key apiKeyCacheKey) ([]apiKeyCandidate, error) {
	if candidates, ok := r.candidatesCache.Get(key); ok {
		return candidates, nil
	}

	var (
		candidates []apiKeyCandidate
		err        error
	)
	if key.identitySource == "" {
		rows, queryErr := r.queries.FindAPIKeyCandidates(ctx, key.prefix)
		err = queryErr
		candidates = make([]apiKeyCandidate, 0, len(rows))
		for _, row := range rows {
			candidates = append(candidates, apiKeyCandidate{id: row.ID, userID: row.UserID, keyHash: row.KeyHash})
		}
	} else {
		rows, queryErr := r.queries.FindAPIKeyCandidatesForSource(ctx, dbgen.FindAPIKeyCandidatesForSourceParams{
			KeyPrefix: key.prefix, IdentitySource: key.identitySource,
		})
		err = queryErr
		candidates = make([]apiKeyCandidate, 0, len(rows))
		for _, row := range rows {
			candidates = append(candidates, apiKeyCandidate{id: row.ID, userID: row.UserID, keyHash: row.KeyHash})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("find API-key candidates: %w", err)
	}

	r.candidatesCache.Set(key, candidates)
	return candidates, nil
}

func (r *APIKeyRepository) invalidateCandidates(prefix string) {
	r.candidatesCache.Delete(apiKeyCacheKey{prefix: prefix})
	r.candidatesCache.Delete(apiKeyCacheKey{prefix: prefix, identitySource: "ldap"})
	r.candidatesCache.Delete(apiKeyCacheKey{prefix: prefix, identitySource: "static"})
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

// ListAll возвращает безопасные metadata всех API-ключей для admin management API.
func (r *APIKeyRepository) ListAll(ctx context.Context) ([]AdminAPIKey, error) {
	rows, err := r.queries.ListAllAPIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all API-keys: %w", err)
	}
	keys := make([]AdminAPIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, AdminAPIKey{
			APIKey: APIKey{
				ID: row.ID, UserID: row.UserID, Prefix: row.KeyPrefix, Name: row.Name.String,
				Status: row.Status, ExpiresAt: datePointer(row.ExpiresAt), Scope: append([]byte(nil), row.Scope...),
				LastUsedAt: timestamptzPointer(row.LastUsedAt), CreatedAt: row.CreatedAt.Time,
			},
			OwnerUsername: row.OwnerUsername, OwnerIdentitySource: row.OwnerIdentitySource, OwnerStatus: row.OwnerStatus,
		})
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
	r.candidatesCache.Clear()
	r.verifiedCache.Clear()
	return nil
}

func (r *APIKeyRepository) userIsActive(ctx context.Context, userID int64) (bool, error) {
	user, err := r.queries.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get API-key user: %w", err)
	}
	return user.Status == "active", nil
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
