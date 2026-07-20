package benchmark

import (
	"fmt"
	"slices"
	"strings"
)

var longMemEvalQALegacyConditions = []string{
	"plain_token_overlap_k10",
	"vermory_lexical_k10",
}

var longMemEvalQAVectorConditions = []string{
	"vermory_lexical_k10",
	"vermory_vector_k10",
}

func ValidateLongMemEvalQAExecution(qualification Qualification, manifest ExecutionManifest) error {
	if err := ValidateExecution(qualification, manifest); err != nil {
		return err
	}
	if manifest.EvaluationTarget != EvaluationTargetQA {
		return fmt.Errorf("LongMemEval reader execution evaluation_target must be qa")
	}
	if manifest.ExecutionScope != ExecutionScopeFull || manifest.ClaimScope != ClaimScopeQualifiedDatasetFull {
		return fmt.Errorf("LongMemEval reader execution must be full qualified_dataset_full")
	}
	if qualification.OfficialScorer.Class != ScorerClassOfficialModelJudge {
		return fmt.Errorf("LongMemEval QA qualification official scorer must be official_model_judge")
	}
	legacyConditions := slices.Equal(manifest.Conditions, longMemEvalQALegacyConditions)
	vectorConditions := slices.Equal(manifest.Conditions, longMemEvalQAVectorConditions)
	if !legacyConditions && !vectorConditions {
		return fmt.Errorf("LongMemEval reader execution conditions must be %v or %v", longMemEvalQALegacyConditions, longMemEvalQAVectorConditions)
	}
	if legacyConditions && strings.TrimSpace(manifest.RunID) == "" {
		return fmt.Errorf("LongMemEval reader execution run_id is required for legacy conditions")
	}
	if manifest.Reader == nil {
		return fmt.Errorf("LongMemEval reader execution reader is required")
	}
	if err := validateExecutionModelConfig("reader", *manifest.Reader, false); err != nil {
		return err
	}
	if manifest.Judge == nil {
		return fmt.Errorf("LongMemEval reader execution judge is required")
	}
	if err := validateExecutionModelConfig("judge", *manifest.Judge, true); err != nil {
		return err
	}
	if manifest.RetrievalInput == nil {
		return fmt.Errorf("LongMemEval reader execution retrieval_input is required")
	}
	if err := validateRetrievalExecutionInput(*manifest.RetrievalInput); err != nil {
		return err
	}
	return nil
}

func validateExecutionModelConfig(label string, config ExecutionModelConfig, judge bool) error {
	if strings.TrimSpace(config.Provider) == "" {
		return fmt.Errorf("%s provider is required", label)
	}
	if strings.TrimSpace(config.Model) == "" {
		return fmt.Errorf("%s model is required", label)
	}
	if strings.TrimSpace(config.Interface) == "" {
		return fmt.Errorf("%s interface is required", label)
	}
	if config.MaxOutputTokens <= 0 {
		return fmt.Errorf("%s max_output_tokens must be positive", label)
	}
	if config.TimeoutSeconds <= 0 {
		return fmt.Errorf("%s timeout_seconds must be positive", label)
	}
	if config.Workers <= 0 {
		return fmt.Errorf("%s workers must be positive", label)
	}
	if config.MaxAttempts <= 0 {
		return fmt.Errorf("%s max_attempts must be positive", label)
	}
	if !judge {
		if config.ScorerClass != "" {
			return fmt.Errorf("reader scorer_class must be empty")
		}
		return nil
	}
	if config.ScorerClass != ScorerClassOfficialModelJudge && config.ScorerClass != ScorerClassCustomModelJudge {
		return fmt.Errorf("judge scorer_class must be official_model_judge or custom_model_judge")
	}
	if config.ScorerClass == ScorerClassOfficialModelJudge && config.Model != "gpt-4o-2024-08-06" {
		return fmt.Errorf("official LongMemEval judge model must be gpt-4o-2024-08-06")
	}
	return nil
}

func validateRetrievalExecutionInput(input RetrievalExecutionInput) error {
	if strings.TrimSpace(input.Path) == "" {
		return fmt.Errorf("retrieval_input path is required")
	}
	if !sha256Pattern.MatchString(input.SHA256) {
		return fmt.Errorf("retrieval_input sha256 must be lowercase SHA-256")
	}
	if strings.TrimSpace(input.RunID) == "" {
		return fmt.Errorf("retrieval_input run_id is required")
	}
	if strings.TrimSpace(input.ImplementationRevision) == "" {
		return fmt.Errorf("retrieval_input implementation_revision is required")
	}
	if input.K != 10 {
		return fmt.Errorf("retrieval_input must use K=10")
	}
	return nil
}
