-- +goose Up

INSERT INTO memory_retrieval_profiles (
  profile_id, provider_base_url, model, dimensions, projection_class,
  lifecycle_status, activated_at
) VALUES (
  'siliconflow-bge-m3-1024-chunked-mean-v2',
  'https://api.siliconflow.cn/v1',
  'BAAI/bge-m3',
  1024,
  'vector_1024',
  'candidate',
  NULL
);

-- +goose Down

DELETE FROM memory_retrieval_runs
WHERE profile_id = 'siliconflow-bge-m3-1024-chunked-mean-v2';
DELETE FROM memory_projection_cursors
WHERE profile_id = 'siliconflow-bge-m3-1024-chunked-mean-v2';
DELETE FROM memory_vector_documents
WHERE profile_id = 'siliconflow-bge-m3-1024-chunked-mean-v2';
DELETE FROM memory_retrieval_profiles
WHERE profile_id = 'siliconflow-bge-m3-1024-chunked-mean-v2';
