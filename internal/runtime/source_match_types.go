package runtime

import "time"

type SourceMatchStatus string

const (
	SourceMatchPending   SourceMatchStatus = "pending"
	SourceMatchMatched   SourceMatchStatus = "matched"
	SourceMatchAbstained SourceMatchStatus = "abstained"
	SourceMatchFailed    SourceMatchStatus = "failed"
)

type SourceMatchCandidate struct {
	MemoryID  string `json:"memory_id"`
	MemoryKey string `json:"memory_key"`
	Content   string `json:"content"`
	SourceRef string `json:"source_ref"`
}

type SourceMatchBeginRequest struct {
	OperationID    string
	SourceRef      string
	SourceContent  string
	ProviderName   string
	RequestedModel string
}

type SourceMatchCompletion struct {
	Decision               SourceMatchStatus
	SelectedMemoryKey      string
	ResolvedModel          string
	ProviderOutput         string
	ProviderArtifactSHA256 string
	Reason                 string
	FailureCode            string
}

type SourceMatchReceipt struct {
	ID                      string                     `json:"id"`
	ContinuityID            string                     `json:"continuity_id"`
	OperationID             string                     `json:"operation_id"`
	RequestFingerprint      string                     `json:"request_fingerprint"`
	SourceRef               string                     `json:"source_ref"`
	SourceContent           string                     `json:"source_content"`
	CandidateSet            []SourceMatchCandidate     `json:"candidate_set"`
	CandidateSetFingerprint string                     `json:"candidate_set_fingerprint"`
	ProviderName            string                     `json:"provider_name"`
	RequestedModel          string                     `json:"requested_model"`
	ResolvedModel           string                     `json:"resolved_model,omitempty"`
	Status                  SourceMatchStatus          `json:"status"`
	ProviderOutput          string                     `json:"provider_output,omitempty"`
	ProviderArtifactSHA256  string                     `json:"provider_artifact_sha256,omitempty"`
	Reason                  string                     `json:"reason,omitempty"`
	FailureCode             string                     `json:"failure_code,omitempty"`
	SelectedMemoryKey       string                     `json:"selected_memory_key,omitempty"`
	TargetMemoryID          string                     `json:"target_memory_id,omitempty"`
	ObservationID           string                     `json:"observation_id,omitempty"`
	CandidateMemoryID       string                     `json:"candidate_memory_id,omitempty"`
	Disposition             SourceCandidateDisposition `json:"disposition,omitempty"`
	CreatedAt               time.Time                  `json:"created_at"`
	CompletedAt             *time.Time                 `json:"completed_at,omitempty"`
	Replayed                bool                       `json:"replayed"`
}
