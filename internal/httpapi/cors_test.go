package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddlewareAllowsConfiguredManagementOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCORSMiddleware([]string{"https://console.example.test"}))
	router.GET("/api/v1/me", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Origin", "https://console.example.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.test" {
		t.Fatalf("allow origin=%q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials=%q", got)
	}
}

func TestCORSMiddlewareHandlesConfiguredManagementPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCORSMiddleware([]string{"https://console.example.test"}))
	router.POST("/api/v1/login", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/login", nil)
	request.Header.Set("Origin", "https://console.example.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Request-ID")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("preflight did not return allowed methods")
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("preflight did not return allowed headers")
	}
}

func TestCORSMiddlewareDoesNotAllowUnknownOrProxyOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCORSMiddleware([]string{"https://console.example.test"}))
	router.GET("/api/v1/me", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, path := range []string{"/api/v1/me", "/v1/models"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Origin", "https://untrusted.example.test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("path=%s allow origin=%q", path, got)
		}
	}
}
