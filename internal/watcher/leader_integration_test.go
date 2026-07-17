package watcher

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresTestImage = "postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"

func TestIntegrationPostgresAdvisoryLocker(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx, firstPool, secondPool := newAdvisoryTestPools(t)

	first := NewPostgresAdvisoryLocker(firstPool, SessionCleanupLock)
	second := NewPostgresAdvisoryLocker(secondPool, SessionCleanupLock)
	lease, acquired, err := first.TryLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("first TryLock() = acquired %t, error %v", acquired, err)
	}
	if _, acquired, err := second.TryLock(ctx); err != nil || acquired {
		t.Fatalf("second TryLock() while held = acquired %t, error %v", acquired, err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	secondLease, acquired, err := second.TryLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("second TryLock() after release = acquired %t, error %v", acquired, err)
	}
	if err := secondLease.Release(ctx); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}
}

func TestIntegrationAdvisoryLeaderFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("integration failover test требует Docker")
	}

	ctx, firstPool, secondPool := newAdvisoryTestPools(t)
	recorder := &leaderCleanupRecorder{started: make(chan string, 2)}
	firstRunner := NewLeaderRunner(NewPostgresAdvisoryLocker(firstPool, SessionCleanupLock), 10*time.Millisecond)
	secondRunner := NewLeaderRunner(NewPostgresAdvisoryLocker(secondPool, SessionCleanupLock), 10*time.Millisecond)

	firstCtx, stopFirst := context.WithCancel(ctx)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		firstRunner.Run(firstCtx, recorder.job("first"))
	}()

	if got := waitForLeaderCleanup(t, recorder.started); got != "first" {
		t.Fatalf("initial leader = %q, want first", got)
	}

	secondCtx, stopSecond := context.WithCancel(ctx)
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		secondRunner.Run(secondCtx, recorder.job("second"))
	}()

	select {
	case replica := <-recorder.started:
		t.Fatalf("standby cleanup started while leader was active: %s", replica)
	case <-time.After(150 * time.Millisecond):
	}

	stopFirst()
	if got := waitForLeaderCleanup(t, recorder.started); got != "second" {
		t.Fatalf("failover leader = %q, want second", got)
	}
	if maxActive := recorder.maxActive.Load(); maxActive != 1 {
		t.Fatalf("simultaneous leader jobs = %d, want 1", maxActive)
	}

	stopSecond()
	waitForRunnerStop(t, firstDone, "first")
	waitForRunnerStop(t, secondDone, "second")
}

func newAdvisoryTestPools(t *testing.T) (context.Context, *pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	container, err := postgrescontainer.Run(ctx, postgresTestImage,
		postgrescontainer.WithDatabase("cliproxy_test"),
		postgrescontainer.WithUsername("cliproxy"),
		postgrescontainer.WithPassword("cliproxy"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("запустить PostgreSQL testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("получить PostgreSQL DSN: %v", err)
	}
	firstPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("создать первый pool: %v", err)
	}
	t.Cleanup(firstPool.Close)
	secondPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("создать второй pool: %v", err)
	}
	t.Cleanup(secondPool.Close)
	return ctx, firstPool, secondPool
}

type leaderCleanupRecorder struct {
	started   chan string
	active    atomic.Int32
	maxActive atomic.Int32
}

func (r *leaderCleanupRecorder) job(replica string) func(context.Context) {
	cleaner := &leaderCleanupProbe{replica: replica, started: r.started}
	cleanup := NewSessionCleanup(cleaner, time.Hour)
	return func(ctx context.Context) {
		active := r.active.Add(1)
		for {
			maximum := r.maxActive.Load()
			if active <= maximum || r.maxActive.CompareAndSwap(maximum, active) {
				break
			}
		}
		defer r.active.Add(-1)
		cleanup.Run(ctx)
	}
}

type leaderCleanupProbe struct {
	once    sync.Once
	replica string
	started chan<- string
}

func (p *leaderCleanupProbe) DeleteExpired(context.Context) (int64, error) {
	p.once.Do(func() { p.started <- p.replica })
	return 0, nil
}

func waitForLeaderCleanup(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case replica := <-started:
		return replica
	case <-time.After(5 * time.Second):
		t.Fatal("leader cleanup did not start before deadline")
		return ""
	}
}

func waitForRunnerStop(t *testing.T, done <-chan struct{}, replica string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("leader runner %s did not stop before deadline", replica)
	}
}
