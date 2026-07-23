package runtime

import (
	"context"
	"testing"
	"time"

	"vermory/internal/authn"
)

func TestMemoryEligibilityRuntimeRole(t *testing.T) {
	admin, databaseURL := openTenantPoolAdmin(t)
	ctx := context.Background()
	tenantA := "eligibility-rls-a"
	tenantB := "eligibility-rls-b"
	continuityA := createEligibilityContinuity(t, admin, tenantA, "workspace")
	continuityB := createEligibilityContinuity(t, admin, tenantB, "workspace")
	memoryA := seedEligibilityMemory(t, admin, eligibilityMemorySeed{
		TenantID: tenantA, ContinuityID: continuityA, Kind: "fact", Lifecycle: "active", Content: "RLS tenant A memory",
	})
	memoryB := seedEligibilityMemory(t, admin, eligibilityMemorySeed{
		TenantID: tenantB, ContinuityID: continuityB, Kind: "fact", Lifecycle: "active", Content: "RLS tenant B memory",
	})
	validUntil := time.Now().UTC().Add(time.Hour)
	for _, request := range []SetMemoryValidityRequest{
		{OperationID: "eligibility-rls-a", TenantID: tenantA, ContinuityID: continuityA, MemoryID: memoryA, ValidUntil: &validUntil},
		{OperationID: "eligibility-rls-b", TenantID: tenantB, ContinuityID: continuityB, MemoryID: memoryB, ValidUntil: &validUntil},
	} {
		if _, err := admin.SetMemoryValidity(ctx, request); err != nil {
			t.Fatal(err)
		}
	}

	roleName, runtimeURL := createTenantPoolRole(t, admin.pool, databaseURL, "eligibility", "")
	if err := authn.GrantRuntimeRole(ctx, admin.pool, roleName); err != nil {
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
	tenantCtxA, err := withTenantContext(ctx, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	var ownReceipts, crossReceipts int
	if err := runtimeStore.pool.QueryRow(tenantCtxA, `
SELECT count(*) FROM memory_eligibility_operations
WHERE operation_id = 'eligibility-rls-a'`).Scan(&ownReceipts); err != nil {
		t.Fatalf("runtime role cannot read authorized eligibility receipt: %v", err)
	}
	if err := runtimeStore.pool.QueryRow(tenantCtxA, `
SELECT count(*) FROM memory_eligibility_operations
WHERE operation_id = 'eligibility-rls-b'`).Scan(&crossReceipts); err != nil {
		t.Fatal(err)
	}
	if ownReceipts != 1 || crossReceipts != 0 {
		t.Fatalf("eligibility receipt RLS mismatch: own=%d cross=%d", ownReceipts, crossReceipts)
	}
	if matches, err := runtimeStore.SearchActiveMemory(tenantCtxA, tenantA, continuityA, "RLS tenant A", 5); err != nil || len(matches) != 1 || matches[0].ID != memoryA {
		t.Fatalf("runtime role could not read eligible memory: matches=%#v err=%v", matches, err)
	}

	for label, statement := range map[string]string{
		"insert receipt": `INSERT INTO memory_eligibility_operations (
  tenant_id, continuity_id, memory_id, operation_id, action, request_fingerprint,
  previous_lifecycle_status, result_lifecycle_status
) VALUES (
  'eligibility-rls-a', '` + continuityA + `'::uuid, '` + memoryA + `'::uuid,
  'runtime-forbidden-insert', 'archive', repeat('a', 64), 'active', 'archived'
)`,
		"update receipt": `UPDATE memory_eligibility_operations
SET action = 'archive' WHERE operation_id = 'eligibility-rls-a'`,
		"delete receipt": `DELETE FROM memory_eligibility_operations
WHERE operation_id = 'eligibility-rls-a'`,
		"update validity": `UPDATE governed_memories
SET valid_until = now() + interval '2 hours' WHERE id = '` + memoryA + `'::uuid`,
	} {
		if _, err := runtimeStore.pool.Exec(tenantCtxA, statement); err == nil {
			t.Fatalf("runtime role succeeded at forbidden %s", label)
		}
	}
	if _, err := NewMemoryEligibilityService(runtimeStore, tenantA).SetValidity(ctx, SetMemoryValidityRequest{
		OperationID: "runtime-forbidden-service", ContinuityID: continuityA,
		MemoryID: memoryA, ValidUntil: &validUntil,
	}); err == nil {
		t.Fatal("runtime role performed operator-only validity mutation")
	}
}
