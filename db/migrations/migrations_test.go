package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestInitialMigrationCoversDesignedSchema(t *testing.T) {
	t.Parallel()

	up := readMigrations(t, ".up.sql")
	down := readMigrations(t, ".down.sql")

	for _, table := range []string{
		"users", "api_keys", "sessions", "upstream_accounts", "model_overrides",
		"usage_events", "admin_audit_log", "oauth_sessions",
	} {
		assertContains(t, up, "create table "+table)
		assertContains(t, down, "drop table "+table)
	}

	assertContains(t, up, "partition by range (created_at)")
	assertContains(t, up, "create table usage_events_20260713")
	assertContains(t, up, "partition of usage_events")
	assertContains(t, up, "create materialized view usage_aggregates")
	assertContains(t, down, "drop materialized view usage_aggregates")
	assertContains(t, up, "credentials_enc bytea not null")
	assertContains(t, up, "enc_key_version integer not null")
	assertContains(t, up, "on delete set null")
}

func readMigrations(t *testing.T, suffix string) string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("прочитать каталог миграций: %v", err)
	}

	var migrations strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("прочитать миграцию %s: %v", entry.Name(), err)
		}
		migrations.Write(data)
		migrations.WriteByte('\n')
	}
	if migrations.Len() == 0 {
		t.Fatalf("миграции %s не найдены", suffix)
	}
	return strings.ToLower(migrations.String())
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Errorf("миграция не содержит %q", want)
	}
}
