package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectionPruneRestartRollback(t *testing.T) {
	if os.Getenv("VERMORY_W18_RESTART_TEST") != "1" {
		t.Skip("VERMORY_W18_RESTART_TEST=1 is required")
	}
	root := strings.TrimSpace(os.Getenv("VERMORY_W18_ROOT"))
	if root == "" {
		t.Fatal("VERMORY_W18_ROOT is required")
	}
	binDir := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES18_BIN"))
	if binDir == "" {
		binDir = "/opt/homebrew/opt/postgresql@18/bin"
	}
	cluster := startDimensionalPostgres18(t, filepath.Clean(root), filepath.Clean(binDir))
	ctx := context.Background()
	store, err := OpenStore(ctx, cluster.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := "w18-restart-rollback"
	events := seedProjectionPruneEvents(t, store, tenantID, 3)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_events SET created_at = $2 WHERE tenant_id = $1`, tenantID, old); err != nil {
		t.Fatal(err)
	}
	request := ProjectionPruneRequest{
		OperationID: "w18-restart-prune-operation",
		Cutoff:      old.Add(24 * time.Hour), RetainTailEvents: 0,
	}
	_, normalized, fingerprint, err := normalizeProjectionPruneRequest(tenantID, request)
	if err != nil {
		t.Fatal(err)
	}
	tenantCtx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.pool.Acquire(tenantCtx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.Begin(tenantCtx)
	if err != nil {
		connection.Release()
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
SELECT pg_advisory_xact_lock(hashtextextended('vermory:projection-retention:' || $1, 0))`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
INSERT INTO memory_projection_retention (tenant_id)
VALUES ($1)
ON CONFLICT (tenant_id) DO NOTHING`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
DELETE FROM memory_projection_events
WHERE tenant_id = $1 AND event_id <= $2`, tenantID, events[len(events)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
UPDATE memory_projection_retention
SET pruned_through_event_id = $2, last_pruned_at = now(), updated_at = now()
WHERE tenant_id = $1`, tenantID, events[len(events)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
INSERT INTO memory_projection_prune_runs (
  tenant_id, operation_id, request_fingerprint, cutoff, retain_tail_events,
  safe_cursor_event_id, previous_floor_event_id, new_floor_event_id,
  deleted_events, result
) VALUES ($1, $2, $3, $4, 0, $5, 0, $5, $6, 'pruned')`,
		tenantID, normalized.OperationID, fingerprint, normalized.Cutoff,
		events[len(events)-1], len(events),
	); err != nil {
		t.Fatal(err)
	}
	var transactionalEvents, transactionalReceipts int
	var transactionalFloor int64
	if err := tx.QueryRow(tenantCtx, `
SELECT
  (SELECT count(*) FROM memory_projection_events WHERE tenant_id = $1),
  (SELECT pruned_through_event_id FROM memory_projection_retention WHERE tenant_id = $1),
  (SELECT count(*) FROM memory_projection_prune_runs WHERE tenant_id = $1)`, tenantID).Scan(
		&transactionalEvents, &transactionalFloor, &transactionalReceipts,
	); err != nil {
		t.Fatal(err)
	}
	if transactionalEvents != 0 || transactionalFloor != events[len(events)-1] || transactionalReceipts != 1 {
		t.Fatalf("uncommitted prune mutations missing: events=%d floor=%d receipts=%d", transactionalEvents, transactionalFloor, transactionalReceipts)
	}

	cluster.stop(t, "immediate")
	rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), time.Second)
	_ = tx.Rollback(rollbackCtx)
	cancelRollback()
	connection.Release()
	cluster.start(t)
	recoveryDuration := waitForProjectionRetentionStoreRecovery(t, store, 30*time.Second)
	if recoveryDuration > 30*time.Second {
		t.Fatalf("same pool recovery exceeded bound: %s", recoveryDuration)
	}

	assertProjectionEventIDs(t, store, tenantID, events)
	retention, err := store.ProjectionRetention(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if retention.PrunedThroughEventID != 0 || retention.UpdatedAt != nil {
		t.Fatalf("interrupted prune committed retention state: %#v", retention)
	}
	var receiptCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM memory_projection_prune_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, normalized.OperationID).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 0 {
		t.Fatalf("interrupted prune committed %d receipts", receiptCount)
	}
	retry, err := store.PruneProjectionEvents(ctx, tenantID, request)
	if err != nil {
		t.Fatal(err)
	}
	if retry.DeletedEvents != int64(len(events)) || retry.NewFloorEventID != events[len(events)-1] || retry.Replayed {
		t.Fatalf("prune retry after restart failed: %#v", retry)
	}
}

func waitForProjectionRetentionStoreRecovery(t *testing.T, store *Store, timeout time.Duration) time.Duration {
	t.Helper()
	started := time.Now()
	deadline := started.Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		queryCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		version, err := store.SchemaVersion(queryCtx)
		cancel()
		if err == nil && version == 17 {
			return time.Since(started)
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("same projection retention pool did not recover: %v", lastErr)
	return 0
}
