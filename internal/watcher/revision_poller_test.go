package watcher

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRevisionPollerCallsShutdownAfterRevisionChange(t *testing.T) {
	revisions := &fakeRevisions{values: []int64{1, 1, 2}}
	called := make(chan struct{}, 1)
	poller := NewRevisionPoller(revisions, "upstream_accounts", time.Millisecond, 0, func() { called <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go poller.Run(ctx)

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
}

type fakeRevisions struct {
	mu     sync.Mutex
	values []int64
}

func (r *fakeRevisions) Get(context.Context, string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.values[0]
	if len(r.values) > 1 {
		r.values = r.values[1:]
	}
	return value, nil
}
