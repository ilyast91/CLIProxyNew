package testing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var (
	// ErrAccountNotFound означает отсутствие аккаунта в runtime-реестре ядра.
	ErrAccountNotFound = errors.New("upstream account not found")
	// ErrExecutorUnavailable означает, что для провайдера нет public executor.
	ErrExecutorUnavailable = errors.New("upstream provider executor unavailable")
	// ErrUnsupportedProvider означает отсутствие безопасного metadata endpoint.
	ErrUnsupportedProvider = errors.New("unsupported provider health check")
)

const (
	// MethodRefresh означает OAuth refresh-token проверку.
	MethodRefresh = "refresh"
	// MethodHTTPProbe означает metadata HTTP-проверку API-key.
	MethodHTTPProbe = "http_probe"
)

type checkerManager interface {
	GetByID(string) (*coreauth.Auth, bool)
	Executor(string) (coreauth.ProviderExecutor, bool)
	Update(context.Context, *coreauth.Auth) (*coreauth.Auth, error)
}

// QuotaInfo передаёт только безопасное runtime-состояние upstream квоты.
type QuotaInfo struct {
	Exceeded      bool       `json:"exceeded"`
	Reason        string     `json:"reason,omitempty"`
	NextRecoverAt *time.Time `json:"next_recover_at,omitempty"`
	Unknown       bool       `json:"unknown"`
}

// Result описывает результат проверки upstream credential без секретов.
type Result struct {
	Valid           bool       `json:"valid"`
	Method          string     `json:"method"`
	StatusCode      int        `json:"status_code,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	Quota           QuotaInfo  `json:"quota"`
	Error           string     `json:"error,omitempty"`
}

// Checker проверяет upstream аккаунты без выполнения inference-запросов.
type Checker struct{ manager checkerManager }

// NewChecker создаёт Checker поверх public coreauth.Manager contract.
func NewChecker(manager checkerManager) *Checker { return &Checker{manager: manager} }

// Test проверяет OAuth через Refresh, а API-key через metadata HTTP endpoint.
func (c *Checker) Test(ctx context.Context, accountID string) (Result, error) {
	if c == nil || c.manager == nil {
		return Result{}, ErrExecutorUnavailable
	}
	auth, ok := c.manager.GetByID(strings.TrimSpace(accountID))
	if !ok || auth == nil {
		return Result{}, ErrAccountNotFound
	}
	executor, ok := c.manager.Executor(auth.Provider)
	if !ok || executor == nil {
		return Result{}, ErrExecutorUnavailable
	}

	switch auth.AuthKind() {
	case coreauth.AuthKindOAuth:
		return c.testOAuth(ctx, auth, executor)
	case coreauth.AuthKindAPIKey:
		return c.testAPIKey(ctx, auth, executor)
	default:
		return Result{}, ErrUnsupportedProvider
	}
}

func (c *Checker) testOAuth(ctx context.Context, auth *coreauth.Auth, executor coreauth.ProviderExecutor) (Result, error) {
	refreshed, err := executor.Refresh(ctx, auth)
	if err != nil || refreshed == nil {
		return resultForAuth(auth, MethodRefresh, 0, false, "credential refresh failed"), nil
	}
	updated, err := c.manager.Update(ctx, refreshed)
	if err != nil {
		return Result{}, fmt.Errorf("persist refreshed upstream account: %w", err)
	}
	if updated == nil {
		return Result{}, ErrAccountNotFound
	}
	return resultForAuth(updated, MethodRefresh, http.StatusOK, true, ""), nil
}

func (c *Checker) testAPIKey(ctx context.Context, auth *coreauth.Auth, executor coreauth.ProviderExecutor) (Result, error) {
	request, err := probeRequest(ctx, auth)
	if err != nil {
		return Result{}, err
	}
	response, err := executor.HttpRequest(ctx, auth, request)
	if err != nil || response == nil {
		return resultForAuth(auth, MethodHTTPProbe, 0, false, "credential probe failed"), nil
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	valid := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	if valid {
		return resultForAuth(auth, MethodHTTPProbe, response.StatusCode, true, ""), nil
	}
	return resultForAuth(auth, MethodHTTPProbe, response.StatusCode, false, fmt.Sprintf("upstream returned HTTP %d", response.StatusCode)), nil
}

func resultForAuth(auth *coreauth.Auth, method string, statusCode int, valid bool, message string) Result {
	expiresAt, hasExpiration := auth.ExpirationTime()
	quota := auth.Quota
	var nextRecoverAt *time.Time
	if !quota.NextRecoverAt.IsZero() {
		nextRecoverAt = &quota.NextRecoverAt
	}
	var expiration *time.Time
	if hasExpiration {
		expiration = &expiresAt
	}
	var lastRefreshedAt *time.Time
	if !auth.LastRefreshedAt.IsZero() {
		lastRefreshedAt = &auth.LastRefreshedAt
	}
	return Result{
		Valid: valid, Method: method, StatusCode: statusCode, ExpiresAt: expiration, LastRefreshedAt: lastRefreshedAt,
		Quota: QuotaInfo{Exceeded: quota.Exceeded, Reason: quota.Reason, NextRecoverAt: nextRecoverAt, Unknown: !quota.Exceeded && quota.Reason == "" && quota.NextRecoverAt.IsZero() && quota.BackoffLevel == 0},
		Error: message,
	}
}

func probeRequest(ctx context.Context, auth *coreauth.Auth) (*http.Request, error) {
	provider := strings.ToLower(strings.TrimSpace(auth.Provider))
	var target string
	headers := make(http.Header)
	switch provider {
	case "claude":
		target = "https://api.anthropic.com/v1/models"
		headers.Set("anthropic-version", "2023-06-01")
	case "codex":
		target = "https://chatgpt.com/backend-api/me"
	case "gemini":
		apiKey := strings.TrimSpace(auth.Attributes[coreauth.AttributeAPIKey])
		if apiKey == "" {
			return nil, ErrUnsupportedProvider
		}
		target = "https://generativelanguage.googleapis.com/v1beta/models"
		parsed, _ := url.Parse(target)
		query := parsed.Query()
		query.Set("key", apiKey)
		parsed.RawQuery = query.Encode()
		target = parsed.String()
	case "openai-compatibility":
		baseURL := strings.TrimSpace(auth.Attributes["base_url"])
		if baseURL == "" {
			return nil, ErrUnsupportedProvider
		}
		target = strings.TrimRight(baseURL, "/") + "/models"
	default:
		return nil, ErrUnsupportedProvider
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("create upstream metadata request: %w", err)
	}
	request.Header = headers
	return request, nil
}
