package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type upstreamAccountLookup interface {
	GetByID(string) (*coreauth.Auth, bool)
}

type quotaResponse struct {
	AccountID          string                  `json:"account_id"`
	Provider           string                  `json:"provider"`
	ExpiresAt          *time.Time              `json:"expires_at,omitempty"`
	Quota              runtimeQuotaResponse    `json:"quota"`
	AntigravityCredits *antigravityCreditsInfo `json:"antigravity_credits,omitempty"`
}

type runtimeQuotaResponse struct {
	Exceeded      bool       `json:"exceeded"`
	Reason        string     `json:"reason,omitempty"`
	NextRecoverAt *time.Time `json:"next_recover_at,omitempty"`
	BackoffLevel  int        `json:"backoff_level,omitempty"`
	Unknown       bool       `json:"unknown"`
}

type antigravityCreditsInfo struct {
	Known           bool       `json:"known"`
	Available       bool       `json:"available"`
	CreditAmount    float64    `json:"credit_amount,omitempty"`
	MinCreditAmount float64    `json:"min_credit_amount,omitempty"`
	PaidTierID      string     `json:"paid_tier_id,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// AdminQuotaHandler возвращает безопасный runtime-срез upstream квоты.
type AdminQuotaHandler struct {
	lookup  upstreamAccountLookup
	credits func(string) (coreauth.AntigravityCreditsHint, bool)
}

// NewAdminQuotaHandler создаёт handler просмотра runtime-квоты upstream аккаунта.
func NewAdminQuotaHandler(lookup upstreamAccountLookup) *AdminQuotaHandler {
	return &AdminQuotaHandler{lookup: lookup, credits: coreauth.GetAntigravityCreditsHint}
}

// Get возвращает реактивную квоту ядра; unknown не означает доступную квоту.
func (h *AdminQuotaHandler) Get(c *gin.Context) {
	if h == nil || h.lookup == nil {
		writeError(c, http.StatusInternalServerError, "upstream quota service is unavailable")
		return
	}
	accountID := strings.TrimSpace(c.Param("accountID"))
	auth, ok := h.lookup.GetByID(accountID)
	if !ok || auth == nil {
		writeError(c, http.StatusNotFound, "upstream account not found")
		return
	}
	response := quotaResponse{AccountID: auth.ID, Provider: auth.Provider, Quota: newRuntimeQuotaResponse(auth.Quota)}
	if expiresAt, ok := auth.ExpirationTime(); ok {
		response.ExpiresAt = &expiresAt
	}
	if strings.EqualFold(auth.Provider, "antigravity") && h.credits != nil {
		hint, found := h.credits(auth.ID)
		response.AntigravityCredits = newAntigravityCreditsInfo(hint, found)
	}
	c.JSON(http.StatusOK, response)
}

func newRuntimeQuotaResponse(quota coreauth.QuotaState) runtimeQuotaResponse {
	response := runtimeQuotaResponse{
		Exceeded: quota.Exceeded, Reason: quota.Reason, BackoffLevel: quota.BackoffLevel,
		Unknown: !quota.Exceeded && quota.Reason == "" && quota.NextRecoverAt.IsZero() && quota.BackoffLevel == 0,
	}
	if !quota.NextRecoverAt.IsZero() {
		response.NextRecoverAt = &quota.NextRecoverAt
	}
	return response
}

func newAntigravityCreditsInfo(hint coreauth.AntigravityCreditsHint, found bool) *antigravityCreditsInfo {
	response := &antigravityCreditsInfo{Known: found && hint.Known}
	if !response.Known {
		return response
	}
	response.Available = hint.Available
	response.CreditAmount = hint.CreditAmount
	response.MinCreditAmount = hint.MinCreditAmount
	response.PaidTierID = hint.PaidTierID
	if !hint.UpdatedAt.IsZero() {
		response.UpdatedAt = &hint.UpdatedAt
	}
	return response
}
