package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

func TestUsageHandlerGetReturnsCurrentUserSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	from := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC)
	reader := &fakeUsageSummaryReader{summary: store.UsageSummary{
		RequestCount: 3, FailedRequestCount: 1, TotalTokens: 42,
		ByModel:  []store.UsageModelSummary{{Model: "gpt-5", RequestCount: 3, TotalTokens: 42}},
		ByAPIKey: []store.UsageAPIKeySummary{{APIKeyID: 7, RequestCount: 3, TotalTokens: 42}},
	}}
	handler := NewUsageHandler(reader)
	handler.now = func() time.Time { return to }
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.GET("/api/v1/me/usage", handler.Get)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/usage?from=2026-07-01T00:00:00Z&to=2026-07-02T00:00:00Z", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if reader.userID != 42 || !reader.from.Equal(from) || !reader.to.Equal(to) {
		t.Fatalf("query = user %d, from %s, to %s", reader.userID, reader.from, reader.to)
	}
	var body usageResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.TotalTokens != 42 || len(body.ByModel) != 1 || body.ByModel[0].Model != "gpt-5" || len(body.ByAPIKey) != 1 || body.ByAPIKey[0].APIKeyID != 7 {
		t.Fatalf("response = %+v", body)
	}
}

func TestUsageHandlerGetDefaultsToLastThirtyDays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	reader := &fakeUsageSummaryReader{}
	handler := NewUsageHandler(reader)
	handler.now = func() time.Time { return now }
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.GET("/api/v1/me/usage", handler.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/usage", nil))

	if response.Code != http.StatusOK || !reader.from.Equal(now.Add(-defaultUsagePeriod)) || !reader.to.Equal(now) {
		t.Fatalf("status = %d, query = from %s, to %s", response.Code, reader.from, reader.to)
	}
}

func TestUsageHandlerGetRejectsInvalidInterval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &fakeUsageSummaryReader{}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.GET("/api/v1/me/usage", NewUsageHandler(reader).Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/usage?from=2026-07-02T00:00:00Z&to=2026-07-01T00:00:00Z", nil))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if reader.called {
		t.Fatal("repository called for invalid interval")
	}
}

type fakeUsageSummaryReader struct {
	summary store.UsageSummary
	err     error
	called  bool
	userID  int64
	from    time.Time
	to      time.Time
}

func (r *fakeUsageSummaryReader) GetSummaryByUser(_ context.Context, userID int64, from, to time.Time) (store.UsageSummary, error) {
	r.called = true
	r.userID = userID
	r.from = from
	r.to = to
	return r.summary, r.err
}
