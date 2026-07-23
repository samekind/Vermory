package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) EnsureGlobalDefaultsContinuity(ctx context.Context, tenantID string) (string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", err
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", fmt.Errorf("tenant_id is required")
	}
	var continuityID string
	err = s.pool.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ($1, 'global_defaults', 'active')
ON CONFLICT (tenant_id)
  WHERE continuity_line = 'global_defaults' AND state = 'active'
DO NOTHING
RETURNING id::text`, tenantID).Scan(&continuityID)
	if err == nil {
		return continuityID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("create global defaults continuity: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
SELECT id::text
FROM continuity_spaces
WHERE tenant_id = $1 AND continuity_line = 'global_defaults' AND state = 'active'`, tenantID).Scan(&continuityID); err != nil {
		return "", fmt.Errorf("resolve global defaults continuity: %w", err)
	}
	return continuityID, nil
}

func (s *Store) ListActiveGlobalDefaults(ctx context.Context, tenantID string) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	continuityID, err := s.EnsureGlobalDefaultsContinuity(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.ListEligibleGlobalDefaultsAt(ctx, tenantID, continuityID, snapshot.AsOf)
}

func (s *Store) ListEligibleGlobalDefaultsAt(ctx context.Context, tenantID, continuityID string, asOf time.Time) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, content
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND memory_kind = 'global_default'
  AND memory_is_eligible(
    lifecycle_status, content, valid_from, valid_until, $3
  )
ORDER BY memory_key ASC, created_at ASC`, tenantID, continuityID, asOf)
	if err != nil {
		return nil, fmt.Errorf("list active global defaults: %w", err)
	}
	defer rows.Close()
	defaults := make([]Memory, 0)
	for rows.Next() {
		var memory Memory
		if err := rows.Scan(&memory.ID, &memory.Content); err != nil {
			return nil, fmt.Errorf("scan active global default: %w", err)
		}
		defaults = append(defaults, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active global defaults: %w", err)
	}
	return defaults, nil
}

func (s *Store) ListGlobalDefaults(ctx context.Context, tenantID string) (string, []GovernedMemory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", nil, err
	}
	continuityID, err := s.EnsureGlobalDefaultsContinuity(ctx, tenantID)
	if err != nil {
		return "", nil, err
	}
	memories, err := s.ListGovernedMemories(ctx, tenantID, continuityID)
	if err != nil {
		return "", nil, err
	}
	return continuityID, memories, nil
}

func (s *Store) SetGlobalDefault(ctx context.Context, tenantID, operationID, memoryKey, content string) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	continuityID, err := s.EnsureGlobalDefaultsContinuity(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	memoryKey = strings.TrimSpace(memoryKey)
	if memoryKey == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("memory_key is required")
	}
	request := CommitObservationRequest{
		OperationID: operationID,
		Kind:        ObservationKindGlobalDefaultSet,
		Content:     content,
		SourceRef:   "global-default:" + memoryKey,
	}
	if err := request.Validate(); err != nil {
		return GovernedObservationReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("begin global default set: %w", err)
	}
	defer tx.Rollback(ctx)
	observation, err := commitObservationTx(ctx, tx, tenantID, continuityID, request)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if existing, found, err := globalDefaultMemoryByOriginTx(ctx, tx, observation.ObservationID); err != nil {
		return GovernedObservationReceipt{}, err
	} else if found {
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("commit replayed global default set: %w", err)
		}
		return GovernedObservationReceipt{Observation: observation, Memory: existing}, nil
	}
	var activeExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM governed_memories
  WHERE tenant_id = $1 AND continuity_id = $2::uuid
    AND memory_kind = 'global_default' AND lifecycle_status = 'active' AND memory_key = $3
)`, tenantID, continuityID, memoryKey).Scan(&activeExists); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("check active global default key: %w", err)
	}
	if activeExists {
		return GovernedObservationReceipt{}, fmt.Errorf("global default %q already has an active value", memoryKey)
	}
	memory, err := insertGlobalDefaultTx(ctx, tx, tenantID, continuityID, observation.ObservationID, memoryKey, request.Content, "")
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("commit global default set: %w", err)
	}
	return GovernedObservationReceipt{Observation: observation, Memory: memory}, nil
}

func (s *Store) CorrectGlobalDefault(ctx context.Context, tenantID, operationID, memoryID, content string) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	continuityID, err := s.EnsureGlobalDefaultsContinuity(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	request := CommitObservationRequest{
		OperationID:        operationID,
		Kind:               ObservationKindUserCorrection,
		Content:            content,
		SourceRef:          "memory:" + strings.TrimSpace(memoryID),
		SupersedesMemoryID: strings.TrimSpace(memoryID),
	}
	if err := request.Validate(); err != nil {
		return GovernedObservationReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("begin global default correction: %w", err)
	}
	defer tx.Rollback(ctx)
	observation, err := commitObservationTx(ctx, tx, tenantID, continuityID, request)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if existing, found, err := globalDefaultMemoryByOriginTx(ctx, tx, observation.ObservationID); err != nil {
		return GovernedObservationReceipt{}, err
	} else if found {
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return GovernedObservationReceipt{}, fmt.Errorf("commit replayed global default correction: %w", err)
		}
		return GovernedObservationReceipt{Observation: observation, Memory: existing}, nil
	}
	var memoryKey string
	err = tx.QueryRow(ctx, `
SELECT memory_key
FROM governed_memories
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND memory_kind = 'global_default' AND lifecycle_status = 'active'`, request.SupersedesMemoryID, tenantID, continuityID).Scan(&memoryKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return GovernedObservationReceipt{}, fmt.Errorf("memory does not belong to the active global defaults continuity")
	}
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("resolve global default correction target: %w", err)
	}
	command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'superseded', updated_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
  AND memory_kind = 'global_default' AND lifecycle_status = 'active'`, request.SupersedesMemoryID, tenantID, continuityID)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("supersede global default: %w", err)
	}
	if command.RowsAffected() != 1 {
		return GovernedObservationReceipt{}, fmt.Errorf("memory must be an active global default")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents WHERE memory_id = $1::uuid`, request.SupersedesMemoryID); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("remove superseded global default projection: %w", err)
	}
	memory, err := insertGlobalDefaultTx(ctx, tx, tenantID, continuityID, observation.ObservationID, memoryKey, request.Content, request.SupersedesMemoryID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("commit global default correction: %w", err)
	}
	return GovernedObservationReceipt{Observation: observation, Memory: memory}, nil
}

func (s *Store) ForgetGlobalDefault(ctx context.Context, tenantID, operationID, memoryID string) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	continuityID, err := s.EnsureGlobalDefaultsContinuity(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	return s.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID:    operationID,
		Kind:           ObservationKindForgetRequest,
		Content:        "Operator requested global default deletion.",
		SourceRef:      "memory:" + strings.TrimSpace(memoryID),
		TargetMemoryID: strings.TrimSpace(memoryID),
	})
}

func globalDefaultMemoryByOriginTx(ctx context.Context, tx pgx.Tx, observationID string) (MemoryReceipt, bool, error) {
	var receipt MemoryReceipt
	err := tx.QueryRow(ctx, `
SELECT id::text, lifecycle_status
FROM governed_memories
WHERE origin_observation_id = $1::uuid AND memory_kind = 'global_default'`, observationID).Scan(&receipt.MemoryID, &receipt.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemoryReceipt{}, false, nil
	}
	if err != nil {
		return MemoryReceipt{}, false, fmt.Errorf("lookup global default by origin: %w", err)
	}
	return receipt, true, nil
}

func insertGlobalDefaultTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, observationID, memoryKey, content, supersedesMemoryID string) (MemoryReceipt, error) {
	var memoryID string
	err := tx.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, memory_key,
  lifecycle_status, content, supersedes_memory_id
)
VALUES ($1, $2::uuid, $3::uuid, 'global_default', $4, 'active', $5, NULLIF($6, '')::uuid)
RETURNING id::text`, tenantID, continuityID, observationID, memoryKey, content, supersedesMemoryID).Scan(&memoryID)
	if err != nil {
		return MemoryReceipt{}, fmt.Errorf("create global default: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
VALUES ($1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4))`, memoryID, tenantID, continuityID, content); err != nil {
		return MemoryReceipt{}, fmt.Errorf("project global default: %w", err)
	}
	return MemoryReceipt{MemoryID: memoryID, Status: "active"}, nil
}
