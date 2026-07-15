package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader — HTTP-заголовок correlation ID.
	RequestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128
)

type requestIDContextKey struct{}

// RequestIDFromContext возвращает request ID, установленный middleware.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// RequestIDMiddleware добавляет correlation ID в response и request context.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		if !validRequestID(id) {
			var err error
			id, err = newRequestID()
			if err != nil {
				writeError(c, http.StatusInternalServerError, "generate request ID failed")
				return
			}
		}
		c.Header(RequestIDHeader, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDContextKey{}, id))
		c.Next()
	}
}

func newRequestID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}
