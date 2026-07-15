package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type upstreamAuthRegistrar interface {
	Register(context.Context, *coreauth.Auth) (*coreauth.Auth, error)
}

type createProviderKeysRequest struct {
	Accounts []createProviderKeyAccount `json:"accounts"`
}

type createProviderKeyAccount struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Label    string `json:"label"`
}

type providerKeyResponse struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Label    string `json:"label,omitempty"`
}

// AdminProviderKeyHandler регистрирует upstream API-keys через public SDK contract.
type AdminProviderKeyHandler struct{ registrar upstreamAuthRegistrar }

// NewAdminProviderKeyHandler создаёт handler batch-регистрации upstream API-keys.
func NewAdminProviderKeyHandler(registrar upstreamAuthRegistrar) *AdminProviderKeyHandler {
	return &AdminProviderKeyHandler{registrar: registrar}
}

// Create регистрирует набор статических upstream API-keys без раскрытия секретов в ответе.
func (h *AdminProviderKeyHandler) Create(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.registrar == nil {
		writeError(c, http.StatusInternalServerError, "provider key service is unavailable")
		return
	}

	var request createProviderKeysRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderKeyAccounts(request.Accounts) {
		writeError(c, http.StatusBadRequest, "invalid provider key batch")
		return
	}

	response := make([]providerKeyResponse, 0, len(request.Accounts))
	for _, account := range request.Accounts {
		provider := strings.TrimSpace(account.Provider)
		label := strings.TrimSpace(account.Label)
		baseURL := strings.TrimSpace(account.BaseURL)
		auth := &coreauth.Auth{
			Provider: provider,
			Label:    label,
			Attributes: map[string]string{
				coreauth.AttributeAuthKind: coreauth.AuthKindAPIKey,
				coreauth.AttributeAPIKey:   strings.TrimSpace(account.APIKey),
			},
		}
		if baseURL != "" {
			auth.Attributes["base_url"] = baseURL
		}
		details, _ := json.Marshal(map[string]string{"provider": provider, "label": label, "base_url": baseURL})
		ctx := store.WithUpstreamAccountAudit(c.Request.Context(), store.AdminAuditLogEntry{
			ActorUserID: actor,
			Action:      "upstream_api_key.created",
			TargetType:  "upstream_account",
			TargetID:    "pending",
			Details:     details,
			ActorIP:     requestActorIP(c),
		})
		registered, err := h.registrar.Register(ctx, auth)
		if err != nil || registered == nil || strings.TrimSpace(registered.ID) == "" {
			writeError(c, http.StatusInternalServerError, "register provider key failed")
			return
		}
		response = append(response, providerKeyResponse{ID: registered.ID, Provider: registered.Provider, Label: registered.Label})
	}
	c.JSON(http.StatusCreated, gin.H{"data": response})
}

func validProviderKeyAccounts(accounts []createProviderKeyAccount) bool {
	if len(accounts) == 0 || len(accounts) > 100 {
		return false
	}
	for _, account := range accounts {
		if strings.TrimSpace(account.Provider) == "" || strings.TrimSpace(account.APIKey) == "" {
			return false
		}
		if baseURL := strings.TrimSpace(account.BaseURL); baseURL != "" {
			parsed, err := url.ParseRequestURI(baseURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
				return false
			}
		}
	}
	return true
}
