package casebook

import "vermory/internal/domain"

type BenchmarkTrack string

const (
	BenchmarkTrackWorkspace      BenchmarkTrack = "workspace"
	BenchmarkTrackConversation   BenchmarkTrack = "conversation"
	BenchmarkTrackGlobalDefaults BenchmarkTrack = "global_defaults"
	BenchmarkTrackBridge         BenchmarkTrack = "bridge"
)

type BenchmarkEvaluationLevel string

const (
	BenchmarkEvaluationLevelReference            BenchmarkEvaluationLevel = "reference"
	BenchmarkEvaluationLevelTranslatedTask       BenchmarkEvaluationLevel = "translated_task"
	BenchmarkEvaluationLevelExecutableEvaluation BenchmarkEvaluationLevel = "executable_evaluation"
)

type BenchmarkMapEntry struct {
	Benchmark                 domain.BenchmarkName       `json:"benchmark"`
	Line                      BenchmarkTrack             `json:"line"`
	Capability                domain.BenchmarkCapability `json:"capability"`
	EvaluationLevel           BenchmarkEvaluationLevel   `json:"evaluation_level"`
	ExecutionMode             string                     `json:"execution_mode,omitempty"`
	CaseIDs                   []string                   `json:"case_ids,omitempty"`
	OriginalExecutionEvidence []string                   `json:"original_execution_evidence,omitempty"`
	Notes                     string                     `json:"notes,omitempty"`
}

type BenchmarkCoverageReport struct {
	Total                  int            `json:"total"`
	TranslatedOrBetter     int            `json:"translated_or_better"`
	ExecutableCount        int            `json:"executable_count"`
	MissingTranslatedTask  []string       `json:"missing_translated_task,omitempty"`
	ExecutableWithoutCases []string       `json:"executable_without_cases,omitempty"`
	ByLine                 map[string]int `json:"by_line"`
}

type Claim struct {
	Type    domain.ClaimType `json:"type"`
	Content string           `json:"content"`
}

type Task struct {
	ID             string     `json:"id"`
	Prompt         string     `json:"prompt"`
	MustInclude    []string   `json:"must_include"`
	MustIncludeAny [][]string `json:"must_include_any,omitempty"`
	MustNotInclude []string   `json:"must_not_include"`
}

type Case struct {
	ID        string
	Directory string
	SourceMD  string
	Tasks     []Task
	Claims    []Claim
}
