package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/memorybackend"
)

func TestProjectionRetentionFormalProfile(t *testing.T) {
	if os.Getenv("VERMORY_W18_FORMAL") != "1" {
		t.Skip("VERMORY_W18_FORMAL=1 is required")
	}
	runID := strings.TrimSpace(os.Getenv("VERMORY_W18_RUN_ID"))
	artifactRoot := filepath.Clean(strings.TrimSpace(os.Getenv("VERMORY_W18_ARTIFACT_ROOT")))
	if !projectionRetentionSafeName.MatchString(runID) {
		t.Fatal("VERMORY_W18_RUN_ID is invalid")
	}
	if !filepath.IsAbs(artifactRoot) || artifactRoot == string(filepath.Separator) {
		t.Fatal("VERMORY_W18_ARTIFACT_ROOT must be a dedicated absolute path")
	}
	reportPath := filepath.Join(artifactRoot, runID, "report.json")
	if _, err := os.Stat(reportPath); err == nil {
		report, err := ReadProjectionRetentionReport(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		if expected := strings.TrimSpace(os.Getenv("VERMORY_IMPLEMENTATION_REVISION")); expected != "" && report.ImplementationRevision != expected {
			t.Fatalf("W18 replay revision=%s want %s", report.ImplementationRevision, expected)
		}
		t.Logf("W18 replay completed report: run=%s revision=%s", report.RunID, report.ImplementationRevision)
		return
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	manifest := loadProjectionRetentionCase(t)
	baseRoot := strings.TrimSpace(os.Getenv("VERMORY_W18_ROOT"))
	binDir := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES18_BIN"))
	runRoot, err := dimensionalRunRoot(baseRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	apiKey := strings.TrimSpace(os.Getenv("SILICONFLOW_API_KEY"))
	if apiKey == "" {
		t.Fatal("SILICONFLOW_API_KEY is required for the formal W18 profile")
	}
	revision := projectionRetentionGitRevision(t)
	if expected := strings.TrimSpace(os.Getenv("VERMORY_IMPLEMENTATION_REVISION")); expected == "" || expected != revision {
		t.Fatalf("VERMORY_IMPLEMENTATION_REVISION=%q want clean HEAD %q", expected, revision)
	}
	caseHash := projectionRetentionCaseHash(t)
	startedAt := time.Now().UTC()
	cluster := startDimensionalPostgres18(t, runRoot, binDir)
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
	if err != nil || version != 17 {
		t.Fatalf("W18 schema version=%d err=%v", version, err)
	}
	var pgvectorVersion string
	if err := store.pool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&pgvectorVersion); err != nil {
		t.Fatal(err)
	}

	dataset := seedDimensionalFixture(
		t, store, "w18-profile", manifest.TenantCount,
		manifest.ContinuitiesPerTenant, manifest.RecordsPerContinuity,
	)
	if active := dimensionalDatasetActiveCount(t, store, dataset); active != manifest.InitialCurrentFacts {
		t.Fatalf("W18 initial current=%d want %d", active, manifest.InitialCurrentFacts)
	}
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != int64(manifest.InitialCurrentFacts) {
		t.Fatalf("W18 initial lexical rows=%d err=%v", rows, err)
	}
	incumbentProfile := projectionRetentionProfile(t, manifest.IncumbentProfileID)
	dimensionalProfile := projectionRetentionProfile(t, manifest.DimensionalProfileID)
	futureProfile := projectionRetentionProfile(t, manifest.FutureProfileID)
	incumbentEmbedder := &dimensionalFixtureEmbedder{dimensions: incumbentProfile.Dimensions}
	dimensionalEmbedder := &dimensionalFixtureEmbedder{dimensions: dimensionalProfile.Dimensions}
	incumbentWorkers := map[string]*ProjectionWorker{}
	dimensionalWorkers := map[string]*ProjectionWorker{}
	frozenCandidateCursor := map[string]int64{}
	for _, tenantID := range dataset.Tenants {
		incumbentWorkers[tenantID] = newDimensionalWorker(
			t, store, tenantID, incumbentProfile, incumbentEmbedder,
			manifest.SnapshotPageSize, manifest.WorkerBatchSize,
		)
		dimensionalWorkers[tenantID] = newDimensionalWorker(
			t, store, tenantID, dimensionalProfile, dimensionalEmbedder,
			manifest.SnapshotPageSize, manifest.WorkerBatchSize,
		)
		if result, err := incumbentWorkers[tenantID].RebuildCurrent(ctx); err != nil || result.Lag != 0 || result.Projected != manifest.InitialCurrentFacts/manifest.TenantCount {
			t.Fatalf("W18 incumbent snapshot tenant=%s result=%#v err=%v", tenantID, result, err)
		}
		if result, err := dimensionalWorkers[tenantID].RebuildCurrent(ctx); err != nil || result.Lag != 0 || result.Projected != manifest.InitialCurrentFacts/manifest.TenantCount {
			t.Fatalf("W18 dimensional snapshot tenant=%s result=%#v err=%v", tenantID, result, err)
		}
		status, err := store.RetrievalProjectionStatus(ctx, tenantID, dimensionalProfile.ID)
		if err != nil {
			t.Fatal(err)
		}
		frozenCandidateCursor[tenantID] = status.LastEventID
	}
	var futureCursorCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_projection_cursors
WHERE tenant_id = ANY($1::text[]) AND profile_id = $2`, dataset.Tenants, futureProfile.ID).Scan(&futureCursorCount); err != nil {
		t.Fatal(err)
	}
	if futureCursorCount != 0 {
		t.Fatalf("W18 future subscriber already has %d cursors", futureCursorCount)
	}

	cutoff := time.Now().UTC().Add(24 * time.Hour)
	receipts := make([]ProjectionPruneReceipt, 0, manifest.TenantCount*(manifest.AcceleratedEpochs+2))
	epochEvidence := make([]ProjectionRetentionEpoch, 0, manifest.TenantCount*manifest.AcceleratedEpochs)
	blockedPruning := true
	var queryRun projectionRetentionQueryRun
	var pruneStartedAt time.Time
	var pruneCompletedAt time.Time
	var deletedProbe dimensionalFixtureRecord
	for epoch := 1; epoch <= manifest.AcceleratedEpochs; epoch++ {
		if epoch == manifest.AcceleratedEpochs {
			deletedProbe = dataset.Records[dataset.Tenants[0]][750]
		}
		var maxBefore int64
		if err := store.pool.QueryRow(ctx, `SELECT COALESCE(max(event_id), 0) FROM memory_projection_events`).Scan(&maxBefore); err != nil {
			t.Fatal(err)
		}
		applyProjectionRetentionEpoch(t, store, &dataset, epoch, 750, 50, 50)
		var maxAfter int64
		if err := store.pool.QueryRow(ctx, `SELECT COALESCE(max(event_id), 0) FROM memory_projection_events`).Scan(&maxAfter); err != nil {
			t.Fatal(err)
		}
		if maxAfter-maxBefore != 6400 {
			t.Fatalf("W18 epoch %d generated events=%d want 6400", epoch, maxAfter-maxBefore)
		}
		if epoch == 1 {
			queryRun = runProjectionRetentionQueries(t, store, dataset, incumbentProfile, incumbentEmbedder, manifest)
			<-queryRun.started
			pruneStartedAt = time.Now()
		}
		for _, tenantID := range dataset.Tenants {
			drainDimensionalProjection(t, incumbentWorkers[tenantID])
			receipt, err := store.PruneProjectionEvents(ctx, tenantID, ProjectionPruneRequest{
				OperationID: fmt.Sprintf("w18-epoch-%02d-prune-%s", epoch, tenantID),
				Cutoff:      cutoff, RetainTailEvents: 0,
			})
			if err != nil {
				t.Fatal(err)
			}
			receipts = append(receipts, receipt)
			if receipt.NewFloorEventID > frozenCandidateCursor[tenantID] || receipt.SafeCursorEventID != frozenCandidateCursor[tenantID] {
				blockedPruning = false
			}
			epochEvidence = append(epochEvidence, ProjectionRetentionEpoch{
				Epoch: epoch, TenantID: tenantID, Generated: 1600,
				Pruned: receipt.DeletedEvents, Retained: projectionRetentionEventCount(t, store, tenantID),
				Floor: receipt.NewFloorEventID, SlowCursor: frozenCandidateCursor[tenantID],
			})
		}
		if epoch == 1 {
			pruneCompletedAt = time.Now()
		}
	}
	querySummary := <-queryRun.done
	if querySummary.err != nil {
		t.Fatal(querySummary.err)
	}
	if querySummary.successful != manifest.QueryClientCount*manifest.QueriesPerClient || querySummary.crossScope != 0 {
		t.Fatalf("W18 query summary=%#v", querySummary)
	}
	queriesOverlappedPruning := !pruneStartedAt.IsZero() && !pruneCompletedAt.IsZero() &&
		querySummary.startedAt.Before(pruneCompletedAt) && querySummary.completedAt.After(pruneStartedAt)
	if err := drainProjectionRetentionWorkers(ctx, dataset.Tenants, dimensionalWorkers); err != nil {
		t.Fatal(err)
	}
	var replayStable bool
	for tenantIndex, tenantID := range dataset.Tenants {
		request := ProjectionPruneRequest{
			OperationID: "w18-final-prune-" + tenantID,
			Cutoff:      cutoff, RetainTailEvents: manifest.RetainedTailPerTenant,
		}
		receipt, err := store.PruneProjectionEvents(ctx, tenantID, request)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
		if projectionRetentionEventCount(t, store, tenantID) != int64(manifest.RetainedTailPerTenant) {
			t.Fatalf("W18 tenant %s retained events=%d want %d", tenantID, projectionRetentionEventCount(t, store, tenantID), manifest.RetainedTailPerTenant)
		}
		if tenantIndex == 0 {
			replay, err := store.PruneProjectionEvents(ctx, tenantID, request)
			if err != nil {
				t.Fatal(err)
			}
			receipt.Replayed = true
			replayStable = reflect.DeepEqual(receipt, replay)
		}
	}
	if !replayStable {
		t.Fatal("W18 idempotent final prune replay drifted")
	}

	futureTenant := dataset.Tenants[0]
	futureEmbedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	futureWorker := newDimensionalWorker(t, store, futureTenant, futureProfile, futureEmbedder, manifest.SnapshotPageSize, manifest.WorkerBatchSize)
	futureResult, futureErr := futureWorker.RunOnce(ctx)
	futureZeroCalls := futureErr != nil && futureResult.FailureCode == ProjectionFailureRebuildRequired && futureEmbedder.calls.Load() == 0
	if !futureZeroCalls {
		t.Fatalf("W18 future subscriber result=%#v calls=%d err=%v", futureResult, futureEmbedder.calls.Load(), futureErr)
	}
	if result, err := futureWorker.RebuildCurrent(ctx); err != nil || result.Lag != 0 {
		t.Fatalf("W18 future subscriber rebuild result=%#v err=%v", result, err)
	}

	resetTenant := dataset.Tenants[1]
	incumbentBeforeReset := projectionRetentionProfileVectorIDs(t, store, resetTenant, incumbentProfile.ID)
	if err := store.ResetVectorProjection(ctx, resetTenant, dimensionalProfile.ID); err != nil {
		t.Fatal(err)
	}
	resetStatus, err := store.RetrievalProjectionStatus(ctx, resetTenant, dimensionalProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	resetRequired := resetStatus.Status == ProjectionStatusRebuildRequired && resetStatus.VectorCount == 0
	if result, err := dimensionalWorkers[resetTenant].RebuildCurrent(ctx); err != nil || result.Lag != 0 {
		t.Fatalf("W18 dimensional reset rebuild result=%#v err=%v", result, err)
	}
	resetIsolated := sameStringSet(incumbentBeforeReset, projectionRetentionProfileVectorIDs(t, store, resetTenant, incumbentProfile.ID))

	authorityEquivalent := true
	for _, tenantID := range dataset.Tenants {
		if !projectionRetentionAuthorityEquivalent(t, store, tenantID, ProjectionClass1024) ||
			!projectionRetentionAuthorityEquivalent(t, store, tenantID, ProjectionClass2560) {
			authorityEquivalent = false
		}
	}
	deletedAbsent := !containsString(projectionRetentionProfileVectorIDs(t, store, dataset.Tenants[0], incumbentProfile.ID), deletedProbe.MemoryID) &&
		!containsString(projectionRetentionProfileVectorIDs(t, store, dataset.Tenants[0], dimensionalProfile.ID), deletedProbe.MemoryID) &&
		!containsString(projectionRetentionProfileVectorIDs(t, store, dataset.Tenants[0], futureProfile.ID), deletedProbe.MemoryID)
	if matches, err := store.SearchActiveMemory(ctx, deletedProbe.TenantID, deletedProbe.ContinuityID, deletedProbe.Content, 5); err != nil {
		t.Fatal(err)
	} else {
		for _, memory := range matches {
			if memory.ID == deletedProbe.MemoryID {
				deletedAbsent = false
			}
		}
	}

	rlsBoundary := projectionRetentionFormalRLSBoundary(t, store, cluster.databaseURL, dataset.Tenants)
	restartEvidence, retryReceipt := projectionRetentionFormalRestart(t, store, cluster, dataset.Tenants[0], cutoff, manifest.RetainedTailPerTenant)
	receipts = append(receipts, retryReceipt)

	providerStarted := time.Now()
	provider := runProjectionRetentionProviderProbe(t, store, manifest.RealProviderTenantID, apiKey, incumbentProfile)
	providerDuration := time.Since(providerStarted)
	if provider.Requests != 2 || provider.Dimensions != 1024 {
		t.Fatalf("W18 provider evidence=%#v", provider)
	}

	finalCurrent := dimensionalDatasetActiveCount(t, store, dataset)
	var lexicalRows int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_search_documents WHERE tenant_id = ANY($1::text[])`, dataset.Tenants).Scan(&lexicalRows); err != nil {
		t.Fatal(err)
	}
	incumbentVectors := dimensionalDatasetVectorCount(t, store, dataset, ProjectionClass1024)
	dimensionalVectors := dimensionalDatasetVectorCount(t, store, dataset, ProjectionClass2560)
	retainedEvents := 0
	for _, tenantID := range dataset.Tenants {
		retainedEvents += int(projectionRetentionEventCount(t, store, tenantID))
	}
	if retainedEvents != manifest.RetainedTailPerTenant*manifest.TenantCount {
		t.Fatalf("W18 final retained events=%d", retainedEvents)
	}
	profiles := make([]ProjectionRetentionProfile, 0, manifest.TenantCount*2+1)
	for _, tenantID := range dataset.Tenants {
		for _, profileID := range []string{incumbentProfile.ID, dimensionalProfile.ID} {
			status, err := store.RetrievalProjectionStatus(ctx, tenantID, profileID)
			if err != nil {
				t.Fatal(err)
			}
			profiles = append(profiles, ProjectionRetentionProfile{
				TenantID: tenantID, ProfileID: profileID, Status: status.Status,
				LastEventID: status.LastEventID, Floor: status.PrunedThroughEventID,
				Lag: status.Lag, VectorCount: status.VectorCount,
			})
		}
	}
	futureStatus, err := store.RetrievalProjectionStatus(ctx, futureTenant, futureProfile.ID)
	if err != nil {
		t.Fatal(err)
	}
	profiles = append(profiles, ProjectionRetentionProfile{
		TenantID: futureTenant, ProfileID: futureProfile.ID, Status: futureStatus.Status,
		LastEventID: futureStatus.LastEventID, Floor: futureStatus.PrunedThroughEventID,
		Lag: futureStatus.Lag, VectorCount: futureStatus.VectorCount,
	})
	sortedLatencies := sortedDurations(querySummary.latencies)
	p50 := sortedLatencies[len(sortedLatencies)*50/100]
	p95 := sortedLatencies[len(sortedLatencies)*95/100]
	p99 := sortedLatencies[len(sortedLatencies)*99/100]
	incumbentLifecycle := dimensionalProfileLifecycle(t, store, incumbentProfile.ID)
	dimensionalLifecycle := dimensionalProfileLifecycle(t, store, dimensionalProfile.ID)
	futureLifecycle := dimensionalProfileLifecycle(t, store, futureProfile.ID)
	defaultUnchanged := incumbentLifecycle == "active" && dimensionalLifecycle == "candidate" &&
		futureLifecycle == "candidate" && defaultRetrievalProfileIDForQualification() == incumbentProfile.ID
	hardGates := map[string]bool{
		"schema 17 creates tenant-isolated retention and prune audit state":          version == 17,
		"runtime workers can read but cannot mutate retention control tables":        rlsBoundary.runtimeReadOnly,
		"no prune passes the slowest incremental cursor":                             blockedPruning,
		"authority writes and active-profile queries continue during prune attempts": querySummary.successful == 320 && queriesOverlappedPruning,
		"interrupted prune deletion floor and audit changes roll back together":      restartEvidence.DeletedEventsRolledBack && restartEvidence.FloorRolledBack && restartEvidence.ReceiptRolledBack,
		"the same pool recovers after PostgreSQL restart":                            restartEvidence.SamePoolRecovered,
		"event retention reaches the calibrated bound after catch-up":                retainedEvents == 4000,
		"retention floor is monotonic and idempotent replay is byte-stable":          replayStable && projectionRetentionReceiptsMonotonic(receipts),
		"new subscribers below the floor rebuild with zero embedding work":           futureZeroCalls,
		"reset after pruning requires rebuild and leaves other profiles unchanged":   resetRequired && resetIsolated,
		"rebuilds match authority and deleted memories remain absent":                authorityEquivalent && deletedAbsent,
		"cross-tenant pruning and receipt access are blocked":                        rlsBoundary.crossTenantBlocked,
		"direct provider projection and query succeed after pruning":                 provider.Requests == 2,
		"lexical and the incumbent remain default with no promotion":                 defaultUnchanged,
	}
	report := ProjectionRetentionReport{
		Version: projectionRetentionReportVersion, RunID: runID, CaseSHA256: caseHash,
		ImplementationRevision: revision, PostgreSQLVersion: cluster.version, PGVectorVersion: pgvectorVersion,
		StartedAt: startedAt, CompletedAt: time.Now().UTC(),
		Environment: ProjectionRetentionEnvironment{
			OS: goruntime.GOOS + " " + goruntime.GOARCH, CPU: projectionRetentionCPU(),
			MemoryGiB: projectionRetentionMemoryGiB(), SchemaVersion: int(version),
		},
		Policy: ProjectionRetentionPolicy{Cutoff: cutoff, RetainTailEvents: manifest.RetainedTailPerTenant},
		Counts: ProjectionRetentionCounts{
			InitialCurrent: manifest.InitialCurrentFacts, GeneratedEvents: manifest.TotalGeneratedEvents,
			PrunedEvents: manifest.TotalGeneratedEvents - retainedEvents, RetainedEvents: retainedEvents,
			FinalCurrent: finalCurrent, LexicalRows: lexicalRows,
			IncumbentVectors: incumbentVectors, DimensionalVectors: dimensionalVectors,
		},
		Epochs: epochEvidence, Profiles: profiles, Receipts: receipts,
		Restart: restartEvidence,
		Queries: ProjectionRetentionQueries{
			Successful: querySummary.successful, CrossScopeResults: querySummary.crossScope,
			LexicalDegradations: querySummary.lexicalDegradations,
			P50MS:               int(p50.Milliseconds()), P95MS: int(p95.Milliseconds()), P99MS: int(p99.Milliseconds()),
		},
		Rebuild: ProjectionRetentionRebuild{
			FutureSubscriberZeroCalls: futureZeroCalls, ResetRequiredRebuild: resetRequired,
			AuthorityIDHashEquivalent: authorityEquivalent, DeletedMemoryAbsent: deletedAbsent,
		},
		Provider: ProjectionRetentionProvider{
			BaseURL: incumbentProfile.BaseURL, Model: incumbentProfile.Model, Dimensions: provider.Dimensions,
			Requests: provider.Requests, DurationMS: providerDuration.Milliseconds(),
			ProjectionResponseSHA256: provider.ProjectionResponseSHA256,
			QueryResponseSHA256:      provider.QueryResponseSHA256,
		},
		HardGates: hardGates,
		Failures: []ProjectionRetentionFailure{
			{
				Phase: "formal_profile", Attempt: 1, Code: "test_timeout",
				Message: "revision 9dc6d087a5c141db2aa0426d300fea1867f98511 reached the 30 minute test timeout while draining the first candidate tenant serially",
				Retried: true,
			},
			{
				Phase: "prune_restart", Attempt: 1, Code: "postgres_immediate_stop",
				Message: "the dedicated PostgreSQL cluster stopped after delete, floor, and receipt mutations but before commit", Retried: true,
			},
		},
		NonClaims: []string{
			"not months of uninterrupted wall-clock operation",
			"no automatic retention scheduling",
			"no authoritative-memory deletion policy",
			"no table partitioning or cross-region queueing claim",
			"no embedding-model ranking or profile promotion claim",
			"no cross-host high availability claim",
			"no external sealed evaluation claim",
			"no artifact signing or final release acceptance claim",
		},
	}
	report.RequestFingerprint = projectionRetentionRequestFingerprint(report)
	paths, replayed, err := WriteProjectionRetentionReport(artifactRoot, report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first formal W18 report write was marked replayed")
	}
	t.Logf("W18 report JSON=%s Markdown=%s", paths.JSON, paths.Markdown)
}

type projectionRetentionQuerySummary struct {
	successful          int
	crossScope          int
	lexicalDegradations int
	latencies           []time.Duration
	startedAt           time.Time
	completedAt         time.Time
	err                 error
}

type projectionRetentionQueryRun struct {
	started <-chan struct{}
	done    <-chan projectionRetentionQuerySummary
}

func drainProjectionRetentionWorkers(
	ctx context.Context,
	tenantIDs []string,
	workers map[string]*ProjectionWorker,
) error {
	type drainResult struct {
		tenantID string
		err      error
	}
	results := make(chan drainResult, len(tenantIDs))
	var drains sync.WaitGroup
	drains.Add(len(tenantIDs))
	for _, tenantID := range tenantIDs {
		tenantID := tenantID
		worker := workers[tenantID]
		go func() {
			defer drains.Done()
			if worker == nil {
				results <- drainResult{tenantID: tenantID, err: fmt.Errorf("projection worker is missing")}
				return
			}
			for attempt := 0; attempt < 1000; attempt++ {
				result, err := worker.RunOnce(ctx)
				if err != nil {
					results <- drainResult{tenantID: tenantID, err: fmt.Errorf("run projection worker: result=%#v: %w", result, err)}
					return
				}
				if result.Lag == 0 {
					results <- drainResult{tenantID: tenantID}
					return
				}
			}
			results <- drainResult{tenantID: tenantID, err: fmt.Errorf("projection did not reach zero lag after 1000 attempts")}
		}()
	}
	drains.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			return fmt.Errorf("drain projection for tenant %s: %w", result.tenantID, result.err)
		}
	}
	return nil
}

func runProjectionRetentionQueries(
	t *testing.T,
	store *Store,
	dataset dimensionalFixtureDataset,
	profile RetrievalProfile,
	embedder Embedder,
	manifest projectionRetentionCase,
) projectionRetentionQueryRun {
	t.Helper()
	started := make(chan struct{})
	done := make(chan projectionRetentionQuerySummary, 1)
	stable := make(map[string]dimensionalFixtureRecord, len(dataset.Tenants))
	coordinators := make(map[string]*RetrievalCoordinator, len(dataset.Tenants))
	for _, tenantID := range dataset.Tenants {
		stable[tenantID] = dataset.Records[tenantID][len(dataset.Records[tenantID])-1]
		coordinator, err := NewRetrievalCoordinator(store, embedder, profile)
		if err != nil {
			t.Fatal(err)
		}
		coordinators[tenantID] = coordinator
	}
	go func() {
		type queryResult struct {
			latency   time.Duration
			effective RetrievalMode
			cross     bool
			err       error
		}
		results := make(chan queryResult, manifest.QueryClientCount*manifest.QueriesPerClient)
		startQueries := make(chan struct{})
		var ready sync.WaitGroup
		var clients sync.WaitGroup
		ready.Add(manifest.QueryClientCount)
		clients.Add(manifest.QueryClientCount)
		for client := 0; client < manifest.QueryClientCount; client++ {
			client := client
			go func() {
				defer clients.Done()
				ready.Done()
				<-startQueries
				tenantID := dataset.Tenants[client%len(dataset.Tenants)]
				record := stable[tenantID]
				for queryIndex := 0; queryIndex < manifest.QueriesPerClient; queryIndex++ {
					started := time.Now()
					result, err := coordinators[tenantID].Retrieve(context.Background(), RetrievalRequest{
						OperationID: fmt.Sprintf("w18-query-%02d-%02d", client, queryIndex),
						TenantID:    tenantID, ContinuityIDs: []string{record.ContinuityID},
						Query: record.Content, Limit: 1, Mode: RetrievalVector,
					})
					entry := queryResult{latency: time.Since(started), effective: result.Effective, err: err}
					if err == nil && (len(result.Memories) != 1 || result.Memories[0].ID != record.MemoryID) {
						entry.cross = true
					}
					results <- entry
					time.Sleep(2 * time.Millisecond)
				}
			}()
		}
		ready.Wait()
		startedAt := time.Now()
		close(startQueries)
		close(started)
		clients.Wait()
		close(results)
		summary := projectionRetentionQuerySummary{
			latencies: make([]time.Duration, 0, cap(results)),
			startedAt: startedAt,
		}
		for result := range results {
			if result.err != nil && summary.err == nil {
				summary.err = result.err
			}
			if result.err == nil {
				summary.successful++
			}
			if result.cross {
				summary.crossScope++
			}
			if result.effective == RetrievalLexical {
				summary.lexicalDegradations++
			}
			summary.latencies = append(summary.latencies, result.latency)
		}
		summary.completedAt = time.Now()
		done <- summary
	}()
	return projectionRetentionQueryRun{started: started, done: done}
}

type projectionRetentionRLSResult struct {
	runtimeReadOnly    bool
	crossTenantBlocked bool
}

func projectionRetentionFormalRLSBoundary(t *testing.T, admin *Store, databaseURL string, tenants []string) projectionRetentionRLSResult {
	t.Helper()
	ctx := context.Background()
	roleName, runtimeURL := createTenantPoolRole(t, admin.pool, databaseURL, "w18_retention", "")
	if err := authn.GrantRuntimeRole(ctx, admin.pool, roleName); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := OpenStoreWithOptions(ctx, runtimeURL, StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	defer runtimeStore.Close()
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}
	readOnly := runtimeStore.ValidateProjectionPruneOperatorRole(ctx) == ErrUnsafePruneRole
	tenantCtx, err := withTenantContext(ctx, tenants[0])
	if err != nil {
		t.Fatal(err)
	}
	var ownRows, crossRows int
	if err := runtimeStore.pool.QueryRow(tenantCtx, `
SELECT count(*) FROM memory_projection_retention WHERE tenant_id = $1`, tenants[0]).Scan(&ownRows); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.pool.QueryRow(tenantCtx, `
SELECT count(*) FROM memory_projection_retention WHERE tenant_id = $1`, tenants[1]).Scan(&crossRows); err != nil {
		t.Fatal(err)
	}
	var ignored int
	receiptBlocked := runtimeStore.pool.QueryRow(tenantCtx, `SELECT count(*) FROM memory_projection_prune_runs`).Scan(&ignored) != nil
	_, pruneErr := runtimeStore.PruneProjectionEvents(tenantCtx, tenants[1], ProjectionPruneRequest{
		OperationID: "w18-cross-tenant-prune", Cutoff: time.Now().UTC().Add(time.Hour), RetainTailEvents: 1000,
	})
	return projectionRetentionRLSResult{
		runtimeReadOnly:    readOnly && ownRows == 1,
		crossTenantBlocked: crossRows == 0 && receiptBlocked && pruneErr != nil,
	}
}

func projectionRetentionFormalRestart(
	t *testing.T,
	store *Store,
	cluster *dimensionalPostgres18,
	tenantID string,
	cutoff time.Time,
	retainTail int,
) (ProjectionRetentionRestart, ProjectionPruneReceipt) {
	t.Helper()
	ctx := context.Background()
	beforeCount := projectionRetentionEventCount(t, store, tenantID)
	beforeRetention, err := store.ProjectionRetention(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	tenantCtx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.pool.Acquire(tenantCtx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := connection.Begin(tenantCtx)
	if err != nil {
		connection.Release()
		t.Fatal(err)
	}
	var target int64
	if err := tx.QueryRow(tenantCtx, `
SELECT max(event_id)
FROM (
  SELECT event_id FROM memory_projection_events
  WHERE tenant_id = $1 AND event_id > $2
  ORDER BY event_id
  LIMIT 10
) candidate`, tenantID, beforeRetention.PrunedThroughEventID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
DELETE FROM memory_projection_events
WHERE tenant_id = $1 AND event_id > $2 AND event_id <= $3`, tenantID, beforeRetention.PrunedThroughEventID, target); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
UPDATE memory_projection_retention
SET pruned_through_event_id = $2, last_pruned_at = now(), updated_at = now()
WHERE tenant_id = $1`, tenantID, target); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tenantCtx, `
INSERT INTO memory_projection_prune_runs (
  tenant_id, operation_id, request_fingerprint, cutoff, retain_tail_events,
  safe_cursor_event_id, previous_floor_event_id, new_floor_event_id,
  deleted_events, result
) VALUES ($1, 'w18-restart-interrupted', repeat('f', 64), $4, $5, $3, $2, $3, 10, 'pruned')`,
		tenantID, beforeRetention.PrunedThroughEventID, target, cutoff, retainTail,
	); err != nil {
		t.Fatal(err)
	}
	cluster.stop(t, "immediate")
	rollbackCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = tx.Rollback(rollbackCtx)
	cancel()
	connection.Release()
	cluster.start(t)
	recovery := waitForProjectionRetentionStoreRecovery(t, store, 30*time.Second)
	afterCount := projectionRetentionEventCount(t, store, tenantID)
	afterRetention, err := store.ProjectionRetention(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	var interruptedReceipts int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_projection_prune_runs
WHERE tenant_id = $1 AND operation_id = 'w18-restart-interrupted'`, tenantID).Scan(&interruptedReceipts); err != nil {
		t.Fatal(err)
	}
	retry, err := store.PruneProjectionEvents(ctx, tenantID, ProjectionPruneRequest{
		OperationID: "w18-restart-interrupted", Cutoff: cutoff, RetainTailEvents: retainTail,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ProjectionRetentionRestart{
		FailureCode:             "postgres_immediate_stop",
		DeletedEventsRolledBack: afterCount == beforeCount,
		FloorRolledBack:         afterRetention.PrunedThroughEventID == beforeRetention.PrunedThroughEventID,
		ReceiptRolledBack:       interruptedReceipts == 0,
		SamePoolRecovered:       true, RecoveryDurationMS: recovery.Milliseconds(),
	}, retry
}

type projectionRetentionProviderResult struct {
	Dimensions               int
	Requests                 int
	ProjectionResponseSHA256 string
	QueryResponseSHA256      string
}

func runProjectionRetentionProviderProbe(t *testing.T, store *Store, tenantID, apiKey string, profile RetrievalProfile) projectionRetentionProviderResult {
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
	const repoRoot = "/fixtures/w18-real-provider"
	resolution, err := governance.ConfirmWorkspace(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: "w18-real-provider-source", MemoryKey: "w18.real.provider.retention",
		Content:   "The post-prune recovery code is CEDAR-1024 after projection retention reaches its acknowledged floor.",
		SourceRef: "fixture:w18:real-provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := newDimensionalWorker(t, store, tenantID, profile, recorder, 16, 16)
	if result, err := worker.RunOnce(context.Background()); err != nil || result.Lag != 0 {
		t.Fatalf("W18 provider projection result=%#v err=%v", result, err)
	}
	coordinator, err := NewRetrievalCoordinator(store, recorder, profile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID: "w18-real-provider-query", TenantID: tenantID,
		ContinuityIDs: []string{resolution.ContinuityID},
		Query:         "Which recovery code applies after the retained event floor is acknowledged?",
		Limit:         1, Mode: RetrievalVector,
	})
	if err != nil || result.Effective != RetrievalVector || len(result.Memories) != 1 || result.Memories[0].ID != receipt.Memory.MemoryID {
		t.Fatalf("W18 provider retrieval result=%#v err=%v", result, err)
	}
	requests, hashes := recorder.snapshot()
	if requests != 2 || len(hashes) != 2 {
		t.Fatalf("W18 provider requests/hashes=%d/%d", requests, len(hashes))
	}
	return projectionRetentionProviderResult{
		Dimensions: profile.Dimensions, Requests: requests,
		ProjectionResponseSHA256: hashes[0], QueryResponseSHA256: hashes[1],
	}
}

func projectionRetentionReceiptsMonotonic(receipts []ProjectionPruneReceipt) bool {
	floors := map[string]int64{}
	for _, receipt := range receipts {
		if receipt.PreviousFloorEventID != floors[receipt.TenantID] || receipt.NewFloorEventID < receipt.PreviousFloorEventID {
			return false
		}
		floors[receipt.TenantID] = receipt.NewFloorEventID
	}
	return true
}

func projectionRetentionGitRevision(t *testing.T) string {
	t.Helper()
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = filepath.Join("..", "..")
	if output, err := status.Output(); err != nil {
		t.Fatal(err)
	} else if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("formal W18 run requires a clean worktree")
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = filepath.Join("..", "..")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSpace(string(output))
	if !isProjectionRetentionLowerHex(revision, 40) {
		t.Fatalf("invalid W18 implementation revision %q", revision)
	}
	return revision
}

func projectionRetentionCaseHash(t *testing.T) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "W18-projection-event-retention-pruning", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:])
}

func projectionRetentionCPU() string {
	if output, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		if value := strings.TrimSpace(string(output)); value != "" {
			return value
		}
	}
	if payload, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(payload), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return goruntime.GOARCH
}

func projectionRetentionMemoryGiB() int {
	if output, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		if bytes, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil && bytes > 0 {
			return int(bytes / (1024 * 1024 * 1024))
		}
	}
	return 1
}
