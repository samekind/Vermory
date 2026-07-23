package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultRecentConversationObservations = 12
	maxRecentConversationObservations     = 50
	maxConversationToolResultsPerTurn     = 16
	maxConversationToolResultBytes        = 8 * 1024
	maxConversationToolResultTotalBytes   = 64 * 1024
	maxConversationCheckpointBytes        = 32 * 1024
	defaultConversationOperationLease     = 2 * time.Minute
)

type ClientOperationProtocol string

const (
	ClientOperationBoundedV1 ClientOperationProtocol = "bounded_v1"
	ClientOperationLeasedV1  ClientOperationProtocol = "leased_v1"
)

type ChatTurnStatus string

const (
	ChatTurnInProgress ChatTurnStatus = "in_progress"
	ChatTurnCompleted  ChatTurnStatus = "completed"
	ChatTurnFailed     ChatTurnStatus = "failed"
	ChatTurnCancelled  ChatTurnStatus = "cancelled"
)

type ConversationAnchor struct {
	Channel  string `json:"channel"`
	ThreadID string `json:"thread_id"`
}

func (a ConversationAnchor) Normalized() (ConversationAnchor, error) {
	a.Channel = strings.TrimSpace(a.Channel)
	a.ThreadID = strings.TrimSpace(a.ThreadID)
	if a.Channel == "" {
		return ConversationAnchor{}, fmt.Errorf("conversation channel is required")
	}
	if a.ThreadID == "" {
		return ConversationAnchor{}, fmt.Errorf("conversation thread_id is required")
	}
	if len(a.Channel) > 128 {
		return ConversationAnchor{}, fmt.Errorf("conversation channel is too long")
	}
	if len(a.ThreadID) > 512 {
		return ConversationAnchor{}, fmt.Errorf("conversation thread_id is too long")
	}
	return a, nil
}

type ConversationResolution struct {
	Status       ResolutionStatus `json:"status"`
	ContinuityID string           `json:"continuity_id"`
	Channel      string           `json:"channel"`
	ThreadID     string           `json:"thread_id"`
	Created      bool             `json:"created"`
}

type ConversationObservation struct {
	ID       string          `json:"id"`
	Sequence int64           `json:"sequence"`
	Kind     ObservationKind `json:"kind"`
	Content  string          `json:"content"`
}

type ChatTurnRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	Message     string             `json:"message"`
}

func (r *ChatTurnRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.Message = strings.TrimSpace(r.Message)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.Message == "" {
		return fmt.Errorf("message is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if len(r.Message) > 128*1024 {
		return fmt.Errorf("message is too long")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type ChatTurnReceipt struct {
	ID                     string                  `json:"turn_id"`
	OperationID            string                  `json:"operation_id"`
	Status                 ChatTurnStatus          `json:"status"`
	Protocol               ClientOperationProtocol `json:"protocol"`
	ContinuityID           string                  `json:"continuity_id"`
	DeliveryID             string                  `json:"delivery_id,omitempty"`
	UserObservationID      string                  `json:"user_observation_id"`
	AssistantObservationID string                  `json:"assistant_observation_id,omitempty"`
	Answer                 string                  `json:"answer,omitempty"`
	Model                  string                  `json:"model,omitempty"`
	FailureCode            string                  `json:"failure_code,omitempty"`
	FailureMessage         string                  `json:"-"`
	CancellationCode       string                  `json:"cancellation_code,omitempty"`
	CancellationMessage    string                  `json:"-"`
	AttemptID              string                  `json:"attempt_id,omitempty"`
	LeaseGeneration        int64                   `json:"lease_generation,omitempty"`
	LeaseExpiresAt         *time.Time              `json:"lease_expires_at,omitempty"`
	LastHeartbeatAt        *time.Time              `json:"last_heartbeat_at,omitempty"`
	CheckpointSequence     int64                   `json:"checkpoint_sequence"`
	Checkpoint             json.RawMessage         `json:"checkpoint,omitempty"`
	CheckpointUpdatedAt    *time.Time              `json:"checkpoint_updated_at,omitempty"`
	Replayed               bool                    `json:"replayed"`
	RequestFingerprint     string                  `json:"-"`
	AnswerFingerprint      string                  `json:"-"`
	CheckpointFingerprint  string                  `json:"-"`
}

type ExternalConversationTurnRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	Message     string             `json:"message"`
}

func (r *ExternalConversationTurnRequest) Validate() error {
	request := ChatTurnRequest{OperationID: r.OperationID, Anchor: r.Anchor, Message: r.Message}
	if err := request.Validate(); err != nil {
		return err
	}
	r.OperationID = request.OperationID
	r.Anchor = request.Anchor
	r.Message = request.Message
	return nil
}

type PreparedConversationTurn struct {
	ChatTurnReceipt
	Context string `json:"context,omitempty"`
}

type LeasedConversationOperationRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	Message     string             `json:"message"`
}

func (r *LeasedConversationOperationRequest) Validate() error {
	request := ExternalConversationTurnRequest{OperationID: r.OperationID, Anchor: r.Anchor, Message: r.Message}
	if err := request.Validate(); err != nil {
		return err
	}
	r.OperationID = request.OperationID
	r.Anchor = request.Anchor
	r.Message = request.Message
	return nil
}

type LeasedConversationOperationMutation struct {
	OperationID     string             `json:"operation_id"`
	Anchor          ConversationAnchor `json:"-"`
	AttemptID       string             `json:"attempt_id"`
	LeaseGeneration int64              `json:"lease_generation"`
}

func (r *LeasedConversationOperationMutation) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.AttemptID = strings.TrimSpace(r.AttemptID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if r.AttemptID == "" {
		return fmt.Errorf("attempt_id is required")
	}
	if len(r.AttemptID) > 64 {
		return fmt.Errorf("attempt_id is too long")
	}
	if r.LeaseGeneration <= 0 {
		return fmt.Errorf("lease_generation must be positive")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type HeartbeatLeasedConversationOperationRequest struct {
	LeasedConversationOperationMutation
}

func (r *HeartbeatLeasedConversationOperationRequest) Validate() error {
	return r.LeasedConversationOperationMutation.Validate()
}

type CheckpointLeasedConversationOperationRequest struct {
	LeasedConversationOperationMutation
	Sequence   int64           `json:"sequence"`
	Checkpoint json.RawMessage `json:"checkpoint"`
}

func (r *CheckpointLeasedConversationOperationRequest) Validate() error {
	if err := r.LeasedConversationOperationMutation.Validate(); err != nil {
		return err
	}
	if r.Sequence <= 0 {
		return fmt.Errorf("checkpoint sequence must be positive")
	}
	normalized, err := normalizeConversationCheckpoint(r.Checkpoint)
	if err != nil {
		return err
	}
	r.Checkpoint = normalized
	return nil
}

type CompleteLeasedConversationOperationRequest struct {
	LeasedConversationOperationMutation
	Answer string `json:"answer"`
	Model  string `json:"model"`
}

func (r *CompleteLeasedConversationOperationRequest) Validate() error {
	if err := r.LeasedConversationOperationMutation.Validate(); err != nil {
		return err
	}
	r.Answer = strings.TrimSpace(r.Answer)
	r.Model = strings.TrimSpace(r.Model)
	if r.Answer == "" {
		return fmt.Errorf("answer is required")
	}
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.Answer) > 128*1024 {
		return fmt.Errorf("answer is too long")
	}
	if len(r.Model) > 512 {
		return fmt.Errorf("model is too long")
	}
	return nil
}

type FailLeasedConversationOperationRequest struct {
	LeasedConversationOperationMutation
	FailureCode    string `json:"failure_code"`
	FailureMessage string `json:"failure_message"`
}

func (r *FailLeasedConversationOperationRequest) Validate() error {
	if err := r.LeasedConversationOperationMutation.Validate(); err != nil {
		return err
	}
	r.FailureCode = strings.TrimSpace(r.FailureCode)
	r.FailureMessage = strings.TrimSpace(r.FailureMessage)
	if r.FailureCode == "" {
		return fmt.Errorf("failure_code is required")
	}
	if len(r.FailureCode) > 128 {
		return fmt.Errorf("failure_code is too long")
	}
	if len(r.FailureMessage) > 512 {
		return fmt.Errorf("failure_message is too long")
	}
	return nil
}

type CancelLeasedConversationOperationRequest struct {
	LeasedConversationOperationMutation
	CancellationCode    string `json:"cancellation_code"`
	CancellationMessage string `json:"cancellation_message"`
}

func (r *CancelLeasedConversationOperationRequest) Validate() error {
	if err := r.LeasedConversationOperationMutation.Validate(); err != nil {
		return err
	}
	r.CancellationCode = strings.TrimSpace(r.CancellationCode)
	r.CancellationMessage = strings.TrimSpace(r.CancellationMessage)
	if r.CancellationCode == "" {
		return fmt.Errorf("cancellation_code is required")
	}
	if len(r.CancellationCode) > 128 {
		return fmt.Errorf("cancellation_code is too long")
	}
	if len(r.CancellationMessage) > 512 {
		return fmt.Errorf("cancellation_message is too long")
	}
	return nil
}

func normalizeConversationCheckpoint(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, fmt.Errorf("checkpoint must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("checkpoint contains trailing JSON")
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode checkpoint: %w", err)
	}
	if len(normalized) > maxConversationCheckpointBytes {
		return nil, fmt.Errorf("checkpoint is too large")
	}
	return normalized, nil
}

type CompleteExternalConversationTurnRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	Answer      string             `json:"answer"`
	Model       string             `json:"model"`
}

func (r *CompleteExternalConversationTurnRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.Answer = strings.TrimSpace(r.Answer)
	r.Model = strings.TrimSpace(r.Model)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.Answer == "" {
		return fmt.Errorf("answer is required")
	}
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if len(r.Answer) > 128*1024 {
		return fmt.Errorf("answer is too long")
	}
	if len(r.Model) > 512 {
		return fmt.Errorf("model is too long")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type FailExternalConversationTurnRequest struct {
	OperationID    string             `json:"operation_id"`
	Anchor         ConversationAnchor `json:"-"`
	FailureCode    string             `json:"failure_code"`
	FailureMessage string             `json:"failure_message"`
}

type RecordConversationToolResultRequest struct {
	OperationID     string             `json:"operation_id"`
	Anchor          ConversationAnchor `json:"-"`
	RunID           string             `json:"run_id"`
	ToolName        string             `json:"tool_name"`
	ToolCallID      string             `json:"tool_call_id"`
	Content         string             `json:"content"`
	AttemptID       string             `json:"attempt_id,omitempty"`
	LeaseGeneration int64              `json:"lease_generation,omitempty"`
}

func (r *RecordConversationToolResultRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.RunID = strings.TrimSpace(r.RunID)
	r.ToolName = strings.TrimSpace(r.ToolName)
	r.ToolCallID = strings.TrimSpace(r.ToolCallID)
	r.Content = strings.TrimSpace(r.Content)
	r.AttemptID = strings.TrimSpace(r.AttemptID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if r.ToolName == "" {
		return fmt.Errorf("tool_name is required")
	}
	if r.ToolCallID == "" {
		return fmt.Errorf("tool_call_id is required")
	}
	if r.Content == "" {
		return fmt.Errorf("content is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if len(r.RunID) > 512 {
		return fmt.Errorf("run_id is too long")
	}
	if len(r.ToolName) > 128 || !validConversationToolName(r.ToolName) {
		return fmt.Errorf("tool_name is unsupported")
	}
	if len(r.ToolCallID) > 512 {
		return fmt.Errorf("tool_call_id is too long")
	}
	if (r.AttemptID == "") != (r.LeaseGeneration == 0) {
		return fmt.Errorf("attempt_id and lease_generation must be supplied together")
	}
	if len(r.AttemptID) > 64 {
		return fmt.Errorf("attempt_id is too long")
	}
	if r.LeaseGeneration < 0 {
		return fmt.Errorf("lease_generation must not be negative")
	}
	if !utf8.ValidString(r.Content) {
		return fmt.Errorf("content is not valid UTF-8")
	}
	if len(r.Content) > maxConversationToolResultBytes {
		return fmt.Errorf("content is too long")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

func validConversationToolName(value string) bool {
	for index, current := range []byte(value) {
		if current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || current >= '0' && current <= '9' {
			continue
		}
		if index > 0 && (current == '_' || current == '-' || current == '.' || current == ':') {
			continue
		}
		return false
	}
	return true
}

type ConversationToolResultReceipt struct {
	TurnID        string `json:"turn_id"`
	ObservationID string `json:"observation_id"`
	ToolName      string `json:"tool_name"`
	Replayed      bool   `json:"replayed"`
}

func (r *FailExternalConversationTurnRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.FailureCode = strings.TrimSpace(r.FailureCode)
	r.FailureMessage = strings.TrimSpace(r.FailureMessage)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.FailureCode == "" {
		return fmt.Errorf("failure_code is required")
	}
	if len(r.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if len(r.FailureCode) > 128 {
		return fmt.Errorf("failure_code is too long")
	}
	if len(r.FailureMessage) > 512 {
		return fmt.Errorf("failure_message is too long")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type ConversationServiceConfig struct {
	MemoryLimit            int
	RecentLimit            int
	Retriever              MemoryRetriever
	OperationLeaseDuration time.Duration
}

type ReviewConversationCandidateRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	MemoryID    string             `json:"memory_id"`
}

type ConversationReviewCandidate struct {
	CandidateMemoryID   string                  `json:"candidate_memory_id"`
	MemoryKey           string                  `json:"memory_key"`
	Content             string                  `json:"content"`
	SourceQuote         string                  `json:"source_quote"`
	SourceObservationID string                  `json:"source_observation_id"`
	SourceKind          ObservationKind         `json:"source_kind"`
	SourceLabel         string                  `json:"source_label,omitempty"`
	Decision            SourceFormationDecision `json:"decision"`
	TargetMemoryID      string                  `json:"target_memory_id,omitempty"`
	CreatedAt           time.Time               `json:"created_at"`
}

type ConversationReviewInbox struct {
	Resolution ConversationResolution        `json:"resolution"`
	Candidates []ConversationReviewCandidate `json:"candidates"`
}

func (r *ReviewConversationCandidateRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.MemoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type ConfirmConversationMemoryRequest struct {
	OperationID   string             `json:"operation_id"`
	Anchor        ConversationAnchor `json:"-"`
	ObservationID string             `json:"observation_id"`
}

func (r *ConfirmConversationMemoryRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.ObservationID = strings.TrimSpace(r.ObservationID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.ObservationID == "" {
		return fmt.Errorf("observation_id is required")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type CorrectConversationMemoryRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	MemoryID    string             `json:"memory_id"`
	Content     string             `json:"content"`
}

func (r *CorrectConversationMemoryRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	r.Content = strings.TrimSpace(r.Content)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.MemoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	if r.Content == "" {
		return fmt.Errorf("content is required")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type ForgetConversationMemoryRequest struct {
	OperationID string             `json:"operation_id"`
	Anchor      ConversationAnchor `json:"-"`
	MemoryID    string             `json:"memory_id"`
}

func (r *ForgetConversationMemoryRequest) Validate() error {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if r.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if r.MemoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	anchor, err := r.Anchor.Normalized()
	if err != nil {
		return err
	}
	r.Anchor = anchor
	return nil
}

type ConversationInspection struct {
	Resolution   ConversationResolution    `json:"conversation"`
	Observations []ConversationObservation `json:"observations"`
	Memories     []GovernedMemory          `json:"memories"`
}

func (c ConversationServiceConfig) normalized() ConversationServiceConfig {
	if c.MemoryLimit <= 0 {
		c.MemoryLimit = defaultContextItems
	}
	if c.MemoryLimit > maxContextItems {
		c.MemoryLimit = maxContextItems
	}
	if c.RecentLimit <= 0 {
		c.RecentLimit = defaultRecentConversationObservations
	}
	if c.RecentLimit > maxRecentConversationObservations {
		c.RecentLimit = maxRecentConversationObservations
	}
	if c.OperationLeaseDuration <= 0 {
		c.OperationLeaseDuration = defaultConversationOperationLease
	}
	return c
}
