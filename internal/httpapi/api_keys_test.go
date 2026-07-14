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
	router.GET("/api/v1/me/keys", NewAPIKeyHandler(fakeAPIKeyLister{
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
	router.GET("/api/v1/me/keys", NewAPIKeyHandler(fakeAPIKeyLister{}).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/keys", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

type fakeAPIKeyLister struct {
	keys []store.APIKey
	err  error
}

func (l fakeAPIKeyLister) ListByUser(context.Context, int64) ([]store.APIKey, error) {
	return l.keys, l.err
}
