package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"vermory/internal/authn"
)

func TestMemoryEligibilityDumpRestore(t *testing.T) {
	baseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	sourceURL, _, _ := createOperationsDatabase(t, baseURL)
	targetURL, _, _ := createOperationsDatabase(t, baseURL)
	source, err := OpenStore(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.Close)
	if err := source.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := "eligibility-restore"
	continuityID := createEligibilityContinuity(t, source, tenantID, "workspace")
	asOf := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	currentID := seedEligibilityMemory(t, source, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-RESTORE current procedure",
	})
	scheduledID := seedEligibilityMemory(t, source, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-RESTORE scheduled procedure",
	})
	expiredID := seedEligibilityMemory(t, source, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-RESTORE expired procedure",
	})
	archivedID := seedEligibilityMemory(t, source, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-RESTORE archived procedure",
	})
	secret := "ELIGIBILITY-RESTORE-FORGOTTEN-7319"
	deletedID := seedEligibilityMemory(t, source, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: secret,
	})
	scheduledFrom := asOf.Add(time.Hour)
	if _, err := source.SetMemoryValidity(ctx, SetMemoryValidityRequest{
		OperationID: "eligibility-restore-scheduled", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: scheduledID, ValidFrom: &scheduledFrom,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.SetMemoryValidity(ctx, SetMemoryValidityRequest{
		OperationID: "eligibility-restore-expired", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: expiredID, ValidUntil: &asOf,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ArchiveMemory(ctx, ArchiveMemoryRequest{
		OperationID: "eligibility-restore-archive", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: archivedID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := source.DeleteMemory(ctx, tenantID, continuityID, deletedID); err != nil {
		t.Fatal(err)
	}

	incumbentEmbedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	incumbentWorker := mustProjectionWorker(t, source, incumbentEmbedder, tenantID, 16)
	if result, err := incumbentWorker.RunOnce(ctx); err != nil || result.Lag != 0 {
		t.Fatalf("source incumbent projection=%#v err=%v", result, err)
	}
	candidateEmbedder := &projectionTestEmbedder{vector: testVector2560(0.25)}
	candidateWorker, err := NewProjectionWorker(source, candidateEmbedder, ProjectionWorkerOptions{
		TenantID: tenantID, Profile: dimensionalMigrationProfile(t), BatchSize: 16, SnapshotPageSize: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := candidateWorker.RunOnce(ctx); err != nil || result.Lag != 0 {
		t.Fatalf("source candidate projection=%#v err=%v", result, err)
	}
	assertMemoryEligibilityVectorCounts(t, source, tenantID, 3, 3)
	sourceAuthority := memoryEligibilityAuthorityFingerprint(t, source, tenantID)
	sourceStates := memoryEligibilityStateFingerprint(t, source, tenantID, asOf)
	assertMemoryEligibilitySecretAbsent(t, source, secret)

	dumpPath := filepath.Join(t.TempDir(), "memory-eligibility.dump")
	dump := exec.Command(postgresTestTool(t, "pg_dump"), "--format=custom", "--no-owner", "--no-acl", "--file", dumpPath, sourceURL)
	if output, err := dump.CombinedOutput(); err != nil {
		t.Fatalf("dump memory eligibility database: %v\n%s", err, output)
	}
	restore := exec.Command(postgresTestTool(t, "pg_restore"), "--exit-on-error", "--no-owner", "--no-acl", "--dbname", targetURL, dumpPath)
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore memory eligibility database: %v\n%s", err, output)
	}
	target, err := OpenStore(ctx, targetURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Close)
	if version, err := target.SchemaVersion(ctx); err != nil || version != MaximumSupportedSchemaVersion {
		t.Fatalf("restored schema version=%d err=%v", version, err)
	}
	if got := memoryEligibilityAuthorityFingerprint(t, target, tenantID); got != sourceAuthority {
		t.Fatalf("restore changed eligibility authority: source=%s target=%s", sourceAuthority, got)
	}
	if got := memoryEligibilityStateFingerprint(t, target, tenantID, asOf); got != sourceStates {
		t.Fatalf("restore changed effective states: source=%s target=%s", sourceStates, got)
	}
	assertMemoryEligibilitySecretAbsent(t, target, secret)

	roleName, runtimeURL := createTenantPoolRole(t, target.pool, targetURL, "eligibility_restore", "")
	if err := authn.GrantRuntimeRole(ctx, target.pool, roleName); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := OpenStoreWithOptions(ctx, runtimeURL, StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}

	if err := target.RebuildProjection(ctx, tenantID, continuityID); err != nil {
		t.Fatal(err)
	}
	if err := target.ResetVectorProjection(ctx, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	if result, err := mustProjectionWorker(t, target, incumbentEmbedder, tenantID, 16).RebuildCurrent(ctx); err != nil || result.Projected != 3 || result.Lag != 0 {
		t.Fatalf("restored incumbent rebuild=%#v err=%v", result, err)
	}
	if err := target.ResetVectorProjection(ctx, tenantID, DimensionalMigrationRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	restoredCandidate, err := NewProjectionWorker(target, candidateEmbedder, ProjectionWorkerOptions{
		TenantID: tenantID, Profile: dimensionalMigrationProfile(t), BatchSize: 16, SnapshotPageSize: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := restoredCandidate.RebuildCurrent(ctx); err != nil || result.Projected != 3 || result.Lag != 0 {
		t.Fatalf("restored candidate rebuild=%#v err=%v", result, err)
	}
	assertMemoryEligibilityVectorCounts(t, target, tenantID, 3, 3)
	if got := memoryEligibilityAuthorityFingerprint(t, target, tenantID); got != sourceAuthority {
		t.Fatalf("projection rebuild changed eligibility authority: source=%s rebuilt=%s", sourceAuthority, got)
	}
	if got := memoryEligibilityStateFingerprint(t, target, tenantID, asOf); got != sourceStates {
		t.Fatalf("projection rebuild changed effective states: source=%s rebuilt=%s", sourceStates, got)
	}
	assertMemoryEligibilitySecretAbsent(t, target, secret)

	lexical, err := target.SearchEligibleMemoryAt(ctx, tenantID, continuityID, "ELIGIBILITY-RESTORE procedure", 10, asOf)
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, lexical, []string{currentID})
	for _, coordinator := range []*RetrievalCoordinator{
		mustRetrievalCoordinator(t, target, incumbentEmbedder),
		mustDimensionalRetrievalCoordinator(t, target, candidateEmbedder),
	} {
		result, err := coordinator.Retrieve(ctx, RetrievalRequest{
			OperationID: fmt.Sprintf("eligibility-restore-vector-%d", coordinator.profile.Dimensions),
			TenantID:    tenantID, ContinuityIDs: []string{continuityID},
			Query: "ELIGIBILITY-RESTORE procedure", Limit: 10, Mode: RetrievalVector, EligibilityAsOf: asOf,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertMemoryIDSet(t, result.Memories, []string{currentID})
	}
}

func mustDimensionalRetrievalCoordinator(t *testing.T, store *Store, embedder Embedder) *RetrievalCoordinator {
	t.Helper()
	coordinator, err := NewRetrievalCoordinator(store, embedder, dimensionalMigrationProfile(t))
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func memoryEligibilityAuthorityFingerprint(t *testing.T, store *Store, tenantID string) string {
	t.Helper()
	var fingerprint string
	if err := store.pool.QueryRow(context.Background(), `
WITH authority AS (
  SELECT 'memory' AS kind, to_jsonb(memory)::text AS value
  FROM governed_memories memory WHERE tenant_id = $1
  UNION ALL
  SELECT 'operation', to_jsonb(operation)::text
  FROM memory_eligibility_operations operation WHERE tenant_id = $1
)
SELECT encode(digest(COALESCE(string_agg(kind || ':' || value, E'\n' ORDER BY kind, value), ''), 'sha256'), 'hex')
FROM authority`, tenantID).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func memoryEligibilityStateFingerprint(t *testing.T, store *Store, tenantID string, asOf time.Time) string {
	t.Helper()
	var fingerprint string
	if err := store.pool.QueryRow(context.Background(), `
WITH states AS (
  SELECT id::text AS memory_id,
    CASE
      WHEN lifecycle_status = 'deleted' OR content = '[redacted]' THEN 'deleted'
      WHEN lifecycle_status = 'superseded' THEN 'superseded'
      WHEN lifecycle_status = 'rejected' THEN 'rejected'
      WHEN lifecycle_status = 'proposed' THEN 'proposed'
      WHEN lifecycle_status = 'archived' THEN 'archived'
      WHEN valid_from IS NOT NULL AND $2 < valid_from THEN 'scheduled'
      WHEN valid_until IS NOT NULL AND $2 >= valid_until THEN 'expired'
      ELSE 'current'
    END AS effective_state
  FROM governed_memories
  WHERE tenant_id = $1
)
SELECT encode(digest(string_agg(effective_state || ':' || memory_id, E'\n' ORDER BY effective_state, memory_id), 'sha256'), 'hex')
FROM states`, tenantID, asOf).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func assertMemoryEligibilityVectorCounts(t *testing.T, store *Store, tenantID string, want1024, want2560 int) {
	t.Helper()
	var got1024, got2560 int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM memory_vector_documents WHERE tenant_id = $1),
  (SELECT count(*) FROM memory_vector_documents_2560 WHERE tenant_id = $1)`, tenantID).Scan(&got1024, &got2560); err != nil {
		t.Fatal(err)
	}
	if got1024 != want1024 || got2560 != want2560 {
		t.Fatalf("vector counts=%d/%d want %d/%d", got1024, got2560, want1024, want2560)
	}
}

func assertMemoryEligibilitySecretAbsent(t *testing.T, store *Store, secret string) {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM governed_memories WHERE position($1 IN content) > 0)
  + (SELECT count(*) FROM observations WHERE position($1 IN content) > 0)
  + (SELECT count(*) FROM memory_search_documents WHERE position($1 IN content) > 0)`, secret).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("forgotten content survived authority or projection: count=%d", count)
	}
}
