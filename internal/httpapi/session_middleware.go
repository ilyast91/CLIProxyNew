package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

const (
	// ContextUserID — ключ Gin context с ID session-пользователя.
	ContextUserID = "cliproxy.user_id"
	// ContextRole — ключ Gin context с ролью session-пользователя.
	ContextRole = "cliproxy.role"
)

// SessionMiddleware аутентифицирует management-запрос по opaque session-cookie.
func SessionMiddleware(authenticator *identity.SessionAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := authenticator.AuthenticateRequest(c.Request.Context(), c.Request)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized")
			return
		}
		c.Set(ContextUserID, principal.UserID)
		c.Set(ContextRole, principal.Role)
		c.Next()
	}
}

// RequireRole разрешает handler только при одной из переданных ролей.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := c.Get(ContextRole)
		if !ok {
			writeError(c, http.StatusUnauthorized, "unauthorized")
			return
		}
		roleValue, ok := role.(string)
		if !ok || !allowedRole(roleValue, roles) {
			writeError(c, http.StatusForbidden, "access denied")
			return
		}
		c.Next()
	}
}

func allowedRole(role string, allowed []string) bool {
	for _, candidate := range allowed {
		if role == candidate {
			return true
		}
	}
	return false
}
