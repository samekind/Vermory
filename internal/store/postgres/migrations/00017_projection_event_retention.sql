-- +goose Up

ALTER TABLE memory_projection_cursors
  DROP CONSTRAINT memory_projection_cursors_status_check,
  ADD CONSTRAINT memory_projection_cursors_status_check
    CHECK (status IN ('idle', 'running', 'failed', 'rebuild_required'));

CREATE TABLE memory_projection_retention (
  tenant_id TEXT PRIMARY KEY CHECK (btrim(tenant_id) <> ''),
  pruned_through_event_id BIGINT NOT NULL DEFAULT 0
    CHECK (pruned_through_event_id >= 0),
  last_pruned_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memory_projection_prune_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  operation_id TEXT NOT NULL
    CHECK (btrim(operation_id) <> '' AND octet_length(operation_id) <= 128),
  request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
  cutoff TIMESTAMPTZ NOT NULL,
  retain_tail_events INTEGER NOT NULL CHECK (retain_tail_events >= 0),
  safe_cursor_event_id BIGINT NOT NULL CHECK (safe_cursor_event_id >= 0),
  previous_floor_event_id BIGINT NOT NULL CHECK (previous_floor_event_id >= 0),
  new_floor_event_id BIGINT NOT NULL CHECK (
    new_floor_event_id >= 0 AND new_floor_event_id >= previous_floor_event_id
  ),
  deleted_events BIGINT NOT NULL CHECK (deleted_events >= 0),
  result TEXT NOT NULL CHECK (result IN ('pruned', 'noop')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);

CREATE INDEX memory_projection_prune_runs_tenant_created_idx
  ON memory_projection_prune_runs (tenant_id, created_at DESC, id);

ALTER TABLE memory_projection_retention ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_projection_prune_runs ENABLE ROW LEVEL SECURITY;

CREATE POLICY memory_projection_retention_tenant_isolation
  ON memory_projection_retention
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

CREATE POLICY memory_projection_prune_runs_tenant_isolation
  ON memory_projection_prune_runs
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

REVOKE ALL ON TABLE memory_projection_retention FROM PUBLIC;
REVOKE ALL ON TABLE memory_projection_prune_runs FROM PUBLIC;

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM memory_projection_cursors
    WHERE status = 'rebuild_required'
  ) THEN
    RAISE EXCEPTION 'cannot downgrade while projection cursors require rebuild';
  END IF;
END
$$;
-- +goose StatementEnd

DROP POLICY IF EXISTS memory_projection_prune_runs_tenant_isolation
  ON memory_projection_prune_runs;
DROP POLICY IF EXISTS memory_projection_retention_tenant_isolation
  ON memory_projection_retention;
DROP TABLE IF EXISTS memory_projection_prune_runs;
DROP TABLE IF EXISTS memory_projection_retention;

ALTER TABLE memory_projection_cursors
  DROP CONSTRAINT memory_projection_cursors_status_check,
  ADD CONSTRAINT memory_projection_cursors_status_check
    CHECK (status IN ('idle', 'running', 'failed'));
