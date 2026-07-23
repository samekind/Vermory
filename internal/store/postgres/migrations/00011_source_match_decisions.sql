-- +goose Up
CREATE TABLE source_match_decisions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  operation_id TEXT NOT NULL CHECK (btrim(operation_id) <> ''),
  request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
  source_ref TEXT NOT NULL CHECK (btrim(source_ref) <> ''),
  source_content TEXT NOT NULL CHECK (btrim(source_content) <> ''),
  candidate_set JSONB NOT NULL CHECK (jsonb_typeof(candidate_set) = 'array'),
  candidate_set_fingerprint TEXT NOT NULL CHECK (length(candidate_set_fingerprint) = 64),
  provider_name TEXT NOT NULL CHECK (btrim(provider_name) <> ''),
  requested_model TEXT NOT NULL CHECK (btrim(requested_model) <> ''),
  resolved_model TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('pending', 'matched', 'abstained', 'failed')),
  provider_output TEXT NOT NULL DEFAULT '',
  provider_artifact_sha256 TEXT NOT NULL DEFAULT ''
    CHECK (provider_artifact_sha256 = '' OR length(provider_artifact_sha256) = 64),
  reason TEXT NOT NULL DEFAULT '',
  failure_code TEXT NOT NULL DEFAULT '',
  selected_memory_key TEXT NOT NULL DEFAULT '',
  target_memory_id UUID,
  observation_id UUID,
  candidate_memory_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id, operation_id),
  CONSTRAINT source_match_decisions_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT source_match_decisions_tenant_target_memory_fk
    FOREIGN KEY (tenant_id, target_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT source_match_decisions_tenant_observation_fk
    FOREIGN KEY (tenant_id, observation_id)
    REFERENCES observations (tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT source_match_decisions_tenant_candidate_memory_fk
    FOREIGN KEY (tenant_id, candidate_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT,
  CHECK (
    (status = 'pending'
      AND completed_at IS NULL
      AND selected_memory_key = ''
      AND target_memory_id IS NULL
      AND observation_id IS NULL
      AND candidate_memory_id IS NULL
      AND failure_code = '')
    OR
    (status = 'matched'
      AND completed_at IS NOT NULL
      AND btrim(selected_memory_key) <> ''
      AND target_memory_id IS NOT NULL
      AND observation_id IS NOT NULL
      AND failure_code = '')
    OR
    (status = 'abstained'
      AND completed_at IS NOT NULL
      AND selected_memory_key = ''
      AND target_memory_id IS NULL
      AND observation_id IS NULL
      AND candidate_memory_id IS NULL
      AND btrim(reason) <> ''
      AND failure_code = '')
    OR
    (status = 'failed'
      AND completed_at IS NOT NULL
      AND target_memory_id IS NULL
      AND observation_id IS NULL
      AND candidate_memory_id IS NULL
      AND btrim(failure_code) <> '')
  )
);

CREATE INDEX source_match_decisions_scope_idx
  ON source_match_decisions (tenant_id, continuity_id, created_at DESC);

ALTER TABLE source_match_decisions ENABLE ROW LEVEL SECURITY;

CREATE POLICY source_match_decisions_tenant_isolation ON source_match_decisions
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS source_match_decisions_tenant_isolation ON source_match_decisions;
DROP TABLE IF EXISTS source_match_decisions;
