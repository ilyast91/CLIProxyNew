package httpapi

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	"net/http"
	"net/netip"
	"time"
)

type adminOAuthSessionStore interface {
	Get(context.Context, string) (store.OAuthSession, error)
	ListPending(context.Context) ([]store.OAuthSession, error)
	CancelWithAudit(context.Context, int64, string, *netip.Addr) error
}

// AdminOAuthSessionHandler обслуживает безопасный status/cancel OAuth flow.
type AdminOAuthSessionHandler struct{ store adminOAuthSessionStore }
type oauthSessionResponse struct {
	State        string    `json:"state"`
	Provider     string    `json:"provider"`
	FlowType     string    `json:"flow_type"`
	Status       string    `json:"status"`
	AuthID       string    `json:"auth_id,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewAdminOAuthSessionHandler создаёт handler OAuth-сессий.
func NewAdminOAuthSessionHandler(s adminOAuthSessionStore) *AdminOAuthSessionHandler {
	return &AdminOAuthSessionHandler{store: s}
}

// ListPending возвращает pending OAuth-сессии без PKCE/device секретов.
func (h *AdminOAuthSessionHandler) ListPending(c *gin.Context) {
	if h == nil || h.store == nil {
		writeError(c, 500, "OAuth session service is unavailable")
		return
	}
	sessions, err := h.store.ListPending(c.Request.Context())
	if err != nil {
		writeError(c, 500, "list OAuth sessions failed")
		return
	}
	out := make([]oauthSessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, oauthSessionResponse{State: s.State, Provider: s.Provider, FlowType: s.FlowType, Status: s.Status, AuthID: s.AuthID, ErrorMessage: s.ErrorMessage, ExpiresAt: s.ExpiresAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt})
	}
	c.JSON(200, gin.H{"data": out})
}

// Get возвращает статус конкретной OAuth-сессии без секретных полей.
func (h *AdminOAuthSessionHandler) Get(c *gin.Context) {
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "OAuth session service is unavailable")
		return
	}
	session, err := h.store.Get(c.Request.Context(), c.Param("state"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(c, http.StatusNotFound, "OAuth session not found")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get OAuth session failed")
		return
	}
	c.JSON(http.StatusOK, oauthSessionResponse{State: session.State, Provider: session.Provider, FlowType: session.FlowType, Status: session.Status, AuthID: session.AuthID, ErrorMessage: session.ErrorMessage, ExpiresAt: session.ExpiresAt, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt})
}

// Cancel отменяет pending OAuth-сессию и пишет audit log.
func (h *AdminOAuthSessionHandler) Cancel(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.store == nil {
		writeError(c, 500, "OAuth session service is unavailable")
		return
	}
	state := c.Param("state")
	if state == "" {
		writeError(c, 400, "invalid OAuth state")
		return
	}
	if err := h.store.CancelWithAudit(c.Request.Context(), actor, state, requestActorIP(c)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, 404, "OAuth session not found")
			return
		}
		writeError(c, 500, "cancel OAuth session failed")
		return
	}
	c.Status(http.StatusNoContent)
}
