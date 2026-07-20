package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLongMemEvalQAExecutionAcceptsFrozenFullRun(t *testing.T) {
	qualification := validLongMemEvalQAQualification()
	manifest := validLongMemEvalQAExecution()
	if err := ValidateLongMemEvalQAExecution(qualification, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLongMemEvalQAExecutionAcceptsFrozenLexicalVectorPair(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.RunID = ""
	manifest.ImplementationRev = ""
	manifest.Conditions = []string{"vermory_lexical_k10", "vermory_vector_k10"}
	if err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLongMemEvalQAExecutionPreservesLegacyRunIDRequirement(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.RunID = ""
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("expected legacy run ID rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsMissingReader(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Reader = nil
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "reader") {
		t.Fatalf("expected reader rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsMissingJudge(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Judge = nil
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "judge") {
		t.Fatalf("expected judge rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsWrongK(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.RetrievalInput.K = 12
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "K=10") {
		t.Fatalf("expected K rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsMissingRetrievalIdentity(t *testing.T) {
	tests := map[string]func(*RetrievalExecutionInput){
		"path":                    func(input *RetrievalExecutionInput) { input.Path = "" },
		"sha256":                  func(input *RetrievalExecutionInput) { input.SHA256 = "" },
		"run_id":                  func(input *RetrievalExecutionInput) { input.RunID = "" },
		"implementation_revision": func(input *RetrievalExecutionInput) { input.ImplementationRevision = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := validLongMemEvalQAExecution()
			mutate(manifest.RetrievalInput)
			err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s rejection, got %v", name, err)
			}
		})
	}
}

func TestValidateLongMemEvalQAExecutionRejectsInvalidModelConfig(t *testing.T) {
	tests := map[string]func(*ExecutionModelConfig){
		"provider":          func(config *ExecutionModelConfig) { config.Provider = "" },
		"model":             func(config *ExecutionModelConfig) { config.Model = "" },
		"interface":         func(config *ExecutionModelConfig) { config.Interface = "" },
		"max_output_tokens": func(config *ExecutionModelConfig) { config.MaxOutputTokens = 0 },
		"timeout_seconds":   func(config *ExecutionModelConfig) { config.TimeoutSeconds = 0 },
		"workers":           func(config *ExecutionModelConfig) { config.Workers = 0 },
		"max_attempts":      func(config *ExecutionModelConfig) { config.MaxAttempts = 0 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := validLongMemEvalQAExecution()
			mutate(manifest.Reader)
			err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s rejection, got %v", name, err)
			}
		})
	}
}

func TestValidateLongMemEvalQAExecutionRejectsReaderScorerClass(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Reader.ScorerClass = ScorerClassCustomModelJudge
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "reader scorer_class") {
		t.Fatalf("expected reader scorer-class rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsUnsupportedJudgeClass(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Judge.ScorerClass = ScorerClassNone
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "judge scorer_class") {
		t.Fatalf("expected judge scorer-class rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsFalseOfficialJudge(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Judge.ScorerClass = ScorerClassOfficialModelJudge
	err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
	if err == nil || !strings.Contains(err.Error(), "gpt-4o-2024-08-06") {
		t.Fatalf("expected official-model rejection, got %v", err)
	}
}

func TestValidateLongMemEvalQAExecutionAcceptsExactOfficialJudge(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Judge.Provider = "openai-compatible"
	manifest.Judge.Model = "gpt-4o-2024-08-06"
	manifest.Judge.Interface = "openai_chat_completions"
	manifest.Judge.ScorerClass = ScorerClassOfficialModelJudge
	if err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLongMemEvalQAExecutionRejectsConditionDrift(t *testing.T) {
	for _, conditions := range [][]string{
		{"vermory_lexical_k10", "plain_token_overlap_k10"},
		{"plain_token_overlap_k10", "vermory_vector_k10"},
		{"vermory_lexical_k10", "unknown_k10"},
	} {
		manifest := validLongMemEvalQAExecution()
		manifest.Conditions = conditions
		err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
		if err == nil || !strings.Contains(err.Error(), "conditions") {
			t.Fatalf("expected condition rejection for %v, got %v", conditions, err)
		}
	}
}

func TestValidateLongMemEvalQAExecutionRejectsNegativeTerminalFailureLimit(t *testing.T) {
	manifest := validLongMemEvalQAExecution()
	manifest.Reader.MaxTerminalFailures = -1
	if err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest); err == nil || !strings.Contains(err.Error(), "max_terminal_failures") {
		t.Fatalf("expected reader terminal-failure-limit rejection, got %v", err)
	}

	manifest = validLongMemEvalQAExecution()
	manifest.Judge.MaxTerminalFailures = -1
	if err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest); err == nil || !strings.Contains(err.Error(), "max_terminal_failures") {
		t.Fatalf("expected judge terminal-failure-limit rejection, got %v", err)
	}
}

func TestLongMemEvalQAOfficialManifestsValidate(t *testing.T) {
	qualification, err := LoadQualification("../../casebook/benchmarks/qualifications/longmemeval-s-cleaned-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadExecution("../../casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLongMemEvalQAExecution(qualification, manifest); err != nil {
		t.Fatal(err)
	}
	vectorManifest, err := LoadExecution("../../casebook/benchmarks/executions/longmemeval-s-full-vector-reader-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLongMemEvalQAExecution(qualification, vectorManifest); err != nil {
		t.Fatal(err)
	}
	domesticManifest, err := LoadExecution("../../casebook/benchmarks/executions/longmemeval-s-full-domestic-vector-reader-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLongMemEvalQAExecution(qualification, domesticManifest); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionManifestRejectsCredentialShapedUnknownFields(t *testing.T) {
	for _, field := range []string{"api_key", "token", "authorization", "secret"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "execution.json")
			data := []byte(`{"schema_version":"benchmark-execution/v1","` + field + `":"must-not-load"}`)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadExecution(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("expected unknown-field rejection, got %v", err)
			}
		})
	}
}

func validLongMemEvalQAQualification() Qualification {
	qualification := validQualification()
	qualification.Dataset.Path = "longmemeval_s_cleaned.json"
	qualification.Dataset.SHA256 = strings.Repeat("a", 64)
	qualification.Dataset.SizeBytes = 277383467
	qualification.OfficialScorer.Path = "src/evaluation/evaluate_qa.py"
	qualification.OfficialScorer.SHA256 = strings.Repeat("b", 64)
	qualification.OfficialScorer.Class = ScorerClassOfficialModelJudge
	return qualification
}

func validLongMemEvalQAExecution() ExecutionManifest {
	return ExecutionManifest{
		SchemaVersion:             "benchmark-execution/v1",
		Benchmark:                 "LongMemEval",
		QualificationPath:         "casebook/benchmarks/qualifications/longmemeval-s-cleaned-qa.json",
		DatasetSHA256:             strings.Repeat("a", 64),
		EvaluationTarget:          EvaluationTargetQA,
		ExecutionScope:            ExecutionScopeFull,
		ClaimScope:                ClaimScopeQualifiedDatasetFull,
		SelectionMode:             SelectionModeAllRecords,
		RecordSetSHA256:           strings.Repeat("c", 64),
		ExpectedSessionCount:      23867,
		ExpectedTurnCount:         246750,
		ExpectedScoredRecordCount: 470,
		HardFactual:               true,
		Scorers: []ExecutionScorer{
			{Name: "normalized_exact_match", Class: ScorerClassDeterministic},
			{Name: "token_f1", Class: ScorerClassDeterministic},
			{Name: "answer_token_recall", Class: ScorerClassDeterministic},
			{Name: "upstream_prompt_custom_judge", Class: ScorerClassCustomModelJudge},
		},
		RunID: "longmemeval-s-full-reader-qa-test",
		Conditions: []string{
			"plain_token_overlap_k10",
			"vermory_lexical_k10",
		},
		Reader: &ExecutionModelConfig{
			Provider:        "grok-cli",
			Model:           "grok-composer-2.5-fast",
			Interface:       "isolated_stateless_cli",
			MaxOutputTokens: 128,
			TimeoutSeconds:  180,
			Workers:         4,
			MaxAttempts:     3,
		},
		Judge: &ExecutionModelConfig{
			Provider:        "grok-cli",
			Model:           "grok-4.5",
			Interface:       "isolated_stateless_cli",
			ScorerClass:     ScorerClassCustomModelJudge,
			MaxOutputTokens: 10,
			TimeoutSeconds:  120,
			Workers:         4,
			MaxAttempts:     3,
		},
		RetrievalInput: &RetrievalExecutionInput{
			Path:                   "retrieval-results.jsonl",
			SHA256:                 strings.Repeat("d", 64),
			RunID:                  "longmemeval-s-full-retrieval-test",
			ImplementationRevision: strings.Repeat("e", 40),
			K:                      10,
		},
	}
}
