package watcher

import (
	"context"
	"testing"
	"time"
)

func TestSessionCleanupRunsImmediately(t *testing.T) {
	cleaner := &fakeExpiredSessionCleaner{calls: make(chan struct{}, 1)}
	job := NewSessionCleanup(cleaner, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go job.Run(ctx)

	select {
	case <-cleaner.calls:
	case <-time.After(time.Second):
		t.Fatal("DeleteExpired() was not called")
	}
}

type fakeExpiredSessionCleaner struct{ calls chan struct{} }

func (c *fakeExpiredSessionCleaner) DeleteExpired(context.Context) (int64, error) {
	c.calls <- struct{}{}
	return 0, nil
}
