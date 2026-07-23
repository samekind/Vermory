-- +goose Up
ALTER TABLE conversation_formation_schedules
  ADD COLUMN IF NOT EXISTS window_fingerprint TEXT NOT NULL DEFAULT ''
  CHECK (window_fingerprint = '' OR window_fingerprint ~ '^[0-9a-f]{64}$');

ALTER TABLE observations
  DROP CONSTRAINT observations_observation_kind_check;

ALTER TABLE observations
  ADD CONSTRAINT observations_observation_kind_check
  CHECK (observation_kind IN (
    'agent_result',
    'user_correction',
    'source_update',
    'forget_request',
    'user_message',
    'assistant_message',
    'user_confirmation',
    'global_default_set',
    'bridge_promote',
    'source_candidate',
    'candidate_rejection',
    'tool_result'
  ));

ALTER TABLE conversation_turns
  ADD CONSTRAINT conversation_turns_tenant_continuity_id_key
  UNIQUE (tenant_id, continuity_id, id);

CREATE TABLE conversation_tool_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  turn_id UUID NOT NULL,
  observation_id UUID NOT NULL,
  run_id TEXT NOT NULL CHECK (btrim(run_id) <> '' AND octet_length(run_id) <= 512),
  tool_name TEXT NOT NULL CHECK (
    btrim(tool_name) <> ''
    AND octet_length(tool_name) <= 128
    AND tool_name ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]*$'
  ),
  tool_call_id TEXT NOT NULL CHECK (btrim(tool_call_id) <> '' AND octet_length(tool_call_id) <= 512),
  content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT conversation_tool_results_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT conversation_tool_results_tenant_turn_fk
    FOREIGN KEY (tenant_id, continuity_id, turn_id)
    REFERENCES conversation_turns (tenant_id, continuity_id, id) ON DELETE CASCADE,
  CONSTRAINT conversation_tool_results_tenant_observation_fk
    FOREIGN KEY (tenant_id, continuity_id, observation_id)
    REFERENCES observations (tenant_id, continuity_id, id) ON DELETE CASCADE,
  UNIQUE (tenant_id, turn_id, tool_call_id),
  UNIQUE (tenant_id, observation_id)
);

CREATE INDEX conversation_tool_results_continuity_idx
  ON conversation_tool_results (tenant_id, continuity_id, created_at);

ALTER TABLE conversation_tool_results ENABLE ROW LEVEL SECURITY;

CREATE POLICY conversation_tool_results_tenant_isolation
  ON conversation_tool_results
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS conversation_tool_results_tenant_isolation
  ON conversation_tool_results;
DROP TABLE IF EXISTS conversation_tool_results;

-- `window_fingerprint` may predate this migration; keep it on downgrade.

ALTER TABLE conversation_turns
  DROP CONSTRAINT IF EXISTS conversation_turns_tenant_continuity_id_key;

UPDATE observations SET observation_kind = 'agent_result' WHERE observation_kind = 'tool_result';

ALTER TABLE observations
  DROP CONSTRAINT observations_observation_kind_check;

ALTER TABLE observations
  ADD CONSTRAINT observations_observation_kind_check
  CHECK (observation_kind IN (
    'agent_result',
    'user_correction',
    'source_update',
    'forget_request',
    'user_message',
    'assistant_message',
    'user_confirmation',
    'global_default_set',
    'bridge_promote',
    'source_candidate',
    'candidate_rejection'
  ));
