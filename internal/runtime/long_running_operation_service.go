package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"vermory/internal/redaction"
)

func (s *ConversationService) PrepareLeasedOperation(ctx context.Context, request LeasedConversationOperationRequest) (PreparedConversationTurn, error) {
	return s.prepareLeasedOperation(ctx, request, false)
}

func (s *ConversationService) ReclaimLeasedOperation(ctx context.Context, request LeasedConversationOperationRequest) (PreparedConversationTurn, error) {
	return s.prepareLeasedOperation(ctx, request, true)
}

func (s *ConversationService) prepareLeasedOperation(
	ctx context.Context,
	request LeasedConversationOperationRequest,
	reclaim bool,
) (PreparedConversationTurn, error) {
	if err := s.configured(); err != nil {
		return PreparedConversationTurn{}, err
	}
	if err := request.Validate(); err != nil {
		return PreparedConversationTurn{}, err
	}
	var (
		resolution ConversationResolution
		err        error
	)
	if reclaim {
		resolution, err = s.confirmedConversation(ctx, request.Anchor)
	} else {
		resolution, err = s.store.ResolveOrCreateConversation(ctx, s.tenantID, request.Anchor)
	}
	if err != nil {
		return PreparedConversationTurn{}, err
	}
	turnRequest := ChatTurnRequest{OperationID: request.OperationID, Anchor: request.Anchor, Message: request.Message}
	turn, err := s.store.BeginLeasedConversationTurn(
		ctx, s.tenantID, resolution.ContinuityID, turnRequest, s.config.OperationLeaseDuration, reclaim,
	)
	if err != nil {
		return PreparedConversationTurn{}, err
	}
	fail := func(ctx context.Context, turn ChatTurnReceipt, code string, cause error) (ChatTurnReceipt, error) {
		message := strings.TrimSpace(cause.Error())
		if len(message) > 512 {
			message = message[:512]
		}
		return s.store.FailLeasedConversationTurn(ctx, s.tenantID, turn.ContinuityID, FailLeasedConversationOperationRequest{
			LeasedConversationOperationMutation: LeasedConversationOperationMutation{
				OperationID: turn.OperationID, Anchor: request.Anchor,
				AttemptID: turn.AttemptID, LeaseGeneration: turn.LeaseGeneration,
			},
			FailureCode: code, FailureMessage: message,
		})
	}
	return s.finishPreparedConversationTurn(ctx, resolution, turn, turnRequest, false, fail)
}

func (s *ConversationService) HeartbeatLeasedOperation(ctx context.Context, request HeartbeatLeasedConversationOperationRequest) (ChatTurnReceipt, error) {
	if err := s.configured(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ChatTurnReceipt{}, err
	}
	resolution, err := s.confirmedConversation(ctx, request.Anchor)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.store.HeartbeatLeasedConversationTurn(
		ctx, s.tenantID, resolution.ContinuityID, request.LeasedConversationOperationMutation, s.config.OperationLeaseDuration,
	)
}

func (s *ConversationService) CheckpointLeasedOperation(ctx context.Context, request CheckpointLeasedConversationOperationRequest) (ChatTurnReceipt, error) {
	if err := s.configured(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if conversationCheckpointContainsSensitive(request.Checkpoint) {
		return ChatTurnReceipt{}, fmt.Errorf("checkpoint contains sensitive data")
	}
	resolution, err := s.confirmedConversation(ctx, request.Anchor)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.store.CheckpointLeasedConversationTurn(
		ctx, s.tenantID, resolution.ContinuityID, request, s.config.OperationLeaseDuration,
	)
}

func conversationCheckpointContainsSensitive(checkpoint json.RawMessage) bool {
	if redaction.ContainsSensitive(string(checkpoint)) {
		return true
	}
	var value any
	if err := json.Unmarshal(checkpoint, &value); err != nil {
		return true
	}
	return checkpointValueContainsSensitive("", value)
}

func checkpointValueContainsSensitive(key string, value any) bool {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	switch normalizedKey {
	case "api_token", "api-token", "api_key", "api-key", "secret", "token", "password", "authorization", "credential", "private_key", "private-key":
		return true
	}
	switch current := value.(type) {
	case map[string]any:
		for childKey, child := range current {
			if checkpointValueContainsSensitive(childKey, child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if checkpointValueContainsSensitive("", child) {
				return true
			}
		}
	case string:
		return redaction.ContainsSensitive(current)
	}
	return false
}

func (s *ConversationService) CompleteLeasedOperation(ctx context.Context, request CompleteLeasedConversationOperationRequest) (ChatTurnReceipt, error) {
	if err := s.configured(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ChatTurnReceipt{}, err
	}
	resolution, err := s.confirmedConversation(ctx, request.Anchor)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.store.CompleteLeasedConversationTurn(ctx, s.tenantID, resolution.ContinuityID, request)
}

func (s *ConversationService) FailLeasedOperation(ctx context.Context, request FailLeasedConversationOperationRequest) (ChatTurnReceipt, error) {
	if err := s.configured(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ChatTurnReceipt{}, err
	}
	resolution, err := s.confirmedConversation(ctx, request.Anchor)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.store.FailLeasedConversationTurn(ctx, s.tenantID, resolution.ContinuityID, request)
}

func (s *ConversationService) CancelLeasedOperation(ctx context.Context, request CancelLeasedConversationOperationRequest) (ChatTurnReceipt, error) {
	if err := s.configured(); err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ChatTurnReceipt{}, err
	}
	resolution, err := s.confirmedConversation(ctx, request.Anchor)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.store.CancelLeasedConversationTurn(ctx, s.tenantID, resolution.ContinuityID, request)
}
