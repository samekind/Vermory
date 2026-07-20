package benchmark

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type SourceClass string

const (
	SourceClassOfficialDataset SourceClass = "official_dataset"
	SourceClassTranslatedProxy SourceClass = "translated_proxy"
	SourceClassInspiredCase    SourceClass = "inspired_case"
	SourceClassDesignMapping   SourceClass = "design_mapping"
	SourceClassUnsupported     SourceClass = "unsupported"
)

type ExecutionScope string

const (
	ExecutionScopeNone   ExecutionScope = "none"
	ExecutionScopeSample ExecutionScope = "sample"
	ExecutionScopeFull   ExecutionScope = "full"
)

type EvaluationTarget string

const (
	EvaluationTargetQA        EvaluationTarget = "qa"
	EvaluationTargetRetrieval EvaluationTarget = "retrieval"
)

type ClaimScope string

const (
	ClaimScopeMappingOnly          ClaimScope = "mapping_only"
	ClaimScopeDatasetSample        ClaimScope = "dataset_sample"
	ClaimScopeQualifiedDatasetFull ClaimScope = "qualified_dataset_full"
	ClaimScopeBenchmarkWide        ClaimScope = "benchmark_wide"
)

type SelectionMode string

const SelectionModeAllRecords SelectionMode = "all_records"

type ScorerClass string

const (
	ScorerClassDeterministic      ScorerClass = "deterministic"
	ScorerClassOfficialModelJudge ScorerClass = "official_model_judge"
	ScorerClassCustomModelJudge   ScorerClass = "custom_model_judge"
	ScorerClassNone               ScorerClass = "none"
)

type SourceReference struct {
	URL      string `json:"url"`
	Revision string `json:"revision"`
}

type DatasetSource struct {
	URL         string `json:"url"`
	Path        string `json:"path"`
	Revision    string `json:"revision"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	RecordCount int    `json:"record_count"`
}

type ScorerSource struct {
	URL      string      `json:"url"`
	Path     string      `json:"path"`
	Revision string      `json:"revision"`
	SHA256   string      `json:"sha256"`
	Class    ScorerClass `json:"class"`
}

type Qualification struct {
	SchemaVersion         string          `json:"schema_version"`
	Benchmark             string          `json:"benchmark"`
	SourceClass           SourceClass     `json:"source_class"`
	Repository            SourceReference `json:"repository"`
	License               string          `json:"license"`
	Dataset               DatasetSource   `json:"dataset"`
	OfficialScorer        ScorerSource    `json:"official_scorer"`
	KnownRisks            []string        `json:"known_risks,omitempty"`
	FixtureRedistribution string          `json:"fixture_redistribution,omitempty"`
}

type ExecutionScorer struct {
	Name  string      `json:"name"`
	Class ScorerClass `json:"class"`
}

type ExecutionModelConfig struct {
	Provider            string      `json:"provider"`
	Model               string      `json:"model"`
	Interface           string      `json:"interface"`
	ScorerClass         ScorerClass `json:"scorer_class,omitempty"`
	MaxOutputTokens     int         `json:"max_output_tokens"`
	TimeoutSeconds      int         `json:"timeout_seconds"`
	Workers             int         `json:"workers"`
	MaxAttempts         int         `json:"max_attempts"`
	MaxTerminalFailures int         `json:"max_terminal_failures,omitempty"`
}

type RetrievalExecutionInput struct {
	Path                   string `json:"path"`
	SHA256                 string `json:"sha256"`
	RunID                  string `json:"run_id"`
	ImplementationRevision string `json:"implementation_revision"`
	K                      int    `json:"k"`
}

type ExecutionManifest struct {
	SchemaVersion             string                   `json:"schema_version"`
	Benchmark                 string                   `json:"benchmark"`
	QualificationPath         string                   `json:"qualification_path"`
	DatasetSHA256             string                   `json:"dataset_sha256"`
	EvaluationTarget          EvaluationTarget         `json:"evaluation_target"`
	ExecutionScope            ExecutionScope           `json:"execution_scope"`
	ClaimScope                ClaimScope               `json:"claim_scope"`
	SelectionMode             SelectionMode            `json:"selection_mode,omitempty"`
	RecordSetSHA256           string                   `json:"record_set_sha256,omitempty"`
	ExpectedSessionCount      int                      `json:"expected_session_count,omitempty"`
	ExpectedTurnCount         int                      `json:"expected_turn_count,omitempty"`
	ExpectedScoredRecordCount int                      `json:"expected_scored_record_count,omitempty"`
	SamplingRule              string                   `json:"sampling_rule,omitempty"`
	FixturePath               string                   `json:"fixture_path,omitempty"`
	FixtureSHA256             string                   `json:"fixture_sha256,omitempty"`
	SelectedRecordIDs         []string                 `json:"selected_record_ids,omitempty"`
	HardFactual               bool                     `json:"hard_factual"`
	Scorers                   []ExecutionScorer        `json:"scorers,omitempty"`
	RunID                     string                   `json:"run_id,omitempty"`
	ImplementationRev         string                   `json:"implementation_revision,omitempty"`
	Conditions                []string                 `json:"conditions,omitempty"`
	Reader                    *ExecutionModelConfig    `json:"reader,omitempty"`
	Judge                     *ExecutionModelConfig    `json:"judge,omitempty"`
	RetrievalInput            *RetrievalExecutionInput `json:"retrieval_input,omitempty"`
	Artifacts                 map[string]string        `json:"artifacts,omitempty"`
	NonClaims                 []string                 `json:"non_claims,omitempty"`
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func LoadQualification(path string) (Qualification, error) {
	var qualification Qualification
	if err := decodeJSONFile(path, &qualification); err != nil {
		return Qualification{}, err
	}
	if err := qualification.Validate(); err != nil {
		return Qualification{}, err
	}
	return qualification, nil
}

func LoadExecution(path string) (ExecutionManifest, error) {
	var manifest ExecutionManifest
	if err := decodeJSONFile(path, &manifest); err != nil {
		return ExecutionManifest{}, err
	}
	return manifest, nil
}

func VerifyFileSHA256(path, expected string) error {
	if !sha256Pattern.MatchString(expected) {
		return fmt.Errorf("expected sha256 must be lowercase SHA-256")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("file sha256 mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func (q Qualification) Validate() error {
	if strings.TrimSpace(q.SchemaVersion) != "benchmark-qualification/v1" {
		return fmt.Errorf("unsupported qualification schema_version %q", q.SchemaVersion)
	}
	if strings.TrimSpace(q.Benchmark) == "" {
		return fmt.Errorf("benchmark is required")
	}
	if q.SourceClass != SourceClassOfficialDataset {
		return fmt.Errorf("qualification source_class must be official_dataset")
	}
	if strings.TrimSpace(q.Repository.URL) == "" {
		return fmt.Errorf("repository url is required")
	}
	if strings.TrimSpace(q.Repository.Revision) == "" {
		return fmt.Errorf("repository revision is required")
	}
	if strings.TrimSpace(q.License) == "" {
		return fmt.Errorf("license is required")
	}
	if strings.TrimSpace(q.Dataset.URL) == "" || strings.TrimSpace(q.Dataset.Path) == "" {
		return fmt.Errorf("dataset url and path are required")
	}
	if strings.TrimSpace(q.Dataset.Revision) == "" {
		return fmt.Errorf("dataset revision is required")
	}
	if !sha256Pattern.MatchString(q.Dataset.SHA256) {
		return fmt.Errorf("dataset sha256 must be lowercase SHA-256")
	}
	if q.Dataset.SizeBytes <= 0 {
		return fmt.Errorf("dataset size must be positive")
	}
	if q.Dataset.RecordCount <= 0 {
		return fmt.Errorf("dataset records must be positive")
	}
	if strings.TrimSpace(q.OfficialScorer.URL) == "" || strings.TrimSpace(q.OfficialScorer.Path) == "" {
		return fmt.Errorf("scorer url and path are required")
	}
	if strings.TrimSpace(q.OfficialScorer.Revision) == "" {
		return fmt.Errorf("scorer revision is required")
	}
	if !sha256Pattern.MatchString(q.OfficialScorer.SHA256) {
		return fmt.Errorf("scorer sha256 must be lowercase SHA-256")
	}
	if !validScorerClass(q.OfficialScorer.Class) || q.OfficialScorer.Class == ScorerClassNone {
		return fmt.Errorf("official scorer class is invalid")
	}
	return nil
}

func ValidateExecution(qualification Qualification, manifest ExecutionManifest) error {
	if err := qualification.Validate(); err != nil {
		return fmt.Errorf("qualification: %w", err)
	}
	if strings.TrimSpace(manifest.SchemaVersion) != "benchmark-execution/v1" {
		return fmt.Errorf("unsupported execution schema_version %q", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.Benchmark) == "" || manifest.Benchmark != qualification.Benchmark {
		return fmt.Errorf("execution benchmark does not match qualification")
	}
	if !sha256Pattern.MatchString(manifest.DatasetSHA256) {
		return fmt.Errorf("execution dataset sha256 must be lowercase SHA-256")
	}
	if manifest.DatasetSHA256 != qualification.Dataset.SHA256 {
		return fmt.Errorf("dataset sha256 does not match qualification")
	}
	if strings.TrimSpace(manifest.QualificationPath) == "" {
		return fmt.Errorf("qualification_path is required")
	}
	if manifest.EvaluationTarget != EvaluationTargetQA && manifest.EvaluationTarget != EvaluationTargetRetrieval {
		return fmt.Errorf("evaluation_target must be qa or retrieval")
	}

	switch manifest.ExecutionScope {
	case ExecutionScopeSample:
		if strings.TrimSpace(manifest.SamplingRule) == "" || len(manifest.SelectedRecordIDs) == 0 {
			return fmt.Errorf("sample execution requires sampling_rule and selected_record_ids")
		}
		if strings.TrimSpace(manifest.FixturePath) == "" || !sha256Pattern.MatchString(manifest.FixtureSHA256) {
			return fmt.Errorf("sample execution requires a frozen fixture path and SHA-256")
		}
		if manifest.ClaimScope == ClaimScopeBenchmarkWide {
			return fmt.Errorf("sample execution cannot claim benchmark_wide")
		}
		if manifest.ClaimScope != ClaimScopeDatasetSample {
			return fmt.Errorf("sample execution claim_scope must be dataset_sample")
		}
		if manifest.SelectionMode != "" || manifest.RecordSetSHA256 != "" ||
			manifest.ExpectedSessionCount != 0 || manifest.ExpectedTurnCount != 0 || manifest.ExpectedScoredRecordCount != 0 {
			return fmt.Errorf("sample execution cannot use full-dataset selection fields")
		}
	case ExecutionScopeFull:
		if manifest.SelectionMode != SelectionModeAllRecords {
			return fmt.Errorf("full execution selection_mode must be all_records")
		}
		if !sha256Pattern.MatchString(manifest.RecordSetSHA256) {
			return fmt.Errorf("full execution record_set_sha256 must be lowercase SHA-256")
		}
		if manifest.ClaimScope != ClaimScopeQualifiedDatasetFull {
			return fmt.Errorf("full execution claim_scope must be qualified_dataset_full")
		}
		if manifest.ExpectedSessionCount <= 0 {
			return fmt.Errorf("full execution expected_session_count must be positive")
		}
		if manifest.ExpectedTurnCount <= 0 {
			return fmt.Errorf("full execution expected_turn_count must be positive")
		}
		if manifest.ExpectedScoredRecordCount <= 0 || manifest.ExpectedScoredRecordCount > qualification.Dataset.RecordCount {
			return fmt.Errorf("full execution expected_scored_record_count must be between 1 and dataset record count")
		}
		if strings.TrimSpace(manifest.SamplingRule) != "" || strings.TrimSpace(manifest.FixturePath) != "" ||
			strings.TrimSpace(manifest.FixtureSHA256) != "" || len(manifest.SelectedRecordIDs) != 0 {
			return fmt.Errorf("full execution cannot use sample selection fields")
		}
	default:
		return fmt.Errorf("execution_scope must be sample or full")
	}

	seen := make(map[string]struct{}, len(manifest.SelectedRecordIDs))
	for _, id := range manifest.SelectedRecordIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("selected_record_ids cannot contain an empty id")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("selected_record_ids contains duplicate %q", id)
		}
		seen[id] = struct{}{}
	}

	hasDeterministic := false
	for _, scorer := range manifest.Scorers {
		if strings.TrimSpace(scorer.Name) == "" || !validScorerClass(scorer.Class) {
			return fmt.Errorf("execution scorer is invalid")
		}
		if scorer.Class == ScorerClassDeterministic {
			hasDeterministic = true
		}
	}
	if manifest.HardFactual && !hasDeterministic {
		return fmt.Errorf("hard factual execution requires a deterministic scorer")
	}
	return nil
}

func validScorerClass(class ScorerClass) bool {
	switch class {
	case ScorerClassDeterministic, ScorerClassOfficialModelJudge, ScorerClassCustomModelJudge, ScorerClassNone:
		return true
	default:
		return false
	}
}

func decodeJSONFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON file contains multiple values")
		}
		return err
	}
	return nil
}
