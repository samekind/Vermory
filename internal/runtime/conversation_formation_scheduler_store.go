package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	conversationFormationWindowLimit = 50
	conversationFormationWindowBytes = 65536
)

func enqueueConversationFormationTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, turnID, userObservationID string) (ConversationFormationSchedule, error) {
	var sequence int64
	if err := tx.QueryRow(ctx, `
SELECT max(observation.observation_seq)
FROM observations observation
WHERE observation.tenant_id = $1 AND observation.continuity_id = $2::uuid
  AND observation.content <> '[redacted]'
  AND (
    (observation.id = $3::uuid AND observation.observation_kind = 'user_message')
    OR
    (observation.observation_kind = 'tool_result' AND EXISTS (
      SELECT 1 FROM conversation_tool_results result
      WHERE result.tenant_id = $1 AND result.continuity_id = $2::uuid
        AND result.turn_id = $4::uuid AND result.observation_id = observation.id
    ))
  )`,
		tenantID, continuityID, userObservationID, turnID,
	).Scan(&sequence); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConversationFormationSchedule{}, fmt.Errorf("conversation turn has no eligible automatic formation observations")
		}
		return ConversationFormationSchedule{}, fmt.Errorf("lookup automatic formation observation: %w", err)
	}
	row := tx.QueryRow(ctx, `
INSERT INTO conversation_formation_schedules (
  tenant_id, continuity_id, requested_through_sequence,
  processed_through_sequence, schedule_state, next_attempt_at
)
VALUES ($1, $2::uuid, $3, 0, 'pending', now())
ON CONFLICT (tenant_id, continuity_id) DO UPDATE
SET requested_through_sequence = GREATEST(
      conversation_formation_schedules.requested_through_sequence,
      EXCLUDED.requested_through_sequence
    ),
    schedule_state = CASE
      WHEN conversation_formation_schedules.schedule_state = 'running' THEN 'running'
      WHEN GREATEST(
        conversation_formation_schedules.requested_through_sequence,
        EXCLUDED.requested_through_sequence
      ) > conversation_formation_schedules.processed_through_sequence THEN 'pending'
      ELSE 'idle'
    END,
    next_attempt_at = CASE
      WHEN conversation_formation_schedules.schedule_state = 'running' THEN NULL
      WHEN GREATEST(
        conversation_formation_schedules.requested_through_sequence,
        EXCLUDED.requested_through_sequence
      ) > conversation_formation_schedules.processed_through_sequence THEN now()
      ELSE NULL
    END,
    updated_at = now()
RETURNING `+conversationFormationScheduleColumns, tenantID, continuityID, sequence)
	return scanConversationFormationSchedule(row)
}

func (s *Store) ConversationFormationSchedule(ctx context.Context, tenantID, continuityID string) (ConversationFormationSchedule, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationFormationSchedule{}, err
	}
	return scanConversationFormationSchedule(s.pool.QueryRow(ctx, `
SELECT `+conversationFormationScheduleColumns+`
FROM conversation_formation_schedules
WHERE tenant_id = $1 AND continuity_id = $2::uuid`, tenantID, continuityID))
}

func (s *Store) ClaimConversationFormation(ctx context.Context, tenantID string, leaseDuration, retryDelay time.Duration) (ConversationFormationClaim, bool, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationFormationClaim{}, false, err
	}
	return s.claimConversationFormation(ctx, tenantID, leaseDuration, retryDelay, 0)
}

func (s *Store) ClaimConversationFormationWithLimit(ctx context.Context, tenantID string, leaseDuration, retryDelay time.Duration, maxAttempts int) (ConversationFormationClaim, bool, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationFormationClaim{}, false, err
	}
	if maxAttempts <= 0 {
		return ConversationFormationClaim{}, false, fmt.Errorf("conversation formation max attempts must be positive")
	}
	return s.claimConversationFormation(ctx, tenantID, leaseDuration, retryDelay, maxAttempts)
}

func (s *Store) claimConversationFormation(ctx context.Context, tenantID string, leaseDuration, retryDelay time.Duration, maxAttempts int) (ConversationFormationClaim, bool, error) {
	if leaseDuration <= 0 {
		return ConversationFormationClaim{}, false, fmt.Errorf("conversation formation lease duration must be positive")
	}
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConversationFormationClaim{}, false, fmt.Errorf("begin automatic conversation formation claim: %w", err)
	}
	defer tx.Rollback(ctx)

	schedule, err := scanConversationFormationSchedule(tx.QueryRow(ctx, `
SELECT `+conversationFormationScheduleColumns+`
FROM conversation_formation_schedules
WHERE tenant_id = $1
  AND requested_through_sequence > processed_through_sequence
  AND (schedule_state = 'running' OR $2 = 0 OR attempt_count < $2)
  AND (
    (schedule_state IN ('pending', 'retry_wait') AND next_attempt_at <= now())
    OR (schedule_state = 'running' AND lease_expires_at <= now())
  )
ORDER BY updated_at, continuity_id
FOR UPDATE SKIP LOCKED
LIMIT 1`, tenantID, maxAttempts))
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationFormationClaim{}, false, nil
	}
	if err != nil {
		return ConversationFormationClaim{}, false, fmt.Errorf("select automatic conversation formation claim: %w", err)
	}

	if schedule.State == ConversationFormationRunning {
		observations, err := loadConversationFormationWindowTx(ctx, tx, tenantID, schedule.ContinuityID, schedule.WindowObservationIDs)
		if err == nil && conversationFormationWindowFingerprint(observations) != schedule.WindowFingerprint {
			err = fmt.Errorf("automatic conversation formation window changed")
		}
		if err != nil {
			if _, releaseErr := tx.Exec(ctx, `
UPDATE conversation_formation_schedules
SET schedule_state = 'retry_wait', lease_token = NULL, lease_expires_at = NULL,
			    next_attempt_at = now() + $3::interval,
			    window_start_sequence = NULL, window_end_sequence = NULL,
			    window_observation_ids = '[]'::jsonb, window_fingerprint = '',
			    active_operation_id = '',
    last_status = 'failed', last_failure_code = 'window_changed', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid`, tenantID, schedule.ContinuityID, durationInterval(retryDelay)); releaseErr != nil {
				return ConversationFormationClaim{}, false, fmt.Errorf("release changed automatic formation window: %w", releaseErr)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return ConversationFormationClaim{}, false, fmt.Errorf("commit changed automatic formation window: %w", commitErr)
			}
			return ConversationFormationClaim{}, false, nil
		}
		leaseToken := ""
		if err := tx.QueryRow(ctx, `
UPDATE conversation_formation_schedules
SET lease_token = gen_random_uuid(), lease_expires_at = now() + $3::interval,
    updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid
RETURNING lease_token::text`, tenantID, schedule.ContinuityID, durationInterval(leaseDuration)).Scan(&leaseToken); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("renew automatic conversation formation lease: %w", err)
		}
		schedule.LeaseToken = leaseToken
		expires := time.Now().UTC().Add(leaseDuration)
		schedule.LeaseExpiresAt = &expires
		if err := tx.Commit(ctx); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("commit renewed automatic conversation formation claim: %w", err)
		}
		return ConversationFormationClaim{Schedule: schedule, Observations: observations}, true, nil
	}

	observations, err := selectConversationFormationWindowTx(
		ctx, tx, tenantID, schedule.ContinuityID,
		schedule.ProcessedThroughSequence, schedule.RequestedThroughSequence,
	)
	if err != nil {
		return ConversationFormationClaim{}, false, err
	}
	if len(observations) == 0 {
		if _, err := tx.Exec(ctx, `
UPDATE conversation_formation_schedules
SET processed_through_sequence = requested_through_sequence,
    schedule_state = 'idle', next_attempt_at = NULL,
    last_status = 'abstained', last_failure_code = '', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid`, tenantID, schedule.ContinuityID); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("complete empty automatic formation window: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("commit empty automatic formation window: %w", err)
		}
		return ConversationFormationClaim{}, false, nil
	}
	if len([]byte(observations[0].Content)) > conversationFormationWindowBytes {
		if _, err := tx.Exec(ctx, `
UPDATE conversation_formation_schedules
SET schedule_state = 'retry_wait', next_attempt_at = now() + $3::interval,
    last_status = 'failed', last_failure_code = 'observation_too_large', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid`, tenantID, schedule.ContinuityID, durationInterval(retryDelay)); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("delay oversized automatic formation observation: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return ConversationFormationClaim{}, false, fmt.Errorf("commit oversized automatic formation delay: %w", err)
		}
		return ConversationFormationClaim{}, false, nil
	}

	ids := make([]string, len(observations))
	for index, observation := range observations {
		ids[index] = observation.ID
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return ConversationFormationClaim{}, false, fmt.Errorf("encode automatic formation observation window: %w", err)
	}
	attempt := schedule.AttemptCount + 1
	operationID := fmt.Sprintf(
		"auto-conversation:%s:%d-%d:attempt-%d",
		schedule.ContinuityID,
		observations[0].Sequence,
		observations[len(observations)-1].Sequence,
		attempt,
	)
	windowFingerprint := conversationFormationWindowFingerprint(observations)
	claimed, err := scanConversationFormationSchedule(tx.QueryRow(ctx, `
UPDATE conversation_formation_schedules
SET schedule_state = 'running', lease_token = gen_random_uuid(),
    lease_expires_at = now() + $3::interval, attempt_count = $4,
    next_attempt_at = NULL, window_start_sequence = $5,
    window_end_sequence = $6, window_observation_ids = $7::jsonb,
    window_fingerprint = $8, active_operation_id = $9,
    last_failure_code = '', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid
RETURNING `+conversationFormationScheduleColumns,
		tenantID, schedule.ContinuityID, durationInterval(leaseDuration), attempt,
		observations[0].Sequence, observations[len(observations)-1].Sequence,
		string(idsJSON), windowFingerprint, operationID,
	))
	if err != nil {
		return ConversationFormationClaim{}, false, fmt.Errorf("claim automatic conversation formation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationFormationClaim{}, false, fmt.Errorf("commit automatic conversation formation claim: %w", err)
	}
	return ConversationFormationClaim{Schedule: claimed, Observations: observations}, true, nil
}

func (s *Store) CompleteConversationFormationClaim(ctx context.Context, tenantID string, claim ConversationFormationClaim, receipt SourceFormationReceipt) (ConversationFormationSchedule, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationFormationSchedule{}, err
	}
	if receipt.Status != SourceFormationCompleted && receipt.Status != SourceFormationAbstained {
		return ConversationFormationSchedule{}, fmt.Errorf("automatic conversation formation can advance only after completed or abstained formation")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConversationFormationSchedule{}, fmt.Errorf("begin automatic conversation formation completion: %w", err)
	}
	defer tx.Rollback(ctx)
	updated, err := scanConversationFormationSchedule(tx.QueryRow(ctx, `
UPDATE conversation_formation_schedules
SET processed_through_sequence = window_end_sequence,
    schedule_state = CASE
      WHEN requested_through_sequence > window_end_sequence THEN 'pending'
      ELSE 'idle'
    END,
    next_attempt_at = CASE
      WHEN requested_through_sequence > window_end_sequence THEN now()
      ELSE NULL
    END,
    lease_token = NULL, lease_expires_at = NULL,
    window_start_sequence = NULL, window_end_sequence = NULL,
    window_observation_ids = '[]'::jsonb, window_fingerprint = '',
    active_operation_id = '',
    last_run_id = $5::uuid, last_status = $6, last_failure_code = '',
    updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND schedule_state = 'running'
  AND lease_token = $3::uuid
  AND active_operation_id = $4
RETURNING `+conversationFormationScheduleColumns,
		tenantID, claim.Schedule.ContinuityID, claim.Schedule.LeaseToken,
		claim.Schedule.ActiveOperationID, receipt.ID, receipt.Status,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationFormationSchedule{}, fmt.Errorf("automatic conversation formation lease is stale")
	}
	if err != nil {
		return ConversationFormationSchedule{}, fmt.Errorf("complete automatic conversation formation schedule: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationFormationSchedule{}, fmt.Errorf("commit automatic conversation formation completion: %w", err)
	}
	return updated, nil
}

func (s *Store) RetryConversationFormationClaim(ctx context.Context, tenantID string, claim ConversationFormationClaim, receipt SourceFormationReceipt, failureCode string, retryDelay time.Duration) (ConversationFormationSchedule, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationFormationSchedule{}, err
	}
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	failureCode = strings.TrimSpace(failureCode)
	if failureCode == "" {
		failureCode = "worker_error"
	}
	if len(failureCode) > 128 {
		failureCode = failureCode[:128]
	}
	var runID any
	var status SourceFormationStatus
	if receipt.ID != "" {
		runID = receipt.ID
		status = receipt.Status
	}
	updated, err := scanConversationFormationSchedule(s.pool.QueryRow(ctx, `
UPDATE conversation_formation_schedules
SET schedule_state = 'retry_wait', next_attempt_at = now() + $5::interval,
    lease_token = NULL, lease_expires_at = NULL,
    window_start_sequence = NULL, window_end_sequence = NULL,
    window_observation_ids = '[]'::jsonb, window_fingerprint = '',
    active_operation_id = '',
    last_run_id = $6::uuid, last_status = $7, last_failure_code = $8,
    updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND schedule_state = 'running'
  AND lease_token = $3::uuid
  AND active_operation_id = $4
RETURNING `+conversationFormationScheduleColumns,
		tenantID, claim.Schedule.ContinuityID, claim.Schedule.LeaseToken,
		claim.Schedule.ActiveOperationID, durationInterval(retryDelay), runID, status, failureCode,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationFormationSchedule{}, fmt.Errorf("automatic conversation formation lease is stale")
	}
	if err != nil {
		return ConversationFormationSchedule{}, fmt.Errorf("retry automatic conversation formation schedule: %w", err)
	}
	return updated, nil
}

const conversationFormationScheduleColumns = `
	tenant_id, continuity_id::text, requested_through_sequence,
	processed_through_sequence, schedule_state, COALESCE(lease_token::text, ''),
	lease_expires_at, attempt_count, next_attempt_at,
	COALESCE(window_start_sequence, 0), COALESCE(window_end_sequence, 0),
	window_observation_ids, window_fingerprint, active_operation_id,
	COALESCE(last_run_id::text, ''),
	last_status, last_failure_code, created_at, updated_at`

type conversationFormationScheduleRow interface {
	Scan(dest ...any) error
}

func scanConversationFormationSchedule(row conversationFormationScheduleRow) (ConversationFormationSchedule, error) {
	var schedule ConversationFormationSchedule
	var observationIDsJSON []byte
	if err := row.Scan(
		&schedule.TenantID,
		&schedule.ContinuityID,
		&schedule.RequestedThroughSequence,
		&schedule.ProcessedThroughSequence,
		&schedule.State,
		&schedule.LeaseToken,
		&schedule.LeaseExpiresAt,
		&schedule.AttemptCount,
		&schedule.NextAttemptAt,
		&schedule.WindowStartSequence,
		&schedule.WindowEndSequence,
		&observationIDsJSON,
		&schedule.WindowFingerprint,
		&schedule.ActiveOperationID,
		&schedule.LastRunID,
		&schedule.LastStatus,
		&schedule.LastFailureCode,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	); err != nil {
		return ConversationFormationSchedule{}, err
	}
	if err := json.Unmarshal(observationIDsJSON, &schedule.WindowObservationIDs); err != nil {
		return ConversationFormationSchedule{}, fmt.Errorf("decode automatic formation observation IDs: %w", err)
	}
	return schedule, nil
}

func conversationFormationWindowFingerprint(observations []ConversationObservation) string {
	return sourceFormationInputManifestFingerprint(sourceFormationManifestFromObservations(observations))
}

func selectConversationFormationWindowTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, processed, requested int64) ([]ConversationObservation, error) {
	rows, err := tx.Query(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND observation_kind IN ('user_message', 'tool_result') AND content <> '[redacted]'
  AND observation_seq > $3 AND observation_seq <= $4
  AND (
    (observation_kind = 'user_message' AND EXISTS (
      SELECT 1 FROM conversation_turns turn
      WHERE turn.tenant_id = observations.tenant_id
        AND turn.continuity_id = observations.continuity_id
        AND turn.user_observation_id = observations.id
        AND turn.status = 'completed'
    ))
    OR
    (observation_kind = 'tool_result' AND EXISTS (
      SELECT 1
      FROM conversation_tool_results result
      JOIN conversation_turns turn
        ON turn.tenant_id = result.tenant_id
       AND turn.continuity_id = result.continuity_id
       AND turn.id = result.turn_id
      WHERE result.tenant_id = observations.tenant_id
        AND result.continuity_id = observations.continuity_id
        AND result.observation_id = observations.id
        AND turn.status = 'completed'
    ))
  )
ORDER BY observation_seq
LIMIT $5`, tenantID, continuityID, processed, requested, conversationFormationWindowLimit)
	if err != nil {
		return nil, fmt.Errorf("select automatic conversation formation window: %w", err)
	}
	defer rows.Close()
	observations := make([]ConversationObservation, 0, conversationFormationWindowLimit)
	totalBytes := 0
	for rows.Next() {
		var observation ConversationObservation
		if err := rows.Scan(&observation.ID, &observation.Sequence, &observation.Kind, &observation.Content); err != nil {
			return nil, fmt.Errorf("scan automatic conversation formation observation: %w", err)
		}
		bytes := len([]byte(observation.Content))
		if len(observations) > 0 && totalBytes+bytes > conversationFormationWindowBytes {
			break
		}
		observations = append(observations, observation)
		totalBytes += bytes
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automatic conversation formation observations: %w", err)
	}
	return observations, nil
}

func loadConversationFormationWindowTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, ids []string) ([]ConversationObservation, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("automatic conversation formation window is empty")
	}
	rows, err := tx.Query(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND id = ANY($3::uuid[])
  AND observation_kind IN ('user_message', 'tool_result') AND content <> '[redacted]'
  AND (
    (observation_kind = 'user_message' AND EXISTS (
      SELECT 1 FROM conversation_turns turn
      WHERE turn.tenant_id = observations.tenant_id
        AND turn.continuity_id = observations.continuity_id
        AND turn.user_observation_id = observations.id
        AND turn.status = 'completed'
    ))
    OR
    (observation_kind = 'tool_result' AND EXISTS (
      SELECT 1
      FROM conversation_tool_results result
      JOIN conversation_turns turn
        ON turn.tenant_id = result.tenant_id
       AND turn.continuity_id = result.continuity_id
       AND turn.id = result.turn_id
      WHERE result.tenant_id = observations.tenant_id
        AND result.continuity_id = observations.continuity_id
        AND result.observation_id = observations.id
        AND turn.status = 'completed'
    ))
  )
ORDER BY array_position($3::uuid[], id)`, tenantID, continuityID, ids)
	if err != nil {
		return nil, fmt.Errorf("load automatic conversation formation window: %w", err)
	}
	defer rows.Close()
	observations := make([]ConversationObservation, 0, len(ids))
	for rows.Next() {
		var observation ConversationObservation
		if err := rows.Scan(&observation.ID, &observation.Sequence, &observation.Kind, &observation.Content); err != nil {
			return nil, fmt.Errorf("scan automatic conversation formation window: %w", err)
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automatic conversation formation window: %w", err)
	}
	if len(observations) != len(ids) {
		return nil, fmt.Errorf("automatic conversation formation window changed")
	}
	for index, observation := range observations {
		if observation.ID != ids[index] {
			return nil, fmt.Errorf("automatic conversation formation window order changed")
		}
	}
	return observations, nil
}

func durationInterval(value time.Duration) string {
	return fmt.Sprintf("%f seconds", value.Seconds())
}
