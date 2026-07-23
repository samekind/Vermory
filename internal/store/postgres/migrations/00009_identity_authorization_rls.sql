-- +goose Up
CREATE SCHEMA vermory_auth;
REVOKE ALL ON SCHEMA vermory_auth FROM PUBLIC;

CREATE TABLE vermory_auth.api_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  issue_operation_id TEXT NOT NULL CHECK (btrim(issue_operation_id) <> ''),
  request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
  public_id TEXT NOT NULL UNIQUE CHECK (public_id ~ '^[A-Za-z0-9_-]{8,64}$'),
  token_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  subject_id TEXT NOT NULL CHECK (btrim(subject_id) <> ''),
  role TEXT NOT NULL CHECK (role IN ('client', 'operator', 'owner')),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ,
  revoke_operation_id TEXT NOT NULL DEFAULT '',
  UNIQUE (tenant_id, issue_operation_id),
  CHECK (expires_at > created_at),
  CHECK (
    (status = 'active' AND revoked_at IS NULL AND revoke_operation_id = '') OR
    (status = 'revoked' AND revoked_at IS NOT NULL AND btrim(revoke_operation_id) <> '')
  )
);

REVOKE ALL ON TABLE vermory_auth.api_tokens FROM PUBLIC;

CREATE FUNCTION vermory_auth.authenticate_token(input_public_id TEXT, input_digest BYTEA)
RETURNS TABLE (
  token_id UUID,
  tenant_id TEXT,
  subject_id TEXT,
  role TEXT,
  expires_at TIMESTAMPTZ
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, vermory_auth
AS $$
  SELECT token.id, token.tenant_id, token.subject_id, token.role, token.expires_at
  FROM vermory_auth.api_tokens AS token
  WHERE token.public_id = input_public_id
    AND token.token_digest = input_digest
    AND token.status = 'active'
    AND token.expires_at > clock_timestamp()
$$;

REVOKE ALL ON FUNCTION vermory_auth.authenticate_token(TEXT, BYTEA) FROM PUBLIC;

ALTER TABLE continuity_spaces
  ADD CONSTRAINT continuity_spaces_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE observations
  ADD CONSTRAINT observations_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE governed_memories
  ADD CONSTRAINT governed_memories_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE memory_deliveries
  ADD CONSTRAINT memory_deliveries_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE bridge_operations
  ADD CONSTRAINT bridge_operations_tenant_id_id_key UNIQUE (tenant_id, id);

ALTER TABLE continuity_bindings
  DROP CONSTRAINT continuity_bindings_continuity_id_fkey,
  ADD CONSTRAINT continuity_bindings_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE conversation_bindings
  DROP CONSTRAINT conversation_bindings_continuity_id_fkey,
  ADD CONSTRAINT conversation_bindings_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE observations
  DROP CONSTRAINT observations_continuity_id_fkey,
  ADD CONSTRAINT observations_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_continuity_id_fkey,
  DROP CONSTRAINT governed_memories_origin_observation_id_fkey,
  DROP CONSTRAINT governed_memories_supersedes_memory_id_fkey,
  ADD CONSTRAINT governed_memories_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT governed_memories_tenant_origin_observation_fk
    FOREIGN KEY (tenant_id, origin_observation_id)
    REFERENCES observations (tenant_id, id)
    ON DELETE SET NULL (origin_observation_id) NOT VALID,
  ADD CONSTRAINT governed_memories_tenant_supersedes_memory_fk
    FOREIGN KEY (tenant_id, supersedes_memory_id)
    REFERENCES governed_memories (tenant_id, id) NOT VALID;
ALTER TABLE memory_deliveries
  DROP CONSTRAINT memory_deliveries_continuity_id_fkey,
  ADD CONSTRAINT memory_deliveries_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE memory_search_documents
  DROP CONSTRAINT memory_search_documents_memory_id_fkey,
  DROP CONSTRAINT memory_search_documents_continuity_id_fkey,
  ADD CONSTRAINT memory_search_documents_tenant_memory_fk
    FOREIGN KEY (tenant_id, memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT memory_search_documents_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE conversation_turns
  DROP CONSTRAINT conversation_turns_continuity_id_fkey,
  DROP CONSTRAINT conversation_turns_user_observation_id_fkey,
  DROP CONSTRAINT conversation_turns_delivery_id_fkey,
  DROP CONSTRAINT conversation_turns_assistant_observation_id_fkey,
  ADD CONSTRAINT conversation_turns_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT conversation_turns_tenant_user_observation_fk
    FOREIGN KEY (tenant_id, user_observation_id)
    REFERENCES observations (tenant_id, id) NOT VALID,
  ADD CONSTRAINT conversation_turns_tenant_delivery_fk
    FOREIGN KEY (tenant_id, delivery_id)
    REFERENCES memory_deliveries (tenant_id, id) NOT VALID,
  ADD CONSTRAINT conversation_turns_tenant_assistant_observation_fk
    FOREIGN KEY (tenant_id, assistant_observation_id)
    REFERENCES observations (tenant_id, id) NOT VALID;
ALTER TABLE bridge_operations
  DROP CONSTRAINT bridge_operations_source_continuity_id_fkey,
  DROP CONSTRAINT bridge_operations_target_continuity_id_fkey,
  ADD CONSTRAINT bridge_operations_tenant_source_continuity_fk
    FOREIGN KEY (tenant_id, source_continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE RESTRICT NOT VALID,
  ADD CONSTRAINT bridge_operations_tenant_target_continuity_fk
    FOREIGN KEY (tenant_id, target_continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE bridge_events
  DROP CONSTRAINT bridge_events_bridge_id_fkey,
  ADD CONSTRAINT bridge_events_tenant_bridge_fk
    FOREIGN KEY (tenant_id, bridge_id)
    REFERENCES bridge_operations (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE bridge_memory_effects
  DROP CONSTRAINT bridge_memory_effects_bridge_id_fkey,
  DROP CONSTRAINT bridge_memory_effects_source_memory_id_fkey,
  DROP CONSTRAINT bridge_memory_effects_target_memory_id_fkey,
  ADD CONSTRAINT bridge_memory_effects_tenant_bridge_fk
    FOREIGN KEY (tenant_id, bridge_id)
    REFERENCES bridge_operations (tenant_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT bridge_memory_effects_tenant_source_memory_fk
    FOREIGN KEY (tenant_id, source_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT NOT VALID,
  ADD CONSTRAINT bridge_memory_effects_tenant_target_memory_fk
    FOREIGN KEY (tenant_id, target_memory_id)
    REFERENCES governed_memories (tenant_id, id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE conversation_links
  DROP CONSTRAINT conversation_links_bridge_id_fkey,
  DROP CONSTRAINT conversation_links_primary_continuity_id_fkey,
  DROP CONSTRAINT conversation_links_linked_continuity_id_fkey,
  ADD CONSTRAINT conversation_links_tenant_bridge_fk
    FOREIGN KEY (tenant_id, bridge_id)
    REFERENCES bridge_operations (tenant_id, id) ON DELETE CASCADE NOT VALID,
  ADD CONSTRAINT conversation_links_tenant_primary_continuity_fk
    FOREIGN KEY (tenant_id, primary_continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE RESTRICT NOT VALID,
  ADD CONSTRAINT conversation_links_tenant_linked_continuity_fk
    FOREIGN KEY (tenant_id, linked_continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE continuity_bindings VALIDATE CONSTRAINT continuity_bindings_tenant_continuity_fk;
ALTER TABLE conversation_bindings VALIDATE CONSTRAINT conversation_bindings_tenant_continuity_fk;
ALTER TABLE observations VALIDATE CONSTRAINT observations_tenant_continuity_fk;
ALTER TABLE governed_memories VALIDATE CONSTRAINT governed_memories_tenant_continuity_fk;
ALTER TABLE governed_memories VALIDATE CONSTRAINT governed_memories_tenant_origin_observation_fk;
ALTER TABLE governed_memories VALIDATE CONSTRAINT governed_memories_tenant_supersedes_memory_fk;
ALTER TABLE memory_deliveries VALIDATE CONSTRAINT memory_deliveries_tenant_continuity_fk;
ALTER TABLE memory_search_documents VALIDATE CONSTRAINT memory_search_documents_tenant_memory_fk;
ALTER TABLE memory_search_documents VALIDATE CONSTRAINT memory_search_documents_tenant_continuity_fk;
ALTER TABLE conversation_turns VALIDATE CONSTRAINT conversation_turns_tenant_continuity_fk;
ALTER TABLE conversation_turns VALIDATE CONSTRAINT conversation_turns_tenant_user_observation_fk;
ALTER TABLE conversation_turns VALIDATE CONSTRAINT conversation_turns_tenant_delivery_fk;
ALTER TABLE conversation_turns VALIDATE CONSTRAINT conversation_turns_tenant_assistant_observation_fk;
ALTER TABLE bridge_operations VALIDATE CONSTRAINT bridge_operations_tenant_source_continuity_fk;
ALTER TABLE bridge_operations VALIDATE CONSTRAINT bridge_operations_tenant_target_continuity_fk;
ALTER TABLE bridge_events VALIDATE CONSTRAINT bridge_events_tenant_bridge_fk;
ALTER TABLE bridge_memory_effects VALIDATE CONSTRAINT bridge_memory_effects_tenant_bridge_fk;
ALTER TABLE bridge_memory_effects VALIDATE CONSTRAINT bridge_memory_effects_tenant_source_memory_fk;
ALTER TABLE bridge_memory_effects VALIDATE CONSTRAINT bridge_memory_effects_tenant_target_memory_fk;
ALTER TABLE conversation_links VALIDATE CONSTRAINT conversation_links_tenant_bridge_fk;
ALTER TABLE conversation_links VALIDATE CONSTRAINT conversation_links_tenant_primary_continuity_fk;
ALTER TABLE conversation_links VALIDATE CONSTRAINT conversation_links_tenant_linked_continuity_fk;

ALTER TABLE continuity_spaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE continuity_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE governed_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_search_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_turns ENABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_memory_effects ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_links ENABLE ROW LEVEL SECURITY;

CREATE POLICY continuity_spaces_tenant_isolation ON continuity_spaces
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY continuity_bindings_tenant_isolation ON continuity_bindings
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY conversation_bindings_tenant_isolation ON conversation_bindings
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY observations_tenant_isolation ON observations
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY governed_memories_tenant_isolation ON governed_memories
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY memory_deliveries_tenant_isolation ON memory_deliveries
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY memory_search_documents_tenant_isolation ON memory_search_documents
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY conversation_turns_tenant_isolation ON conversation_turns
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY bridge_operations_tenant_isolation ON bridge_operations
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY bridge_events_tenant_isolation ON bridge_events
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY bridge_memory_effects_tenant_isolation ON bridge_memory_effects
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY conversation_links_tenant_isolation ON conversation_links
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS conversation_links_tenant_isolation ON conversation_links;
DROP POLICY IF EXISTS bridge_memory_effects_tenant_isolation ON bridge_memory_effects;
DROP POLICY IF EXISTS bridge_events_tenant_isolation ON bridge_events;
DROP POLICY IF EXISTS bridge_operations_tenant_isolation ON bridge_operations;
DROP POLICY IF EXISTS conversation_turns_tenant_isolation ON conversation_turns;
DROP POLICY IF EXISTS memory_search_documents_tenant_isolation ON memory_search_documents;
DROP POLICY IF EXISTS memory_deliveries_tenant_isolation ON memory_deliveries;
DROP POLICY IF EXISTS governed_memories_tenant_isolation ON governed_memories;
DROP POLICY IF EXISTS observations_tenant_isolation ON observations;
DROP POLICY IF EXISTS conversation_bindings_tenant_isolation ON conversation_bindings;
DROP POLICY IF EXISTS continuity_bindings_tenant_isolation ON continuity_bindings;
DROP POLICY IF EXISTS continuity_spaces_tenant_isolation ON continuity_spaces;

ALTER TABLE conversation_links DISABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_memory_effects DISABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE bridge_operations DISABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_turns DISABLE ROW LEVEL SECURITY;
ALTER TABLE memory_search_documents DISABLE ROW LEVEL SECURITY;
ALTER TABLE memory_deliveries DISABLE ROW LEVEL SECURITY;
ALTER TABLE governed_memories DISABLE ROW LEVEL SECURITY;
ALTER TABLE observations DISABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_bindings DISABLE ROW LEVEL SECURITY;
ALTER TABLE continuity_bindings DISABLE ROW LEVEL SECURITY;
ALTER TABLE continuity_spaces DISABLE ROW LEVEL SECURITY;

ALTER TABLE continuity_bindings
  DROP CONSTRAINT continuity_bindings_tenant_continuity_fk,
  ADD CONSTRAINT continuity_bindings_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE;
ALTER TABLE conversation_bindings
  DROP CONSTRAINT conversation_bindings_tenant_continuity_fk,
  ADD CONSTRAINT conversation_bindings_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE;
ALTER TABLE observations
  DROP CONSTRAINT observations_tenant_continuity_fk,
  ADD CONSTRAINT observations_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE;
ALTER TABLE governed_memories
  DROP CONSTRAINT governed_memories_tenant_continuity_fk,
  DROP CONSTRAINT governed_memories_tenant_origin_observation_fk,
  DROP CONSTRAINT governed_memories_tenant_supersedes_memory_fk,
  ADD CONSTRAINT governed_memories_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE,
  ADD CONSTRAINT governed_memories_origin_observation_id_fkey
    FOREIGN KEY (origin_observation_id) REFERENCES observations (id) ON DELETE SET NULL,
  ADD CONSTRAINT governed_memories_supersedes_memory_id_fkey
    FOREIGN KEY (supersedes_memory_id) REFERENCES governed_memories (id);
ALTER TABLE memory_deliveries
  DROP CONSTRAINT memory_deliveries_tenant_continuity_fk,
  ADD CONSTRAINT memory_deliveries_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE;
ALTER TABLE memory_search_documents
  DROP CONSTRAINT memory_search_documents_tenant_memory_fk,
  DROP CONSTRAINT memory_search_documents_tenant_continuity_fk,
  ADD CONSTRAINT memory_search_documents_memory_id_fkey
    FOREIGN KEY (memory_id) REFERENCES governed_memories (id) ON DELETE CASCADE,
  ADD CONSTRAINT memory_search_documents_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE;
ALTER TABLE conversation_turns
  DROP CONSTRAINT conversation_turns_tenant_continuity_fk,
  DROP CONSTRAINT conversation_turns_tenant_user_observation_fk,
  DROP CONSTRAINT conversation_turns_tenant_delivery_fk,
  DROP CONSTRAINT conversation_turns_tenant_assistant_observation_fk,
  ADD CONSTRAINT conversation_turns_continuity_id_fkey
    FOREIGN KEY (continuity_id) REFERENCES continuity_spaces (id) ON DELETE CASCADE,
  ADD CONSTRAINT conversation_turns_user_observation_id_fkey
    FOREIGN KEY (user_observation_id) REFERENCES observations (id),
  ADD CONSTRAINT conversation_turns_delivery_id_fkey
    FOREIGN KEY (delivery_id) REFERENCES memory_deliveries (id),
  ADD CONSTRAINT conversation_turns_assistant_observation_id_fkey
    FOREIGN KEY (assistant_observation_id) REFERENCES observations (id);
ALTER TABLE bridge_operations
  DROP CONSTRAINT bridge_operations_tenant_source_continuity_fk,
  DROP CONSTRAINT bridge_operations_tenant_target_continuity_fk,
  ADD CONSTRAINT bridge_operations_source_continuity_id_fkey
    FOREIGN KEY (source_continuity_id) REFERENCES continuity_spaces (id) ON DELETE RESTRICT,
  ADD CONSTRAINT bridge_operations_target_continuity_id_fkey
    FOREIGN KEY (target_continuity_id) REFERENCES continuity_spaces (id) ON DELETE RESTRICT;
ALTER TABLE bridge_events
  DROP CONSTRAINT bridge_events_tenant_bridge_fk,
  ADD CONSTRAINT bridge_events_bridge_id_fkey
    FOREIGN KEY (bridge_id) REFERENCES bridge_operations (id) ON DELETE CASCADE;
ALTER TABLE bridge_memory_effects
  DROP CONSTRAINT bridge_memory_effects_tenant_bridge_fk,
  DROP CONSTRAINT bridge_memory_effects_tenant_source_memory_fk,
  DROP CONSTRAINT bridge_memory_effects_tenant_target_memory_fk,
  ADD CONSTRAINT bridge_memory_effects_bridge_id_fkey
    FOREIGN KEY (bridge_id) REFERENCES bridge_operations (id) ON DELETE CASCADE,
  ADD CONSTRAINT bridge_memory_effects_source_memory_id_fkey
    FOREIGN KEY (source_memory_id) REFERENCES governed_memories (id) ON DELETE RESTRICT,
  ADD CONSTRAINT bridge_memory_effects_target_memory_id_fkey
    FOREIGN KEY (target_memory_id) REFERENCES governed_memories (id) ON DELETE RESTRICT;
ALTER TABLE conversation_links
  DROP CONSTRAINT conversation_links_tenant_bridge_fk,
  DROP CONSTRAINT conversation_links_tenant_primary_continuity_fk,
  DROP CONSTRAINT conversation_links_tenant_linked_continuity_fk,
  ADD CONSTRAINT conversation_links_bridge_id_fkey
    FOREIGN KEY (bridge_id) REFERENCES bridge_operations (id) ON DELETE CASCADE,
  ADD CONSTRAINT conversation_links_primary_continuity_id_fkey
    FOREIGN KEY (primary_continuity_id) REFERENCES continuity_spaces (id) ON DELETE RESTRICT,
  ADD CONSTRAINT conversation_links_linked_continuity_id_fkey
    FOREIGN KEY (linked_continuity_id) REFERENCES continuity_spaces (id) ON DELETE RESTRICT;

ALTER TABLE bridge_operations DROP CONSTRAINT bridge_operations_tenant_id_id_key;
ALTER TABLE memory_deliveries DROP CONSTRAINT memory_deliveries_tenant_id_id_key;
ALTER TABLE governed_memories DROP CONSTRAINT governed_memories_tenant_id_id_key;
ALTER TABLE observations DROP CONSTRAINT observations_tenant_id_id_key;
ALTER TABLE continuity_spaces DROP CONSTRAINT continuity_spaces_tenant_id_id_key;

DROP FUNCTION vermory_auth.authenticate_token(TEXT, BYTEA);
DROP TABLE vermory_auth.api_tokens;
DROP SCHEMA vermory_auth;
