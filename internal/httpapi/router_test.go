package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

func TestRouterConfiguratorRegistersLoginRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	login := NewLoginHandler(fakeLoginService{result: identity.LoginResult{
		UserID:    42,
		Role:      identity.RoleUser,
		Token:     "opaque-session-token",
		ExpiresAt: time.Date(2026, time.July, 14, 18, 0, 0, 0, time.UTC),
	}}, false)

	RouterConfigurator(login, nil, nil, nil, nil, nil, nil, nil, nil)(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"username":"debug","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
