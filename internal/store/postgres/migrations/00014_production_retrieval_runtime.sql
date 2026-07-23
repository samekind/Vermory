-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE memory_projection_events (
  event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  desired_state TEXT NOT NULL CHECK (desired_state IN ('active', 'absent')),
  authority_version TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_projection_events_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT memory_projection_events_tenant_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE CASCADE
);

CREATE TABLE memory_projection_cursors (
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  profile_id TEXT NOT NULL CHECK (profile_id = 'siliconflow-bge-m3-1024-v1'),
  last_event_id BIGINT NOT NULL DEFAULT 0 CHECK (last_event_id >= 0),
  status TEXT NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'running', 'failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  last_error_code TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error_code) <= 64),
  last_attempt_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, profile_id)
);

CREATE TABLE memory_vector_documents (
  profile_id TEXT NOT NULL CHECK (profile_id = 'siliconflow-bge-m3-1024-v1'),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  continuity_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  content_sha256 TEXT NOT NULL CHECK (length(content_sha256) = 64),
  embedding vector(1024) NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (profile_id, tenant_id, memory_id),
  CONSTRAINT memory_vector_documents_tenant_continuity_fk
    FOREIGN KEY (tenant_id, continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT memory_vector_documents_tenant_memory_fk
    FOREIGN KEY (tenant_id, continuity_id, memory_id)
    REFERENCES governed_memories (tenant_id, continuity_id, id) ON DELETE CASCADE
);

CREATE TABLE memory_retrieval_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  primary_continuity_id UUID NOT NULL,
  continuity_ids UUID[] NOT NULL CHECK (cardinality(continuity_ids) > 0),
  operation_id TEXT NOT NULL CHECK (btrim(operation_id) <> ''),
  request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) = 64),
  requested_mode TEXT NOT NULL CHECK (requested_mode IN ('lexical', 'shadow', 'vector')),
  effective_mode TEXT NOT NULL CHECK (effective_mode IN ('lexical', 'shadow', 'vector')),
  profile_id TEXT NOT NULL CHECK (profile_id = 'siliconflow-bge-m3-1024-v1'),
  query_sha256 TEXT NOT NULL CHECK (length(query_sha256) = 64),
  lexical_memory_ids UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
  vector_memory_ids UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
  delivered_memory_ids UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
  projection_current BOOLEAN NOT NULL,
  degraded BOOLEAN NOT NULL,
  failure_code TEXT NOT NULL DEFAULT '' CHECK (octet_length(failure_code) <= 64),
  lexical_latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (lexical_latency_ms >= 0),
  vector_latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (vector_latency_ms >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id),
  CONSTRAINT memory_retrieval_runs_tenant_primary_continuity_fk
    FOREIGN KEY (tenant_id, primary_continuity_id)
    REFERENCES continuity_spaces (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX memory_projection_events_tenant_event_idx
  ON memory_projection_events (tenant_id, event_id);
CREATE INDEX memory_projection_events_memory_idx
  ON memory_projection_events (tenant_id, memory_id, event_id DESC);
CREATE INDEX memory_vector_documents_scope_idx
  ON memory_vector_documents (profile_id, tenant_id, continuity_id, memory_id);
CREATE INDEX memory_vector_documents_embedding_hnsw_idx
  ON memory_vector_documents USING hnsw (embedding vector_cosine_ops);
CREATE INDEX memory_retrieval_runs_scope_idx
  ON memory_retrieval_runs (tenant_id, primary_continuity_id, created_at DESC);

-- +goose StatementBegin
CREATE FUNCTION enqueue_memory_projection_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND NEW.tenant_id IS NOT DISTINCT FROM OLD.tenant_id
     AND NEW.continuity_id IS NOT DISTINCT FROM OLD.continuity_id
     AND NEW.memory_kind IS NOT DISTINCT FROM OLD.memory_kind
     AND NEW.lifecycle_status IS NOT DISTINCT FROM OLD.lifecycle_status
     AND NEW.content IS NOT DISTINCT FROM OLD.content THEN
    RETURN NEW;
  END IF;

  INSERT INTO memory_projection_events (
    tenant_id, continuity_id, memory_id, desired_state, authority_version
  ) VALUES (
    NEW.tenant_id,
    NEW.continuity_id,
    NEW.id,
    CASE
      WHEN NEW.memory_kind = 'fact'
       AND NEW.lifecycle_status = 'active'
       AND NEW.content <> '[redacted]'
      THEN 'active'
      ELSE 'absent'
    END,
    NEW.updated_at
  );
  RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER enqueue_memory_projection_event
AFTER INSERT OR UPDATE OF tenant_id, continuity_id, memory_kind, lifecycle_status, content
ON governed_memories
FOR EACH ROW
EXECUTE FUNCTION enqueue_memory_projection_event();

INSERT INTO memory_projection_events (
  tenant_id, continuity_id, memory_id, desired_state, authority_version, created_at
)
SELECT
  tenant_id,
  continuity_id,
  id,
  CASE
    WHEN memory_kind = 'fact'
     AND lifecycle_status = 'active'
     AND content <> '[redacted]'
    THEN 'active'
    ELSE 'absent'
  END,
  updated_at,
  now()
FROM governed_memories
ORDER BY created_at, id;

ALTER TABLE memory_projection_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_projection_cursors ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_vector_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_retrieval_runs ENABLE ROW LEVEL SECURITY;

CREATE POLICY memory_projection_events_tenant_isolation ON memory_projection_events
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY memory_projection_cursors_tenant_isolation ON memory_projection_cursors
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY memory_vector_documents_tenant_isolation ON memory_vector_documents
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));
CREATE POLICY memory_retrieval_runs_tenant_isolation ON memory_retrieval_runs
  USING (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''))
  WITH CHECK (tenant_id = NULLIF(current_setting('vermory.tenant_id', true), ''));

-- +goose Down
DROP POLICY IF EXISTS memory_retrieval_runs_tenant_isolation ON memory_retrieval_runs;
DROP POLICY IF EXISTS memory_vector_documents_tenant_isolation ON memory_vector_documents;
DROP POLICY IF EXISTS memory_projection_cursors_tenant_isolation ON memory_projection_cursors;
DROP POLICY IF EXISTS memory_projection_events_tenant_isolation ON memory_projection_events;
DROP TRIGGER IF EXISTS enqueue_memory_projection_event ON governed_memories;
DROP FUNCTION IF EXISTS enqueue_memory_projection_event();
DROP TABLE IF EXISTS memory_retrieval_runs;
DROP TABLE IF EXISTS memory_vector_documents;
DROP TABLE IF EXISTS memory_projection_cursors;
DROP TABLE IF EXISTS memory_projection_events;
