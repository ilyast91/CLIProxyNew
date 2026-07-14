package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

const defaultUsagePeriod = 30 * 24 * time.Hour

type usageSummaryReader interface {
	GetSummaryByUser(context.Context, int64, time.Time, time.Time) (store.UsageSummary, error)
}

// UsageHandler обслуживает личную usage-статистику management API.
type UsageHandler struct {
	reader usageSummaryReader
	now    func() time.Time
}

type usageResponse struct {
	From               time.Time             `json:"from"`
	To                 time.Time             `json:"to"`
	RequestCount       int64                 `json:"request_count"`
	FailedRequestCount int64                 `json:"failed_request_count"`
	InputTokens        int64                 `json:"input_tokens"`
	OutputTokens       int64                 `json:"output_tokens"`
	ReasoningTokens    int64                 `json:"reasoning_tokens"`
	CachedTokens       int64                 `json:"cached_tokens"`
	TotalTokens        int64                 `json:"total_tokens"`
	ByModel            []usageModelResponse  `json:"by_model"`
	ByAPIKey           []usageAPIKeyResponse `json:"by_api_key"`
}

type usageModelResponse struct {
	Model              string `json:"model"`
	RequestCount       int64  `json:"request_count"`
	FailedRequestCount int64  `json:"failed_request_count"`
	TotalTokens        int64  `json:"total_tokens"`
}

type usageAPIKeyResponse struct {
	APIKeyID           int64 `json:"api_key_id"`
	RequestCount       int64 `json:"request_count"`
	FailedRequestCount int64 `json:"failed_request_count"`
	TotalTokens        int64 `json:"total_tokens"`
}

// NewUsageHandler создаёт handler личной usage-статистики.
func NewUsageHandler(reader usageSummaryReader) *UsageHandler {
	return &UsageHandler{reader: reader, now: time.Now}
}

// Get возвращает статистику текущего пользователя за заданный интервал.
func (h *UsageHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.reader == nil || h.now == nil {
		writeError(c, http.StatusInternalServerError, "usage service is unavailable")
		return
	}
	from, to, err := usageRange(c, h.now().UTC())
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid usage interval")
		return
	}
	summary, err := h.reader.GetSummaryByUser(c.Request.Context(), userID, from, to)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get usage failed")
		return
	}
	c.JSON(http.StatusOK, usageResponseFromStore(summary, from, to))
}

func usageRange(c *gin.Context, now time.Time) (time.Time, time.Time, error) {
	to, err := parseUsageTime(c.Query("to"), now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	from, err := parseUsageTime(c.Query("from"), to.Add(-defaultUsagePeriod))
	if err != nil || !from.Before(to) {
		return time.Time{}, time.Time{}, store.ErrInvalidInput
	}
	return from, to, nil
}

func parseUsageTime(raw string, defaultValue time.Time) (time.Time, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func usageResponseFromStore(summary store.UsageSummary, from, to time.Time) usageResponse {
	response := usageResponse{
		From: from, To: to,
		RequestCount: summary.RequestCount, FailedRequestCount: summary.FailedRequestCount,
		InputTokens: summary.InputTokens, OutputTokens: summary.OutputTokens,
		ReasoningTokens: summary.ReasoningTokens, CachedTokens: summary.CachedTokens,
		TotalTokens: summary.TotalTokens,
		ByModel:     make([]usageModelResponse, 0, len(summary.ByModel)),
		ByAPIKey:    make([]usageAPIKeyResponse, 0, len(summary.ByAPIKey)),
	}
	for _, model := range summary.ByModel {
		response.ByModel = append(response.ByModel, usageModelResponse{
			Model: model.Model, RequestCount: model.RequestCount,
			FailedRequestCount: model.FailedRequestCount, TotalTokens: model.TotalTokens,
		})
	}
	for _, key := range summary.ByAPIKey {
		response.ByAPIKey = append(response.ByAPIKey, usageAPIKeyResponse{
			APIKeyID: key.APIKeyID, RequestCount: key.RequestCount,
			FailedRequestCount: key.FailedRequestCount, TotalTokens: key.TotalTokens,
		})
	}
	return response
}
