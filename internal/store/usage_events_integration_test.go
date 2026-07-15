package store

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationUsageEventRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	repository := NewUsageEventRepository(pool)
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

func TestIntegrationUsageEventRepositoryInsertBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	repository := NewUsageEventRepository(newTestPool(t))
	if err := repository.InsertBatch(context.Background(), []UsageEvent{
		{Provider: "openai", Model: "gpt-5", TotalTokens: 19, StatusCode: 200, LatencyMS: 50},
		{Provider: "anthropic", Model: "claude", TotalTokens: 9, StatusCode: 502, LatencyMS: 40, Failed: true},
	}); err != nil {
		t.Fatalf("InsertBatch() error = %v", err)
	}
	if err := repository.InsertBatch(context.Background(), []UsageEvent{{StatusCode: -1}}); err != ErrInvalidInput {
		t.Fatalf("InsertBatch(invalid) error = %v, want ErrInvalidInput", err)
	}
}

func TestIntegrationUsageEventRepositoryGetSummaryByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx := context.Background()
	pool := newTestPool(t)
	users := NewUserRepository(pool)
	keys := NewAPIKeyRepository(pool)
	repository := NewUsageEventRepository(pool)
	user, err := users.UpsertFromLDAP(ctx, UpsertUserParams{Username: "usage-user", Role: "user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := keys.Create(ctx, CreateAPIKeyParams{UserID: user.ID, Plaintext: "cpn_usage_test_key"})
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	otherUser, err := users.UpsertFromLDAP(ctx, UpsertUserParams{Username: "other-usage-user", Role: "user"})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	userID := user.ID
	keyID := key.ID
	for _, event := range []UsageEvent{
		{UserID: &userID, APIKeyID: &keyID, Model: "gpt-5", InputTokens: 8, OutputTokens: 4, TotalTokens: 12, StatusCode: 200, LatencyMS: 10},
		{UserID: &userID, APIKeyID: &keyID, Model: "gpt-5-mini", InputTokens: 3, OutputTokens: 2, TotalTokens: 5, StatusCode: 500, Failed: true, LatencyMS: 10},
		{UserID: &otherUser.ID, Model: "private-model", TotalTokens: 99, StatusCode: 200, LatencyMS: 10},
	} {
		if err := repository.Insert(ctx, event); err != nil {
			t.Fatalf("insert usage event: %v", err)
		}
	}

	summary, err := repository.GetSummaryByUser(ctx, user.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetSummaryByUser() error = %v", err)
	}
	if summary.RequestCount != 2 || summary.FailedRequestCount != 1 || summary.TotalTokens != 17 || summary.InputTokens != 11 || summary.OutputTokens != 6 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.ByModel) != 2 || len(summary.ByAPIKey) != 1 || summary.ByAPIKey[0].APIKeyID != key.ID || summary.ByAPIKey[0].TotalTokens != 17 {
		t.Fatalf("breakdown = %+v", summary)
	}
	now := time.Now()
	if _, err := repository.GetSummaryByUser(ctx, user.ID, now, now); err != ErrInvalidInput {
		t.Fatalf("GetSummaryByUser(invalid interval) error = %v, want ErrInvalidInput", err)
	}
}
