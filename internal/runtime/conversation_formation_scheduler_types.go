package runtime

import "time"

type ConversationFormationScheduleState string

const (
	ConversationFormationIdle      ConversationFormationScheduleState = "idle"
	ConversationFormationPending   ConversationFormationScheduleState = "pending"
	ConversationFormationRunning   ConversationFormationScheduleState = "running"
	ConversationFormationRetryWait ConversationFormationScheduleState = "retry_wait"
)

type ConversationFormationSchedule struct {
	TenantID                 string                             `json:"tenant_id,omitempty"`
	ContinuityID             string                             `json:"continuity_id"`
	RequestedThroughSequence int64                              `json:"requested_through_sequence"`
	ProcessedThroughSequence int64                              `json:"processed_through_sequence"`
	State                    ConversationFormationScheduleState `json:"state"`
	LeaseToken               string                             `json:"lease_token,omitempty"`
	LeaseExpiresAt           *time.Time                         `json:"lease_expires_at,omitempty"`
	AttemptCount             int                                `json:"attempt_count"`
	NextAttemptAt            *time.Time                         `json:"next_attempt_at,omitempty"`
	WindowStartSequence      int64                              `json:"window_start_sequence,omitempty"`
	WindowEndSequence        int64                              `json:"window_end_sequence,omitempty"`
	WindowObservationIDs     []string                           `json:"window_observation_ids,omitempty"`
	WindowFingerprint        string                             `json:"window_fingerprint,omitempty"`
	ActiveOperationID        string                             `json:"active_operation_id,omitempty"`
	LastRunID                string                             `json:"last_run_id,omitempty"`
	LastStatus               SourceFormationStatus              `json:"last_status,omitempty"`
	LastFailureCode          string                             `json:"last_failure_code,omitempty"`
	CreatedAt                time.Time                          `json:"created_at"`
	UpdatedAt                time.Time                          `json:"updated_at"`
}

type ConversationFormationClaim struct {
	Schedule     ConversationFormationSchedule `json:"schedule"`
	Observations []ConversationObservation     `json:"observations"`
}

type ConversationFormationWorkerOptions struct {
	TenantID      string
	PollInterval  time.Duration
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	MaxAttempts   int
}

type ConversationFormationWorkerResult struct {
	TenantID                 string                `json:"tenant_id"`
	ContinuityID             string                `json:"continuity_id,omitempty"`
	OperationID              string                `json:"operation_id,omitempty"`
	FormationRunID           string                `json:"formation_run_id,omitempty"`
	FormationStatus          SourceFormationStatus `json:"formation_status,omitempty"`
	FailureCode              string                `json:"failure_code,omitempty"`
	ProcessedThroughSequence int64                 `json:"processed_through_sequence,omitempty"`
	Found                    bool                  `json:"found"`
	Replayed                 bool                  `json:"replayed"`
}
