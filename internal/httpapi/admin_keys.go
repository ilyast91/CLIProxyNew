package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type adminAPIKeyStore interface {
	ListAll(context.Context) ([]store.AdminAPIKey, error)
}

// AdminAPIKeyHandler обслуживает read-only admin обзор пользовательских ключей.
type AdminAPIKeyHandler struct{ store adminAPIKeyStore }

type adminAPIKeyResponse struct {
	ID                  int64           `json:"id"`
	UserID              int64           `json:"user_id"`
	Prefix              string          `json:"prefix"`
	Name                string          `json:"name"`
	Status              string          `json:"status"`
	ExpiresAt           *time.Time      `json:"expires_at"`
	Scope               json.RawMessage `json:"scope"`
	LastUsedAt          *time.Time      `json:"last_used_at"`
	CreatedAt           time.Time       `json:"created_at"`
	OwnerUsername       string          `json:"owner_username"`
	OwnerIdentitySource string          `json:"owner_identity_source"`
	OwnerStatus         string          `json:"owner_status"`
}

// NewAdminAPIKeyHandler создаёт handler admin обзора API-ключей.
func NewAdminAPIKeyHandler(keyStore adminAPIKeyStore) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{store: keyStore}
}

// List возвращает безопасные metadata всех пользовательских API-ключей.
func (h *AdminAPIKeyHandler) List(c *gin.Context) {
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "admin API key service is unavailable")
		return
	}
	keys, err := h.store.ListAll(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list API keys failed")
		return
	}
	response := make([]adminAPIKeyResponse, 0, len(keys))
	for _, key := range keys {
		response = append(response, adminAPIKeyResponse{
			ID: key.ID, UserID: key.UserID, Prefix: key.Prefix, Name: key.Name, Status: key.Status,
			ExpiresAt: key.ExpiresAt, Scope: key.Scope, LastUsedAt: key.LastUsedAt, CreatedAt: key.CreatedAt,
			OwnerUsername: key.OwnerUsername, OwnerIdentitySource: key.OwnerIdentitySource, OwnerStatus: key.OwnerStatus,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": response})
}
