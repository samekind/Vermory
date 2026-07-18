-- +goose Up
ALTER TABLE continuity_bindings
  ADD COLUMN filesystem_namespace TEXT NOT NULL DEFAULT '';

DROP INDEX continuity_bindings_lookup_idx;
DROP INDEX continuity_bindings_confirmed_anchor_idx;

CREATE INDEX continuity_bindings_lookup_idx
  ON continuity_bindings (tenant_id, filesystem_namespace, repo_root, binding_state);

CREATE UNIQUE INDEX continuity_bindings_confirmed_anchor_idx
  ON continuity_bindings (tenant_id, filesystem_namespace, repo_root)
  WHERE binding_state = 'confirmed';

ALTER TABLE bridge_operations
  ADD COLUMN source_filesystem_namespace TEXT NOT NULL DEFAULT '',
  ADD COLUMN target_filesystem_namespace TEXT NOT NULL DEFAULT '';

-- +goose Down
DROP INDEX continuity_bindings_lookup_idx;
DROP INDEX continuity_bindings_confirmed_anchor_idx;

CREATE INDEX continuity_bindings_lookup_idx
  ON continuity_bindings (tenant_id, repo_root, binding_state);

CREATE UNIQUE INDEX continuity_bindings_confirmed_anchor_idx
  ON continuity_bindings (tenant_id, repo_root)
  WHERE binding_state = 'confirmed';

ALTER TABLE continuity_bindings
  DROP COLUMN filesystem_namespace;

ALTER TABLE bridge_operations
  DROP COLUMN target_filesystem_namespace,
  DROP COLUMN source_filesystem_namespace;
