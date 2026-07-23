package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectionRetentionDefaultsToZero(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	retention, err := store.ProjectionRetention(ctx, "retention-default-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if retention.TenantID != "retention-default-tenant" || retention.PrunedThroughEventID != 0 ||
		retention.LastPrunedAt != nil || retention.UpdatedAt != nil {
		t.Fatalf("unexpected default retention: %#v", retention)
	}
	var rows int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM memory_projection_retention
WHERE tenant_id = 'retention-default-tenant'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("retention read created %d rows", rows)
	}
}

func TestProjectionRetentionReadsPersistedFloor(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	lastPruned := time.Date(2026, 7, 16, 7, 30, 0, 0, time.UTC)
	updated := lastPruned.Add(time.Minute)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_retention (
  tenant_id, pruned_through_event_id, last_pruned_at, updated_at
) VALUES ($1, 42, $2, $3)`, "retention-persisted-tenant", lastPruned, updated); err != nil {
		t.Fatal(err)
	}
	retention, err := store.ProjectionRetention(ctx, "retention-persisted-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if retention.TenantID != "retention-persisted-tenant" || retention.PrunedThroughEventID != 42 ||
		retention.LastPrunedAt == nil || !retention.LastPrunedAt.Equal(lastPruned) ||
		retention.UpdatedAt == nil || !retention.UpdatedAt.Equal(updated) {
		t.Fatalf("unexpected persisted retention: %#v", retention)
	}
}

func TestProjectionStatusReportsRetentionFloor(t *testing.T) {
	status := ProjectionStatus{
		TenantID:             "retention-status-tenant",
		ProfileID:            ProductionRetrievalProfileID,
		Status:               ProjectionStatusRebuildRequired,
		LastEventID:          40,
		PrunedThroughEventID: 42,
		RebuildRequired:      true,
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, fragment := range []string{
		`"status":"rebuild_required"`,
		`"pruned_through_event_id":42`,
		`"rebuild_required":true`,
	} {
		if !strings.Contains(encoded, fragment) {
			t.Fatalf("projection status JSON lacks %s: %s", fragment, encoded)
		}
	}
}

func setProjectionRetentionFloor(t *testing.T, store *Store, tenantID string, floor int64) {
	t.Helper()
	if _, err := store.pool.Exec(context.Background(), `
INSERT INTO memory_projection_retention (
  tenant_id, pruned_through_event_id, last_pruned_at, updated_at
) VALUES ($1, $2, now(), now())
ON CONFLICT (tenant_id) DO UPDATE SET
  pruned_through_event_id = EXCLUDED.pruned_through_event_id,
  last_pruned_at = EXCLUDED.last_pruned_at,
  updated_at = EXCLUDED.updated_at`, tenantID, floor); err != nil {
		t.Fatal(err)
	}
}

func latestProjectionEventID(t *testing.T, store *Store, tenantID string) int64 {
	t.Helper()
	var eventID int64
	if err := store.pool.QueryRow(context.Background(), `
SELECT COALESCE(max(event_id), 0)
FROM memory_projection_events
WHERE tenant_id = $1`, tenantID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	return eventID
}
