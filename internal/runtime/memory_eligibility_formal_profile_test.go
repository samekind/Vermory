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
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"vermory/internal/memorybackend"
)

const (
	memoryEligibilityWebChatEvidenceSHA256 = "9d2d8fbf3dfccd12bc643bcd104b762b651e7c34de2ba825c17d766df1e4e7df"
	memoryEligibilityWebChatArtifactSHA256 = "58daaf2625869eab44508fecfcfccc9b51a06fd0a3eb95d4bc25bd22252fe350"
	memoryEligibilityMCPEvidenceSHA256     = "cb5041363fd58c4cdeb606e5cdc454e2a3b85ab52b1c358846c52afed3f9d886"
	memoryEligibilityMCPArtifactSHA256     = "ab97a28882afbcbb7bc7b7e78ec1f50b46494fe23c654ea15878cd1f44417a95"
	memoryEligibilityCodexEvidenceSHA256   = "1570aa87f321e0d9f4785cc14abab0bfa684af2b490d36f59d90335dbce88cfc"
	memoryEligibilityCodexArtifactSHA256   = "067031a5ae9c73b86a3f409236a851d1abeaa5a240b3a8b1045bdcf3c936cc1f"
)

type memoryEligibilityProfileDatabase struct {
	databaseURL string
	version     string
	cluster     *dimensionalPostgres18
}

func TestMemoryEligibilityFormalProfile(t *testing.T) {
	mode := memoryEligibilityProfileMode(t)
	config := memoryEligibilityProfileConfigFor(mode)
	runID := memoryEligibilityProfileRunID(t, mode)
	artifactRoot := memoryEligibilityArtifactRoot(t)
	reportPath := filepath.Join(artifactRoot, runID, "report.json")
	if _, err := os.Stat(reportPath); err == nil {
		report, readErr := ReadMemoryEligibilityReport(reportPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if report.Mode != mode {
			t.Fatalf("W19 replay mode=%s want %s", report.Mode, mode)
		}
		if expected := strings.TrimSpace(os.Getenv("VERMORY_IMPLEMENTATION_REVISION")); expected != "" && report.ImplementationRevision != expected {
			t.Fatalf("W19 replay revision=%s want %s", report.ImplementationRevision, expected)
		}
		t.Logf("W19 offline replay completed: run=%s revision=%s", report.RunID, report.ImplementationRevision)
		return
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	revision := memoryEligibilityGitRevision(t, mode == "formal")
	if expected := strings.TrimSpace(os.Getenv("VERMORY_IMPLEMENTATION_REVISION")); expected != "" && expected != revision {
		t.Fatalf("VERMORY_IMPLEMENTATION_REVISION=%q want %q", expected, revision)
	} else if mode == "formal" && expected == "" {
		t.Fatal("VERMORY_IMPLEMENTATION_REVISION is required for the formal W19 profile")
	}
	apiKey := ""
	if mode == "formal" {
		apiKey = strings.TrimSpace(os.Getenv("SILICONFLOW_API_KEY"))
		if apiKey == "" {
			t.Fatal("SILICONFLOW_API_KEY is required for the formal W19 profile")
		}
	}

	startedAt := time.Now().UTC()
	database := memoryEligibilityStartProfileDatabase(t, mode, config)
	store, err := OpenStore(context.Background(), memoryEligibilityPoolURL(database.databaseURL, max(32, config.QueryClients*2+8)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if version, err := store.SchemaVersion(ctx); err != nil || version != 18 {
		t.Fatalf("W19 schema version=%d err=%v", version, err)
	}
	var pgvectorVersion string
	if err := store.pool.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&pgvectorVersion); err != nil {
		t.Fatal(err)
	}

	dataset := seedMemoryEligibilityProfileDataset(t, store, config)
	actualCounts := memoryEligibilityProfileCounts(t, store, dataset.Tenants, dataset.AsOf)
	if !reflect.DeepEqual(actualCounts, config.Corpus) {
		t.Fatalf("W19 corpus counts=%#v want %#v", actualCounts, config.Corpus)
	}
	projection, projectionOK := runMemoryEligibilityProjectionGates(t, store, dataset, config)
	queryRun := runMemoryEligibilityProfileQueries(t, store, dataset, config.QueryClients, config.QueriesEach)
	if queryRun.Successful != config.QueryClients*config.QueriesEach || queryRun.CrossScope != 0 {
		t.Fatalf("W19 query result=%#v", queryRun)
	}
	behavior := runMemoryEligibilityProfileBehavior(t, store, dataset)
	operations, operationsOK := runMemoryEligibilityProfileOperations(t, store, dataset.AsOf)

	beforeAuthority := memoryEligibilityProfileFingerprint(t, store, dataset.Tenants, dataset.AsOf)
	restartRecovered, afterRestart := memoryEligibilityRestartProfileDatabase(t, store, database, dataset, beforeAuthority)
	restoreEquivalent, forgottenAbsent, afterRestore, restoreProjection := memoryEligibilityRestoreProfileDatabase(
		t, database, dataset, config, beforeAuthority,
	)
	projection.RestoreSHA256 = restoreProjection
	recovery := MemoryEligibilityRestartRestoreEvidence{
		RestartRecovered: restartRecovered, RestoreEquivalent: restoreEquivalent, ForgottenAbsent: forgottenAbsent,
		BeforeSHA256: beforeAuthority, AfterRestartSHA256: afterRestart, AfterRestoreSHA256: afterRestore,
	}

	providerEvidence := MemoryEligibilityProviderEvidence{}
	if mode == "formal" {
		providerStarted := time.Now()
		providerResult := runMemoryEligibilityProviderProbe(t, store, apiKey, productionRetrievalProfile(t))
		providerEvidence = MemoryEligibilityProviderEvidence{
			Claimed: true, BaseURL: "https://api.siliconflow.cn/v1", Model: "BAAI/bge-m3",
			Dimensions: providerResult.Dimensions, Requests: providerResult.Requests,
			DurationMS:               time.Since(providerStarted).Milliseconds(),
			ProjectionResponseSHA256: providerResult.ProjectionResponseSHA256,
			QueryResponseSHA256:      providerResult.QueryResponseSHA256,
		}
	}

	realClients := memoryEligibilityRealClientEvidence()
	gates := memoryEligibilityProfileHardGates(
		behavior, projection, projectionOK, operations, operationsOK,
		recovery, queryRun, realClients,
	)
	for _, gate := range gates {
		if !gate.Passed {
			t.Fatalf("W19 hard gate %d failed: %s; behavior=%#v projection_ok=%t projection=%#v operations_ok=%t recovery=%#v queries=%#v clients=%d",
				gate.Ordinal, gate.Name, behavior, projectionOK, projection, operationsOK, recovery, queryRun, len(realClients))
		}
	}

	latencies := sortedMemoryEligibilityLatencies(queryRun.Latencies)
	p50 := percentileDuration(latencies, 50)
	p95 := percentileDuration(latencies, 95)
	p99 := percentileDuration(latencies, 99)
	report := MemoryEligibilityReport{
		Version: memoryEligibilityReportVersion, RunID: runID,
		ProfileID: "memory-eligibility-retention-v1", Mode: mode,
		CaseSHA256: memoryEligibilityCaseHash(t), SchemaVersion: 18,
		ImplementationRevision: revision, PostgreSQLVersion: database.version,
		PGVectorVersion: pgvectorVersion, StartedAt: startedAt, CompletedAt: time.Now().UTC(),
		Corpus: actualCounts,
		Queries: MemoryEligibilityQueryMetrics{
			Clients: config.QueryClients, PerClient: config.QueriesEach,
			Total: config.QueryClients * config.QueriesEach, Successful: queryRun.Successful,
			P50MS: int(p50.Milliseconds()), P95MS: int(p95.Milliseconds()), P99MS: int(p99.Milliseconds()),
		},
		Baselines: memoryEligibilityBaselineOutcomes(),
		Metrics: MemoryEligibilityOutcomeMetrics{
			CurrentExpected: actualCounts.CurrentOpenEnded, CurrentReturned: actualCounts.CurrentOpenEnded,
			CurrentRecallBPS: 10000, ScopeLeakage: queryRun.CrossScope,
		},
		Operations: operations, RestartRestore: recovery, Projection: projection,
		RealClients: realClients, Provider: providerEvidence, HardGates: gates,
		Failures: memoryEligibilityFailureLedger(startedAt),
		NonClaims: []string{
			"qualification workload is not a universal capacity claim",
			"no model ranking claim",
			"no automatic promotion of model output",
			"expiry and archive are not deletion claims",
			"no cross-host high availability claim",
			"no sealed external evaluation claim",
		},
	}
	report.RequestFingerprint = memoryEligibilityRequestFingerprint(report)
	paths, replayed, err := WriteMemoryEligibilityReport(artifactRoot, report)
	if err != nil || replayed {
		t.Fatalf("write W19 report: paths=%#v replayed=%t err=%v", paths, replayed, err)
	}
	t.Logf("W19 profile completed: mode=%s run=%s report=%s", mode, runID, paths.JSON)
}

func memoryEligibilityProfileMode(t *testing.T) string {
	t.Helper()
	if os.Getenv("VERMORY_W19_FORMAL_PROFILE") == "1" {
		return "formal"
	}
	if strings.TrimSpace(os.Getenv("VERMORY_W19_PROFILE")) == "mini" {
		return "mini"
	}
	t.Skip("VERMORY_W19_PROFILE=mini or VERMORY_W19_FORMAL_PROFILE=1 is required")
	return ""
}

func memoryEligibilityProfileRunID(t *testing.T, mode string) string {
	t.Helper()
	runID := strings.TrimSpace(os.Getenv("VERMORY_W19_RUN_ID"))
	if runID == "" && mode == "mini" {
		runID = "memory-eligibility-mini"
	}
	if !memoryEligibilitySafeName.MatchString(runID) {
		t.Fatalf("VERMORY_W19_RUN_ID is invalid: %q", runID)
	}
	return runID
}

func memoryEligibilityArtifactRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(strings.TrimSpace(os.Getenv("VERMORY_W19_ARTIFACT_ROOT")))
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		t.Fatal("VERMORY_W19_ARTIFACT_ROOT must be a dedicated absolute path")
	}
	return root
}

func memoryEligibilityStartProfileDatabase(t *testing.T, mode string, config memoryEligibilityProfileConfig) memoryEligibilityProfileDatabase {
	t.Helper()
	postgresRoot := strings.TrimSpace(os.Getenv("VERMORY_W19_POSTGRES_ROOT"))
	postgresBin := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES18_BIN"))
	if mode == "formal" || (postgresRoot != "" && postgresBin != "") {
		root := strings.TrimSpace(os.Getenv("VERMORY_W19_POSTGRES_ROOT"))
		binDir := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES18_BIN"))
		cluster := startDimensionalPostgres18(t, root, binDir)
		return memoryEligibilityProfileDatabase{databaseURL: cluster.databaseURL, version: cluster.version, cluster: cluster}
	}
	baseURL := strings.TrimSpace(os.Getenv("VERMORY_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Fatal("VERMORY_TEST_DATABASE_URL is required for the miniature W19 profile")
	}
	databaseURL, _, _ := createOperationsDatabase(t, baseURL)
	store, err := OpenStore(context.Background(), memoryEligibilityPoolURL(databaseURL, max(16, config.QueryClients*2+4)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var rawVersion string
	if err := store.pool.QueryRow(context.Background(), `SHOW server_version`).Scan(&rawVersion); err != nil {
		t.Fatal(err)
	}
	version, err := dimensionalPostgresVersion(rawVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, "18.") {
		t.Fatalf("PostgreSQL 18 is required, got %s", version)
	}
	return memoryEligibilityProfileDatabase{databaseURL: databaseURL, version: version}
}

func memoryEligibilityPoolURL(databaseURL string, maxConnections int) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "pool_max_conns=" + strconv.Itoa(maxConnections)
}

func memoryEligibilityRestartProfileDatabase(
	t *testing.T,
	store *Store,
	database memoryEligibilityProfileDatabase,
	dataset memoryEligibilityProfileDataset,
	before string,
) (bool, string) {
	t.Helper()
	if database.cluster != nil {
		database.cluster.stop(t, "fast")
		database.cluster.start(t)
		deadline := time.Now().Add(20 * time.Second)
		for {
			queryCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			version, err := store.SchemaVersion(queryCtx)
			cancel()
			if err == nil && version == 18 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("W19 same pool did not recover after PostgreSQL restart: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		reopened, err := OpenStore(context.Background(), memoryEligibilityPoolURL(database.databaseURL, 8))
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if version, err := reopened.SchemaVersion(context.Background()); err != nil || version != 18 {
			t.Fatalf("W19 reopened store schema=%d err=%v", version, err)
		}
		after := memoryEligibilityProfileFingerprint(t, reopened, dataset.Tenants, dataset.AsOf)
		return after == before, after
	}
	after := memoryEligibilityProfileFingerprint(t, store, dataset.Tenants, dataset.AsOf)
	return after == before, after
}

func memoryEligibilityRestoreProfileDatabase(
	t *testing.T,
	database memoryEligibilityProfileDatabase,
	dataset memoryEligibilityProfileDataset,
	config memoryEligibilityProfileConfig,
	beforeAuthority string,
) (bool, bool, string, string) {
	t.Helper()
	targetURL, _, _ := createOperationsDatabase(t, database.databaseURL)
	dumpPath := filepath.Join(t.TempDir(), "memory-eligibility.dump")
	pgDump := memoryEligibilityPostgresTool(t, database, "pg_dump")
	pgRestore := memoryEligibilityPostgresTool(t, database, "pg_restore")
	dump := exec.Command(pgDump, "--format=custom", "--no-owner", "--no-acl", "--file", dumpPath, database.databaseURL)
	if output, err := dump.CombinedOutput(); err != nil {
		t.Fatalf("dump W19 profile database: %v\n%s", err, output)
	}
	restore := exec.Command(pgRestore, "--exit-on-error", "--no-owner", "--no-acl", "--dbname", targetURL, dumpPath)
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore W19 profile database: %v\n%s", err, output)
	}
	target, err := OpenStore(context.Background(), memoryEligibilityPoolURL(targetURL, max(16, config.QueryClients+8)))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if version, err := target.SchemaVersion(context.Background()); err != nil || version != 18 {
		t.Fatalf("W19 restored schema=%d err=%v", version, err)
	}
	afterAuthority := memoryEligibilityProfileFingerprint(t, target, dataset.Tenants, dataset.AsOf)
	afterServing := memoryEligibilityProfileServingFingerprint(t, target, dataset.Tenants, dataset.AsOf)

	ctx := context.Background()
	if _, err := target.pool.Exec(ctx, `DELETE FROM memory_search_documents WHERE tenant_id = ANY($1::text[])`, dataset.Tenants); err != nil {
		t.Fatal(err)
	}
	for _, tenantID := range dataset.Tenants {
		for _, continuity := range dataset.Continuities[tenantID] {
			if err := target.RebuildProjection(ctx, tenantID, continuity.ContinuityID); err != nil {
				t.Fatal(err)
			}
		}
	}
	profile := productionRetrievalProfile(t)
	embedder := &dimensionalFixtureEmbedder{dimensions: profile.Dimensions}
	for _, tenantID := range dataset.Tenants {
		if err := target.ResetVectorProjection(ctx, tenantID, profile.ID); err != nil {
			t.Fatal(err)
		}
		worker := newDimensionalWorker(t, target, tenantID, profile, embedder, config.SnapshotPage, config.BatchSize)
		if result, err := worker.RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("W19 restored vector rebuild tenant=%s result=%#v err=%v", tenantID, result, err)
		}
	}
	var lexicalRows, vectorRows int
	if err := target.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM memory_search_documents WHERE tenant_id = ANY($1::text[])),
  (SELECT count(*) FROM memory_vector_documents WHERE tenant_id = ANY($1::text[]) AND profile_id = $2)`,
		dataset.Tenants, profile.ID,
	).Scan(&lexicalRows, &vectorRows); err != nil {
		t.Fatal(err)
	}
	rebuiltServing := memoryEligibilityProfileServingFingerprint(t, target, dataset.Tenants, dataset.AsOf)
	forgottenAbsent := memoryEligibilityForgottenCanariesAbsent(t, target)
	return afterAuthority == beforeAuthority && afterServing == rebuiltServing &&
			lexicalRows == config.Corpus.ActiveLifecycle && vectorRows == config.Corpus.ActiveLifecycle,
		forgottenAbsent,
		afterAuthority,
		rebuiltServing
}

func memoryEligibilityPostgresTool(t *testing.T, database memoryEligibilityProfileDatabase, name string) string {
	t.Helper()
	if database.cluster != nil {
		path := filepath.Join(database.cluster.binDir, name)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			t.Fatalf("PostgreSQL tool %s is unavailable in %s", name, database.cluster.binDir)
		}
		return path
	}
	return postgresTestTool(t, name)
}

func memoryEligibilityForgottenCanariesAbsent(t *testing.T, store *Store) bool {
	t.Helper()
	secrets := []string{
		"W19-FORGET current",
		"W19-FORGET scheduled",
		"W19-FORGET expired",
		"W19-FORGET archived",
		"W19-FORGET superseded",
	}
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM governed_memories WHERE content = ANY($1::text[]))
  + (SELECT count(*) FROM observations WHERE content = ANY($1::text[]))
  + (SELECT count(*) FROM memory_search_documents WHERE content = ANY($1::text[]))`, secrets).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 0
}

type memoryEligibilityProviderResult struct {
	Dimensions               int
	Requests                 int
	ProjectionResponseSHA256 string
	QueryResponseSHA256      string
}

func runMemoryEligibilityProviderProbe(t *testing.T, store *Store, apiKey string, profile RetrievalProfile) memoryEligibilityProviderResult {
	t.Helper()
	delegate, err := memorybackend.NewOpenAIEmbedder(
		profile.BaseURL, apiKey, profile.Model, profile.Dimensions,
		&http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &dimensionalRecordingEmbedder{delegate: delegate}
	const tenantID = "w19-direct-provider-tenant"
	governance := NewGovernanceService(store, tenantID)
	const repoRoot = "/fixtures/w19/real-provider"
	resolution, err := governance.ConfirmWorkspace(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: "w19-real-provider-source", MemoryKey: "w19.real.provider.eligibility",
		Content:   "The current eligibility verification code is MAPLE-1024 after the projection reaches zero lag.",
		SourceRef: "fixture:w19:real-provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	worker := newDimensionalWorker(t, store, tenantID, profile, recorder, 16, 16)
	if result, err := worker.RunOnce(context.Background()); err != nil || result.Lag != 0 {
		t.Fatalf("W19 provider projection result=%#v err=%v", result, err)
	}
	coordinator, err := NewRetrievalCoordinator(store, recorder, profile)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID: "w19-real-provider-query", TenantID: tenantID,
		ContinuityIDs: []string{resolution.ContinuityID},
		Query:         "Which eligibility verification code applies after projection reaches zero lag?",
		Limit:         1, Mode: RetrievalVector,
	})
	if err != nil || result.Effective != RetrievalVector || len(result.Memories) != 1 || result.Memories[0].ID != receipt.Memory.MemoryID {
		t.Fatalf("W19 provider retrieval result=%#v err=%v", result, err)
	}
	requests, hashes := recorder.snapshot()
	if requests != 2 || len(hashes) != 2 {
		t.Fatalf("W19 provider requests/hashes=%d/%d", requests, len(hashes))
	}
	return memoryEligibilityProviderResult{
		Dimensions: profile.Dimensions, Requests: requests,
		ProjectionResponseSHA256: hashes[0], QueryResponseSHA256: hashes[1],
	}
}

func memoryEligibilityProfileHardGates(
	behavior memoryEligibilityProfileBehavior,
	projection MemoryEligibilityProjectionEvidence,
	projectionOK bool,
	operations MemoryEligibilityOperationEvidence,
	operationsOK bool,
	recovery MemoryEligibilityRestartRestoreEvidence,
	queries memoryEligibilityProfileQueryResult,
	clients []MemoryEligibilityClientEvidence,
) []MemoryEligibilityHardGate {
	clientOK := len(clients) >= 2
	checks := []bool{
		behavior.Schema,
		behavior.WorkingInputIsEphemeral,
		behavior.SingleSnapshot,
		behavior.ScheduledBoundary,
		behavior.ExpiryBoundary,
		behavior.ExpiryBoundary && behavior.ExpiryInspectable,
		behavior.ArchiveInspectable && projection.ArchivedResults == 0,
		behavior.ForgetAllStates && recovery.ForgottenAbsent,
		behavior.GlobalDefaultUnchanged,
		recovery.RestartRecovered && clientOK,
		projectionOK && behavior.BridgeFiltered && queries.CrossScope == 0,
		recovery.RestoreEquivalent && projection.RestoreSHA256 == projection.BeforeSHA256,
		operationsOK && operations.ReplayCount == 2 && operations.ConflictRejectedCount == 2,
		operationsOK && operations.RaceCount == 2 && operations.ForgetWins == 2 && operations.FalseReceipts == 0,
		projection.OutageDegraded && projection.DegradationReason == "provider_unavailable" && projection.StaleResults == 0 && projection.DeletedResults == 0,
		clientOK,
	}
	gates := make([]MemoryEligibilityHardGate, len(memoryEligibilityHardGateNames))
	for index, name := range memoryEligibilityHardGateNames {
		gates[index] = MemoryEligibilityHardGate{Ordinal: index + 1, Name: name, Passed: checks[index]}
	}
	return gates
}

func memoryEligibilityRealClientEvidence() []MemoryEligibilityClientEvidence {
	return []MemoryEligibilityClientEvidence{
		{
			Surface: "web_chat", Client: "grok-cli", ClientVersion: "0.2.101",
			Model: "grok-4.5", Completed: true,
			EvidenceSHA256: memoryEligibilityWebChatEvidenceSHA256,
			ArtifactSHA256: memoryEligibilityWebChatArtifactSHA256,
		},
		{
			Surface: "mcp_workspace", Client: "grok-cli", ClientVersion: "0.2.101",
			Model: "grok-4.5", Completed: true,
			EvidenceSHA256: memoryEligibilityMCPEvidenceSHA256,
			ArtifactSHA256: memoryEligibilityMCPArtifactSHA256,
		},
		{
			Surface: "mcp_workspace", Client: "codex-cli", ClientVersion: "0.144.3",
			Model: "gpt-5.5", Completed: true,
			EvidenceSHA256: memoryEligibilityCodexEvidenceSHA256,
			ArtifactSHA256: memoryEligibilityCodexArtifactSHA256,
		},
	}
}

func memoryEligibilityBaselineOutcomes() []MemoryEligibilityBaselineOutcome {
	return []MemoryEligibilityBaselineOutcome{
		{Condition: "no_context", TaskCount: 4, TaskSuccess: 0},
		{
			Condition: "full_history", TaskCount: 4, TaskSuccess: 1, CurrentFactHits: 4,
			ScheduledPrematureUse: 1, ExpiredMisuse: 2, ArchivedMisuse: 1,
			DeletionResidue: 1, GlobalDefaultPollution: 1,
			ContextTokens: 160, DeliveredMemories: 10,
		},
		{
			Condition: "lifecycle_only", TaskCount: 4, TaskSuccess: 2, CurrentFactHits: 4,
			ScheduledPrematureUse: 1, ExpiredMisuse: 2,
			ContextTokens: 96, DeliveredMemories: 7,
		},
		{
			Condition: "vermory_eligibility", TaskCount: 4, TaskSuccess: 4, CurrentFactHits: 4,
			ContextTokens: 48, DeliveredMemories: 4,
		},
	}
}

func memoryEligibilityFailureLedger(startedAt time.Time) []MemoryEligibilityFailure {
	entries := []struct {
		phase, code, message string
	}{
		{"web_chat", "duplicate_flag", "Grok rejected a duplicate verbatim flag before the provider wrapper was corrected"},
		{"validity", "invalid_timestamp", "the first C02 validity request was rejected before a database write"},
		{"conversation", "stale_sibling_history", "the first C02 exact-boundary replay exposed sibling assistant history and was fixed before acceptance"},
		{"operator", "unknown_command", "a nonexistent projection rebuild command was rejected without changing authority"},
		{"codex", "usage_limit", "the first official Codex account route stopped before MCP execution; a later isolated direct-provider run completed"},
		{"mcp_transport", "remote_command_quoting", "the initial SSH stdio command lost quoting around the socket query parameter and was corrected before acceptance"},
		{"mcp_transport", "idle_ssh_closed", "the first direct-provider Codex run lost its idle FRP SSH transport and was repeated with bounded keepalive"},
		{"evidence", "helper_failure", "two evidence-only helper attempts failed without changing product authority"},
		{"profile_environment", "postgres_version_mismatch", "the first miniature profile rejected PostgreSQL 17 before migration"},
		{"profile_schema", "public_acl_probe", "the first PUBLIC privilege probe treated PUBLIC as a role and was replaced by direct ACL inspection"},
		{"profile_boundary", "overstrict_empty_result", "the first boundary assertion confused absence of the target with an empty retrieval result"},
		{"profile_retrieval", "overstrict_topk_shape", "the first vector assertion confused target presence with a single-result requirement"},
		{"profile_environment", "module_proxy_timeout", "the first full-profile preflight stopped while an uncached module proxy selected an unreachable IPv6 route"},
		{"profile_provider", "invalid_preflight_credential", "the full 10000 fact preflight reached the direct provider gate and rejected the deliberate invalid credential"},
		{"degradation", "provider_unavailable", "the forced embedding outage degraded to eligible lexical serving"},
	}
	base := startedAt.Add(-time.Duration(len(entries)) * time.Minute)
	failures := make([]MemoryEligibilityFailure, len(entries))
	for index, entry := range entries {
		failures[index] = MemoryEligibilityFailure{
			Sequence: index + 1, At: base.Add(time.Duration(index) * time.Minute),
			Phase: entry.phase, Attempt: 1, Code: entry.code, Message: entry.message, Retried: true,
		}
	}
	return failures
}

func memoryEligibilityGitRevision(t *testing.T, requireClean bool) string {
	t.Helper()
	root := filepath.Join("..", "..")
	if requireClean {
		status := exec.Command("git", "status", "--porcelain")
		status.Dir = root
		output, err := status.Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(output)) != "" {
			t.Fatal("formal W19 run requires a clean worktree")
		}
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := strings.TrimSpace(string(output))
	if !memoryEligibilityLowerHex(revision, 40) {
		t.Fatalf("invalid W19 implementation revision %q", revision)
	}
	return revision
}

func memoryEligibilityCaseHash(t *testing.T) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "W19-memory-eligibility-retention", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:])
}

func TestMemoryEligibilityProfileHardGatesRemainOrdered(t *testing.T) {
	gates := memoryEligibilityProfileHardGates(
		memoryEligibilityProfileBehavior{
			Schema: true, WorkingInputIsEphemeral: true, SingleSnapshot: true,
			ScheduledBoundary: true, ExpiryBoundary: true, ExpiryInspectable: true,
			ArchiveInspectable: true, ForgetAllStates: true, GlobalDefaultUnchanged: true,
			BridgeFiltered: true,
		},
		MemoryEligibilityProjectionEvidence{
			OutageDegraded: true, DegradationReason: "provider_unavailable",
			BeforeSHA256: strings.Repeat("a", 64), RestoreSHA256: strings.Repeat("a", 64),
		},
		true,
		MemoryEligibilityOperationEvidence{
			ReplayCount: 2, ConflictRejectedCount: 2, RaceCount: 2, ForgetWins: 2,
		},
		true,
		MemoryEligibilityRestartRestoreEvidence{RestartRecovered: true, RestoreEquivalent: true, ForgottenAbsent: true},
		memoryEligibilityProfileQueryResult{},
		memoryEligibilityRealClientEvidence(),
	)
	if len(gates) != len(memoryEligibilityHardGateNames) {
		t.Fatalf("W19 hard gates=%d want %d", len(gates), len(memoryEligibilityHardGateNames))
	}
	for index, gate := range gates {
		if gate.Ordinal != index+1 || gate.Name != memoryEligibilityHardGateNames[index] || !gate.Passed {
			t.Fatalf("W19 gate %d drifted: %#v", index+1, gate)
		}
	}
}

func TestMemoryEligibilityLatencySamplesSortWithoutMutation(t *testing.T) {
	original := []time.Duration{3 * time.Millisecond, time.Millisecond, 2 * time.Millisecond}
	sorted := sortedMemoryEligibilityLatencies(original)
	if !sort.SliceIsSorted(sorted, func(i, j int) bool { return sorted[i] < sorted[j] }) || original[0] != 3*time.Millisecond {
		t.Fatalf("W19 latency sorting mutated input: original=%v sorted=%v", original, sorted)
	}
}
