package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProjectionPruneRequestNormalizesAndFingerprints(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	request := ProjectionPruneRequest{
		OperationID:      " prune-operation-1 ",
		Cutoff:           time.Date(2026, 7, 16, 16, 0, 0, 123, location),
		RetainTailEvents: 1000,
	}
	tenantID, normalized, fingerprint, err := normalizeProjectionPruneRequest(" tenant-a ", request)
	if err != nil {
		t.Fatal(err)
	}
	if tenantID != "tenant-a" || normalized.OperationID != "prune-operation-1" ||
		normalized.Cutoff.Location() != time.UTC ||
		!normalized.Cutoff.Equal(time.Date(2026, 7, 16, 8, 0, 0, 123, time.UTC)) ||
		normalized.RetainTailEvents != 1000 {
		t.Fatalf("projection prune request was not normalized: tenant=%q request=%#v", tenantID, normalized)
	}
	if len(fingerprint) != 64 || strings.Trim(fingerprint, "0123456789abcdef") != "" {
		t.Fatalf("invalid projection prune fingerprint: %q", fingerprint)
	}
	_, _, replayFingerprint, err := normalizeProjectionPruneRequest("tenant-a", normalized)
	if err != nil || replayFingerprint != fingerprint {
		t.Fatalf("normalized request fingerprint drifted: %q %v", replayFingerprint, err)
	}
	changed := normalized
	changed.RetainTailEvents++
	_, _, changedFingerprint, err := normalizeProjectionPruneRequest("tenant-a", changed)
	if err != nil || changedFingerprint == fingerprint {
		t.Fatalf("changed request retained fingerprint: %q %v", changedFingerprint, err)
	}
}

func TestProjectionPruneRequestRejectsInvalidInputs(t *testing.T) {
	valid := ProjectionPruneRequest{
		OperationID: "projection-prune-valid",
		Cutoff:      time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC),
	}
	for name, test := range map[string]struct {
		tenantID string
		mutate   func(*ProjectionPruneRequest)
	}{
		"tenant":         {tenantID: " ", mutate: func(*ProjectionPruneRequest) {}},
		"operation":      {tenantID: "tenant-a", mutate: func(request *ProjectionPruneRequest) { request.OperationID = "" }},
		"operation size": {tenantID: "tenant-a", mutate: func(request *ProjectionPruneRequest) { request.OperationID = strings.Repeat("x", 129) }},
		"cutoff":         {tenantID: "tenant-a", mutate: func(request *ProjectionPruneRequest) { request.Cutoff = time.Time{} }},
		"tail":           {tenantID: "tenant-a", mutate: func(request *ProjectionPruneRequest) { request.RetainTailEvents = -1 }},
	} {
		t.Run(name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if _, _, _, err := normalizeProjectionPruneRequest(test.tenantID, request); err == nil {
				t.Fatalf("invalid projection prune request was accepted: tenant=%q request=%#v", test.tenantID, request)
			}
		})
	}
}

func TestProjectionPruneStopsAtSlowCursorThenRetainsTail(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantAEvents := seedProjectionPruneEvents(t, store, "prune-bound-a", 6)
	tenantBEvents := seedProjectionPruneEvents(t, store, "prune-bound-b", 4)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_events
SET created_at = $1
WHERE tenant_id = ANY($2::text[])`, old, []string{"prune-bound-a", "prune-bound-b"}); err != nil {
		t.Fatal(err)
	}
	for profileID, cursor := range map[string]struct {
		last   int64
		status string
	}{
		ProductionRetrievalProfileID:           {last: tenantAEvents[len(tenantAEvents)-1], status: "idle"},
		DimensionalMigrationRetrievalProfileID: {last: tenantAEvents[2], status: "idle"},
		MigrationRetrievalProfileID:            {last: 0, status: ProjectionStatusRebuildRequired},
	} {
		if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, $3, $4)`, "prune-bound-a", profileID, cursor.last, cursor.status); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.PruneProjectionEvents(ctx, "prune-bound-a", ProjectionPruneRequest{
		OperationID: "prune-bound-first", Cutoff: old.Add(24 * time.Hour), RetainTailEvents: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.SafeCursorEventID != tenantAEvents[2] || first.PreviousFloorEventID != 0 ||
		first.NewFloorEventID != tenantAEvents[2] || first.DeletedEvents != 3 || first.Result != "pruned" || first.Replayed {
		t.Fatalf("prune passed or missed the slow cursor: %#v", first)
	}
	assertProjectionEventIDs(t, store, "prune-bound-a", tenantAEvents[3:])
	assertProjectionEventIDs(t, store, "prune-bound-b", tenantBEvents)

	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_cursors
SET last_event_id = $3
WHERE tenant_id = $1 AND profile_id = $2`,
		"prune-bound-a", DimensionalMigrationRetrievalProfileID, tenantAEvents[len(tenantAEvents)-1],
	); err != nil {
		t.Fatal(err)
	}
	second, err := store.PruneProjectionEvents(ctx, "prune-bound-a", ProjectionPruneRequest{
		OperationID: "prune-bound-second", Cutoff: old.Add(24 * time.Hour), RetainTailEvents: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.PreviousFloorEventID != tenantAEvents[2] || second.NewFloorEventID != tenantAEvents[3] ||
		second.DeletedEvents != 1 || second.Result != "pruned" {
		t.Fatalf("tail retention bound was not enforced: %#v", second)
	}
	assertProjectionEventIDs(t, store, "prune-bound-a", tenantAEvents[4:])
	retention, err := store.ProjectionRetention(ctx, "prune-bound-a")
	if err != nil || retention.PrunedThroughEventID != tenantAEvents[3] {
		t.Fatalf("unexpected retained floor: %#v err=%v", retention, err)
	}
}

func TestProjectionPruneReplayIsStableAndConflictIsRejected(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	events := seedProjectionPruneEvents(t, store, "prune-replay", 2)
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_events SET created_at = '2026-07-01T00:00:00Z'
WHERE tenant_id = 'prune-replay'`); err != nil {
		t.Fatal(err)
	}
	request := ProjectionPruneRequest{
		OperationID: "prune-replay-operation",
		Cutoff:      time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	}
	first, err := store.PruneProjectionEvents(ctx, "prune-replay", request)
	if err != nil {
		t.Fatal(err)
	}
	if first.NewFloorEventID != events[1] || first.DeletedEvents != 2 || first.Replayed {
		t.Fatalf("unexpected first prune receipt: %#v", first)
	}
	var persistedBefore string
	if err := store.pool.QueryRow(ctx, `
SELECT to_jsonb(run)::text
FROM memory_projection_prune_runs run
WHERE tenant_id = $1 AND operation_id = $2`, "prune-replay", request.OperationID).Scan(&persistedBefore); err != nil {
		t.Fatal(err)
	}
	replay, err := store.PruneProjectionEvents(ctx, " prune-replay ", request)
	if err != nil {
		t.Fatal(err)
	}
	first.Replayed = true
	if !reflect.DeepEqual(replay, first) {
		t.Fatalf("projection prune replay drifted: first=%#v replay=%#v", first, replay)
	}
	var persistedAfter string
	if err := store.pool.QueryRow(ctx, `
SELECT to_jsonb(run)::text
FROM memory_projection_prune_runs run
WHERE tenant_id = $1 AND operation_id = $2`, "prune-replay", request.OperationID).Scan(&persistedAfter); err != nil {
		t.Fatal(err)
	}
	if persistedAfter != persistedBefore {
		t.Fatalf("projection prune replay changed persisted receipt: before=%s after=%s", persistedBefore, persistedAfter)
	}
	conflict := request
	conflict.RetainTailEvents = 1
	if _, err := store.PruneProjectionEvents(ctx, "prune-replay", conflict); err == nil ||
		!strings.Contains(err.Error(), "projection prune operation conflict") {
		t.Fatalf("conflicting projection prune replay was accepted: %v", err)
	}
}

func TestProjectionPruneNoopIsAudited(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	seedProjectionPruneEvents(t, store, "prune-noop", 2)
	receipt, err := store.PruneProjectionEvents(ctx, "prune-noop", ProjectionPruneRequest{
		OperationID:      "prune-noop-operation",
		Cutoff:           time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		RetainTailEvents: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result != "noop" || receipt.DeletedEvents != 0 ||
		receipt.PreviousFloorEventID != 0 || receipt.NewFloorEventID != 0 {
		t.Fatalf("unexpected noop receipt: %#v", receipt)
	}
	var receiptCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_projection_prune_runs
WHERE tenant_id = 'prune-noop' AND operation_id = 'prune-noop-operation'`).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 {
		t.Fatalf("noop receipt count=%d want 1", receiptCount)
	}
}

func TestProjectionPruneDoesNotDeleteEventInsertedAfterBoundSelection(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "prune-concurrent-insert"
	events := seedProjectionPruneEvents(t, store, tenantID, 3)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_events SET created_at = $2 WHERE tenant_id = $1`, tenantID, old); err != nil {
		t.Fatal(err)
	}
	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err := blocker.Exec(ctx, `
SELECT event_id
FROM memory_projection_events
WHERE tenant_id = $1 AND event_id = $2
FOR UPDATE`, tenantID, events[0]); err != nil {
		t.Fatal(err)
	}
	receiptCh := make(chan ProjectionPruneReceipt, 1)
	errCh := make(chan error, 1)
	go func() {
		receipt, pruneErr := store.PruneProjectionEvents(context.Background(), tenantID, ProjectionPruneRequest{
			OperationID: "prune-concurrent-insert-operation",
			Cutoff:      old.Add(24 * time.Hour), RetainTailEvents: 0,
		})
		receiptCh <- receipt
		errCh <- pruneErr
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		if err := store.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_stat_activity
  WHERE datname = current_database()
    AND pid <> pg_backend_pid()
    AND wait_event_type = 'Lock'
    AND query LIKE '%DELETE FROM memory_projection_events%'
)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("projection prune did not block after bound selection")
		}
		time.Sleep(10 * time.Millisecond)
	}
	governance := NewGovernanceService(store, tenantID)
	receipt, err := governance.AddSource(ctx, "/fixtures/"+tenantID, GovernanceWriteRequest{
		OperationID: "prune-concurrent-insert-late-source",
		MemoryKey:   "prune.concurrent.late", Content: "Late event must survive the selected prune bound.",
		SourceRef: "fixture:prune-concurrent-insert-late",
	})
	if err != nil {
		t.Fatal(err)
	}
	var lateEventID int64
	if err := store.pool.QueryRow(ctx, `
UPDATE memory_projection_events
SET created_at = $3
WHERE tenant_id = $1 AND memory_id = $2::uuid
RETURNING event_id`, tenantID, receipt.Memory.MemoryID, old).Scan(&lateEventID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pruneReceipt := <-receiptCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if pruneReceipt.NewFloorEventID != events[2] || pruneReceipt.DeletedEvents != 3 {
		t.Fatalf("unexpected blocked prune receipt: %#v", pruneReceipt)
	}
	assertProjectionEventIDs(t, store, tenantID, []int64{lateEventID})
}

func TestProjectionPruneConcurrentOperationsKeepFloorMonotonic(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "prune-concurrent-operations"
	events := seedProjectionPruneEvents(t, store, tenantID, 6)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_events SET created_at = $2 WHERE tenant_id = $1`, tenantID, old); err != nil {
		t.Fatal(err)
	}
	requests := []ProjectionPruneRequest{
		{OperationID: "prune-concurrent-tail-three", Cutoff: old.Add(24 * time.Hour), RetainTailEvents: 3},
		{OperationID: "prune-concurrent-tail-one", Cutoff: old.Add(24 * time.Hour), RetainTailEvents: 1},
	}
	var wait sync.WaitGroup
	wait.Add(len(requests))
	errorsCh := make(chan error, len(requests))
	for _, request := range requests {
		request := request
		go func() {
			defer wait.Done()
			_, err := store.PruneProjectionEvents(context.Background(), tenantID, request)
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.pool.Query(ctx, `
SELECT previous_floor_event_id, new_floor_event_id
FROM memory_projection_prune_runs
WHERE tenant_id = $1
ORDER BY created_at, id`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lastFloor := int64(0)
	runs := 0
	for rows.Next() {
		var previousFloor, newFloor int64
		if err := rows.Scan(&previousFloor, &newFloor); err != nil {
			t.Fatal(err)
		}
		if previousFloor != lastFloor || newFloor < previousFloor {
			t.Fatalf("non-monotonic concurrent receipt: previous=%d new=%d last=%d", previousFloor, newFloor, lastFloor)
		}
		lastFloor = newFloor
		runs++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if runs != 2 || lastFloor != events[4] {
		t.Fatalf("concurrent prune runs=%d final floor=%d want %d", runs, lastFloor, events[4])
	}
	assertProjectionEventIDs(t, store, tenantID, events[5:])
}

func seedProjectionPruneEvents(t *testing.T, store *Store, tenantID string, count int) []int64 {
	t.Helper()
	repoRoot := "/fixtures/" + tenantID
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		if _, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
			OperationID: tenantID + "-source-" + string(rune('a'+index)),
			MemoryKey:   tenantID + ".memory." + string(rune('a'+index)),
			Content:     "Projection prune fact " + tenantID + " " + string(rune('A'+index)) + ".",
			SourceRef:   "fixture:" + tenantID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.pool.Query(context.Background(), `
SELECT event_id
FROM memory_projection_events
WHERE tenant_id = $1
ORDER BY event_id`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	eventIDs := make([]int64, 0, count)
	for rows.Next() {
		var eventID int64
		if err := rows.Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		eventIDs = append(eventIDs, eventID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(eventIDs) != count {
		t.Fatalf("seeded projection events=%d want %d", len(eventIDs), count)
	}
	return eventIDs
}

func assertProjectionEventIDs(t *testing.T, store *Store, tenantID string, want []int64) {
	t.Helper()
	rows, err := store.pool.Query(context.Background(), `
SELECT event_id
FROM memory_projection_events
WHERE tenant_id = $1
ORDER BY event_id`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := make([]int64, 0, len(want))
	for rows.Next() {
		var eventID int64
		if err := rows.Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		got = append(got, eventID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("tenant %s projection events=%s want %s", tenantID, gotJSON, wantJSON)
	}
}
