package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	authtesting "github.com/ilyast91/CLIProxyNew/internal/auth/testing"
)

func TestAdminAccountTestHandlerReturnsCheckerResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakeAccountChecker{result: authtesting.Result{Valid: false, Method: authtesting.MethodHTTPProbe, StatusCode: http.StatusUnauthorized, Error: "upstream returned HTTP 401"}}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/accounts/:accountID/test", NewAdminAccountTestHandler(checker).Test)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/account-1/test", nil))

	if response.Code != http.StatusOK || checker.accountID != "account-1" {
		t.Fatalf("status=%d account=%q body=%s", response.Code, checker.accountID, response.Body.String())
	}
}

func TestAdminAccountTestHandlerMapsMissingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakeAccountChecker{err: authtesting.ErrAccountNotFound}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/accounts/:accountID/test", NewAdminAccountTestHandler(checker).Test)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/missing/test", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminAccountTestHandlerMapsCheckerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakeAccountChecker{err: errors.New("runtime unavailable")}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(ContextUserID, int64(42)) })
	router.POST("/api/v1/admin/accounts/:accountID/test", NewAdminAccountTestHandler(checker).Test)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/account-1/test", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeAccountChecker struct {
	result    authtesting.Result
	err       error
	accountID string
}

func (c *fakeAccountChecker) Test(_ context.Context, accountID string) (authtesting.Result, error) {
	c.accountID = accountID
	return c.result, c.err
}
