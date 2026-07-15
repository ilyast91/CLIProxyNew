package usage

import (
	"context"
	"testing"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestBufferedPluginFlushesPendingEventsOnClose(t *testing.T) {
	writer := &batchWriter{batches: make(chan []store.UsageEvent, 1), touched: make(chan []int64, 1)}
	plugin := NewBufferedPlugin(writer)
	plugin.HandleUsage(context.Background(), sdkusage.Record{
		APIKey: access.EncodePrincipal(42, 17), Provider: "openai", Model: "gpt-5", AuthID: "auth-1",
		Detail: sdkusage.Detail{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}, Latency: 50, TTFT: 10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := plugin.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case events := <-writer.batches:
		if len(events) != 1 || events[0].UserID == nil || *events[0].UserID != 42 || events[0].StatusCode != 200 {
			t.Fatalf("flushed events = %+v", events)
		}
	case <-ctx.Done():
		t.Fatal("pending event was not flushed")
	}
	select {
	case ids := <-writer.touched:
		if len(ids) != 1 || ids[0] != 17 {
			t.Fatalf("touched API key IDs = %v, want [17]", ids)
		}
	case <-ctx.Done():
		t.Fatal("API key last_used_at was not touched")
	}
}

type batchWriter struct {
	batches chan []store.UsageEvent
	touched chan []int64
}

func (w *batchWriter) InsertBatch(_ context.Context, events []store.UsageEvent) error {
	copyOfEvents := append([]store.UsageEvent(nil), events...)
	w.batches <- copyOfEvents
	return nil
}

func (w *batchWriter) TouchAPIKeysLastUsed(_ context.Context, ids []int64) error {
	w.touched <- append([]int64(nil), ids...)
	return nil
}
