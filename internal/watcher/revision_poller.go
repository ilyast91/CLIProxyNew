package watcher

import (
	"context"
	"log/slog"
	"time"
)

// RevisionReader читает named runtime revision из persistence.
type RevisionReader interface {
	Get(context.Context, string) (int64, error)
}

// RevisionPoller инициирует controlled shutdown после изменения revision.
type RevisionPoller struct {
	reader   RevisionReader
	name     string
	interval time.Duration
	jitter   time.Duration
	shutdown func()
}

// NewRevisionPoller создаёт poller runtime revision.
func NewRevisionPoller(reader RevisionReader, name string, interval, jitter time.Duration, shutdown func()) *RevisionPoller {
	return &RevisionPoller{reader: reader, name: name, interval: interval, jitter: jitter, shutdown: shutdown}
}

// Run блокируется до отмены context или обнаружения новой revision.
func (p *RevisionPoller) Run(ctx context.Context) {
	if p == nil || p.reader == nil || p.shutdown == nil || p.interval <= 0 {
		return
	}
	baseline, err := p.reader.Get(ctx, p.name)
	if err != nil {
		slog.Error("read runtime revision", "error", err)
		return
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			value, err := p.reader.Get(ctx, p.name)
			if err != nil {
				slog.Warn("read runtime revision", "error", err)
				continue
			}
			if value == baseline {
				continue
			}
			if p.jitter > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(p.jitter):
				}
			}
			p.shutdown()
			return
		}
	}
}
