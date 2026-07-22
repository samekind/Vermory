package runtime

import (
	"context"
	"testing"
)

func TestRetrievalDimensionMigrationSchema(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := store.pool.QueryRow(ctx, `
SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != MaximumSupportedSchemaVersion {
		t.Fatalf("schema version=%d want %d", version, MaximumSupportedSchemaVersion)
	}

	var model string
	var dimensions int
	var projectionClass, lifecycle string
	if err := store.pool.QueryRow(ctx, `
SELECT model, dimensions, projection_class, lifecycle_status
FROM memory_retrieval_profiles
WHERE profile_id = 'siliconflow-qwen3-embedding-4b-2560-v3'`).Scan(
		&model, &dimensions, &projectionClass, &lifecycle,
	); err != nil {
		t.Fatal(err)
	}
	if model != "Qwen/Qwen3-Embedding-4B" || dimensions != 2560 ||
		projectionClass != "halfvec_2560" || lifecycle != "candidate" {
		t.Fatalf("unexpected candidate profile: model=%s dimensions=%d class=%s lifecycle=%s", model, dimensions, projectionClass, lifecycle)
	}

	var embeddingType string
	if err := store.pool.QueryRow(ctx, `
SELECT format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
WHERE a.attrelid = 'public.memory_vector_documents_2560'::regclass
  AND a.attname = 'embedding'`).Scan(&embeddingType); err != nil {
		t.Fatal(err)
	}
	if embeddingType != "halfvec(2560)" {
		t.Fatalf("candidate embedding type=%q want halfvec(2560)", embeddingType)
	}

	var policyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_policies
WHERE schemaname = 'public'
  AND tablename = 'memory_vector_documents_2560'`).Scan(&policyCount); err != nil {
		t.Fatal(err)
	}
	if policyCount != 1 {
		t.Fatalf("candidate RLS policy count=%d want 1", policyCount)
	}

	var indexCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_indexes
WHERE schemaname = 'public'
  AND tablename = 'memory_vector_documents_2560'
  AND indexdef LIKE '%hnsw%halfvec_cosine_ops%'`).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("candidate HNSW index count=%d want 1", indexCount)
	}

	var classConstraintCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_constraint
WHERE conrelid = 'public.memory_vector_documents_2560'::regclass
  AND pg_get_constraintdef(oid) LIKE '%halfvec_2560%'`).Scan(&classConstraintCount); err != nil {
		t.Fatal(err)
	}
	if classConstraintCount < 1 {
		t.Fatalf("candidate projection class constraint missing")
	}
}
