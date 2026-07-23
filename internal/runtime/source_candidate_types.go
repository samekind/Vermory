package runtime

type SourceCandidateDisposition string

const (
	SourceCandidateNew         SourceCandidateDisposition = "new"
	SourceCandidateReplacement SourceCandidateDisposition = "replacement"
	SourceCandidateUnchanged   SourceCandidateDisposition = "unchanged"
)

type SourceCandidateReceipt struct {
	Disposition    SourceCandidateDisposition `json:"disposition"`
	MemoryKey      string                     `json:"memory_key"`
	TargetMemoryID string                     `json:"target_memory_id,omitempty"`
	Observation    ObservationReceipt         `json:"observation"`
	Candidate      MemoryReceipt              `json:"candidate"`
	Replayed       bool                       `json:"replayed"`
}
