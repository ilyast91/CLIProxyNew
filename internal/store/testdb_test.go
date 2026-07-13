package store

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresTestImage = "postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	container, err := postgrescontainer.Run(ctx, postgresTestImage,
		postgrescontainer.WithDatabase("cliproxy_test"),
		postgrescontainer.WithUsername("cliproxy"),
		postgrescontainer.WithPassword("cliproxy"),
		postgrescontainer.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("запустить PostgreSQL testcontainer: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("получить PostgreSQL DSN: %v", err)
	}

	migrations := newTestMigrator(t, dsn)
	t.Cleanup(func() {
		downMigrations(t, migrations)
		sourceErr, databaseErr := migrations.Close()
		if sourceErr != nil {
			t.Errorf("закрыть source миграций: %v", sourceErr)
		}
		if databaseErr != nil {
			t.Errorf("закрыть database миграций: %v", databaseErr)
		}
	})

	upMigrations(t, migrations)
	downMigrations(t, migrations)
	upMigrations(t, migrations)

	pool, err := OpenPool(ctx, dsn)
	if err != nil {
		t.Fatalf("OpenPool() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func upMigrations(t *testing.T, migrations *migrate.Migrate) {
	t.Helper()
	if err := migrations.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("применить миграции: %v", err)
	}
}

func downMigrations(t *testing.T, migrations *migrate.Migrate) {
	t.Helper()
	if err := migrations.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Errorf("откатить миграции: %v", err)
	}
}

func newTestMigrator(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("разобрать test DSN: %v", err)
	}
	database := stdlib.OpenDB(*config)

	driver, err := migratepgx.WithInstance(database, &migratepgx.Config{DatabaseName: config.Database})
	if err != nil {
		database.Close()
		t.Fatalf("создать migrate pgx driver: %v", err)
	}

	migrationsPath, err := filepath.Abs(filepath.Join("..", "..", "db", "migrations"))
	if err != nil {
		database.Close()
		t.Fatalf("получить путь миграций: %v", err)
	}
	sourceURL := (&url.URL{Scheme: "file", Path: migrationsPath}).String()

	migrator, err := migrate.NewWithDatabaseInstance(sourceURL, "pgx5", driver)
	if err != nil {
		database.Close()
		t.Fatalf("создать migrator для %q: %v", migrationsPath, err)
	}
	return migrator
}
