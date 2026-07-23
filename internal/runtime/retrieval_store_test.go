package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProductionRetrievalProfileIsFrozen(t *testing.T) {
	valid := RetrievalProfile{
		ID:              ProductionRetrievalProfileID,
		BaseURL:         "https://api.siliconflow.cn/v1",
		Model:           "BAAI/bge-m3",
		Dimensions:      1024,
		ProjectionClass: ProjectionClass1024,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RetrievalProfile){
		"profile":     func(profile *RetrievalProfile) { profile.ID = "other" },
		"base URL":    func(profile *RetrievalProfile) { profile.BaseURL = "" },
		"credentials": func(profile *RetrievalProfile) { profile.BaseURL = "https://user:secret@example.com/v1" },
		"model":       func(profile *RetrievalProfile) { profile.Model = "other" },
		"dimensions":  func(profile *RetrievalProfile) { profile.Dimensions = 768 },
		"class":       func(profile *RetrievalProfile) { profile.ProjectionClass = ProjectionClass2560 },
	} {
		t.Run(name, func(t *testing.T) {
			profile := valid
			mutate(&profile)
			if err := profile.Validate(); err == nil {
				t.Fatalf("invalid profile was accepted: %#v", profile)
			}
		})
	}
}

func TestRetrievalProjectionStatusCountsTenantEventsAcrossGlobalIDGaps(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	governanceA := NewGovernanceService(store, "retrieval-lag-tenant-a")
	governanceB := NewGovernanceService(store, "retrieval-lag-tenant-b")
	const repoA = "/fixtures/retrieval-lag/a"
	const repoB = "/fixtures/retrieval-lag/b"
	if _, err := governanceA.ConfirmWorkspace(ctx, repoA); err != nil {
		t.Fatal(err)
	}
	if _, err := governanceB.ConfirmWorkspace(ctx, repoB); err != nil {
		t.Fatal(err)
	}
	add := func(governance *GovernanceService, repoRoot, operationID, memoryKey string) {
		t.Helper()
		if _, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
			OperationID: operationID,
			MemoryKey:   memoryKey,
			Content:     "Projection lag fixture " + operationID + ".",
			SourceRef:   "fixture:" + operationID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add(governanceA, repoA, "lag-a-1", "lag.a.1")
	add(governanceB, repoB, "lag-b-1", "lag.b.1")
	add(governanceB, repoB, "lag-b-2", "lag.b.2")
	add(governanceA, repoA, "lag-a-2", "lag.a.2")

	var firstA, latestA int64
	if err := store.pool.QueryRow(ctx, `
SELECT min(event_id), max(event_id)
FROM memory_projection_events
WHERE tenant_id = 'retrieval-lag-tenant-a'`).Scan(&firstA, &latestA); err != nil {
		t.Fatal(err)
	}
	if latestA-firstA <= 1 {
		t.Fatalf("fixture did not create global event ID gaps: first=%d latest=%d", firstA, latestA)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, $3, 'idle')`, "retrieval-lag-tenant-a", ProductionRetrievalProfileID, firstA); err != nil {
		t.Fatal(err)
	}
	status, err := store.RetrievalProjectionStatus(ctx, "retrieval-lag-tenant-a", ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestEventID != latestA || status.Lag != 1 {
		t.Fatalf("tenant lag counted global ID gaps: %#v", status)
	}
}

func TestResetVectorProjectionRejectsRunningWorker(t *testing.T) {
	store, tenantID, _, _ := seedProjectionWorkerActive(t, "retrieval-reset-lock")
	blocking := &projectionTestEmbedder{
		vector:  testVector1024(0.5),
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	worker := mustProjectionWorker(t, store, blocking, tenantID, 8)
	done := make(chan error, 1)
	go func() {
		_, err := worker.RunOnce(context.Background())
		done <- err
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not acquire the projection lock")
	}
	err := store.ResetVectorProjection(context.Background(), tenantID, ProductionRetrievalProfileID)
	close(blocking.release)
	workerErr := <-done
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("projection reset raced a running worker: %v", err)
	}
	if workerErr != nil {
		t.Fatal(workerErr)
	}
}

func TestResetVectorProjectionLeavesAuthorityAndLexicalState(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-reset"
	repoRoot := "/fixtures/retrieval-reset"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	active, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "retrieval-reset-active",
		MemoryKey:   "release.command",
		Content:     "Run release-safe --locked.",
		SourceRef:   "fixture:retrieval-reset",
	})
	if err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_vector_documents (
  profile_id, tenant_id, continuity_id, memory_id, content_sha256, embedding
) VALUES (
  $1, $2, $3::uuid, $4::uuid, repeat('a', 64), array_fill(0::real, ARRAY[1024])::vector
)`, ProductionRetrievalProfileID, tenantID, continuityID, active.Memory.MemoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, 99, 'idle')`, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}

	if err := store.ResetVectorProjection(ctx, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	status, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastEventID != 0 || status.VectorCount != 0 ||
		status.Status != ProjectionStatusRebuildRequired || !status.RebuildRequired ||
		status.LastErrorCode != ProjectionFailureRebuildRequired {
		t.Fatalf("unexpected reset status: %#v", status)
	}
	memories, err := store.SearchActiveMemory(ctx, tenantID, continuityID, "release-safe --locked", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].ID != active.Memory.MemoryID {
		t.Fatalf("reset changed authority or lexical state: %#v", memories)
	}
}

func TestResetVectorProjectionAfterRetentionRequiresRebuildAndIsolatesProfiles(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-reset-retention"
	repoRoot := "/fixtures/retrieval-reset-retention"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	active, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "retrieval-reset-retention-active",
		MemoryKey:   "release.command", Content: "Run release-safe --locked.",
		SourceRef: "fixture:retrieval-reset-retention",
	})
	if err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
	floor := latestProjectionEventID(t, store, tenantID)
	setProjectionRetentionFloor(t, store, tenantID, floor)
	if _, err := store.pool.Exec(ctx, `DELETE FROM memory_projection_events WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	for _, profileID := range []string{ProductionRetrievalProfileID, MigrationRetrievalProfileID} {
		if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_vector_documents (
  profile_id, tenant_id, continuity_id, memory_id, content_sha256, embedding
)
SELECT $1, $2, $3::uuid, $4::uuid,
       encode(digest(convert_to(content, 'UTF8'), 'sha256'), 'hex'),
       array_fill(0::real, ARRAY[1024])::vector
FROM governed_memories
WHERE tenant_id = $2 AND id = $4::uuid`, profileID, tenantID, continuityID, active.Memory.MemoryID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, $3, 'idle')`, tenantID, profileID, floor); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ResetVectorProjection(ctx, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	status, err := store.RetrievalProjectionStatus(ctx, tenantID, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastEventID != floor || status.Status != ProjectionStatusRebuildRequired ||
		status.LastErrorCode != ProjectionFailureRebuildRequired || status.VectorCount != 0 {
		t.Fatalf("reset did not require rebuild at floor: %#v", status)
	}
	candidate, err := store.RetrievalProjectionStatus(ctx, tenantID, MigrationRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.LastEventID != floor || candidate.Status != "idle" || candidate.VectorCount != 1 {
		t.Fatalf("reset changed candidate profile: %#v", candidate)
	}
	memories, err := store.SearchActiveMemory(ctx, tenantID, continuityID, "release-safe --locked", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].ID != active.Memory.MemoryID {
		t.Fatalf("reset changed authority or lexical state: %#v", memories)
	}
}
