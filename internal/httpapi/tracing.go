package httpapi

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracingInstrumentationName = "github.com/ilyast91/CLIProxyNew/internal/httpapi"

// TracingMiddleware создаёт server span и передаёт trace context обработчикам и SDK.
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, span := otel.Tracer(tracingInstrumentationName).Start(
			c.Request.Context(),
			fmt.Sprintf("HTTP %s", c.Request.Method),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		status := c.Writer.Status()
		span.SetName(fmt.Sprintf("HTTP %s %s", c.Request.Method, path))
		span.SetAttributes(
			attribute.String("http.request.method", c.Request.Method),
			attribute.String("http.route", path),
			attribute.Int("http.response.status_code", status),
		)
		if userID, ok := c.Get(ContextUserID); ok {
			switch id := userID.(type) {
			case int64:
				span.SetAttributes(attribute.Int64("user.id", id))
			case int:
				span.SetAttributes(attribute.Int("user.id", id))
			}
		}
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}
