package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDimensionalProjectionClassRoutesWorkerSearchAndReset(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-dimension-routing")
	defer store.Close()
	ctx := context.Background()
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)

	candidateWorker, err := NewProjectionWorker(store, &projectionTestEmbedder{vector: testVector2560(1)}, ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   dimensionalMigrationProfile(t),
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := candidateWorker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("candidate worker failed: result=%#v err=%v", result, err)
	}
	if result.Processed != 1 || result.Lag != 0 {
		t.Fatalf("candidate worker result=%#v", result)
	}
	assertDimensionalVectorPresence(t, store, active.Memory.MemoryID, true)
	assertProfileVectorPresence(t, store, DimensionalMigrationRetrievalProfileID, active.Memory.MemoryID, false)

	candidateStatus, err := store.RetrievalProjectionStatus(ctx, tenantID, DimensionalMigrationRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if candidateStatus.VectorCount != 1 || candidateStatus.Lag != 0 {
		t.Fatalf("candidate status=%#v", candidateStatus)
	}

	candidateCoordinator, err := NewRetrievalCoordinator(
		store,
		&projectionTestEmbedder{vector: testVector2560(1)},
		dimensionalMigrationProfile(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidateRetrieval, err := candidateCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID:   "dimension-routing-candidate",
		TenantID:      tenantID,
		ContinuityIDs: []string{continuityID},
		Query:         "Which governed fact is current?",
		Limit:         1,
		Mode:          RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidateRetrieval.Effective != RetrievalVector || len(candidateRetrieval.Memories) != 1 ||
		candidateRetrieval.Memories[0].ID != active.Memory.MemoryID {
		t.Fatalf("candidate retrieval=%#v", candidateRetrieval)
	}

	incumbentWorker := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(1)}, tenantID, 8)
	if incumbentResult, incumbentErr := incumbentWorker.RunOnce(ctx); incumbentErr != nil || incumbentResult.Lag != 0 {
		t.Fatalf("incumbent worker result=%#v err=%v", incumbentResult, incumbentErr)
	}
	incumbentStatus, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if incumbentStatus.VectorCount != 1 || incumbentStatus.Lag != 0 {
		t.Fatalf("incumbent status=%#v", incumbentStatus)
	}
	incumbentCoordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVector1024(1)})
	if _, err := incumbentCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID:   "dimension-routing-candidate",
		TenantID:      tenantID,
		ContinuityIDs: []string{continuityID},
		Query:         "Which governed fact is current?",
		Limit:         1,
		Mode:          RetrievalVector,
	}); err == nil || !strings.Contains(err.Error(), "operation conflict") {
		t.Fatalf("cross-profile operation was replayed instead of rejected: %v", err)
	}

	if err := store.ResetVectorProjection(ctx, tenantID, DimensionalMigrationRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	candidateStatus, err = store.RetrievalProjectionStatus(ctx, tenantID, DimensionalMigrationRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	incumbentAfterReset, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if candidateStatus.VectorCount != 0 || candidateStatus.LastEventID != 0 || !projectionStatusCoreEqual(incumbentAfterReset, incumbentStatus) {
		t.Fatalf("candidate reset crossed class boundary: candidate=%#v incumbent_before=%#v incumbent_after=%#v", candidateStatus, incumbentStatus, incumbentAfterReset)
	}

	rebuild, err := candidateWorker.RebuildCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuild.Projected != 1 || rebuild.Lag != 0 {
		t.Fatalf("candidate rebuild=%#v", rebuild)
	}
	assertDimensionalVectorPresence(t, store, active.Memory.MemoryID, true)
	incumbentAfterRebuild, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if !projectionStatusCoreEqual(incumbentAfterRebuild, incumbentStatus) {
		t.Fatalf("candidate rebuild changed incumbent: before=%#v after=%#v", incumbentStatus, incumbentAfterRebuild)
	}
}

func TestDimensionalProjectionProfilesUseIndependentLocks(t *testing.T) {
	store, tenantID, _, _ := seedProjectionWorkerActive(t, "retrieval-dimension-locks")
	defer store.Close()
	blocking := &projectionTestEmbedder{
		vector:  testVector2560(0.7),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	candidate, err := NewProjectionWorker(store, blocking, ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   dimensionalMigrationProfile(t),
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidateResult := make(chan ProjectionRunResult, 1)
	candidateErr := make(chan error, 1)
	go func() {
		result, runErr := candidate.RunOnce(context.Background())
		candidateResult <- result
		candidateErr <- runErr
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("candidate did not acquire its lock and reach embedding")
	}

	incumbent := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(0.7)}, tenantID, 8)
	incumbentResult, incumbentErr := incumbent.RunOnce(context.Background())
	if incumbentErr != nil || incumbentResult.AlreadyRunning || incumbentResult.Processed != 1 {
		t.Fatalf("incumbent was blocked by candidate lock: result=%#v err=%v", incumbentResult, incumbentErr)
	}
	close(blocking.release)
	if result, runErr := <-candidateResult, <-candidateErr; runErr != nil || result.AlreadyRunning || result.Processed != 1 {
		t.Fatalf("candidate did not finish after release: result=%#v err=%v", result, runErr)
	}
}

func TestDimensionalProjectionWorkerRejectsWrongDimensionsWithoutCursorAdvance(t *testing.T) {
	store, tenantID, _, _ := seedProjectionWorkerActive(t, "retrieval-dimension-mismatch")
	defer store.Close()
	worker, err := NewProjectionWorker(store, &projectionTestEmbedder{vector: testVector1024(1)}, ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   dimensionalMigrationProfile(t),
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, runErr := worker.RunOnce(context.Background())
	if runErr == nil || result.FailureCode != "embedding_dimension_mismatch" || result.LastEventID != 0 {
		t.Fatalf("wrong dimensions advanced candidate: result=%#v err=%v", result, runErr)
	}
	assertDimensionalVectorPresence(t, store, resultMemoryID(t, store, tenantID), false)
}

func TestDimensionalProjectionLateEmbeddingCannotRestoreDeletedMemory(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-dimension-late-delete")
	defer store.Close()
	blocking := &projectionTestEmbedder{
		vector:  testVector2560(0.8),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	worker, err := NewProjectionWorker(store, blocking, ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   dimensionalMigrationProfile(t),
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	resultCh := make(chan ProjectionRunResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, runErr := worker.RunOnce(context.Background())
		resultCh <- result
		errCh <- runErr
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("dimensional candidate did not reach embedding")
	}
	if _, err := NewGovernanceService(store, tenantID).Forget(
		context.Background(), repoRoot, active.Memory.MemoryID, "dimension-late-delete",
	); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	result := <-resultCh
	runErr := <-errCh
	if runErr == nil || result.FailureCode != "authority_changed" {
		t.Fatalf("late candidate authority change was not detected: result=%#v err=%v", result, runErr)
	}
	assertDimensionalVectorPresence(t, store, active.Memory.MemoryID, false)

	retry, err := NewProjectionWorker(store, &projectionTestEmbedder{vector: testVector2560(0.1)}, ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   dimensionalMigrationProfile(t),
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retry.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertDimensionalVectorPresence(t, store, active.Memory.MemoryID, false)
}

func dimensionalMigrationProfile(t *testing.T) RetrievalProfile {
	t.Helper()
	spec, ok := SupportedRetrievalProfile(DimensionalMigrationRetrievalProfileID)
	if !ok {
		t.Fatal("dimensional migration profile is not registered")
	}
	return RetrievalProfile{
		ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
		Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
	}
}

func testVector2560(value float32) []float32 {
	vector := make([]float32, 2560)
	vector[0] = value
	return vector
}

func assertDimensionalVectorPresence(t *testing.T, store *Store, memoryID string, want bool) {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_vector_documents_2560
WHERE profile_id = $1 AND memory_id = $2::uuid`,
		DimensionalMigrationRetrievalProfileID, memoryID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if got := count == 1; got != want {
		t.Fatalf("dimensional vector presence=%t want %t for %s", got, want, memoryID)
	}
}

func resultMemoryID(t *testing.T, store *Store, tenantID string) string {
	t.Helper()
	var memoryID string
	if err := store.pool.QueryRow(context.Background(), `
SELECT id::text
FROM governed_memories
WHERE tenant_id = $1 AND lifecycle_status = 'active'
ORDER BY id
LIMIT 1`, tenantID).Scan(&memoryID); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(memoryID) == "" {
		t.Fatal("active memory ID is empty")
	}
	return memoryID
}

func projectionStatusCoreEqual(left, right ProjectionStatus) bool {
	return left.TenantID == right.TenantID &&
		left.ProfileID == right.ProfileID &&
		left.LastEventID == right.LastEventID &&
		left.LatestEventID == right.LatestEventID &&
		left.Lag == right.Lag &&
		left.Status == right.Status &&
		left.AttemptCount == right.AttemptCount &&
		left.LastErrorCode == right.LastErrorCode &&
		left.VectorCount == right.VectorCount
}
