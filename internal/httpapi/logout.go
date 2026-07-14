package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

type sessionDeleter interface {
	DeleteByTokenForSource(context.Context, string, string) error
}

// LogoutHandler удаляет текущую opaque session-cookie.
func LogoutHandler(deleter sessionDeleter, identitySource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Request.Cookie(identity.SessionCookieName)
		if err == nil && deleter != nil {
			_ = deleter.DeleteByTokenForSource(c.Request.Context(), cookie.Value, identitySource)
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: identity.SessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		c.Status(http.StatusNoContent)
	}
}
