package watcher

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLeaderRunnerRunsJobWhileLeaseIsHeld(t *testing.T) {
	lease := &fakeLease{released: make(chan struct{})}
	locker := &fakeLocker{lease: lease}
	runner := NewLeaderRunner(locker, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go runner.Run(ctx, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leader job was not started")
	}
	cancel()
	select {
	case <-lease.released:
	case <-time.After(time.Second):
		t.Fatal("leader lease was not released")
	}
	if locker.calls != 1 {
		t.Fatalf("TryLock() calls = %d, want 1", locker.calls)
	}
}

type fakeLocker struct {
	mu    sync.Mutex
	lease AdvisoryLease
	calls int
}

func (l *fakeLocker) TryLock(context.Context) (AdvisoryLease, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	return l.lease, true, nil
}

type fakeLease struct {
	once     sync.Once
	released chan struct{}
}

func (l *fakeLease) Release(context.Context) error {
	l.once.Do(func() { close(l.released) })
	return nil
}
