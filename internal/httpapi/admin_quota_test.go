package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestAdminQuotaHandlerReturnsRuntimeQuotaAndCredits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nextRecoverAt := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	creditsUpdatedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	lookup := &fakeQuotaLookup{auth: &coreauth.Auth{ID: "account-1", Provider: "antigravity", Quota: coreauth.QuotaState{Exceeded: true, Reason: "rate limited", NextRecoverAt: nextRecoverAt, BackoffLevel: 2}}}
	handler := NewAdminQuotaHandler(lookup)
	handler.credits = func(string) (coreauth.AntigravityCreditsHint, bool) {
		return coreauth.AntigravityCreditsHint{Known: true, Available: true, CreditAmount: 12.5, MinCreditAmount: 1.5, PaidTierID: "pro", UpdatedAt: creditsUpdatedAt}, true
	}
	router := gin.New()
	router.GET("/api/v1/admin/accounts/:accountID/quota", handler.Get)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/account-1/quota", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !containsAll(response.Body.String(), `"exceeded":true`, `"unknown":false`, `"credit_amount":12.5`, `"paid_tier_id":"pro"`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestAdminQuotaHandlerMarksFreshAccountAsUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/:accountID/quota", NewAdminQuotaHandler(&fakeQuotaLookup{auth: &coreauth.Auth{ID: "fresh", Provider: "claude"}}).Get)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/fresh/quota", nil))

	if response.Code != http.StatusOK || !containsAll(response.Body.String(), `"unknown":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminQuotaHandlerMapsMissingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/admin/accounts/:accountID/quota", NewAdminQuotaHandler(&fakeQuotaLookup{}).Get)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/missing/quota", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

type fakeQuotaLookup struct{ auth *coreauth.Auth }

func (l *fakeQuotaLookup) GetByID(id string) (*coreauth.Auth, bool) {
	if l.auth == nil || l.auth.ID != id {
		return nil, false
	}
	return l.auth.Clone(), true
}
