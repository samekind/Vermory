package utilityeval

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/provider"
	"vermory/internal/reality"
)

type ConditionID string

const (
	ConditionNoContext      ConditionID = "no_context"
	ConditionFullHistory    ConditionID = "full_history"
	ConditionPlainSummary   ConditionID = "plain_summary"
	ConditionPlainRetrieval ConditionID = "plain_retrieval"
	ConditionMem0OSS        ConditionID = "mem0_oss"
	ConditionVermoryNative  ConditionID = "vermory_native"
)

var FrozenConditions = []ConditionID{
	ConditionNoContext,
	ConditionFullHistory,
	ConditionPlainSummary,
	ConditionPlainRetrieval,
	ConditionMem0OSS,
	ConditionVermoryNative,
}

type ContextEvidence struct {
	Body       string `json:"body"`
	Source     string `json:"source"`
	SHA256     string `json:"sha256"`
	ByteSize   int    `json:"byte_size"`
	DeliveryID string `json:"delivery_id,omitempty"`
}

type CaseInput struct {
	ID             string                          `json:"id"`
	Task           string                          `json:"task"`
	Checks         reality.DownstreamTask          `json:"checks"`
	ScoringAliases map[string][]string             `json:"scoring_aliases,omitempty"`
	Context        map[ConditionID]ContextEvidence `json:"context"`
}

func NewCaseInput(c reality.Case, contexts map[ConditionID]string) (CaseInput, error) {
	if strings.TrimSpace(c.Manifest.ID) == "" {
		return CaseInput{}, errors.New("utilityeval: case id is required")
	}
	if strings.TrimSpace(c.Manifest.Task.Prompt) == "" {
		return CaseInput{}, fmt.Errorf("utilityeval: case %s has no task", c.Manifest.ID)
	}
	input := CaseInput{
		ID:      c.Manifest.ID,
		Task:    c.Manifest.Task.Prompt,
		Checks:  c.Manifest.Task,
		Context: make(map[ConditionID]ContextEvidence, len(contexts)),
	}
	for condition, body := range contexts {
		input.Context[condition] = NewContextEvidence(body, string(condition))
	}
	return input, input.Validate()
}

func NewContextEvidence(body, source string) ContextEvidence {
	body = strings.TrimSpace(body)
	return ContextEvidence{
		Body:     body,
		Source:   strings.TrimSpace(source),
		SHA256:   sha256Hex([]byte(body)),
		ByteSize: len([]byte(body)),
	}
}

func (input CaseInput) Validate() error {
	if strings.TrimSpace(input.ID) == "" {
		return errors.New("utilityeval: case id is required")
	}
	if strings.TrimSpace(input.Task) == "" {
		return fmt.Errorf("utilityeval: case %s task is required", input.ID)
	}
	if err := validateScoringAliases(input.ScoringAliases, input.Checks.DeterministicChecks); err != nil {
		return fmt.Errorf("utilityeval: case %s scoring aliases: %w", input.ID, err)
	}
	for _, condition := range FrozenConditions {
		evidence, ok := input.Context[condition]
		if !ok {
			return fmt.Errorf("utilityeval: case %s missing %s context", input.ID, condition)
		}
		if err := evidence.Validate(condition); err != nil {
			return fmt.Errorf("utilityeval: case %s %s: %w", input.ID, condition, err)
		}
	}
	return nil
}

func (e ContextEvidence) Validate(condition ConditionID) error {
	if strings.TrimSpace(e.Source) == "" {
		return errors.New("context source is required")
	}
	if e.ByteSize != len([]byte(e.Body)) {
		return errors.New("context byte size does not match body")
	}
	if e.SHA256 != sha256Hex([]byte(e.Body)) {
		return errors.New("context sha256 does not match body")
	}
	if condition != ConditionNoContext && strings.TrimSpace(e.Body) == "" {
		return errors.New("non-empty context is required")
	}
	return nil
}

type RunOptions struct {
	RunID           string
	ProfileID       string
	ProfileSHA256   string
	ProviderName    string
	ProviderMode    string
	Model           string
	System          string
	MaxTokens       int
	MaxContextBytes int
	ScorerVersion   string
	Workers         int
	DisableThinking bool
	Temperature     float64
	Inputs          []CaseInput
	Provider        provider.Provider
	Artifacts       artifact.Store
}

func (o RunOptions) Validate() error {
	if strings.TrimSpace(o.RunID) == "" {
		return errors.New("utilityeval: run id is required")
	}
	if strings.TrimSpace(o.ProfileID) == "" || len(o.ProfileSHA256) != 64 {
		return errors.New("utilityeval: runtime profile identity is invalid")
	}
	if strings.TrimSpace(o.ProviderName) == "" {
		return errors.New("utilityeval: provider name is required")
	}
	if strings.TrimSpace(o.Model) == "" {
		return errors.New("utilityeval: model is required")
	}
	if o.ScorerVersion != ScorerVersion {
		return fmt.Errorf("utilityeval: scorer is %q, expected %q", o.ScorerVersion, ScorerVersion)
	}
	if o.Workers < 1 || o.Workers > 16 {
		return errors.New("utilityeval: workers must be between 1 and 16")
	}
	if o.Provider == nil {
		return errors.New("utilityeval: provider is required")
	}
	if o.Artifacts == nil {
		return errors.New("utilityeval: artifact store is required")
	}
	if len(o.Inputs) == 0 {
		return errors.New("utilityeval: at least one case input is required")
	}
	for _, input := range o.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
		if o.MaxContextBytes > 0 {
			for condition, evidence := range input.Context {
				if evidence.ByteSize > o.MaxContextBytes {
					return fmt.Errorf("utilityeval: case %s %s context exceeds %d bytes", input.ID, condition, o.MaxContextBytes)
				}
			}
		}
	}
	return nil
}

type CheckResult struct {
	Check  string `json:"check"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

type Score struct {
	Success          bool          `json:"success"`
	NormalizedOutput string        `json:"normalized_output"`
	RequiredChecks   []CheckResult `json:"required_checks"`
	ForbiddenChecks  []CheckResult `json:"forbidden_checks"`
	ForbiddenHits    int           `json:"forbidden_hits"`
}

type CallResult struct {
	CaseID       string          `json:"case_id"`
	Condition    ConditionID     `json:"condition"`
	ProviderName string          `json:"provider_name"`
	Model        string          `json:"model"`
	Status       string          `json:"status"`
	Context      ContextEvidence `json:"context"`
	Output       string          `json:"output,omitempty"`
	OutputSHA256 string          `json:"output_sha256,omitempty"`
	LatencyMS    int64           `json:"latency_ms,omitempty"`
	Score        Score           `json:"score"`
	ErrorClass   string          `json:"error_class,omitempty"`
	Error        string          `json:"error,omitempty"`
	InputURI     string          `json:"input_uri,omitempty"`
	OutputURI    string          `json:"output_uri,omitempty"`
	ScoreURI     string          `json:"score_uri,omitempty"`
	RawURI       string          `json:"raw_uri,omitempty"`
}

type Aggregate struct {
	Calls               int     `json:"calls"`
	Completed           int     `json:"completed"`
	Failed              int     `json:"failed"`
	Successful          int     `json:"successful"`
	ForbiddenHits       int     `json:"forbidden_hits"`
	ContextBytes        int64   `json:"context_bytes"`
	AverageContextBytes float64 `json:"average_context_bytes"`
}

type Report struct {
	RunID           string                    `json:"run_id"`
	ProfileID       string                    `json:"profile_id"`
	ProfileSHA256   string                    `json:"profile_sha256"`
	ProviderName    string                    `json:"provider_name"`
	ProviderMode    string                    `json:"provider_mode"`
	Model           string                    `json:"model"`
	Scorer          string                    `json:"scorer"`
	Workers         int                       `json:"workers"`
	DisableThinking bool                      `json:"disable_thinking"`
	Temperature     float64                   `json:"temperature"`
	Results         []CallResult              `json:"results"`
	Aggregates      map[ConditionID]Aggregate `json:"aggregates"`
	ReportURI       string                    `json:"report_uri,omitempty"`
}

func (r Report) Validate() error {
	if strings.TrimSpace(r.RunID) == "" || strings.TrimSpace(r.ProfileID) == "" || len(r.ProfileSHA256) != 64 {
		return errors.New("utilityeval: report identity is invalid")
	}
	if strings.TrimSpace(r.ProviderName) == "" || strings.TrimSpace(r.Model) == "" || r.Scorer != ScorerVersion {
		return errors.New("utilityeval: report provider or scorer is invalid")
	}
	if r.Workers < 1 || r.Workers > 16 || len(r.Results) == 0 {
		return errors.New("utilityeval: report execution shape is invalid")
	}
	for _, result := range r.Results {
		if result.ProviderName != r.ProviderName || result.Model != r.Model {
			return errors.New("utilityeval: report result provider or model drifted")
		}
		if result.Status != "completed" && result.Status != "failed" {
			return fmt.Errorf("utilityeval: report result status %q is invalid", result.Status)
		}
	}
	if recomputed := aggregate(r.Results); !reflect.DeepEqual(recomputed, r.Aggregates) {
		return errors.New("utilityeval: report aggregates do not match call results")
	}
	return nil
}

func normalizeProviderError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "timeout"):
		return "provider_timeout"
	case strings.Contains(message, "rate") || strings.Contains(message, "quota"):
		return "provider_rate_limited"
	case strings.Contains(message, "auth") || strings.Contains(message, "401") || strings.Contains(message, "403"):
		return "provider_auth_failed"
	default:
		return "provider_failed"
	}
}
