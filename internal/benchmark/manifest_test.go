package benchmark

import (
	"strings"
	"testing"
)

func TestQualificationRequiresOfficialSourceEvidence(t *testing.T) {
	valid := validQualification()
	tests := map[string]func(*Qualification){
		"repository revision": func(q *Qualification) { q.Repository.Revision = "" },
		"license":             func(q *Qualification) { q.License = "" },
		"dataset revision":    func(q *Qualification) { q.Dataset.Revision = "" },
		"dataset sha256":      func(q *Qualification) { q.Dataset.SHA256 = "" },
		"dataset size":        func(q *Qualification) { q.Dataset.SizeBytes = 0 },
		"dataset records":     func(q *Qualification) { q.Dataset.RecordCount = 0 },
		"scorer revision":     func(q *Qualification) { q.OfficialScorer.Revision = "" },
		"scorer sha256":       func(q *Qualification) { q.OfficialScorer.SHA256 = "" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			qualification := valid
			mutate(&qualification)
			if err := qualification.Validate(); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s validation error, got %v", name, err)
			}
		})
	}
}

func TestQualificationRejectsInvalidSHA256(t *testing.T) {
	qualification := validQualification()
	qualification.Dataset.SHA256 = "ABC123"
	if err := qualification.Validate(); err == nil || !strings.Contains(err.Error(), "dataset sha256") {
		t.Fatalf("expected dataset sha256 validation error, got %v", err)
	}
}

func TestExecutionRejectsSampleBenchmarkWideClaim(t *testing.T) {
	manifest := validExecution()
	manifest.ExecutionScope = ExecutionScopeSample
	manifest.ClaimScope = ClaimScopeBenchmarkWide
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "sample execution cannot claim benchmark_wide") {
		t.Fatalf("expected sample claim rejection, got %v", err)
	}
}

func TestExecutionRejectsIncompleteFullRun(t *testing.T) {
	manifest := validFullRetrievalExecution()
	manifest.SelectionMode = ""
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "selection_mode") {
		t.Fatalf("expected incomplete full-run rejection, got %v", err)
	}
}

func TestExecutionAcceptsTargetSpecificQualifiedDatasetFull(t *testing.T) {
	manifest := validFullRetrievalExecution()
	if err := ValidateExecution(validQualification(), manifest); err != nil {
		t.Fatalf("expected valid full retrieval execution, got %v", err)
	}
}

func TestExecutionRejectsFullRunWithoutRecordSetDigest(t *testing.T) {
	manifest := validFullRetrievalExecution()
	manifest.RecordSetSHA256 = ""
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "record_set_sha256") {
		t.Fatalf("expected record-set rejection, got %v", err)
	}
}

func TestExecutionRejectsFullRunWithoutExpectedSourceCounts(t *testing.T) {
	manifest := validFullRetrievalExecution()
	manifest.ExpectedSessionCount = 0
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "expected_session_count") {
		t.Fatalf("expected session-count rejection, got %v", err)
	}
	manifest = validFullRetrievalExecution()
	manifest.ExpectedTurnCount = 0
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "expected_turn_count") {
		t.Fatalf("expected turn-count rejection, got %v", err)
	}
	manifest = validFullRetrievalExecution()
	manifest.ExpectedScoredRecordCount = 0
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "expected_scored_record_count") {
		t.Fatalf("expected scored-record rejection, got %v", err)
	}
}

func TestExecutionRejectsBenchmarkWideRetrievalClaim(t *testing.T) {
	manifest := validFullRetrievalExecution()
	manifest.ClaimScope = ClaimScopeBenchmarkWide
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "qualified_dataset_full") {
		t.Fatalf("expected overclaim rejection, got %v", err)
	}
}

func TestExecutionRequiresDeterministicScorerForHardFacts(t *testing.T) {
	manifest := validExecution()
	manifest.HardFactual = true
	manifest.Scorers = []ExecutionScorer{{Name: "judge", Class: ScorerClassOfficialModelJudge}}
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "hard factual execution requires a deterministic scorer") {
		t.Fatalf("expected deterministic scorer rejection, got %v", err)
	}
}

func TestExecutionRejectsDatasetDigestMismatch(t *testing.T) {
	manifest := validExecution()
	manifest.DatasetSHA256 = strings.Repeat("f", 64)
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "dataset sha256 does not match qualification") {
		t.Fatalf("expected dataset digest mismatch, got %v", err)
	}
}

func TestExecutionRequiresFrozenSampleFixture(t *testing.T) {
	manifest := validExecution()
	manifest.FixturePath = ""
	manifest.FixtureSHA256 = ""
	if err := ValidateExecution(validQualification(), manifest); err == nil || !strings.Contains(err.Error(), "sample execution requires a frozen fixture") {
		t.Fatalf("expected frozen-fixture rejection, got %v", err)
	}
}

func TestExecutionAcceptsQualifiedDatasetSample(t *testing.T) {
	if err := ValidateExecution(validQualification(), validExecution()); err != nil {
		t.Fatalf("expected valid sample execution, got %v", err)
	}
}

func validQualification() Qualification {
	return Qualification{
		SchemaVersion: "benchmark-qualification/v1",
		Benchmark:     "LongMemEval",
		SourceClass:   SourceClassOfficialDataset,
		Repository: SourceReference{
			URL:      "https://github.com/xiaowu0162/LongMemEval",
			Revision: "9e0b455f4ef0e2ab8f2e582289761153549043fc",
		},
		License: "MIT",
		Dataset: DatasetSource{
			URL:         "https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned",
			Path:        "longmemeval_oracle.json",
			Revision:    "98d7416c24c778c2fee6e6f3006e7a073259d48f",
			SHA256:      "821a2034d219ab45846873dd14c14f12cfe7776e73527a483f9dac095d38620c",
			SizeBytes:   15388478,
			RecordCount: 500,
		},
		OfficialScorer: ScorerSource{
			URL:      "https://github.com/xiaowu0162/LongMemEval",
			Path:     "src/evaluation/evaluate_qa.py",
			Revision: "9e0b455f4ef0e2ab8f2e582289761153549043fc",
			SHA256:   "ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251",
			Class:    ScorerClassOfficialModelJudge,
		},
	}
}

func validExecution() ExecutionManifest {
	return ExecutionManifest{
		SchemaVersion:     "benchmark-execution/v1",
		Benchmark:         "LongMemEval",
		QualificationPath: "casebook/benchmarks/qualifications/longmemeval-cleaned-oracle.json",
		DatasetSHA256:     "821a2034d219ab45846873dd14c14f12cfe7776e73527a483f9dac095d38620c",
		EvaluationTarget:  EvaluationTargetQA,
		ExecutionScope:    ExecutionScopeSample,
		ClaimScope:        ClaimScopeDatasetSample,
		SamplingRule:      "frozen factual records selected before provider execution",
		FixturePath:       "casebook/benchmarks/fixtures/longmemeval-oracle-sample.json",
		FixtureSHA256:     "bfd60ccc2f577a1f4e0596797a5143c4b6cf1169d18ace4b349c7ef87142a3f2",
		SelectedRecordIDs: []string{
			"record-a",
			"record-b",
		},
		HardFactual: true,
		Scorers: []ExecutionScorer{
			{Name: "token_f1", Class: ScorerClassDeterministic},
		},
	}
}

func validFullRetrievalExecution() ExecutionManifest {
	manifest := validExecution()
	manifest.EvaluationTarget = EvaluationTargetRetrieval
	manifest.ExecutionScope = ExecutionScopeFull
	manifest.ClaimScope = ClaimScopeQualifiedDatasetFull
	manifest.SelectionMode = SelectionModeAllRecords
	manifest.RecordSetSHA256 = strings.Repeat("d", 64)
	manifest.ExpectedSessionCount = 1000
	manifest.ExpectedTurnCount = 10000
	manifest.ExpectedScoredRecordCount = 470
	manifest.SamplingRule = ""
	manifest.FixturePath = ""
	manifest.FixtureSHA256 = ""
	manifest.SelectedRecordIDs = nil
	return manifest
}
