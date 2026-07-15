package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	authtesting "github.com/ilyast91/CLIProxyNew/internal/auth/testing"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type upstreamAccountChecker interface {
	Test(context.Context, string) (authtesting.Result, error)
}

// AdminAccountTestHandler проверяет upstream credential без inference-запроса.
type AdminAccountTestHandler struct{ checker upstreamAccountChecker }

// NewAdminAccountTestHandler создаёт handler проверки upstream аккаунта.
func NewAdminAccountTestHandler(checker upstreamAccountChecker) *AdminAccountTestHandler {
	return &AdminAccountTestHandler{checker: checker}
}

// Test запускает refresh или metadata-probe согласно типу credential.
func (h *AdminAccountTestHandler) Test(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.checker == nil {
		writeError(c, http.StatusInternalServerError, "upstream account checker is unavailable")
		return
	}
	accountID := strings.TrimSpace(c.Param("accountID"))
	if accountID == "" {
		writeError(c, http.StatusBadRequest, "invalid upstream account ID")
		return
	}
	ctx := store.WithUpstreamAccountAudit(c.Request.Context(), store.AdminAuditLogEntry{
		ActorUserID: actor,
		Action:      "upstream_account.tested",
		TargetType:  "upstream_account",
		TargetID:    "pending",
		ActorIP:     requestActorIP(c),
	})
	result, err := h.checker.Test(ctx, accountID)
	if errors.Is(err, authtesting.ErrAccountNotFound) {
		writeError(c, http.StatusNotFound, "upstream account not found")
		return
	}
	if errors.Is(err, authtesting.ErrUnsupportedProvider) {
		writeError(c, http.StatusBadRequest, "unsupported upstream account health check")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "upstream account test failed")
		return
	}
	c.JSON(http.StatusOK, result)
}
