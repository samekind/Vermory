package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vermory/internal/memorybackend"
)

type serverScaleCase struct {
	Version               string                      `json:"version"`
	ID                    string                      `json:"id"`
	ProfileName           string                      `json:"profile_name"`
	TenantCount           int                         `json:"tenant_count"`
	ContinuitiesPerTenant int                         `json:"continuities_per_tenant"`
	RecordsPerContinuity  int                         `json:"records_per_continuity"`
	ActiveMemoryCount     int                         `json:"active_memory_count"`
	GovernedMemoryCount   int                         `json:"governed_memory_count"`
	RevisionCount         int                         `json:"revision_count"`
	FullRevisionRounds    int                         `json:"full_revision_rounds"`
	PartialRevisionCount  int                         `json:"partial_revision_count"`
	ProjectionEventCount  int                         `json:"projection_event_count"`
	QueryClientCount      int                         `json:"query_client_count"`
	QueriesPerClient      int                         `json:"queries_per_client"`
	DeleteCount           int                         `json:"delete_count"`
	SnapshotPageSize      int                         `json:"snapshot_page_size"`
	TailWorkersPerTenant  int                         `json:"tail_worker_competitors_per_tenant"`
	PoolMaxConnections    int                         `json:"pool_max_connections"`
	ProfileID             string                      `json:"profile_id"`
	ReferenceHardware     serverScaleHardware         `json:"reference_hardware"`
	CalibratedLimits      serverScaleCalibratedLimits `json:"calibrated_limits"`
	HardGates             []string                    `json:"hard_gates"`
}

type serverScaleHardware struct {
	OS         string `json:"os"`
	CPU        string `json:"cpu"`
	MemoryGiB  int    `json:"memory_gib"`
	Storage    string `json:"storage"`
	PostgreSQL string `json:"postgresql"`
	PGVector   string `json:"pgvector"`
}

type serverScaleCalibratedLimits struct {
	AuthoritySeedSeconds     int `json:"authority_seed_seconds"`
	HistoryGenerationSeconds int `json:"history_generation_seconds"`
	LexicalRebuildSeconds    int `json:"lexical_rebuild_seconds"`
	VectorSnapshotSeconds    int `json:"vector_snapshot_seconds"`
	TailCatchupSeconds       int `json:"tail_catchup_seconds"`
	QueryP95MS               int `json:"query_p95_ms"`
	QueryP99MS               int `json:"query_p99_ms"`
	DatabaseSizeGiB          int `json:"database_size_gib"`
}

type serverScaleDataset struct {
	Tenants      []string
	Continuities map[string][]string
	Records      map[string]serverScaleRecord
	RecordsByID  map[string]serverScaleRecord
}

type serverScaleRecord struct {
	TenantID     string
	ContinuityID string
	MemoryID     string
	MemoryKey    string
	Marker       string
}

type serverScaleQueryMeasurement struct {
	Latency   time.Duration
	Mode      RetrievalMode
	Effective RetrievalMode
	Degraded  bool
}

type serverScaleRetrievalAuditCounts struct {
	RequestedVector int
	EffectiveVector int
	ProjectionLag   int
	OtherDegraded   int
}

func TestServerScaleCaseIsFrozen(t *testing.T) {
	manifest := loadServerScaleCase(t)
	if manifest.Version != "2" || manifest.ID != "W12-server-qualification-scale-profile" ||
		manifest.ProfileName != "server-qualification-v1" {
		t.Fatalf("unexpected W12 identity: %#v", manifest)
	}
	if manifest.TenantCount*manifest.ContinuitiesPerTenant*manifest.RecordsPerContinuity != manifest.ActiveMemoryCount {
		t.Fatalf("W12 active-memory arithmetic drifted: %#v", manifest)
	}
	if manifest.ActiveMemoryCount+manifest.RevisionCount != manifest.GovernedMemoryCount ||
		manifest.ActiveMemoryCount+manifest.RevisionCount*2 != manifest.ProjectionEventCount {
		t.Fatalf("W12 authority/event arithmetic drifted: %#v", manifest)
	}
	if manifest.FullRevisionRounds*manifest.ActiveMemoryCount+manifest.PartialRevisionCount != manifest.RevisionCount {
		t.Fatalf("W12 revision-round arithmetic drifted: %#v", manifest)
	}
	if manifest.QueryClientCount*manifest.QueriesPerClient != 1000 || manifest.ProfileID != ProductionRetrievalProfileID {
		t.Fatalf("W12 client/profile contract drifted: %#v", manifest)
	}
	if manifest.DeleteCount != 1000 || manifest.TailWorkersPerTenant != 2 ||
		manifest.PoolMaxConnections != 64 || len(manifest.HardGates) != 10 {
		t.Fatalf("W12 hard-gate contract drifted: %#v", manifest)
	}
}

func TestServerScaleHarnessMiniature(t *testing.T) {
	manifest := serverScaleCase{
		TenantCount: 2, ContinuitiesPerTenant: 2, RecordsPerContinuity: 5,
		ActiveMemoryCount: 20, GovernedMemoryCount: 110, RevisionCount: 90,
		FullRevisionRounds: 4, PartialRevisionCount: 10, ProjectionEventCount: 200,
		QueryClientCount: 2, QueriesPerClient: 4, DeleteCount: 2,
		SnapshotPageSize: 3, TailWorkersPerTenant: 2, ProfileID: ProductionRetrievalProfileID,
	}
	store := openTestStore(t)
	dataset := prepareServerScaleContinuities(t, store, manifest)
	seedServerScaleAuthority(t, store, manifest, dataset)
	generateServerScaleHistory(t, store, manifest, dataset)
	assertServerScaleAuthorityCounts(t, store, manifest, 0)
	assertServerScaleEventCount(t, store, int64(manifest.ProjectionEventCount))
	assertInitialTenantLag(t, store, manifest, dataset)
	if rows, err := store.RebuildAllProjections(context.Background()); err != nil || rows != int64(manifest.ActiveMemoryCount) {
		t.Fatalf("miniature lexical rebuild rows=%d err=%v", rows, err)
	}
	dataset.Records, dataset.RecordsByID = loadCurrentServerScaleRecords(t, store, manifest)
	embedder := &serverScaleEmbedder{}
	profile := productionRetrievalProfile(t)
	rebuildServerScaleVectors(t, store, manifest, dataset, profile, embedder)
	assertServerScaleProjectionCounts(t, store, manifest.ActiveMemoryCount, manifest.ActiveMemoryCount)
	coordinators := newServerScaleCoordinators(t, store, dataset, profile, embedder)
	measurements := make(chan serverScaleQueryMeasurement, manifest.QueryClientCount*manifest.QueriesPerClient)
	errorsCh := make(chan error, 32)
	runServerScaleQueryPhase(
		context.Background(), manifest, dataset, coordinators, 0, manifest.QueriesPerClient, measurements, errorsCh,
	)
	for tenantIndex := 0; tenantIndex < manifest.TenantCount; tenantIndex++ {
		deleteServerScaleTenantRecords(context.Background(), manifest, store, dataset, tenantIndex, errorsCh)
	}
	workerResults := drainServerScaleTail(t, store, manifest, dataset, embedder)
	if workerResults["processed"] != manifest.DeleteCount {
		t.Fatalf("miniature tail processed=%d want %d", workerResults["processed"], manifest.DeleteCount)
	}
	close(measurements)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	latencies, _, effectiveVector := collectServerScaleMeasurements(measurements)
	if len(latencies) != manifest.QueryClientCount*manifest.QueriesPerClient || effectiveVector != len(latencies) {
		t.Fatalf("miniature query coverage samples/vector=%d/%d", len(latencies), effectiveVector)
	}
	assertServerScaleAuthorityCounts(t, store, manifest, manifest.DeleteCount)
	assertServerScaleEventCount(t, store, int64(manifest.ProjectionEventCount+manifest.DeleteCount))
	assertServerScaleProjectionCounts(
		t, store, manifest.ActiveMemoryCount-manifest.DeleteCount, manifest.ActiveMemoryCount-manifest.DeleteCount,
	)
	assertDeletedServerScaleRecordsAbsent(t, store, manifest, dataset)
	assertServerScaleScopes(t, store, manifest, dataset, coordinators)
	assertAllServerScaleTenantsCurrent(t, store, manifest, dataset)
}

func TestServerQualificationScaleProfile(t *testing.T) {
	if os.Getenv("VERMORY_SERVER_SCALE_PROFILE") != "1" {
		t.Skip("VERMORY_SERVER_SCALE_PROFILE=1 is required")
	}
	apiKey := strings.TrimSpace(os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY"))
	if apiKey == "" {
		t.Skip("VERMORY_LIVE_EMBEDDING_API_KEY is required")
	}
	manifest := loadServerScaleCase(t)
	cluster := startDisposablePostgres18(t)
	defer cluster.stop(t, "fast")

	ctx := context.Background()
	store, err := OpenStore(ctx, fmt.Sprintf("%s&pool_max_conns=%d", cluster.databaseURL, manifest.PoolMaxConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profile := productionRetrievalProfile(t)
	dataset := prepareServerScaleContinuities(t, store, manifest)

	seedStarted := time.Now()
	seedServerScaleAuthority(t, store, manifest, dataset)
	seedDuration := time.Since(seedStarted)
	assertDurationWithin(t, "authority seed", seedDuration, manifest.CalibratedLimits.AuthoritySeedSeconds)
	t.Logf("W12 authority seed complete: records=%d duration=%s", manifest.ActiveMemoryCount, seedDuration)

	historyStarted := time.Now()
	generateServerScaleHistory(t, store, manifest, dataset)
	historyDuration := time.Since(historyStarted)
	assertDurationWithin(t, "history generation", historyDuration, manifest.CalibratedLimits.HistoryGenerationSeconds)
	assertServerScaleAuthorityCounts(t, store, manifest, 0)
	assertServerScaleEventCount(t, store, int64(manifest.ProjectionEventCount))
	assertInitialTenantLag(t, store, manifest, dataset)
	t.Logf("W12 governed history complete: revisions=%d events=%d duration=%s", manifest.RevisionCount, manifest.ProjectionEventCount, historyDuration)

	lexicalStarted := time.Now()
	lexicalRows, err := store.RebuildAllProjections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lexicalDuration := time.Since(lexicalStarted)
	if lexicalRows != int64(manifest.ActiveMemoryCount) {
		t.Fatalf("lexical rebuild rows=%d want %d", lexicalRows, manifest.ActiveMemoryCount)
	}
	assertDurationWithin(t, "lexical rebuild", lexicalDuration, manifest.CalibratedLimits.LexicalRebuildSeconds)
	dataset.Records, dataset.RecordsByID = loadCurrentServerScaleRecords(t, store, manifest)
	t.Logf("W12 lexical rebuild complete: rows=%d duration=%s", lexicalRows, lexicalDuration)

	scaleEmbedder := &serverScaleEmbedder{}
	vectorStarted := time.Now()
	rebuildResults := rebuildServerScaleVectors(t, store, manifest, dataset, profile, scaleEmbedder)
	vectorDuration := time.Since(vectorStarted)
	assertDurationWithin(t, "vector snapshot", vectorDuration, manifest.CalibratedLimits.VectorSnapshotSeconds)
	if scaleEmbedder.calls.Load() != int64(manifest.ActiveMemoryCount) {
		t.Fatalf("snapshot embedding calls=%d want %d", scaleEmbedder.calls.Load(), manifest.ActiveMemoryCount)
	}
	for _, result := range rebuildResults {
		if result.Scanned != manifest.ActiveMemoryCount/manifest.TenantCount ||
			result.Projected != result.Scanned || result.SkippedChanged != 0 || result.Lag != 0 {
			t.Fatalf("unexpected tenant snapshot result: %#v", result)
		}
	}
	assertServerScaleProjectionCounts(t, store, manifest.ActiveMemoryCount, manifest.ActiveMemoryCount)
	t.Logf("W12 vector snapshot complete: vectors=%d duration=%s", manifest.ActiveMemoryCount, vectorDuration)

	coordinators := newServerScaleCoordinators(t, store, dataset, profile, scaleEmbedder)
	measurements := make(chan serverScaleQueryMeasurement, manifest.QueryClientCount*manifest.QueriesPerClient)
	queryErrors := make(chan error, manifest.QueryClientCount*manifest.QueriesPerClient+manifest.DeleteCount)
	runServerScaleQueryPhase(ctx, manifest, dataset, coordinators, 0, 1, measurements, queryErrors)

	deleteStarted := time.Now()
	var concurrentQueries sync.WaitGroup
	queryStart := make(chan struct{})
	for client := 0; client < manifest.QueryClientCount; client++ {
		client := client
		concurrentQueries.Add(1)
		go func() {
			defer concurrentQueries.Done()
			<-queryStart
			for queryIndex := 1; queryIndex < manifest.QueriesPerClient-1; queryIndex++ {
				mode := RetrievalLexical
				if queryIndex%2 == 0 {
					mode = RetrievalVector
				}
				measurement, err := executeServerScaleQuery(
					ctx, manifest, dataset, coordinators, client, queryIndex, mode,
				)
				if err != nil {
					queryErrors <- err
					continue
				}
				measurements <- measurement
			}
		}()
	}
	var deleters sync.WaitGroup
	for tenantIndex := 0; tenantIndex < manifest.TenantCount; tenantIndex++ {
		tenantIndex := tenantIndex
		deleters.Add(1)
		go func() {
			defer deleters.Done()
			<-queryStart
			deleteServerScaleTenantRecords(ctx, manifest, store, dataset, tenantIndex, queryErrors)
		}()
	}
	close(queryStart)
	deleters.Wait()
	tailStarted := time.Now()
	competingWorkerResults := drainServerScaleTail(t, store, manifest, dataset, scaleEmbedder)
	if competingWorkerResults["processed"] != manifest.DeleteCount {
		t.Fatalf("tail processed=%d want %d", competingWorkerResults["processed"], manifest.DeleteCount)
	}
	tailDuration := time.Since(tailStarted)
	assertDurationWithin(t, "tail catchup", tailDuration, manifest.CalibratedLimits.TailCatchupSeconds)
	concurrentQueries.Wait()
	runServerScaleQueryPhase(
		ctx, manifest, dataset, coordinators, manifest.QueriesPerClient-1, manifest.QueriesPerClient,
		measurements, queryErrors,
	)
	close(measurements)
	close(queryErrors)
	for queryErr := range queryErrors {
		t.Fatal(queryErr)
	}
	deleteDuration := time.Since(deleteStarted)

	latencies, degradedCount, effectiveVectorCount := collectServerScaleMeasurements(measurements)
	if len(latencies) != manifest.QueryClientCount*manifest.QueriesPerClient {
		t.Fatalf("query samples=%d want %d", len(latencies), manifest.QueryClientCount*manifest.QueriesPerClient)
	}
	p50 := percentileDuration(latencies, 50)
	p95 := percentileDuration(latencies, 95)
	p99 := percentileDuration(latencies, 99)
	if p95 > time.Duration(manifest.CalibratedLimits.QueryP95MS)*time.Millisecond ||
		p99 > time.Duration(manifest.CalibratedLimits.QueryP99MS)*time.Millisecond {
		t.Fatalf("query latency exceeded profile: p95=%s p99=%s", p95, p99)
	}
	if effectiveVectorCount < manifest.QueryClientCount*2 {
		t.Fatalf("post-snapshot vector coverage=%d want at least %d", effectiveVectorCount, manifest.QueryClientCount*2)
	}
	auditCounts := loadServerScaleRetrievalAuditCounts(t, store, dataset)
	expectedVectorRequests := manifest.QueryClientCount * (2 + (manifest.QueriesPerClient-2)/2)
	if auditCounts.RequestedVector != expectedVectorRequests ||
		auditCounts.EffectiveVector != effectiveVectorCount ||
		auditCounts.ProjectionLag != degradedCount || auditCounts.OtherDegraded != 0 {
		t.Fatalf(
			"retrieval audit requested/effective/projection_lag/other=%d/%d/%d/%d want %d/%d/%d/0",
			auditCounts.RequestedVector, auditCounts.EffectiveVector, auditCounts.ProjectionLag,
			auditCounts.OtherDegraded, expectedVectorRequests, effectiveVectorCount, degradedCount,
		)
	}

	assertServerScaleAuthorityCounts(t, store, manifest, manifest.DeleteCount)
	assertServerScaleEventCount(t, store, int64(manifest.ProjectionEventCount+manifest.DeleteCount))
	assertServerScaleProjectionCounts(
		t, store, manifest.ActiveMemoryCount-manifest.DeleteCount, manifest.ActiveMemoryCount-manifest.DeleteCount,
	)
	assertDeletedServerScaleRecordsAbsent(t, store, manifest, dataset)
	assertServerScaleScopes(t, store, manifest, dataset, coordinators)
	assertAllServerScaleTenantsCurrent(t, store, manifest, dataset)

	var databaseSize int64
	if err := store.pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseSize); err != nil {
		t.Fatal(err)
	}
	if databaseSize > int64(manifest.CalibratedLimits.DatabaseSizeGiB)<<30 {
		t.Fatalf("database size=%d exceeds %d GiB", databaseSize, manifest.CalibratedLimits.DatabaseSizeGiB)
	}
	realProviderRequests := runServerScaleRealProviderProbe(t, store, apiKey, profile)

	payload, _ := json.Marshal(map[string]any{
		"profile":                       manifest.ProfileName,
		"active_before_delete":          manifest.ActiveMemoryCount,
		"active_after_delete":           manifest.ActiveMemoryCount - manifest.DeleteCount,
		"governed_memories":             manifest.GovernedMemoryCount,
		"superseded_memories":           manifest.RevisionCount,
		"projection_events_before_tail": manifest.ProjectionEventCount,
		"projection_events_final":       manifest.ProjectionEventCount + manifest.DeleteCount,
		"lexical_rows_before_delete":    manifest.ActiveMemoryCount,
		"vector_rows_before_delete":     manifest.ActiveMemoryCount,
		"snapshot_embedding_requests":   manifest.ActiveMemoryCount,
		"query_clients":                 manifest.QueryClientCount,
		"query_samples":                 len(latencies),
		"query_p50_ms":                  p50.Microseconds() / 1000.0,
		"query_p95_ms":                  p95.Microseconds() / 1000.0,
		"query_p99_ms":                  p99.Microseconds() / 1000.0,
		"degraded_queries":              degradedCount,
		"effective_vector_queries":      effectiveVectorCount,
		"requested_vector_queries":      auditCounts.RequestedVector,
		"projection_lag_fallbacks":      auditCounts.ProjectionLag,
		"other_degraded_queries":        auditCounts.OtherDegraded,
		"competing_worker_results":      competingWorkerResults,
		"authority_seed_ms":             seedDuration.Milliseconds(),
		"history_generation_ms":         historyDuration.Milliseconds(),
		"lexical_rebuild_ms":            lexicalDuration.Milliseconds(),
		"vector_snapshot_ms":            vectorDuration.Milliseconds(),
		"tail_catchup_ms":               tailDuration.Milliseconds(),
		"delete_and_query_ms":           deleteDuration.Milliseconds(),
		"database_size_bytes":           databaseSize,
		"real_provider_requests":        realProviderRequests,
	})
	t.Logf("server qualification evidence=%s", payload)
}

func loadServerScaleCase(t *testing.T) serverScaleCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W12-server-qualification-scale-profile", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest serverScaleCase
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func prepareServerScaleContinuities(t *testing.T, store *Store, manifest serverScaleCase) serverScaleDataset {
	t.Helper()
	dataset := serverScaleDataset{
		Tenants:      make([]string, manifest.TenantCount),
		Continuities: make(map[string][]string, manifest.TenantCount),
	}
	for tenantIndex := 0; tenantIndex < manifest.TenantCount; tenantIndex++ {
		tenantID := fmt.Sprintf("w12-tenant-%02d", tenantIndex)
		dataset.Tenants[tenantIndex] = tenantID
		continuities := make([]string, manifest.ContinuitiesPerTenant)
		for continuityIndex := 0; continuityIndex < manifest.ContinuitiesPerTenant; continuityIndex++ {
			continuityID, err := store.ConfirmWorkspaceBinding(
				context.Background(), tenantID,
				fmt.Sprintf("/fixtures/w12/tenant-%02d/workspace-%02d", tenantIndex, continuityIndex),
			)
			if err != nil {
				t.Fatal(err)
			}
			continuities[continuityIndex] = continuityID
		}
		dataset.Continuities[tenantID] = continuities
	}
	return dataset
}

func seedServerScaleAuthority(t *testing.T, store *Store, manifest serverScaleCase, dataset serverScaleDataset) {
	t.Helper()
	errorsCh := make(chan error, manifest.TenantCount)
	var tenants sync.WaitGroup
	for tenantIndex, tenantID := range dataset.Tenants {
		tenantIndex := tenantIndex
		tenantID := tenantID
		tenants.Add(1)
		go func() {
			defer tenants.Done()
			errorsCh <- seedServerScaleTenant(store, manifest, dataset, tenantIndex, tenantID)
		}()
	}
	tenants.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func seedServerScaleTenant(
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	tenantIndex int,
	tenantID string,
) error {
	tenantCtx, err := withTenantContext(context.Background(), tenantID)
	if err != nil {
		return err
	}
	const batchSize = 250
	for continuityIndex, continuityID := range dataset.Continuities[tenantID] {
		for batchStart := 0; batchStart < manifest.RecordsPerContinuity; batchStart += batchSize {
			batchEnd := batchStart + batchSize
			if batchEnd > manifest.RecordsPerContinuity {
				batchEnd = manifest.RecordsPerContinuity
			}
			tx, err := store.pool.Begin(tenantCtx)
			if err != nil {
				return err
			}
			for recordIndex := batchStart; recordIndex < batchEnd; recordIndex++ {
				marker := serverScaleMarker(tenantIndex, continuityIndex, recordIndex)
				memoryKey := serverScaleMemoryKey(tenantIndex, continuityIndex, recordIndex)
				request := CommitObservationRequest{
					OperationID: fmt.Sprintf("w12-seed-%02d-%02d-%04d", tenantIndex, continuityIndex, recordIndex),
					Kind:        ObservationKindSourceUpdate,
					Content: fmt.Sprintf(
						"Scale marker %s; endpoint /v1/items/%04d; retry budget %d ms; version 1; 中文记录 %04d。",
						marker, recordIndex, 300+(recordIndex%7)*100, recordIndex,
					),
					SourceRef: fmt.Sprintf("fixture:w12:%02d:%02d:%04d", tenantIndex, continuityIndex, recordIndex),
					MemoryKey: memoryKey,
				}
				observation, err := commitObservationTx(tenantCtx, tx, tenantID, continuityID, request)
				if err != nil {
					tx.Rollback(tenantCtx)
					return err
				}
				if _, err := governObservationTx(tenantCtx, tx, tenantID, continuityID, observation.ObservationID, request); err != nil {
					tx.Rollback(tenantCtx)
					return err
				}
			}
			if err := tx.Commit(tenantCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateServerScaleHistory(t *testing.T, store *Store, manifest serverScaleCase, dataset serverScaleDataset) {
	t.Helper()
	version := 2
	for round := 0; round < manifest.FullRevisionRounds; round++ {
		runServerScaleRevisionRound(t, store, dataset, version, manifest.ActiveMemoryCount/manifest.TenantCount)
		version++
	}
	partialPerTenant := manifest.PartialRevisionCount / manifest.TenantCount
	runServerScaleRevisionRound(t, store, dataset, version, partialPerTenant)
}

func runServerScaleRevisionRound(
	t *testing.T,
	store *Store,
	dataset serverScaleDataset,
	version int,
	perTenant int,
) {
	t.Helper()
	errorsCh := make(chan error, len(dataset.Tenants))
	var tenants sync.WaitGroup
	for _, tenantID := range dataset.Tenants {
		tenantID := tenantID
		tenants.Add(1)
		go func() {
			defer tenants.Done()
			errorsCh <- createServerScaleTenantRevisions(store, tenantID, version, perTenant)
		}()
	}
	tenants.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func createServerScaleTenantRevisions(store *Store, tenantID string, version, limit int) error {
	tenantCtx, err := withTenantContext(context.Background(), tenantID)
	if err != nil {
		return err
	}
	result, err := store.pool.Exec(tenantCtx, `
WITH current_memories AS MATERIALIZED (
  SELECT id, tenant_id, continuity_id, memory_kind, memory_key, content
  FROM governed_memories
  WHERE tenant_id = $1
    AND memory_kind = 'fact'
    AND lifecycle_status = 'active'
  ORDER BY continuity_id, memory_key, id
  LIMIT $3
), inserted_observations AS (
  INSERT INTO observations (
    tenant_id, continuity_id, operation_id, observation_kind, content, source_ref, memory_key
  )
  SELECT tenant_id,
         continuity_id,
         format('w12-revision-v%s-%s', $2::int, id),
         'source_update',
         regexp_replace(content, 'version [0-9]+', 'version ' || $2::text),
         format('fixture:w12:revision:v%s', $2::int),
         memory_key
  FROM current_memories
  RETURNING id, tenant_id, continuity_id, operation_id, content, memory_key
), inserted_memories AS (
  INSERT INTO governed_memories (
    tenant_id, continuity_id, origin_observation_id, memory_kind, memory_key,
    lifecycle_status, content, supersedes_memory_id
  )
  SELECT observation.tenant_id,
         observation.continuity_id,
         observation.id,
         current.memory_kind,
         observation.memory_key,
         'active',
         observation.content,
         current.id
  FROM inserted_observations observation
  JOIN current_memories current
    ON observation.operation_id = format('w12-revision-v%s-%s', $2::int, current.id)
  RETURNING supersedes_memory_id
)
UPDATE governed_memories previous
SET lifecycle_status = 'superseded', updated_at = now()
FROM inserted_memories revision
WHERE previous.id = revision.supersedes_memory_id`, tenantID, version, limit)
	if err != nil {
		return err
	}
	if result.RowsAffected() != int64(limit) {
		return fmt.Errorf("tenant %s revision v%d rows=%d want %d", tenantID, version, result.RowsAffected(), limit)
	}
	return nil
}

func assertServerScaleAuthorityCounts(t *testing.T, store *Store, manifest serverScaleCase, deleted int) {
	t.Helper()
	var total, active, superseded, deletedCount int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*),
       count(*) FILTER (WHERE lifecycle_status = 'active'),
       count(*) FILTER (WHERE lifecycle_status = 'superseded'),
       count(*) FILTER (WHERE lifecycle_status = 'deleted')
FROM governed_memories
WHERE tenant_id LIKE 'w12-tenant-%'`).Scan(&total, &active, &superseded, &deletedCount); err != nil {
		t.Fatal(err)
	}
	if total != manifest.GovernedMemoryCount || active != manifest.ActiveMemoryCount-deleted ||
		superseded != manifest.RevisionCount || deletedCount != deleted {
		t.Fatalf("authority counts total/active/superseded/deleted=%d/%d/%d/%d", total, active, superseded, deletedCount)
	}
}

func assertServerScaleEventCount(t *testing.T, store *Store, want int64) {
	t.Helper()
	var count int64
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM memory_projection_events
WHERE tenant_id LIKE 'w12-tenant-%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("projection event count=%d want %d", count, want)
	}
}

func assertInitialTenantLag(t *testing.T, store *Store, manifest serverScaleCase, dataset serverScaleDataset) {
	t.Helper()
	want := int64(manifest.ProjectionEventCount / manifest.TenantCount)
	for _, tenantID := range dataset.Tenants {
		status, err := store.RetrievalProjectionStatus(context.Background(), tenantID, manifest.ProfileID)
		if err != nil {
			t.Fatal(err)
		}
		if status.LastEventID != 0 || status.Lag != want {
			t.Fatalf("tenant %s initial status=%#v want lag %d", tenantID, status, want)
		}
	}
}

func loadCurrentServerScaleRecords(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
) (map[string]serverScaleRecord, map[string]serverScaleRecord) {
	t.Helper()
	rows, err := store.pool.Query(context.Background(), `
SELECT id::text, tenant_id, continuity_id::text, memory_key, content
FROM governed_memories
WHERE tenant_id LIKE 'w12-tenant-%'
  AND memory_kind = 'fact'
  AND lifecycle_status = 'active'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	byMarker := make(map[string]serverScaleRecord, manifest.ActiveMemoryCount)
	byID := make(map[string]serverScaleRecord, manifest.ActiveMemoryCount)
	for rows.Next() {
		var record serverScaleRecord
		var content string
		if err := rows.Scan(&record.MemoryID, &record.TenantID, &record.ContinuityID, &record.MemoryKey, &content); err != nil {
			t.Fatal(err)
		}
		record.Marker = serverScaleMarkerPattern.FindString(content)
		if record.Marker == "" {
			t.Fatalf("active scale memory %s lost its marker", record.MemoryID)
		}
		byMarker[record.Marker] = record
		byID[record.MemoryID] = record
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(byMarker) != manifest.ActiveMemoryCount || len(byID) != manifest.ActiveMemoryCount {
		t.Fatalf("active record maps=%d/%d want %d", len(byMarker), len(byID), manifest.ActiveMemoryCount)
	}
	return byMarker, byID
}

func rebuildServerScaleVectors(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	profile RetrievalProfile,
	embedder Embedder,
) []ProjectionRebuildResult {
	t.Helper()
	results := make(chan ProjectionRebuildResult, manifest.TenantCount)
	errorsCh := make(chan error, manifest.TenantCount)
	var tenants sync.WaitGroup
	for _, tenantID := range dataset.Tenants {
		tenantID := tenantID
		tenants.Add(1)
		go func() {
			defer tenants.Done()
			worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
				TenantID: tenantID, Profile: profile, SnapshotPageSize: manifest.SnapshotPageSize,
			})
			if err != nil {
				errorsCh <- err
				return
			}
			result, err := worker.RebuildCurrent(context.Background())
			results <- result
			errorsCh <- err
		}()
	}
	tenants.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	collected := make([]ProjectionRebuildResult, 0, manifest.TenantCount)
	for result := range results {
		collected = append(collected, result)
	}
	if len(collected) != manifest.TenantCount {
		t.Fatalf("snapshot results=%d want %d", len(collected), manifest.TenantCount)
	}
	return collected
}

func assertServerScaleProjectionCounts(t *testing.T, store *Store, lexicalWant, vectorWant int) {
	t.Helper()
	var lexicalCount, vectorCount int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM memory_search_documents WHERE tenant_id LIKE 'w12-tenant-%'),
  (SELECT count(*) FROM memory_vector_documents
   WHERE tenant_id LIKE 'w12-tenant-%' AND profile_id = $1)`, ProductionRetrievalProfileID).Scan(&lexicalCount, &vectorCount); err != nil {
		t.Fatal(err)
	}
	if lexicalCount != lexicalWant || vectorCount != vectorWant {
		t.Fatalf("projection counts lexical/vector=%d/%d want %d/%d", lexicalCount, vectorCount, lexicalWant, vectorWant)
	}
}

func newServerScaleCoordinators(
	t *testing.T,
	store *Store,
	dataset serverScaleDataset,
	profile RetrievalProfile,
	embedder Embedder,
) map[string]*RetrievalCoordinator {
	t.Helper()
	coordinators := make(map[string]*RetrievalCoordinator, len(dataset.Tenants))
	for _, tenantID := range dataset.Tenants {
		coordinator, err := NewRetrievalCoordinator(store, embedder, profile)
		if err != nil {
			t.Fatal(err)
		}
		coordinators[tenantID] = coordinator
	}
	return coordinators
}

func runServerScaleQueryPhase(
	ctx context.Context,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	coordinators map[string]*RetrievalCoordinator,
	startQuery int,
	endQuery int,
	measurements chan<- serverScaleQueryMeasurement,
	errorsCh chan<- error,
) {
	var clients sync.WaitGroup
	for client := 0; client < manifest.QueryClientCount; client++ {
		client := client
		clients.Add(1)
		go func() {
			defer clients.Done()
			for queryIndex := startQuery; queryIndex < endQuery; queryIndex++ {
				measurement, err := executeServerScaleQuery(
					ctx, manifest, dataset, coordinators, client, queryIndex, RetrievalVector,
				)
				if err != nil {
					errorsCh <- err
					continue
				}
				measurements <- measurement
			}
		}()
	}
	clients.Wait()
}

func executeServerScaleQuery(
	ctx context.Context,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	coordinators map[string]*RetrievalCoordinator,
	client int,
	queryIndex int,
	mode RetrievalMode,
) (serverScaleQueryMeasurement, error) {
	tenantIndex := client % manifest.TenantCount
	continuityIndex := (client/manifest.TenantCount + queryIndex) % manifest.ContinuitiesPerTenant
	safeStart := manifest.DeleteCount/manifest.TenantCount + 1
	available := manifest.RecordsPerContinuity - safeStart
	if available <= 0 {
		return serverScaleQueryMeasurement{}, fmt.Errorf("no non-deleted query records remain")
	}
	recordIndex := safeStart + (client*manifest.QueriesPerClient+queryIndex)%available
	marker := serverScaleMarker(tenantIndex, continuityIndex, recordIndex)
	record, ok := dataset.Records[marker]
	if !ok {
		return serverScaleQueryMeasurement{}, fmt.Errorf("missing query target %s", marker)
	}
	started := time.Now()
	result, err := coordinators[record.TenantID].Retrieve(ctx, RetrievalRequest{
		OperationID: fmt.Sprintf("w12-query-%03d-%02d-%s", client, queryIndex, mode),
		TenantID:    record.TenantID, ContinuityIDs: []string{record.ContinuityID},
		Query: marker, Limit: 1, Mode: mode,
	})
	latency := time.Since(started)
	if err != nil {
		return serverScaleQueryMeasurement{}, err
	}
	if len(result.Memories) != 1 || result.Memories[0].ID != record.MemoryID {
		return serverScaleQueryMeasurement{}, fmt.Errorf(
			"query %s returned %#v effective=%s degraded=%v", marker, result.Memories, result.Effective, result.Degraded,
		)
	}
	return serverScaleQueryMeasurement{Latency: latency, Mode: mode, Effective: result.Effective, Degraded: result.Degraded}, nil
}

func deleteServerScaleTenantRecords(
	ctx context.Context,
	manifest serverScaleCase,
	store *Store,
	dataset serverScaleDataset,
	tenantIndex int,
	errorsCh chan<- error,
) {
	tenantID := dataset.Tenants[tenantIndex]
	perTenant := manifest.DeleteCount / manifest.TenantCount
	for recordIndex := 0; recordIndex < perTenant; recordIndex++ {
		marker := serverScaleMarker(tenantIndex, 0, recordIndex)
		record, ok := dataset.Records[marker]
		if !ok {
			errorsCh <- fmt.Errorf("missing delete target %s", marker)
			continue
		}
		if err := store.DeleteMemory(ctx, tenantID, record.ContinuityID, record.MemoryID); err != nil {
			errorsCh <- err
		}
	}
}

func drainServerScaleTail(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	embedder Embedder,
) map[string]int {
	t.Helper()
	var alreadyRunning atomic.Int64
	var processed atomic.Int64
	profile := productionRetrievalProfile(t)
	errorsCh := make(chan error, manifest.TenantCount*manifest.TailWorkersPerTenant)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, tenantID := range dataset.Tenants {
		tenantID := tenantID
		for competitor := 0; competitor < manifest.TailWorkersPerTenant; competitor++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
					TenantID: tenantID, Profile: profile, BatchSize: 256,
				})
				if err != nil {
					errorsCh <- err
					return
				}
				<-start
				result, err := worker.RunOnce(context.Background())
				if result.AlreadyRunning {
					alreadyRunning.Add(1)
				}
				processed.Add(int64(result.Processed))
				errorsCh <- err
			}()
		}
	}
	close(start)
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tenantID := range dataset.Tenants {
		worker := mustProjectionWorker(t, store, embedder, tenantID, 256)
		for attempt := 0; attempt < 10; attempt++ {
			status, err := store.RetrievalProjectionStatus(context.Background(), tenantID, manifest.ProfileID)
			if err != nil {
				t.Fatal(err)
			}
			if status.Lag == 0 {
				break
			}
			result, err := worker.RunOnce(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			processed.Add(int64(result.Processed))
		}
	}
	return map[string]int{
		"already_running": int(alreadyRunning.Load()),
		"processed":       int(processed.Load()),
	}
}

func collectServerScaleMeasurements(
	measurements <-chan serverScaleQueryMeasurement,
) ([]time.Duration, int, int) {
	latencies := make([]time.Duration, 0)
	degraded := 0
	effectiveVector := 0
	for measurement := range measurements {
		latencies = append(latencies, measurement.Latency)
		if measurement.Degraded {
			degraded++
		}
		if measurement.Effective == RetrievalVector {
			effectiveVector++
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return latencies, degraded, effectiveVector
}

func loadServerScaleRetrievalAuditCounts(
	t *testing.T,
	store *Store,
	dataset serverScaleDataset,
) serverScaleRetrievalAuditCounts {
	t.Helper()
	var counts serverScaleRetrievalAuditCounts
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FILTER (WHERE requested_mode = 'vector'),
       count(*) FILTER (WHERE effective_mode = 'vector' AND NOT degraded),
       count(*) FILTER (WHERE failure_code = 'projection_lag' AND degraded),
       count(*) FILTER (WHERE degraded AND failure_code <> 'projection_lag')
FROM memory_retrieval_runs
WHERE tenant_id = ANY($1::text[])
  AND operation_id LIKE 'w12-query-%'`, dataset.Tenants).Scan(
		&counts.RequestedVector,
		&counts.EffectiveVector,
		&counts.ProjectionLag,
		&counts.OtherDegraded,
	); err != nil {
		t.Fatal(err)
	}
	return counts
}

func percentileDuration(sorted []time.Duration, percentile int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := (len(sorted) - 1) * percentile / 100
	return sorted[index]
}

func assertDeletedServerScaleRecordsAbsent(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
) {
	t.Helper()
	deletedIDs := make([]string, 0, manifest.DeleteCount)
	for tenantIndex := 0; tenantIndex < manifest.TenantCount; tenantIndex++ {
		for recordIndex := 0; recordIndex < manifest.DeleteCount/manifest.TenantCount; recordIndex++ {
			deletedIDs = append(deletedIDs, dataset.Records[serverScaleMarker(tenantIndex, 0, recordIndex)].MemoryID)
		}
	}
	var activeRows, lexicalRows, vectorRows int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM governed_memories WHERE id = ANY($1::uuid[]) AND lifecycle_status = 'active'),
  (SELECT count(*) FROM memory_search_documents WHERE memory_id = ANY($1::uuid[])),
  (SELECT count(*) FROM memory_vector_documents WHERE memory_id = ANY($1::uuid[]))`, deletedIDs).Scan(
		&activeRows, &lexicalRows, &vectorRows,
	); err != nil {
		t.Fatal(err)
	}
	if activeRows != 0 || lexicalRows != 0 || vectorRows != 0 {
		t.Fatalf("deleted residue active/lexical/vector=%d/%d/%d", activeRows, lexicalRows, vectorRows)
	}
}

func assertServerScaleScopes(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
	coordinators map[string]*RetrievalCoordinator,
) {
	t.Helper()
	for tenantIndex := 0; tenantIndex < manifest.TenantCount; tenantIndex++ {
		targetContinuityIndex := 1 % manifest.ContinuitiesPerTenant
		wrongContinuityIndex := (targetContinuityIndex + 1) % manifest.ContinuitiesPerTenant
		recordIndex := manifest.DeleteCount/manifest.TenantCount + 1
		marker := serverScaleMarker(tenantIndex, targetContinuityIndex, recordIndex)
		target, ok := dataset.Records[marker]
		if !ok {
			t.Fatalf("missing scope target %s", marker)
		}
		wrongContinuity := dataset.Continuities[target.TenantID][wrongContinuityIndex]
		matches, err := store.SearchActiveMemory(context.Background(), target.TenantID, wrongContinuity, marker, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, memory := range matches {
			if memory.ID == target.MemoryID {
				t.Fatalf("cross-continuity lexical leak for %s", marker)
			}
		}
		otherTenant := dataset.Tenants[(tenantIndex+1)%manifest.TenantCount]
		result, err := coordinators[otherTenant].Retrieve(context.Background(), RetrievalRequest{
			OperationID: fmt.Sprintf("w12-cross-tenant-%02d", tenantIndex),
			TenantID:    otherTenant, ContinuityIDs: []string{dataset.Continuities[otherTenant][targetContinuityIndex]},
			Query: marker, Limit: 1, Mode: RetrievalVector,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, memory := range result.Memories {
			if memory.ID == target.MemoryID {
				t.Fatalf("cross-tenant vector leak for %s", marker)
			}
		}
	}
}

func assertAllServerScaleTenantsCurrent(
	t *testing.T,
	store *Store,
	manifest serverScaleCase,
	dataset serverScaleDataset,
) {
	t.Helper()
	for _, tenantID := range dataset.Tenants {
		status, err := store.RetrievalProjectionStatus(context.Background(), tenantID, manifest.ProfileID)
		if err != nil {
			t.Fatal(err)
		}
		if status.Lag != 0 || status.Status != "idle" ||
			status.VectorCount != int64((manifest.ActiveMemoryCount-manifest.DeleteCount)/manifest.TenantCount) {
			t.Fatalf("tenant %s final status=%#v", tenantID, status)
		}
	}
}

func runServerScaleRealProviderProbe(
	t *testing.T,
	store *Store,
	apiKey string,
	profile RetrievalProfile,
) int64 {
	t.Helper()
	const tenantID = "w12-real-provider-tenant"
	const repoRoot = "/fixtures/w12/real-provider"
	continuityID, err := store.ConfirmWorkspaceBinding(context.Background(), tenantID, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	governance := NewGovernanceService(store, tenantID)
	memory, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: "w12-real-provider-source", MemoryKey: "w12.real.provider",
		Content: "The W12 direct provider recovery code is W12-SILICON-9081.", SourceRef: "fixture:w12-real-provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := memorybackend.NewOpenAIEmbedder(
		profile.BaseURL, apiKey, profile.Model, profile.Dimensions, &http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	embedder := &faultCountingEmbedder{Embedder: base}
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID: tenantID, Profile: profile, SnapshotPageSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.RebuildCurrent(context.Background()); err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewRetrievalCoordinator(store, embedder, profile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID: "w12-real-provider-query", TenantID: tenantID,
		ContinuityIDs: []string{continuityID}, Query: "What is the W12 direct provider recovery code?",
		Limit: 3, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsMemoryID(result.Memories, memory.Memory.MemoryID) || embedder.Count() != 2 ||
		result.Effective != RetrievalVector || result.Degraded {
		t.Fatalf("real provider probe mismatch: memories=%#v requests=%d", result.Memories, embedder.Count())
	}
	return embedder.Count()
}

func assertDurationWithin(t *testing.T, phase string, duration time.Duration, limitSeconds int) {
	t.Helper()
	if duration > time.Duration(limitSeconds)*time.Second {
		t.Fatalf("%s duration=%s exceeds %ds", phase, duration, limitSeconds)
	}
}

var serverScaleMarkerPattern = regexp.MustCompile(`W12-T[0-9]{2}-C[0-9]{2}-R[0-9]{4}`)

type serverScaleEmbedder struct {
	calls atomic.Int64
}

func (embedder *serverScaleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	embedder.calls.Add(1)
	key := serverScaleMarkerPattern.FindString(text)
	if key == "" {
		key = strings.TrimSpace(text)
	}
	vector := make([]float32, 1024)
	digest := sha256.Sum256([]byte(key))
	for round := 0; round < 4; round++ {
		for offset := 0; offset < len(digest); offset += 2 {
			index := int(binary.BigEndian.Uint16(digest[offset:offset+2]) % 1024)
			value := float32(digest[(offset+round+1)%len(digest)]+32) / 287
			vector[index] += value
		}
		digest = sha256.Sum256(digest[:])
	}
	return vector, nil
}

func serverScaleMarker(tenantIndex, continuityIndex, recordIndex int) string {
	return fmt.Sprintf("W12-T%02d-C%02d-R%04d", tenantIndex, continuityIndex, recordIndex)
}

func serverScaleMemoryKey(tenantIndex, continuityIndex, recordIndex int) string {
	return fmt.Sprintf("w12.t%02d.c%02d.r%04d", tenantIndex, continuityIndex, recordIndex)
}
