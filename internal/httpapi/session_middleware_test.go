package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestSessionMiddlewareSetsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lookup := &sessionLookup{session: store.Session{UserID: 42, Role: identity.RoleAdmin}}
	router := gin.New()
	protected := router.Group("/api/v1/me", SessionMiddleware(identity.NewSessionAuthenticator(lookup, identity.SourceStatic)))
	protected.GET("/profile", func(c *gin.Context) {
		userID, _ := c.Get(ContextUserID)
		role, _ := c.Get(ContextRole)
		if userID != int64(42) || role != identity.RoleAdmin {
			t.Fatalf("context user_id=%v role=%v", userID, role)
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/profile", nil)
	request.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "opaque"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSessionMiddlewareAndRoleGuardRejectUnauthorizedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api/v1/admin", SessionMiddleware(identity.NewSessionAuthenticator(&sessionLookup{session: store.Session{UserID: 42, Role: identity.RoleUser}}, identity.SourceLDAP)))
	protected.GET("/users", RequireRole(identity.RoleAdmin), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	missingCookie := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	missingResponse := httptest.NewRecorder()
	router.ServeHTTP(missingResponse, missingCookie)
	if missingResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d", missingResponse.Code)
	}

	userSession := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	userSession.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "opaque"})
	userResponse := httptest.NewRecorder()
	router.ServeHTTP(userResponse, userSession)
	if userResponse.Code != http.StatusForbidden {
		t.Fatalf("user role status = %d", userResponse.Code)
	}
}

type sessionLookup struct {
	session store.Session
	err     error
}

func (l *sessionLookup) GetByTokenForSource(context.Context, string, string) (store.Session, error) {
	return l.session, l.err
}
