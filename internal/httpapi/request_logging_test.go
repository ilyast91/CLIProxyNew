package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLoggerWritesSafeStructuredAccessLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(RequestIDMiddleware(), RequestLogger(slog.New(slog.NewJSONHandler(&output, nil))))
	router.GET("/v1/models/:model", func(c *gin.Context) {
		c.Set(ContextUserID, int64(42))
		c.Status(http.StatusCreated)
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-5?api_key=query-secret", nil)
	request.Header.Set(RequestIDHeader, "request-123")
	request.Header.Set("Authorization", "Bearer header-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d", response.Code)
	}
	line := output.String()
	for _, secret := range []string{"query-secret", "header-secret"} {
		if strings.Contains(line, secret) {
			t.Fatalf("access log contains secret %q: %s", secret, line)
		}
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode access log: %v", err)
	}
	if record["msg"] != "http request completed" || record["method"] != http.MethodGet || record["path"] != "/v1/models/:model" || record["status"] != float64(http.StatusCreated) || record["request_id"] != "request-123" || record["user_id"] != float64(42) {
		t.Fatalf("access log = %#v", record)
	}
	if _, ok := record["duration_ms"]; !ok {
		t.Fatalf("access log has no duration: %#v", record)
	}
}
