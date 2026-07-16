package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger пишет безопасную структурированную запись о завершённом HTTP-запросе.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		}
		if requestID := RequestIDFromContext(c.Request.Context()); requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
		if userID, ok := c.Get(ContextUserID); ok {
			switch id := userID.(type) {
			case int64:
				attrs = append(attrs, slog.Int64("user_id", id))
			case int:
				attrs = append(attrs, slog.Int("user_id", id))
			}
		}
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "http request completed", attrs...)
	}
}
