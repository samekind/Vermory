-- +goose Up
ALTER TABLE observations
  ADD CONSTRAINT observations_tenant_continuity_id_key
  UNIQUE (tenant_id, continuity_id, id);

ALTER TABLE governed_memories
  ADD CONSTRAINT governed_memories_tenant_continuity_id_key
  UNIQUE (tenant_id, continuity_id, id);

UPDATE source_match_decisions decision
SET status = 'failed',
    source_ref = '[redacted]',
    source_content = '[redacted]',
    candidate_set = '[]'::jsonb,
    candidate_set_fingerprint = encode(digest(convert_to('[]', 'UTF8'), 'sha256'), 'hex'),
    provider_output = '[redacted]',
    reason = 'invalid cross-continuity reference was removed during migration',
    failure_code = 'invalid_cross_continuity_reference',
    target_memory_id = NULL,
    observation_id = NULL,
    candidate_memory_id = NULL,
    completed_at = COALESCE(completed_at, now())
WHERE (
  decision.target_memory_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM governed_memories memory
    WHERE memory.tenant_id = decision.tenant_id
      AND memory.continuity_id = decision.continuity_id
      AND memory.id = decision.target_memory_id
  )
) OR (
  decision.observation_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM observations observation
    WHERE observation.tenant_id = decision.tenant_id
      AND observation.continuity_id = decision.continuity_id
      AND observation.id = decision.observation_id
  )
) OR (
  decision.candidate_memory_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM governed_memories memory
    WHERE memory.tenant_id = decision.tenant_id
      AND memory.continuity_id = decision.continuity_id
      AND memory.id = decision.candidate_memory_id
  )
);

ALTER TABLE source_match_decisions
  DROP CONSTRAINT source_match_decisions_tenant_target_memory_fk,
  DROP CONSTRAINT source_match_decisions_tenant_observation_fk,
  DROP CONSTRAINT source_match_decisions_tenant_candidate_memory_fk,
  ADD CONSTRAINT source_match_decisions_tenant_target_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, target_memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT source_match_decisions_tenant_observation_fk
    FOREIGN KEY (tenant_id, continuity_id, observation_id)
    REFERENCES observations (tenant_id, continuity_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT source_match_decisions_tenant_candidate_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, candidate_memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE source_match_decisions
  DROP CONSTRAINT source_match_decisions_tenant_target_memory_fk,
  DROP CONSTRAINT source_match_decisions_tenant_observation_fk,
  DROP CONSTRAINT source_match_decisions_tenant_candidate_memory_fk,
  ADD CONSTRAINT source_match_decisions_tenant_target_memory_fk
    FOREIGN KEY (tenant_id, target_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT source_match_decisions_tenant_observation_fk
    FOREIGN KEY (tenant_id, observation_id)
    REFERENCES observations (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT source_match_decisions_tenant_candidate_memory_fk
    FOREIGN KEY (tenant_id, candidate_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_tenant_continuity_id_key;

ALTER TABLE observations
  DROP CONSTRAINT observations_tenant_continuity_id_key;
