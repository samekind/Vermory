-- +goose Up

CREATE TABLE memory_retrieval_profiles (
  profile_id TEXT PRIMARY KEY CHECK (btrim(profile_id) <> ''),
  provider_base_url TEXT NOT NULL CHECK (provider_base_url ~ '^https://[^/].*'),
  model TEXT NOT NULL CHECK (btrim(model) <> ''),
  dimensions INTEGER NOT NULL CHECK (dimensions = 1024),
  lifecycle_status TEXT NOT NULL CHECK (lifecycle_status IN ('candidate', 'active', 'retired')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_at TIMESTAMPTZ
);

INSERT INTO memory_retrieval_profiles (
  profile_id, provider_base_url, model, dimensions, lifecycle_status, activated_at
) VALUES
  ('siliconflow-bge-m3-1024-v1', 'https://api.siliconflow.cn/v1', 'BAAI/bge-m3', 1024, 'active', now()),
  ('siliconflow-bge-large-zh-1024-v2', 'https://api.siliconflow.cn/v1', 'BAAI/bge-large-zh-v1.5', 1024, 'candidate', NULL);

ALTER TABLE memory_projection_cursors
  DROP CONSTRAINT IF EXISTS memory_projection_cursors_profile_id_check;
ALTER TABLE memory_projection_cursors
  ADD CONSTRAINT memory_projection_cursors_profile_fk
  FOREIGN KEY (profile_id) REFERENCES memory_retrieval_profiles (profile_id);

ALTER TABLE memory_vector_documents
  DROP CONSTRAINT IF EXISTS memory_vector_documents_profile_id_check;
ALTER TABLE memory_vector_documents
  ADD CONSTRAINT memory_vector_documents_profile_fk
  FOREIGN KEY (profile_id) REFERENCES memory_retrieval_profiles (profile_id);

ALTER TABLE memory_retrieval_runs
  DROP CONSTRAINT IF EXISTS memory_retrieval_runs_profile_id_check;
ALTER TABLE memory_retrieval_runs
  ADD CONSTRAINT memory_retrieval_runs_profile_fk
  FOREIGN KEY (profile_id) REFERENCES memory_retrieval_profiles (profile_id);

REVOKE ALL ON TABLE memory_retrieval_profiles FROM PUBLIC;

-- +goose Down

ALTER TABLE memory_projection_cursors
  DROP CONSTRAINT IF EXISTS memory_projection_cursors_profile_fk;
ALTER TABLE memory_projection_cursors
  ADD CONSTRAINT memory_projection_cursors_profile_id_check
  CHECK (profile_id = 'siliconflow-bge-m3-1024-v1');

ALTER TABLE memory_vector_documents
  DROP CONSTRAINT IF EXISTS memory_vector_documents_profile_fk;
ALTER TABLE memory_vector_documents
  ADD CONSTRAINT memory_vector_documents_profile_id_check
  CHECK (profile_id = 'siliconflow-bge-m3-1024-v1');

ALTER TABLE memory_retrieval_runs
  DROP CONSTRAINT IF EXISTS memory_retrieval_runs_profile_fk;
ALTER TABLE memory_retrieval_runs
  ADD CONSTRAINT memory_retrieval_runs_profile_id_check
  CHECK (profile_id = 'siliconflow-bge-m3-1024-v1');

DROP TABLE IF EXISTS memory_retrieval_profiles;
