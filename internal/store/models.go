package store

import (
	"encoding/json"
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

// User — пользователь, provisioned из выбранного identity source.
type User struct {
	ID             int64
	Username       string
	Email          string
	Role           string
	Status         string
	IdentitySource string
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

// ModelOverride задаёт разрешённую модель и её upstream mapping.
type ModelOverride struct {
	ID            int64
	Provider      string
	ModelAlias    string
	UpstreamModel string
	Enabled       bool
	Config        []byte
}

// UpsertModelOverrideParams — параметры создания или обновления model override.
type UpsertModelOverrideParams struct {
	Provider      string
	ModelAlias    string
	UpstreamModel string
	Enabled       bool
	Config        []byte
}

// UsageEvent — аналитика одного upstream-вызова без исходного request payload.
type UsageEvent struct {
	UserID            *int64
	APIKeyID          *int64
	UpstreamAccountID string
	Provider          string
	Model             string
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CachedTokens      int64
	TotalTokens       int64
	StatusCode        int
	Error             string
	LatencyMS         int64
	TTFTMS            int64
	Failed            bool
}

// UsageSummary — личная агрегированная статистика за выбранный период.
type UsageSummary struct {
	RequestCount       int64
	FailedRequestCount int64
	InputTokens        int64
	OutputTokens       int64
	ReasoningTokens    int64
	CachedTokens       int64
	TotalTokens        int64
	ByModel            []UsageModelSummary
	ByAPIKey           []UsageAPIKeySummary
}

// UsageModelSummary — статистика пользователя по одной модели.
type UsageModelSummary struct {
	Model              string
	RequestCount       int64
	FailedRequestCount int64
	TotalTokens        int64
}

// UsageAPIKeySummary — статистика пользователя по одному API-ключу.
type UsageAPIKeySummary struct {
	APIKeyID           int64
	RequestCount       int64
	FailedRequestCount int64
	TotalTokens        int64
}

// AdminAuditLogEntry — append-only запись о mutating-действии администратора.
type AdminAuditLogEntry struct {
	ActorUserID int64
	Action      string
	TargetType  string
	TargetID    string
	Details     []byte
	ActorIP     *netip.Addr
}

func validModelOverrideParams(params UpsertModelOverrideParams) bool {
	return params.Provider != "" && params.ModelAlias != "" && params.UpstreamModel != "" &&
		(len(params.Config) == 0 || json.Valid(params.Config))
}
