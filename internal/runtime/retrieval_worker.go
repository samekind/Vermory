package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectionWorker struct {
	store    *Store
	embedder Embedder
	options  ProjectionWorkerOptions
}

func NewProjectionWorker(store *Store, embedder Embedder, options ProjectionWorkerOptions) (*ProjectionWorker, error) {
	if store == nil {
		return nil, fmt.Errorf("projection worker store is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("projection worker embedder is required")
	}
	if err := options.normalize(); err != nil {
		return nil, err
	}
	return &ProjectionWorker{store: store, embedder: embedder, options: options}, nil
}

func (w *ProjectionWorker) RunOnce(ctx context.Context) (ProjectionRunResult, error) {
	tenantCtx, err := withTenantContext(ctx, w.options.TenantID)
	if err != nil {
		return ProjectionRunResult{}, err
	}
	connection, err := w.store.pool.Acquire(tenantCtx)
	if err != nil {
		return ProjectionRunResult{}, fmt.Errorf("acquire projection worker connection: %w", err)
	}
	defer connection.Release()

	lockKey1, lockKey2 := projectionAdvisoryLockKeys(w.options.Profile.ID, w.options.TenantID)
	var locked bool
	if err := connection.QueryRow(tenantCtx, `SELECT pg_try_advisory_lock($1, $2)`, lockKey1, lockKey2).Scan(&locked); err != nil {
		return ProjectionRunResult{}, fmt.Errorf("acquire projection worker lock: %w", err)
	}
	if !locked {
		status, statusErr := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
		if statusErr != nil {
			return ProjectionRunResult{}, statusErr
		}
		return projectionResult(status, 0, "already_running", true), nil
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock($1, $2)`, lockKey1, lockKey2)
	}()

	rebuildRequired, err := ensureProjectionCursor(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
	if err != nil {
		return ProjectionRunResult{}, err
	}
	if rebuildRequired {
		status, statusErr := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
		if statusErr != nil {
			return ProjectionRunResult{}, statusErr
		}
		return projectionResult(status, 0, ProjectionFailureRebuildRequired, false),
			projectionRunError{code: ProjectionFailureRebuildRequired}
	}
	events, err := nextProjectionEvents(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID, w.options.BatchSize)
	if err != nil {
		return w.fail(ctx, connection, 0, "projection_read_error")
	}
	prepared, err := w.prepareEvents(tenantCtx, connection, events)
	if err != nil {
		var coded projectionRunError
		if errors.As(err, &coded) {
			return w.fail(ctx, connection, 0, coded.code)
		}
		return w.fail(ctx, connection, 0, "projection_read_error")
	}
	processed := 0
	for _, event := range prepared {
		if err := w.processPreparedEvent(tenantCtx, connection, event); err != nil {
			var coded projectionRunError
			if errors.As(err, &coded) {
				return w.fail(ctx, connection, processed, coded.code)
			}
			return w.fail(ctx, connection, processed, "projection_write_error")
		}
		processed++
	}
	if _, err := connection.Exec(tenantCtx, `
UPDATE memory_projection_cursors
SET status = 'idle', last_error_code = '', updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, w.options.TenantID, w.options.Profile.ID); err != nil {
		return ProjectionRunResult{}, fmt.Errorf("finish projection worker cursor: %w", err)
	}
	status, err := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
	if err != nil {
		return ProjectionRunResult{}, err
	}
	return projectionResult(status, processed, "", false), nil
}

func (w *ProjectionWorker) RebuildCurrent(ctx context.Context) (ProjectionRebuildResult, error) {
	tenantCtx, err := withTenantContext(ctx, w.options.TenantID)
	if err != nil {
		return ProjectionRebuildResult{}, err
	}
	projectionSQL, err := retrievalProjectionSQLForClass(w.options.Profile.ProjectionClass)
	if err != nil {
		return ProjectionRebuildResult{}, err
	}
	connection, err := w.store.pool.Acquire(tenantCtx)
	if err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("acquire projection rebuild connection: %w", err)
	}
	defer connection.Release()

	lockKey1, lockKey2 := projectionAdvisoryLockKeys(w.options.Profile.ID, w.options.TenantID)
	var locked bool
	if err := connection.QueryRow(tenantCtx, `SELECT pg_try_advisory_lock($1, $2)`, lockKey1, lockKey2).Scan(&locked); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("acquire projection rebuild lock: %w", err)
	}
	if !locked {
		status, statusErr := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
		if statusErr != nil {
			return ProjectionRebuildResult{}, statusErr
		}
		return projectionRebuildResult(status, 0, 0, 0, status.LastEventID, "already_running", true), nil
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock($1, $2)`, lockKey1, lockKey2)
	}()

	var watermark int64
	if err := connection.QueryRow(tenantCtx, `
	SELECT GREATEST(
	  COALESCE((
	    SELECT max(event_id)
	    FROM memory_projection_events
	    WHERE tenant_id = $1
	  ), 0),
	  COALESCE((
	    SELECT pruned_through_event_id
	    FROM memory_projection_retention
	    WHERE tenant_id = $1
	  ), 0)
	)`, w.options.TenantID).Scan(&watermark); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("read projection rebuild watermark: %w", err)
	}
	tx, err := connection.Begin(tenantCtx)
	if err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("begin projection rebuild reset: %w", err)
	}
	defer tx.Rollback(tenantCtx)
	if _, err := tx.Exec(tenantCtx, projectionSQL.clearTenant, w.options.TenantID, w.options.Profile.ID); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("clear projection rebuild vectors: %w", err)
	}
	if _, err := tx.Exec(tenantCtx, `
INSERT INTO memory_projection_cursors (
  tenant_id, profile_id, status, attempt_count, last_error_code, last_attempt_at, updated_at
) VALUES ($1, $2, 'running', 0, '', now(), now())
ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
  status = 'running',
  last_error_code = '',
  last_attempt_at = now(),
  updated_at = now()`, w.options.TenantID, w.options.Profile.ID); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("initialize projection rebuild cursor: %w", err)
	}
	if err := tx.Commit(tenantCtx); err != nil {
		return ProjectionRebuildResult{}, fmt.Errorf("commit projection rebuild reset: %w", err)
	}

	result := ProjectionRebuildResult{Watermark: watermark}
	lastMemoryID := ""
	for {
		page, err := loadProjectionSnapshotPage(
			tenantCtx, connection, w.options.TenantID, lastMemoryID, w.options.SnapshotPageSize,
		)
		if err != nil {
			return w.failRebuild(ctx, connection, result, "projection_read_error")
		}
		if len(page) == 0 {
			break
		}
		texts := make([]string, len(page))
		for index, memory := range page {
			texts[index] = memory.Content
		}
		vectors, err := w.embedTexts(tenantCtx, texts)
		if err != nil {
			var dimensionErr embeddingDimensionError
			if errors.As(err, &dimensionErr) {
				return w.failRebuild(ctx, connection, result, "embedding_dimension_mismatch")
			}
			return w.failRebuild(ctx, connection, result, "embedding_unavailable")
		}
		for index, memory := range page {
			result.Scanned++
			vector := vectors[index]
			if len(vector) != w.options.Profile.Dimensions {
				return w.failRebuild(ctx, connection, result, "embedding_dimension_mismatch")
			}
			hash := sha256.Sum256([]byte(memory.Content))
			mutation, err := connection.Exec(tenantCtx, projectionSQL.upsertSnapshot,
				w.options.Profile.ID,
				w.options.TenantID,
				memory.MemoryID,
				memory.ContinuityID,
				hex.EncodeToString(hash[:]),
				retrievalVectorLiteral(vector),
				memory.Content,
				memory.UpdatedAt,
			)
			if err != nil {
				return w.failRebuild(ctx, connection, result, "projection_write_error")
			}
			if mutation.RowsAffected() == 0 {
				result.SkippedChanged++
			} else {
				result.Projected++
			}
			lastMemoryID = memory.MemoryID
		}
	}
	if _, err := connection.Exec(tenantCtx, `
UPDATE memory_projection_cursors
SET last_event_id = GREATEST(last_event_id, $3),
    status = 'idle',
    attempt_count = attempt_count + 1,
    last_error_code = '',
    last_attempt_at = now(),
    updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, w.options.TenantID, w.options.Profile.ID, watermark); err != nil {
		return w.failRebuild(ctx, connection, result, "projection_write_error")
	}
	status, err := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
	if err != nil {
		return ProjectionRebuildResult{}, err
	}
	return projectionRebuildResult(status, result.Scanned, result.Projected, result.SkippedChanged, watermark, "", false), nil
}

type projectionSnapshotMemory struct {
	MemoryID     string
	ContinuityID string
	Content      string
	UpdatedAt    time.Time
}

func loadProjectionSnapshotPage(
	ctx context.Context,
	connection *pgxpool.Conn,
	tenantID string,
	lastMemoryID string,
	limit int,
) ([]projectionSnapshotMemory, error) {
	rows, err := connection.Query(ctx, `
SELECT id::text, continuity_id::text, content, updated_at
FROM governed_memories
WHERE tenant_id = $1
  AND memory_kind = 'fact'
  AND lifecycle_status = 'active'
  AND content <> '[redacted]'
  AND id > COALESCE(NULLIF($2, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
ORDER BY id
LIMIT $3`, tenantID, lastMemoryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := make([]projectionSnapshotMemory, 0, limit)
	for rows.Next() {
		var memory projectionSnapshotMemory
		if err := rows.Scan(&memory.MemoryID, &memory.ContinuityID, &memory.Content, &memory.UpdatedAt); err != nil {
			return nil, err
		}
		page = append(page, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return page, nil
}

func (w *ProjectionWorker) failRebuild(
	ctx context.Context,
	connection *pgxpool.Conn,
	result ProjectionRebuildResult,
	code string,
) (ProjectionRebuildResult, error) {
	tenantCtx, tenantErr := withTenantContext(ctx, w.options.TenantID)
	if tenantErr == nil {
		_, _ = connection.Exec(tenantCtx, `
UPDATE memory_projection_cursors
SET status = 'failed',
    attempt_count = attempt_count + 1,
    last_error_code = $3,
    last_attempt_at = now(),
    updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, w.options.TenantID, w.options.Profile.ID, code)
	}
	status, err := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ProjectionRebuildResult{}, ctxErr
		}
		return ProjectionRebuildResult{}, err
	}
	return projectionRebuildResult(
		status, result.Scanned, result.Projected, result.SkippedChanged, result.Watermark, code, false,
	), projectionRunError{code: code}
}

func projectionRebuildResult(
	status ProjectionStatus,
	scanned int,
	projected int,
	skippedChanged int,
	watermark int64,
	failureCode string,
	alreadyRunning bool,
) ProjectionRebuildResult {
	return ProjectionRebuildResult{
		Scanned:        scanned,
		Projected:      projected,
		SkippedChanged: skippedChanged,
		Watermark:      watermark,
		LastEventID:    status.LastEventID,
		LatestEventID:  status.LatestEventID,
		Lag:            status.Lag,
		Status:         status.Status,
		FailureCode:    failureCode,
		AlreadyRunning: alreadyRunning,
	}
}

func projectionAdvisoryLockKeys(profileID, tenantID string) (int32, int32) {
	digest := sha256.Sum256([]byte(profileID + "\x00" + tenantID))
	return int32(binary.BigEndian.Uint32(digest[:4])), int32(binary.BigEndian.Uint32(digest[4:8]))
}

func (w *ProjectionWorker) Run(ctx context.Context) error {
	for {
		if _, err := w.RunOnce(ctx); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		timer := time.NewTimer(w.options.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type preparedProjectionEvent struct {
	event         ProjectionEvent
	before        projectionMemory
	shouldProject bool
	vector        []float32
}

func (w *ProjectionWorker) prepareEvents(ctx context.Context, connection *pgxpool.Conn, events []ProjectionEvent) ([]preparedProjectionEvent, error) {
	prepared := make([]preparedProjectionEvent, len(events))
	texts := make([]string, 0, len(events))
	textIndexes := make([]int, 0, len(events))
	for index, event := range events {
		before, err := loadProjectionMemory(ctx, connection, event.TenantID, event.MemoryID)
		if err != nil {
			return nil, projectionRunError{code: "projection_read_error"}
		}
		shouldProject := before.Exists && before.Kind == "fact" && before.Status == "active" && before.Content != "[redacted]"
		prepared[index] = preparedProjectionEvent{event: event, before: before, shouldProject: shouldProject}
		if shouldProject {
			texts = append(texts, before.Content)
			textIndexes = append(textIndexes, index)
		}
	}
	vectors, err := w.embedTexts(ctx, texts)
	if err != nil {
		var dimensionErr embeddingDimensionError
		if errors.As(err, &dimensionErr) {
			return nil, projectionRunError{code: "embedding_dimension_mismatch"}
		}
		return nil, projectionRunError{code: "embedding_unavailable"}
	}
	for vectorIndex, preparedIndex := range textIndexes {
		if len(vectors[vectorIndex]) != w.options.Profile.Dimensions {
			return nil, projectionRunError{code: "embedding_dimension_mismatch"}
		}
		prepared[preparedIndex].vector = vectors[vectorIndex]
	}
	return prepared, nil
}

func (w *ProjectionWorker) embedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if w.options.Profile.InputPolicy != (EmbeddingInputPolicy{}) {
		vectors, _, err := embedLogicalTexts(
			ctx,
			w.embedder,
			texts,
			w.options.EmbeddingBatchSize,
			w.options.Profile.Dimensions,
			w.options.Profile.InputPolicy,
		)
		return vectors, err
	}
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	vectors := make([][]float32, 0, len(texts))
	batch, batchCapable := w.embedder.(BatchEmbedder)
	for start := 0; start < len(texts); start += w.options.EmbeddingBatchSize {
		end := start + w.options.EmbeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		if batchCapable && w.options.EmbeddingBatchSize > 1 {
			batchVectors, err := batch.EmbedBatch(ctx, texts[start:end])
			if err != nil {
				return nil, err
			}
			if len(batchVectors) != end-start {
				return nil, fmt.Errorf("embedding batch returned %d vectors, want %d", len(batchVectors), end-start)
			}
			vectors = append(vectors, batchVectors...)
			continue
		}
		for _, text := range texts[start:end] {
			vector, err := w.embedder.Embed(ctx, text)
			if err != nil {
				return nil, err
			}
			vectors = append(vectors, vector)
		}
	}
	if observer, ok := w.embedder.(LogicalEmbeddingObserver); ok {
		observer.RecordLogicalEmbeddingSuccess(len(vectors))
	}
	return vectors, nil
}

func (w *ProjectionWorker) processPreparedEvent(ctx context.Context, connection *pgxpool.Conn, prepared preparedProjectionEvent) error {
	projectionSQL, err := retrievalProjectionSQLForClass(w.options.Profile.ProjectionClass)
	if err != nil {
		return projectionRunError{code: "projection_write_error"}
	}
	tx, err := connection.Begin(ctx)
	if err != nil {
		return projectionRunError{code: "projection_write_error"}
	}
	defer tx.Rollback(ctx)
	after, err := loadProjectionMemoryTx(ctx, tx, prepared.event.TenantID, prepared.event.MemoryID)
	if err != nil {
		return projectionRunError{code: "projection_read_error"}
	}
	before := prepared.before
	if before.Exists != after.Exists || before.ContinuityID != after.ContinuityID || before.Kind != after.Kind || before.Status != after.Status || before.Content != after.Content || !before.UpdatedAt.Equal(after.UpdatedAt) {
		return projectionRunError{code: "authority_changed"}
	}
	if !prepared.shouldProject {
		if _, err := tx.Exec(ctx, projectionSQL.deleteMemory,
			w.options.Profile.ID, prepared.event.TenantID, prepared.event.MemoryID); err != nil {
			return projectionRunError{code: "projection_write_error"}
		}
	} else {
		hash := sha256.Sum256([]byte(after.Content))
		if _, err := tx.Exec(ctx, projectionSQL.upsertMemory,
			w.options.Profile.ID,
			prepared.event.TenantID,
			after.ContinuityID,
			prepared.event.MemoryID,
			hex.EncodeToString(hash[:]),
			retrievalVectorLiteral(prepared.vector),
		); err != nil {
			return projectionRunError{code: "projection_write_error"}
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE memory_projection_cursors
SET last_event_id = $3,
    status = 'running',
    attempt_count = attempt_count + 1,
    last_error_code = '',
    last_attempt_at = now(),
    updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, prepared.event.TenantID, w.options.Profile.ID, prepared.event.EventID); err != nil {
		return projectionRunError{code: "projection_write_error"}
	}
	if err := tx.Commit(ctx); err != nil {
		return projectionRunError{code: "projection_write_error"}
	}
	return nil
}

func (w *ProjectionWorker) fail(ctx context.Context, connection *pgxpool.Conn, processed int, code string) (ProjectionRunResult, error) {
	tenantCtx, tenantErr := withTenantContext(ctx, w.options.TenantID)
	if tenantErr == nil {
		_, _ = connection.Exec(tenantCtx, `
UPDATE memory_projection_cursors
SET status = 'failed',
    attempt_count = attempt_count + 1,
    last_error_code = $3,
    last_attempt_at = now(),
    updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, w.options.TenantID, w.options.Profile.ID, code)
	}
	status, err := retrievalProjectionStatus(tenantCtx, connection, w.options.TenantID, w.options.Profile.ID)
	if err != nil {
		return ProjectionRunResult{}, err
	}
	return projectionResult(status, processed, code, false), projectionRunError{code: code}
}

func ensureProjectionCursor(ctx context.Context, connection *pgxpool.Conn, tenantID, profileID string) (bool, error) {
	var rebuildRequired bool
	err := connection.QueryRow(ctx, `
INSERT INTO memory_projection_cursors (
  tenant_id, profile_id, last_event_id, status, last_error_code, last_attempt_at
)
SELECT $1, $2, boundary.floor,
       CASE WHEN boundary.floor > 0 THEN 'rebuild_required' ELSE 'running' END,
       CASE WHEN boundary.floor > 0 THEN 'projection_rebuild_required' ELSE '' END,
       now()
FROM (
  SELECT COALESCE((
    SELECT pruned_through_event_id
    FROM memory_projection_retention
    WHERE tenant_id = $1
  ), 0) AS floor
) boundary
ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
  status = CASE
    WHEN memory_projection_cursors.status = 'rebuild_required'
      OR memory_projection_cursors.last_event_id < EXCLUDED.last_event_id
    THEN 'rebuild_required'
    ELSE 'running'
  END,
  last_error_code = CASE
    WHEN memory_projection_cursors.status = 'rebuild_required'
      OR memory_projection_cursors.last_event_id < EXCLUDED.last_event_id
    THEN 'projection_rebuild_required'
    ELSE ''
  END,
  last_attempt_at = now(),
  updated_at = now()
RETURNING status = 'rebuild_required'`, tenantID, profileID).Scan(&rebuildRequired)
	if err != nil {
		return false, fmt.Errorf("initialize projection cursor: %w", err)
	}
	return rebuildRequired, nil
}

func nextProjectionEvents(ctx context.Context, connection *pgxpool.Conn, tenantID, profileID string, limit int) ([]ProjectionEvent, error) {
	rows, err := connection.Query(ctx, `
SELECT event.event_id, event.tenant_id, event.continuity_id::text,
       event.memory_id::text, event.desired_state, event.authority_version
FROM memory_projection_events event
JOIN memory_projection_cursors cursor
  ON cursor.tenant_id = event.tenant_id AND cursor.profile_id = $2
WHERE event.tenant_id = $1 AND event.event_id > cursor.last_event_id
ORDER BY event.event_id
LIMIT $3`, tenantID, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]ProjectionEvent, 0, limit)
	for rows.Next() {
		var event ProjectionEvent
		if err := rows.Scan(&event.EventID, &event.TenantID, &event.ContinuityID, &event.MemoryID, &event.DesiredState, &event.AuthorityVersion); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type projectionMemory struct {
	Exists       bool
	ContinuityID string
	Kind         string
	Status       string
	Content      string
	UpdatedAt    time.Time
}

func loadProjectionMemory(ctx context.Context, connection *pgxpool.Conn, tenantID, memoryID string) (projectionMemory, error) {
	var memory projectionMemory
	err := connection.QueryRow(ctx, `
SELECT continuity_id::text, memory_kind, lifecycle_status, content, updated_at
FROM governed_memories
WHERE tenant_id = $1 AND id = $2::uuid`, tenantID, memoryID).Scan(
		&memory.ContinuityID,
		&memory.Kind,
		&memory.Status,
		&memory.Content,
		&memory.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return memory, nil
	}
	if err != nil {
		return projectionMemory{}, err
	}
	memory.Exists = true
	return memory, nil
}

func loadProjectionMemoryTx(ctx context.Context, tx pgx.Tx, tenantID, memoryID string) (projectionMemory, error) {
	var memory projectionMemory
	err := tx.QueryRow(ctx, `
SELECT continuity_id::text, memory_kind, lifecycle_status, content, updated_at
FROM governed_memories
WHERE tenant_id = $1 AND id = $2::uuid
FOR SHARE`, tenantID, memoryID).Scan(
		&memory.ContinuityID,
		&memory.Kind,
		&memory.Status,
		&memory.Content,
		&memory.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return memory, nil
	}
	if err != nil {
		return projectionMemory{}, err
	}
	memory.Exists = true
	return memory, nil
}

func projectionResult(status ProjectionStatus, processed int, failureCode string, alreadyRunning bool) ProjectionRunResult {
	return ProjectionRunResult{
		Processed:      processed,
		LastEventID:    status.LastEventID,
		LatestEventID:  status.LatestEventID,
		Lag:            status.Lag,
		Status:         status.Status,
		FailureCode:    failureCode,
		AlreadyRunning: alreadyRunning,
	}
}

func retrievalVectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
