package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type adminModelStore interface {
	List(context.Context) ([]store.ModelOverride, error)
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
