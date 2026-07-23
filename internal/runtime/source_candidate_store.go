package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListActiveMemoriesByKey(ctx context.Context, tenantID, continuityID, memoryKey string, limit int) ([]GovernedMemory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.ListEligibleMemoriesByKeyAt(ctx, tenantID, continuityID, memoryKey, limit, snapshot.AsOf)
}

func (s *Store) ListEligibleMemoriesByKeyAt(ctx context.Context, tenantID, continuityID, memoryKey string, limit int, asOf time.Time) ([]GovernedMemory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 2 {
		limit = 2
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, memory_key, lifecycle_status, content, COALESCE(supersedes_memory_id::text, '')
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid
	AND memory_key = $3
	AND memory_is_eligible(lifecycle_status, content, valid_from, valid_until, $5)
ORDER BY created_at ASC, id ASC
LIMIT $4`, tenantID, continuityID, memoryKey, limit, asOf)
	if err != nil {
		return nil, fmt.Errorf("list active memories by key: %w", err)
	}
	defer rows.Close()

	memories := make([]GovernedMemory, 0, limit)
	for rows.Next() {
		var memory GovernedMemory
		if err := rows.Scan(&memory.ID, &memory.MemoryKey, &memory.LifecycleStatus, &memory.Content, &memory.SupersedesMemoryID); err != nil {
			return nil, fmt.Errorf("scan active memory by key: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active memories by key: %w", err)
	}
	return memories, nil
}

func (s *Store) LookupSourceCandidateOperation(ctx context.Context, tenantID, continuityID string, request CommitObservationRequest) (SourceCandidateReceipt, bool, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceCandidateReceipt{}, false, err
	}
	var observationID, existingContinuityID, kind, content, sourceRef, memoryKey string
	var candidateID, candidateStatus, targetMemoryID *string
	err = s.pool.QueryRow(ctx, `
SELECT observation.id::text, observation.continuity_id::text,
       observation.observation_kind, observation.content, observation.source_ref,
       observation.memory_key, memory.id::text, memory.lifecycle_status,
       memory.supersedes_memory_id::text
FROM observations observation
LEFT JOIN governed_memories memory ON memory.origin_observation_id = observation.id
WHERE observation.tenant_id = $1 AND observation.operation_id = $2`, tenantID, request.OperationID).Scan(
		&observationID,
		&existingContinuityID,
		&kind,
		&content,
		&sourceRef,
		&memoryKey,
		&candidateID,
		&candidateStatus,
		&targetMemoryID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceCandidateReceipt{}, false, nil
	}
	if err != nil {
		return SourceCandidateReceipt{}, false, fmt.Errorf("lookup source candidate operation: %w", err)
	}
	if existingContinuityID != continuityID ||
		kind != string(ObservationKindSourceCandidate) ||
		content != request.Content ||
		sourceRef != request.SourceRef ||
		memoryKey != request.MemoryKey {
		return SourceCandidateReceipt{}, true, fmt.Errorf("operation_id is already bound to another logical source candidate")
	}
	receipt := SourceCandidateReceipt{
		Disposition: SourceCandidateUnchanged,
		MemoryKey:   memoryKey,
		Observation: ObservationReceipt{ObservationID: observationID, Replayed: true},
		Replayed:    true,
	}
	if candidateID == nil {
		return receipt, true, nil
	}
	receipt.Candidate = MemoryReceipt{MemoryID: *candidateID, Status: *candidateStatus, Replayed: true}
	if targetMemoryID != nil {
		receipt.Disposition = SourceCandidateReplacement
		receipt.TargetMemoryID = *targetMemoryID
	} else {
		receipt.Disposition = SourceCandidateNew
	}
	return receipt, true, nil
}

func (s *Store) AcceptSourceCandidate(ctx context.Context, tenantID, continuityID, candidateMemoryID, operationID string) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if candidateMemoryID == "" || operationID == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("candidate memory_id and operation_id are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("begin source candidate acceptance: %w", err)
	}
	defer tx.Rollback(ctx)

	var status, content, memoryKey, targetMemoryID string
	err = tx.QueryRow(ctx, `
SELECT memory.lifecycle_status, memory.content, memory.memory_key,
       COALESCE(memory.supersedes_memory_id::text, '')
FROM governed_memories memory
JOIN observations observation ON observation.id = memory.origin_observation_id
WHERE memory.id = $1::uuid AND memory.tenant_id = $2
  AND memory.continuity_id = $3::uuid
  AND observation.observation_kind = 'source_candidate'
FOR UPDATE OF memory`, candidateMemoryID, tenantID, continuityID).Scan(&status, &content, &memoryKey, &targetMemoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GovernedObservationReceipt{}, fmt.Errorf("candidate does not belong to this workspace continuity")
	}
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("lock source candidate: %w", err)
	}
	decision, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: operationID,
		Kind:        ObservationKindUserConfirmation,
		Content:     "Operator accepted a source candidate.",
		SourceRef:   "memory:" + candidateMemoryID,
	})
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if status == "active" {
		if !decision.Replayed {
			return GovernedObservationReceipt{}, fmt.Errorf("source candidate is already active")
		}
		if err := tx.Commit(ctx); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("commit replayed source candidate acceptance: %w", err)
		}
		return GovernedObservationReceipt{
			Observation: decision,
			Memory:      MemoryReceipt{MemoryID: candidateMemoryID, Status: status, Replayed: true},
		}, nil
	}
	if status != "proposed" {
		return GovernedObservationReceipt{}, fmt.Errorf("only a proposed source candidate can be accepted")
	}
	if targetMemoryID != "" {
		command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'superseded', updated_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND lifecycle_status = 'active' AND memory_key = $4`, targetMemoryID, tenantID, continuityID, memoryKey)
		if err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("supersede accepted candidate target: %w", err)
		}
		if command.RowsAffected() != 1 {
			return GovernedObservationReceipt{}, fmt.Errorf("source candidate target is no longer the active keyed fact")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents WHERE memory_id = $1::uuid`, targetMemoryID); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("remove accepted candidate target projection: %w", err)
		}
	} else {
		var activeCount int
		if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND memory_key = $3 AND lifecycle_status = 'active'`, tenantID, continuityID, memoryKey).Scan(&activeCount); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("check new source candidate key: %w", err)
		}
		if activeCount != 0 {
			return GovernedObservationReceipt{}, fmt.Errorf("source candidate key now has an active fact; propose it again")
		}
	}
	command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'active', updated_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND lifecycle_status = 'proposed'`, candidateMemoryID, tenantID, continuityID)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("activate source candidate: %w", err)
	}
	if command.RowsAffected() != 1 {
		return GovernedObservationReceipt{}, fmt.Errorf("source candidate activation changed no row")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
VALUES ($1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4))`, candidateMemoryID, tenantID, continuityID, content); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("project accepted source candidate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("commit source candidate acceptance: %w", err)
	}
	return GovernedObservationReceipt{
		Observation: decision,
		Memory:      MemoryReceipt{MemoryID: candidateMemoryID, Status: "active"},
	}, nil
}

func (s *Store) RejectSourceCandidate(ctx context.Context, tenantID, continuityID, candidateMemoryID, operationID string) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if candidateMemoryID == "" || operationID == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("candidate memory_id and operation_id are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("begin source candidate rejection: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
SELECT memory.lifecycle_status
FROM governed_memories memory
JOIN observations observation ON observation.id = memory.origin_observation_id
WHERE memory.id = $1::uuid AND memory.tenant_id = $2
  AND memory.continuity_id = $3::uuid
  AND observation.observation_kind = 'source_candidate'
FOR UPDATE OF memory`, candidateMemoryID, tenantID, continuityID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return GovernedObservationReceipt{}, fmt.Errorf("candidate does not belong to this workspace continuity")
	}
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("lock source candidate for rejection: %w", err)
	}
	decision, err := commitObservationTx(ctx, tx, tenantID, continuityID, CommitObservationRequest{
		OperationID: operationID,
		Kind:        ObservationKindCandidateReject,
		Content:     "Operator rejected a source candidate.",
		SourceRef:   "memory:" + candidateMemoryID,
	})
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if status == "rejected" {
		if !decision.Replayed {
			return GovernedObservationReceipt{}, fmt.Errorf("source candidate is already rejected")
		}
		if err := tx.Commit(ctx); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("commit replayed source candidate rejection: %w", err)
		}
		return GovernedObservationReceipt{
			Observation: decision,
			Memory:      MemoryReceipt{MemoryID: candidateMemoryID, Status: status, Replayed: true},
		}, nil
	}
	if status != "proposed" {
		return GovernedObservationReceipt{}, fmt.Errorf("only a proposed source candidate can be rejected")
	}
	command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'rejected', updated_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND lifecycle_status = 'proposed'`, candidateMemoryID, tenantID, continuityID)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("reject source candidate: %w", err)
	}
	if command.RowsAffected() != 1 {
		return GovernedObservationReceipt{}, fmt.Errorf("source candidate rejection changed no row")
	}
	if err := tx.Commit(ctx); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("commit source candidate rejection: %w", err)
	}
	return GovernedObservationReceipt{
		Observation: decision,
		Memory:      MemoryReceipt{MemoryID: candidateMemoryID, Status: "rejected"},
	}, nil
}
