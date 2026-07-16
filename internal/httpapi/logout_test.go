package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestLogoutHandlerInvalidatesCachedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lookup := &logoutLookup{session: store.Session{UserID: 7, Role: identity.RoleUser}}
	authenticator := identity.NewCachedSessionAuthenticator(lookup, identity.SourceLDAP, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "opaque-token"})
	if _, err := authenticator.AuthenticateRequest(context.Background(), request); err != nil {
		t.Fatalf("AuthenticateRequest() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/v1/logout", LogoutHandler(lookup, identity.SourceLDAP, authenticator))
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "opaque-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, logoutRequest)

	if response.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", response.Code)
	}
	if _, err := authenticator.AuthenticateRequest(context.Background(), request); err == nil {
		t.Fatal("AuthenticateRequest() accepted a cached session after logout")
	}
}

type logoutLookup struct {
	session store.Session
	deleted bool
}

func (l *logoutLookup) GetByTokenForSource(context.Context, string, string) (store.Session, error) {
	if l.deleted {
		return store.Session{}, store.ErrInvalidCredential
	}
	return l.session, nil
}

func (l *logoutLookup) DeleteByTokenForSource(context.Context, string, string) error {
	l.deleted = true
	return nil
}
