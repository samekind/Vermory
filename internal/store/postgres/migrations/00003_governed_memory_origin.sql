-- +goose Up
CREATE UNIQUE INDEX governed_memories_origin_observation_idx
  ON governed_memories (origin_observation_id)
  WHERE origin_observation_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS governed_memories_origin_observation_idx;
