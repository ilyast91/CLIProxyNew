package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
)

func TestIntegrationModelOverrideRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	ctx := context.Background()
	repository := NewModelOverrideRepository(pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM model_overrides WHERE model_alias = $1", "business-gpt"); err != nil {
			t.Errorf("удалить test model override: %v", err)
		}
	})
	if _, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider: "openai", ModelAlias: "invalid-config", UpstreamModel: "gpt-5", Config: []byte("not-json"),
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Upsert(invalid config) error = %v, want ErrInvalidInput", err)
	}

	override, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider:      "openai",
		ModelAlias:    "business-gpt",
		UpstreamModel: "gpt-5",
		Enabled:       true,
		Config:        []byte(`{"reasoning_effort":"high"}`),
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if override.Provider != "openai" || override.ModelAlias != "business-gpt" || override.UpstreamModel != "gpt-5" || !override.Enabled {
		t.Fatalf("Upsert() = %+v", override)
	}

	updated, err := repository.Upsert(ctx, UpsertModelOverrideParams{
		Provider:      "openai",
		ModelAlias:    "business-gpt",
		UpstreamModel: "gpt-5-mini",
		Enabled:       false,
	})
	if err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}
	if updated.ID != override.ID || updated.UpstreamModel != "gpt-5-mini" || updated.Enabled {
		t.Fatalf("Upsert(update) = %+v", updated)
	}

	overrides, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(overrides) != 1 || overrides[0].ID != override.ID {
		t.Fatalf("List() = %+v", overrides)
	}

	if err := repository.Delete(ctx, override.ModelAlias); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := repository.Delete(ctx, override.ModelAlias); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(second) error = %v, want ErrNotFound", err)
	}
}

func TestIntegrationAdminModelRepositoryWritesAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	ctx := context.Background()
	pool := newTestPool(t)
	admin, err := NewUserRepository(pool).UpsertFromLDAP(ctx, UpsertUserParams{Username: "model-audit-admin", Role: "admin"})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	repository := NewAdminModelRepository(pool)
	address := netip.MustParseAddr("192.0.2.30")
	params := UpsertModelOverrideParams{Provider: "openai", ModelAlias: "audit-model", UpstreamModel: "gpt-5", Enabled: true}

	if _, err := repository.UpsertWithAudit(ctx, admin.ID, params, &address); err != nil {
		t.Fatalf("UpsertWithAudit: %v", err)
	}
	if err := repository.DeleteWithAudit(ctx, admin.ID, params.ModelAlias, &address); err != nil {
		t.Fatalf("DeleteWithAudit: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT action, target_type, target_id, details
		FROM admin_audit_log
		ORDER BY id`)
	if err != nil {
		t.Fatalf("read audit rows: %v", err)
	}
	defer rows.Close()

	type auditRow struct {
		action, targetType, targetID string
		details                      []byte
	}
	var auditRows []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.action, &row.targetType, &row.targetID, &row.details); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		auditRows = append(auditRows, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit rows: %v", err)
	}
	if len(auditRows) != 2 {
		t.Fatalf("audit rows = %+v, want two rows", auditRows)
	}
	if auditRows[0].action != "model_override.upserted" || auditRows[0].targetType != "model_override" || auditRows[0].targetID != params.ModelAlias {
		t.Fatalf("upsert audit = %+v", auditRows[0])
	}
	var details map[string]string
	if err := json.Unmarshal(auditRows[0].details, &details); err != nil {
		t.Fatalf("decode upsert audit details: %v", err)
	}
	if details["provider"] != params.Provider || details["upstream_model"] != params.UpstreamModel {
		t.Fatalf("upsert audit details = %v", details)
	}
	if auditRows[1].action != "model_override.deleted" || auditRows[1].targetType != "model_override" || auditRows[1].targetID != params.ModelAlias {
		t.Fatalf("delete audit = %+v", auditRows[1])
	}
}
