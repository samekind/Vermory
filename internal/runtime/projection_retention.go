package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

type ProjectionRetention struct {
	TenantID             string     `json:"tenant_id"`
	PrunedThroughEventID int64      `json:"pruned_through_event_id"`
	LastPrunedAt         *time.Time `json:"last_pruned_at,omitempty"`
	UpdatedAt            *time.Time `json:"updated_at,omitempty"`
}

type ProjectionPruneRequest struct {
	OperationID      string
	Cutoff           time.Time
	RetainTailEvents int
}

type ProjectionPruneReceipt struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	OperationID          string    `json:"operation_id"`
	RequestFingerprint   string    `json:"request_fingerprint"`
	Cutoff               time.Time `json:"cutoff"`
	RetainTailEvents     int       `json:"retain_tail_events"`
	SafeCursorEventID    int64     `json:"safe_cursor_event_id"`
	PreviousFloorEventID int64     `json:"previous_floor_event_id"`
	NewFloorEventID      int64     `json:"new_floor_event_id"`
	DeletedEvents        int64     `json:"deleted_events"`
	Result               string    `json:"result"`
	CreatedAt            time.Time `json:"created_at"`
	Replayed             bool      `json:"replayed"`
}

type projectionRetentionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) ProjectionRetention(ctx context.Context, tenantID string) (ProjectionRetention, error) {
	tenantID = strings.TrimSpace(tenantID)
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ProjectionRetention{}, err
	}
	return projectionRetention(ctx, s.pool, tenantID)
}

func projectionRetention(ctx context.Context, querier projectionRetentionQuerier, tenantID string) (ProjectionRetention, error) {
	retention := ProjectionRetention{TenantID: tenantID}
	err := querier.QueryRow(ctx, `
SELECT pruned_through_event_id, last_pruned_at, updated_at
FROM memory_projection_retention
WHERE tenant_id = $1`, tenantID).Scan(
		&retention.PrunedThroughEventID,
		&retention.LastPrunedAt,
		&retention.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return retention, nil
	}
	if err != nil {
		return ProjectionRetention{}, fmt.Errorf("read projection retention: %w", err)
	}
	return retention, nil
}

func normalizeProjectionPruneRequest(tenantID string, request ProjectionPruneRequest) (string, ProjectionPruneRequest, string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if _, err := withTenantContext(context.Background(), tenantID); err != nil {
		return "", ProjectionPruneRequest{}, "", err
	}
	request.OperationID = strings.TrimSpace(request.OperationID)
	if request.OperationID == "" || len(request.OperationID) > 128 ||
		strings.IndexFunc(request.OperationID, unicode.IsControl) >= 0 {
		return "", ProjectionPruneRequest{}, "", fmt.Errorf("projection prune operation ID is invalid")
	}
	if request.Cutoff.IsZero() {
		return "", ProjectionPruneRequest{}, "", fmt.Errorf("projection prune cutoff is required")
	}
	if request.RetainTailEvents < 0 {
		return "", ProjectionPruneRequest{}, "", fmt.Errorf("projection prune retained tail must be non-negative")
	}
	request.Cutoff = request.Cutoff.UTC()
	canonical := struct {
		TenantID         string `json:"tenant_id"`
		OperationID      string `json:"operation_id"`
		Cutoff           string `json:"cutoff"`
		RetainTailEvents int    `json:"retain_tail_events"`
	}{
		TenantID: tenantID, OperationID: request.OperationID,
		Cutoff: request.Cutoff.Format(time.RFC3339Nano), RetainTailEvents: request.RetainTailEvents,
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", ProjectionPruneRequest{}, "", fmt.Errorf("encode projection prune fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return tenantID, request, hex.EncodeToString(digest[:]), nil
}

func (s *Store) PruneProjectionEvents(ctx context.Context, tenantID string, request ProjectionPruneRequest) (ProjectionPruneReceipt, error) {
	tenantID, request, fingerprint, err := normalizeProjectionPruneRequest(tenantID, request)
	if err != nil {
		return ProjectionPruneReceipt{}, err
	}
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return ProjectionPruneReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("begin projection prune: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended('vermory:projection-retention:' || $1, 0))`, tenantID); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("lock projection prune tenant: %w", err)
	}
	existing, found, err := readProjectionPruneReceipt(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return ProjectionPruneReceipt{}, err
	}
	if found {
		if existing.RequestFingerprint != fingerprint {
			return ProjectionPruneReceipt{}, fmt.Errorf("projection prune operation conflict")
		}
		existing.Replayed = true
		return existing, nil
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_projection_retention (tenant_id)
VALUES ($1)
ON CONFLICT (tenant_id) DO NOTHING`, tenantID); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("initialize projection retention: %w", err)
	}
	var previousFloor int64
	if err := tx.QueryRow(ctx, `
SELECT pruned_through_event_id
FROM memory_projection_retention
WHERE tenant_id = $1
FOR UPDATE`, tenantID).Scan(&previousFloor); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("lock projection retention: %w", err)
	}
	var safeCursor, cutoffBound, tailBound int64
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(
  (SELECT min(last_event_id)
   FROM memory_projection_cursors
   WHERE tenant_id = $1 AND status <> 'rebuild_required'),
  (SELECT COALESCE(max(event_id), 0)
   FROM memory_projection_events
   WHERE tenant_id = $1)
)`, tenantID).Scan(&safeCursor); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("read projection prune cursor bound: %w", err)
	}
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(max(event_id), 0)
FROM memory_projection_events
WHERE tenant_id = $1 AND created_at < $2`, tenantID, request.Cutoff).Scan(&cutoffBound); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("read projection prune cutoff bound: %w", err)
	}
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(max(event_id), 0)
FROM (
  SELECT event_id
  FROM memory_projection_events
  WHERE tenant_id = $1
  ORDER BY event_id DESC
  OFFSET $2
) removable`, tenantID, request.RetainTailEvents).Scan(&tailBound); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("read projection prune tail bound: %w", err)
	}
	selectedBound := minimumProjectionEventID(safeCursor, cutoffBound, tailBound)
	newFloor := previousFloor
	deletedEvents := int64(0)
	if selectedBound > previousFloor {
		if err := tx.QueryRow(ctx, `
WITH deleted AS (
  DELETE FROM memory_projection_events
  WHERE tenant_id = $1 AND event_id > $2 AND event_id <= $3
  RETURNING event_id
)
SELECT count(*), COALESCE(max(event_id), $2)
FROM deleted`, tenantID, previousFloor, selectedBound).Scan(&deletedEvents, &newFloor); err != nil {
			return ProjectionPruneReceipt{}, fmt.Errorf("delete projection events: %w", err)
		}
	}
	result := "noop"
	if deletedEvents > 0 {
		result = "pruned"
		if _, err := tx.Exec(ctx, `
UPDATE memory_projection_retention
SET pruned_through_event_id = $2, last_pruned_at = now(), updated_at = now()
WHERE tenant_id = $1`, tenantID, newFloor); err != nil {
			return ProjectionPruneReceipt{}, fmt.Errorf("advance projection retention floor: %w", err)
		}
	}
	var receipt ProjectionPruneReceipt
	err = tx.QueryRow(ctx, `
INSERT INTO memory_projection_prune_runs (
  tenant_id, operation_id, request_fingerprint, cutoff, retain_tail_events,
  safe_cursor_event_id, previous_floor_event_id, new_floor_event_id,
  deleted_events, result
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id::text, tenant_id, operation_id, request_fingerprint, cutoff,
          retain_tail_events, safe_cursor_event_id, previous_floor_event_id,
          new_floor_event_id, deleted_events, result, created_at`,
		tenantID, request.OperationID, fingerprint, request.Cutoff, request.RetainTailEvents,
		safeCursor, previousFloor, newFloor, deletedEvents, result,
	).Scan(
		&receipt.ID, &receipt.TenantID, &receipt.OperationID, &receipt.RequestFingerprint,
		&receipt.Cutoff, &receipt.RetainTailEvents, &receipt.SafeCursorEventID,
		&receipt.PreviousFloorEventID, &receipt.NewFloorEventID, &receipt.DeletedEvents,
		&receipt.Result, &receipt.CreatedAt,
	)
	if err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("record projection prune receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectionPruneReceipt{}, fmt.Errorf("commit projection prune: %w", err)
	}
	return receipt, nil
}

func readProjectionPruneReceipt(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (ProjectionPruneReceipt, bool, error) {
	var receipt ProjectionPruneReceipt
	err := tx.QueryRow(ctx, `
SELECT id::text, tenant_id, operation_id, request_fingerprint, cutoff,
       retain_tail_events, safe_cursor_event_id, previous_floor_event_id,
       new_floor_event_id, deleted_events, result, created_at
FROM memory_projection_prune_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(
		&receipt.ID, &receipt.TenantID, &receipt.OperationID, &receipt.RequestFingerprint,
		&receipt.Cutoff, &receipt.RetainTailEvents, &receipt.SafeCursorEventID,
		&receipt.PreviousFloorEventID, &receipt.NewFloorEventID, &receipt.DeletedEvents,
		&receipt.Result, &receipt.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectionPruneReceipt{}, false, nil
	}
	if err != nil {
		return ProjectionPruneReceipt{}, false, fmt.Errorf("read projection prune receipt: %w", err)
	}
	return receipt, true, nil
}

func minimumProjectionEventID(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}
