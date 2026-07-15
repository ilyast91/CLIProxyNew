package observability

import (
	"context"
	"log/slog"
	"strings"
)

// RedactedValue заменяет значение sensitive log attribute.
const RedactedValue = "[REDACTED]"

// NewRedactingHandler оборачивает slog handler и исключает credentials из attrs.
func NewRedactingHandler(next slog.Handler) slog.Handler {
	if next == nil {
		next = slog.DiscardHandler
	}
	return &redactingHandler{next: next}
}

type redactingHandler struct{ next slog.Handler }

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		redacted[i] = redactAttr(attr)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, RedactedValue)
	}
	value := attr.Value.Resolve()
	if value.Kind() != slog.KindGroup {
		return slog.Attr{Key: attr.Key, Value: value}
	}
	group := value.Group()
	redacted := make([]slog.Attr, len(group))
	for i, nested := range group {
		redacted[i] = redactAttr(nested)
	}
	return slog.Attr{Key: attr.Key, Value: slog.GroupValue(redacted...)}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range []string{"password", "secret", "token", "credential", "authorization", "api_key", "apikey"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
