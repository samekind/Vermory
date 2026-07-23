package retrievalablation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReportCreatesDeterministicArtifactsAndReplays(t *testing.T) {
	report := reportTestFixture()
	paths, replayed, err := WriteReport(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first report write was marked replayed")
	}
	jsonPayload, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(jsonPayload, &decoded); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	if decoded.RequestFingerprint == "" || decoded.RequestFingerprint != report.RequestFingerprint {
		t.Fatalf("request fingerprint mismatch: %#v", decoded)
	}
	markdownText := string(markdown)
	for _, required := range []string{"# Production Retrieval Ablation", "lexical_runtime", "vector_pg", "hybrid_rrf", "Hard gates: PASS", "not a sealed result"} {
		if !strings.Contains(markdownText, required) {
			t.Fatalf("markdown missing %q:\n%s", required, markdownText)
		}
	}

	changedBody := report
	changedBody.Conditions = nil
	replayedPaths, replayed, err := WriteReport(filepath.Dir(paths.JSON), changedBody)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || replayedPaths != paths {
		t.Fatalf("exact report replay mismatch: paths=%#v replayed=%v", replayedPaths, replayed)
	}
	afterReplay, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterReplay) != string(jsonPayload) {
		t.Fatal("exact replay overwrote the existing report")
	}
}

func TestWriteReportRejectsConflictingReplay(t *testing.T) {
	dir := t.TempDir()
	report := reportTestFixture()
	if _, _, err := WriteReport(dir, report); err != nil {
		t.Fatal(err)
	}
	conflict := report
	conflict.CorpusSHA256 = strings.Repeat("b", 64)
	conflict.RequestFingerprint = ReportRequestFingerprint(conflict)
	if _, _, err := WriteReport(dir, conflict); err == nil || !strings.Contains(err.Error(), "conflicting report replay") {
		t.Fatalf("conflicting replay error=%v", err)
	}
}

func TestNormalizeReportOrdersConditionsQueriesAndNonClaims(t *testing.T) {
	report := reportTestFixture()
	report.Conditions[0], report.Conditions[2] = report.Conditions[2], report.Conditions[0]
	report.Conditions[0].Queries = []QueryReport{{QueryID: "z"}, {QueryID: "a"}}
	report.NonClaims = []string{"z claim", "a claim"}
	normalized := NormalizeReport(report)
	if got := []string{normalized.Conditions[0].Name, normalized.Conditions[1].Name, normalized.Conditions[2].Name}; strings.Join(got, ",") != "lexical_runtime,vector_pg,hybrid_rrf" {
		t.Fatalf("condition order=%v", got)
	}
	if normalized.Conditions[2].Queries[0].QueryID != "a" || strings.Join(normalized.NonClaims, ",") != "a claim,z claim" {
		t.Fatalf("normalized order mismatch: %#v", normalized)
	}
}

func TestExecuteReplaysBeforeOpeningDatabase(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "casebook", "cases", "101-example")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "source.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	corpusDir := filepath.Join(root, "runtime", "cases", "W08-replay")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	corpus := Corpus{
		Version: "1", Name: "replay",
		Scopes: []Scope{{ID: "workspace", TenantID: "tenant", Line: "workspace", Anchor: "/fixtures/replay"}},
		Records: []Record{
			{ID: "current", ScopeID: "workspace", Content: "Current fact.", Lifecycle: "active", ProvenanceCase: "101-example"},
			{ID: "forbidden", ScopeID: "workspace", Content: "Forbidden distractor.", Lifecycle: "active", ProvenanceCase: "101-example"},
		},
		Queries: []Query{{ID: "q", ScopeID: "workspace", Text: "current", Limit: 2, RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"forbidden"}, TaskExcludedRecordIDs: []string{"forbidden"}, Cohorts: []string{"semantic"}}},
	}
	payload, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(corpusDir, "corpus.json")
	if err := os.WriteFile(corpusPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := CorpusSHA256(corpus)
	if err != nil {
		t.Fatal(err)
	}
	options := Options{
		DatabaseURL: "postgresql://invalid-host", CorpusPath: corpusPath, RunID: "replay-run",
		EmbeddingBaseURL: "https://api.siliconflow.cn/v1", EmbeddingAPIKey: "unused-key",
		EmbeddingModel: "BAAI/bge-m3", EmbeddingDimensions: 1024, ImplementationRevision: "replay-revision",
	}
	report := reportTestFixture()
	report.RunID = options.RunID
	report.CorpusSHA256 = digest
	report.ImplementationRevision = options.ImplementationRevision
	report.Embedding = EmbeddingProfile{BaseURL: options.EmbeddingBaseURL, Model: options.EmbeddingModel, Dimensions: options.EmbeddingDimensions}
	report.RequestFingerprint = ReportRequestFingerprint(report)
	outputDir := filepath.Join(root, "artifacts")
	if _, _, err := WriteReport(outputDir, report); err != nil {
		t.Fatal(err)
	}
	replayedReport, _, replayed, err := Execute(context.Background(), options, outputDir)
	if err != nil {
		t.Fatalf("Execute opened the invalid database instead of replaying: %v", err)
	}
	if !replayed || replayedReport.RequestFingerprint != report.RequestFingerprint {
		t.Fatalf("execute replay mismatch: replayed=%v report=%#v", replayed, replayedReport)
	}
}

func TestExecuteRequiresOutputDirectoryBeforeLoadingCorpus(t *testing.T) {
	_, _, _, err := Execute(context.Background(), Options{
		DatabaseURL: "postgresql://example", CorpusPath: "missing.json", RunID: "run",
		EmbeddingBaseURL: "https://api.siliconflow.cn/v1", EmbeddingAPIKey: "secret",
		EmbeddingModel: "BAAI/bge-m3", EmbeddingDimensions: 1024, ImplementationRevision: "rev",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "output directory") {
		t.Fatalf("Execute error=%v", err)
	}
}

func reportTestFixture() Report {
	report := Report{
		RunID:                  "report-run",
		CorpusSHA256:           strings.Repeat("a", 64),
		ImplementationRevision: "0123456789abcdef",
		EngineVersion:          "rrf-v1",
		SchemaVersion:          13,
		AuthorityFingerprint:   strings.Repeat("c", 64),
		Embedding: EmbeddingProfile{
			BaseURL: "https://api.siliconflow.cn/v1", Model: "BAAI/bge-m3", Dimensions: 1024,
		},
		StartedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		Duration:  2 * time.Second,
		Conditions: []ConditionReport{
			{Name: ConditionLexical, Metrics: Aggregate{QueryCount: 1, RecallAtK: 0.5}},
			{Name: ConditionVector, Metrics: Aggregate{QueryCount: 1, RecallAtK: 0.75}},
			{Name: ConditionHybrid, Metrics: Aggregate{QueryCount: 1, RecallAtK: 1}},
		},
		HardGates:                   HardGateReport{Pass: true},
		ProjectionRebuildEquivalent: true,
		QualificationStatus:         "measured",
		NonClaims:                   []string{"not a sealed result"},
	}
	report.RequestFingerprint = ReportRequestFingerprint(report)
	return report
}
