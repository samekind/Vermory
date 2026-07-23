-- +goose Up
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

CREATE TABLE bridge_operations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('promote', 'link', 'export', 'adopt', 'rebind')),
  status TEXT NOT NULL CHECK (status IN ('active', 'reversed', 'revoked')),
  source_continuity_id UUID REFERENCES continuity_spaces(id) ON DELETE RESTRICT,
  target_continuity_id UUID REFERENCES continuity_spaces(id) ON DELETE RESTRICT,
  source_anchor TEXT NOT NULL DEFAULT '',
  target_anchor TEXT NOT NULL DEFAULT '',
  target_profile TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  export_body TEXT NOT NULL DEFAULT '',
  request_fingerprint TEXT NOT NULL,
  reverse_operation_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reversed_at TIMESTAMPTZ,
  UNIQUE (tenant_id, operation_id)
);

CREATE UNIQUE INDEX bridge_operations_reverse_operation_idx
  ON bridge_operations (tenant_id, reverse_operation_id)
  WHERE reverse_operation_id <> '';

CREATE INDEX bridge_operations_scope_idx
  ON bridge_operations (tenant_id, action, status, created_at);

CREATE TABLE bridge_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  bridge_id UUID NOT NULL REFERENCES bridge_operations(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('created', 'reversed', 'revoked')),
  operation_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);

CREATE INDEX bridge_events_bridge_idx
  ON bridge_events (tenant_id, bridge_id, created_at);

CREATE TABLE bridge_memory_effects (
  bridge_id UUID NOT NULL REFERENCES bridge_operations(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  effect_kind TEXT NOT NULL CHECK (effect_kind IN ('promote', 'export')),
  source_memory_id UUID NOT NULL REFERENCES governed_memories(id) ON DELETE RESTRICT,
  target_memory_id UUID REFERENCES governed_memories(id) ON DELETE RESTRICT,
  order_index INT NOT NULL,
  PRIMARY KEY (bridge_id, source_memory_id)
);

CREATE INDEX bridge_memory_effects_target_idx
  ON bridge_memory_effects (tenant_id, target_memory_id)
  WHERE target_memory_id IS NOT NULL;

CREATE TABLE conversation_links (
  bridge_id UUID PRIMARY KEY REFERENCES bridge_operations(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  primary_continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE RESTRICT,
  linked_continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE RESTRICT,
  link_state TEXT NOT NULL CHECK (link_state IN ('active', 'reversed')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (primary_continuity_id <> linked_continuity_id)
);

CREATE UNIQUE INDEX conversation_links_active_linked_idx
  ON conversation_links (tenant_id, linked_continuity_id)
  WHERE link_state = 'active';

CREATE UNIQUE INDEX conversation_links_active_pair_idx
  ON conversation_links (tenant_id, primary_continuity_id, linked_continuity_id)
  WHERE link_state = 'active';

CREATE INDEX conversation_links_primary_idx
  ON conversation_links (tenant_id, primary_continuity_id, link_state);

-- +goose Down
DROP TABLE IF EXISTS conversation_links;
DROP TABLE IF EXISTS bridge_memory_effects;
DROP TABLE IF EXISTS bridge_events;
DROP TABLE IF EXISTS bridge_operations;

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
