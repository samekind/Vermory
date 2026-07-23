package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryEligibilityForgetRace(t *testing.T) {
	t.Run("validity commits before forget", func(t *testing.T) {
		store := openTestStore(t)
		ctx := context.Background()
		tenantID := "eligibility-race-validity-first"
		continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
		memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
			TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
			Content: "RACE-VALIDITY-FIRST protected content",
		})
		locked := make(chan struct{})
		release := make(chan struct{})
		store.memoryEligibilityAfterTargetLock = func() {
			close(locked)
			<-release
		}
		validUntil := time.Now().UTC().Add(time.Hour)
		validityDone := make(chan error, 1)
		go func() {
			_, err := store.SetMemoryValidity(ctx, SetMemoryValidityRequest{
				OperationID: "eligibility-race-validity-first", TenantID: tenantID,
				ContinuityID: continuityID, MemoryID: memoryID, ValidUntil: &validUntil,
			})
			validityDone <- err
		}()
		waitForEligibilitySignal(t, locked, "validity target lock")
		forgetDone := make(chan error, 1)
		go func() { forgetDone <- store.DeleteMemory(ctx, tenantID, continuityID, memoryID) }()
		assertEligibilityOperationBlocked(t, forgetDone, "forget while validity holds row lock")
		close(release)
		if err := waitForEligibilityResult(t, validityDone, "validity result"); err != nil {
			t.Fatal(err)
		}
		if err := waitForEligibilityResult(t, forgetDone, "forget result"); err != nil {
			t.Fatal(err)
		}
		assertForgottenEligibilityMemory(t, store, tenantID, continuityID, memoryID, "RACE-VALIDITY-FIRST")
		var receipts int
		if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_eligibility_operations
WHERE tenant_id = $1 AND operation_id = 'eligibility-race-validity-first'`, tenantID).Scan(&receipts); err != nil {
			t.Fatal(err)
		}
		if receipts != 1 {
			t.Fatalf("serialized validity receipt count=%d want 1", receipts)
		}
	})

	t.Run("forget commits before validity", func(t *testing.T) {
		store := openTestStore(t)
		ctx := context.Background()
		tenantID := "eligibility-race-forget-first"
		continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
		memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
			TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
			Content: "RACE-FORGET-FIRST protected content",
		})
		locked := make(chan struct{})
		release := make(chan struct{})
		store.memoryDeleteAfterTargetLock = func() {
			close(locked)
			<-release
		}
		forgetDone := make(chan error, 1)
		go func() { forgetDone <- store.DeleteMemory(ctx, tenantID, continuityID, memoryID) }()
		waitForEligibilitySignal(t, locked, "forget target lock")
		validUntil := time.Now().UTC().Add(time.Hour)
		validityDone := make(chan error, 1)
		go func() {
			_, err := store.SetMemoryValidity(ctx, SetMemoryValidityRequest{
				OperationID: "eligibility-race-forget-first", TenantID: tenantID,
				ContinuityID: continuityID, MemoryID: memoryID, ValidUntil: &validUntil,
			})
			validityDone <- err
		}()
		assertEligibilityOperationBlocked(t, validityDone, "validity while forget holds row lock")
		close(release)
		if err := waitForEligibilityResult(t, forgetDone, "forget result"); err != nil {
			t.Fatal(err)
		}
		if err := waitForEligibilityResult(t, validityDone, "validity result"); err == nil {
			t.Fatal("late validity operation returned false success after forget")
		}
		assertForgottenEligibilityMemory(t, store, tenantID, continuityID, memoryID, "RACE-FORGET-FIRST")
		var receipts int
		if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_eligibility_operations
WHERE tenant_id = $1 AND operation_id = 'eligibility-race-forget-first'`, tenantID).Scan(&receipts); err != nil {
			t.Fatal(err)
		}
		if receipts != 0 {
			t.Fatalf("failed late validity operation persisted receipts=%d", receipts)
		}
	})
}

func TestMemoryEligibilityImmediateStopRollback(t *testing.T) {
	cluster := startDisposablePostgres18(t)
	ctx := context.Background()
	store, err := OpenStore(ctx, cluster.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := "eligibility-immediate-stop"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "IMMEDIATE-ROLLBACK archive transaction",
	})
	ready := make(chan struct{})
	release := make(chan struct{})
	store.memoryEligibilityBeforeCommit = func() {
		close(ready)
		<-release
	}
	archiveDone := make(chan error, 1)
	go func() {
		_, err := store.ArchiveMemory(ctx, ArchiveMemoryRequest{
			OperationID: "eligibility-immediate-stop", TenantID: tenantID,
			ContinuityID: continuityID, MemoryID: memoryID,
		})
		archiveDone <- err
	}()
	waitForEligibilitySignal(t, ready, "archive before commit")
	cluster.stop(t, "immediate")
	close(release)
	if err := waitForEligibilityResult(t, archiveDone, "interrupted archive"); err == nil {
		t.Fatal("immediate stop returned a false archive success")
	}
	cluster.start(t)
	waitForStoreRecovery(t, store)
	store.memoryEligibilityBeforeCommit = nil

	var lifecycle, content string
	var lexicalRows, receipts int
	if err := store.pool.QueryRow(ctx, `
SELECT lifecycle_status, content FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&lifecycle, &content); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_search_documents WHERE memory_id = $1::uuid`, memoryID).Scan(&lexicalRows); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_eligibility_operations
WHERE tenant_id = $1 AND operation_id = 'eligibility-immediate-stop'`, tenantID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	var desiredState string
	if err := store.pool.QueryRow(ctx, `
SELECT desired_state FROM memory_projection_events
WHERE tenant_id = $1 AND memory_id = $2::uuid
ORDER BY event_id DESC LIMIT 1`, tenantID, memoryID).Scan(&desiredState); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "active" || content != "IMMEDIATE-ROLLBACK archive transaction" || lexicalRows != 1 || receipts != 0 || desiredState != "active" {
		t.Fatalf("interrupted archive committed partial state: lifecycle=%s content=%q lexical=%d receipts=%d projection=%s", lifecycle, content, lexicalRows, receipts, desiredState)
	}
	retry, err := store.ArchiveMemory(ctx, ArchiveMemoryRequest{
		OperationID: "eligibility-immediate-stop", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: memoryID,
	})
	if err != nil || retry.Replayed || retry.ResultState != MemoryEffectiveArchived {
		t.Fatalf("archive retry after restart failed: receipt=%#v err=%v", retry, err)
	}
}

func TestMemoryEligibilityStaleVector(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-stale-vector"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	asOf := time.Now().UTC()
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-STALE-VECTOR current procedure",
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-STALE-VECTOR expired procedure", ValidUntil: &asOf,
	})
	embedder := &projectionTestEmbedder{vector: testVector1024(0.5)}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 8)
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var vectorRows int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, ProductionRetrievalProfileID).Scan(&vectorRows); err != nil {
		t.Fatal(err)
	}
	if vectorRows != 2 {
		t.Fatalf("stale-vector control missing: rows=%d", vectorRows)
	}
	result, err := mustRetrievalCoordinator(t, store, embedder).Retrieve(ctx, RetrievalRequest{
		OperationID: "eligibility-stale-vector-query", TenantID: tenantID,
		ContinuityIDs: []string{continuityID}, Query: "ELIGIBILITY-STALE-VECTOR procedure",
		Limit: 5, Mode: RetrievalVector, EligibilityAsOf: asOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, result.Memories, []string{currentID})
	outage, err := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{err: errors.New("embedding timeout")}).Retrieve(ctx, RetrievalRequest{
		OperationID: "eligibility-stale-vector-outage", TenantID: tenantID,
		ContinuityIDs: []string{continuityID}, Query: "ELIGIBILITY-STALE-VECTOR procedure",
		Limit: 5, Mode: RetrievalVector, EligibilityAsOf: asOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outage.Degraded || outage.Effective != RetrievalLexical {
		t.Fatalf("embedding outage did not degrade safely: %#v", outage)
	}
	assertMemoryIDSet(t, outage.Memories, []string{currentID})
}

func waitForEligibilitySignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func assertEligibilityOperationBlocked(t *testing.T, result <-chan error, label string) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("%s completed before lock release: %v", label, err)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForEligibilityResult(t *testing.T, result <-chan error, label string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		return nil
	}
}

func assertForgottenEligibilityMemory(t *testing.T, store *Store, tenantID, continuityID, memoryID, forbidden string) {
	t.Helper()
	var lifecycle, content string
	if err := store.pool.QueryRow(context.Background(), `
SELECT lifecycle_status, content FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&lifecycle, &content); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "deleted" || content != "[redacted]" {
		t.Fatalf("forget race left lifecycle=%s content=%q", lifecycle, content)
	}
	matches, err := store.SearchActiveMemory(context.Background(), tenantID, continuityID, forbidden, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("forgotten race content remained searchable: %#v", matches)
	}
}
