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
)

func TestAPIKeyHandlerListReturnsCurrentUserKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ContextUserID, int64(42))
	})
	router.GET("/api/v1/me/keys", NewAPIKeyHandler(&fakeAPIKeyStore{
		keys: []store.APIKey{{ID: 7, UserID: 42, Prefix: "cpn_live", Name: "local", Status: "active"}},
	}).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/keys", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []apiKeyResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != 7 || body.Data[0].Prefix != "cpn_live" {
		t.Fatalf("data = %+v", body.Data)
	}
	if got := response.Body.String(); strings.Contains(got, "key_hash") || strings.Contains(got, "UserID") {
		t.Fatalf("response exposed persistence fields: %s", got)
	}
}

func TestAPIKeyHandlerListRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/me/keys", NewAPIKeyHandler(&fakeAPIKeyStore{}).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/keys", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAPIKeyHandlerCreateReturnsPlaintextOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyStore := &fakeAPIKeyStore{created: store.APIKey{ID: 7, UserID: 42, Prefix: "cpn_live", Name: "local", Status: "active"}}
	handler := NewAPIKeyHandler(keyStore)
	handler.newKey = func() (string, error) { return "cpn_live_secret", nil }
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/me/keys", handler.Create)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/keys", strings.NewReader(`{"name":"local","scope":{"models":["gpt-5"]}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if keyStore.params.UserID != 42 || keyStore.params.Plaintext != "cpn_live_secret" || keyStore.params.Name != "local" {
		t.Fatalf("create params = %+v", keyStore.params)
	}
	if string(keyStore.params.Scope) != `{"models":["gpt-5"]}` {
		t.Fatalf("scope = %s", keyStore.params.Scope)
	}
	var body createAPIKeyResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Key != "cpn_live_secret" || body.APIKey.ID != 7 {
		t.Fatalf("response = %+v", body)
	}
}

func TestAPIKeyHandlerCreateRejectsInvalidScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyStore := &fakeAPIKeyStore{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/me/keys", NewAPIKeyHandler(keyStore).Create)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/keys", strings.NewReader(`{"scope":"all"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if keyStore.createCalled {
		t.Fatal("repository Create() called for invalid request")
	}
}

func TestAPIKeyHandlerRevokeUsesCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyStore := &fakeAPIKeyStore{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.DELETE("/api/v1/me/keys/:keyID", NewAPIKeyHandler(keyStore).Revoke)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/me/keys/7", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if keyStore.revokeUserID != 42 || keyStore.revokeKeyID != 7 {
		t.Fatalf("revoke arguments = user %d, key %d", keyStore.revokeUserID, keyStore.revokeKeyID)
	}
}

func TestAPIKeyHandlerRevokeMapsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyStore := &fakeAPIKeyStore{revokeErr: store.ErrNotFound}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.DELETE("/api/v1/me/keys/:keyID", NewAPIKeyHandler(keyStore).Revoke)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/me/keys/8", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGenerateAPIKeyUsesExpectedPrefix(t *testing.T) {
	key, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generate API key: %v", err)
	}
	if !strings.HasPrefix(key, "cpn_") || len(key) <= store.APIKeyPrefixLength {
		t.Fatalf("key = %q", key)
	}
}

type fakeAPIKeyStore struct {
	keys []store.APIKey
	err  error

	params       store.CreateAPIKeyParams
	created      store.APIKey
	createErr    error
	createCalled bool
	revokeUserID int64
	revokeKeyID  int64
	revokeErr    error
}

func (s *fakeAPIKeyStore) ListByUser(context.Context, int64) ([]store.APIKey, error) {
	return s.keys, s.err
}

func (s *fakeAPIKeyStore) Create(_ context.Context, params store.CreateAPIKeyParams) (store.APIKey, error) {
	s.createCalled = true
	s.params = params
	return s.created, s.createErr
}

func (s *fakeAPIKeyStore) Revoke(_ context.Context, userID, keyID int64) error {
	s.revokeUserID = userID
	s.revokeKeyID = keyID
	return s.revokeErr
}
