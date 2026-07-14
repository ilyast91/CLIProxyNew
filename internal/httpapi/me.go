package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CurrentUserHandler возвращает principal текущей management-сессии.
func CurrentUserHandler(c *gin.Context) {
	userID, ok := c.Get(ContextUserID)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	role, _ := c.Get(ContextRole)
	c.JSON(http.StatusOK, gin.H{"user_id": userID, "role": role})
}
