package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
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
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(manager, &fakeAdminAuditLogger{}).Import)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/oauth/import", strings.NewReader(`{"id":"foreign-id","provider":"claude","attributes":{"auth_kind":"oauth"},"metadata":{"email":"admin@example.com","refresh_token":"secret-refresh"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || manager.registered == nil || manager.registered.ID != "" {
		t.Fatalf("status=%d registered=%+v", response.Code, manager.registered)
	}
}

func TestAdminOAuthCredentialHandlerImportAcceptsMultipartJSONFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := &fakeOAuthCredentialManager{}
	audit := &fakeAdminAuditLogger{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(manager, audit).Import)
	request := multipartOAuthCredentialRequest(t, []byte(`{"provider":"claude","attributes":{"auth_kind":"oauth"},"metadata":{"email":"file@example.com","refresh_token":"secret-refresh"}}`), true)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || manager.registered == nil {
		t.Fatalf("status=%d registered=%+v body=%s", response.Code, manager.registered, response.Body.String())
	}
}

func TestAdminOAuthCredentialHandlerImportRejectsMultipartWithoutFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(&fakeOAuthCredentialManager{}, &fakeAdminAuditLogger{}).Import)
	request := multipartOAuthCredentialRequest(t, nil, false)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestAdminOAuthCredentialHandlerImportRejectsOversizedMultipartFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(&fakeOAuthCredentialManager{}, &fakeAdminAuditLogger{}).Import)
	payload := bytes.Repeat([]byte(" "), maxOAuthCredentialImportBytes+1)
	request := multipartOAuthCredentialRequest(t, payload, true)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want=%d body=%s", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
}

func TestAdminOAuthCredentialHandlerImportRejectsUnsupportedMediaType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/oauth/import", NewAdminOAuthCredentialHandler(&fakeOAuthCredentialManager{}, &fakeAdminAuditLogger{}).Import)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/oauth/import", strings.NewReader(`{"provider":"claude"}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d want=%d body=%s", response.Code, http.StatusUnsupportedMediaType, response.Body.String())
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

func multipartOAuthCredentialRequest(t *testing.T, payload []byte, includeFile bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeFile {
		part, err := writer.CreateFormFile("file", "oauth-credential.json")
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
	} else if err := writer.WriteField("note", "missing file"); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/oauth/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
