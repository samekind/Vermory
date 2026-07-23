-- +goose Up
ALTER TABLE source_formation_runs
  ADD COLUMN input_kind TEXT NOT NULL DEFAULT 'document',
  ADD COLUMN input_manifest JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN input_manifest_fingerprint TEXT NOT NULL
    DEFAULT '4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945';

ALTER TABLE source_formation_runs
  ADD CONSTRAINT source_formation_runs_input_kind_check
    CHECK (input_kind IN ('document', 'conversation')),
  ADD CONSTRAINT source_formation_runs_input_manifest_check
    CHECK (jsonb_typeof(input_manifest) = 'array'),
  ADD CONSTRAINT source_formation_runs_input_manifest_fingerprint_check
    CHECK (length(input_manifest_fingerprint) = 64),
  ADD CONSTRAINT source_formation_runs_input_shape_check
    CHECK (
      (input_kind = 'document' AND input_manifest = '[]'::jsonb)
      OR
      (input_kind = 'conversation' AND jsonb_array_length(input_manifest) > 0)
    );

ALTER TABLE source_formation_items
  ADD COLUMN evidence_observation_id UUID;

ALTER TABLE source_formation_items
  ADD CONSTRAINT source_formation_items_tenant_evidence_observation_fk
    FOREIGN KEY (tenant_id, continuity_id, evidence_observation_id)
    REFERENCES observations (tenant_id, continuity_id, id) ON DELETE RESTRICT;

CREATE INDEX source_formation_items_evidence_idx
  ON source_formation_items (tenant_id, continuity_id, evidence_observation_id)
  WHERE evidence_observation_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS source_formation_items_evidence_idx;

ALTER TABLE source_formation_items
  DROP CONSTRAINT IF EXISTS source_formation_items_tenant_evidence_observation_fk,
  DROP COLUMN IF EXISTS evidence_observation_id;

ALTER TABLE source_formation_runs
  DROP CONSTRAINT IF EXISTS source_formation_runs_input_shape_check,
  DROP CONSTRAINT IF EXISTS source_formation_runs_input_manifest_fingerprint_check,
  DROP CONSTRAINT IF EXISTS source_formation_runs_input_manifest_check,
  DROP CONSTRAINT IF EXISTS source_formation_runs_input_kind_check,
  DROP COLUMN IF EXISTS input_manifest_fingerprint,
  DROP COLUMN IF EXISTS input_manifest,
  DROP COLUMN IF EXISTS input_kind;
