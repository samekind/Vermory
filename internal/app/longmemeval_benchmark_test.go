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

	"vermory/internal/benchmark"
	"vermory/internal/provider"
	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLongMemEvalRunnerUsesFourComparableConditionsAndProductionVermoryPath(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	paths, records := prepareLongMemEvalBenchmarkTestFiles(t)
	answers := make(map[string]string, len(records))
	for _, record := range records {
		answers[record.Question] = record.Answer
	}
	llm := &recordingBenchmarkProvider{answers: answers, failAt: -1}
	artifactRoot := t.TempDir()

	report, err := RunLongMemEvalSample(context.Background(), LongMemEvalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           artifactRoot,
		ProviderOverride:       llm,
		ProviderName:           "test-provider",
		ProviderMode:           "test",
		Model:                  "test-model",
		RunID:                  "longmemeval-test-run",
		ImplementationRevision: "test-revision",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Results) != len(records)*4 {
		t.Fatalf("expected %d condition results, got %d", len(records)*4, len(report.Results))
	}
	if len(llm.calls) != len(records)*4 {
		t.Fatalf("expected %d provider calls, got %d", len(records)*4, len(llm.calls))
	}
	for _, condition := range LongMemEvalConditions() {
		aggregate := report.Aggregates[condition]
		if aggregate.Total != len(records) || aggregate.Completed != len(records) || aggregate.Failed != 0 {
			t.Fatalf("unexpected aggregate for %s: %#v", condition, aggregate)
		}
	}

	for _, call := range llm.calls {
		if !strings.Contains(call.System, "Respond in English") {
			t.Fatalf("benchmark reader did not receive the frozen English output contract: %q", call.System)
		}
		for _, forbidden := range []string{"continuity_id", "tenant_id", "memory_id", "has_answer"} {
			if strings.Contains(call.ContextPacket, forbidden) {
				t.Fatalf("model-facing packet leaked %q: %s", forbidden, call.ContextPacket)
			}
		}
	}
	if llm.calls[0].ContextPacket != "" {
		t.Fatalf("no_context unexpectedly received context: %q", llm.calls[0].ContextPacket)
	}
	if !strings.Contains(llm.calls[1].ContextPacket, "Date:") {
		t.Fatalf("full_oracle_history did not receive timestamped sessions: %q", llm.calls[1].ContextPacket)
	}
	if strings.TrimSpace(llm.calls[2].ContextPacket) == "" {
		t.Fatal("plain_lexical_retrieval received no context")
	}
	if !strings.Contains(llm.calls[3].ContextPacket, "Governed memory:") {
		t.Fatalf("vermory_packet did not use production context delivery: %q", llm.calls[3].ContextPacket)
	}

	assertLongMemEvalDatabaseEvidence(t, databaseURL, "benchmark:longmemeval-test-run", records)
	for _, relative := range []string{"source.json", "scores.json", "report.md", "execution-manifest.json"} {
		path := filepath.Join(artifactRoot, "benchmarks", "longmemeval-test-run", relative)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(artifactRoot, "benchmarks", "longmemeval-test-run", "execution-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var finalManifest benchmark.ExecutionManifest
	if err := json.Unmarshal(manifestData, &finalManifest); err != nil {
		t.Fatal(err)
	}
	if finalManifest.ImplementationRev != "test-revision" {
		t.Fatalf("expected explicit implementation revision, got %q", finalManifest.ImplementationRev)
	}
}

func TestLongMemEvalRunnerRetainsProviderFailures(t *testing.T) {
	databaseURL := resetBenchmarkDatabase(t)
	paths, records := prepareLongMemEvalBenchmarkTestFiles(t)
	answers := make(map[string]string, len(records))
	for _, record := range records {
		answers[record.Question] = record.Answer
	}
	longFailure := "planned provider failure: " + strings.Repeat("x", 700)
	llm := &recordingBenchmarkProvider{answers: answers, failAt: 3, failureMessage: longFailure}

	report, err := RunLongMemEvalSample(context.Background(), LongMemEvalOptions{
		QualificationPath:      paths.qualification,
		ExecutionPath:          paths.execution,
		SourceDatasetPath:      paths.source,
		DatabaseURL:            databaseURL,
		ArtifactRoot:           t.TempDir(),
		ProviderOverride:       llm,
		ProviderName:           "test-provider",
		ProviderMode:           "test",
		Model:                  "test-model",
		RunID:                  "longmemeval-failure-run",
		ImplementationRevision: "test-revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, result := range report.Results {
		if result.Status == "failed" {
			failed++
			if result.Error != longFailure {
				t.Fatalf("failure reason was not retained: %#v", result)
			}
		}
	}
	if failed != 1 {
		t.Fatalf("expected one retained provider failure, got %d", failed)
	}
}

type benchmarkTestPaths struct {
	qualification string
	execution     string
	source        string
}

func prepareLongMemEvalBenchmarkTestFiles(t *testing.T) (benchmarkTestPaths, []benchmark.LongMemEvalRecord) {
	t.Helper()
	fixture, err := os.ReadFile("../../casebook/benchmarks/fixtures/longmemeval-oracle-sample.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.json")
	fixturePath := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(sourcePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	records, err := benchmark.LoadLongMemEval(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(fixture)
	sha := hex.EncodeToString(digest[:])
	qualificationPath := filepath.Join(dir, "qualification.json")
	qualification := benchmark.Qualification{
		SchemaVersion: "benchmark-qualification/v1",
		Benchmark:     "LongMemEval",
		SourceClass:   benchmark.SourceClassOfficialDataset,
		Repository: benchmark.SourceReference{
			URL:      "https://example.test/LongMemEval",
			Revision: strings.Repeat("a", 40),
		},
		License: "MIT",
		Dataset: benchmark.DatasetSource{
			URL:         "https://example.test/longmemeval_oracle.json",
			Path:        "longmemeval_oracle.json",
			Revision:    strings.Repeat("b", 40),
			SHA256:      sha,
			SizeBytes:   int64(len(fixture)),
			RecordCount: len(records),
		},
		OfficialScorer: benchmark.ScorerSource{
			URL:      "https://example.test/evaluate_qa.py",
			Path:     "src/evaluation/evaluate_qa.py",
			Revision: strings.Repeat("a", 40),
			SHA256:   strings.Repeat("c", 64),
			Class:    benchmark.ScorerClassOfficialModelJudge,
		},
	}
	writeBenchmarkJSON(t, qualificationPath, qualification)

	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.QuestionID)
	}
	executionPath := filepath.Join(dir, "execution.json")
	execution := benchmark.ExecutionManifest{
		SchemaVersion:     "benchmark-execution/v1",
		Benchmark:         "LongMemEval",
		QualificationPath: qualificationPath,
		DatasetSHA256:     sha,
		EvaluationTarget:  benchmark.EvaluationTargetQA,
		ExecutionScope:    benchmark.ExecutionScopeSample,
		ClaimScope:        benchmark.ClaimScopeDatasetSample,
		SamplingRule:      "test fixture records frozen before provider execution",
		FixturePath:       fixturePath,
		FixtureSHA256:     sha,
		SelectedRecordIDs: ids,
		HardFactual:       true,
		Scorers: []benchmark.ExecutionScorer{
			{Name: "token_f1", Class: benchmark.ScorerClassDeterministic},
		},
		RunID:      "longmemeval-test-run",
		Conditions: LongMemEvalConditions(),
	}
	writeBenchmarkJSON(t, executionPath, execution)
	return benchmarkTestPaths{qualification: qualificationPath, execution: executionPath, source: sourcePath}, records
}

func writeBenchmarkJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func resetBenchmarkDatabase(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	return databaseURL
}

func assertLongMemEvalDatabaseEvidence(t *testing.T, databaseURL, tenantID string, records []benchmark.LongMemEvalRecord) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	wantMemories := 0
	for _, record := range records {
		wantMemories += len(record.HaystackSessions)
	}
	queries := []struct {
		name string
		sql  string
		want int
	}{
		{"continuities", `SELECT count(*) FROM continuity_spaces WHERE tenant_id=$1 AND continuity_line='conversation'`, len(records)},
		{"active memories", `SELECT count(*) FROM governed_memories WHERE tenant_id=$1 AND lifecycle_status='active'`, wantMemories},
		{"deliveries", `SELECT count(*) FROM memory_deliveries WHERE tenant_id=$1`, len(records)},
		{"completed turns", `SELECT count(*) FROM conversation_turns WHERE tenant_id=$1 AND status='completed'`, len(records)},
	}
	for _, query := range queries {
		var got int
		if err := pool.QueryRow(context.Background(), query.sql, tenantID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != query.want {
			t.Fatalf("expected %s=%d, got %d", query.name, query.want, got)
		}
	}
	var crossContinuity int
	if err := pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_deliveries delivery
JOIN governed_memories memory
  ON memory.tenant_id = delivery.tenant_id
 AND memory.continuity_id <> delivery.continuity_id
WHERE delivery.tenant_id = $1
  AND position(memory.content IN delivery.context_body) > 0`, tenantID).Scan(&crossContinuity); err != nil {
		t.Fatal(err)
	}
	if crossContinuity != 0 {
		t.Fatalf("cross-record memory leaked into %d deliveries", crossContinuity)
	}
}

type recordingBenchmarkProvider struct {
	answers        map[string]string
	calls          []provider.GenerateRequest
	failAt         int
	failureMessage string
}

func (p *recordingBenchmarkProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	callIndex := len(p.calls)
	p.calls = append(p.calls, request)
	if callIndex == p.failAt {
		message := p.failureMessage
		if message == "" {
			message = "planned provider failure"
		}
		return provider.GenerateResponse{}, errors.New(message)
	}
	answer := p.answers[request.Prompt]
	return provider.GenerateResponse{Output: answer, Model: request.Model, RawArtifact: []byte(`{"test":true}`)}, nil
}
