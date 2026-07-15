package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/store"
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

	RouterConfigurator(login, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)(router, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(`{"username":"debug","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRouterConfiguratorEnforcesManagementSessionAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	sessions := identity.NewSessionAuthenticator(routerSessionLookup{sessions: map[string]store.Session{
		"user-token":  {UserID: 7, Role: identity.RoleUser},
		"admin-token": {UserID: 42, Role: identity.RoleAdmin},
	}}, identity.SourceLDAP)
	adminUsers := NewAdminUserHandler(&fakeAdminUserStore{users: []store.User{{ID: 7, Username: "user", Role: identity.RoleUser, Status: "active"}}})
	RouterConfigurator(nil, sessions, nil, nil, nil, adminUsers, nil, nil, nil, nil, nil, nil, nil)(router, nil, nil)

	if response := managementRequest(router, http.MethodGet, "/api/v1/me", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous /me status=%d", response.Code)
	}
	if response := managementRequest(router, http.MethodGet, "/api/v1/me", "user-token"); response.Code != http.StatusOK {
		t.Fatalf("user /me status=%d body=%s", response.Code, response.Body.String())
	}
	if response := managementRequest(router, http.MethodGet, "/api/v1/admin/users", "user-token"); response.Code != http.StatusForbidden {
		t.Fatalf("user /admin/users status=%d", response.Code)
	}
	if response := managementRequest(router, http.MethodGet, "/api/v1/admin/users", "admin-token"); response.Code != http.StatusOK {
		t.Fatalf("admin /admin/users status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouterConfiguratorPassesSessionPrincipalToMutatingManagementHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	sessions := identity.NewSessionAuthenticator(routerSessionLookup{sessions: map[string]store.Session{
		"user-token":  {UserID: 7, Role: identity.RoleUser},
		"admin-token": {UserID: 42, Role: identity.RoleAdmin},
	}}, identity.SourceLDAP)
	keys := &fakeAPIKeyStore{keys: []store.APIKey{{ID: 10, UserID: 7, Prefix: "cpn_live", Status: "active"}}}
	adminUsers := &fakeAdminUserStore{}
	RouterConfigurator(nil, sessions, nil, NewAPIKeyHandler(keys), nil, NewAdminUserHandler(adminUsers), nil, nil, nil, nil, nil, nil, nil)(router, nil, nil)

	if response := managementRequest(router, http.MethodGet, "/api/v1/me/keys", "user-token"); response.Code != http.StatusOK || keys.listUserID != 7 {
		t.Fatalf("user keys status=%d list user=%d", response.Code, keys.listUserID)
	}
	if response := managementRequest(router, http.MethodDelete, "/api/v1/me/keys/10", "user-token"); response.Code != http.StatusNoContent || keys.revokeUserID != 7 || keys.revokeKeyID != 10 {
		t.Fatalf("revoke status=%d user=%d key=%d", response.Code, keys.revokeUserID, keys.revokeKeyID)
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/7", strings.NewReader(`{"status":"blocked"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "admin-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || adminUsers.actorUserID != 42 || adminUsers.targetUserID != 7 || adminUsers.status != "blocked" {
		t.Fatalf("admin status=%d actor=%d target=%d state=%q", response.Code, adminUsers.actorUserID, adminUsers.targetUserID, adminUsers.status)
	}
}

func managementRequest(router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: token})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type routerSessionLookup struct{ sessions map[string]store.Session }

func (l routerSessionLookup) GetByTokenForSource(_ context.Context, token, source string) (store.Session, error) {
	if source != identity.SourceLDAP {
		return store.Session{}, errors.New("unexpected identity source")
	}
	session, ok := l.sessions[token]
	if !ok {
		return store.Session{}, store.ErrInvalidCredential
	}
	return session, nil
}
