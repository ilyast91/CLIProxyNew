package usage

import (
	"context"
	"log/slog"

	"github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

// EventWriter сохраняет usage event.
type EventWriter interface {
	Insert(ctx context.Context, event store.UsageEvent) error
}

// Plugin реализует публичный контракт SDK usage.Plugin.
type Plugin struct {
	writer EventWriter
}

// NewPlugin создаёт plugin аналитики.
func NewPlugin(writer EventWriter) *Plugin {
	return &Plugin{writer: writer}
}

// HandleUsage сохраняет завершённый upstream-вызов без зависимости от request context.
func (p *Plugin) HandleUsage(_ context.Context, record sdkusage.Record) {
	if p == nil || p.writer == nil {
		return
	}
	principal, err := access.DecodePrincipal(record.APIKey)
	if err != nil {
		slog.Warn("skip usage event with invalid principal", "error", err)
		return
	}
	statusCode := record.Fail.StatusCode
	if statusCode == 0 && !record.Failed {
		statusCode = 200
	}
	model := record.Model
	if record.Alias != "" {
		model = record.Alias
	}
	if err := p.writer.Insert(context.Background(), store.UsageEvent{
		UserID: principalPointer(principal.UserID), APIKeyID: principal.APIKeyID,
		UpstreamAccountID: record.AuthID, Provider: record.Provider, Model: model,
		InputTokens: record.Detail.InputTokens, OutputTokens: record.Detail.OutputTokens,
		ReasoningTokens: record.Detail.ReasoningTokens, CachedTokens: record.Detail.CachedTokens,
		TotalTokens: record.Detail.TotalTokens, StatusCode: statusCode, Error: record.Fail.Body,
		LatencyMS: record.Latency.Milliseconds(), TTFTMS: record.TTFT.Milliseconds(), Failed: record.Failed,
	}); err != nil {
		slog.Error("insert usage event", "error", err)
	}
}

func principalPointer(value int64) *int64 { return &value }
