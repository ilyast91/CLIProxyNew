// Package identity определяет единый контракт источников пользовательской identity.
package identity

import (
	"context"
	"errors"
)

const (
	// SourceLDAP обозначает identity, подтверждённую LDAP.
	SourceLDAP = "ldap"
	// SourceStatic обозначает development/test identity из env.
	SourceStatic = "static"

	// RoleUser даёт обычные пользовательские права.
	RoleUser = "user"
	// RoleAdmin даёт административные права.
	RoleAdmin = "admin"
)

var (
	// ErrInvalidCredentials скрывает конкретную причину отказа identity provider.
	ErrInvalidCredentials = errors.New("неверные учётные данные")
	// ErrInvalidConfiguration означает некорректные параметры identity provider.
	ErrInvalidConfiguration = errors.New("некорректная конфигурация identity provider")
	// ErrAccessDenied означает, что identity подтверждена, но не имеет права на вход.
	ErrAccessDenied = errors.New("доступ запрещён")
)

// Identity — результат успешной аутентификации из выбранного source.
type Identity struct {
	Username string
	Email    string
	Role     string
	Source   string
}

// Provider аутентифицирует пользователя в одном выбранном identity source.
type Provider interface {
	Authenticate(ctx context.Context, username, password string) (Identity, error)
}
