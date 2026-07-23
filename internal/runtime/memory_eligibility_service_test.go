package runtime

import (
	"context"
	"testing"
	"time"
)

func TestMemoryEligibilityServiceScopesTenant(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-service"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "service scoped memory",
	})
	validUntil := time.Date(2026, 7, 21, 6, 0, 0, 0, time.UTC)
	service := NewMemoryEligibilityService(store, tenantID)
	receipt, err := service.SetValidity(ctx, SetMemoryValidityRequest{
		OperationID: "eligibility-service-validity", ContinuityID: continuityID,
		MemoryID: memoryID, ValidUntil: &validUntil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.MemoryID != memoryID || receipt.ContinuityID != continuityID {
		t.Fatalf("unexpected service receipt: %#v", receipt)
	}
	if _, err := service.Archive(ctx, ArchiveMemoryRequest{
		OperationID: "eligibility-service-cross-tenant", TenantID: "other",
		ContinuityID: continuityID, MemoryID: memoryID,
	}); err == nil {
		t.Fatal("service accepted caller-supplied cross-tenant target")
	}
}
