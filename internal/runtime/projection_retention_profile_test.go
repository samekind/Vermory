package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProjectionRetentionMiniatureProfile(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	dataset := seedDimensionalFixture(t, store, "w18-mini", 2, 2, 20)
	incumbent := projectionRetentionProfile(t, ProductionRetrievalProfileID)
	dimensional := projectionRetentionProfile(t, DimensionalMigrationRetrievalProfileID)
	incumbentWorkers := map[string]*ProjectionWorker{}
	dimensionalWorkers := map[string]*ProjectionWorker{}
	frozenCandidateCursor := map[string]int64{}
	for _, tenantID := range dataset.Tenants {
		incumbentWorkers[tenantID] = newDimensionalWorker(t, store, tenantID, incumbent, &dimensionalFixtureEmbedder{dimensions: 1024}, 32, 64)
		dimensionalWorkers[tenantID] = newDimensionalWorker(t, store, tenantID, dimensional, &dimensionalFixtureEmbedder{dimensions: 2560}, 32, 64)
		if result, err := incumbentWorkers[tenantID].RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("mini incumbent rebuild %s: result=%#v err=%v", tenantID, result, err)
		}
		if result, err := dimensionalWorkers[tenantID].RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("mini dimensional rebuild %s: result=%#v err=%v", tenantID, result, err)
		}
		status, err := store.RetrievalProjectionStatus(ctx, tenantID, DimensionalMigrationRetrievalProfileID)
		if err != nil {
			t.Fatal(err)
		}
		frozenCandidateCursor[tenantID] = status.LastEventID
	}
	cutoff := time.Now().UTC().Add(time.Hour)
	for epoch := 1; epoch <= 3; epoch++ {
		applyProjectionRetentionEpoch(t, store, &dataset, epoch, 5, 2, 2)
		for _, tenantID := range dataset.Tenants {
			drainDimensionalProjection(t, incumbentWorkers[tenantID])
			receipt, err := store.PruneProjectionEvents(ctx, tenantID, ProjectionPruneRequest{
				OperationID: "w18-mini-blocked-" + tenantID + "-" + string(rune('0'+epoch)),
				Cutoff:      cutoff, RetainTailEvents: 0,
			})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.NewFloorEventID > frozenCandidateCursor[tenantID] {
				t.Fatalf("mini prune passed slow candidate: tenant=%s receipt=%#v frozen=%d", tenantID, receipt, frozenCandidateCursor[tenantID])
			}
		}
	}
	if err := drainProjectionRetentionWorkers(ctx, dataset.Tenants, dimensionalWorkers); err != nil {
		t.Fatal(err)
	}
	for _, tenantID := range dataset.Tenants {
		receipt, err := store.PruneProjectionEvents(ctx, tenantID, ProjectionPruneRequest{
			OperationID: "w18-mini-final-" + tenantID, Cutoff: cutoff, RetainTailEvents: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Result != "pruned" || projectionRetentionEventCount(t, store, tenantID) != 10 {
			t.Fatalf("mini retention did not converge: tenant=%s receipt=%#v retained=%d", tenantID, receipt, projectionRetentionEventCount(t, store, tenantID))
		}
		if !projectionRetentionAuthorityEquivalent(t, store, tenantID, ProjectionClass1024) ||
			!projectionRetentionAuthorityEquivalent(t, store, tenantID, ProjectionClass2560) {
			t.Fatalf("mini projection diverged from authority for %s", tenantID)
		}
	}

	futureTenant := dataset.Tenants[0]
	futureEmbedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	futureWorker, err := NewProjectionWorker(store, futureEmbedder, ProjectionWorkerOptions{
		TenantID: futureTenant, Profile: projectionRetentionProfile(t, MigrationRetrievalProfileID), BatchSize: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	futureResult, err := futureWorker.RunOnce(ctx)
	if err == nil || futureResult.FailureCode != ProjectionFailureRebuildRequired || futureEmbedder.calls.Load() != 0 {
		t.Fatalf("mini future subscriber skipped rebuild: result=%#v calls=%d err=%v", futureResult, futureEmbedder.calls.Load(), err)
	}
	if rebuilt, err := futureWorker.RebuildCurrent(ctx); err != nil || rebuilt.Lag != 0 {
		t.Fatalf("mini future subscriber rebuild: result=%#v err=%v", rebuilt, err)
	}

	resetTenant := dataset.Tenants[1]
	beforeIncumbent := dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024)
	if err := store.ResetVectorProjection(ctx, resetTenant, DimensionalMigrationRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	status, err := store.RetrievalProjectionStatus(ctx, resetTenant, DimensionalMigrationRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RebuildRequired || status.Status != ProjectionStatusRebuildRequired || status.VectorCount != 0 {
		t.Fatalf("mini reset did not require rebuild: %#v", status)
	}
	if rebuilt, err := dimensionalWorkers[resetTenant].RebuildCurrent(ctx); err != nil || rebuilt.Lag != 0 {
		t.Fatalf("mini reset rebuild: result=%#v err=%v", rebuilt, err)
	}
	if !sameStringSet(beforeIncumbent, dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024)) ||
		!projectionRetentionAuthorityEquivalent(t, store, resetTenant, ProjectionClass2560) {
		t.Fatal("mini reset changed another profile or rebuilt the wrong authority set")
	}

	deleted := dataset.Records[futureTenant][0]
	if _, err := NewGovernanceService(store, futureTenant).Forget(ctx, deleted.RepoRoot, deleted.MemoryID, "w18-mini-delete-after-prune"); err != nil {
		t.Fatal(err)
	}
	drainDimensionalProjection(t, incumbentWorkers[futureTenant])
	drainDimensionalProjection(t, dimensionalWorkers[futureTenant])
	drainDimensionalProjection(t, futureWorker)
	if containsString(dimensionalVectorIDs(t, store, futureTenant, ProjectionClass1024), deleted.MemoryID) ||
		containsString(dimensionalVectorIDs(t, store, futureTenant, ProjectionClass2560), deleted.MemoryID) ||
		containsString(projectionRetentionProfileVectorIDs(t, store, futureTenant, MigrationRetrievalProfileID), deleted.MemoryID) {
		t.Fatal("mini deleted memory reappeared in a projection")
	}
	if memories, err := store.SearchActiveMemory(ctx, futureTenant, deleted.ContinuityID, deleted.Content, 5); err != nil {
		t.Fatal(err)
	} else {
		for _, memory := range memories {
			if memory.ID == deleted.MemoryID || strings.Contains(memory.Content, deleted.Content) {
				t.Fatalf("mini deleted memory remained in lexical retrieval: %#v", memories)
			}
		}
	}
}
