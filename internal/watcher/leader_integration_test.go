package watcher

import (
	"context"
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

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
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
