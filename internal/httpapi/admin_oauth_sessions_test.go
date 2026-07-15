package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestAdminOAuthSessionHandlerListHidesSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAdminOAuthSessionHandler(&fakeAdminOAuthSessions{sessions: []store.OAuthSession{{State: "state", Provider: "claude", FlowType: "callback", Status: "pending", PKCEVerifier: "secret", DeviceCode: "device", ExpiresAt: time.Now()}}})
	router := gin.New()
	router.GET("/api/v1/admin/oauth/sessions", handler.ListPending)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/oauth/sessions", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "device") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []oauthSessionResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || len(body.Data) != 1 || body.Data[0].State != "state" {
		t.Fatalf("body=%+v err=%v", body, err)
	}
}

func TestAdminOAuthSessionHandlerCancelMapsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeAdminOAuthSessions{cancelErr: store.ErrNotFound}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.DELETE("/api/v1/admin/oauth/sessions/:state", NewAdminOAuthSessionHandler(store).Cancel)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/oauth/sessions/missing", nil))
	if response.Code != http.StatusNotFound || store.actor != 42 || store.state != "missing" {
		t.Fatalf("status=%d actor=%d state=%q", response.Code, store.actor, store.state)
	}
}

type fakeAdminOAuthSessions struct {
	sessions  []store.OAuthSession
	err       error
	cancelErr error
	actor     int64
	state     string
}

func (s *fakeAdminOAuthSessions) Get(context.Context, string) (store.OAuthSession, error) {
	if s.cancelErr != nil {
		return store.OAuthSession{}, s.cancelErr
	}
	if len(s.sessions) == 0 {
		return store.OAuthSession{}, store.ErrNotFound
	}
	return s.sessions[0], nil
}

func (s *fakeAdminOAuthSessions) ListPending(context.Context) ([]store.OAuthSession, error) {
	return s.sessions, s.err
}
func (s *fakeAdminOAuthSessions) CancelWithAudit(_ context.Context, actor int64, state string, _ *netip.Addr) error {
	s.actor = actor
	s.state = state
	return s.cancelErr
}
