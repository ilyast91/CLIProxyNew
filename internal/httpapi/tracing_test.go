package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestTracingMiddlewareCreatesServerSpanAndPropagatesParent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	router := gin.New()
	router.Use(TracingMiddleware())
	router.GET("/v1/models/:model", func(c *gin.Context) {
		c.Set(ContextUserID, int64(42))
		c.Status(http.StatusCreated)
	})
	parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{1},
		SpanID:     oteltrace.SpanID{2},
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-5?api_key=query-secret", nil)
	request = request.WithContext(oteltrace.ContextWithRemoteSpanContext(request.Context(), parent))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d", response.Code)
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans=%d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "HTTP GET /v1/models/:model" || span.SpanKind() != oteltrace.SpanKindServer || span.Parent().TraceID() != parent.TraceID() || span.Parent().SpanID() != parent.SpanID() {
		t.Fatalf("span name=%q kind=%v parent=%v", span.Name(), span.SpanKind(), span.Parent())
	}
	attributes := attributeMap(span.Attributes())
	if attributes["http.request.method"].AsString() != http.MethodGet || attributes["http.route"].AsString() != "/v1/models/:model" || attributes["http.response.status_code"].AsInt64() != http.StatusCreated || attributes["user.id"].AsInt64() != 42 {
		t.Fatalf("span attributes=%v", attributes)
	}
	if _, exists := attributes["url.query"]; exists {
		t.Fatalf("span must not contain query: %v", attributes)
	}
}

func attributeMap(attributes []attribute.KeyValue) map[string]attribute.Value {
	result := make(map[string]attribute.Value, len(attributes))
	for _, attr := range attributes {
		result[string(attr.Key)] = attr.Value
	}
	return result
}
