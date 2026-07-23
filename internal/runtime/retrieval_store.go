package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RetrievalProjectionStatus(ctx context.Context, tenantID, profileID string) (ProjectionStatus, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ProjectionStatus{}, err
	}
	return retrievalProjectionStatus(ctx, s.pool, tenantID, profileID)
}

type retrievalStatusQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func retrievalProjectionStatus(ctx context.Context, querier retrievalStatusQuerier, tenantID, profileID string) (ProjectionStatus, error) {
	projectionSQL, err := retrievalProjectionSQLForProfile(profileID)
	if err != nil {
		return ProjectionStatus{}, err
	}
	status := ProjectionStatus{
		TenantID:  tenantID,
		ProfileID: profileID,
		Status:    "idle",
	}
	err = querier.QueryRow(ctx, `
SELECT last_event_id, status, attempt_count, last_error_code, last_attempt_at
FROM memory_projection_cursors
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, profileID).Scan(
		&status.LastEventID,
		&status.Status,
		&status.AttemptCount,
		&status.LastErrorCode,
		&status.LastAttemptAt,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ProjectionStatus{}, fmt.Errorf("read retrieval projection cursor: %w", err)
	}
	retention, err := projectionRetention(ctx, querier, tenantID)
	if err != nil {
		return ProjectionStatus{}, err
	}
	status.PrunedThroughEventID = retention.PrunedThroughEventID
	status.RebuildRequired = status.Status == ProjectionStatusRebuildRequired ||
		status.LastEventID < status.PrunedThroughEventID
	if err := querier.QueryRow(ctx, `
SELECT GREATEST(COALESCE((
         SELECT max(event_id)
         FROM memory_projection_events
         WHERE tenant_id = $1
       ), 0), $2, $3),
       (SELECT count(*)
        FROM memory_projection_events
        WHERE tenant_id = $1 AND event_id > $2)`, tenantID, status.LastEventID, status.PrunedThroughEventID).Scan(&status.LatestEventID, &status.Lag); err != nil {
		return ProjectionStatus{}, fmt.Errorf("read latest retrieval projection event: %w", err)
	}
	if err := querier.QueryRow(ctx, projectionSQL.count, tenantID, profileID).Scan(&status.VectorCount); err != nil {
		return ProjectionStatus{}, fmt.Errorf("count retrieval vector documents: %w", err)
	}
	return status, nil
}

func (s *Store) ResetVectorProjection(ctx context.Context, tenantID, profileID string) error {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return err
	}
	projectionSQL, err := retrievalProjectionSQLForProfile(profileID)
	if err != nil {
		return err
	}
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire vector projection reset connection: %w", err)
	}
	defer connection.Release()
	lockKey1, lockKey2 := projectionAdvisoryLockKeys(profileID, tenantID)
	var locked bool
	if err := connection.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, $2)`, lockKey1, lockKey2).Scan(&locked); err != nil {
		return fmt.Errorf("acquire vector projection reset lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("retrieval projection worker is already running")
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock($1, $2)`, lockKey1, lockKey2)
	}()
	tx, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin vector projection reset: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, projectionSQL.clearTenant, tenantID, profileID); err != nil {
		return fmt.Errorf("clear vector projection: %w", err)
	}
	var floor int64
	if err := tx.QueryRow(ctx, `
SELECT COALESCE((
  SELECT pruned_through_event_id
  FROM memory_projection_retention
  WHERE tenant_id = $1
), 0)`, tenantID).Scan(&floor); err != nil {
		return fmt.Errorf("read vector projection retention floor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_projection_cursors (
  tenant_id, profile_id, last_event_id, status, attempt_count,
  last_error_code, last_attempt_at, updated_at
) VALUES ($1, $2, $3, 'rebuild_required', 0, 'projection_rebuild_required', NULL, now())
ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
  last_event_id = EXCLUDED.last_event_id,
  status = 'rebuild_required',
  attempt_count = 0,
  last_error_code = 'projection_rebuild_required',
  last_attempt_at = NULL,
  updated_at = now()`, tenantID, profileID, floor); err != nil {
		return fmt.Errorf("reset vector projection cursor: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit vector projection reset: %w", err)
	}
	return nil
}

func (s *Store) searchActiveVectorMemory(ctx context.Context, tenantID string, continuityIDs []string, queryVector []float32, limit int, profileID string) ([]Memory, error) {
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.searchEligibleVectorMemoryAt(
		ctx, tenantID, continuityIDs, queryVector, limit, profileID, snapshot.AsOf,
	)
}

func (s *Store) searchEligibleVectorMemoryAt(ctx context.Context, tenantID string, continuityIDs []string, queryVector []float32, limit int, profileID string, asOf time.Time) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	projectionSQL, err := retrievalProjectionSQLForProfile(profileID)
	if err != nil {
		return nil, err
	}
	candidateLimit := limit * 4
	if candidateLimit < 20 {
		candidateLimit = 20
	}
	if candidateLimit > 100 {
		candidateLimit = 100
	}
	rows, err := s.pool.Query(
		ctx, projectionSQL.search, profileID, tenantID, continuityIDs,
		retrievalVectorLiteral(queryVector), candidateLimit, limit, asOf,
	)
	if err != nil {
		return nil, fmt.Errorf("search active vector memory: %w", err)
	}
	defer rows.Close()
	memories := make([]Memory, 0)
	for rows.Next() {
		var memory Memory
		if err := rows.Scan(&memory.ID, &memory.Content); err != nil {
			return nil, fmt.Errorf("scan active vector memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active vector memory: %w", err)
	}
	return memories, nil
}

func (s *Store) checkRetrievalAuditReplay(ctx context.Context, tenantID, operationID, fingerprint string) (string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", err
	}
	var auditID, existingFingerprint string
	err = s.pool.QueryRow(ctx, `
SELECT id::text, request_fingerprint
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&auditID, &existingFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup retrieval audit replay: %w", err)
	}
	if existingFingerprint != fingerprint {
		return "", fmt.Errorf("retrieval audit operation conflict")
	}
	return auditID, nil
}

func (s *Store) retrievalAuditEligibilityAsOf(ctx context.Context, tenantID, operationID string) (time.Time, bool, error) {
	if operationID == "" {
		return time.Time{}, false, nil
	}
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return time.Time{}, false, err
	}
	var asOf time.Time
	err = s.pool.QueryRow(ctx, `
SELECT eligibility_as_of
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&asOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("lookup retrieval audit eligibility time: %w", err)
	}
	return asOf.UTC(), true, nil
}

func (s *Store) recordRetrievalAudit(ctx context.Context, input retrievalAuditInput) (string, error) {
	ctx, err := withTenantContext(ctx, input.TenantID)
	if err != nil {
		return "", err
	}
	var auditID string
	err = s.pool.QueryRow(ctx, `
INSERT INTO memory_retrieval_runs (
  tenant_id, primary_continuity_id, continuity_ids, operation_id,
  request_fingerprint, requested_mode, effective_mode, profile_id,
  query_sha256, lexical_memory_ids, vector_memory_ids, delivered_memory_ids,
  projection_current, degraded, failure_code, lexical_latency_ms, vector_latency_ms,
  eligibility_as_of
) VALUES (
  $1, $2::uuid, $3::uuid[], $4,
  $5, $6, $7, $8,
  $9, $10::uuid[], $11::uuid[], $12::uuid[],
  $13, $14, $15, $16, $17, $18
)
ON CONFLICT (tenant_id, operation_id) DO UPDATE SET
  operation_id = memory_retrieval_runs.operation_id
WHERE memory_retrieval_runs.request_fingerprint = EXCLUDED.request_fingerprint
RETURNING id::text`,
		input.TenantID,
		input.PrimaryContinuityID,
		input.ContinuityIDs,
		input.OperationID,
		input.RequestFingerprint,
		input.RequestedMode,
		input.EffectiveMode,
		input.ProfileID,
		input.QuerySHA256,
		input.LexicalMemoryIDs,
		input.VectorMemoryIDs,
		input.DeliveredMemoryIDs,
		input.ProjectionCurrent,
		input.Degraded,
		input.FailureCode,
		durationMilliseconds(input.LexicalLatency),
		durationMilliseconds(input.VectorLatency),
		input.EligibilityAsOf,
	).Scan(&auditID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("retrieval audit operation conflict")
	}
	if err != nil {
		return "", fmt.Errorf("record retrieval audit: %w", err)
	}
	return auditID, nil
}

func durationMilliseconds(duration time.Duration) int {
	milliseconds := duration.Milliseconds()
	if milliseconds < 0 {
		return 0
	}
	if milliseconds > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(milliseconds)
}
