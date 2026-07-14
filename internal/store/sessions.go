package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SessionRepository хранит SHA-256-хэши opaque session tokens.
type SessionRepository struct {
	queries *dbgen.Queries
}

// NewSessionRepository создаёт репозиторий сессий.
func NewSessionRepository(db dbgen.DBTX) *SessionRepository {
	return &SessionRepository{queries: dbgen.New(db)}
}

// Create сохраняет новую сессию без plaintext token.
func (r *SessionRepository) Create(ctx context.Context, params CreateSessionParams) (Session, error) {
	if params.UserID <= 0 || params.Token == "" || (params.Role != "user" && params.Role != "admin") {
		return Session{}, ErrInvalidInput
	}

	row, err := r.queries.CreateSession(ctx, dbgen.CreateSessionParams{
		UserID:    params.UserID,
		TokenHash: hashSessionToken(params.Token),
		Role:      params.Role,
		ExpiresAt: pgtype.Timestamptz{Time: params.ExpiresAt, Valid: true},
		CreatedIp: params.CreatedIP,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return sessionFromDB(row), nil
}

// GetByToken возвращает активную сессию активного пользователя.
func (r *SessionRepository) GetByToken(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidCredential
	}

	row, err := r.queries.GetSessionByTokenHash(ctx, hashSessionToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidCredential
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session by token: %w", err)
	}
	if row.UserStatus != "active" {
		return Session{}, ErrInvalidCredential
	}
	return sessionFromRow(row), nil
}

// GetByTokenForSource возвращает сессию только для активного identity source.
func (r *SessionRepository) GetByTokenForSource(ctx context.Context, token, identitySource string) (Session, error) {
	if token == "" || !validIdentitySource(identitySource) {
		return Session{}, ErrInvalidCredential
	}

	row, err := r.queries.GetSessionByTokenHashForSource(ctx, dbgen.GetSessionByTokenHashForSourceParams{
		TokenHash:      hashSessionToken(token),
		IdentitySource: identitySource,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrInvalidCredential
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session by token for source: %w", err)
	}
	if row.UserStatus != "active" {
		return Session{}, ErrInvalidCredential
	}
	return sessionFromSourceRow(row), nil
}

// DeleteByUser инвалидирует все сессии пользователя.
func (r *SessionRepository) DeleteByUser(ctx context.Context, userID int64) error {
	if err := r.queries.DeleteSessionsByUser(ctx, userID); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// DeleteByTokenForSource удаляет текущую сессию активного identity source.
func (r *SessionRepository) DeleteByTokenForSource(ctx context.Context, token, identitySource string) error {
	if token == "" || !validIdentitySource(identitySource) {
		return ErrInvalidCredential
	}
	deleted, err := r.queries.DeleteSessionByTokenHashForSource(ctx, dbgen.DeleteSessionByTokenHashForSourceParams{
		TokenHash: hashSessionToken(token), IdentitySource: identitySource,
	})
	if err != nil {
		return fmt.Errorf("delete session by token: %w", err)
	}
	if deleted == 0 {
		return ErrInvalidCredential
	}
	return nil
}

// DeleteExpired удаляет истёкшие сессии и возвращает их количество.
func (r *SessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	count, err := r.queries.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return count, nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validIdentitySource(source string) bool {
	return source == "ldap" || source == "static"
}

func sessionFromDB(row dbgen.Session) Session {
	return Session{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		Role:      row.Role,
		ExpiresAt: row.ExpiresAt.Time,
		CreatedIP: row.CreatedIp,
		CreatedAt: row.CreatedAt.Time,
	}
}

func sessionFromRow(row dbgen.GetSessionByTokenHashRow) Session {
	return Session{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		Role:      row.Role,
		ExpiresAt: row.ExpiresAt.Time,
		CreatedIP: row.CreatedIp,
		CreatedAt: row.CreatedAt.Time,
	}
}

func sessionFromSourceRow(row dbgen.GetSessionByTokenHashForSourceRow) Session {
	return Session{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		Role:      row.Role,
		ExpiresAt: row.ExpiresAt.Time,
		CreatedIP: row.CreatedIp,
		CreatedAt: row.CreatedAt.Time,
	}
}
