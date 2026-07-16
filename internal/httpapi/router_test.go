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
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
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

func TestRouterConfiguratorRunsProviderAndModelAdminFlows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	sessions := routerSessions()
	registrar := &fakeUpstreamAuthRegistrar{}
	models := &fakeAdminModelStore{override: store.ModelOverride{ID: 9, Provider: "openai", ModelAlias: "fast", UpstreamModel: "gpt-5", Enabled: true}}
	RouterConfigurator(nil, sessions, nil, nil, nil, nil, nil, nil, NewAdminProviderKeyHandler(registrar), nil, nil, nil, NewAdminModelHandler(models))(router, nil, nil)

	request := managementJSONRequest(http.MethodPost, "/api/v1/admin/providers/keys", `{"accounts":[{"provider":"claude","api_key":"upstream-secret"}]}`, "admin-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || len(registrar.auths) != 1 {
		t.Fatalf("provider keys status=%d registered=%d body=%s", response.Code, len(registrar.auths), response.Body.String())
	}

	request = managementJSONRequest(http.MethodPut, "/api/v1/admin/models/fast", `{"provider":"openai","upstream_model":"gpt-5","enabled":true}`, "admin-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || models.actor != 42 || models.params.ModelAlias != "fast" {
		t.Fatalf("model upsert status=%d actor=%d params=%+v body=%s", response.Code, models.actor, models.params, response.Body.String())
	}

	forbidden := managementJSONRequest(http.MethodPost, "/api/v1/admin/providers/keys", `{"accounts":[{"provider":"claude","api_key":"upstream-secret"}]}`, "user-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, forbidden)
	if response.Code != http.StatusForbidden {
		t.Fatalf("user provider keys status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouterConfiguratorRunsOAuthAndAccountAdminFlows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	sessions := routerSessions()
	oauthSessions := &fakeAdminOAuthSessions{sessions: []store.OAuthSession{{State: "state-1", Provider: "claude", FlowType: "callback", Status: "pending"}}}
	checker := &fakeAccountChecker{}
	quota := &fakeQuotaLookup{auth: &coreauth.Auth{ID: "account-1", Provider: "claude"}}
	oauthManager := &fakeOAuthCredentialManager{auths: []*coreauth.Auth{{ID: "oauth-1", Provider: "claude", Attributes: map[string]string{coreauth.AttributeAuthKind: coreauth.AuthKindOAuth}, Metadata: map[string]any{"email": "admin@example.com", "refresh_token": "secret-refresh"}}}}
	RouterConfigurator(nil, sessions, nil, nil, nil, nil, nil, NewAdminOAuthSessionHandler(oauthSessions), nil, NewAdminAccountTestHandler(checker), NewAdminQuotaHandler(quota), NewAdminOAuthCredentialHandler(oauthManager, &fakeAdminAuditLogger{}), nil)(router, nil, nil)

	for _, path := range []string{"/api/v1/admin/oauth/sessions", "/api/v1/admin/accounts/account-1/quota", "/api/v1/admin/oauth/accounts/oauth-1/export"} {
		response := managementRequest(router, http.MethodGet, path, "admin-token")
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := managementRequest(router, http.MethodPost, "/api/v1/admin/accounts/account-1/test", "admin-token")
	if response.Code != http.StatusOK || checker.accountID != "account-1" {
		t.Fatalf("account test status=%d account=%q body=%s", response.Code, checker.accountID, response.Body.String())
	}
	response = managementRequest(router, http.MethodDelete, "/api/v1/admin/oauth/sessions/state-1", "admin-token")
	if response.Code != http.StatusNoContent || oauthSessions.actor != 42 || oauthSessions.state != "state-1" {
		t.Fatalf("oauth cancel status=%d actor=%d state=%q body=%s", response.Code, oauthSessions.actor, oauthSessions.state, response.Body.String())
	}

	request := managementJSONRequest(http.MethodPost, "/api/v1/admin/oauth/import", `{"provider":"claude","attributes":{"auth_kind":"oauth"},"metadata":{"email":"new@example.com","refresh_token":"secret-refresh"}}`, "admin-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || oauthManager.registered == nil {
		t.Fatalf("oauth import status=%d registered=%+v body=%s", response.Code, oauthManager.registered, response.Body.String())
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

func managementJSONRequest(method, path, body, token string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: token})
	return request
}

func routerSessions() *identity.SessionAuthenticator {
	return identity.NewSessionAuthenticator(routerSessionLookup{sessions: map[string]store.Session{
		"user-token":  {UserID: 7, Role: identity.RoleUser},
		"admin-token": {UserID: 42, Role: identity.RoleAdmin},
	}}, identity.SourceLDAP)
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
