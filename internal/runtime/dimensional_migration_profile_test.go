package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestDimensionalMigrationHarnessMiniature(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	dataset := seedDimensionalFixture(t, store, "w17-mini", 2, 2, 10)
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != 40 {
		t.Fatalf("mini lexical rebuild rows=%d err=%v", rows, err)
	}
	incumbentEmbedder := &dimensionalFixtureEmbedder{dimensions: 1024}
	candidateDelegate := &dimensionalFixtureEmbedder{dimensions: 2560}
	incumbentProfile, _ := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	incumbent := RetrievalProfile{
		ID: incumbentProfile.ID, BaseURL: incumbentProfile.BaseURL, Model: incumbentProfile.Model,
		Dimensions: incumbentProfile.Dimensions, ProjectionClass: incumbentProfile.ProjectionClass,
	}
	candidate := dimensionalMigrationProfile(t)

	incumbentWorkers := make(map[string]*ProjectionWorker, len(dataset.Tenants))
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(t, store, tenantID, incumbent, incumbentEmbedder, 10, 16)
		incumbentWorkers[tenantID] = worker
		rebuild, err := worker.RebuildCurrent(ctx)
		if err != nil || rebuild.Projected != 20 || rebuild.Lag != 0 {
			t.Fatalf("mini incumbent snapshot tenant=%s result=%#v err=%v", tenantID, rebuild, err)
		}
	}

	var eventsBefore int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_projection_events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	blocking := &dimensionalBlockingEmbedder{
		delegate: candidateDelegate, started: make(chan struct{}), release: make(chan struct{}),
	}
	candidateWorkers := make(map[string]*ProjectionWorker, len(dataset.Tenants))
	type rebuildResult struct {
		tenant string
		result ProjectionRebuildResult
		err    error
	}
	rebuilds := make(chan rebuildResult, len(dataset.Tenants))
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(t, store, tenantID, candidate, blocking, 10, 16)
		candidateWorkers[tenantID] = worker
		go func(tenant string, candidateWorker *ProjectionWorker) {
			result, err := candidateWorker.RebuildCurrent(ctx)
			rebuilds <- rebuildResult{tenant: tenant, result: result, err: err}
		}(tenantID, worker)
	}
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("mini candidate snapshot did not capture a watermark and reach embedding")
	}

	writerStarted := time.Now()
	applyDimensionalFixtureTail(t, store, &dataset, 4, 2, 2)
	writerDuration := time.Since(writerStarted)
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != 40 {
		t.Fatalf("mini tail lexical rebuild rows=%d err=%v", rows, err)
	}
	var eventsAfter int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_projection_events`).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter-eventsBefore != 24 {
		t.Fatalf("mini tail events=%d want 24", eventsAfter-eventsBefore)
	}
	if writerDuration <= 0 {
		t.Fatal("mini writer duration was not measured")
	}

	queryErrors := make(chan error, 16)
	var clients sync.WaitGroup
	for client := 0; client < 4; client++ {
		client := client
		clients.Add(1)
		go func() {
			defer clients.Done()
			tenantID := dataset.Tenants[client%len(dataset.Tenants)]
			record := dataset.Records[tenantID][6]
			coordinator, err := NewRetrievalCoordinator(store, incumbentEmbedder, incumbent)
			if err != nil {
				queryErrors <- err
				return
			}
			for queryIndex := 0; queryIndex < 4; queryIndex++ {
				result, err := coordinator.Retrieve(ctx, RetrievalRequest{
					OperationID: fmt.Sprintf("w17-mini-query-%02d-%02d", client, queryIndex),
					TenantID:    tenantID, ContinuityIDs: []string{record.ContinuityID},
					Query: record.Content, Limit: 1, Mode: RetrievalVector,
				})
				if err != nil {
					queryErrors <- err
					continue
				}
				if len(result.Memories) != 1 || result.Memories[0].ID != record.MemoryID {
					queryErrors <- fmt.Errorf("client %d query %d returned %#v", client, queryIndex, result)
				}
			}
		}()
	}
	clients.Wait()
	close(queryErrors)
	for err := range queryErrors {
		if err != nil {
			t.Fatal(err)
		}
	}

	close(blocking.release)
	for range dataset.Tenants {
		rebuild := <-rebuilds
		if rebuild.err != nil {
			t.Fatalf("mini candidate snapshot tenant=%s result=%#v err=%v", rebuild.tenant, rebuild.result, rebuild.err)
		}
	}
	for _, tenantID := range dataset.Tenants {
		incumbentStatus := drainDimensionalProjection(t, incumbentWorkers[tenantID])
		candidateStatus := drainDimensionalProjection(t, candidateWorkers[tenantID])
		if incumbentStatus.Lag != 0 || candidateStatus.Lag != 0 {
			t.Fatalf("mini final lag tenant=%s incumbent=%#v candidate=%#v", tenantID, incumbentStatus, candidateStatus)
		}
		authorityIDs := governedActiveIDs(t, store, tenantID)
		incumbentIDs := dimensionalVectorIDs(t, store, tenantID, ProjectionClass1024)
		candidateIDs := dimensionalVectorIDs(t, store, tenantID, ProjectionClass2560)
		if len(authorityIDs) != 20 || !sameStringSet(authorityIDs, incumbentIDs) || !sameStringSet(authorityIDs, candidateIDs) {
			t.Fatalf("mini projection convergence tenant=%s authority/incumbent/candidate=%d/%d/%d", tenantID, len(authorityIDs), len(incumbentIDs), len(candidateIDs))
		}
	}

	resetTenant := dataset.Tenants[0]
	incumbentBefore := dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024)
	authorityBefore := governedActiveIDs(t, store, resetTenant)
	if err := store.ResetVectorProjection(ctx, resetTenant, DimensionalMigrationRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	if candidateAfterReset := dimensionalVectorIDs(t, store, resetTenant, ProjectionClass2560); len(candidateAfterReset) != 0 {
		t.Fatalf("mini candidate reset rows=%d want 0", len(candidateAfterReset))
	}
	if incumbentAfter := dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024); !sameStringSet(incumbentBefore, incumbentAfter) {
		t.Fatal("mini candidate reset changed incumbent vectors")
	}
	if authorityAfter := governedActiveIDs(t, store, resetTenant); !sameStringSet(authorityBefore, authorityAfter) {
		t.Fatal("mini candidate reset changed governed authority")
	}
	rebuild, err := candidateWorkers[resetTenant].RebuildCurrent(ctx)
	if err != nil || rebuild.Projected != 20 || rebuild.Lag != 0 {
		t.Fatalf("mini candidate rebuild after reset=%#v err=%v", rebuild, err)
	}
	if candidateAfterRebuild := dimensionalVectorIDs(t, store, resetTenant, ProjectionClass2560); !sameStringSet(authorityBefore, candidateAfterRebuild) {
		t.Fatal("mini candidate rebuild did not restore authority equivalence")
	}
}
