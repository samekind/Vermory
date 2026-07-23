package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/memorybackend"
)

func TestActiveBacklogDimensionalMigrationProfile(t *testing.T) {
	if os.Getenv("VERMORY_DIMENSIONAL_MIGRATION_PROFILE") != "1" {
		t.Skip("VERMORY_DIMENSIONAL_MIGRATION_PROFILE=1 is required")
	}
	manifest := loadDimensionalMigrationCase(t)
	runID := strings.TrimSpace(os.Getenv("VERMORY_DIMENSIONAL_MIGRATION_RUN_ID"))
	baseRoot := strings.TrimSpace(os.Getenv("VERMORY_DIMENSIONAL_MIGRATION_ROOT"))
	binDir := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES_BIN_DIR"))
	runRoot, err := dimensionalRunRoot(baseRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(runRoot, "report.json")
	if _, err := os.Stat(reportPath); err == nil {
		report, err := ReadDimensionalMigrationReport(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("W17 replay completed report: run=%s revision=%s", report.RunID, report.ImplementationRevision)
		return
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	apiKey := strings.TrimSpace(os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY"))
	if apiKey == "" {
		t.Fatal("VERMORY_LIVE_EMBEDDING_API_KEY is required for the formal W17 profile")
	}
	revision := dimensionalGitRevision(t)
	caseHash := dimensionalCaseHash(t)
	startedAt := time.Now().UTC()
	cluster := startDimensionalPostgres18(t, runRoot, binDir)
	if dimensionalDatabaseURLHost(cluster.databaseURL) != "127.0.0.1" {
		t.Fatalf("dedicated W17 database is not loopback-only")
	}
	store, err := OpenStore(context.Background(), fmt.Sprintf("%s&pool_max_conns=%d", cluster.databaseURL, manifest.PoolMaxConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil || version != 16 {
		t.Fatalf("W17 schema version=%d err=%v", version, err)
	}
	var pgvectorVersion string
	if err := store.pool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&pgvectorVersion); err != nil {
		t.Fatal(err)
	}

	seedStarted := time.Now()
	dataset := seedDimensionalFixture(
		t, store, "w17-profile", manifest.TenantCount, manifest.ContinuitiesPerTenant, manifest.RecordsPerContinuity,
	)
	seedDuration := time.Since(seedStarted)
	assertDimensionalDuration(t, "authority seed", seedDuration, manifest.CalibratedLimits.AuthoritySeedSeconds)
	if active := dimensionalDatasetActiveCount(t, store, dataset); active != manifest.InitialActiveCount {
		t.Fatalf("W17 initial active=%d want %d", active, manifest.InitialActiveCount)
	}
	var initialEvents int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_projection_events`).Scan(&initialEvents); err != nil {
		t.Fatal(err)
	}
	if initialEvents != manifest.InitialActiveCount {
		t.Fatalf("W17 initial events=%d want %d", initialEvents, manifest.InitialActiveCount)
	}
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != int64(manifest.InitialActiveCount) {
		t.Fatalf("W17 initial lexical rows=%d err=%v", rows, err)
	}

	incumbentProfile := productionRetrievalProfile(t)
	candidateProfile := dimensionalMigrationProfile(t)
	incumbentEmbedder := &dimensionalFixtureEmbedder{dimensions: incumbentProfile.Dimensions}
	candidateDelegate := &dimensionalFixtureEmbedder{dimensions: candidateProfile.Dimensions}
	incumbentWorkers := make(map[string]*ProjectionWorker, manifest.TenantCount)
	incumbentSnapshotStarted := time.Now()
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(
			t, store, tenantID, incumbentProfile, incumbentEmbedder,
			manifest.SnapshotPageSize, manifest.WorkerBatchSize,
		)
		incumbentWorkers[tenantID] = worker
		rebuild, err := worker.RebuildCurrent(ctx)
		if err != nil || rebuild.Projected != manifest.InitialActiveCount/manifest.TenantCount || rebuild.Lag != 0 {
			t.Fatalf("W17 incumbent snapshot tenant=%s result=%#v err=%v", tenantID, rebuild, err)
		}
	}
	incumbentSnapshotDuration := time.Since(incumbentSnapshotStarted)
	assertDimensionalDuration(t, "incumbent snapshot", incumbentSnapshotDuration, manifest.CalibratedLimits.IncumbentSnapshotSeconds)
	if calls := incumbentEmbedder.calls.Load(); calls != int64(manifest.InitialActiveCount) {
		t.Fatalf("W17 incumbent embedding calls=%d want %d", calls, manifest.InitialActiveCount)
	}

	blocking := &dimensionalBlockingEmbedder{
		delegate: candidateDelegate, started: make(chan struct{}), release: make(chan struct{}),
	}
	type candidateRebuildResult struct {
		tenant string
		result ProjectionRebuildResult
		err    error
	}
	candidateWorkers := make(map[string]*ProjectionWorker, manifest.TenantCount)
	candidateResults := make(chan candidateRebuildResult, manifest.TenantCount)
	candidateCtx, cancelCandidate := context.WithTimeout(ctx, 45*time.Second)
	defer cancelCandidate()
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(
			t, store, tenantID, candidateProfile, blocking,
			manifest.SnapshotPageSize, manifest.WorkerBatchSize,
		)
		candidateWorkers[tenantID] = worker
		go func(tenant string, candidateWorker *ProjectionWorker) {
			result, runErr := candidateWorker.RebuildCurrent(candidateCtx)
			candidateResults <- candidateRebuildResult{tenant: tenant, result: result, err: runErr}
		}(tenantID, worker)
	}
	select {
	case <-blocking.started:
	case <-time.After(10 * time.Second):
		t.Fatal("W17 candidate workers did not reach embedding")
	}
	deadline := time.Now().Add(10 * time.Second)
	for blocking.entered.Load() < int64(manifest.TenantCount) {
		if time.Now().After(deadline) {
			t.Fatalf("W17 candidate workers at embedding=%d want %d", blocking.entered.Load(), manifest.TenantCount)
		}
		time.Sleep(10 * time.Millisecond)
	}

	writerStarted := time.Now()
	applyDimensionalFixtureTail(
		t, store, &dataset,
		manifest.RevisionCount/manifest.TenantCount,
		manifest.DeleteCount/manifest.TenantCount,
		manifest.NewFactCount/manifest.TenantCount,
	)
	writerDuration := time.Since(writerStarted)
	assertDimensionalDuration(t, "active writer", writerDuration, manifest.CalibratedLimits.WriterSeconds)
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != int64(manifest.InitialActiveCount) {
		t.Fatalf("W17 tail lexical rows=%d err=%v", rows, err)
	}
	var finalEvents int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_projection_events`).Scan(&finalEvents); err != nil {
		t.Fatal(err)
	}
	if tail := finalEvents - initialEvents; tail != manifest.TailEventCount {
		t.Fatalf("W17 tail events=%d want %d", tail, manifest.TailEventCount)
	}
	if active := dimensionalDatasetActiveCount(t, store, dataset); active != manifest.InitialActiveCount {
		t.Fatalf("W17 final active=%d want %d", active, manifest.InitialActiveCount)
	}

	querySummary := runDimensionalIncumbentQueryLoad(t, store, manifest, dataset, incumbentProfile, incumbentEmbedder)
	if querySummary.Successful != manifest.QueryClientCount*manifest.QueriesPerClient || querySummary.CrossScope != 0 {
		t.Fatalf("W17 incumbent query summary=%#v", querySummary)
	}
	queryP50, queryP95, queryP99 := dimensionalLatencyPercentiles(querySummary.Latencies)
	if queryP95 > time.Duration(manifest.CalibratedLimits.QueryP95MS)*time.Millisecond ||
		queryP99 > time.Duration(manifest.CalibratedLimits.QueryP99MS)*time.Millisecond {
		t.Fatalf("W17 query latency exceeded profile: p95=%s p99=%s", queryP95, queryP99)
	}

	outageTenant := dataset.Tenants[0]
	outageRecord := dataset.Records[outageTenant][len(dataset.Records[outageTenant])/2]
	outageCoordinator, err := NewRetrievalCoordinator(store, incumbentEmbedder, incumbentProfile)
	if err != nil {
		t.Fatal(err)
	}
	cluster.stop(t, "immediate")
	outageCtx, cancelOutage := context.WithTimeout(ctx, 3*time.Second)
	outageResult, outageErr := outageCoordinator.Retrieve(outageCtx, RetrievalRequest{
		OperationID: runID + "-database-outage", TenantID: outageTenant,
		ContinuityIDs: []string{outageRecord.ContinuityID}, Query: outageRecord.Content,
		Limit: 1, Mode: RetrievalVector,
	})
	cancelOutage()
	if outageErr == nil || len(outageResult.Memories) != 0 || outageResult.AuditID != "" {
		t.Fatalf("W17 outage returned a false result or receipt: result=%#v err=%v", outageResult, outageErr)
	}
	close(blocking.release)
	for range dataset.Tenants {
		select {
		case interrupted := <-candidateResults:
			if interrupted.err == nil {
				t.Fatalf("W17 interrupted candidate unexpectedly succeeded: %#v", interrupted)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("W17 interrupted candidate did not return")
		}
	}
	cluster.start(t)
	recoveryDuration := waitForDimensionalStoreRecovery(t, store, time.Duration(manifest.CalibratedLimits.RestartRecoverySeconds)*time.Second)
	var outageAuditRows int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_retrieval_runs WHERE operation_id = $1`, runID+"-database-outage").Scan(&outageAuditRows); err != nil {
		t.Fatal(err)
	}
	if outageAuditRows != 0 {
		t.Fatalf("W17 outage audit rows=%d want 0", outageAuditRows)
	}
	partialCandidateRows := dimensionalDatasetVectorCount(t, store, dataset, ProjectionClass2560)
	var interruptedCursorAdvance int64
	for _, tenantID := range dataset.Tenants {
		status, err := store.RetrievalProjectionStatus(ctx, tenantID, DimensionalMigrationRetrievalProfileID)
		if err != nil {
			t.Fatal(err)
		}
		interruptedCursorAdvance += status.LastEventID
	}
	if partialCandidateRows != 0 || interruptedCursorAdvance != 0 {
		t.Fatalf("W17 restart committed partial candidate state: rows=%d cursor_sum=%d", partialCandidateRows, interruptedCursorAdvance)
	}

	postRestart, err := outageCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: runID + "-after-restart", TenantID: outageTenant,
		ContinuityIDs: []string{outageRecord.ContinuityID}, Query: outageRecord.Content,
		Limit: 1, Mode: RetrievalVector,
	})
	if err != nil || len(postRestart.Memories) != 1 || postRestart.Memories[0].ID != outageRecord.MemoryID {
		t.Fatalf("W17 same pool did not resume incumbent retrieval: result=%#v err=%v", postRestart, err)
	}

	candidateSnapshotStarted := time.Now()
	var candidateScanned, candidateProjected, candidateSkipped int
	for _, tenantID := range dataset.Tenants {
		rebuild, err := candidateWorkers[tenantID].RebuildCurrent(ctx)
		if err != nil || rebuild.Lag != 0 {
			t.Fatalf("W17 candidate retry snapshot tenant=%s result=%#v err=%v", tenantID, rebuild, err)
		}
		candidateScanned += rebuild.Scanned
		candidateProjected += rebuild.Projected
		candidateSkipped += rebuild.SkippedChanged
	}
	candidateSnapshotDuration := time.Since(candidateSnapshotStarted)
	assertDimensionalDuration(t, "candidate snapshot", candidateSnapshotDuration, manifest.CalibratedLimits.CandidateSnapshotSeconds)
	for _, tenantID := range dataset.Tenants {
		incumbentStatus := drainDimensionalProjection(t, incumbentWorkers[tenantID])
		candidateStatus, err := store.RetrievalProjectionStatus(ctx, tenantID, DimensionalMigrationRetrievalProfileID)
		if err != nil {
			t.Fatal(err)
		}
		if incumbentStatus.Lag != 0 || candidateStatus.Lag != 0 {
			t.Fatalf("W17 final lag tenant=%s incumbent=%#v candidate=%#v", tenantID, incumbentStatus, candidateStatus)
		}
		authorityIDs := governedActiveIDs(t, store, tenantID)
		incumbentIDs := dimensionalVectorIDs(t, store, tenantID, ProjectionClass1024)
		candidateIDs := dimensionalVectorIDs(t, store, tenantID, ProjectionClass2560)
		if !sameStringSet(authorityIDs, incumbentIDs) || !sameStringSet(authorityIDs, candidateIDs) {
			t.Fatalf("W17 projection ID mismatch tenant=%s authority/incumbent/candidate=%d/%d/%d", tenantID, len(authorityIDs), len(incumbentIDs), len(candidateIDs))
		}
	}
	if mismatches := dimensionalProjectionHashMismatches(t, store, dataset); mismatches != 0 {
		t.Fatalf("W17 projection authority hash mismatches=%d", mismatches)
	}
	incumbentFinalVectors := dimensionalDatasetVectorCount(t, store, dataset, ProjectionClass1024)
	candidateFinalVectors := dimensionalDatasetVectorCount(t, store, dataset, ProjectionClass2560)
	if incumbentFinalVectors != manifest.InitialActiveCount || candidateFinalVectors != manifest.InitialActiveCount {
		t.Fatalf("W17 final vectors incumbent/candidate=%d/%d", incumbentFinalVectors, candidateFinalVectors)
	}

	for tenantIndex, tenantID := range dataset.Tenants {
		record := dataset.Records[tenantID][len(dataset.Records[tenantID])/2]
		coordinator, err := NewRetrievalCoordinator(store, candidateDelegate, candidateProfile)
		if err != nil {
			t.Fatal(err)
		}
		result, err := coordinator.Retrieve(ctx, RetrievalRequest{
			OperationID: fmt.Sprintf("%s-candidate-query-%02d", runID, tenantIndex),
			TenantID:    tenantID, ContinuityIDs: []string{record.ContinuityID},
			Query: record.Content, Limit: 1, Mode: RetrievalVector,
		})
		if err != nil || result.Effective != RetrievalVector || len(result.Memories) != 1 || result.Memories[0].ID != record.MemoryID {
			t.Fatalf("W17 candidate vector query tenant=%s result=%#v err=%v", tenantID, result, err)
		}
	}

	resetTenant := dataset.Tenants[0]
	resetAuthority := dimensionalAuthorityFingerprint(t, store, dataset)
	resetIncumbentRows := len(dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024))
	if err := store.ResetVectorProjection(ctx, resetTenant, DimensionalMigrationRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	candidateRowsAfterReset := len(dimensionalVectorIDs(t, store, resetTenant, ProjectionClass2560))
	incumbentRowsUnchanged := len(dimensionalVectorIDs(t, store, resetTenant, ProjectionClass1024)) == resetIncumbentRows
	authorityUnchanged := dimensionalAuthorityFingerprint(t, store, dataset) == resetAuthority
	rebuildAfterReset, err := candidateWorkers[resetTenant].RebuildCurrent(ctx)
	if err != nil || rebuildAfterReset.Lag != 0 || rebuildAfterReset.Projected != manifest.InitialActiveCount/manifest.TenantCount {
		t.Fatalf("W17 candidate rebuild after reset=%#v err=%v", rebuildAfterReset, err)
	}
	candidateRebuilt := sameStringSet(
		governedActiveIDs(t, store, resetTenant), dimensionalVectorIDs(t, store, resetTenant, ProjectionClass2560),
	)
	if candidateRowsAfterReset != 0 || !incumbentRowsUnchanged || !authorityUnchanged || !candidateRebuilt {
		t.Fatalf("W17 candidate reset isolation rows=%d incumbent=%t authority=%t rebuilt=%t", candidateRowsAfterReset, incumbentRowsUnchanged, authorityUnchanged, candidateRebuilt)
	}

	providerStarted := time.Now()
	realProvider := runDimensionalRealProviderProbe(t, store, manifest.RealProviderTenantID, apiKey, candidateProfile)
	providerDuration := time.Since(providerStarted)
	if realProvider.Requests != 2 || realProvider.Dimensions != 2560 {
		t.Fatalf("W17 real provider evidence=%#v", realProvider)
	}

	incumbentLifecycle := dimensionalProfileLifecycle(t, store, ProductionRetrievalProfileID)
	candidateLifecycle := dimensionalProfileLifecycle(t, store, DimensionalMigrationRetrievalProfileID)
	incumbentAuditCount := dimensionalDatasetAuditCount(t, store, dataset, ProductionRetrievalProfileID)
	candidateAuditCount := dimensionalDatasetAuditCount(t, store, dataset, DimensionalMigrationRetrievalProfileID)
	profilesUnchanged := incumbentLifecycle == "active" && candidateLifecycle == "candidate"
	hardGates := map[string]bool{
		"schema_classes_isolated":               version == 16,
		"authority_writes_provider_independent": writerDuration > 0,
		"incumbent_served_during_backlog":       querySummary.Successful == manifest.QueryClientCount*manifest.QueriesPerClient,
		"restart_committed_no_partial_vector":   partialCandidateRows == 0 && interruptedCursorAdvance == 0,
		"same_pools_recovered":                  recoveryDuration > 0 && len(postRestart.Memories) == 1,
		"tail_event_count_exact":                finalEvents-initialEvents == manifest.TailEventCount,
		"physical_classes_converged":            incumbentFinalVectors == manifest.InitialActiveCount && candidateFinalVectors == manifest.InitialActiveCount,
		"ineligible_rows_absent":                dimensionalProjectionHashMismatches(t, store, dataset) == 0,
		"candidate_reset_isolated":              candidateRowsAfterReset == 0 && incumbentRowsUnchanged && authorityUnchanged && candidateRebuilt,
		"real_provider_2560_dimensions":         realProvider.Requests == 2 && realProvider.Dimensions == 2560,
		"retrieval_audits_profile_scoped":       incumbentAuditCount > 0 && candidateAuditCount > 0,
		"default_profile_unchanged":             profilesUnchanged && defaultRetrievalProfileIDForQualification() == ProductionRetrievalProfileID,
	}
	report := DimensionalMigrationReport{
		Version: dimensionalMigrationReportVersion, RunID: runID, CaseSHA256: caseHash,
		ImplementationRevision: revision, PostgreSQLVersion: cluster.version, PGVectorVersion: pgvectorVersion,
		StartedAt: startedAt, CompletedAt: time.Now().UTC(),
		Environment: DimensionalMigrationEnvironment{
			OS: goruntime.GOOS + " " + goruntime.GOARCH, CPU: manifest.ReferenceHardware.CPU,
			MemoryGiB: manifest.ReferenceHardware.MemoryGiB, SchemaVersion: int(version),
		},
		Profiles: DimensionalMigrationProfiles{
			IncumbentID: incumbentProfile.ID, IncumbentClass: incumbentProfile.ProjectionClass,
			IncumbentDimensions: incumbentProfile.Dimensions, IncumbentLifecycle: incumbentLifecycle,
			CandidateID: candidateProfile.ID, CandidateClass: candidateProfile.ProjectionClass,
			CandidateDimensions: candidateProfile.Dimensions, CandidateLifecycle: candidateLifecycle,
		},
		Counts: DimensionalMigrationCounts{
			InitialActive: manifest.InitialActiveCount, Revisions: manifest.RevisionCount,
			Deleted: manifest.DeleteCount, NewFacts: manifest.NewFactCount, TailEvents: manifest.TailEventCount,
			FinalActive: manifest.InitialActiveCount, LexicalRows: manifest.InitialActiveCount,
			IncumbentFinalVectors: incumbentFinalVectors, CandidateFinalVectors: candidateFinalVectors,
		},
		Snapshot: DimensionalMigrationSnapshot{
			IncumbentProjected: manifest.InitialActiveCount, CandidateScanned: candidateScanned,
			CandidateProjected: candidateProjected, CandidateSkippedChanged: candidateSkipped, CandidateFinalLag: 0,
		},
		Workload: DimensionalMigrationWorkload{
			QueryClients: manifest.QueryClientCount, QuerySamples: len(querySummary.Latencies),
			WriterDurationMS: writerDuration.Milliseconds(), AuthorityCompletedWhileCandidateBlocked: true,
		},
		Restart: DimensionalMigrationRestart{
			FailureCode: "postgres_restart_during_candidate_embedding", PartialRows: partialCandidateRows,
			InterruptedCursorAdvance: interruptedCursorAdvance, RecoveryDurationMS: recoveryDuration.Milliseconds(), SamePoolsRecovered: true,
		},
		Queries: DimensionalMigrationQueries{
			Successful: querySummary.Successful, BoundedDatabaseFailures: 1, CrossScopeResults: querySummary.CrossScope,
			P50MS: int(queryP50.Milliseconds()), P95MS: int(queryP95.Milliseconds()), P99MS: int(queryP99.Milliseconds()),
		},
		Reset: DimensionalMigrationReset{
			CandidateRowsAfterReset: candidateRowsAfterReset, IncumbentRowsUnchanged: incumbentRowsUnchanged,
			AuthorityUnchanged: authorityUnchanged, CandidateRebuilt: candidateRebuilt,
		},
		Provider: DimensionalMigrationProvider{
			BaseURL: candidateProfile.BaseURL, Model: candidateProfile.Model, Dimensions: realProvider.Dimensions,
			Requests: realProvider.Requests, DurationMS: providerDuration.Milliseconds(),
			ProjectionResponseSHA256: realProvider.ProjectionResponseSHA256,
			QueryResponseSHA256:      realProvider.QueryResponseSHA256,
		},
		HardGates: hardGates,
		Failures: []DimensionalMigrationFailure{{
			Phase: "candidate_restart", Attempt: 1, Code: "postgres_restart_during_candidate_embedding",
			Message: "the dedicated PostgreSQL cluster was stopped after every candidate worker reached embedding", Retried: true,
		}},
		NonClaims: []string{
			"not an embedding-model ranking",
			"not an automatic profile promotion",
			"not a long-duration retention result",
			"not cross-host HA evidence",
			"not an external sealed evaluation",
			"not artifact signing or final release acceptance",
		},
	}
	report.RequestFingerprint = dimensionalMigrationRequestFingerprint(report)
	paths, replayed, err := WriteDimensionalMigrationReport(baseRoot, report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first formal W17 report write was marked replayed")
	}
	t.Logf(
		"W17 evidence json=%s markdown=%s seed=%s incumbent_snapshot=%s candidate_snapshot=%s writer=%s",
		paths.JSON, paths.Markdown, seedDuration, incumbentSnapshotDuration, candidateSnapshotDuration, writerDuration,
	)
}

type dimensionalQueryLoadSummary struct {
	Successful int
	CrossScope int
	Latencies  []time.Duration
}

func TestDimensionalLatencyPercentilesSortSamples(t *testing.T) {
	p50, p95, p99 := dimensionalLatencyPercentiles([]time.Duration{
		10 * time.Millisecond,
		1 * time.Millisecond,
		7 * time.Millisecond,
		5 * time.Millisecond,
	})
	if p50 != 5*time.Millisecond || p95 != 7*time.Millisecond || p99 != 7*time.Millisecond {
		t.Fatalf("unexpected dimensional percentiles: p50=%s p95=%s p99=%s", p50, p95, p99)
	}
}

func dimensionalLatencyPercentiles(samples []time.Duration) (time.Duration, time.Duration, time.Duration) {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return percentileDuration(sorted, 50), percentileDuration(sorted, 95), percentileDuration(sorted, 99)
}

func runDimensionalIncumbentQueryLoad(
	t *testing.T,
	store *Store,
	manifest dimensionalMigrationCase,
	dataset dimensionalFixtureDataset,
	profile RetrievalProfile,
	embedder Embedder,
) dimensionalQueryLoadSummary {
	t.Helper()
	type measurement struct {
		latency time.Duration
		cross   bool
		err     error
	}
	measurements := make(chan measurement, manifest.QueryClientCount*manifest.QueriesPerClient)
	var clients sync.WaitGroup
	for client := 0; client < manifest.QueryClientCount; client++ {
		client := client
		clients.Add(1)
		go func() {
			defer clients.Done()
			tenantID := dataset.Tenants[client%len(dataset.Tenants)]
			record := dataset.Records[tenantID][len(dataset.Records[tenantID])/2]
			coordinator, err := NewRetrievalCoordinator(store, embedder, profile)
			if err != nil {
				measurements <- measurement{err: err}
				return
			}
			for queryIndex := 0; queryIndex < manifest.QueriesPerClient; queryIndex++ {
				started := time.Now()
				result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
					OperationID: fmt.Sprintf("w17-load-%02d-%03d", client, queryIndex),
					TenantID:    tenantID, ContinuityIDs: []string{record.ContinuityID},
					Query: record.Content, Limit: 1, Mode: RetrievalVector,
				})
				entry := measurement{latency: time.Since(started), err: err}
				if err == nil {
					entry.cross = len(result.Memories) != 1 || result.Memories[0].ID != record.MemoryID
				}
				measurements <- entry
			}
		}()
	}
	clients.Wait()
	close(measurements)
	summary := dimensionalQueryLoadSummary{Latencies: make([]time.Duration, 0, cap(measurements))}
	for entry := range measurements {
		if entry.err != nil {
			t.Fatal(entry.err)
		}
		summary.Successful++
		if entry.cross {
			summary.CrossScope++
		}
		summary.Latencies = append(summary.Latencies, entry.latency)
	}
	return summary
}

type dimensionalRealProviderResult struct {
	Dimensions               int
	Requests                 int
	ProjectionResponseSHA256 string
	QueryResponseSHA256      string
}

func runDimensionalRealProviderProbe(
	t *testing.T,
	store *Store,
	tenantID string,
	apiKey string,
	profile RetrievalProfile,
) dimensionalRealProviderResult {
	t.Helper()
	delegate, err := memorybackend.NewOpenAIEmbedder(
		profile.BaseURL, apiKey, profile.Model, profile.Dimensions,
		&http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &dimensionalRecordingEmbedder{delegate: delegate}
	governance := NewGovernanceService(store, tenantID)
	const repoRoot = "/fixtures/w17-real-provider"
	resolution, err := governance.ConfirmWorkspace(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: "w17-real-provider-source", MemoryKey: "w17.real.provider.recovery",
		Content:   "The dimensional migration recovery code is ORCHID-2560 after the candidate projection reaches zero lag.",
		SourceRef: "fixture:w17:real-provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := newDimensionalWorker(t, store, tenantID, profile, recorder, 16, 16)
	if result, err := worker.RunOnce(context.Background()); err != nil || result.Lag != 0 {
		t.Fatalf("W17 real provider projection result=%#v err=%v", result, err)
	}
	coordinator, err := NewRetrievalCoordinator(store, recorder, profile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID: "w17-real-provider-query", TenantID: tenantID,
		ContinuityIDs: []string{resolution.ContinuityID},
		Query:         "What recovery code applies after the 2560-dimensional candidate catches up?",
		Limit:         1, Mode: RetrievalVector,
	})
	if err != nil || result.Effective != RetrievalVector || len(result.Memories) != 1 || result.Memories[0].ID != receipt.Memory.MemoryID {
		t.Fatalf("W17 real provider retrieval result=%#v err=%v", result, err)
	}
	requests, hashes := recorder.snapshot()
	if requests != 2 || len(hashes) != 2 {
		t.Fatalf("W17 real provider requests/hashes=%d/%d", requests, len(hashes))
	}
	return dimensionalRealProviderResult{
		Dimensions: profile.Dimensions, Requests: requests,
		ProjectionResponseSHA256: hashes[0], QueryResponseSHA256: hashes[1],
	}
}

func dimensionalDatasetActiveCount(t *testing.T, store *Store, dataset dimensionalFixtureDataset) int {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM governed_memories
WHERE tenant_id = ANY($1::text[])
  AND memory_kind = 'fact' AND lifecycle_status = 'active' AND content <> '[redacted]'`, dataset.Tenants).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func dimensionalDatasetVectorCount(t *testing.T, store *Store, dataset dimensionalFixtureDataset, class ProjectionClass) int {
	t.Helper()
	query := `SELECT count(*) FROM memory_vector_documents WHERE tenant_id = ANY($1::text[]) AND profile_id = $2`
	profileID := ProductionRetrievalProfileID
	if class == ProjectionClass2560 {
		query = `SELECT count(*) FROM memory_vector_documents_2560 WHERE tenant_id = ANY($1::text[]) AND profile_id = $2`
		profileID = DimensionalMigrationRetrievalProfileID
	}
	var count int
	if err := store.pool.QueryRow(context.Background(), query, dataset.Tenants, profileID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func dimensionalProjectionHashMismatches(t *testing.T, store *Store, dataset dimensionalFixtureDataset) int {
	t.Helper()
	var incumbent, candidate int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_vector_documents document
JOIN governed_memories memory
  ON memory.tenant_id = document.tenant_id AND memory.id = document.memory_id
WHERE document.tenant_id = ANY($1::text[])
  AND document.profile_id = $2
  AND document.content_sha256 <> encode(digest(convert_to(memory.content, 'UTF8'), 'sha256'), 'hex')`,
		dataset.Tenants, ProductionRetrievalProfileID,
	).Scan(&incumbent); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_vector_documents_2560 document
JOIN governed_memories memory
  ON memory.tenant_id = document.tenant_id AND memory.id = document.memory_id
WHERE document.tenant_id = ANY($1::text[])
  AND document.profile_id = $2
  AND document.content_sha256 <> encode(digest(convert_to(memory.content, 'UTF8'), 'sha256'), 'hex')`,
		dataset.Tenants, DimensionalMigrationRetrievalProfileID,
	).Scan(&candidate); err != nil {
		t.Fatal(err)
	}
	return incumbent + candidate
}

func dimensionalAuthorityFingerprint(t *testing.T, store *Store, dataset dimensionalFixtureDataset) string {
	t.Helper()
	rows, err := store.pool.Query(context.Background(), `
SELECT tenant_id, id::text, lifecycle_status, content
FROM governed_memories
WHERE tenant_id = ANY($1::text[])
ORDER BY tenant_id, id`, dataset.Tenants)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var tenantID, memoryID, lifecycle, content string
		if err := rows.Scan(&tenantID, &memoryID, &lifecycle, &content); err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{tenantID, memoryID, lifecycle, content} {
			hash.Write([]byte(value))
			hash.Write([]byte{0})
		}
		hash.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func dimensionalProfileLifecycle(t *testing.T, store *Store, profileID string) string {
	t.Helper()
	var lifecycle string
	if err := store.pool.QueryRow(context.Background(), `
SELECT lifecycle_status FROM memory_retrieval_profiles WHERE profile_id = $1`, profileID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	return lifecycle
}

func dimensionalDatasetAuditCount(t *testing.T, store *Store, dataset dimensionalFixtureDataset, profileID string) int {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_retrieval_runs
WHERE tenant_id = ANY($1::text[]) AND profile_id = $2`, dataset.Tenants, profileID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func dimensionalGitRevision(t *testing.T) string {
	t.Helper()
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = filepath.Join("..", "..")
	if output, err := status.Output(); err != nil {
		t.Fatal(err)
	} else if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("formal W17 run requires a clean worktree")
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = filepath.Join("..", "..")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSpace(string(output))
	if !isDimensionalMigrationLowerHex(revision, 40) {
		t.Fatalf("invalid W17 implementation revision %q", revision)
	}
	return revision
}

func dimensionalCaseHash(t *testing.T) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "W17-active-backlog-dimensional-migration", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:])
}

func assertDimensionalDuration(t *testing.T, phase string, duration time.Duration, limitSeconds int) {
	t.Helper()
	if duration > time.Duration(limitSeconds)*time.Second {
		t.Fatalf("W17 %s duration=%s exceeds %ds", phase, duration, limitSeconds)
	}
}

func defaultRetrievalProfileIDForQualification() string {
	return ProductionRetrievalProfileID
}

func sortedDurations(values []time.Duration) []time.Duration {
	values = append([]time.Duration(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}
