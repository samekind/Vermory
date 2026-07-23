-- +goose Up
CREATE TABLE source_formation_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  operation_id TEXT NOT NULL CHECK (btrim(operation_id) <> ''),
  request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
  source_ref TEXT NOT NULL CHECK (
    btrim(source_ref) <> ''
    AND octet_length(source_ref) <= 512
    AND position(E'\n' IN source_ref) = 0
    AND position(E'\r' IN source_ref) = 0
  ),
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256) = 64),
  source_bytes INTEGER NOT NULL CHECK (source_bytes > 0 AND source_bytes <= 65536),
  active_snapshot JSONB NOT NULL CHECK (jsonb_typeof(active_snapshot) = 'array'),
  active_snapshot_fingerprint TEXT NOT NULL CHECK (length(active_snapshot_fingerprint) = 64),
  provider_name TEXT NOT NULL CHECK (btrim(provider_name) <> ''),
  requested_model TEXT NOT NULL CHECK (btrim(requested_model) <> ''),
  resolved_model TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'abstained', 'failed')),
  provider_output TEXT NOT NULL DEFAULT '',
  provider_artifact_sha256 TEXT NOT NULL DEFAULT '' CHECK (
    provider_artifact_sha256 = '' OR length(provider_artifact_sha256) = 64
  ),
  reason TEXT NOT NULL DEFAULT '' CHECK (octet_length(reason) <= 512),
  failure_code TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id, operation_id),
  UNIQUE (tenant_id, continuity_id, id),
  CONSTRAINT source_formation_runs_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CHECK (
    (status = 'pending' AND completed_at IS NULL AND failure_code = '')
    OR
    (status = 'completed' AND completed_at IS NOT NULL AND failure_code = '')
    OR
    (status = 'abstained' AND completed_at IS NOT NULL AND btrim(reason) <> '' AND failure_code = '')
    OR
    (status = 'failed' AND completed_at IS NOT NULL AND btrim(failure_code) <> '')
  )
);

CREATE TABLE source_formation_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  run_id UUID NOT NULL,
  ordinal INTEGER NOT NULL CHECK (ordinal > 0 AND ordinal <= 16),
  decision TEXT NOT NULL CHECK (decision IN ('new', 'update', 'unchanged')),
  memory_key TEXT NOT NULL CHECK (
    octet_length(memory_key) > 0
    AND octet_length(memory_key) <= 160
    AND memory_key ~ '^[a-z0-9]+([._-][a-z0-9]+)*$'
  ),
  quote TEXT NOT NULL CHECK (octet_length(quote) > 0 AND octet_length(quote) <= 2048),
  quote_occurrence INTEGER NOT NULL CHECK (quote_occurrence > 0),
  byte_start INTEGER NOT NULL CHECK (byte_start >= 0),
  byte_end INTEGER NOT NULL,
  content TEXT NOT NULL CHECK (octet_length(content) > 0 AND octet_length(content) <= 2048),
  reason TEXT NOT NULL CHECK (octet_length(reason) > 0 AND octet_length(reason) <= 512),
  target_memory_id UUID,
  observation_id UUID NOT NULL,
  candidate_memory_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, continuity_id, run_id, ordinal),
  UNIQUE (tenant_id, continuity_id, run_id, memory_key),
  CONSTRAINT source_formation_items_tenant_run_fk
    FOREIGN KEY (tenant_id, continuity_id, run_id)
    REFERENCES source_formation_runs (tenant_id, continuity_id, id) ON DELETE CASCADE,
  CONSTRAINT source_formation_items_tenant_target_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, target_memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE RESTRICT,
  CONSTRAINT source_formation_items_tenant_observation_fk
    FOREIGN KEY (tenant_id, continuity_id, observation_id)
    REFERENCES observations (tenant_id, continuity_id, id) ON DELETE RESTRICT,
  CONSTRAINT source_formation_items_tenant_candidate_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, candidate_memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE RESTRICT,
  CHECK (byte_end > byte_start AND byte_end - byte_start = octet_length(quote)),
  CHECK (
    (decision = 'new' AND target_memory_id IS NULL AND candidate_memory_id IS NOT NULL)
    OR
    (decision = 'update' AND target_memory_id IS NOT NULL AND candidate_memory_id IS NOT NULL)
    OR
    (decision = 'unchanged' AND target_memory_id IS NOT NULL AND candidate_memory_id IS NULL)
  )
);

CREATE INDEX source_formation_runs_scope_idx
  ON source_formation_runs (tenant_id, continuity_id, created_at DESC);

CREATE INDEX source_formation_items_run_idx
  ON source_formation_items (tenant_id, continuity_id, run_id, ordinal);

ALTER TABLE source_formation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_formation_items ENABLE ROW LEVEL SECURITY;

CREATE POLICY source_formation_runs_tenant_isolation ON source_formation_runs
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

CREATE POLICY source_formation_items_tenant_isolation ON source_formation_items
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS source_formation_items_tenant_isolation ON source_formation_items;
DROP POLICY IF EXISTS source_formation_runs_tenant_isolation ON source_formation_runs;
DROP TABLE IF EXISTS source_formation_items;
DROP TABLE IF EXISTS source_formation_runs;
