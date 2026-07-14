package store

import (
	"errors"
	"net/netip"
	"time"
)

const (
	// APIKeyPrefixLength — длина индексируемого префикса клиентского API-ключа.
	APIKeyPrefixLength = 8
)

var (
	// ErrNotFound означает отсутствие запрошенной сущности.
	ErrNotFound = errors.New("сущность не найдена")
	// ErrInvalidCredential скрывает конкретную причину отказа аутентификации.
	ErrInvalidCredential = errors.New("неверные учётные данные")
	// ErrInvalidInput означает некорректные параметры репозитория.
	ErrInvalidInput = errors.New("некорректные параметры")
	// ErrCorruptCredential означает несогласованные или повреждённые upstream credentials.
	ErrCorruptCredential = errors.New("повреждённые upstream credentials")
)

// User — пользователь, provisioned из LDAP.
type User struct {
	ID        int64
	Username  string
	Email     string
	Role      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UpsertUserParams — LDAP snapshot для создания или обновления пользователя.
type UpsertUserParams struct {
	Username string
	Email    string
	Role     string
}

// APIKey — безопасное представление API-ключа без hash и plaintext.
type APIKey struct {
	ID         int64
	UserID     int64
	Prefix     string
	Name       string
	Status     string
	ExpiresAt  *time.Time
	Scope      []byte
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// CreateAPIKeyParams — параметры сохранения нового API-ключа.
type CreateAPIKeyParams struct {
	UserID    int64
	Plaintext string
	Name      string
	ExpiresAt *time.Time
	Scope     []byte
}

// APIKeyPrincipal — идентификаторы, нужные access.Provider после проверки ключа.
type APIKeyPrincipal struct {
	UserID   int64
	APIKeyID int64
}

// Session — серверная opaque-сессия; TokenHash безопасен для хранения и диагностики.
type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	Role      string
	ExpiresAt time.Time
	CreatedIP *netip.Addr
	CreatedAt time.Time
}

// CreateSessionParams — параметры сохранения opaque-сессии.
type CreateSessionParams struct {
	UserID    int64
	Token     string
	Role      string
	ExpiresAt time.Time
	CreatedIP *netip.Addr
}
