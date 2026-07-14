package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

func TestLoginHandlerSetsSessionCookieWithoutExposingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	expiresAt := time.Date(2026, time.July, 14, 18, 0, 0, 0, time.UTC)
	handler := NewLoginHandler(fakeLoginService{result: identity.LoginResult{
		UserID:    42,
		Role:      identity.RoleAdmin,
		Token:     "opaque-secret-token",
		ExpiresAt: expiresAt,
	}}, false)
	router := gin.New()
	router.POST("/api/v1/login", handler.Handle)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"username":"debug","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	cookie := response.Result().Cookies()[0]
	if cookie.Name != identity.SessionCookieName || cookie.Value != "opaque-secret-token" || !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie = %+v", cookie)
	}
	if strings.Contains(response.Body.String(), "opaque-secret-token") {
		t.Fatal("login response leaked opaque token")
	}
	var body loginResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.UserID != 42 || body.Role != identity.RoleAdmin || !body.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("response = %+v", body)
	}
}

func TestLoginHandlerMapsAuthenticationFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid credentials", err: identity.ErrInvalidCredentials, want: http.StatusUnauthorized},
		{name: "blocked", err: identity.ErrUserBlocked, want: http.StatusForbidden},
		{name: "access denied", err: identity.ErrAccessDenied, want: http.StatusForbidden},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewLoginHandler(fakeLoginService{err: testCase.err}, true)
			router := gin.New()
			router.POST("/api/v1/login", handler.Handle)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"username":"debug","password":"secret"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != testCase.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

type fakeLoginService struct {
	result identity.LoginResult
	err    error
}

func (s fakeLoginService) Login(context.Context, string, string) (identity.LoginResult, error) {
	return s.result, s.err
}
