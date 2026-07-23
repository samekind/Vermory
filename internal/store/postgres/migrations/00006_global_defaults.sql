-- +goose Up
ALTER TABLE continuity_spaces
  DROP CONSTRAINT continuity_spaces_continuity_line_check;

ALTER TABLE continuity_spaces
  ADD CONSTRAINT continuity_spaces_continuity_line_check
  CHECK (continuity_line IN ('workspace', 'conversation', 'global_defaults'));

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
    'global_default_set'
  ));

ALTER TABLE governed_memories
  ADD COLUMN memory_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX continuity_spaces_active_global_defaults_tenant_idx
  ON continuity_spaces (tenant_id)
  WHERE continuity_line = 'global_defaults' AND state = 'active';

CREATE UNIQUE INDEX governed_memories_active_global_default_key_idx
  ON governed_memories (tenant_id, continuity_id, memory_key)
  WHERE lifecycle_status = 'active' AND memory_kind = 'global_default';

-- +goose Down
DROP INDEX IF EXISTS governed_memories_active_global_default_key_idx;
DROP INDEX IF EXISTS continuity_spaces_active_global_defaults_tenant_idx;

ALTER TABLE governed_memories
  DROP COLUMN IF EXISTS memory_key;

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
    'user_confirmation'
  ));

ALTER TABLE continuity_spaces
  DROP CONSTRAINT continuity_spaces_continuity_line_check;

ALTER TABLE continuity_spaces
  ADD CONSTRAINT continuity_spaces_continuity_line_check
  CHECK (continuity_line IN ('workspace', 'conversation'));
