package testing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestCheckerOAuthRefreshPersistsUpdatedAuth(t *testing.T) {
	refreshedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	auth := &coreauth.Auth{ID: "oauth-1", Provider: "claude", Attributes: map[string]string{coreauth.AttributeAuthKind: coreauth.AuthKindOAuth}}
	manager := &fakeCheckerManager{auth: auth, executor: &fakeCheckerExecutor{refresh: func(_ context.Context, input *coreauth.Auth) (*coreauth.Auth, error) {
		updated := input.Clone()
		updated.LastRefreshedAt = refreshedAt
		return updated, nil
	}}}

	result, err := NewChecker(manager).Test(context.Background(), auth.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.Valid || result.Method != MethodRefresh || result.LastRefreshedAt == nil || !result.LastRefreshedAt.Equal(refreshedAt) || manager.updated == nil || !manager.updated.LastRefreshedAt.Equal(refreshedAt) {
		t.Fatalf("result=%+v updated=%+v", result, manager.updated)
	}
}

func TestCheckerAPIKeyProbesCompatibleModelsEndpoint(t *testing.T) {
	auth := &coreauth.Auth{ID: "key-1", Provider: "openai-compatibility", Attributes: map[string]string{
		coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
		coreauth.AttributeAPIKey:   "upstream-secret",
		"base_url":                 "https://example.com/v1/",
	}}
	executor := &fakeCheckerExecutor{httpRequest: func(_ context.Context, _ *coreauth.Auth, request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://example.com/v1/models" {
			t.Fatalf("request=%s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}}

	result, err := NewChecker(&fakeCheckerManager{auth: auth, executor: executor}).Test(context.Background(), auth.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.Valid || result.Method != MethodHTTPProbe || result.StatusCode != http.StatusOK {
		t.Fatalf("result=%+v", result)
	}
}

func TestCheckerReturnsInvalidForUpstreamHTTPFailure(t *testing.T) {
	auth := &coreauth.Auth{ID: "key-2", Provider: "openai-compatibility", Attributes: map[string]string{
		coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
		coreauth.AttributeAPIKey:   "upstream-secret",
		"base_url":                 "https://example.com/v1",
	}}
	executor := &fakeCheckerExecutor{httpRequest: func(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
	}}

	result, err := NewChecker(&fakeCheckerManager{auth: auth, executor: executor}).Test(context.Background(), auth.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.Valid || result.StatusCode != http.StatusUnauthorized || result.Error == "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCheckerRejectsUnknownAccount(t *testing.T) {
	_, err := NewChecker(&fakeCheckerManager{}).Test(context.Background(), "missing")
	if err != ErrAccountNotFound {
		t.Fatalf("error=%v, want %v", err, ErrAccountNotFound)
	}
}

type fakeCheckerManager struct {
	auth     *coreauth.Auth
	executor coreauth.ProviderExecutor
	updated  *coreauth.Auth
}

func (m *fakeCheckerManager) GetByID(id string) (*coreauth.Auth, bool) {
	if m.auth == nil || m.auth.ID != id {
		return nil, false
	}
	return m.auth.Clone(), true
}

func (m *fakeCheckerManager) Executor(string) (coreauth.ProviderExecutor, bool) {
	return m.executor, m.executor != nil
}

func (m *fakeCheckerManager) Update(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	m.updated = auth.Clone()
	return m.updated.Clone(), nil
}

type fakeCheckerExecutor struct {
	coreauth.ProviderExecutor
	refresh     func(context.Context, *coreauth.Auth) (*coreauth.Auth, error)
	httpRequest func(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error)
}

func (e *fakeCheckerExecutor) Refresh(ctx context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return e.refresh(ctx, auth)
}

func (e *fakeCheckerExecutor) HttpRequest(ctx context.Context, auth *coreauth.Auth, request *http.Request) (*http.Response, error) {
	return e.httpRequest(ctx, auth, request)
}
