package store

import (
	"context"
	"testing"
)

func TestIntegrationUsageEventRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	repository := NewUsageEventRepository(newTestPool(t))
	if err := repository.Insert(context.Background(), UsageEvent{
		Provider: "openai", Model: "gpt-5", InputTokens: 12, OutputTokens: 7, TotalTokens: 19,
		StatusCode: 200, LatencyMS: 50, TTFTMS: 10,
	}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := repository.Insert(context.Background(), UsageEvent{StatusCode: -1}); err != ErrInvalidInput {
		t.Fatalf("Insert(invalid) error = %v, want ErrInvalidInput", err)
	}
}
