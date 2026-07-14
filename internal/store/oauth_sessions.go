package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OAuthSessionRepository хранит multi-replica OAuth-сессии в Postgres.
type OAuthSessionRepository struct{ queries *dbgen.Queries }

// NewOAuthSessionRepository создаёт репозиторий OAuth-сессий.
func NewOAuthSessionRepository(db dbgen.DBTX) *OAuthSessionRepository {
	return &OAuthSessionRepository{queries: dbgen.New(db)}
}

// Create сохраняет pending OAuth-сессию.
func (r *OAuthSessionRepository) Create(ctx context.Context, p CreateOAuthSessionParams) error {
	if r == nil || r.queries == nil || p.State == "" || p.Provider == "" || (p.FlowType != "callback" && p.FlowType != "device") || p.ExpiresAt.IsZero() {
		return ErrInvalidInput
	}
	if err := r.queries.CreateOAuthSession(ctx, dbgen.CreateOAuthSessionParams{State: p.State, Provider: p.Provider, FlowType: p.FlowType, ExpiresAt: pgtype.Timestamptz{Time: p.ExpiresAt, Valid: true}, PkceVerifier: nullableText(p.PKCEVerifier), DeviceCode: nullableText(p.DeviceCode), UserCode: nullableText(p.UserCode)}); err != nil {
		return fmt.Errorf("create OAuth session: %w", err)
	}
	return nil
}

// Get возвращает OAuth-сессию по state.
func (r *OAuthSessionRepository) Get(ctx context.Context, state string) (OAuthSession, error) {
	row, err := r.queries.GetOAuthSession(ctx, state)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthSession{}, ErrNotFound
	}
	if err != nil {
		return OAuthSession{}, fmt.Errorf("get OAuth session: %w", err)
	}
	return oauthSessionFromDB(row), nil
}

// ListPending возвращает ожидающие OAuth-сессии.
func (r *OAuthSessionRepository) ListPending(ctx context.Context) ([]OAuthSession, error) {
	rows, err := r.queries.ListPendingOAuthSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending OAuth sessions: %w", err)
	}
	out := make([]OAuthSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, oauthSessionFromDB(row))
	}
	return out, nil
}

// Cancel переводит pending OAuth-сессию в cancelled.
func (r *OAuthSessionRepository) Cancel(ctx context.Context, state string) error {
	n, err := r.queries.CancelOAuthSession(ctx, state)
	if err != nil {
		return fmt.Errorf("cancel OAuth session: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Complete переводит pending OAuth-сессию в completed и привязывает account.
func (r *OAuthSessionRepository) Complete(ctx context.Context, state, authID string) error {
	n, err := r.queries.CompleteOAuthSession(ctx, dbgen.CompleteOAuthSessionParams{State: state, AuthID: nullableText(authID)})
	if err != nil {
		return fmt.Errorf("complete OAuth session: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Fail переводит pending OAuth-сессию в error.
func (r *OAuthSessionRepository) Fail(ctx context.Context, state, message string) error {
	n, err := r.queries.FailOAuthSession(ctx, dbgen.FailOAuthSessionParams{State: state, ErrorMessage: nullableText(message)})
	if err != nil {
		return fmt.Errorf("fail OAuth session: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func oauthSessionFromDB(row dbgen.OauthSession) OAuthSession {
	return OAuthSession{State: row.State, Provider: row.Provider, FlowType: row.FlowType, Status: row.Status, AuthID: row.AuthID.String, PKCEVerifier: row.PkceVerifier.String, DeviceCode: row.DeviceCode.String, UserCode: row.UserCode.String, ErrorMessage: row.ErrorMessage.String, ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}
