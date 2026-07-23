package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vermory/internal/memorybackend"

	"github.com/jackc/pgx/v5/pgxpool"
)

type projectionTestEmbedder struct {
	vector  []float32
	err     error
	calls   atomic.Int64
	started chan struct{}
	release chan struct{}
}

type projectionBatchTestEmbedder struct {
	vector      []float32
	singleCalls atomic.Int64
	batchCalls  atomic.Int64
	batchItems  atomic.Int64
}

func (embedder *projectionBatchTestEmbedder) Embed(context.Context, string) ([]float32, error) {
	embedder.singleCalls.Add(1)
	return append([]float32(nil), embedder.vector...), nil
}

func (embedder *projectionBatchTestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	embedder.batchCalls.Add(1)
	embedder.batchItems.Add(int64(len(texts)))
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = append([]float32(nil), embedder.vector...)
	}
	return vectors, nil
}

func TestProjectionWorkerBatchesDurableEventsWithoutChangingAuthority(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-worker-batch"
	repoRoot := "/fixtures/retrieval-worker-batch"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	for index, content := range []string{"first active fact", "second active fact", "third active fact"} {
		if _, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
			OperationID: fmt.Sprintf("retrieval-worker-batch-%d", index),
			MemoryKey:   fmt.Sprintf("batch.fact.%d", index),
			Content:     content,
			SourceRef:   fmt.Sprintf("fixture:batch:%d", index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	embedder := &projectionBatchTestEmbedder{vector: testVector1024(0.25)}
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID:           tenantID,
		Profile:            productionRetrievalProfile(t),
		BatchSize:          8,
		EmbeddingBatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 3 || result.Lag != 0 || result.Status != "idle" {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	if embedder.batchCalls.Load() != 1 || embedder.batchItems.Load() != 3 || embedder.singleCalls.Load() != 0 {
		t.Fatalf("unexpected provider calls: batch=%d items=%d single=%d", embedder.batchCalls.Load(), embedder.batchItems.Load(), embedder.singleCalls.Load())
	}
	status, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.VectorCount != 3 || status.Lag != 0 {
		t.Fatalf("batch projection diverged from authority: %#v", status)
	}
}

func TestProjectionWorkerRequiresRebuildBelowRetentionFloor(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retention-worker-below-floor")
	ctx := context.Background()
	floor := latestProjectionEventID(t, store, tenantID)
	setProjectionRetentionFloor(t, store, tenantID, floor)
	if _, err := store.pool.Exec(ctx, `
DELETE FROM memory_projection_events
WHERE tenant_id = $1 AND event_id <= $2`, tenantID, floor); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, 0, 'idle')`, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	embedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 8)
	result, err := worker.RunOnce(ctx)
	if err == nil || result.FailureCode != ProjectionFailureRebuildRequired ||
		result.Status != ProjectionStatusRebuildRequired || result.LastEventID != 0 || result.Lag != 0 {
		t.Fatalf("worker silently skipped pruned history: result=%#v err=%v", result, err)
	}
	if embedder.calls.Load() != 0 {
		t.Fatalf("worker embedded below the floor: calls=%d", embedder.calls.Load())
	}
	status, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastEventID != 0 || status.PrunedThroughEventID != floor ||
		status.Status != ProjectionStatusRebuildRequired || status.LastErrorCode != ProjectionFailureRebuildRequired {
		t.Fatalf("unexpected rebuild-required cursor: %#v", status)
	}

	rebuild, err := worker.RebuildCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuild.Watermark != floor || rebuild.LastEventID != floor || rebuild.Lag != 0 ||
		rebuild.Status != "idle" || rebuild.Projected != 1 {
		t.Fatalf("rebuild did not recover from the retention floor: %#v", rebuild)
	}
	if embedder.calls.Load() != 1 {
		t.Fatalf("authority rebuild calls=%d want 1", embedder.calls.Load())
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, true)

	revised, err := NewGovernanceService(store, tenantID).ReviseSource(ctx, repoRoot, active.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "retention-worker-after-rebuild",
		MemoryKey:   "projection.current",
		Content:     "Current projection fact after retention rebuild.",
		SourceRef:   "fixture:retention-worker-after-rebuild",
	})
	if err != nil {
		t.Fatal(err)
	}
	tail, err := worker.RunOnce(ctx)
	if err != nil || tail.Lag != 0 || tail.Status != "idle" {
		t.Fatalf("worker did not drain the retained tail: result=%#v err=%v", tail, err)
	}
	assertVectorPresence(t, store, revised.Memory.MemoryID, true)
}

func TestProjectionWorkerCreatesFutureSubscriberAsRebuildRequired(t *testing.T) {
	store, tenantID, _, _ := seedProjectionWorkerActive(t, "retention-worker-future")
	ctx := context.Background()
	floor := latestProjectionEventID(t, store, tenantID)
	setProjectionRetentionFloor(t, store, tenantID, floor)
	if _, err := store.pool.Exec(ctx, `DELETE FROM memory_projection_events WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	spec, ok := SupportedRetrievalProfile(MigrationRetrievalProfileID)
	if !ok {
		t.Fatal("future subscriber profile is not registered")
	}
	embedder := &projectionTestEmbedder{vector: testVector1024(0.5)}
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID: tenantID,
		Profile: RetrievalProfile{
			ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
			Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
		},
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.RunOnce(ctx)
	if err == nil || result.FailureCode != ProjectionFailureRebuildRequired ||
		result.LastEventID != floor || result.Status != ProjectionStatusRebuildRequired {
		t.Fatalf("future subscriber pretended to consume pruned history: result=%#v err=%v", result, err)
	}
	if embedder.calls.Load() != 0 {
		t.Fatalf("future subscriber embedded before rebuild: calls=%d", embedder.calls.Load())
	}
}

func (e *projectionTestEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	e.calls.Add(1)
	if e.started != nil {
		select {
		case e.started <- struct{}{}:
		default:
		}
	}
	if e.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-e.release:
		}
	}
	if e.err != nil {
		return nil, e.err
	}
	return append([]float32(nil), e.vector...), nil
}

func TestProjectionWorkerProjectsOnlyCurrentActiveFacts(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-worker-active"
	repoRoot := "/fixtures/retrieval-worker-active"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
	active, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "retrieval-worker-active-source",
		MemoryKey:   "rollback.approvals",
		Content:     "Rollback requires two maintainers.",
		SourceRef:   "fixture:retrieval-worker-active",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID: "retrieval-worker-proposed",
		Kind:        ObservationKindAgentResult,
		Content:     "Unconfirmed rollback suggestion.",
		SourceRef:   "fixture:retrieval-worker-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.SetGlobalDefault(ctx, tenantID, "retrieval-worker-global", "reply_language", "Reply in Chinese.")
	if err != nil {
		t.Fatal(err)
	}

	embedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 32)
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 3 || result.Lag != 0 || result.Status != "idle" {
		t.Fatalf("unexpected worker result: %#v", result)
	}
	if embedder.calls.Load() != 1 {
		t.Fatalf("embedding calls=%d want 1 active fact", embedder.calls.Load())
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, true)
	assertVectorPresence(t, store, proposed.Memory.MemoryID, false)
	assertVectorPresence(t, store, global.Memory.MemoryID, false)

	replay, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Processed != 0 || replay.Lag != 0 || embedder.calls.Load() != 1 {
		t.Fatalf("worker replay was not idempotent: result=%#v calls=%d", replay, embedder.calls.Load())
	}

	revised, err := governance.ReviseSource(ctx, repoRoot, active.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "retrieval-worker-revision",
		Content:     "Rollback requires three maintainers.",
		SourceRef:   "fixture:retrieval-worker-revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
	assertVectorPresence(t, store, revised.Memory.MemoryID, true)

	if _, err := governance.Forget(ctx, repoRoot, revised.Memory.MemoryID, "retrieval-worker-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	assertVectorPresence(t, store, revised.Memory.MemoryID, false)
}

func TestProjectionWorkerRebuildCurrentCollapsesEventHistory(t *testing.T) {
	store, tenantID, _, active := seedProjectionWorkerActive(t, "retrieval-rebuild-history")
	ctx := context.Background()
	for version := 2; version <= 4; version++ {
		if _, err := store.pool.Exec(ctx, `
UPDATE governed_memories
SET content = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2::uuid`,
			tenantID, active.Memory.MemoryID, "Current projection fact version "+string(rune('0'+version))+"."); err != nil {
			t.Fatal(err)
		}
	}
	var eventCount, watermark int64
	if err := store.pool.QueryRow(ctx, `
SELECT count(*), max(event_id)
FROM memory_projection_events
WHERE tenant_id = $1`, tenantID).Scan(&eventCount, &watermark); err != nil {
		t.Fatal(err)
	}
	if eventCount != 4 {
		t.Fatalf("history event count=%d want 4", eventCount)
	}
	embedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 8)
	result, err := worker.RebuildCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 || result.Projected != 1 || result.SkippedChanged != 0 ||
		result.Watermark != watermark || result.LastEventID != watermark || result.Lag != 0 {
		t.Fatalf("unexpected current rebuild result: %#v", result)
	}
	if embedder.calls.Load() != 1 {
		t.Fatalf("current rebuild replayed history: calls=%d", embedder.calls.Load())
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, true)
}

func TestProjectionWorkerRebuildCurrentLeavesConcurrentDeleteInTail(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-rebuild-delete")
	blocking := &projectionTestEmbedder{
		vector: testVector1024(0.5), started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	worker := mustProjectionWorker(t, store, blocking, tenantID, 8)
	resultCh := make(chan ProjectionRebuildResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := worker.RebuildCurrent(context.Background())
		resultCh <- result
		errCh <- err
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("current rebuild did not reach embedding")
	}
	if _, err := NewGovernanceService(store, tenantID).Forget(
		context.Background(), repoRoot, active.Memory.MemoryID, "retrieval-rebuild-delete-op",
	); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 || result.Projected != 0 || result.SkippedChanged != 1 || result.Lag != 1 {
		t.Fatalf("concurrent delete was not left in tail: %#v", result)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)

	tail := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(0.1)}, tenantID, 8)
	tailResult, err := tail.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tailResult.Processed != 1 || tailResult.Lag != 0 {
		t.Fatalf("delete tail did not drain: %#v", tailResult)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
}

func TestProjectionWorkerRebuildCurrentFailureLeavesCursorPending(t *testing.T) {
	store, tenantID, _, active := seedProjectionWorkerActive(t, "retrieval-rebuild-failure")
	worker := mustProjectionWorker(t, store, &projectionTestEmbedder{err: errors.New("provider unavailable")}, tenantID, 8)
	result, err := worker.RebuildCurrent(context.Background())
	if err == nil || result.FailureCode != "embedding_unavailable" {
		t.Fatalf("current rebuild failure was not bounded: result=%#v err=%v", result, err)
	}
	if result.LastEventID != 0 || result.Lag != 1 || result.Status != "failed" {
		t.Fatalf("failed current rebuild advanced cursor: %#v", result)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
}

func TestProjectionWorkerRebuildCurrentRejectsWrongDimensions(t *testing.T) {
	store, tenantID, _, active := seedProjectionWorkerActive(t, "retrieval-rebuild-dimensions")
	worker := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: []float32{1, 2, 3}}, tenantID, 8)
	result, err := worker.RebuildCurrent(context.Background())
	if err == nil || result.FailureCode != "embedding_dimension_mismatch" || result.LastEventID != 0 || result.Lag != 1 {
		t.Fatalf("wrong snapshot dimensions were accepted: result=%#v err=%v", result, err)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
}

func TestProjectionWorkerRebuildCurrentSharesWorkerLock(t *testing.T) {
	store := openProjectionTestStore(t, 2)
	tenantID := "retrieval-rebuild-lock"
	seedProjectionWorkerActiveInStore(t, store, tenantID)
	blocking := &projectionTestEmbedder{
		vector: testVector1024(0.5), started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	rebuild := mustProjectionWorker(t, store, blocking, tenantID, 8)
	done := make(chan error, 1)
	go func() {
		_, err := rebuild.RebuildCurrent(context.Background())
		done <- err
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot rebuild did not acquire the projection lock")
	}
	competitor := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(0.1)}, tenantID, 8)
	result, err := competitor.RunOnce(context.Background())
	if err != nil || !result.AlreadyRunning || result.FailureCode != "already_running" {
		t.Fatalf("tail worker acquired the snapshot rebuild lock: result=%#v err=%v", result, err)
	}
	close(blocking.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProjectionWorkerFailureLeavesAuthorityAndCursorPending(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-worker-failure")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "Bearer secret-provider-detail", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	embedder, err := memorybackend.NewOpenAIEmbedder(server.URL, "test-key", "BAAI/bge-m3", 1024, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 8)
	result, err := worker.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "embedding_unavailable") || strings.Contains(err.Error(), "secret-provider-detail") {
		t.Fatalf("unexpected bounded worker error: result=%#v err=%v", result, err)
	}
	if result.FailureCode != "embedding_unavailable" || result.Processed != 0 {
		t.Fatalf("unexpected failed result: %#v", result)
	}
	status, err := store.RetrievalProjectionStatus(context.Background(), tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastEventID != 0 || status.Lag == 0 || status.Status != "failed" || status.LastErrorCode != "embedding_unavailable" {
		t.Fatalf("failed cursor advanced or lost error: %#v", status)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
	memories, err := store.SearchActiveMemory(context.Background(), tenantID, continuityID, "current projection fact", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].ID != active.Memory.MemoryID {
		t.Fatalf("provider failure changed authority/lexical state: %#v", memories)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
}

func TestProjectionWorkerRunStopsOnCancellation(t *testing.T) {
	store := openTestStore(t)
	tenantID := "retrieval-worker-cancel"
	worker, err := NewProjectionWorker(store, &projectionTestEmbedder{vector: testVector1024(0.1)}, ProjectionWorkerOptions{
		TenantID: tenantID,
		Profile: RetrievalProfile{
			ID:              ProductionRetrievalProfileID,
			BaseURL:         "https://api.siliconflow.cn/v1",
			Model:           "BAAI/bge-m3",
			Dimensions:      1024,
			ProjectionClass: ProjectionClass1024,
		},
		BatchSize:    8,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var exists bool
		if err := store.pool.QueryRow(context.Background(), `
SELECT EXISTS (
  SELECT 1 FROM memory_projection_cursors
  WHERE tenant_id = $1 AND profile_id = $2
)`, tenantID, ProductionRetrievalProfileID).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not finish its first polling pass")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("worker cancellation error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}

func TestProjectionWorkerRejectsWrongDimensions(t *testing.T) {
	store, tenantID, _, _ := seedProjectionWorkerActive(t, "retrieval-worker-dimensions")
	worker := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: []float32{1, 2, 3}}, tenantID, 8)
	result, err := worker.RunOnce(context.Background())
	if err == nil || result.FailureCode != "embedding_dimension_mismatch" {
		t.Fatalf("wrong dimensions were accepted: result=%#v err=%v", result, err)
	}
}

func TestProjectionWorkerSerializesTenantProfile(t *testing.T) {
	store := openProjectionTestStore(t, 2)
	tenantID := "retrieval-worker-lock"
	seedProjectionWorkerActiveInStore(t, store, tenantID)
	blocking := &projectionTestEmbedder{
		vector:  testVector1024(0.5),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	first := mustProjectionWorker(t, store, blocking, tenantID, 8)
	second := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(0.7)}, tenantID, 8)
	firstDone := make(chan error, 1)
	go func() {
		_, err := first.RunOnce(context.Background())
		firstDone <- err
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not reach embedding")
	}
	secondCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := second.RunOnce(secondCtx)
	if err != nil || !result.AlreadyRunning || result.FailureCode != "already_running" {
		t.Fatalf("second worker acquired the same tenant/profile: result=%#v err=%v", result, err)
	}
	close(blocking.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestProjectionWorkerLateEmbeddingCannotRestoreDeletedMemory(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-worker-late-delete")
	blocking := &projectionTestEmbedder{
		vector:  testVector1024(0.9),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	worker := mustProjectionWorker(t, store, blocking, tenantID, 8)
	resultCh := make(chan ProjectionRunResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := worker.RunOnce(context.Background())
		resultCh <- result
		errCh <- err
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach embedding")
	}
	if _, err := NewGovernanceService(store, tenantID).Forget(context.Background(), repoRoot, active.Memory.MemoryID, "retrieval-worker-late-delete-op"); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	result := <-resultCh
	err := <-errCh
	if err == nil || result.FailureCode != "authority_changed" {
		t.Fatalf("late authority change was not detected: result=%#v err=%v", result, err)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)

	retry := mustProjectionWorker(t, store, &projectionTestEmbedder{vector: testVector1024(0.1)}, tenantID, 8)
	if _, err := retry.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertVectorPresence(t, store, active.Memory.MemoryID, false)
}

func TestCandidateProfileLateEmbeddingCannotRestoreDeletedMemory(t *testing.T) {
	store, tenantID, repoRoot, active := seedProjectionWorkerActive(t, "retrieval-worker-candidate-late-delete")
	blocking := &projectionTestEmbedder{
		vector:  testVector1024(0.8),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	worker, err := NewProjectionWorker(store, blocking, ProjectionWorkerOptions{
		TenantID: tenantID,
		Profile: RetrievalProfile{
			ID:              MigrationRetrievalProfileID,
			BaseURL:         "https://api.siliconflow.cn/v1",
			Model:           "BAAI/bge-large-zh-v1.5",
			Dimensions:      1024,
			ProjectionClass: ProjectionClass1024,
		},
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
		t.Fatal("candidate worker did not reach embedding")
	}
	if _, err := NewGovernanceService(store, tenantID).Forget(context.Background(), repoRoot, active.Memory.MemoryID, "candidate-late-delete"); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	result := <-resultCh
	workerErr := <-errCh
	if workerErr == nil || result.FailureCode != "authority_changed" {
		t.Fatalf("candidate late authority change was not detected: result=%#v err=%v", result, workerErr)
	}
	assertProfileVectorPresence(t, store, MigrationRetrievalProfileID, active.Memory.MemoryID, false)

	retry, err := NewProjectionWorker(store, &projectionTestEmbedder{vector: testVector1024(0.1)}, ProjectionWorkerOptions{
		TenantID: tenantID,
		Profile: RetrievalProfile{
			ID:              MigrationRetrievalProfileID,
			BaseURL:         "https://api.siliconflow.cn/v1",
			Model:           "BAAI/bge-large-zh-v1.5",
			Dimensions:      1024,
			ProjectionClass: ProjectionClass1024,
		},
		BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retry.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertProfileVectorPresence(t, store, MigrationRetrievalProfileID, active.Memory.MemoryID, false)
}

func seedProjectionWorkerActive(t *testing.T, tenantID string) (*Store, string, string, GovernedObservationReceipt) {
	t.Helper()
	store := openTestStore(t)
	repoRoot, active := seedProjectionWorkerActiveInStore(t, store, tenantID)
	return store, tenantID, repoRoot, active
}

func seedProjectionWorkerActiveInStore(t *testing.T, store *Store, tenantID string) (string, GovernedObservationReceipt) {
	t.Helper()
	repoRoot := "/fixtures/" + tenantID
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(context.Background(), repoRoot); err != nil {
		t.Fatal(err)
	}
	active, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: tenantID + "-active",
		MemoryKey:   "projection.current",
		Content:     "Current projection fact.",
		SourceRef:   "fixture:" + tenantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return repoRoot, active
}

func openProjectionTestStore(t *testing.T, maxConns int32) *Store {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{pool: pool, databaseURL: databaseURL}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func mustProjectionWorker(t *testing.T, store *Store, embedder Embedder, tenantID string, batchSize int) *ProjectionWorker {
	t.Helper()
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID: tenantID,
		Profile: RetrievalProfile{
			ID:              ProductionRetrievalProfileID,
			BaseURL:         "https://api.siliconflow.cn/v1",
			Model:           "BAAI/bge-m3",
			Dimensions:      1024,
			ProjectionClass: ProjectionClass1024,
		},
		BatchSize:    batchSize,
		PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func assertVectorPresence(t *testing.T, store *Store, memoryID string, want bool) {
	assertProfileVectorPresence(t, store, ProductionRetrievalProfileID, memoryID, want)
}

func assertProfileVectorPresence(t *testing.T, store *Store, profileID, memoryID string, want bool) {
	t.Helper()
	var exists bool
	if err := store.pool.QueryRow(context.Background(), `
SELECT EXISTS (
  SELECT 1 FROM memory_vector_documents
  WHERE profile_id = $1 AND memory_id = $2::uuid

)`, profileID, memoryID).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("memory %s vector presence=%v want %v", memoryID, exists, want)
	}
}

func testVector1024(value float32) []float32 {
	vector := make([]float32, 1024)
	for index := range vector {
		vector[index] = value
	}
	return vector
}
