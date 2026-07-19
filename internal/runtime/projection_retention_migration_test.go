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

func TestProjectionRetentionSchema(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != 23 {
		t.Fatalf("schema version=%d want 23", version)
	}

	for _, table := range []string{"memory_projection_retention", "memory_projection_prune_runs"} {
		var rlsEnabled, rlsForced bool
		if err := store.pool.QueryRow(ctx, `
SELECT relrowsecurity, relforcerowsecurity
FROM pg_class
WHERE oid = ('public.' || $1)::regclass`, table).Scan(&rlsEnabled, &rlsForced); err != nil {
			t.Fatalf("read %s RLS: %v", table, err)
		}
		if !rlsEnabled || rlsForced {
			t.Fatalf("%s RLS enabled/forced=%t/%t want true/false", table, rlsEnabled, rlsForced)
		}
		var policyCount int
		if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_policies
WHERE schemaname = 'public' AND tablename = $1
  AND qual LIKE '%vermory.tenant_id%'
  AND with_check LIKE '%vermory.tenant_id%'`, table).Scan(&policyCount); err != nil {
			t.Fatal(err)
		}
		if policyCount != 1 {
			t.Fatalf("%s tenant policy count=%d want 1", table, policyCount)
		}
		var publicPrivileges bool
		if err := store.pool.QueryRow(ctx, `
SELECT has_table_privilege('public', 'public.' || $1, 'SELECT')
    OR has_table_privilege('public', 'public.' || $1, 'INSERT')
    OR has_table_privilege('public', 'public.' || $1, 'UPDATE')
    OR has_table_privilege('public', 'public.' || $1, 'DELETE')`, table).Scan(&publicPrivileges); err != nil {
			t.Fatal(err)
		}
		if publicPrivileges {
			t.Fatalf("%s retained PUBLIC privileges", table)
		}
	}

	assertProjectionRetentionConstraints(t, store)
}

func assertProjectionRetentionConstraints(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	var cursorConstraint, retentionConstraints, pruneConstraints string
	if err := store.pool.QueryRow(ctx, `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.memory_projection_cursors'::regclass`).Scan(&cursorConstraint); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cursorConstraint, "rebuild_required") {
		t.Fatalf("cursor status constraint lacks rebuild_required: %s", cursorConstraint)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.memory_projection_retention'::regclass`).Scan(&retentionConstraints); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(retentionConstraints, "PRIMARY KEY (tenant_id)") ||
		!strings.Contains(retentionConstraints, "pruned_through_event_id >= 0") {
		t.Fatalf("retention constraints incomplete: %s", retentionConstraints)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.memory_projection_prune_runs'::regclass`).Scan(&pruneConstraints); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"UNIQUE (tenant_id, operation_id)",
		"retain_tail_events >= 0",
		"length(request_fingerprint) = 64",
		"result = ANY (ARRAY['pruned'::text, 'noop'::text])",
	} {
		if !strings.Contains(pruneConstraints, fragment) {
			t.Fatalf("prune constraints lack %q: %s", fragment, pruneConstraints)
		}
	}
}

func TestProjectionRetentionMigrationRejectsDowngradeWithRebuildRequired(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_projection_cursors (
  tenant_id, profile_id, last_event_id, status, last_error_code
) VALUES ($1, $2, 0, 'rebuild_required', 'projection_rebuild_required')`,
		"retention-downgrade-blocked", ProductionRetrievalProfileID,
	); err != nil {
		t.Fatal(err)
	}
	db := openProjectionRetentionMigrationDB(t)
	if err := goose.DownToContext(ctx, db, "migrations", 16); err == nil ||
		!strings.Contains(err.Error(), "cannot downgrade while projection cursors require rebuild") {
		t.Fatalf("schema 17 downgrade was not blocked: %v", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != 17 {
		t.Fatalf("failed downgrade changed schema version to %d", version)
	}
	if _, err := store.pool.Exec(ctx, `
DELETE FROM memory_projection_cursors
WHERE tenant_id = $1 AND profile_id = $2`, "retention-downgrade-blocked", ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, db, "migrations", 18); err != nil {
		t.Fatalf("restore schema 18 after blocked W17 downgrade: %v", err)
	}
}

func TestProjectionRetentionMigrationUpDown(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	db := openProjectionRetentionMigrationDB(t)
	if err := goose.DownToContext(ctx, db, "migrations", 16); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := goose.UpToContext(context.Background(), db, "migrations", 18); err != nil {
			t.Errorf("restore schema 18: %v", err)
		}
	})
	for _, table := range []string{"memory_projection_retention", "memory_projection_prune_runs"} {
		var exists bool
		if err := store.pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("schema 17 table %s survived downgrade", table)
		}
	}
	var cursorConstraint string
	if err := store.pool.QueryRow(ctx, `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.memory_projection_cursors'::regclass`).Scan(&cursorConstraint); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cursorConstraint, "rebuild_required") {
		t.Fatalf("schema 16 cursor constraint retained rebuild_required: %s", cursorConstraint)
	}
	if err := goose.UpToContext(ctx, db, "migrations", 17); err != nil {
		t.Fatal(err)
	}
	assertProjectionRetentionConstraints(t, store)
}

func openProjectionRetentionMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
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
	return db
}
