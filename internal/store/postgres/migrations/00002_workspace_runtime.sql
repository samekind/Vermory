-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE continuity_spaces (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_line TEXT NOT NULL CHECK (continuity_line = 'workspace'),
  state TEXT NOT NULL CHECK (state IN ('active', 'needs_confirmation')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE continuity_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  repo_root TEXT NOT NULL,
  binding_state TEXT NOT NULL CHECK (binding_state IN ('confirmed', 'ambiguous', 'retired')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX continuity_bindings_lookup_idx
  ON continuity_bindings (tenant_id, repo_root, binding_state);

CREATE UNIQUE INDEX continuity_bindings_confirmed_anchor_idx
  ON continuity_bindings (tenant_id, repo_root)
  WHERE binding_state = 'confirmed';

CREATE TABLE observations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  operation_id TEXT NOT NULL,
  observation_kind TEXT NOT NULL CHECK (observation_kind IN ('agent_result', 'user_correction', 'source_update', 'forget_request')),
  content TEXT NOT NULL,
  source_ref TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);

CREATE TABLE governed_memories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  origin_observation_id UUID REFERENCES observations(id) ON DELETE SET NULL,
  memory_kind TEXT NOT NULL,
  lifecycle_status TEXT NOT NULL CHECK (lifecycle_status IN ('proposed', 'active', 'superseded', 'deleted')),
  content TEXT NOT NULL,
  supersedes_memory_id UUID REFERENCES governed_memories(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX governed_memories_current_idx
  ON governed_memories (tenant_id, continuity_id, lifecycle_status);

CREATE TABLE memory_deliveries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  operation_id TEXT NOT NULL,
  task TEXT NOT NULL,
  context_body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);

CREATE TABLE memory_search_documents (
  memory_id UUID PRIMARY KEY REFERENCES governed_memories(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  search_document TSVECTOR NOT NULL
);

CREATE INDEX memory_search_documents_scope_idx
  ON memory_search_documents (tenant_id, continuity_id);

CREATE INDEX memory_search_documents_tsv_idx
  ON memory_search_documents USING GIN (search_document);

CREATE INDEX memory_search_documents_trgm_idx
  ON memory_search_documents USING GIN (content gin_trgm_ops);

-- +goose Down
DROP TABLE IF EXISTS memory_search_documents;
DROP TABLE IF EXISTS memory_deliveries;
DROP TABLE IF EXISTS governed_memories;
DROP TABLE IF EXISTS observations;
DROP TABLE IF EXISTS continuity_bindings;
DROP TABLE IF EXISTS continuity_spaces;
