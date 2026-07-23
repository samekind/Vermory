package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestGovernanceInspectDoesNotCreateUnknownWorkspace(t *testing.T) {
	service := NewGovernanceService(openTestStore(t), "local")

	got, err := service.InspectWorkspace(context.Background(), "/repo/unknown")
	requireNoError(t, err)
	if got.Status != ResolutionNeedsConfirmation || got.ContinuityID != "" {
		t.Fatalf("unknown workspace was attached: %#v", got)
	}
}

func TestGovernanceSourceCorrectionAndForgetStayScoped(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewGovernanceService(store, "local")
	_, err := service.ConfirmWorkspace(ctx, "/repo/web-checkout")
	requireNoError(t, err)
	_, err = service.ConfirmWorkspace(ctx, "/repo/ops-console")
	requireNoError(t, err)

	source, err := service.AddSource(ctx, "/repo/web-checkout", GovernanceWriteRequest{
		OperationID: "operator-source-v1",
		Content:     "Use checkout_eta_v1 for the staged checkout release.",
		SourceRef:   "fixture:operator:v1",
	})
	requireNoError(t, err)
	if source.Memory.Status != "active" {
		t.Fatalf("source fact is not active: %#v", source)
	}

	corrected, err := service.Correct(ctx, "/repo/web-checkout", source.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "operator-correct-v2",
		Content:     "Use checkout_eta_v2 for the staged checkout release.",
	})
	requireNoError(t, err)
	if corrected.Memory.Status != "active" {
		t.Fatalf("correction is not active: %#v", corrected)
	}

	resolution, err := service.InspectWorkspace(ctx, "/repo/web-checkout")
	requireNoError(t, err)
	requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
	matches := mustSearch(t, store, resolution.ContinuityID, "checkout_eta_v1")
	if len(matches) != 1 || !strings.Contains(matches[0].Content, "checkout_eta_v2") {
		t.Fatalf("stale source was not replaced: %#v", matches)
	}

	forgotten, err := service.Forget(ctx, "/repo/web-checkout", corrected.Memory.MemoryID, "operator-forget-v2")
	requireNoError(t, err)
	if forgotten.Memory.Status != "deleted" {
		t.Fatalf("forget did not delete the named fact: %#v", forgotten)
	}
	var forgetContent string
	err = store.pool.QueryRow(ctx, `
SELECT content FROM observations
WHERE tenant_id = 'local' AND operation_id = 'operator-forget-v2'`).Scan(&forgetContent)
	requireNoError(t, err)
	if forgetContent != "Operator requested deletion." {
		t.Fatalf("forget observation retained free-text content: %q", forgetContent)
	}
	requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
	for _, query := range []string{"checkout_eta_v2", "Which checkout flag should the staged release use?"} {
		if got := mustSearch(t, store, resolution.ContinuityID, query); len(got) != 0 {
			t.Fatalf("deleted fact returned for %q: %#v", query, got)
		}
	}
}

func TestGovernanceRejectsCrossWorkspaceCorrectionAndReplaysSource(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewGovernanceService(store, "local")
	_, err := service.ConfirmWorkspace(ctx, "/repo/web-checkout")
	requireNoError(t, err)
	_, err = service.ConfirmWorkspace(ctx, "/repo/ops-console")
	requireNoError(t, err)
	source, err := service.AddSource(ctx, "/repo/ops-console", GovernanceWriteRequest{
		OperationID: "operator-ops-source",
		Content:     "Run ops_exception_queue_refresh before handling incidents.",
		SourceRef:   "fixture:operator:ops",
	})
	requireNoError(t, err)

	_, err = service.Correct(ctx, "/repo/web-checkout", source.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "operator-cross-scope-correction",
		Content:     "Do not cross scope.",
	})
	if err == nil {
		t.Fatal("expected cross-workspace correction to fail")
	}

	replay, err := service.AddSource(ctx, "/repo/ops-console", GovernanceWriteRequest{
		OperationID: "operator-ops-source",
		Content:     "Run ops_exception_queue_refresh before handling incidents.",
		SourceRef:   "fixture:operator:ops",
	})
	requireNoError(t, err)
	if !replay.Observation.Replayed || !replay.Memory.Replayed || replay.Memory.MemoryID != source.Memory.MemoryID {
		t.Fatalf("source replay was not idempotent: first=%#v replay=%#v", source, replay)
	}
}

func TestGovernanceSourceRevisionPreservesIndependentFacts(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewGovernanceService(store, "local")
	resolution, err := service.ConfirmWorkspace(ctx, "/repo/release-console")
	requireNoError(t, err)
	_, err = service.ConfirmWorkspace(ctx, "/repo/ops-console")
	requireNoError(t, err)

	original, err := service.AddSource(ctx, "/repo/release-console", GovernanceWriteRequest{
		OperationID: "source-command-v1",
		Content:     "Use npm run release:verify -- --legacy.",
		SourceRef:   "repo:release-manifest@v1",
	})
	requireNoError(t, err)
	timeout, err := service.AddSource(ctx, "/repo/release-console", GovernanceWriteRequest{
		OperationID: "source-timeout-v1",
		Content:     "The independent API timeout remains 800 ms.",
		SourceRef:   "repo:api-contract@v1",
	})
	requireNoError(t, err)
	distractor, err := service.AddSource(ctx, "/repo/ops-console", GovernanceWriteRequest{
		OperationID: "source-ops-v1",
		Content:     "Run ops_exception_queue_refresh before incidents.",
		SourceRef:   "repo:ops-runbook@v1",
	})
	requireNoError(t, err)

	revised, err := service.ReviseSource(ctx, "/repo/release-console", original.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "source-command-v2",
		Content:     "Use pnpm exec release:verify --mode locked.",
		SourceRef:   "repo:release-manifest@v2",
	})
	requireNoError(t, err)
	if revised.Memory.Status != "active" || revised.Memory.MemoryID == original.Memory.MemoryID {
		t.Fatalf("unexpected source revision receipt: %#v", revised)
	}

	memories, err := store.ListGovernedMemories(ctx, "local", resolution.ContinuityID)
	requireNoError(t, err)
	assertGovernedMemory(t, memories, original.Memory.MemoryID, "superseded", "")
	assertGovernedMemory(t, memories, revised.Memory.MemoryID, "active", original.Memory.MemoryID)
	assertGovernedMemory(t, memories, timeout.Memory.MemoryID, "active", "")

	assertSearchExcludes(t, store, resolution.ContinuityID, "npm run release:verify -- --legacy", "npm run release:verify -- --legacy")
	assertSearchIncludes(t, store, resolution.ContinuityID, "release verify locked", "pnpm exec release:verify --mode locked")
	assertSearchIncludes(t, store, resolution.ContinuityID, "API timeout", "800 ms")
	assertSearchExcludes(t, store, resolution.ContinuityID, "ops exception", "ops_exception_queue_refresh")

	requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
	assertSearchExcludes(t, store, resolution.ContinuityID, "npm run release:verify -- --legacy", "npm run release:verify -- --legacy")
	assertSearchIncludes(t, store, resolution.ContinuityID, "release verify locked", "pnpm exec release:verify --mode locked")
	assertSearchIncludes(t, store, resolution.ContinuityID, "API timeout", "800 ms")

	replay, err := service.ReviseSource(ctx, "/repo/release-console", original.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "source-command-v2",
		Content:     "Use pnpm exec release:verify --mode locked.",
		SourceRef:   "repo:release-manifest@v2",
	})
	requireNoError(t, err)
	if !replay.Observation.Replayed || !replay.Memory.Replayed || replay.Memory.MemoryID != revised.Memory.MemoryID {
		t.Fatalf("source revision replay changed the receipt: first=%#v replay=%#v", revised, replay)
	}

	_, err = service.ReviseSource(ctx, "/repo/release-console", timeout.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "source-command-v2",
		Content:     "Use pnpm exec release:verify --mode locked.",
		SourceRef:   "repo:release-manifest@v2",
	})
	if err == nil {
		t.Fatal("source revision accepted a conflicting replay target")
	}
	assertSearchIncludes(t, store, resolution.ContinuityID, "API timeout", "800 ms")

	_, err = service.ReviseSource(ctx, "/repo/release-console", distractor.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "source-cross-workspace-v2",
		Content:     "Do not cross workspace boundaries.",
		SourceRef:   "repo:release-manifest@v3",
	})
	if err == nil {
		t.Fatal("source revision accepted a cross-workspace target")
	}

	_, err = service.ReviseSource(ctx, "/repo/release-console", revised.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "source-missing-ref-v3",
		Content:     "Missing source authority must fail.",
	})
	if err == nil {
		t.Fatal("source revision accepted an empty source_ref")
	}

	var kind ObservationKind
	var sourceRef string
	err = store.pool.QueryRow(ctx, `
SELECT observation_kind, source_ref
FROM observations
WHERE tenant_id = 'local' AND operation_id = 'source-command-v2'`).Scan(&kind, &sourceRef)
	requireNoError(t, err)
	if kind != ObservationKindSourceUpdate || sourceRef != "repo:release-manifest@v2" {
		t.Fatalf("source revision lost source authority: kind=%s source_ref=%s", kind, sourceRef)
	}
}

func assertGovernedMemory(t *testing.T, memories []GovernedMemory, id, status, supersedes string) {
	t.Helper()
	for _, memory := range memories {
		if memory.ID == id {
			if memory.LifecycleStatus != status || memory.SupersedesMemoryID != supersedes {
				t.Fatalf("unexpected governed memory state: %#v", memory)
			}
			return
		}
	}
	t.Fatalf("governed memory %s not found: %#v", id, memories)
}

func assertSearchIncludes(t *testing.T, store *Store, continuityID, query, expected string) {
	t.Helper()
	for _, memory := range mustSearch(t, store, continuityID, query) {
		if strings.Contains(memory.Content, expected) {
			return
		}
	}
	t.Fatalf("search %q did not include %q", query, expected)
}

func assertSearchExcludes(t *testing.T, store *Store, continuityID, query, forbidden string) {
	t.Helper()
	for _, memory := range mustSearch(t, store, continuityID, query) {
		if strings.Contains(memory.Content, forbidden) {
			t.Fatalf("search %q returned forbidden content %q: %#v", query, forbidden, memory)
		}
	}
}
