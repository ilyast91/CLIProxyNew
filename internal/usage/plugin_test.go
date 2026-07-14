package usage

import (
	"context"
	"testing"

	"github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestPluginWritesUsageEventFromVersionedPrincipal(t *testing.T) {
	writer := &fakeWriter{}
	NewPlugin(writer).HandleUsage(context.Background(), sdkusage.Record{
		APIKey: access.EncodePrincipal(42, 17), Provider: "openai", Model: "gpt-5", AuthID: "auth-1",
		Detail: sdkusage.Detail{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}, Latency: 50, TTFT: 10,
	})
	if writer.event == nil || *writer.event.UserID != 42 || *writer.event.APIKeyID != 17 || writer.event.StatusCode != 200 {
		t.Fatalf("event = %+v", writer.event)
	}
}

type fakeWriter struct{ event *store.UsageEvent }

func (w *fakeWriter) Insert(_ context.Context, event store.UsageEvent) error {
	w.event = &event
	return nil
}
