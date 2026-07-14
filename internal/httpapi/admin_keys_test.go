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

func TestAdminAPIKeyHandlerListReturnsSafeOwnerMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keyStore := &fakeAdminAPIKeyStore{keys: []store.AdminAPIKey{{
		APIKey:        store.APIKey{ID: 7, UserID: 42, Prefix: "cpn_live", Name: "laptop", Status: "active"},
		OwnerUsername: "ivanov", OwnerIdentitySource: "ldap", OwnerStatus: "active",
	}}}
	router := gin.New()
	router.GET("/api/v1/admin/keys", NewAdminAPIKeyHandler(keyStore).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/keys", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []adminAPIKeyResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != 7 || body.Data[0].OwnerUsername != "ivanov" || body.Data[0].OwnerStatus != "active" {
		t.Fatalf("data = %+v", body.Data)
	}
	if got := response.Body.String(); strings.Contains(got, "key_hash") || strings.Contains(got, "plaintext") {
		t.Fatalf("response exposed secret fields: %s", got)
	}
}

func TestAdminAPIKeyHandlerListMapsFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/admin/keys", NewAdminAPIKeyHandler(&fakeAdminAPIKeyStore{err: store.ErrInvalidInput}).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/keys", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

type fakeAdminAPIKeyStore struct {
	keys []store.AdminAPIKey
	err  error
}

func (s *fakeAdminAPIKeyStore) ListAll(context.Context) ([]store.AdminAPIKey, error) {
	return s.keys, s.err
}
