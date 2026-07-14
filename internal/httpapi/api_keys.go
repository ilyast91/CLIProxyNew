package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type apiKeyStore interface {
	ListByUser(context.Context, int64) ([]store.APIKey, error)
	Create(context.Context, store.CreateAPIKeyParams) (store.APIKey, error)
	Revoke(context.Context, int64, int64) error
}

// APIKeyHandler обслуживает management API-ключи текущего пользователя.
type APIKeyHandler struct {
	store  apiKeyStore
	newKey func() (string, error)
}

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

type createAPIKeyRequest struct {
	Name      string          `json:"name"`
	ExpiresAt *time.Time      `json:"expires_at"`
	Scope     json.RawMessage `json:"scope"`
}

type createAPIKeyResponse struct {
	APIKey apiKeyResponse `json:"api_key"`
	Key    string         `json:"key"`
}

// NewAPIKeyHandler создаёт handler API-key metadata.
func NewAPIKeyHandler(keyStore apiKeyStore) *APIKeyHandler {
	return &APIKeyHandler{store: keyStore, newKey: generateAPIKey}
}

// List возвращает безопасные метаданные API-ключей текущего пользователя.
func (h *APIKeyHandler) List(c *gin.Context) {
	userID, ok := c.Get(ContextUserID)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := userID.(int64)
	if !ok || h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "API key service is unavailable")
		return
	}
	keys, err := h.store.ListByUser(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list API keys failed")
		return
	}
	response := make([]apiKeyResponse, 0, len(keys))
	for _, key := range keys {
		response = append(response, newAPIKeyResponse(key))
	}
	c.JSON(http.StatusOK, gin.H{"data": response})
}

// Create выпускает API-ключ и возвращает его открытое значение только один раз.
func (h *APIKeyHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.store == nil || h.newKey == nil {
		writeError(c, http.StatusInternalServerError, "API key service is unavailable")
		return
	}

	var request createAPIKeyRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validAPIKeyScope(request.Scope) {
		writeError(c, http.StatusBadRequest, "invalid API key request")
		return
	}
	plaintext, err := h.newKey()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "generate API key failed")
		return
	}
	key, err := h.store.Create(c.Request.Context(), store.CreateAPIKeyParams{
		UserID: userID, Plaintext: plaintext, Name: request.Name, ExpiresAt: request.ExpiresAt, Scope: request.Scope,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "create API key failed")
		return
	}
	c.JSON(http.StatusCreated, createAPIKeyResponse{APIKey: newAPIKeyResponse(key), Key: plaintext})
}

// Revoke отзывает API-ключ текущего пользователя.
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "API key service is unavailable")
		return
	}
	keyID, err := strconv.ParseInt(c.Param("keyID"), 10, 64)
	if err != nil || keyID <= 0 {
		writeError(c, http.StatusBadRequest, "invalid API key ID")
		return
	}
	if err := h.store.Revoke(c.Request.Context(), userID, keyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, "API key not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "revoke API key failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func currentUserID(c *gin.Context) (int64, bool) {
	userID, ok := c.Get(ContextUserID)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	id, ok := userID.(int64)
	if !ok || id <= 0 {
		writeError(c, http.StatusInternalServerError, "invalid session principal")
		return 0, false
	}
	return id, true
}

func newAPIKeyResponse(key store.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:         key.ID,
		Prefix:     key.Prefix,
		Name:       key.Name,
		Status:     key.Status,
		ExpiresAt:  key.ExpiresAt,
		Scope:      key.Scope,
		LastUsedAt: key.LastUsedAt,
		CreatedAt:  key.CreatedAt,
	}
}

func validAPIKeyScope(scope json.RawMessage) bool {
	if len(scope) == 0 {
		return true
	}
	scope = bytes.TrimSpace(scope)
	if !json.Valid(scope) {
		return false
	}
	return bytes.Equal(scope, []byte("null")) || scope[0] == '{' || scope[0] == '['
}

func generateAPIKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "cpn_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
