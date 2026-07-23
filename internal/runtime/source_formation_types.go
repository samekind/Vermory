package runtime

import "time"

type SourceFormationInputKind string

const (
	SourceFormationInputDocument     SourceFormationInputKind = "document"
	SourceFormationInputConversation SourceFormationInputKind = "conversation"
)

type SourceFormationStatus string

const (
	SourceFormationPending   SourceFormationStatus = "pending"
	SourceFormationCompleted SourceFormationStatus = "completed"
	SourceFormationAbstained SourceFormationStatus = "abstained"
	SourceFormationFailed    SourceFormationStatus = "failed"
)

type SourceFormationDecision string

const (
	SourceFormationNew       SourceFormationDecision = "new"
	SourceFormationUpdate    SourceFormationDecision = "update"
	SourceFormationUnchanged SourceFormationDecision = "unchanged"
)

type SourceFormationBeginRequest struct {
	OperationID    string
	SourceRef      string
	SourceSHA256   string
	SourceBytes    int
	InputKind      SourceFormationInputKind
	InputManifest  []SourceFormationInputObservation
	ProviderName   string
	RequestedModel string
}

type SourceFormationInputObservation struct {
	ID       string          `json:"id"`
	Sequence int64           `json:"sequence"`
	Kind     ObservationKind `json:"kind"`
	SHA256   string          `json:"sha256"`
	Bytes    int             `json:"bytes"`
}

type SourceFormationProviderItem struct {
	Decision            SourceFormationDecision `json:"decision"`
	MemoryKey           string                  `json:"memory_key"`
	SourceObservationID string                  `json:"source_observation_id,omitempty"`
	Quote               string                  `json:"quote"`
	Occurrence          int                     `json:"occurrence"`
	Content             string                  `json:"content"`
	Reason              string                  `json:"reason"`
}

type SourceFormationCompletion struct {
	Status                 SourceFormationStatus
	ResolvedModel          string
	ProviderOutput         string
	ProviderArtifactSHA256 string
	Reason                 string
	FailureCode            string
	Items                  []SourceFormationProviderItem
}

type SourceFormationItemReceipt struct {
	ID                    string                  `json:"id"`
	Ordinal               int                     `json:"ordinal"`
	Decision              SourceFormationDecision `json:"decision"`
	MemoryKey             string                  `json:"memory_key"`
	Quote                 string                  `json:"quote"`
	Occurrence            int                     `json:"occurrence"`
	ByteStart             int                     `json:"byte_start"`
	ByteEnd               int                     `json:"byte_end"`
	Content               string                  `json:"content"`
	Reason                string                  `json:"reason"`
	TargetMemoryID        string                  `json:"target_memory_id,omitempty"`
	EvidenceObservationID string                  `json:"evidence_observation_id,omitempty"`
	ObservationID         string                  `json:"observation_id"`
	CandidateMemoryID     string                  `json:"candidate_memory_id,omitempty"`
	CandidateStatus       string                  `json:"candidate_status,omitempty"`
	CreatedAt             time.Time               `json:"created_at"`
}

type SourceFormationReceipt struct {
	ID                        string                            `json:"id"`
	ContinuityID              string                            `json:"continuity_id"`
	OperationID               string                            `json:"operation_id"`
	RequestFingerprint        string                            `json:"request_fingerprint"`
	SourceRef                 string                            `json:"source_ref"`
	SourceSHA256              string                            `json:"source_sha256"`
	SourceBytes               int                               `json:"source_bytes"`
	InputKind                 SourceFormationInputKind          `json:"input_kind"`
	InputManifest             []SourceFormationInputObservation `json:"input_manifest"`
	InputManifestFingerprint  string                            `json:"input_manifest_fingerprint"`
	ActiveSnapshot            []SourceMatchCandidate            `json:"active_snapshot"`
	ActiveSnapshotFingerprint string                            `json:"active_snapshot_fingerprint"`
	ProviderName              string                            `json:"provider_name"`
	RequestedModel            string                            `json:"requested_model"`
	ResolvedModel             string                            `json:"resolved_model,omitempty"`
	Status                    SourceFormationStatus             `json:"status"`
	ProviderOutput            string                            `json:"provider_output,omitempty"`
	ProviderArtifactSHA256    string                            `json:"provider_artifact_sha256,omitempty"`
	Reason                    string                            `json:"reason,omitempty"`
	FailureCode               string                            `json:"failure_code,omitempty"`
	Items                     []SourceFormationItemReceipt      `json:"items"`
	CreatedAt                 time.Time                         `json:"created_at"`
	CompletedAt               *time.Time                        `json:"completed_at,omitempty"`
	Replayed                  bool                              `json:"replayed"`
}
