package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestAdminOAuthCredentialHandlerExportReturnsAttachmentAndAudits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &fakeOAuthCredentialManager{auths: []*coreauth.Auth{{ID: "oauth-1", Provider: "claude", Attributes: map[string]string{coreauth.AttributeAuthKind: coreauth.AuthKindOAuth}, Metadata: map[string]any{"email": "admin@example.com", "refresh_token": "secret-refresh"}}}}
	audit := &fakeAdminAuditLogger{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.GET("/api/v1/admin/oauth/accounts/:accountID/export", NewAdminOAuthCredentialHandler(manager, audit).Export)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/oauth/accounts/oauth-1/export", nil))

	if response.Code != http.StatusOK || response.Header().Get("Content-Disposition") == "" || audit.entry.Action != "oauth.credential.exported" {
		t.Fatalf("status=%d disposition=%q audit=%+v", response.Code, response.Header().Get("Content-Disposition"), audit.entry)
	}
	var exported coreauth.Auth
	if err := json.Unmarshal(response.Body.Bytes(), &exported); err != nil || exported.Metadata["refresh_token"] != "secret-refresh" {
		t.Fatalf("export=%s err=%v", response.Body.String(), err)
	}
}

func TestAdminOAuthCredentialHandlerImportRegistersSDKManagedID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &fakeOAuthCredentialManager{}
	audit := &fakeAdminAuditLogger{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(manager, audit).Import)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/oauth/import", strings.NewReader(`{"id":"foreign-id","provider":"claude","attributes":{"auth_kind":"oauth"},"metadata":{"email":"admin@example.com","refresh_token":"secret-refresh"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || manager.registered == nil || manager.registered.ID != "" {
		t.Fatalf("status=%d registered=%+v audit=%+v", response.Code, manager.registered, audit.entry)
	}
}

func TestAdminOAuthCredentialHandlerImportRejectsDuplicateProviderEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &fakeOAuthCredentialManager{auths: []*coreauth.Auth{{ID: "existing", Provider: "claude", Attributes: map[string]string{coreauth.AttributeAuthKind: coreauth.AuthKindOAuth}, Metadata: map[string]any{"email": "admin@example.com"}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(manager, &fakeAdminAuditLogger{}).Import)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/oauth/import", strings.NewReader(`{"provider":"claude","attributes":{"auth_kind":"oauth"},"metadata":{"email":"admin@example.com","refresh_token":"secret-refresh"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || manager.registered != nil {
		t.Fatalf("status=%d registered=%+v", response.Code, manager.registered)
	}
}

type fakeOAuthCredentialManager struct {
	auths      []*coreauth.Auth
	registered *coreauth.Auth
}

func (m *fakeOAuthCredentialManager) GetByID(id string) (*coreauth.Auth, bool) {
	for _, auth := range m.auths {
		if auth.ID == id {
			return auth.Clone(), true
		}
	}
	return nil, false
}

func (m *fakeOAuthCredentialManager) List() []*coreauth.Auth {
	result := make([]*coreauth.Auth, 0, len(m.auths))
	for _, auth := range m.auths {
		result = append(result, auth.Clone())
	}
	return result
}

func (m *fakeOAuthCredentialManager) Register(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	m.registered = auth.Clone()
	registered := auth.Clone()
	registered.ID = "sdk-assigned"
	return registered, nil
}

type fakeAdminAuditLogger struct{ entry store.AdminAuditLogEntry }

func (l *fakeAdminAuditLogger) Insert(_ context.Context, entry store.AdminAuditLogEntry) error {
	l.entry = entry
	return nil
}
