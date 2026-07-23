package runtime

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	storepostgres "vermory/internal/store/postgres"

	"github.com/pressly/goose/v3"
)

func TestProductionRetrievalMigrationCreatesProjectionTables(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	for table, required := range map[string][]string{
		"memory_projection_events": {
			"event_id", "tenant_id", "continuity_id", "memory_id", "desired_state",
			"authority_version", "created_at",
		},
		"memory_projection_cursors": {
			"tenant_id", "profile_id", "last_event_id", "status", "attempt_count",
			"last_error_code", "last_attempt_at", "updated_at",
		},
		"memory_vector_documents": {
			"profile_id", "projection_class", "tenant_id", "continuity_id", "memory_id", "content_sha256",
			"embedding", "updated_at",
		},
		"memory_vector_documents_2560": {
			"profile_id", "projection_class", "tenant_id", "continuity_id", "memory_id", "content_sha256",
			"embedding", "updated_at",
		},
		"memory_retrieval_runs": {
			"id", "tenant_id", "primary_continuity_id", "continuity_ids", "operation_id",
			"request_fingerprint", "requested_mode", "effective_mode", "profile_id",
			"query_sha256", "lexical_memory_ids", "vector_memory_ids",
			"delivered_memory_ids", "projection_current", "degraded", "failure_code",
			"lexical_latency_ms", "vector_latency_ms", "created_at",
		},
	} {
		var exists bool
		if err := store.pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("%s is missing", table)
		}
		rows, err := store.pool.Query(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1`, table)
		if err != nil {
			t.Fatal(err)
		}
		columns := map[string]bool{}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			columns[column] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		for _, column := range required {
			if !columns[column] {
				t.Fatalf("%s column %q is missing: %#v", table, column, columns)
			}
		}
		if table == "memory_projection_events" && columns["content"] {
			t.Fatal("projection events must not store memory content")
		}

		var rlsEnabled bool
		var policyCount int
		if err := store.pool.QueryRow(ctx, `
SELECT c.relrowsecurity,
       (SELECT count(*) FROM pg_policy policy WHERE policy.polrelid = c.oid)
FROM pg_class c
WHERE c.oid = ('public.' || $1)::regclass`, table).Scan(&rlsEnabled, &policyCount); err != nil {
			t.Fatal(err)
		}
		if !rlsEnabled || policyCount != 1 {
			t.Fatalf("%s is not protected by one tenant policy: enabled=%v policies=%d", table, rlsEnabled, policyCount)
		}
	}

	var embeddingType string
	if err := store.pool.QueryRow(ctx, `
SELECT format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
WHERE a.attrelid = 'public.memory_vector_documents'::regclass
  AND a.attname = 'embedding' AND NOT a.attisdropped`).Scan(&embeddingType); err != nil {
		t.Fatal(err)
	}
	if embeddingType != "vector(1024)" {
		t.Fatalf("unexpected production embedding type %q", embeddingType)
	}

	var checks []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(pg_get_constraintdef(oid) ORDER BY conname), ARRAY[]::text[])
FROM pg_constraint
WHERE conrelid IN (
  'public.memory_projection_events'::regclass,
  'public.memory_projection_cursors'::regclass,
  'public.memory_vector_documents'::regclass,
	  'public.memory_vector_documents_2560'::regclass,
  'public.memory_retrieval_runs'::regclass
) AND contype = 'c'`).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(checks, " ")
	for _, expected := range []string{
		"active", "absent", "idle", "running", "failed", "lexical", "shadow", "vector",
		"64",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("retrieval checks do not constrain %q: %s", expected, joined)
		}
	}
	var profileCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_retrieval_profiles`).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if profileCount != 4 {
		t.Fatalf("retrieval profile registry count=%d, want 4", profileCount)
	}
	var candidateModel string
	if err := store.pool.QueryRow(ctx, `
SELECT model FROM memory_retrieval_profiles
WHERE profile_id = $1 AND lifecycle_status = 'candidate'`, MigrationRetrievalProfileID).Scan(&candidateModel); err != nil {
		t.Fatal(err)
	}
	if candidateModel != "BAAI/bge-large-zh-v1.5" {
		t.Fatalf("unexpected migration profile model %q", candidateModel)
	}
	var dimensionalModel, dimensionalClass string
	var dimensionalDimensions int
	if err := store.pool.QueryRow(ctx, `
SELECT model, dimensions, projection_class
FROM memory_retrieval_profiles
WHERE profile_id = $1 AND lifecycle_status = 'candidate'`,
		DimensionalMigrationRetrievalProfileID,
	).Scan(&dimensionalModel, &dimensionalDimensions, &dimensionalClass); err != nil {
		t.Fatal(err)
	}
	if dimensionalModel != "Qwen/Qwen3-Embedding-4B" || dimensionalDimensions != 2560 || dimensionalClass != "halfvec_2560" {
		t.Fatalf("unexpected dimensional profile %q/%d/%q", dimensionalModel, dimensionalDimensions, dimensionalClass)
	}

	var triggerCount, hnswCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM pg_trigger
WHERE tgrelid = 'public.governed_memories'::regclass
  AND tgname = 'enqueue_memory_projection_event'
  AND NOT tgisinternal`).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != 1 {
		t.Fatalf("expected one governed-memory projection trigger, got %d", triggerCount)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM pg_indexes
WHERE schemaname = 'public' AND tablename = 'memory_vector_documents'
  AND indexdef ILIKE '%USING hnsw%vector_cosine_ops%'`).Scan(&hnswCount); err != nil {
		t.Fatal(err)
	}
	if hnswCount != 1 {
		t.Fatalf("expected one HNSW cosine index, got %d", hnswCount)
	}
}

func TestProductionRetrievalTriggerTracksCurrentLifecycle(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-trigger"
	repoRoot := "/fixtures/retrieval-trigger"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}

	active, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "retrieval-trigger-active",
		MemoryKey:   "release.rollback.approvals",
		Content:     "Rollback requires two maintainers.",
		SourceRef:   "fixture:retrieval-trigger-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLatestProjectionState(t, store, active.Memory.MemoryID, "active")

	proposed, err := store.CommitGovernedObservation(ctx, tenantID, mustWorkspaceContinuity(t, store, tenantID, repoRoot), CommitObservationRequest{
		OperationID: "retrieval-trigger-proposed",
		Kind:        ObservationKindAgentResult,
		Content:     "Unconfirmed agent suggestion.",
		SourceRef:   "fixture:retrieval-trigger-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLatestProjectionState(t, store, proposed.Memory.MemoryID, "absent")

	global, err := store.SetGlobalDefault(ctx, tenantID, "retrieval-trigger-global", "reply_language", "Reply in Chinese by default.")
	if err != nil {
		t.Fatal(err)
	}
	assertLatestProjectionState(t, store, global.Memory.MemoryID, "absent")

	revised, err := governance.ReviseSource(ctx, repoRoot, active.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "retrieval-trigger-revised",
		Content:     "Rollback requires three maintainers.",
		SourceRef:   "fixture:retrieval-trigger-v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLatestProjectionState(t, store, active.Memory.MemoryID, "absent")
	assertLatestProjectionState(t, store, revised.Memory.MemoryID, "active")

	if _, err := governance.Forget(ctx, repoRoot, revised.Memory.MemoryID, "retrieval-trigger-delete"); err != nil {
		t.Fatal(err)
	}
	assertLatestProjectionState(t, store, revised.Memory.MemoryID, "absent")
}

func TestProductionRetrievalMigrationSeedsExistingGovernedMemory(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store := openTestStore(t)
	ctx := context.Background()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(storepostgres.Migrations)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	if err := goose.DownToContext(ctx, db, "migrations", 13); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := goose.UpToContext(context.Background(), db, "migrations", 18); err != nil {
			t.Errorf("restore schema 18: %v", err)
		}
	})

	var continuityID string
	if err := store.pool.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ('retrieval-upgrade', 'workspace', 'active')
RETURNING id::text`).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	memoryIDs := map[string]string{}
	for _, memory := range []struct {
		kind    string
		status  string
		content string
		want    string
	}{
		{kind: "fact", status: "active", content: "UPGRADE-ACTIVE-1771", want: "active"},
		{kind: "fact", status: "proposed", content: "UPGRADE-PROPOSED-2831", want: "absent"},
		{kind: "global_default", status: "active", content: "UPGRADE-GLOBAL-3941", want: "absent"},
	} {
		var observationID, memoryID string
		if err := store.pool.QueryRow(ctx, `
INSERT INTO observations (tenant_id, continuity_id, operation_id, observation_kind, content)
VALUES ('retrieval-upgrade', $1::uuid, $2, 'source_update', $3)
RETURNING id::text`, continuityID, "retrieval-upgrade-observation-"+memory.content, memory.content).Scan(&observationID); err != nil {
			t.Fatal(err)
		}
		if err := store.pool.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content
)
VALUES ('retrieval-upgrade', $1::uuid, $2::uuid, $3, $4, $5)
RETURNING id::text`, continuityID, observationID, memory.kind, memory.status, memory.content).Scan(&memoryID); err != nil {
			t.Fatal(err)
		}
		memoryIDs[memoryID] = memory.want
	}
	if err := goose.UpToContext(ctx, db, "migrations", 14); err != nil {
		t.Fatal(err)
	}

	for memoryID, want := range memoryIDs {
		assertLatestProjectionState(t, store, memoryID, want)
	}
}

func assertLatestProjectionState(t *testing.T, store *Store, memoryID, expected string) {
	t.Helper()
	var state string
	if err := store.pool.QueryRow(context.Background(), `
SELECT desired_state
FROM memory_projection_events
WHERE memory_id = $1::uuid
ORDER BY event_id DESC
LIMIT 1`, memoryID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != expected {
		t.Fatalf("memory %s projection state=%q want %q", memoryID, state, expected)
	}
}

func mustWorkspaceContinuity(t *testing.T, store *Store, tenantID, repoRoot string) string {
	t.Helper()
	resolution, err := store.ResolveWorkspace(context.Background(), tenantID, WorkspaceAnchor{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != ResolutionResolved {
		t.Fatalf("workspace was not resolved: %#v", resolution)
	}
	return resolution.ContinuityID
}
