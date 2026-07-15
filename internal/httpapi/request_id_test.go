package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewarePropagatesValidClientID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/", func(c *gin.Context) {
		if id := RequestIDFromContext(c.Request.Context()); id != "trace-123" {
			t.Fatalf("context request ID=%q", id)
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "trace-123")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || response.Header().Get(RequestIDHeader) != "trace-123" {
		t.Fatalf("status=%d request_id=%q", response.Code, response.Header().Get(RequestIDHeader))
	}
}

func TestRequestIDMiddlewareReplacesInvalidClientID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "invalid value")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	id := response.Header().Get(RequestIDHeader)
	if response.Code != http.StatusNoContent || id == "" || id == "invalid value" || !validRequestID(id) {
		t.Fatalf("status=%d request_id=%q", response.Code, id)
	}
}
