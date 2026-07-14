package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type apiKeyLister interface {
	ListByUser(context.Context, int64) ([]store.APIKey, error)
}

// APIKeyHandler обслуживает management API-ключи текущего пользователя.
type APIKeyHandler struct{ lister apiKeyLister }

type apiKeyResponse struct {
	ID         int64           `json:"id"`
	Prefix     string          `json:"prefix"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	ExpiresAt  *time.Time      `json:"expires_at"`
	Scope      json.RawMessage `json:"scope"`
	LastUsedAt *time.Time      `json:"last_used_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

// NewAPIKeyHandler создаёт handler API-key metadata.
func NewAPIKeyHandler(lister apiKeyLister) *APIKeyHandler { return &APIKeyHandler{lister: lister} }

// List возвращает безопасные метаданные API-ключей текущего пользователя.
func (h *APIKeyHandler) List(c *gin.Context) {
	userID, ok := c.Get(ContextUserID)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := userID.(int64)
	if !ok || h == nil || h.lister == nil {
		writeError(c, http.StatusInternalServerError, "API key service is unavailable")
		return
	}
	keys, err := h.lister.ListByUser(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list API keys failed")
		return
	}
	response := make([]apiKeyResponse, 0, len(keys))
	for _, key := range keys {
		response = append(response, apiKeyResponse{
			ID:         key.ID,
			Prefix:     key.Prefix,
			Name:       key.Name,
			Status:     key.Status,
			ExpiresAt:  key.ExpiresAt,
			Scope:      key.Scope,
			LastUsedAt: key.LastUsedAt,
			CreatedAt:  key.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": response})
}
