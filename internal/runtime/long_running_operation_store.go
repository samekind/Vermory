package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) BeginLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request ChatTurnRequest,
	leaseDuration time.Duration,
	reclaim bool,
) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin leased conversation operation: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, found, err := lookupConversationTurnForUpdateTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if found {
		if existing.ContinuityID != continuityID || existing.RequestFingerprint != conversationContentFingerprint(request.Message) {
			return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to another conversation turn")
		}
		if existing.Protocol != ClientOperationLeasedV1 {
			return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to the bounded client protocol")
		}
		if existing.Status != ChatTurnInProgress || !reclaim {
			existing.Replayed = true
			if err := tx.Commit(ctx); err != nil {
				return ChatTurnReceipt{}, fmt.Errorf("commit replayed leased conversation operation: %w", err)
			}
			return existing, nil
		}

		result, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET attempt_id = gen_random_uuid(),
    lease_generation = lease_generation + 1,
    last_heartbeat_at = statement_timestamp(),
    lease_expires_at = statement_timestamp() + ($1::bigint * interval '1 microsecond'),
    updated_at = statement_timestamp()
WHERE id = $2::uuid
  AND tenant_id = $3
  AND status = 'in_progress'
  AND operation_protocol = 'leased_v1'
  AND lease_expires_at <= statement_timestamp()`, leaseDuration.Microseconds(), existing.ID, tenantID)
		if err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("reclaim leased conversation operation: %w", err)
		}
		if result.RowsAffected() != 1 {
			return ChatTurnReceipt{}, fmt.Errorf("leased conversation operation has a live lease")
		}
		reclaimed, found, err := lookupConversationTurnTx(ctx, tx, tenantID, request.OperationID)
		if err != nil {
			return ChatTurnReceipt{}, err
		}
		if !found {
			return ChatTurnReceipt{}, fmt.Errorf("reclaimed conversation operation disappeared")
		}
		if err := tx.Commit(ctx); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("commit reclaimed conversation operation: %w", err)
		}
		return reclaimed, nil
	}
	if reclaim {
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation does not exist")
	}

	observation, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: request.OperationID,
		Kind:        ObservationKindUserMessage,
		Content:     request.Message,
		SourceRef:   conversationUserSourceRef,
	})
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO conversation_turns (
  tenant_id, continuity_id, operation_id, status, operation_protocol,
  user_observation_id, request_fingerprint, attempt_id, lease_generation,
  last_heartbeat_at, lease_expires_at
)
VALUES (
  $1, $2::uuid, $3, 'in_progress', 'leased_v1',
  $4::uuid, $5, gen_random_uuid(), 1,
  statement_timestamp(), statement_timestamp() + ($6::bigint * interval '1 microsecond')
)`, tenantID, continuityID, request.OperationID, observation.ObservationID,
		conversationContentFingerprint(request.Message), leaseDuration.Microseconds()); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("create leased conversation operation: %w", err)
	}
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("created leased conversation operation is missing")
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit leased conversation operation: %w", err)
	}
	return receipt, nil
}

func (s *Store) HeartbeatLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request LeasedConversationOperationMutation,
	leaseDuration time.Duration,
) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin leased conversation heartbeat: %w", err)
	}
	defer tx.Rollback(ctx)

	turn, err := lockCurrentLeasedConversationTurn(ctx, tx, tenantID, continuityID, request)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := requireLeasedTurnInProgress(turn); err != nil {
		return ChatTurnReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET last_heartbeat_at = statement_timestamp(),
    lease_expires_at = statement_timestamp() + ($1::bigint * interval '1 microsecond'),
    updated_at = statement_timestamp()
WHERE id = $2::uuid`, leaseDuration.Microseconds(), turn.ID); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("heartbeat leased conversation operation: %w", err)
	}
	receipt, err := loadLeasedConversationTurnTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit leased conversation heartbeat: %w", err)
	}
	return receipt, nil
}

func (s *Store) CheckpointLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request CheckpointLeasedConversationOperationRequest,
	leaseDuration time.Duration,
) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin leased conversation checkpoint: %w", err)
	}
	defer tx.Rollback(ctx)

	turn, err := lockCurrentLeasedConversationTurn(ctx, tx, tenantID, continuityID, request.LeasedConversationOperationMutation)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := requireLeasedTurnInProgress(turn); err != nil {
		return ChatTurnReceipt{}, err
	}
	fingerprint := conversationContentFingerprint(string(request.Checkpoint))
	switch {
	case request.Sequence < turn.CheckpointSequence:
		return ChatTurnReceipt{}, fmt.Errorf("checkpoint sequence is stale")
	case request.Sequence == turn.CheckpointSequence && fingerprint != turn.CheckpointFingerprint:
		return ChatTurnReceipt{}, fmt.Errorf("checkpoint sequence is already bound to different content")
	case request.Sequence == turn.CheckpointSequence:
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET last_heartbeat_at = statement_timestamp(),
    lease_expires_at = statement_timestamp() + ($1::bigint * interval '1 microsecond'),
    updated_at = statement_timestamp()
WHERE id = $2::uuid`, leaseDuration.Microseconds(), turn.ID); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("renew replayed leased checkpoint: %w", err)
		}
	default:
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET checkpoint_sequence = $1,
    checkpoint_payload = $2::jsonb,
    checkpoint_fingerprint = $3,
    checkpoint_updated_at = statement_timestamp(),
    last_heartbeat_at = statement_timestamp(),
    lease_expires_at = statement_timestamp() + ($4::bigint * interval '1 microsecond'),
    updated_at = statement_timestamp()
WHERE id = $5::uuid`, request.Sequence, string(request.Checkpoint), fingerprint, leaseDuration.Microseconds(), turn.ID); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("checkpoint leased conversation operation: %w", err)
		}
	}
	receipt, err := loadLeasedConversationTurnTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	receipt.Replayed = request.Sequence == turn.CheckpointSequence
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit leased conversation checkpoint: %w", err)
	}
	return receipt, nil
}

func (s *Store) CompleteLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request CompleteLeasedConversationOperationRequest,
) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin leased conversation completion: %w", err)
	}
	defer tx.Rollback(ctx)

	turn, err := lockCurrentLeasedConversationTurn(ctx, tx, tenantID, continuityID, request.LeasedConversationOperationMutation)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	switch turn.Status {
	case ChatTurnCompleted:
		if turn.AnswerFingerprint != conversationContentFingerprint(request.Answer) || turn.Model != request.Model {
			return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to another conversation completion")
		}
		turn.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("commit replayed leased conversation completion: %w", err)
		}
		return turn, nil
	case ChatTurnFailed:
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation is already failed")
	case ChatTurnCancelled:
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation is already cancelled")
	case ChatTurnInProgress:
	default:
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation has invalid status %q", turn.Status)
	}
	if turn.DeliveryID == "" {
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation has no prepared delivery")
	}
	var validDelivery bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM memory_deliveries
  WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
)`, turn.DeliveryID, tenantID, continuityID).Scan(&validDelivery); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("check leased conversation delivery: %w", err)
	}
	if !validDelivery {
		return ChatTurnReceipt{}, fmt.Errorf("delivery does not belong to this conversation")
	}
	assistant, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: request.OperationID + ":assistant",
		Kind:        ObservationKindAssistantMessage,
		Content:     request.Answer,
		SourceRef:   "provider:" + request.Model,
	})
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET status = 'completed', assistant_observation_id = $1::uuid,
    answer = $2, answer_fingerprint = $3, provider_model = $4,
    failure_code = '', failure_message = '', lease_expires_at = NULL,
    updated_at = statement_timestamp()
WHERE id = $5::uuid`, assistant.ObservationID, request.Answer,
		conversationContentFingerprint(request.Answer), request.Model, turn.ID); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("complete leased conversation operation: %w", err)
	}
	if _, err := enqueueConversationFormationTx(ctx, tx, tenantID, continuityID, turn.ID, turn.UserObservationID); err != nil {
		return ChatTurnReceipt{}, err
	}
	receipt, err := loadLeasedConversationTurnTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit leased conversation completion: %w", err)
	}
	return receipt, nil
}

func (s *Store) FailLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request FailLeasedConversationOperationRequest,
) (ChatTurnReceipt, error) {
	var err error
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.terminateLeasedConversationTurn(ctx, tenantID, continuityID, request.LeasedConversationOperationMutation,
		ChatTurnFailed, request.FailureCode, request.FailureMessage)
}

func (s *Store) CancelLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	request CancelLeasedConversationOperationRequest,
) (ChatTurnReceipt, error) {
	var err error
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	return s.terminateLeasedConversationTurn(ctx, tenantID, continuityID, request.LeasedConversationOperationMutation,
		ChatTurnCancelled, request.CancellationCode, request.CancellationMessage)
}

func (s *Store) terminateLeasedConversationTurn(
	ctx context.Context,
	tenantID, continuityID string,
	mutation LeasedConversationOperationMutation,
	target ChatTurnStatus,
	code, message string,
) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin leased conversation termination: %w", err)
	}
	defer tx.Rollback(ctx)

	turn, err := lockCurrentLeasedConversationTurn(ctx, tx, tenantID, continuityID, mutation)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if turn.Status == target {
		existingCode, existingMessage := turn.FailureCode, turn.FailureMessage
		if target == ChatTurnCancelled {
			existingCode, existingMessage = turn.CancellationCode, turn.CancellationMessage
		}
		if existingCode != code || existingMessage != message {
			return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to another %s result", target)
		}
		turn.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("commit replayed leased conversation termination: %w", err)
		}
		return turn, nil
	}
	if turn.Status != ChatTurnInProgress {
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation is already %s", turn.Status)
	}
	if target == ChatTurnFailed {
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET status = 'failed', failure_code = $1, failure_message = $2,
    lease_expires_at = NULL, updated_at = statement_timestamp()
WHERE id = $3::uuid`, code, message, turn.ID); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("fail leased conversation operation: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET status = 'cancelled', cancellation_code = $1, cancellation_message = $2,
    lease_expires_at = NULL, updated_at = statement_timestamp()
WHERE id = $3::uuid`, code, message, turn.ID); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("cancel leased conversation operation: %w", err)
		}
	}
	receipt, err := loadLeasedConversationTurnTx(ctx, tx, tenantID, mutation.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit leased conversation termination: %w", err)
	}
	return receipt, nil
}

func lockCurrentLeasedConversationTurn(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, continuityID string,
	request LeasedConversationOperationMutation,
) (ChatTurnReceipt, error) {
	turn, found, err := lookupConversationTurnForUpdateTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation does not exist")
	}
	if turn.ContinuityID != continuityID {
		return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to another conversation turn")
	}
	if turn.Protocol != ClientOperationLeasedV1 {
		return ChatTurnReceipt{}, fmt.Errorf("conversation operation is not leased")
	}
	if turn.AttemptID != request.AttemptID || turn.LeaseGeneration != request.LeaseGeneration {
		return ChatTurnReceipt{}, fmt.Errorf("stale leased conversation attempt")
	}
	return turn, nil
}

func requireLeasedTurnInProgress(turn ChatTurnReceipt) error {
	if turn.Status == ChatTurnInProgress {
		return nil
	}
	return fmt.Errorf("conversation operation is already %s", turn.Status)
}

func loadLeasedConversationTurnTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (ChatTurnReceipt, error) {
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("leased conversation operation disappeared")
	}
	return receipt, nil
}
