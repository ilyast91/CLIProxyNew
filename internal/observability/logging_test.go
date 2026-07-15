package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandlerRedactsSensitiveAttrsAndGroups(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&output, nil))).With("api_key", "client-secret")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "upstream auth",
		slog.String("password", "db-password"),
		slog.Group("oauth", slog.String("refresh_token", "refresh-secret"), slog.String("email", "user@example.com")),
	)

	line := output.String()
	for _, secret := range []string{"client-secret", "db-password", "refresh-secret"} {
		if strings.Contains(line, secret) {
			t.Fatalf("log contains secret %q: %s", secret, line)
		}
	}
	if strings.Count(line, RedactedValue) != 3 {
		t.Fatalf("redacted values=%d, log=%s", strings.Count(line, RedactedValue), line)
	}
	if !strings.Contains(line, "user@example.com") {
		t.Fatalf("non-sensitive attr was removed: %s", line)
	}
}
