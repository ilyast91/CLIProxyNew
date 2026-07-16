package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrationIdentitySourceMigrationDownRejectsStaticUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()
	staticUser, err := NewUserRepository(pool).UpsertStatic(ctx, UpsertUserParams{
		Username: "static:migration-down",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("UpsertStatic() error = %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", staticUser.ID); err != nil {
			t.Errorf("delete static migration user: %v", err)
		}
	})

	path := filepath.Join("..", "..", "db", "migrations", "20260714000100_users_identity_source.down.sql")
	downSQL, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downSQL)); err == nil {
		t.Fatal("identity_source down migration succeeded while a static user exists")
	}
}
