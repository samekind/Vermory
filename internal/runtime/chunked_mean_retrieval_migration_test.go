package runtime

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestChunkedMeanRetrievalProfileMigrationIsCandidate(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	var baseURL, model, projectionClass, lifecycle string
	var dimensions int
	if err := store.pool.QueryRow(ctx, `
SELECT provider_base_url, model, dimensions, projection_class, lifecycle_status
FROM memory_retrieval_profiles
WHERE profile_id = $1`, ChunkedMeanRetrievalProfileID).Scan(
		&baseURL, &model, &dimensions, &projectionClass, &lifecycle,
	); err != nil {
		t.Fatal(err)
	}
	if baseURL != "https://api.siliconflow.cn/v1" || model != "BAAI/bge-m3" ||
		dimensions != 1024 || projectionClass != string(ProjectionClass1024) || lifecycle != "candidate" {
		t.Fatalf("unexpected chunked profile row: base=%q model=%q dimensions=%d class=%q lifecycle=%q",
			baseURL, model, dimensions, projectionClass, lifecycle)
	}

	var activeCount, vectorPolicyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_retrieval_profiles
WHERE profile_id = $1 AND lifecycle_status = 'active'`, ProductionRetrievalProfileID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM pg_policies
WHERE schemaname = 'public' AND tablename = 'memory_vector_documents'
  AND qual LIKE '%vermory.tenant_id%' AND with_check LIKE '%vermory.tenant_id%'`).Scan(&vectorPolicyCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 || vectorPolicyCount != 1 {
		t.Fatalf("candidate changed production activation or tenant isolation: active=%d policies=%d", activeCount, vectorPolicyCount)
	}
}

func TestChunkedMeanRetrievalProfileMigrationUpDown(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ('chunked-migration-down', $1, 0, 'idle')`, ChunkedMeanRetrievalProfileID); err != nil {
		t.Fatal(err)
	}

	db := openProjectionRetentionMigrationDB(t)
	if err := goose.DownToContext(ctx, db, "migrations", 22); err != nil {
		t.Fatal(err)
	}
	var profileCount, cursorCount, productionCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_retrieval_profiles WHERE profile_id = $1`, ChunkedMeanRetrievalProfileID).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_projection_cursors WHERE tenant_id = 'chunked-migration-down'`).Scan(&cursorCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_retrieval_profiles WHERE profile_id = $1 AND lifecycle_status = 'active'`, ProductionRetrievalProfileID).Scan(&productionCount); err != nil {
		t.Fatal(err)
	}
	if profileCount != 0 || cursorCount != 0 || productionCount != 1 {
		t.Fatalf("chunked profile downgrade left dependencies or changed production: profile=%d cursor=%d production=%d", profileCount, cursorCount, productionCount)
	}
	if err := goose.UpToContext(ctx, db, "migrations", MaximumSupportedSchemaVersion); err != nil {
		t.Fatal(err)
	}
}
