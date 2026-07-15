package usage

import (
	"context"
	"log/slog"

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
	event, err := usageEventFromRecord(record)
	if err != nil {
		slog.Warn("skip usage event with invalid principal", "error", err)
		return
	}
	if err := p.writer.Insert(context.Background(), event); err != nil {
		slog.Error("insert usage event", "error", err)
	}
}

func principalPointer(value int64) *int64 { return &value }
