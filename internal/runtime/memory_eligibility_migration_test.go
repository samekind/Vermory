package runtime

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	storepostgres "vermory/internal/store/postgres"

	"github.com/pressly/goose/v3"
)

func TestMemoryEligibilitySchema(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != MaximumSupportedSchemaVersion {
		t.Fatalf("schema version=%d want %d", version, MaximumSupportedSchemaVersion)
	}

	for _, column := range []struct {
		table    string
		name     string
		nullable string
	}{
		{table: "governed_memories", name: "valid_from", nullable: "YES"},
		{table: "governed_memories", name: "valid_until", nullable: "YES"},
		{table: "memory_deliveries", name: "eligibility_as_of", nullable: "NO"},
		{table: "memory_retrieval_runs", name: "eligibility_as_of", nullable: "NO"},
	} {
		var dataType, nullable string
		if err := store.pool.QueryRow(ctx, `
SELECT data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
			column.table, column.name).Scan(&dataType, &nullable); err != nil {
			t.Fatalf("read %s.%s: %v", column.table, column.name, err)
		}
		if dataType != "timestamp with time zone" || nullable != column.nullable {
			t.Fatalf("%s.%s type/nullability=%s/%s", column.table, column.name, dataType, nullable)
		}
	}

	var memoryConstraints string
	if err := store.pool.QueryRow(ctx, `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.governed_memories'::regclass`).Scan(&memoryConstraints); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"archived", "valid_until", "valid_from"} {
		if !strings.Contains(memoryConstraints, fragment) {
			t.Fatalf("governed memory constraints lack %q: %s", fragment, memoryConstraints)
		}
	}

	var functionVolatility string
	if err := store.pool.QueryRow(ctx, `
SELECT provolatile::text
FROM pg_proc
WHERE oid = 'public.memory_is_eligible(text,text,timestamp with time zone,timestamp with time zone,timestamp with time zone)'::regprocedure`).Scan(&functionVolatility); err != nil {
		t.Fatal(err)
	}
	if functionVolatility != "i" {
		t.Fatalf("memory_is_eligible volatility=%q want immutable", functionVolatility)
	}

	var rlsEnabled, rlsForced bool
	if err := store.pool.QueryRow(ctx, `
SELECT relrowsecurity, relforcerowsecurity
FROM pg_class
WHERE oid = 'public.memory_eligibility_operations'::regclass`).Scan(&rlsEnabled, &rlsForced); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled || rlsForced {
		t.Fatalf("eligibility operation RLS enabled/forced=%t/%t want true/false", rlsEnabled, rlsForced)
	}
	var policyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_policies
WHERE schemaname = 'public' AND tablename = 'memory_eligibility_operations'
  AND qual LIKE '%vermory.tenant_id%'
  AND with_check LIKE '%vermory.tenant_id%'`).Scan(&policyCount); err != nil {
		t.Fatal(err)
	}
	if policyCount != 1 {
		t.Fatalf("eligibility operation tenant policy count=%d want 1", policyCount)
	}
	var publicPrivileges bool
	if err := store.pool.QueryRow(ctx, `
SELECT has_table_privilege('public', 'public.memory_eligibility_operations', 'SELECT')
    OR has_table_privilege('public', 'public.memory_eligibility_operations', 'INSERT')
    OR has_table_privilege('public', 'public.memory_eligibility_operations', 'UPDATE')
    OR has_table_privilege('public', 'public.memory_eligibility_operations', 'DELETE')`).Scan(&publicPrivileges); err != nil {
		t.Fatal(err)
	}
	if publicPrivileges {
		t.Fatal("memory_eligibility_operations retained PUBLIC privileges")
	}

	assertMemoryEligibilityOperationConstraints(t, store)
}

func assertMemoryEligibilityOperationConstraints(t *testing.T, store *Store) {
	t.Helper()
	var constraints string
	if err := store.pool.QueryRow(context.Background(), `
SELECT string_agg(pg_get_constraintdef(oid), ' ' ORDER BY conname)
FROM pg_constraint
WHERE conrelid = 'public.memory_eligibility_operations'::regclass`).Scan(&constraints); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"UNIQUE (tenant_id, operation_id)",
		"action = ANY (ARRAY['set_validity'::text, 'archive'::text])",
		"request_fingerprint ~ '^[0-9a-f]{64}$'::text",
		"FOREIGN KEY (tenant_id, continuity_id, memory_id)",
	} {
		if !strings.Contains(constraints, fragment) {
			t.Fatalf("eligibility operation constraints lack %q: %s", fragment, constraints)
		}
	}
}

func TestMemoryEligibilityMigrationRejectsInvalidState(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	var continuityID, observationID, memoryID string
	if err := store.pool.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ('eligibility-invalid', 'workspace', 'active')
RETURNING id::text`).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
INSERT INTO observations (
  tenant_id, continuity_id, operation_id, observation_kind, content, source_ref
) VALUES (
  'eligibility-invalid', $1::uuid, 'eligibility-invalid-observation',
  'source_update', 'valid control', 'fixture:eligibility-invalid'
)
RETURNING id::text`, continuityID).Scan(&observationID); err != nil {
		t.Fatal(err)
	}
	boundary := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind,
  lifecycle_status, content, valid_from, valid_until
) VALUES (
  'eligibility-invalid', $1::uuid, $2::uuid, 'fact',
  'active', 'invalid interval', $3, $3
)`, continuityID, observationID, boundary); err == nil {
		t.Fatal("equal validity interval was accepted")
	}
	if err := store.pool.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind,
  lifecycle_status, content
) VALUES (
  'eligibility-invalid', $1::uuid, $2::uuid, 'fact', 'active', 'valid control'
)
RETURNING id::text`, continuityID, observationID).Scan(&memoryID); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		action      string
		fingerprint string
	}{
		"action":      {action: "expire", fingerprint: strings.Repeat("a", 64)},
		"fingerprint": {action: "archive", fingerprint: strings.Repeat("G", 64)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := store.pool.Exec(ctx, `
INSERT INTO memory_eligibility_operations (
  tenant_id, continuity_id, memory_id, operation_id, action,
  request_fingerprint, previous_lifecycle_status, result_lifecycle_status
) VALUES (
  'eligibility-invalid', $1::uuid, $2::uuid, $3, $4, $5, 'active', 'active'
)`, continuityID, memoryID, "eligibility-invalid-"+name, test.action, test.fingerprint)
			if err == nil {
				t.Fatalf("invalid %s was accepted", name)
			}
		})
	}
}

func TestMemoryEligibilityMigrationRejectsUnsafeDowngrade(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	var continuityID, observationID, memoryID string
	if err := store.pool.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ('eligibility-downgrade', 'workspace', 'active')
RETURNING id::text`).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
INSERT INTO observations (
  tenant_id, continuity_id, operation_id, observation_kind, content, source_ref
) VALUES (
  'eligibility-downgrade', $1::uuid, 'eligibility-downgrade-observation',
  'source_update', 'bounded fact', 'fixture:eligibility-downgrade'
)
RETURNING id::text`, continuityID).Scan(&observationID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind,
  lifecycle_status, content, valid_until
) VALUES (
  'eligibility-downgrade', $1::uuid, $2::uuid, 'fact',
  'active', 'bounded fact', '2026-07-21T00:00:00Z'
)
RETURNING id::text`, continuityID, observationID).Scan(&memoryID); err != nil {
		t.Fatal(err)
	}
	db := openMemoryEligibilityMigrationDB(t)
	if err := goose.DownToContext(ctx, db, "migrations", 17); err == nil ||
		!strings.Contains(err.Error(), "cannot downgrade while memory eligibility state exists") {
		t.Fatalf("schema 18 unsafe downgrade was not blocked: %v", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != 18 {
		t.Fatalf("blocked downgrade changed schema version to %d", version)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE governed_memories
SET valid_until = NULL
WHERE id = $1::uuid`, memoryID); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryEligibilityMigrationUpDown(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	db := openMemoryEligibilityMigrationDB(t)
	if err := goose.DownToContext(ctx, db, "migrations", 17); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := goose.UpToContext(context.Background(), db, "migrations", 18); err != nil {
			t.Errorf("restore schema 18: %v", err)
		}
	})
	var tableExists bool
	if err := store.pool.QueryRow(ctx, `SELECT to_regclass('public.memory_eligibility_operations') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatal(err)
	}
	if tableExists {
		t.Fatal("schema 18 operation table survived downgrade")
	}
	var functionExists bool
	if err := store.pool.QueryRow(ctx, `
SELECT to_regprocedure('public.memory_is_eligible(text,text,timestamp with time zone,timestamp with time zone,timestamp with time zone)') IS NOT NULL`).Scan(&functionExists); err != nil {
		t.Fatal(err)
	}
	if functionExists {
		t.Fatal("schema 18 eligibility function survived downgrade")
	}
	if err := goose.UpToContext(ctx, db, "migrations", 18); err != nil {
		t.Fatal(err)
	}
	assertMemoryEligibilityOperationConstraints(t, store)
}

func openMemoryEligibilityMigrationDB(t *testing.T) *sql.DB {
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
