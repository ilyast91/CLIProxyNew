package store

import (
	"context"
	"fmt"
	"math"
	"time"

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

// InsertBatch сохраняет группу usage events через один pgx batch.
func (r *UsageEventRepository) InsertBatch(ctx context.Context, events []UsageEvent) error {
	if r == nil || r.queries == nil {
		return ErrInvalidInput
	}
	if len(events) == 0 {
		return nil
	}

	params := make([]dbgen.InsertUsageEventsParams, 0, len(events))
	for _, event := range events {
		param, err := usageEventParams(event)
		if err != nil {
			return err
		}
		params = append(params, dbgen.InsertUsageEventsParams(param))
	}

	results := r.queries.InsertUsageEvents(ctx, params)
	var firstErr error
	results.Exec(func(_ int, err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	})
	if firstErr != nil {
		return fmt.Errorf("insert usage event batch: %w", firstErr)
	}
	return nil
}

// GetSummaryByUser возвращает личную статистику за полуоткрытый интервал [from, to).
func (r *UsageEventRepository) GetSummaryByUser(ctx context.Context, userID int64, from, to time.Time) (UsageSummary, error) {
	if r == nil || r.queries == nil || userID <= 0 || !from.Before(to) {
		return UsageSummary{}, ErrInvalidInput
	}
	params := usageSummaryParams(userID, from, to)
	total, err := r.queries.GetUsageSummaryByUser(ctx, params)
	if err != nil {
		return UsageSummary{}, fmt.Errorf("get usage summary: %w", err)
	}
	models, err := r.queries.ListUsageByModelForUser(ctx, dbgen.ListUsageByModelForUserParams(params))
	if err != nil {
		return UsageSummary{}, fmt.Errorf("list usage by model: %w", err)
	}
	keys, err := r.queries.ListUsageByAPIKeyForUser(ctx, dbgen.ListUsageByAPIKeyForUserParams(params))
	if err != nil {
		return UsageSummary{}, fmt.Errorf("list usage by API-key: %w", err)
	}

	result := UsageSummary{
		RequestCount: total.RequestCount, FailedRequestCount: total.FailedRequestCount,
		InputTokens: total.InputTokens, OutputTokens: total.OutputTokens,
		ReasoningTokens: total.ReasoningTokens, CachedTokens: total.CachedTokens,
		TotalTokens: total.TotalTokens,
		ByModel:     make([]UsageModelSummary, 0, len(models)), ByAPIKey: make([]UsageAPIKeySummary, 0, len(keys)),
	}
	for _, model := range models {
		result.ByModel = append(result.ByModel, UsageModelSummary{
			Model: model.Model.String, RequestCount: model.RequestCount,
			FailedRequestCount: model.FailedRequestCount, TotalTokens: model.TotalTokens,
		})
	}
	for _, key := range keys {
		result.ByAPIKey = append(result.ByAPIKey, UsageAPIKeySummary{
			APIKeyID: key.ApiKeyID.Int64, RequestCount: key.RequestCount,
			FailedRequestCount: key.FailedRequestCount, TotalTokens: key.TotalTokens,
		})
	}
	return result, nil
}

func usageSummaryParams(userID int64, from, to time.Time) dbgen.GetUsageSummaryByUserParams {
	return dbgen.GetUsageSummaryByUserParams{
		UserID:      pgtype.Int8{Int64: userID, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: from, Valid: true},
		CreatedAt_2: pgtype.Timestamptz{Time: to, Valid: true},
	}
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
