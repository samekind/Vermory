package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMemoryEligibilityRequestValidation(t *testing.T) {
	utcFrom := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	utcUntil := utcFrom.Add(time.Hour)
	nonUTC := time.Date(2026, 7, 20, 14, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	zero := time.Time{}
	valid := SetMemoryValidityRequest{
		OperationID: "eligibility-valid", TenantID: "eligibility-validation",
		ContinuityID: "11111111-1111-1111-1111-111111111111",
		MemoryID:     "22222222-2222-2222-2222-222222222222",
		ValidFrom:    &utcFrom, ValidUntil: &utcUntil,
	}
	invalid := []SetMemoryValidityRequest{
		{},
		func() SetMemoryValidityRequest { request := valid; request.OperationID = " "; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.TenantID = " "; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.ContinuityID = " "; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.MemoryID = " "; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.ValidFrom = &nonUTC; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.ValidUntil = &zero; return request }(),
		func() SetMemoryValidityRequest { request := valid; request.ValidUntil = &utcFrom; return request }(),
	}
	for index := range invalid {
		if err := invalid[index].Validate(); err == nil {
			t.Fatalf("invalid validity request %d was accepted: %#v", index, invalid[index])
		}
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := (ArchiveMemoryRequest{}).Validate(); err == nil {
		t.Fatal("blank archive request was accepted")
	}

	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-validation"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	otherContinuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	otherTenantContinuityID := createEligibilityContinuity(t, store, "eligibility-validation-other", "workspace")
	activeID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "validation active",
	})
	for _, lifecycle := range []string{"proposed", "superseded", "rejected", "archived", "deleted"} {
		content := "validation " + lifecycle
		if lifecycle == "deleted" {
			content = "[redacted]"
		}
		memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
			TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: lifecycle, Content: content,
		})
		request := valid
		request.OperationID = "eligibility-validity-reject-" + lifecycle
		request.ContinuityID = continuityID
		request.MemoryID = memoryID
		if _, err := store.SetMemoryValidity(ctx, request); err == nil {
			t.Fatalf("set-validity accepted lifecycle %s", lifecycle)
		}
		if _, err := store.ArchiveMemory(ctx, ArchiveMemoryRequest{
			OperationID: "eligibility-archive-reject-" + lifecycle,
			TenantID:    tenantID, ContinuityID: continuityID, MemoryID: memoryID,
		}); err == nil {
			t.Fatalf("archive accepted lifecycle %s", lifecycle)
		}
	}
	for _, request := range []SetMemoryValidityRequest{
		{OperationID: "eligibility-wrong-continuity", TenantID: tenantID, ContinuityID: otherContinuityID, MemoryID: activeID, ValidUntil: &utcUntil},
		{OperationID: "eligibility-cross-tenant", TenantID: "eligibility-validation-other", ContinuityID: otherTenantContinuityID, MemoryID: activeID, ValidUntil: &utcUntil},
	} {
		if _, err := store.SetMemoryValidity(ctx, request); err == nil {
			t.Fatalf("out-of-scope validity request was accepted: %#v", request)
		}
	}
}

func TestMemoryEligibilityOperationReplay(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-replay"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	content := "DO-NOT-AUDIT sk-secret-example postgresql://private.example/vermory"
	memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: content,
	})
	validFrom := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	validUntil := validFrom.Add(2 * time.Hour)
	request := SetMemoryValidityRequest{
		OperationID: "eligibility-replay-validity", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: memoryID,
		ValidFrom: &validFrom, ValidUntil: &validUntil,
	}
	first, err := store.SetMemoryValidity(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.SetMemoryValidity(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed {
		t.Fatalf("equal operation did not replay: %#v", replay)
	}
	first.Replayed = true
	if !reflect.DeepEqual(first, replay) {
		t.Fatalf("replay changed persisted receipt: first=%#v replay=%#v", first, replay)
	}
	conflict := request
	changedUntil := validUntil.Add(time.Hour)
	conflict.ValidUntil = &changedUntil
	if _, err := store.SetMemoryValidity(ctx, conflict); err == nil || !strings.Contains(err.Error(), "another") {
		t.Fatalf("conflicting operation replay was accepted: %v", err)
	}

	var audit string
	if err := store.pool.QueryRow(ctx, `
SELECT row_to_json(operation)::text
FROM memory_eligibility_operations operation
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, request.OperationID).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{content, "DO-NOT-AUDIT", "sk-secret-example", "postgresql://"} {
		if strings.Contains(audit, forbidden) {
			t.Fatalf("eligibility audit contains protected content %q: %s", forbidden, audit)
		}
	}
}

func TestArchiveMemory(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-archive"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	content := "ELIGIBILITY-ARCHIVE preserve authorized historical content"
	memoryID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: content,
	})
	receipt, err := store.ArchiveMemory(ctx, ArchiveMemoryRequest{
		OperationID: "eligibility-archive-memory", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: memoryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PreviousState != MemoryEffectiveCurrent || receipt.ResultState != MemoryEffectiveArchived {
		t.Fatalf("unexpected archive state transition: %#v", receipt)
	}
	var lifecycle, storedContent string
	if err := store.pool.QueryRow(ctx, `
SELECT lifecycle_status, content FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&lifecycle, &storedContent); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "archived" || storedContent != content {
		t.Fatalf("archive altered authority incorrectly: lifecycle=%s content=%q", lifecycle, storedContent)
	}
	var lexicalRows int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_search_documents WHERE memory_id = $1::uuid`, memoryID).Scan(&lexicalRows); err != nil {
		t.Fatal(err)
	}
	if lexicalRows != 0 {
		t.Fatalf("archive retained lexical projection rows=%d", lexicalRows)
	}
	var desiredState string
	if err := store.pool.QueryRow(ctx, `
SELECT desired_state
FROM memory_projection_events
WHERE tenant_id = $1 AND memory_id = $2::uuid
ORDER BY event_id DESC LIMIT 1`, tenantID, memoryID).Scan(&desiredState); err != nil {
		t.Fatal(err)
	}
	if desiredState != "absent" {
		t.Fatalf("archive projection event state=%s", desiredState)
	}
	if err := store.RebuildProjection(ctx, tenantID, continuityID); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_search_documents WHERE memory_id = $1::uuid`, memoryID).Scan(&lexicalRows); err != nil {
		t.Fatal(err)
	}
	if lexicalRows != 0 {
		t.Fatalf("projection rebuild restored archived memory rows=%d", lexicalRows)
	}
	replay, err := store.ArchiveMemory(ctx, ArchiveMemoryRequest{
		OperationID: "eligibility-archive-memory", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: memoryID,
	})
	if err != nil || !replay.Replayed || replay.MemoryID != memoryID {
		t.Fatalf("archive replay failed: receipt=%#v err=%v", replay, err)
	}
}

func TestExtendExpiredMemory(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-extend"
	repoRoot := "/fixtures/eligibility-extend"
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	governance := NewGovernanceService(store, tenantID)
	old, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "eligibility-extend-old", MemoryKey: "deploy.workaround",
		Content: "Use the temporary deployment workaround.", SourceRef: "fixture:eligibility-extend-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := snapshot.AsOf.Add(-time.Minute)
	if _, err := store.SetMemoryValidity(ctx, SetMemoryValidityRequest{
		OperationID: "eligibility-expire-old", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: old.Memory.MemoryID, ValidUntil: &expiredAt,
	}); err != nil {
		t.Fatal(err)
	}
	if memories, err := store.SearchEligibleMemoryAt(ctx, tenantID, continuityID, "temporary deployment workaround", 5, snapshot.AsOf); err != nil || len(memories) != 0 {
		t.Fatalf("expired memory remained eligible: memories=%#v err=%v", memories, err)
	}
	extendedUntil := snapshot.AsOf.Add(time.Hour)
	if _, err := store.SetMemoryValidity(ctx, SetMemoryValidityRequest{
		OperationID: "eligibility-extend-old", TenantID: tenantID,
		ContinuityID: continuityID, MemoryID: old.Memory.MemoryID, ValidUntil: &extendedUntil,
	}); err != nil {
		t.Fatal(err)
	}
	if memories, err := store.SearchEligibleMemoryAt(ctx, tenantID, continuityID, "temporary deployment workaround", 5, snapshot.AsOf); err != nil || len(memories) != 1 || memories[0].ID != old.Memory.MemoryID {
		t.Fatalf("extended memory did not become current: memories=%#v err=%v", memories, err)
	}

	futureStart := snapshot.AsOf.Add(time.Hour)
	futureID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-FUTURE natural activation", ValidFrom: &futureStart,
	})
	if before, err := store.SearchEligibleMemoryAt(ctx, tenantID, continuityID, "ELIGIBILITY-FUTURE", 5, futureStart.Add(-time.Microsecond)); err != nil || len(before) != 0 {
		t.Fatalf("future memory activated early: memories=%#v err=%v", before, err)
	}
	if atBoundary, err := store.SearchEligibleMemoryAt(ctx, tenantID, continuityID, "ELIGIBILITY-FUTURE", 5, futureStart); err != nil || len(atBoundary) != 1 || atBoundary[0].ID != futureID {
		t.Fatalf("future memory did not activate at boundary: memories=%#v err=%v", atBoundary, err)
	}

	corrected, err := governance.Correct(ctx, repoRoot, old.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "eligibility-correct-old", Content: "Use the permanent deployment procedure.",
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.ListGovernedMemories(ctx, tenantID, continuityID)
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range history {
		if memory.ID == corrected.Memory.MemoryID {
			if memory.ValidFrom != nil || memory.ValidUntil != nil {
				t.Fatalf("corrected revision inherited old validity: %#v", memory)
			}
			return
		}
	}
	t.Fatalf("corrected memory %s not found in history: %s", corrected.Memory.MemoryID, fmt.Sprint(history))
}
