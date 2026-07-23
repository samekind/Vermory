package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RecordConversationToolResult(
	ctx context.Context,
	tenantID string,
	turn ChatTurnReceipt,
	request RecordConversationToolResultRequest,
) (ConversationToolResultReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationToolResultReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConversationToolResultReceipt{}, fmt.Errorf("begin conversation tool result: %w", err)
	}
	defer tx.Rollback(ctx)

	var continuityID, operationID, status, protocol, attemptID string
	var leaseGeneration int64
	if err := tx.QueryRow(ctx, `
SELECT continuity_id::text, operation_id, status, operation_protocol,
       COALESCE(attempt_id::text, ''), lease_generation
FROM conversation_turns
WHERE tenant_id = $1 AND id = $2::uuid
FOR UPDATE`, tenantID, turn.ID).Scan(&continuityID, &operationID, &status, &protocol, &attemptID, &leaseGeneration); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConversationToolResultReceipt{}, fmt.Errorf("conversation turn does not exist")
		}
		return ConversationToolResultReceipt{}, fmt.Errorf("lock conversation turn for tool result: %w", err)
	}
	if continuityID != turn.ContinuityID || operationID != request.OperationID {
		return ConversationToolResultReceipt{}, fmt.Errorf("operation_id is already bound to another conversation turn")
	}
	if ClientOperationProtocol(protocol) == ClientOperationLeasedV1 {
		if request.AttemptID == "" || request.LeaseGeneration <= 0 {
			return ConversationToolResultReceipt{}, fmt.Errorf("leased conversation operation requires fenced tool result")
		}
		if request.AttemptID != attemptID || request.LeaseGeneration != leaseGeneration {
			return ConversationToolResultReceipt{}, fmt.Errorf("stale leased conversation attempt")
		}
	} else if request.AttemptID != "" || request.LeaseGeneration != 0 {
		return ConversationToolResultReceipt{}, fmt.Errorf("bounded conversation turn does not accept leased tool fencing")
	}

	contentSHA := contentSHA256(request.Content)
	var existing ConversationToolResultReceipt
	var existingRunID, existingToolName, existingContentSHA string
	err = tx.QueryRow(ctx, `
SELECT observation_id::text, run_id, tool_name, content_sha256
FROM conversation_tool_results
WHERE tenant_id = $1 AND turn_id = $2::uuid AND tool_call_id = $3`,
		tenantID, turn.ID, request.ToolCallID,
	).Scan(&existing.ObservationID, &existingRunID, &existingToolName, &existingContentSHA)
	if err == nil {
		if existingRunID != request.RunID || existingToolName != request.ToolName || existingContentSHA != contentSHA {
			return ConversationToolResultReceipt{}, fmt.Errorf("tool_call_id is already bound to another tool result")
		}
		existing.TurnID = turn.ID
		existing.ToolName = request.ToolName
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return ConversationToolResultReceipt{}, fmt.Errorf("commit replayed conversation tool result: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationToolResultReceipt{}, fmt.Errorf("lookup conversation tool result: %w", err)
	}
	if ChatTurnStatus(status) != ChatTurnInProgress {
		return ConversationToolResultReceipt{}, fmt.Errorf("conversation turn is already %s", status)
	}

	var resultCount, resultBytes int
	if err := tx.QueryRow(ctx, `
SELECT count(*), COALESCE(sum(octet_length(observation.content)), 0)
FROM conversation_tool_results result
JOIN observations observation
  ON observation.tenant_id = result.tenant_id
 AND observation.continuity_id = result.continuity_id
 AND observation.id = result.observation_id
WHERE result.tenant_id = $1 AND result.turn_id = $2::uuid`, tenantID, turn.ID).Scan(&resultCount, &resultBytes); err != nil {
		return ConversationToolResultReceipt{}, fmt.Errorf("count conversation tool results: %w", err)
	}
	if resultCount >= maxConversationToolResultsPerTurn {
		return ConversationToolResultReceipt{}, fmt.Errorf("conversation turn has too many tool results")
	}
	if resultBytes+len(request.Content) > maxConversationToolResultTotalBytes {
		return ConversationToolResultReceipt{}, fmt.Errorf("conversation turn tool result total content is too long")
	}

	observation, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: toolResultObservationOperationID(request.OperationID, request.ToolCallID),
		Kind:        ObservationKindToolResult,
		Content:     request.Content,
		SourceRef:   "tool:" + request.ToolName,
	})
	if err != nil {
		return ConversationToolResultReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO conversation_tool_results (
  tenant_id, continuity_id, turn_id, observation_id,
  run_id, tool_name, tool_call_id, content_sha256
) VALUES ($1, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8)`,
		tenantID, continuityID, turn.ID, observation.ObservationID,
		request.RunID, request.ToolName, request.ToolCallID, contentSHA,
	); err != nil {
		return ConversationToolResultReceipt{}, fmt.Errorf("insert conversation tool result: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationToolResultReceipt{}, fmt.Errorf("commit conversation tool result: %w", err)
	}
	return ConversationToolResultReceipt{
		TurnID:        turn.ID,
		ObservationID: observation.ObservationID,
		ToolName:      request.ToolName,
	}, nil
}

func toolResultObservationOperationID(operationID, toolCallID string) string {
	sum := sha256.Sum256([]byte(operationID + "\x00" + toolCallID))
	return "tool-result:" + hex.EncodeToString(sum[:])
}

func contentSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
