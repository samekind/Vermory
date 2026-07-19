package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vermory/internal/benchmark"
	vermoryruntime "vermory/internal/runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

type longMemEvalVectorTestEmbedder struct{}

type longMemEvalProjectionRecoveryTestEmbedder struct {
	longMemEvalVectorTestEmbedder
	batchFailuresRemaining int
}

func (longMemEvalVectorTestEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vector := make([]float32, 1024)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "launch") || strings.Contains(lower, "orbit"):
		vector[0] = 1
	case strings.Contains(lower, "maintenance") || strings.Contains(lower, "friday"):
		vector[1] = 1
	default:
		vector[2] = 1
	}
	return vector, nil
}

func (embedder longMemEvalVectorTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for index, text := range texts {
		vector, err := embedder.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors[index] = vector
	}
	return vectors, nil
}

func (embedder *longMemEvalProjectionRecoveryTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if embedder.batchFailuresRemaining > 0 {
		embedder.batchFailuresRemaining--
		return nil, errors.New("transient provider outage")
	}
	return embedder.longMemEvalVectorTestEmbedder.EmbedBatch(ctx, texts)
}

func TestLongMemEvalVectorRetrievalRunnerUsesDurableProductionProjection(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	records := []benchmark.LongMemEvalRecord{
		retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319."),
		retrievalTestRecord("record-b", "When is the maintenance window?", "Friday 22:30", "answer-b", "The maintenance window is Friday 22:30."),
	}
	paths := prepareLongMemEvalRetrievalTestFiles(t, records)
	var execution benchmark.ExecutionManifest
	executionBytes, err := os.ReadFile(paths.execution)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(executionBytes, &execution); err != nil {
		t.Fatal(err)
	}
	execution.Conditions = longMemEvalVectorConditions()
	writeBenchmarkJSON(t, paths.execution, execution)
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	report, err := RunLongMemEvalRetrieval(context.Background(), LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           t.TempDir(),
		RunID:                  "longmemeval-vector-retrieval-test",
		ImplementationRevision: "test-revision",
		VectorProfilePath:      filepath.Join(root, "casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v1.json"),
		VectorEmbedder:         longMemEvalVectorTestEmbedder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Vector == nil || !report.Vector.HardGatesPass {
		t.Fatalf("vector hard gates did not pass: %#v", report.Vector)
	}
	if report.Vector.Projection.VectorCount != 4 || report.Vector.Projection.Lag != 0 || report.Vector.EffectiveVectorQueries != 2 || report.Vector.DegradedVectorQueries != 0 {
		t.Fatalf("unexpected vector production evidence: %#v", report.Vector)
	}
	if report.Vector.Embedding.SuccessfulItems != 6 || report.Vector.Embedding.TerminalFailures != 0 {
		t.Fatalf("unexpected embedding accounting: %#v", report.Vector.Embedding)
	}
	for _, result := range report.Results {
		if len(result.Conditions) != 3 {
			t.Fatalf("record %s does not contain three conditions: %#v", result.RecordID, result.Conditions)
		}
		vector := result.Conditions[2]
		if vector.Condition != longMemEvalRetrievalVector || vector.Status != "completed" || vector.EffectiveMode != vermoryruntime.RetrievalVector || vector.Degraded || vector.MetricAt12 == nil || vector.MetricAt12.RecallAll != 1 || vector.AuditID == "" {
			t.Fatalf("record %s did not use effective production vector retrieval: %#v", result.RecordID, vector)
		}
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var audits, vectors int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_retrieval_runs WHERE tenant_id=$1 AND requested_mode='vector' AND effective_mode='vector' AND degraded=false`, "benchmark:longmemeval-vector-retrieval-test").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_vector_documents WHERE tenant_id=$1 AND profile_id=$2`, "benchmark:longmemeval-vector-retrieval-test", vermoryruntime.ProductionRetrievalProfileID).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if audits != 2 || vectors != 4 {
		t.Fatalf("production vector evidence is missing: audits=%d vectors=%d", audits, vectors)
	}
}

func TestLongMemEvalVectorRetrievalRunnerRecoversExhaustedProjectionOperation(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	records := []benchmark.LongMemEvalRecord{
		retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319."),
		retrievalTestRecord("record-b", "When is the maintenance window?", "Friday 22:30", "answer-b", "The maintenance window is Friday 22:30."),
	}
	paths := prepareLongMemEvalRetrievalTestFiles(t, records)
	prepareLongMemEvalVectorExecution(t, paths.execution)
	profilePath := prepareLongMemEvalProjectionRecoveryProfile(t, func(profile *LongMemEvalVectorProfile) {
		profile.RetryDelayMilliseconds = 0
	})
	embedder := &longMemEvalProjectionRecoveryTestEmbedder{batchFailuresRemaining: 5}
	var recoveryDelays []time.Duration
	report, err := RunLongMemEvalRetrieval(context.Background(), LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           t.TempDir(),
		RunID:                  "longmemeval-vector-projection-recovery-test",
		ImplementationRevision: "test-revision",
		VectorProfilePath:      profilePath,
		VectorEmbedder:         embedder,
		ProjectionRecoverySleeper: func(_ context.Context, delay time.Duration) error {
			recoveryDelays = append(recoveryDelays, delay)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Vector == nil || !report.Vector.HardGatesPass {
		t.Fatalf("recovered vector hard gates did not pass: %#v", report.Vector)
	}
	if report.Vector.ProjectionMaxRecoveries != 10 || report.Vector.ProjectionRecoveryDelaySeconds != 30 ||
		report.Vector.RecoveredProjectionFailures != 1 || report.Vector.UnrecoveredProjectionFailures != 0 ||
		report.Vector.ProjectionRecoverySleeps != 1 {
		t.Fatalf("unexpected projection recovery evidence: %#v", report.Vector)
	}
	if len(recoveryDelays) != 1 || recoveryDelays[0] != 30*time.Second {
		t.Fatalf("unexpected projection recovery delays: %#v", recoveryDelays)
	}
	if report.Vector.Embedding.TerminalFailures != 1 || report.Vector.Embedding.SuccessfulItems != 6 {
		t.Fatalf("exhausted operation was not transparently accounted: %#v", report.Vector.Embedding)
	}
}

func TestLongMemEvalVectorRetrievalRunnerFailsAfterProjectionRecoveryBudget(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	records := []benchmark.LongMemEvalRecord{
		retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319."),
	}
	paths := prepareLongMemEvalRetrievalTestFiles(t, records)
	prepareLongMemEvalVectorExecution(t, paths.execution)
	profilePath := prepareLongMemEvalProjectionRecoveryProfile(t, func(profile *LongMemEvalVectorProfile) {
		profile.RetryDelayMilliseconds = 0
		profile.ProjectionMaxRecoveries = 1
	})
	embedder := &longMemEvalProjectionRecoveryTestEmbedder{batchFailuresRemaining: 10}
	recoverySleeps := 0
	_, err := RunLongMemEvalRetrieval(context.Background(), LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           t.TempDir(),
		RunID:                  "longmemeval-vector-projection-budget-test",
		ImplementationRevision: "test-revision",
		VectorProfilePath:      profilePath,
		VectorEmbedder:         embedder,
		ProjectionRecoverySleeper: func(_ context.Context, _ time.Duration) error {
			recoverySleeps++
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "projection recovery budget exhausted") {
		t.Fatalf("expected projection recovery budget failure, got %v", err)
	}
	if recoverySleeps != 1 {
		t.Fatalf("unexpected projection recovery sleeps: %d", recoverySleeps)
	}
	pool, poolErr := pgxpool.New(context.Background(), databaseURL)
	if poolErr != nil {
		t.Fatal(poolErr)
	}
	defer pool.Close()
	var status, failureCode string
	var lastEventID int64
	var attempts, vectors int
	if queryErr := pool.QueryRow(context.Background(), `
SELECT status, last_event_id, attempt_count, last_error_code
FROM memory_projection_cursors
WHERE tenant_id=$1 AND profile_id=$2`,
		"benchmark:longmemeval-vector-projection-budget-test", vermoryruntime.ProductionRetrievalProfileID,
	).Scan(&status, &lastEventID, &attempts, &failureCode); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_vector_documents WHERE tenant_id=$1`, "benchmark:longmemeval-vector-projection-budget-test").Scan(&vectors); queryErr != nil {
		t.Fatal(queryErr)
	}
	if status != "failed" || failureCode != "embedding_unavailable" || lastEventID != 0 || attempts != 2 || vectors != 0 {
		t.Fatalf("projection failure evidence drifted: status=%s failure=%s last=%d attempts=%d vectors=%d", status, failureCode, lastEventID, attempts, vectors)
	}
}

func TestLongMemEvalRetrievalRunnerUsesProductionPathAndResumes(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	records := []benchmark.LongMemEvalRecord{
		retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319."),
		retrievalTestRecord("record-b", "When is the maintenance window?", "Friday 22:30", "answer-b", "The maintenance window is Friday 22:30."),
	}
	paths := prepareLongMemEvalRetrievalTestFiles(t, records)
	artifactRoot := t.TempDir()
	options := LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           artifactRoot,
		RunID:                  "longmemeval-retrieval-test",
		ImplementationRevision: "test-revision",
	}

	report, err := RunLongMemEvalRetrieval(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceSummary.RecordCount != 2 || report.RecordCount != 2 || report.ScoredRecordCount != 2 || report.ImportedMemoryCount != 4 {
		t.Fatalf("unexpected report counts: %#v", report)
	}
	if len(report.Results) != 2 || len(report.Failures) != 0 {
		t.Fatalf("unexpected record evidence: results=%#v failures=%#v", report.Results, report.Failures)
	}
	for _, result := range report.Results {
		if len(result.Conditions) != 2 {
			t.Fatalf("expected two conditions for %s: %#v", result.RecordID, result)
		}
		for _, condition := range result.Conditions {
			if condition.Status != "completed" || condition.MetricAt12 == nil || condition.MetricAt12.RecallAll != 1 {
				t.Fatalf("expected complete retrieval for %s/%s: %#v", result.RecordID, condition.Condition, condition)
			}
			if len(condition.RankedSessionIDs) == 0 || len(condition.RankedOccurrenceKeys) != len(condition.RankedSessionIDs) {
				t.Fatalf("missing ordered retrieval evidence: %#v", condition)
			}
		}
		checkpoint := filepath.Join(artifactRoot, "benchmarks", options.RunID, "checkpoints", result.RecordID+".json")
		if _, err := os.Stat(checkpoint); err != nil {
			t.Fatalf("missing checkpoint %s: %v", checkpoint, err)
		}
	}
	assertLongMemEvalRetrievalDatabaseCounts(t, databaseURL, "benchmark:"+options.RunID, 2, 4)

	before := countLongMemEvalRetrievalMemories(t, databaseURL, "benchmark:"+options.RunID)
	options.Resume = true
	resumed, err := RunLongMemEvalRetrieval(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	after := countLongMemEvalRetrievalMemories(t, databaseURL, "benchmark:"+options.RunID)
	if before != after || after != 4 {
		t.Fatalf("resume created duplicate memories: before=%d after=%d", before, after)
	}
	if resumed.Aggregates["vermory_lexical"] != report.Aggregates["vermory_lexical"] ||
		resumed.Aggregates["plain_token_overlap"] != report.Aggregates["plain_token_overlap"] {
		t.Fatalf("resume changed aggregate evidence: first=%#v resumed=%#v", report.Aggregates, resumed.Aggregates)
	}
}

func TestLongMemEvalRetrievalRunnerRejectsMismatchedCheckpoint(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	records := []benchmark.LongMemEvalRecord{
		retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319."),
	}
	paths := prepareLongMemEvalRetrievalTestFiles(t, records)
	artifactRoot := t.TempDir()
	options := LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           artifactRoot,
		RunID:                  "longmemeval-checkpoint-test",
		ImplementationRevision: "test-revision",
	}
	if _, err := RunLongMemEvalRetrieval(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(artifactRoot, "benchmarks", options.RunID, "checkpoints", "record-a.json")
	data, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	payload["dataset_sha256"] = strings.Repeat("f", 64)
	data, err = json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checkpoint, data, 0o600); err != nil {
		t.Fatal(err)
	}
	options.Resume = true
	if _, err := RunLongMemEvalRetrieval(context.Background(), options); err == nil || !strings.Contains(err.Error(), "checkpoint dataset_sha256") {
		t.Fatalf("expected checkpoint mismatch rejection, got %v", err)
	}
}

func TestLongMemEvalRetrievalRunnerRetainsRecordFailures(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	record := retrievalTestRecord("record-a", "What is my launch code?", "ORBIT-7319", "answer-a", "My launch code is ORBIT-7319.")
	record.AnswerSessionIDs = nil
	paths := prepareLongMemEvalRetrievalTestFiles(t, []benchmark.LongMemEvalRecord{record})
	artifactRoot := t.TempDir()
	report, err := RunLongMemEvalRetrieval(context.Background(), LongMemEvalRetrievalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           artifactRoot,
		RunID:                  "longmemeval-failure-test",
		ImplementationRevision: "test-revision",
	})
	if err == nil || !strings.Contains(err.Error(), "runtime failures") {
		t.Fatalf("expected retained runtime failure, got %v", err)
	}
	if len(report.Failures) != 1 || report.Failures[0].RecordID != "record-a" {
		t.Fatalf("failure was not retained: %#v", report.Failures)
	}
	if _, statErr := os.Stat(filepath.Join(artifactRoot, "benchmarks", report.RunID, "failure-ledger.json")); statErr != nil {
		t.Fatalf("failure ledger was not written: %v", statErr)
	}
}

type longMemEvalRetrievalTestPaths struct {
	qualification string
	execution     string
	source        string
}

func prepareLongMemEvalRetrievalTestFiles(t *testing.T, records []benchmark.LongMemEvalRecord) longMemEvalRetrievalTestPaths {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "longmemeval_s_cleaned.json")
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	datasetSHA := hex.EncodeToString(digest[:])
	summary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		t.Fatal(err)
	}

	qualificationPath := filepath.Join(dir, "qualification.json")
	writeBenchmarkJSON(t, qualificationPath, benchmark.Qualification{
		SchemaVersion: "benchmark-qualification/v1",
		Benchmark:     "LongMemEval-S",
		SourceClass:   benchmark.SourceClassOfficialDataset,
		Repository: benchmark.SourceReference{
			URL:      "https://example.test/LongMemEval",
			Revision: strings.Repeat("a", 40),
		},
		License: "MIT",
		Dataset: benchmark.DatasetSource{
			URL:         "https://example.test/longmemeval_s_cleaned.json",
			Path:        "longmemeval_s_cleaned.json",
			Revision:    strings.Repeat("b", 40),
			SHA256:      datasetSHA,
			SizeBytes:   int64(len(data)),
			RecordCount: len(records),
		},
		OfficialScorer: benchmark.ScorerSource{
			URL:      "https://example.test/print_retrieval_metrics.py",
			Path:     "src/evaluation/print_retrieval_metrics.py",
			Revision: strings.Repeat("a", 40),
			SHA256:   strings.Repeat("c", 64),
			Class:    benchmark.ScorerClassDeterministic,
		},
	})

	executionPath := filepath.Join(dir, "execution.json")
	writeBenchmarkJSON(t, executionPath, benchmark.ExecutionManifest{
		SchemaVersion:             "benchmark-execution/v1",
		Benchmark:                 "LongMemEval-S",
		QualificationPath:         qualificationPath,
		DatasetSHA256:             datasetSHA,
		EvaluationTarget:          benchmark.EvaluationTargetRetrieval,
		ExecutionScope:            benchmark.ExecutionScopeFull,
		ClaimScope:                benchmark.ClaimScopeQualifiedDatasetFull,
		SelectionMode:             benchmark.SelectionModeAllRecords,
		RecordSetSHA256:           summary.RecordSetSHA256,
		ExpectedSessionCount:      summary.SessionCount,
		ExpectedTurnCount:         summary.TurnCount,
		ExpectedScoredRecordCount: summary.ScoredRecordCount,
		HardFactual:               true,
		Scorers: []benchmark.ExecutionScorer{
			{Name: "session_recall", Class: benchmark.ScorerClassDeterministic},
		},
		RunID:      "longmemeval-retrieval-test",
		Conditions: []string{"plain_token_overlap", "vermory_lexical"},
	})
	return longMemEvalRetrievalTestPaths{qualification: qualificationPath, execution: executionPath, source: sourcePath}
}

func retrievalTestRecord(questionID, question, answer, answerSessionID, answerContent string) benchmark.LongMemEvalRecord {
	return benchmark.LongMemEvalRecord{
		QuestionID:         questionID,
		QuestionType:       "single-session-user",
		Question:           question,
		Answer:             answer,
		QuestionDate:       "2026/07/15",
		HaystackDates:      []string{"2026/07/13", "2026/07/14"},
		HaystackSessionIDs: []string{"noise-" + questionID, answerSessionID},
		HaystackSessions: [][]benchmark.LongMemEvalTurn{
			{{Role: "user", Content: "I bought groceries and cleaned the kitchen."}},
			{{Role: "user", Content: answerContent, HasAnswer: true}},
		},
		AnswerSessionIDs: []string{answerSessionID},
	}
}

func assertLongMemEvalRetrievalDatabaseCounts(t *testing.T, databaseURL, tenantID string, continuities, memories int) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var gotContinuities, gotMemories int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM continuity_spaces WHERE tenant_id=$1 AND continuity_line='conversation'`, tenantID).Scan(&gotContinuities); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM governed_memories WHERE tenant_id=$1 AND lifecycle_status='active'`, tenantID).Scan(&gotMemories); err != nil {
		t.Fatal(err)
	}
	if gotContinuities != continuities || gotMemories != memories {
		t.Fatalf("unexpected database counts: continuities=%d memories=%d", gotContinuities, gotMemories)
	}
}

func countLongMemEvalRetrievalMemories(t *testing.T, databaseURL, tenantID string) int {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM governed_memories WHERE tenant_id=$1`, tenantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func prepareLongMemEvalVectorExecution(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var execution benchmark.ExecutionManifest
	if err := json.Unmarshal(data, &execution); err != nil {
		t.Fatal(err)
	}
	execution.Conditions = longMemEvalVectorConditions()
	writeBenchmarkJSON(t, path, execution)
}

func prepareLongMemEvalProjectionRecoveryProfile(t *testing.T, mutate func(*LongMemEvalVectorProfile)) string {
	t.Helper()
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	profile, _, err := loadLongMemEvalVectorProfile(filepath.Join(root, "casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	mutate(&profile)
	path := filepath.Join(t.TempDir(), "vector-profile.json")
	writeBenchmarkJSON(t, path, profile)
	return path
}
