package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestOperationalScaleProfile(t *testing.T) {
	if os.Getenv("VERMORY_SCALE_PROFILE") != "1" {
		t.Skip("VERMORY_SCALE_PROFILE=1 is required")
	}
	databaseURL := os.Getenv("VERMORY_SCALE_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("VERMORY_TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("VERMORY_SCALE_DATABASE_URL or VERMORY_TEST_DATABASE_URL is required")
	}

	ctx := context.Background()
	store, err := OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}

	const (
		tenantID         = "scale-profile-tenant"
		repoRoot         = "/fixtures/scale-profile/workspace"
		recordCount      = 10000
		deleteCount      = 50
		readerCount      = 8
		queriesPerReader = 25
	)
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	seedStarted := time.Now()
	deleteIDs := make([]string, 0, deleteCount)
	for batchStart := 0; batchStart < recordCount; batchStart += 500 {
		batchEnd := batchStart + 500
		if batchEnd > recordCount {
			batchEnd = recordCount
		}
		tenantCtx, err := withTenantContext(ctx, tenantID)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.pool.Begin(tenantCtx)
		if err != nil {
			t.Fatal(err)
		}
		for index := batchStart; index < batchEnd; index++ {
			request := CommitObservationRequest{
				OperationID: fmt.Sprintf("scale-source-%05d", index),
				Kind:        ObservationKindSourceUpdate,
				Content: fmt.Sprintf(
					"Service atlas-%04d uses endpoint /v1/items/%04d; retry budget is %d ms; flag scale_route_%02d is active; 中文说明第 %04d 条。",
					index%1000, index, 300+index%7*100, index%11, index,
				),
				SourceRef: fmt.Sprintf("fixture:scale-profile:%05d", index),
				MemoryKey: fmt.Sprintf("scale.record.%05d", index),
			}
			observation, err := commitObservationTx(tenantCtx, tx, tenantID, continuityID, request)
			if err != nil {
				tx.Rollback(tenantCtx)
				t.Fatal(err)
			}
			memory, err := governObservationTx(tenantCtx, tx, tenantID, continuityID, observation.ObservationID, request)
			if err != nil {
				tx.Rollback(tenantCtx)
				t.Fatal(err)
			}
			if index < deleteCount {
				deleteIDs = append(deleteIDs, memory.MemoryID)
			}
		}
		if err := tx.Commit(tenantCtx); err != nil {
			t.Fatal(err)
		}
	}

	var activeCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND lifecycle_status = 'active'`, tenantID, continuityID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != recordCount {
		t.Fatalf("active authority count=%d, want %d", activeCount, recordCount)
	}

	latencies := make(chan time.Duration, readerCount*queriesPerReader)
	errors := make(chan error, readerCount*queriesPerReader)
	var readers sync.WaitGroup
	for reader := 0; reader < readerCount; reader++ {
		reader := reader
		readers.Add(1)
		go func() {
			defer readers.Done()
			for queryIndex := 0; queryIndex < queriesPerReader; queryIndex++ {
				recordIndex := (reader*queriesPerReader + queryIndex) % recordCount
				started := time.Now()
				memories, searchErr := store.SearchActiveMemory(ctx, tenantID, continuityID, fmt.Sprintf("atlas-%04d", recordIndex%1000), 12)
				latencies <- time.Since(started)
				if searchErr != nil {
					errors <- searchErr
					continue
				}
				if len(memories) == 0 {
					errors <- fmt.Errorf("empty result for scale query %d", recordIndex)
				}
			}
		}()
	}

	var deleter sync.WaitGroup
	deleter.Add(1)
	go func() {
		defer deleter.Done()
		for _, memoryID := range deleteIDs {
			if deleteErr := store.DeleteMemory(ctx, tenantID, continuityID, memoryID); deleteErr != nil {
				errors <- deleteErr
			}
		}
	}()
	readers.Wait()
	deleter.Wait()
	close(latencies)
	close(errors)
	for searchErr := range errors {
		t.Fatal(searchErr)
	}

	var remainingDeleted int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND lifecycle_status = 'deleted'`, tenantID, continuityID).Scan(&remainingDeleted); err != nil {
		t.Fatal(err)
	}
	if remainingDeleted != deleteCount {
		t.Fatalf("deleted authority count=%d, want %d", remainingDeleted, deleteCount)
	}
	for _, memoryID := range deleteIDs[:3] {
		memories, err := store.SearchActiveMemory(ctx, tenantID, continuityID, memoryID, 12)
		if err != nil {
			t.Fatal(err)
		}
		for _, memory := range memories {
			if memory.ID == memoryID {
				t.Fatalf("deleted memory %s remained searchable", memoryID)
			}
		}
	}

	first, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var backendPID int
	if err := first.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&backendPID); err != nil {
		first.Release()
		t.Fatal(err)
	}
	second, err := store.pool.Acquire(ctx)
	if err != nil {
		first.Release()
		t.Fatal(err)
	}
	if _, err := second.Exec(ctx, "SELECT pg_terminate_backend($1)", backendPID); err != nil {
		second.Release()
		first.Release()
		t.Fatal(err)
	}
	second.Release()
	first.Release()
	var pingErr error
	for attempt := 0; attempt < 10; attempt++ {
		pingErr = store.pool.Ping(ctx)
		if pingErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("pool did not recover after terminating one backend connection: %v", pingErr)
	}
	if _, err := store.SearchActiveMemory(ctx, tenantID, continuityID, "scale_route_01", 12); err != nil {
		t.Fatalf("search after connection recovery failed: %v", err)
	}

	durations := make([]time.Duration, 0, cap(latencies))
	for latency := range latencies {
		durations = append(durations, latency)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	if len(durations) != readerCount*queriesPerReader {
		t.Fatalf("latency sample count=%d, want %d", len(durations), readerCount*queriesPerReader)
	}
	p50 := durations[(len(durations)-1)*50/100]
	p95 := durations[(len(durations)-1)*95/100]
	payload, _ := json.Marshal(map[string]any{
		"records":                        recordCount,
		"active_after_seed":              activeCount,
		"deleted_after_concurrent_write": remainingDeleted,
		"readers":                        readerCount,
		"queries_per_reader":             queriesPerReader,
		"seed_duration_ms":               time.Since(seedStarted).Milliseconds(),
		"search_p50_ms":                  p50.Microseconds() / 1000.0,
		"search_p95_ms":                  p95.Microseconds() / 1000.0,
		"connection_recovery":            true,
	})
	t.Logf("operational scale evidence=%s", payload)
}
