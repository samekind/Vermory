-- +goose Up
ALTER TABLE observations
  ADD COLUMN memory_key TEXT NOT NULL DEFAULT '';

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

ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_lifecycle_status_check;

ALTER TABLE governed_memories
  ADD CONSTRAINT governed_memories_lifecycle_status_check
  CHECK (lifecycle_status IN ('proposed', 'active', 'superseded', 'rejected', 'deleted'));

-- +goose Down
UPDATE governed_memories
SET lifecycle_status = 'proposed'
WHERE lifecycle_status = 'rejected';

ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_lifecycle_status_check;

ALTER TABLE governed_memories
  ADD CONSTRAINT governed_memories_lifecycle_status_check
  CHECK (lifecycle_status IN ('proposed', 'active', 'superseded', 'deleted'));

UPDATE observations
SET observation_kind = CASE observation_kind
  WHEN 'source_candidate' THEN 'source_update'
  WHEN 'candidate_rejection' THEN 'user_confirmation'
  ELSE observation_kind
END
WHERE observation_kind IN ('source_candidate', 'candidate_rejection');

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
    'bridge_promote'
  ));

ALTER TABLE observations
  DROP COLUMN memory_key;
