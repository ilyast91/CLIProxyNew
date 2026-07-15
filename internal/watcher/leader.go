package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// SessionCleanupLock идентифицирует cluster-wide leader для очистки сессий.
	SessionCleanupLock int64 = 741918273
	lockReleaseTimeout       = 5 * time.Second
)

// AdvisoryLease удерживает session-level Postgres advisory lock.
type AdvisoryLease interface {
	Release(context.Context) error
}

// AdvisoryLocker пытается получить лидерство для одной реплики.
type AdvisoryLocker interface {
	TryLock(context.Context) (AdvisoryLease, bool, error)
}

// LeaderRunner удерживает лидерство и запускает job только на выбранной реплике.
type LeaderRunner struct {
	locker        AdvisoryLocker
	retryInterval time.Duration
}

// NewLeaderRunner создаёт runner с повторной попыткой занять лидерство.
func NewLeaderRunner(locker AdvisoryLocker, retryInterval time.Duration) *LeaderRunner {
	return &LeaderRunner{locker: locker, retryInterval: retryInterval}
}

// Run пытается занять advisory lock и вызывает job, пока удерживает лидерство.
func (r *LeaderRunner) Run(ctx context.Context, job func(context.Context)) {
	if r == nil || r.locker == nil || job == nil || r.retryInterval <= 0 {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		lease, acquired, err := r.locker.TryLock(ctx)
		if err != nil {
			slog.Warn("acquire leader lock", "error", err)
			if !waitForRetry(ctx, r.retryInterval) {
				return
			}
			continue
		}
		if !acquired {
			if !waitForRetry(ctx, r.retryInterval) {
				return
			}
			continue
		}

		job(ctx)
		releaseCtx, cancel := context.WithTimeout(context.Background(), lockReleaseTimeout)
		if err := lease.Release(releaseCtx); err != nil {
			slog.Warn("release leader lock", "error", err)
		}
		cancel()
		if !waitForRetry(ctx, r.retryInterval) {
			return
		}
	}
}

func waitForRetry(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// PostgresAdvisoryLocker получает session-level advisory lock на выделенном connection pool.
type PostgresAdvisoryLocker struct {
	pool   *pgxpool.Pool
	lockID int64
}

// NewPostgresAdvisoryLocker создаёт Postgres-реализацию advisory leader lock.
func NewPostgresAdvisoryLocker(pool *pgxpool.Pool, lockID int64) *PostgresAdvisoryLocker {
	return &PostgresAdvisoryLocker{pool: pool, lockID: lockID}
}

// TryLock пытается занять lock и возвращает lease, удерживающий выделенное соединение.
func (l *PostgresAdvisoryLocker) TryLock(ctx context.Context) (AdvisoryLease, bool, error) {
	if l == nil || l.pool == nil {
		return nil, false, fmt.Errorf("Postgres advisory locker is not configured")
	}
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire advisory lock connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", l.lockID).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("try advisory lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}
	return &postgresAdvisoryLease{conn: conn, lockID: l.lockID}, true, nil
}

type postgresAdvisoryLease struct {
	conn   *pgxpool.Conn
	lockID int64
}

func (l *postgresAdvisoryLease) Release(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}
	defer l.conn.Release()
	var released bool
	err := l.conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", l.lockID).Scan(&released)
	if err == nil && released {
		return nil
	}
	closeErr := l.conn.Conn().Close(context.Background())
	if err != nil {
		return fmt.Errorf("unlock advisory lock: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("advisory lock was not held and connection close failed: %w", closeErr)
	}
	return fmt.Errorf("advisory lock was not held")
}
