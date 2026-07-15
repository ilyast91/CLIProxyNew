package watcher

import (
	"context"
	"log/slog"
	"time"
)

// ExpiredSessionCleaner удаляет истёкшие session records.
type ExpiredSessionCleaner interface {
	DeleteExpired(context.Context) (int64, error)
}

// SessionCleanup запускает периодическую очистку сессий на leader-реплике.
type SessionCleanup struct {
	cleaner  ExpiredSessionCleaner
	interval time.Duration
}

// NewSessionCleanup создаёт периодическую очистку истёкших сессий.
func NewSessionCleanup(cleaner ExpiredSessionCleaner, interval time.Duration) *SessionCleanup {
	return &SessionCleanup{cleaner: cleaner, interval: interval}
}

// Run очищает сессии сразу и затем с заданным interval до отмены ctx.
func (c *SessionCleanup) Run(ctx context.Context) {
	if c == nil || c.cleaner == nil || c.interval <= 0 {
		return
	}
	c.cleanup(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *SessionCleanup) cleanup(ctx context.Context) {
	if _, err := c.cleaner.DeleteExpired(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("delete expired sessions", "error", err)
	}
}
