package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestIntegrationUsagePartitionForCurrentDay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test требует Docker")
	}

	pool := newTestPool(t)
	partition := "usage_events_" + time.Now().Format("20060102")
	var exists bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass($1) IS NOT NULL", partition).Scan(&exists); err != nil {
		t.Fatalf("проверить partition %s: %v", partition, err)
	}
	if !exists {
		t.Fatalf("partition %s не создана", partition)
	}

	if _, err := pool.Exec(context.Background(), "SELECT manage_usage_event_partitions(2, 90)"); err != nil {
		t.Fatalf("обновить usage partitions: %v", err)
	}
	if _, err := pool.Exec(context.Background(), fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", partition)); err != nil {
		t.Fatalf("прочитать partition %s: %v", partition, err)
	}
}
