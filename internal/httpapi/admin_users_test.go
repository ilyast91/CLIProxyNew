package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestAdminUserHandlerListReturnsUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeAdminUserStore{users: []store.User{{ID: 7, Username: "ivanov", Role: "user", Status: "active", IdentitySource: "ldap"}}}
	router := gin.New()
	router.GET("/api/v1/admin/users", NewAdminUserHandler(store).List)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []adminUserResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != 7 || body.Data[0].IdentitySource != "ldap" {
		t.Fatalf("data = %+v", body.Data)
	}
}

func TestAdminUserHandlerSetStatusWritesActorAndTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeAdminUserStore{}
	invalidator := &fakeSessionInvalidator{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.PATCH("/api/v1/admin/users/:userID", NewAdminUserHandler(store, invalidator).SetStatus)

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/7", strings.NewReader(`{"status":"blocked"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.5:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if store.actorUserID != 42 || store.targetUserID != 7 || store.status != "blocked" {
		t.Fatalf("arguments = actor %d target %d status %q", store.actorUserID, store.targetUserID, store.status)
	}
	if invalidator.userID != 7 {
		t.Fatalf("InvalidateUser() user ID = %d, want 7", invalidator.userID)
	}
}

type fakeSessionInvalidator struct{ userID int64 }

func (i *fakeSessionInvalidator) InvalidateUser(userID int64) { i.userID = userID }

func TestAdminUserHandlerSetStatusMapsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeAdminUserStore{statusErr: store.ErrNotFound}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.PATCH("/api/v1/admin/users/:userID", NewAdminUserHandler(store).SetStatus)

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/7", strings.NewReader(`{"status":"active"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAdminUserHandlerSetStatusRejectsInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeAdminUserStore{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.PATCH("/api/v1/admin/users/:userID", NewAdminUserHandler(store).SetStatus)

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/0", strings.NewReader(`{"status":"disabled"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || store.called {
		t.Fatalf("status = %d, called = %t", response.Code, store.called)
	}
}

type fakeAdminUserStore struct {
	users []store.User
	err   error

	called       bool
	actorUserID  int64
	targetUserID int64
	status       string
	statusErr    error
}

func (s *fakeAdminUserStore) List(context.Context) ([]store.User, error) {
	return s.users, s.err
}

func (s *fakeAdminUserStore) SetStatusWithAudit(_ context.Context, actorUserID, targetUserID int64, status string, _ *netip.Addr) error {
	s.called = true
	s.actorUserID = actorUserID
	s.targetUserID = targetUserID
	s.status = status
	return s.statusErr
}
