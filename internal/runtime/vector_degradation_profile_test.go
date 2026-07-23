package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type vectorDegradationCase struct {
	Version                string                      `json:"version"`
	ID                     string                      `json:"id"`
	ProfileName            string                      `json:"profile_name"`
	SourceProfile          string                      `json:"source_profile"`
	ActiveMemoryCount      int                         `json:"active_memory_count"`
	TenantCount            int                         `json:"tenant_count"`
	ContinuitiesPerTenant  int                         `json:"continuities_per_tenant"`
	RecordsPerContinuity   int                         `json:"records_per_continuity"`
	QueryClientCount       int                         `json:"query_client_count"`
	VectorQueriesPerClient int                         `json:"vector_queries_per_client"`
	RequestedVectorQueries int                         `json:"requested_vector_queries"`
	ConcurrentDeleteCount  int                         `json:"concurrent_delete_count"`
	SnapshotPageSize       int                         `json:"snapshot_page_size"`
	PoolMaxConnections     int                         `json:"pool_max_connections"`
	ProfileID              string                      `json:"profile_id"`
	ObservedRuns           vectorDegradationRuns       `json:"observed_runs"`
	CalibratedLimits       serverScaleCalibratedLimits `json:"calibrated_limits"`
	HardGates              []string                    `json:"hard_gates"`
}

type vectorDegradationRuns struct {
	W12Original              vectorDegradationRun `json:"w12_original"`
	W12Attribution           vectorDegradationRun `json:"w12_attribution"`
	ProjectionCurrentControl vectorDegradationRun `json:"projection_current_control"`
}

type vectorDegradationRun struct {
	EffectiveVectorQueries     int  `json:"effective_vector_queries"`
	DegradedQueries            int  `json:"degraded_queries"`
	ProjectionLagFallbacks     int  `json:"projection_lag_fallbacks"`
	OtherDegradedQueries       int  `json:"other_degraded_queries"`
	FailureCodeBreakdownStored bool `json:"failure_code_breakdown_recorded"`
}

type vectorCurrentResult struct {
	QuerySamples    int
	EffectiveVector int
	Degraded        int
	P50             time.Duration
	P95             time.Duration
	P99             time.Duration
	VectorDuration  time.Duration
	DatabaseSize    int64
}

func TestVectorDegradationCaseIsFrozen(t *testing.T) {
	manifest := loadVectorDegradationCase(t)
	if manifest.Version != "1" || manifest.ID != "W13-vector-degradation-attribution" ||
		manifest.ProfileName != "vector-degradation-attribution-v1" ||
		manifest.SourceProfile != "W12-server-qualification-scale-profile" {
		t.Fatalf("unexpected W13 identity: %#v", manifest)
	}
	if manifest.TenantCount*manifest.ContinuitiesPerTenant*manifest.RecordsPerContinuity != manifest.ActiveMemoryCount ||
		manifest.QueryClientCount*manifest.VectorQueriesPerClient != manifest.RequestedVectorQueries {
		t.Fatalf("W13 arithmetic drifted: %#v", manifest)
	}
	if manifest.ActiveMemoryCount != 100000 || manifest.RequestedVectorQueries != 550 ||
		manifest.ConcurrentDeleteCount != 1000 || manifest.ProfileID != ProductionRetrievalProfileID ||
		manifest.PoolMaxConnections != 64 || len(manifest.HardGates) != 8 {
		t.Fatalf("W13 hard-gate contract drifted: %#v", manifest)
	}
	if manifest.ObservedRuns.W12Attribution.EffectiveVectorQueries+
		manifest.ObservedRuns.W12Attribution.ProjectionLagFallbacks != manifest.RequestedVectorQueries ||
		manifest.ObservedRuns.W12Attribution.OtherDegradedQueries != 0 ||
		manifest.ObservedRuns.ProjectionCurrentControl.EffectiveVectorQueries != manifest.RequestedVectorQueries ||
		manifest.ObservedRuns.ProjectionCurrentControl.DegradedQueries != 0 {
		t.Fatalf("W13 measured attribution drifted: %#v", manifest.ObservedRuns)
	}
}

func TestVectorProjectionCurrentScaleProfile(t *testing.T) {
	if os.Getenv("VERMORY_VECTOR_CURRENT_SCALE_PROFILE") != "1" {
		t.Skip("VERMORY_VECTOR_CURRENT_SCALE_PROFILE=1 is required")
	}
	frozen := loadVectorDegradationCase(t)
	manifest := serverScaleCase{
		TenantCount: frozen.TenantCount, ContinuitiesPerTenant: frozen.ContinuitiesPerTenant,
		RecordsPerContinuity: frozen.RecordsPerContinuity, ActiveMemoryCount: frozen.ActiveMemoryCount,
		QueryClientCount: frozen.QueryClientCount, QueriesPerClient: frozen.VectorQueriesPerClient,
		SnapshotPageSize: frozen.SnapshotPageSize, PoolMaxConnections: frozen.PoolMaxConnections,
		ProfileID: frozen.ProfileID, CalibratedLimits: frozen.CalibratedLimits,
	}
	result := runVectorCurrentScaleProfile(t, manifest)
	if result.QuerySamples != frozen.RequestedVectorQueries ||
		result.EffectiveVector != frozen.RequestedVectorQueries || result.Degraded != 0 {
		t.Fatalf(
			"vector current samples/effective/degraded=%d/%d/%d want %d/%d/0",
			result.QuerySamples, result.EffectiveVector, result.Degraded,
			frozen.RequestedVectorQueries, frozen.RequestedVectorQueries,
		)
	}
	if result.P95 > time.Duration(frozen.CalibratedLimits.QueryP95MS)*time.Millisecond ||
		result.P99 > time.Duration(frozen.CalibratedLimits.QueryP99MS)*time.Millisecond ||
		result.VectorDuration > time.Duration(frozen.CalibratedLimits.VectorSnapshotSeconds)*time.Second ||
		result.DatabaseSize > int64(frozen.CalibratedLimits.DatabaseSizeGiB)<<30 {
		t.Fatalf("vector current profile exceeded limits: %#v", result)
	}
	payload, _ := json.Marshal(map[string]any{
		"query_samples": result.QuerySamples, "effective_vector_queries": result.EffectiveVector,
		"degraded_queries": result.Degraded, "query_p50_ms": result.P50.Microseconds() / 1000.0,
		"query_p95_ms": result.P95.Microseconds() / 1000.0, "query_p99_ms": result.P99.Microseconds() / 1000.0,
		"vector_snapshot_ms": result.VectorDuration.Milliseconds(), "database_size_bytes": result.DatabaseSize,
	})
	t.Logf("vector current control evidence=%s", payload)
}

func runVectorCurrentScaleProfile(t *testing.T, manifest serverScaleCase) vectorCurrentResult {
	t.Helper()
	cluster := startDisposablePostgres18(t)
	defer cluster.stop(t, "fast")
	ctx := context.Background()
	store, err := OpenStore(ctx, cluster.databaseURL+"&pool_max_conns="+strconv.Itoa(manifest.PoolMaxConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	dataset := prepareServerScaleContinuities(t, store, manifest)
	seedServerScaleAuthority(t, store, manifest, dataset)
	if rows, err := store.RebuildAllProjections(ctx); err != nil {
		t.Fatal(err)
	} else if rows != int64(manifest.ActiveMemoryCount) {
		t.Fatalf("lexical rows=%d want %d", rows, manifest.ActiveMemoryCount)
	}
	dataset.Records, dataset.RecordsByID = loadCurrentServerScaleRecords(t, store, manifest)
	embedder := &serverScaleEmbedder{}
	vectorStarted := time.Now()
	results := rebuildServerScaleVectors(t, store, manifest, dataset, productionRetrievalProfile(t), embedder)
	vectorDuration := time.Since(vectorStarted)
	for _, result := range results {
		if result.Projected != result.Scanned || result.SkippedChanged != 0 || result.Lag != 0 {
			t.Fatalf("unexpected snapshot result: %#v", result)
		}
	}
	coordinators := newServerScaleCoordinators(t, store, dataset, productionRetrievalProfile(t), embedder)
	measurements := make(chan serverScaleQueryMeasurement, manifest.QueryClientCount*manifest.QueriesPerClient)
	errorsCh := make(chan error, manifest.QueryClientCount*manifest.QueriesPerClient)
	runServerScaleQueryPhase(ctx, manifest, dataset, coordinators, 0, manifest.QueriesPerClient, measurements, errorsCh)
	close(measurements)
	close(errorsCh)
	for queryErr := range errorsCh {
		if queryErr != nil {
			t.Fatal(queryErr)
		}
	}
	latencies, degraded, effectiveVector := collectServerScaleMeasurements(measurements)
	assertServerScaleScopes(t, store, manifest, dataset, coordinators)
	assertAllServerScaleTenantsCurrent(t, store, manifest, dataset)
	var databaseSize int64
	if err := store.pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseSize); err != nil {
		t.Fatal(err)
	}
	return vectorCurrentResult{
		QuerySamples: len(latencies), EffectiveVector: effectiveVector, Degraded: degraded,
		P50: percentileDuration(latencies, 50), P95: percentileDuration(latencies, 95),
		P99: percentileDuration(latencies, 99), VectorDuration: vectorDuration, DatabaseSize: databaseSize,
	}
}

func loadVectorDegradationCase(t *testing.T) vectorDegradationCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W13-vector-degradation-attribution", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest vectorDegradationCase
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
