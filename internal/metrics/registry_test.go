package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestRegistryExportsHTTPAndUpstreamMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hook := usage.NewHook()
	hook.OnResult(context.Background(), coreauth.Result{Success: true})
	hook.OnResult(context.Background(), coreauth.Result{Success: false})
	registry := NewRegistry(nil, hook, nil)

	router := gin.New()
	router.Use(registry.Middleware())
	router.GET("/v1/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/test", nil))

	response := httptest.NewRecorder()
	registry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%q", response.Code, response.Body.String())
	}
	for _, metric := range []string{
		`cliproxy_http_requests_total{method="GET",path="/v1/test",status="204"} 1`,
		`cliproxy_upstream_results_total{outcome="success"} 1`,
		`cliproxy_upstream_results_total{outcome="failure"} 1`,
		"cliproxy_usage_queue_depth 0",
	} {
		if !strings.Contains(response.Body.String(), metric) {
			t.Fatalf("metrics body does not contain %q:\n%s", metric, response.Body.String())
		}
	}
}
