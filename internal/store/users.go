package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	if !validUserParams(params) || strings.HasPrefix(params.Username, "static:") {
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
	return userFromValues(row.ID, row.Username, row.Email, row.Role, row.Status, row.IdentitySource, row.CreatedAt, row.UpdatedAt), nil
}

// UpsertStatic создаёт или обновляет пользователя static identity source.
func (r *UserRepository) UpsertStatic(ctx context.Context, params UpsertUserParams) (User, error) {
	if !validUserParams(params) || !strings.HasPrefix(params.Username, "static:") {
		return User{}, ErrInvalidInput
	}

	row, err := r.queries.UpsertStaticUser(ctx, dbgen.UpsertStaticUserParams{
		Username: params.Username,
		Email:    nullableText(params.Email),
		Role:     params.Role,
	})
	if err != nil {
		return User{}, fmt.Errorf("upsert static user: %w", err)
	}
	return userFromValues(row.ID, row.Username, row.Email, row.Role, row.Status, row.IdentitySource, row.CreatedAt, row.UpdatedAt), nil
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
	return userFromValues(row.ID, row.Username, row.Email, row.Role, row.Status, row.IdentitySource, row.CreatedAt, row.UpdatedAt), nil
}

// List возвращает пользователей для management API.
func (r *UserRepository) List(ctx context.Context) ([]User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromValues(row.ID, row.Username, row.Email, row.Role, row.Status, row.IdentitySource, row.CreatedAt, row.UpdatedAt))
	}
	return users, nil
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

func userFromValues(
	id int64,
	username string,
	email pgtype.Text,
	role, status, identitySource string,
	createdAt, updatedAt pgtype.Timestamptz,
) User {
	return User{
		ID:             id,
		Username:       username,
		Email:          email.String,
		Role:           role,
		Status:         status,
		IdentitySource: identitySource,
		CreatedAt:      createdAt.Time,
		UpdatedAt:      updatedAt.Time,
	}
}

func validUserParams(params UpsertUserParams) bool {
	return params.Username != "" && (params.Role == "user" || params.Role == "admin")
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
