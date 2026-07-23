-- +goose Up
ALTER TABLE continuity_spaces
  DROP CONSTRAINT continuity_spaces_continuity_line_check;

ALTER TABLE continuity_spaces
  ADD CONSTRAINT continuity_spaces_continuity_line_check
  CHECK (continuity_line IN ('workspace', 'conversation'));

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

ALTER TABLE observations
  ADD COLUMN observation_seq BIGSERIAL NOT NULL;

CREATE UNIQUE INDEX observations_sequence_idx
  ON observations (observation_seq);

CREATE TABLE conversation_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  channel TEXT NOT NULL,
  thread_id TEXT NOT NULL,
  binding_state TEXT NOT NULL CHECK (binding_state IN ('confirmed', 'retired')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX conversation_bindings_continuity_idx
  ON conversation_bindings (tenant_id, continuity_id);

CREATE UNIQUE INDEX conversation_bindings_confirmed_anchor_idx
  ON conversation_bindings (tenant_id, channel, thread_id)
  WHERE binding_state = 'confirmed';

-- +goose Down
DROP TABLE IF EXISTS conversation_bindings;
DROP INDEX IF EXISTS observations_sequence_idx;
ALTER TABLE observations DROP COLUMN IF EXISTS observation_seq;

ALTER TABLE observations
  DROP CONSTRAINT observations_observation_kind_check;

ALTER TABLE observations
  ADD CONSTRAINT observations_observation_kind_check
  CHECK (observation_kind IN ('agent_result', 'user_correction', 'source_update', 'forget_request'));

ALTER TABLE continuity_spaces
  DROP CONSTRAINT continuity_spaces_continuity_line_check;

ALTER TABLE continuity_spaces
  ADD CONSTRAINT continuity_spaces_continuity_line_check
  CHECK (continuity_line = 'workspace');
