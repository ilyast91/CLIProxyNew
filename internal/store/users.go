package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserRepository хранит LDAP-derived пользователей.
type UserRepository struct {
	queries *dbgen.Queries
}

// NewUserRepository создаёт репозиторий пользователей.
func NewUserRepository(db dbgen.DBTX) *UserRepository {
	return &UserRepository{queries: dbgen.New(db)}
}

// UpsertFromLDAP создаёт пользователя либо обновляет LDAP snapshot.
func (r *UserRepository) UpsertFromLDAP(ctx context.Context, params UpsertUserParams) (User, error) {
	if params.Username == "" || (params.Role != "user" && params.Role != "admin") {
		return User{}, ErrInvalidInput
	}

	row, err := r.queries.UpsertUserFromLDAP(ctx, dbgen.UpsertUserFromLDAPParams{
		Username: params.Username,
		Email:    nullableText(params.Email),
		Role:     params.Role,
	})
	if err != nil {
		return User{}, fmt.Errorf("upsert LDAP user: %w", err)
	}
	return userFromDB(row), nil
}

// GetByID возвращает пользователя по идентификатору.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return userFromDB(row), nil
}

// SetStatus блокирует или разблокирует пользователя.
func (r *UserRepository) SetStatus(ctx context.Context, id int64, status string) error {
	if status != "active" && status != "blocked" {
		return ErrInvalidInput
	}

	var (
		updated int64
		err     error
	)
	if status == "blocked" {
		updated, err = r.queries.BlockUserAndDeleteSessions(ctx, id)
	} else {
		updated, err = r.queries.SetUserStatus(ctx, dbgen.SetUserStatusParams{ID: id, Status: status})
	}
	if err != nil {
		return fmt.Errorf("set user status: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

func userFromDB(row dbgen.User) User {
	return User{
		ID:        row.ID,
		Username:  row.Username,
		Email:     row.Email.String,
		Role:      row.Role,
		Status:    row.Status,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
