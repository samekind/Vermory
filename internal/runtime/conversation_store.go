package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) SearchActiveConversationMemory(ctx context.Context, tenantID, continuityID, query string, limit int) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.SearchEligibleConversationMemoryAt(ctx, tenantID, continuityID, query, limit, snapshot.AsOf)
}

func (s *Store) SearchEligibleConversationMemoryAt(ctx context.Context, tenantID, continuityID, query string, limit int, asOf time.Time) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if limit <= 0 {
		limit = defaultContextItems
	}
	if limit > maxContextItems {
		limit = maxContextItems
	}
	rows, err := s.pool.Query(ctx, `
WITH link_root AS (
  SELECT COALESCE(
    (
      SELECT primary_continuity_id
      FROM conversation_links
      WHERE tenant_id = $1 AND linked_continuity_id = $2::uuid AND link_state = 'active'
      LIMIT 1
    ),
    $2::uuid
  ) AS continuity_id
), scope AS (
  SELECT continuity_id FROM link_root
  UNION
  SELECT link.linked_continuity_id
  FROM conversation_links link
  JOIN link_root root ON root.continuity_id = link.primary_continuity_id
  WHERE link.tenant_id = $1 AND link.link_state = 'active'
), query_terms AS (
  SELECT
    lower($3)::text AS exact_query,
    plainto_tsquery('simple', $3) AS all_terms,
    to_tsquery('simple', array_to_string(tsvector_to_array(to_tsvector('simple', $3)), ' | ')) AS any_terms
), exact_matches AS (
  SELECT 1
  FROM memory_search_documents document
  JOIN governed_memories memory ON memory.id = document.memory_id
  JOIN observations origin ON origin.tenant_id = $1 AND origin.id = memory.origin_observation_id
  CROSS JOIN query_terms
  WHERE document.tenant_id = $1
    AND document.continuity_id IN (SELECT continuity_id FROM scope)
    AND memory.tenant_id = $1
    AND memory.continuity_id = document.continuity_id
	AND memory_is_eligible(
	  memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
	)
    AND position(query_terms.exact_query IN lower(document.content)) > 0
  LIMIT 1
)
SELECT memory.id::text, memory.content
FROM memory_search_documents document
JOIN governed_memories memory ON memory.id = document.memory_id
JOIN observations origin ON origin.tenant_id = $1 AND origin.id = memory.origin_observation_id
CROSS JOIN query_terms
WHERE document.tenant_id = $1
  AND document.continuity_id IN (SELECT continuity_id FROM scope)
  AND memory.tenant_id = $1
  AND memory.continuity_id = document.continuity_id
	AND memory_is_eligible(
	  memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
	)
  AND (
    position(query_terms.exact_query IN lower(document.content)) > 0
    OR (
      NOT EXISTS (SELECT 1 FROM exact_matches)
      AND (
        document.search_document @@ query_terms.any_terms
        OR similarity(lower(document.content), query_terms.exact_query) >= 0.2
      )
    )
  )
ORDER BY
  (position(query_terms.exact_query IN lower(document.content)) > 0) DESC,
  ts_rank(document.search_document, query_terms.all_terms) DESC,
  ts_rank(document.search_document, query_terms.any_terms) DESC,
  similarity(lower(document.content), query_terms.exact_query) DESC,
  CASE origin.observation_kind
    WHEN 'user_correction' THEN 4
    WHEN 'user_confirmation' THEN 4
    WHEN 'source_update' THEN 3
    WHEN 'bridge_promote' THEN 2
    ELSE 1
  END DESC,
  memory.updated_at DESC
LIMIT $4`, tenantID, continuityID, query, limit, asOf)
	if err != nil {
		return nil, fmt.Errorf("search active conversation memory: %w", err)
	}
	defer rows.Close()
	memories := make([]Memory, 0)
	for rows.Next() {
		var memory Memory
		if err := rows.Scan(&memory.ID, &memory.Content); err != nil {
			return nil, fmt.Errorf("scan active conversation memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active conversation memory: %w", err)
	}
	return memories, nil
}

func (s *Store) ResolveLinkedConversationContinuityIDs(ctx context.Context, tenantID, continuityID string) ([]string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
WITH requested AS (
  SELECT id
  FROM continuity_spaces
  WHERE tenant_id = $1 AND id = $2::uuid
    AND continuity_line = 'conversation' AND state = 'active'
), link_root AS (
  SELECT COALESCE(
    (
      SELECT primary_continuity_id
      FROM conversation_links
      WHERE tenant_id = $1
        AND linked_continuity_id = requested.id
        AND link_state = 'active'
      LIMIT 1
    ),
    requested.id
  ) AS continuity_id
  FROM requested
), scope AS (
  SELECT continuity_id FROM link_root
  UNION
  SELECT link.linked_continuity_id
  FROM conversation_links link
  JOIN link_root root ON root.continuity_id = link.primary_continuity_id
  WHERE link.tenant_id = $1 AND link.link_state = 'active'
)
SELECT continuity.id::text
FROM scope
JOIN continuity_spaces continuity
  ON continuity.tenant_id = $1 AND continuity.id = scope.continuity_id
WHERE continuity.continuity_line = 'conversation' AND continuity.state = 'active'
ORDER BY continuity.id::text`, tenantID, continuityID)
	if err != nil {
		return nil, fmt.Errorf("resolve linked conversation scope: %w", err)
	}
	defer rows.Close()
	continuityIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan linked conversation scope: %w", err)
		}
		continuityIDs = append(continuityIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked conversation scope: %w", err)
	}
	if len(continuityIDs) == 0 {
		return nil, fmt.Errorf("active conversation continuity was not found")
	}
	return continuityIDs, nil
}

const conversationUserSourceRef = "conversation:user"

func (s *Store) ResolveConversation(ctx context.Context, tenantID string, anchor ConversationAnchor) (ConversationResolution, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationResolution{}, err
	}
	anchor, err = anchor.Normalized()
	if err != nil {
		return ConversationResolution{}, err
	}
	var continuityID string
	err = s.pool.QueryRow(ctx, `
SELECT b.continuity_id::text
FROM conversation_bindings b
JOIN continuity_spaces c ON c.id = b.continuity_id
WHERE b.tenant_id = $1 AND b.channel = $2 AND b.thread_id = $3
  AND b.binding_state = 'confirmed'
  AND c.continuity_line = 'conversation' AND c.state = 'active'`,
		tenantID, anchor.Channel, anchor.ThreadID).Scan(&continuityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationResolution{
			Status:   ResolutionUnresolved,
			Channel:  anchor.Channel,
			ThreadID: anchor.ThreadID,
		}, nil
	}
	if err != nil {
		return ConversationResolution{}, fmt.Errorf("resolve conversation binding: %w", err)
	}
	return ConversationResolution{
		Status:       ResolutionResolved,
		ContinuityID: continuityID,
		Channel:      anchor.Channel,
		ThreadID:     anchor.ThreadID,
	}, nil
}

func (s *Store) ResolveOrCreateConversation(ctx context.Context, tenantID string, anchor ConversationAnchor) (ConversationResolution, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ConversationResolution{}, err
	}
	anchor, err = anchor.Normalized()
	if err != nil {
		return ConversationResolution{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConversationResolution{}, fmt.Errorf("begin conversation binding: %w", err)
	}
	defer tx.Rollback(ctx)

	lockKey := fmt.Sprintf("%d:%s%d:%s%d:%s", len(tenantID), tenantID, len(anchor.Channel), anchor.Channel, len(anchor.ThreadID), anchor.ThreadID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return ConversationResolution{}, fmt.Errorf("lock conversation binding: %w", err)
	}

	var continuityID string
	err = tx.QueryRow(ctx, `
SELECT b.continuity_id::text
FROM conversation_bindings b
JOIN continuity_spaces c ON c.id = b.continuity_id
WHERE b.tenant_id = $1 AND b.channel = $2 AND b.thread_id = $3
  AND b.binding_state = 'confirmed'
  AND c.continuity_line = 'conversation' AND c.state = 'active'`,
		tenantID, anchor.Channel, anchor.ThreadID).Scan(&continuityID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return ConversationResolution{}, fmt.Errorf("commit existing conversation binding: %w", err)
		}
		return ConversationResolution{
			Status:       ResolutionResolved,
			ContinuityID: continuityID,
			Channel:      anchor.Channel,
			ThreadID:     anchor.ThreadID,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationResolution{}, fmt.Errorf("lookup conversation binding: %w", err)
	}

	if err := tx.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ($1, 'conversation', 'active')
RETURNING id::text`, tenantID).Scan(&continuityID); err != nil {
		return ConversationResolution{}, fmt.Errorf("create conversation continuity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO conversation_bindings (continuity_id, tenant_id, channel, thread_id, binding_state)
VALUES ($1::uuid, $2, $3, $4, 'confirmed')`, continuityID, tenantID, anchor.Channel, anchor.ThreadID); err != nil {
		return ConversationResolution{}, fmt.Errorf("create conversation binding: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationResolution{}, fmt.Errorf("commit conversation binding: %w", err)
	}
	return ConversationResolution{
		Status:       ResolutionResolved,
		ContinuityID: continuityID,
		Channel:      anchor.Channel,
		ThreadID:     anchor.ThreadID,
		Created:      true,
	}, nil
}

func (s *Store) ListRecentConversationObservations(ctx context.Context, tenantID, continuityID, beforeObservationID string, limit int) ([]ConversationObservation, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.ListRecentConversationObservationsAt(ctx, tenantID, continuityID, beforeObservationID, limit, snapshot.AsOf)
}

func (s *Store) ListRecentConversationObservationsAt(ctx context.Context, tenantID, continuityID, beforeObservationID string, limit int, asOf time.Time) ([]ConversationObservation, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultRecentConversationObservations
	}
	if limit > maxRecentConversationObservations {
		limit = maxRecentConversationObservations
	}

	var beforeSequence int64
	if beforeObservationID != "" {
		err = s.pool.QueryRow(ctx, `
SELECT o.observation_seq
FROM observations o
JOIN continuity_spaces c ON c.id = o.continuity_id
WHERE o.id = $1::uuid AND o.tenant_id = $2 AND o.continuity_id = $3::uuid
  AND c.continuity_line = 'conversation' AND c.state = 'active'`,
			beforeObservationID, tenantID, continuityID).Scan(&beforeSequence)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("before observation does not belong to this conversation")
		}
		if err != nil {
			return nil, fmt.Errorf("lookup conversation observation boundary: %w", err)
		}
	}

	rows, err := s.pool.Query(ctx, `
SELECT observation.id::text, observation.observation_seq, observation.observation_kind, observation.content
FROM observations observation
WHERE observation.tenant_id = $1 AND observation.continuity_id = $2::uuid
  AND observation.observation_kind IN ('user_message', 'assistant_message')
  AND observation.content <> '[redacted]'
  AND ($3::bigint = 0 OR observation.observation_seq < $3)
  AND NOT EXISTS (
    SELECT 1
    FROM governed_memories memory
    WHERE memory.tenant_id = observation.tenant_id
      AND memory.continuity_id = observation.continuity_id
      AND memory.origin_observation_id = observation.id
      AND NOT memory_is_eligible(
        memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
      )
  )
  AND NOT EXISTS (
    SELECT 1
    FROM conversation_turns turn_record
    JOIN memory_deliveries delivery
      ON delivery.tenant_id = turn_record.tenant_id
     AND delivery.id = turn_record.delivery_id
    JOIN governed_memories memory
      ON memory.tenant_id = observation.tenant_id
     AND memory.continuity_id = observation.continuity_id
    WHERE turn_record.tenant_id = observation.tenant_id
      AND turn_record.continuity_id = observation.continuity_id
      AND turn_record.assistant_observation_id = observation.id
      AND (
        memory.origin_observation_id = turn_record.user_observation_id
        OR position(lower(memory.content) IN lower(delivery.context_body)) > 0
      )
      AND NOT memory_is_eligible(
        memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
      )
  )
ORDER BY observation.observation_seq DESC
LIMIT $4`, tenantID, continuityID, beforeSequence, limit, asOf)
	if err != nil {
		return nil, fmt.Errorf("list recent conversation observations: %w", err)
	}
	defer rows.Close()

	reversed := make([]ConversationObservation, 0, limit)
	for rows.Next() {
		var observation ConversationObservation
		if err := rows.Scan(&observation.ID, &observation.Sequence, &observation.Kind, &observation.Content); err != nil {
			return nil, fmt.Errorf("scan recent conversation observation: %w", err)
		}
		reversed = append(reversed, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent conversation observations: %w", err)
	}

	observations := make([]ConversationObservation, len(reversed))
	for i := range reversed {
		observations[len(reversed)-1-i] = reversed[i]
	}
	return observations, nil
}

func (s *Store) ListConversationObservations(ctx context.Context, tenantID, continuityID string, limit int) ([]ConversationObservation, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecentConversationObservations {
		limit = maxRecentConversationObservations
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid
ORDER BY observation_seq DESC
LIMIT $3`, tenantID, continuityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversation observations: %w", err)
	}
	defer rows.Close()

	reversed := make([]ConversationObservation, 0, limit)
	for rows.Next() {
		var observation ConversationObservation
		if err := rows.Scan(&observation.ID, &observation.Sequence, &observation.Kind, &observation.Content); err != nil {
			return nil, fmt.Errorf("scan conversation observation: %w", err)
		}
		reversed = append(reversed, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation observations: %w", err)
	}
	observations := make([]ConversationObservation, len(reversed))
	for i := range reversed {
		observations[len(reversed)-1-i] = reversed[i]
	}
	return observations, nil
}

func (s *Store) ConfirmConversationObservation(ctx context.Context, tenantID, continuityID, observationID, operationID string) (MemoryReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return MemoryReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MemoryReceipt{}, fmt.Errorf("begin conversation confirmation: %w", err)
	}
	defer tx.Rollback(ctx)

	var kind, content string
	if err := tx.QueryRow(ctx, `
SELECT observation_kind, content
FROM observations
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND observation_kind IN ('user_message', 'assistant_message')
FOR UPDATE`, observationID, tenantID, continuityID).Scan(&kind, &content); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemoryReceipt{}, fmt.Errorf("observation does not belong to this conversation")
		}
		return MemoryReceipt{}, fmt.Errorf("lock conversation observation: %w", err)
	}
	if content == "[redacted]" {
		return MemoryReceipt{}, fmt.Errorf("redacted observation cannot become memory")
	}

	confirmation, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: operationID,
		Kind:        ObservationKindUserConfirmation,
		Content:     "User confirmed an observation.",
		SourceRef:   "observation:" + observationID,
	})
	if err != nil {
		return MemoryReceipt{}, err
	}

	var existing MemoryReceipt
	err = tx.QueryRow(ctx, `
SELECT id::text, lifecycle_status
FROM governed_memories
WHERE origin_observation_id = $1::uuid`, observationID).Scan(&existing.MemoryID, &existing.Status)
	if err == nil {
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return MemoryReceipt{}, fmt.Errorf("commit replayed conversation confirmation: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MemoryReceipt{}, fmt.Errorf("lookup confirmed conversation memory: %w", err)
	}

	var memoryID string
	if err := tx.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content
)
VALUES ($1, $2::uuid, $3::uuid, 'fact', 'active', $4)
RETURNING id::text`, tenantID, continuityID, observationID, content).Scan(&memoryID); err != nil {
		return MemoryReceipt{}, fmt.Errorf("create confirmed conversation memory: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
VALUES ($1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4))`, memoryID, tenantID, continuityID, content); err != nil {
		return MemoryReceipt{}, fmt.Errorf("project confirmed conversation memory: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MemoryReceipt{}, fmt.Errorf("commit conversation confirmation: %w", err)
	}
	return MemoryReceipt{MemoryID: memoryID, Status: "active", Replayed: confirmation.Replayed}, nil
}

func (s *Store) BeginConversationTurn(ctx context.Context, tenantID, continuityID string, request ChatTurnRequest) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin conversation turn: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, found, err := lookupConversationTurnTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if found {
		if existing.ContinuityID != continuityID || existing.RequestFingerprint != conversationContentFingerprint(request.Message) {
			return ChatTurnReceipt{}, fmt.Errorf("operation_id is already bound to another conversation turn")
		}
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("commit replayed conversation turn: %w", err)
		}
		return existing, nil
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

	var receipt ChatTurnReceipt
	if err := tx.QueryRow(ctx, `
INSERT INTO conversation_turns (
  tenant_id, continuity_id, operation_id, status, user_observation_id, request_fingerprint
)
VALUES ($1, $2::uuid, $3, 'in_progress', $4::uuid, $5)
RETURNING id::text, operation_id, status, continuity_id::text, user_observation_id::text`,
		tenantID, continuityID, request.OperationID, observation.ObservationID, conversationContentFingerprint(request.Message)).Scan(
		&receipt.ID,
		&receipt.OperationID,
		&receipt.Status,
		&receipt.ContinuityID,
		&receipt.UserObservationID,
	); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("create conversation turn: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit conversation turn: %w", err)
	}
	receipt.Protocol = ClientOperationBoundedV1
	return receipt, nil
}

func (s *Store) AttachConversationTurnDelivery(ctx context.Context, tenantID, turnID, deliveryID string) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin conversation delivery attachment: %w", err)
	}
	defer tx.Rollback(ctx)

	var operationID, continuityID, existingDeliveryID string
	if err := tx.QueryRow(ctx, `
SELECT operation_id, continuity_id::text, COALESCE(delivery_id::text, '')
FROM conversation_turns
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, turnID, tenantID).Scan(&operationID, &continuityID, &existingDeliveryID); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("lock conversation turn for delivery: %w", err)
	}

	var validDelivery bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM memory_deliveries
  WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
)`, deliveryID, tenantID, continuityID).Scan(&validDelivery); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("check prepared conversation delivery: %w", err)
	}
	if !validDelivery {
		return ChatTurnReceipt{}, fmt.Errorf("delivery does not belong to this conversation")
	}
	if existingDeliveryID != "" && existingDeliveryID != deliveryID {
		return ChatTurnReceipt{}, fmt.Errorf("conversation turn is already bound to another delivery")
	}
	replayed := existingDeliveryID == deliveryID
	if existingDeliveryID == "" {
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET delivery_id = $1::uuid, updated_at = now()
WHERE id = $2::uuid AND tenant_id = $3`, deliveryID, turnID, tenantID); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("attach conversation delivery: %w", err)
		}
	}
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("conversation turn disappeared during delivery attachment")
	}
	receipt.Replayed = replayed
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit conversation delivery attachment: %w", err)
	}
	return receipt, nil
}

func (s *Store) LookupConversationTurn(ctx context.Context, tenantID, operationID string) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin conversation turn lookup: %w", err)
	}
	defer tx.Rollback(ctx)
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("conversation turn does not exist")
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit conversation turn lookup: %w", err)
	}
	return receipt, nil
}

func (s *Store) CompleteConversationTurn(ctx context.Context, tenantID, turnID, deliveryID, answer, model string) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin conversation completion: %w", err)
	}
	defer tx.Rollback(ctx)

	var operationID, continuityID, status, protocol, userObservationID string
	if err := tx.QueryRow(ctx, `
SELECT operation_id, continuity_id::text, status, operation_protocol, user_observation_id::text
FROM conversation_turns
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, turnID, tenantID).Scan(&operationID, &continuityID, &status, &protocol, &userObservationID); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("lock conversation turn: %w", err)
	}
	if ClientOperationProtocol(protocol) != ClientOperationBoundedV1 {
		return ChatTurnReceipt{}, fmt.Errorf("leased conversation operation requires fenced completion")
	}
	if ChatTurnStatus(status) != ChatTurnInProgress {
		receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
		if err != nil {
			return ChatTurnReceipt{}, err
		}
		if !found {
			return ChatTurnReceipt{}, fmt.Errorf("conversation turn disappeared during completion")
		}
		receipt.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("commit replayed conversation completion: %w", err)
		}
		return receipt, nil
	}

	var validDelivery bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM memory_deliveries
  WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
)`, deliveryID, tenantID, continuityID).Scan(&validDelivery); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("check conversation delivery: %w", err)
	}
	if !validDelivery {
		return ChatTurnReceipt{}, fmt.Errorf("delivery does not belong to this conversation")
	}

	assistant, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: operationID + ":assistant",
		Kind:        ObservationKindAssistantMessage,
		Content:     answer,
		SourceRef:   "provider:" + model,
	})
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET status = 'completed', delivery_id = $1::uuid, assistant_observation_id = $2::uuid,
    answer = $3, answer_fingerprint = $4, provider_model = $5,
    failure_code = '', failure_message = '', updated_at = now()
WHERE id = $6::uuid`, deliveryID, assistant.ObservationID, answer, conversationContentFingerprint(answer), model, turnID); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("complete conversation turn: %w", err)
	}
	if _, err := enqueueConversationFormationTx(ctx, tx, tenantID, continuityID, turnID, userObservationID); err != nil {
		return ChatTurnReceipt{}, err
	}
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("completed conversation turn is missing")
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit conversation completion: %w", err)
	}
	return receipt, nil
}

func (s *Store) FailConversationTurn(ctx context.Context, tenantID, turnID, failureCode, failureMessage string) (ChatTurnReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("begin failed conversation turn: %w", err)
	}
	defer tx.Rollback(ctx)

	var operationID, protocol string
	if err := tx.QueryRow(ctx, `
UPDATE conversation_turns
SET status = 'failed', failure_code = $1, failure_message = $2, updated_at = now()
WHERE id = $3::uuid AND tenant_id = $4 AND status = 'in_progress' AND operation_protocol = 'bounded_v1'
RETURNING operation_id, operation_protocol`, failureCode, failureMessage, turnID, tenantID).Scan(&operationID, &protocol); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return ChatTurnReceipt{}, fmt.Errorf("fail conversation turn: %w", err)
		}
		if err := tx.QueryRow(ctx, `
SELECT operation_id, operation_protocol FROM conversation_turns WHERE id = $1::uuid AND tenant_id = $2`, turnID, tenantID).Scan(&operationID, &protocol); err != nil {
			return ChatTurnReceipt{}, fmt.Errorf("lookup existing failed conversation turn: %w", err)
		}
	}
	if ClientOperationProtocol(protocol) != ClientOperationBoundedV1 {
		return ChatTurnReceipt{}, fmt.Errorf("leased conversation operation requires fenced failure")
	}
	receipt, found, err := lookupConversationTurnTx(ctx, tx, tenantID, operationID)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if !found {
		return ChatTurnReceipt{}, fmt.Errorf("failed conversation turn is missing")
	}
	if err := tx.Commit(ctx); err != nil {
		return ChatTurnReceipt{}, fmt.Errorf("commit failed conversation turn: %w", err)
	}
	return receipt, nil
}

func lookupConversationTurnTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (ChatTurnReceipt, bool, error) {
	receipt, err := scanConversationTurn(tx.QueryRow(ctx, `
SELECT id::text, operation_id, status, operation_protocol, continuity_id::text,
       COALESCE(delivery_id::text, ''), user_observation_id::text,
       COALESCE(assistant_observation_id::text, ''), answer, provider_model, failure_code,
       failure_message, cancellation_code, cancellation_message,
       COALESCE(attempt_id::text, ''), lease_generation, lease_expires_at,
       last_heartbeat_at, checkpoint_sequence, checkpoint_payload,
       checkpoint_fingerprint, checkpoint_updated_at,
       request_fingerprint, answer_fingerprint
FROM conversation_turns
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatTurnReceipt{}, false, nil
	}
	if err != nil {
		return ChatTurnReceipt{}, false, fmt.Errorf("lookup conversation turn: %w", err)
	}
	return receipt, true, nil
}

func lookupConversationTurnForUpdateTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (ChatTurnReceipt, bool, error) {
	receipt, err := scanConversationTurn(tx.QueryRow(ctx, `
SELECT id::text, operation_id, status, operation_protocol, continuity_id::text,
       COALESCE(delivery_id::text, ''), user_observation_id::text,
       COALESCE(assistant_observation_id::text, ''), answer, provider_model, failure_code,
       failure_message, cancellation_code, cancellation_message,
       COALESCE(attempt_id::text, ''), lease_generation, lease_expires_at,
       last_heartbeat_at, checkpoint_sequence, checkpoint_payload,
       checkpoint_fingerprint, checkpoint_updated_at,
       request_fingerprint, answer_fingerprint
FROM conversation_turns
WHERE tenant_id = $1 AND operation_id = $2
FOR UPDATE`, tenantID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatTurnReceipt{}, false, nil
	}
	if err != nil {
		return ChatTurnReceipt{}, false, fmt.Errorf("lock conversation turn: %w", err)
	}
	return receipt, true, nil
}

type conversationTurnRow interface {
	Scan(dest ...any) error
}

func scanConversationTurn(row conversationTurnRow) (ChatTurnReceipt, error) {
	var receipt ChatTurnReceipt
	var checkpoint []byte
	err := row.Scan(
		&receipt.ID,
		&receipt.OperationID,
		&receipt.Status,
		&receipt.Protocol,
		&receipt.ContinuityID,
		&receipt.DeliveryID,
		&receipt.UserObservationID,
		&receipt.AssistantObservationID,
		&receipt.Answer,
		&receipt.Model,
		&receipt.FailureCode,
		&receipt.FailureMessage,
		&receipt.CancellationCode,
		&receipt.CancellationMessage,
		&receipt.AttemptID,
		&receipt.LeaseGeneration,
		&receipt.LeaseExpiresAt,
		&receipt.LastHeartbeatAt,
		&receipt.CheckpointSequence,
		&checkpoint,
		&receipt.CheckpointFingerprint,
		&receipt.CheckpointUpdatedAt,
		&receipt.RequestFingerprint,
		&receipt.AnswerFingerprint,
	)
	if err != nil {
		return ChatTurnReceipt{}, err
	}
	if receipt.CheckpointSequence > 0 {
		receipt.Checkpoint = checkpoint
	}
	return receipt, nil
}

func conversationContentFingerprint(content string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(digest[:])
}
