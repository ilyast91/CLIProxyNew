package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type adminModelStore interface {
	List(context.Context) ([]store.ModelOverride, error)
	UpsertWithAudit(context.Context, int64, store.UpsertModelOverrideParams, *netip.Addr) (store.ModelOverride, error)
	DeleteWithAudit(context.Context, int64, string, *netip.Addr) error
}

type upsertModelRequest struct {
	Provider      string          `json:"provider"`
	UpstreamModel string          `json:"upstream_model"`
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`
}

// AdminModelHandler обслуживает admin обзор model overrides.
type AdminModelHandler struct{ store adminModelStore }
type adminModelResponse struct {
	ID            int64           `json:"id"`
	Provider      string          `json:"provider"`
	ModelAlias    string          `json:"model_alias"`
	UpstreamModel string          `json:"upstream_model"`
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`
}

// Upsert сохраняет allow-list/model mapping и audit log.
func (h *AdminModelHandler) Upsert(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	alias := c.Param("modelAlias")
	var req upsertModelRequest
	if h == nil || h.store == nil {
		writeError(c, 500, "model override service is unavailable")
		return
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid model override")
		return
	}
	v, err := h.store.UpsertWithAudit(c.Request.Context(), actor, store.UpsertModelOverrideParams{Provider: req.Provider, ModelAlias: alias, UpstreamModel: req.UpstreamModel, Enabled: req.Enabled, Config: req.Config}, requestActorIP(c))
	if err != nil {
		writeError(c, 400, "invalid model override")
		return
	}
	c.JSON(http.StatusOK, adminModelResponse{ID: v.ID, Provider: v.Provider, ModelAlias: v.ModelAlias, UpstreamModel: v.UpstreamModel, Enabled: v.Enabled, Config: v.Config})
}

// Delete удаляет model override и пишет audit log.
func (h *AdminModelHandler) Delete(c *gin.Context) {
	actor, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.store == nil {
		writeError(c, 500, "model override service is unavailable")
		return
	}
	err := h.store.DeleteWithAudit(c.Request.Context(), actor, c.Param("modelAlias"), requestActorIP(c))
	if errors.Is(err, store.ErrNotFound) {
		writeError(c, 404, "model override not found")
		return
	}
	if err != nil {
		writeError(c, 400, "invalid model override")
		return
	}
	c.Status(http.StatusNoContent)
}

// NewAdminModelHandler создаёт handler model overrides.
func NewAdminModelHandler(s adminModelStore) *AdminModelHandler { return &AdminModelHandler{store: s} }

// List возвращает allow-list и mapping моделей без обращения к SDK реестру.
func (h *AdminModelHandler) List(c *gin.Context) {
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "model override service is unavailable")
		return
	}
	rows, err := h.store.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list model overrides failed")
		return
	}
	out := make([]adminModelResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, adminModelResponse{ID: r.ID, Provider: r.Provider, ModelAlias: r.ModelAlias, UpstreamModel: r.UpstreamModel, Enabled: r.Enabled, Config: r.Config})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
