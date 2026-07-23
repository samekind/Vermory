package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var runtimeSeedSequence atomic.Int64

func TestPrepareContextExcludesOtherWorkspaceAndSupersededFacts(t *testing.T) {
	service, store, webCheckout, opsConsole := seededService(t)
	seedMemory(t, store, webCheckout, "superseded", "The current checkout flag is checkout_eta_v1.")
	seedMemory(t, store, webCheckout, "active", "The current checkout flag is checkout_eta_v2.")
	seedMemory(t, store, opsConsole, "active", "Run ops_exception_queue_refresh before handling incidents.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	requireNoError(t, store.RebuildProjection(context.Background(), "local", opsConsole))

	got, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-current-checkout",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Find the current checkout flag.",
		MaxItems:    5,
	})
	requireNoError(t, err)
	if got.Status != ResolutionResolved {
		t.Fatalf("expected resolved context, got %#v", got)
	}
	if got.DeliveryID == "" {
		t.Fatalf("expected delivery receipt, got %#v", got)
	}
	requireContains(t, got.Context, "checkout_eta_v2")
	requireNotContains(t, got.Context, "checkout_eta_v1")
	requireNotContains(t, got.Context, "ops_exception_queue_refresh")
}

func TestPrepareContextFindsCurrentFactWhenTaskHasPartialTermOverlap(t *testing.T) {
	service, store, webCheckout, _ := seededService(t)
	seedMemory(t, store, webCheckout, "active", "Use checkout_eta_v2 for the staged checkout release.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))

	got, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-partial-term-overlap",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Create the W02 Grok client artifact using the governed current checkout flag.",
	})
	requireNoError(t, err)
	requireContains(t, got.Context, "checkout_eta_v2")
}

func TestPrepareContextUsesInjectedRetrieverWithoutExposingRetrievalMetadata(t *testing.T) {
	_, store, continuityID, _ := seededService(t)
	retriever := &recordingMemoryRetriever{result: RetrievalResult{
		Memories:  []Memory{{ID: "11111111-1111-1111-1111-111111111111", Content: "Semantic rollback approval requires two maintainers."}},
		Effective: RetrievalVector,
		AuditID:   "22222222-2222-2222-2222-222222222222",
	}}
	service := NewServiceWithRetriever(store, "local", retriever)
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "workspace-semantic-prepare",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Who approves rollback?",
		MaxItems:    4,
	})
	requireNoError(t, err)
	if len(retriever.requests) != 1 {
		t.Fatalf("retriever calls=%d", len(retriever.requests))
	}
	request := retriever.requests[0]
	if request.OperationID != "workspace-retrieval:workspace-semantic-prepare" || request.TenantID != "local" || request.Query != "Who approves rollback?" || request.Limit != 4 {
		t.Fatalf("unexpected workspace retrieval request: %#v", request)
	}
	if len(request.ContinuityIDs) != 1 || request.ContinuityIDs[0] != continuityID {
		t.Fatalf("unexpected workspace retrieval scope: %#v", request.ContinuityIDs)
	}
	requireContains(t, prepared.Context, "Semantic rollback approval requires two maintainers.")
	for _, internal := range []string{"vector", "22222222-2222-2222-2222-222222222222", "audit"} {
		requireNotContains(t, prepared.Context, internal)
	}
}

type recordingMemoryRetriever struct {
	requests []RetrievalRequest
	result   RetrievalResult
	err      error
}

func (retriever *recordingMemoryRetriever) Retrieve(_ context.Context, request RetrievalRequest) (RetrievalResult, error) {
	retriever.requests = append(retriever.requests, request)
	return retriever.result, retriever.err
}

func TestWorkspaceConsumerReceivesGlobalDefaultsWithoutChangingDeliveryScope(t *testing.T) {
	ctx := context.Background()
	service, store, workspaceContinuityID, _ := seededService(t)
	defaults := NewGlobalDefaultsService(store, "local")
	created, err := defaults.Set(ctx, SetGlobalDefaultRequest{
		OperationID: "workspace-global-language-set",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese unless the active task explicitly requests another language.",
	})
	requireNoError(t, err)
	seedMemory(t, store, workspaceContinuityID, "active", "Use checkout_eta_v2 for the staged checkout release.")
	requireNoError(t, store.RebuildProjection(ctx, "local", workspaceContinuityID))

	prepared, err := service.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "workspace-global-language-prepare",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Explain the current checkout flag.",
	})
	requireNoError(t, err)
	requireContains(t, prepared.Context, "Global defaults:\nDefault user-facing replies to Chinese")
	requireContains(t, prepared.Context, "Governed memory:\nUse checkout_eta_v2")
	deliveryContinuityID, err := store.DeliveryContinuity(ctx, "local", prepared.DeliveryID)
	requireNoError(t, err)
	if deliveryContinuityID != workspaceContinuityID || deliveryContinuityID == created.ContinuityID {
		t.Fatalf("workspace delivery attached to the wrong continuity: delivery=%s workspace=%s global=%s", deliveryContinuityID, workspaceContinuityID, created.ContinuityID)
	}
	loadedDelivery, err := store.LookupDelivery(ctx, "local", prepared.DeliveryID)
	requireNoError(t, err)
	if loadedDelivery.DeliveryID != prepared.DeliveryID || loadedDelivery.Context != prepared.Context || loadedDelivery.EligibilityAsOf.IsZero() {
		t.Fatalf("unexpected loaded delivery: %#v", loadedDelivery)
	}
	if _, err := store.LookupDelivery(ctx, "other", prepared.DeliveryID); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-tenant delivery lookup was accepted: %v", err)
	}

	_, err = defaults.Forget(ctx, ForgetGlobalDefaultRequest{
		OperationID: "workspace-global-language-forget",
		MemoryID:    created.MemoryID,
	})
	requireNoError(t, err)
	prepared, err = service.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "workspace-global-language-after-forget",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Explain the current checkout flag again.",
	})
	requireNoError(t, err)
	requireNotContains(t, prepared.Context, "Default user-facing replies to Chinese")
	requireContains(t, prepared.Context, "checkout_eta_v2")
}

func TestDeletedMemoryDoesNotReturnAfterRebuild(t *testing.T) {
	_, store, webCheckout, _ := seededService(t)
	memoryID := seedMemory(t, store, webCheckout, "active", "The Orchard recovery code is ORCHID-7419.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	if got := mustSearch(t, store, webCheckout, "ORCHID-7419"); len(got) != 1 {
		t.Fatalf("expected the active memory before deletion, got %#v", got)
	}

	requireNoError(t, store.DeleteMemory(context.Background(), "local", webCheckout, memoryID))
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	if got := mustSearch(t, store, webCheckout, "ORCHID-7419"); len(got) != 0 {
		t.Fatalf("deleted memory returned after rebuild: %#v", got)
	}

	var memoryContent, observationContent string
	err := store.pool.QueryRow(context.Background(), `
SELECT m.content, o.content
FROM governed_memories m
JOIN observations o ON o.id = m.origin_observation_id
WHERE m.id = $1::uuid`, memoryID).Scan(&memoryContent, &observationContent)
	requireNoError(t, err)
	if memoryContent != "[redacted]" || observationContent != "[redacted]" {
		t.Fatalf("deleted authority content was not redacted: memory=%q observation=%q", memoryContent, observationContent)
	}
}

func TestSearchActiveMemoryRechecksLifecycleAfterProjectionLookup(t *testing.T) {
	_, store, webCheckout, _ := seededService(t)
	memoryID := seedMemory(t, store, webCheckout, "active", "Use checkout_eta_v2 for the release.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))

	_, err := store.pool.Exec(context.Background(), `
UPDATE governed_memories SET lifecycle_status = 'deleted' WHERE id = $1::uuid`, memoryID)
	requireNoError(t, err)
	if got := mustSearch(t, store, webCheckout, "checkout_eta_v2"); len(got) != 0 {
		t.Fatalf("stale projection bypassed lifecycle check: %#v", got)
	}
}

func TestSearchActiveMemoryPrefersExactIdentifier(t *testing.T) {
	_, store, webCheckout, _ := seededService(t)
	seedMemory(t, store, webCheckout, "active", "The checkout flag controls the staged rollout.")
	seedMemory(t, store, webCheckout, "active", "Use checkout_eta_v2 for the staged checkout rollout.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))

	got := mustSearch(t, store, webCheckout, "checkout_eta_v2")
	if len(got) == 0 || !strings.Contains(got[0].Content, "checkout_eta_v2") {
		t.Fatalf("exact identifier was not ranked first: %#v", got)
	}
}

func TestCommitObservationKeepsAgentResultProposed(t *testing.T) {
	service, store, webCheckout, _ := seededService(t)
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-agent-result",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Continue checkout work.",
	})
	requireNoError(t, err)

	got, err := service.CommitObservation(context.Background(), CommitObservationRequest{
		OperationID: "writeback-agent-result",
		DeliveryID:  prepared.DeliveryID,
		Kind:        ObservationKindAgentResult,
		Content:     "The checkout flag should be checkout_eta_redis.",
	})
	requireNoError(t, err)
	if got.MemoryStatus != "proposed" {
		t.Fatalf("agent output must remain proposed, got %#v", got)
	}
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	if matches := mustSearch(t, store, webCheckout, "checkout_eta_redis"); len(matches) != 0 {
		t.Fatalf("proposed agent output became searchable authority: %#v", matches)
	}
}

func TestCommitObservationReplaysWithoutDuplicatingGovernedMemory(t *testing.T) {
	service, store, webCheckout, _ := seededService(t)
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-idempotent-writeback",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Continue checkout work.",
	})
	requireNoError(t, err)
	request := CommitObservationRequest{
		OperationID: "writeback-idempotent",
		DeliveryID:  prepared.DeliveryID,
		Kind:        ObservationKindAgentResult,
		Content:     "The checkout test suite passed.",
	}
	first, err := service.CommitObservation(context.Background(), request)
	requireNoError(t, err)
	second, err := service.CommitObservation(context.Background(), request)
	requireNoError(t, err)
	if first.MemoryID != second.MemoryID || !second.Replayed {
		t.Fatalf("expected idempotent governed memory receipt, first=%#v second=%#v", first, second)
	}

	var count int
	err = store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM governed_memories
WHERE tenant_id = 'local' AND continuity_id = $1::uuid`, webCheckout).Scan(&count)
	requireNoError(t, err)
	if count != 1 {
		t.Fatalf("expected one governed memory after replay, got %d", count)
	}
}

func TestRejectedGovernanceRollsBackObservation(t *testing.T) {
	service, store, _, _ := seededService(t)
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-atomic-governance",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Correct checkout configuration.",
	})
	requireNoError(t, err)

	_, err = service.CommitObservation(context.Background(), CommitObservationRequest{
		OperationID:        "writeback-rejected-governance",
		DeliveryID:         prepared.DeliveryID,
		Kind:               ObservationKindUserCorrection,
		Content:            "The current checkout flag is checkout_eta_v2.",
		SupersedesMemoryID: "fd6462d9-86da-49b5-a014-7dceceeeeb82",
	})
	if err == nil {
		t.Fatal("expected governance failure for an unknown superseded memory")
	}

	var count int
	err = store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM observations
WHERE tenant_id = 'local' AND operation_id = 'writeback-rejected-governance'`).Scan(&count)
	requireNoError(t, err)
	if count != 0 {
		t.Fatalf("rejected governance left an observation behind: %d", count)
	}
}

func TestUserCorrectionSupersedesOnlyNamedActiveMemory(t *testing.T) {
	service, store, webCheckout, _ := seededService(t)
	oldMemoryID := seedMemory(t, store, webCheckout, "active", "The current checkout flag is checkout_eta_v1.")
	otherMemoryID := seedMemory(t, store, webCheckout, "active", "The current inventory flag is inventory_eta_v1.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-user-correction",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Update the checkout flag.",
	})
	requireNoError(t, err)

	got, err := service.CommitObservation(context.Background(), CommitObservationRequest{
		OperationID:        "writeback-user-correction",
		DeliveryID:         prepared.DeliveryID,
		Kind:               ObservationKindUserCorrection,
		Content:            "The current checkout flag is checkout_eta_v2.",
		SupersedesMemoryID: oldMemoryID,
	})
	requireNoError(t, err)
	if got.MemoryStatus != "active" {
		t.Fatalf("user correction must become active, got %#v", got)
	}

	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	checkout := mustSearch(t, store, webCheckout, "checkout_eta_v1")
	if len(checkout) == 0 || !strings.Contains(checkout[0].Content, "checkout_eta_v2") {
		t.Fatalf("current checkout fact was not returned: %#v", checkout)
	}
	for _, memory := range checkout {
		requireNotContains(t, memory.Content, "checkout_eta_v1")
	}
	if inventory := mustSearch(t, store, webCheckout, "inventory_eta_v1"); len(inventory) != 1 || inventory[0].ID != otherMemoryID {
		t.Fatalf("unrelated active fact was modified: %#v", inventory)
	}
}

func TestForgetRequestDeletesNamedMemory(t *testing.T) {
	service, store, webCheckout, _ := seededService(t)
	memoryID := seedMemory(t, store, webCheckout, "active", "The recovery code is ORCHID-7419.")
	requireNoError(t, store.RebuildProjection(context.Background(), "local", webCheckout))
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "prepare-forget-request",
		Workspace:   WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
		Task:        "Forget the recovery code.",
	})
	requireNoError(t, err)

	got, err := service.CommitObservation(context.Background(), CommitObservationRequest{
		OperationID:    "writeback-forget-request",
		DeliveryID:     prepared.DeliveryID,
		Kind:           ObservationKindForgetRequest,
		Content:        "Forget the recovery code.",
		TargetMemoryID: memoryID,
	})
	requireNoError(t, err)
	if got.MemoryStatus != "deleted" {
		t.Fatalf("forget request did not delete the named memory: %#v", got)
	}
	if matches := mustSearch(t, store, webCheckout, "ORCHID-7419"); len(matches) != 0 {
		t.Fatalf("forgotten fact returned: %#v", matches)
	}
}

func seededService(t *testing.T) (*Service, *Store, string, string) {
	t.Helper()
	store := openTestStore(t)
	ctx := context.Background()
	webCheckout, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/web-checkout")
	requireNoError(t, err)
	opsConsole, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/ops-console")
	requireNoError(t, err)
	return NewService(store, "local"), store, webCheckout, opsConsole
}

func seedMemory(t *testing.T, store *Store, continuityID, lifecycleStatus, content string) string {
	t.Helper()
	ctx := context.Background()
	operationID := fmt.Sprintf("seed-%d", runtimeSeedSequence.Add(1))
	var memoryID string
	err := store.pool.QueryRow(ctx, `
WITH observation AS (
  INSERT INTO observations (tenant_id, continuity_id, operation_id, observation_kind, content)
  VALUES ('local', $1::uuid, $2, 'source_update', $3)
  RETURNING id
)
INSERT INTO governed_memories (tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content)
SELECT 'local', $1::uuid, id, 'fact', $4, $3 FROM observation
RETURNING id::text`, continuityID, operationID, content, lifecycleStatus).Scan(&memoryID)
	requireNoError(t, err)
	return memoryID
}

func mustSearch(t *testing.T, store *Store, continuityID, query string) []Memory {
	t.Helper()
	got, err := store.SearchActiveMemory(context.Background(), "local", continuityID, query, 10)
	requireNoError(t, err)
	return got
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireContains(t *testing.T, value, want string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}

func requireNotContains(t *testing.T, value, forbidden string) {
	t.Helper()
	if strings.Contains(value, forbidden) {
		t.Fatalf("expected %q not to contain %q", value, forbidden)
	}
}
