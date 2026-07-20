package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/benchmark"
	"vermory/internal/casebook"
	"vermory/internal/domain"
)

func TestBenchmarkCoverageWritesInternalReadyArtifacts(t *testing.T) {
	artifactRoot := t.TempDir()

	report, err := BenchmarkCoverage(context.Background(), BenchmarkCoverageOptions{
		MapPath:      "../../casebook/benchmarks/public-benchmark-map.json",
		ArtifactRoot: artifactRoot,
		RunID:        "benchmark-coverage-run",
	})
	if err != nil {
		t.Fatalf("BenchmarkCoverage returned error: %v", err)
	}

	if report.Total != 11 {
		t.Fatalf("expected 11 benchmark entries, got %d", report.Total)
	}
	if report.ExecutableCount < 4 {
		t.Fatalf("expected at least 4 executable benchmark mappings, got %d", report.ExecutableCount)
	}
	if report.TranslatedProxyCount != 8 {
		t.Fatalf("expected 8 translated proxies, got %d", report.TranslatedProxyCount)
	}
	if report.DesignMappingCount != 3 {
		t.Fatalf("expected 3 design mappings, got %d", report.DesignMappingCount)
	}
	if report.OriginalExecutionCount != 4 {
		t.Fatalf("expected four separately registered original executions, got %d", report.OriginalExecutionCount)
	}
	longMemEvalEvidence := false
	for _, entry := range report.Entries {
		if entry.Benchmark == "LongMemEval" && len(entry.OriginalExecutionEvidence) == 4 {
			longMemEvalEvidence = true
		}
	}
	if !longMemEvalEvidence {
		t.Fatal("expected LongMemEval original execution evidence to remain separate from its translated proxy")
	}
	if len(report.MissingTranslatedTask) != 0 {
		t.Fatalf("expected no missing translated benchmark mappings, got %v", report.MissingTranslatedTask)
	}
	if report.Artifacts.JSONURI == "" || report.Artifacts.MDURI == "" {
		t.Fatalf("expected benchmark coverage artifact URIs, got %#v", report.Artifacts)
	}

	jsonPath := filepath.Join(artifactRoot, "benchmark-coverage", "benchmark-coverage-run", "report.json")
	mdPath := filepath.Join(artifactRoot, "benchmark-coverage", "benchmark-coverage-run", "report.md")
	for _, path := range []string{jsonPath, mdPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}

	var persisted BenchmarkCoverageArtifact
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read benchmark coverage json: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal benchmark coverage json: %v", err)
	}
	if persisted.ExecutableCount != report.ExecutableCount {
		t.Fatalf("expected persisted executable count %d, got %d", report.ExecutableCount, persisted.ExecutableCount)
	}
}

func TestBenchmarkEvidenceRejectsMissingOriginalExecutionReference(t *testing.T) {
	_, err := countOriginalExecutionEvidence([]casebook.BenchmarkMapEntry{{
		Benchmark:                 domain.BenchmarkName("LongMemEval"),
		OriginalExecutionEvidence: []string{"docs/evidence/snapshots/missing.json"},
	}}, t.TempDir())
	if err == nil {
		t.Fatal("expected missing original execution evidence to be rejected")
	}
}

func TestBenchmarkEvidenceRejectsMissingFrozenFixture(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatalf("projectRoot returned error: %v", err)
	}
	execution, err := benchmark.LoadExecution(filepath.Join(root, "docs/evidence/snapshots/2026-07-14-longmemeval-original-sample-execution.json"))
	if err != nil {
		t.Fatalf("load execution snapshot: %v", err)
	}
	execution.QualificationPath = filepath.Join(root, execution.QualificationPath)
	execution.FixturePath = "casebook/benchmarks/fixtures/missing.json"

	executionPath := filepath.Join(t.TempDir(), "execution.json")
	data, err := json.Marshal(execution)
	if err != nil {
		t.Fatalf("marshal execution: %v", err)
	}
	if err := os.WriteFile(executionPath, data, 0o600); err != nil {
		t.Fatalf("write execution: %v", err)
	}

	_, err = countOriginalExecutionEvidence([]casebook.BenchmarkMapEntry{{
		Benchmark:                 domain.BenchmarkName("LongMemEval"),
		OriginalExecutionEvidence: []string{executionPath},
	}}, root)
	if err == nil {
		t.Fatal("expected missing frozen fixture to be rejected")
	}
}

func TestBenchmarkCoverageValidatesFullQAWithoutRequiringRawRetrievalInGit(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	execution, err := benchmark.LoadExecution(filepath.Join(root, "casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json"))
	if err != nil {
		t.Fatal(err)
	}
	execution.QualificationPath = filepath.Join(root, execution.QualificationPath)
	execution.RetrievalInput.Path = "runtime-only-retrieval-results.jsonl"
	executionPath := filepath.Join(t.TempDir(), "full-qa-execution.json")
	writeExecution := func() {
		data, err := json.Marshal(execution)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executionPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeExecution()
	count, err := countOriginalExecutionEvidence([]casebook.BenchmarkMapEntry{{
		Benchmark:                 domain.BenchmarkName("LongMemEval"),
		OriginalExecutionEvidence: []string{executionPath},
	}}, root)
	if err != nil || count != 1 {
		t.Fatalf("full QA evidence was not accepted: count=%d err=%v", count, err)
	}

	execution.Reader.Workers = 0
	writeExecution()
	if _, err := countOriginalExecutionEvidence([]casebook.BenchmarkMapEntry{{
		Benchmark:                 domain.BenchmarkName("LongMemEval"),
		OriginalExecutionEvidence: []string{executionPath},
	}}, root); err == nil || !strings.Contains(err.Error(), "reader workers") {
		t.Fatalf("expected QA-specific reader validation, got %v", err)
	}
}
