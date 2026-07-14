package store

import (
	"context"
	"fmt"
	"math"

	"github.com/ilyast91/CLIProxyNew/internal/store/dbgen"
	"github.com/jackc/pgx/v5/pgtype"
)

// UsageEventRepository сохраняет аналитику upstream-вызовов.
type UsageEventRepository struct {
	queries *dbgen.Queries
}

// NewUsageEventRepository создаёт репозиторий usage events.
func NewUsageEventRepository(db dbgen.DBTX) *UsageEventRepository {
	return &UsageEventRepository{queries: dbgen.New(db)}
}

// Insert сохраняет одно событие использования.
func (r *UsageEventRepository) Insert(ctx context.Context, event UsageEvent) error {
	params, err := usageEventParams(event)
	if err != nil {
		return err
	}
	if err := r.queries.InsertUsageEvent(ctx, params); err != nil {
		return fmt.Errorf("insert usage event: %w", err)
	}
	return nil
}

func usageEventParams(event UsageEvent) (dbgen.InsertUsageEventParams, error) {
	if event.StatusCode < 0 || event.LatencyMS < 0 || event.TTFTMS < 0 {
		return dbgen.InsertUsageEventParams{}, ErrInvalidInput
	}
	values := []int64{event.InputTokens, event.OutputTokens, event.ReasoningTokens, event.CachedTokens, event.TotalTokens, event.LatencyMS, event.TTFTMS}
	for _, value := range values {
		if value < 0 || value > math.MaxInt32 {
			return dbgen.InsertUsageEventParams{}, ErrInvalidInput
		}
	}
	if event.StatusCode > math.MaxInt32 {
		return dbgen.InsertUsageEventParams{}, ErrInvalidInput
	}

	return dbgen.InsertUsageEventParams{
		UserID:            nullableInt8(event.UserID),
		ApiKeyID:          nullableInt8(event.APIKeyID),
		UpstreamAccountID: nullableText(event.UpstreamAccountID),
		Provider:          nullableText(event.Provider),
		Model:             nullableText(event.Model),
		InputTokens:       int32(event.InputTokens),
		OutputTokens:      int32(event.OutputTokens),
		ReasoningTokens:   int32(event.ReasoningTokens),
		CachedTokens:      int32(event.CachedTokens),
		TotalTokens:       int32(event.TotalTokens),
		StatusCode:        int32(event.StatusCode),
		Error:             nullableText(event.Error),
		LatencyMs:         int32(event.LatencyMS),
		TtftMs:            int32(event.TTFTMS),
		Failed:            event.Failed,
	}, nil
}

func nullableInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
