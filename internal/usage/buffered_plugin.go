package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

const (
	usageQueueCapacity = 1024
	usageBatchSize     = 100
	usageFlushInterval = 250 * time.Millisecond
	usageFlushTimeout  = 5 * time.Second
)

// BatchEventWriter сохраняет группу usage events одной операцией persistence-слоя.
type BatchEventWriter interface {
	InsertBatch(ctx context.Context, events []store.UsageEvent) error
}

// BufferedPlugin асинхронно буферизует usage events перед пакетной записью.
type BufferedPlugin struct {
	writer BatchEventWriter
	queue  chan store.UsageEvent
	done   chan struct{}

	mu     sync.RWMutex
	closed bool
	once   sync.Once
}

var _ sdkusage.Plugin = (*BufferedPlugin)(nil)

// NewBufferedPlugin создаёт bounded очередь аналитики и запускает writer.
func NewBufferedPlugin(writer BatchEventWriter) *BufferedPlugin {
	p := &BufferedPlugin{
		writer: writer,
		queue:  make(chan store.UsageEvent, usageQueueCapacity),
		done:   make(chan struct{}),
	}
	if writer == nil {
		close(p.done)
		return p
	}
	go p.run()
	return p
}

// HandleUsage добавляет завершённый upstream-вызов в очередь без ожидания БД.
func (p *BufferedPlugin) HandleUsage(_ context.Context, record sdkusage.Record) {
	if p == nil || p.writer == nil {
		return
	}
	event, err := usageEventFromRecord(record)
	if err != nil {
		slog.Warn("skip usage event with invalid principal", "error", err)
		return
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return
	}
	select {
	case p.queue <- event:
	default:
		slog.Warn("drop usage event: queue is full")
	}
}

// Close прекращает приём событий и сбрасывает накопленный batch до отмены ctx.
func (p *BufferedPlugin) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.queue)
		p.mu.Unlock()
	})
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *BufferedPlugin) run() {
	defer close(p.done)

	ticker := time.NewTicker(usageFlushInterval)
	defer ticker.Stop()
	batch := make([]store.UsageEvent, 0, usageBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), usageFlushTimeout)
		err := p.writer.InsertBatch(ctx, batch)
		cancel()
		if err != nil {
			slog.Error("insert usage event batch", "count", len(batch), "error", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-p.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) == usageBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func usageEventFromRecord(record sdkusage.Record) (store.UsageEvent, error) {
	principal, err := access.DecodePrincipal(record.APIKey)
	if err != nil {
		return store.UsageEvent{}, err
	}
	statusCode := record.Fail.StatusCode
	if statusCode == 0 && !record.Failed {
		statusCode = 200
	}
	model := record.Model
	if record.Alias != "" {
		model = record.Alias
	}
	return store.UsageEvent{
		UserID: principalPointer(principal.UserID), APIKeyID: principal.APIKeyID,
		UpstreamAccountID: record.AuthID, Provider: record.Provider, Model: model,
		InputTokens: record.Detail.InputTokens, OutputTokens: record.Detail.OutputTokens,
		ReasoningTokens: record.Detail.ReasoningTokens, CachedTokens: record.Detail.CachedTokens,
		TotalTokens: record.Detail.TotalTokens, StatusCode: statusCode, Error: record.Fail.Body,
		LatencyMS: record.Latency.Milliseconds(), TTFTMS: record.TTFT.Milliseconds(), Failed: record.Failed,
	}, nil
}
