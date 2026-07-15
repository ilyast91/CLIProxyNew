package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestAdminProviderKeyHandlerCreateRegistersBatchWithoutReturningSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registrar := &fakeUpstreamAuthRegistrar{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/providers/keys", NewAdminProviderKeyHandler(registrar).Create)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/keys", strings.NewReader(`{"accounts":[{"provider":"openai-compatibility","api_key":"sk-secret","base_url":"https://openrouter.ai/api/v1","label":"primary"},{"provider":"claude","api_key":"sk-another"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(registrar.auths) != 2 {
		t.Fatalf("registered=%d, want 2", len(registrar.auths))
	}
	first := registrar.auths[0]
	if first.Provider != "openai-compatibility" || first.Label != "primary" || first.Attributes[coreauth.AttributeAPIKey] != "sk-secret" || first.Attributes["base_url"] != "https://openrouter.ai/api/v1" || first.Attributes[coreauth.AttributeAuthKind] != coreauth.AuthKindAPIKey {
		t.Fatalf("first auth=%+v", first)
	}
	if strings.Contains(response.Body.String(), "sk-secret") || strings.Contains(response.Body.String(), "sk-another") {
		t.Fatalf("response exposed upstream credential: %s", response.Body.String())
	}
}

func TestAdminProviderKeyHandlerCreateRejectsInvalidBatchBeforeRegistering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registrar := &fakeUpstreamAuthRegistrar{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/providers/keys", NewAdminProviderKeyHandler(registrar).Create)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/keys", strings.NewReader(`{"accounts":[{"provider":"claude","api_key":"valid"},{"provider":"","api_key":"missing-provider"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || len(registrar.auths) != 0 {
		t.Fatalf("status=%d registered=%d body=%s", response.Code, len(registrar.auths), response.Body.String())
	}
}

func TestAdminProviderKeyHandlerCreateMapsRegistrationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registrar := &fakeUpstreamAuthRegistrar{err: errors.New("store unavailable")}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/providers/keys", NewAdminProviderKeyHandler(registrar).Create)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/keys", strings.NewReader(`{"accounts":[{"provider":"claude","api_key":"valid"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeUpstreamAuthRegistrar struct {
	auths []*coreauth.Auth
	err   error
}

func (r *fakeUpstreamAuthRegistrar) Register(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	if r.err != nil {
		return nil, r.err
	}
	copy := auth.Clone()
	copy.ID = "sdk-auth-" + string(rune('1'+len(r.auths)))
	r.auths = append(r.auths, copy)
	return copy, nil
}
