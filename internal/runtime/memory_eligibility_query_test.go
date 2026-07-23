package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestMemoryEligibilityLexicalBoundary(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	asOf := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)

	workspaceID := createEligibilityContinuity(t, store, "eligibility-query", "workspace")
	conversationID := createEligibilityContinuity(t, store, "eligibility-query", "conversation")

	wantWorkspace := seedEligibilityBoundarySet(t, store, "eligibility-query", workspaceID, asOf, "workspace")
	wantConversation := seedEligibilityBoundarySet(t, store, "eligibility-query", conversationID, asOf, "conversation")

	workspace, err := store.SearchEligibleMemoryAt(
		ctx, "eligibility-query", workspaceID, "ELIGIBILITY-BOUNDARY", 20, asOf,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, workspace, wantWorkspace)

	conversation, err := store.SearchEligibleConversationMemoryAt(
		ctx, "eligibility-query", conversationID, "ELIGIBILITY-BOUNDARY", 20, asOf,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, conversation, wantConversation)
}

func TestGlobalDefaultsEligibilityBoundary(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	asOf := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	continuityID := createEligibilityContinuity(t, store, "eligibility-defaults", "global_defaults")
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: "eligibility-defaults", ContinuityID: continuityID,
		Kind: "global_default", Key: "reply_language", Lifecycle: "active",
		Content: "ELIGIBILITY-DEFAULT-CURRENT use Chinese replies.",
	})
	future := asOf.Add(time.Microsecond)
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: "eligibility-defaults", ContinuityID: continuityID,
		Kind: "global_default", Key: "future_default", Lifecycle: "active",
		Content: "ELIGIBILITY-DEFAULT-FUTURE use English replies.", ValidFrom: &future,
	})
	expires := asOf
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: "eligibility-defaults", ContinuityID: continuityID,
		Kind: "global_default", Key: "expired_default", Lifecycle: "active",
		Content: "ELIGIBILITY-DEFAULT-EXPIRED use French replies.", ValidUntil: &expires,
	})

	defaults, err := store.ListEligibleGlobalDefaultsAt(ctx, "eligibility-defaults", continuityID, asOf)
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, defaults, []string{currentID})
}

func TestRetrievalEligibilityModes(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-retrieval"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	asOf := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-VECTOR current verification guidance",
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-VECTOR expired stale guidance", ValidUntil: &asOf,
	})

	embedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	worker := mustProjectionWorker(t, store, embedder, tenantID, 8)
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var vectorRows int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, ProductionRetrievalProfileID).Scan(&vectorRows); err != nil {
		t.Fatal(err)
	}
	if vectorRows != 2 {
		t.Fatalf("projection did not preserve active future/expired storage: rows=%d", vectorRows)
	}

	coordinator := mustRetrievalCoordinator(t, store, embedder)
	for _, mode := range []RetrievalMode{RetrievalVector, RetrievalShadow} {
		result, err := coordinator.Retrieve(ctx, RetrievalRequest{
			OperationID: "eligibility-retrieval-" + string(mode),
			TenantID:    tenantID, ContinuityIDs: []string{continuityID},
			Query: "ELIGIBILITY-VECTOR guidance", Limit: 5, Mode: mode,
			EligibilityAsOf: asOf,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertMemoryIDSet(t, result.Memories, []string{currentID})
		if !result.EligibilityAsOf.Equal(asOf) {
			t.Fatalf("result eligibility_as_of=%s want %s", result.EligibilityAsOf, asOf)
		}
		var stored time.Time
		if err := store.pool.QueryRow(ctx, `
SELECT eligibility_as_of
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, "eligibility-retrieval-"+string(mode)).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !stored.Equal(asOf) {
			t.Fatalf("stored eligibility_as_of=%s want %s", stored, asOf)
		}
	}

	failing := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{err: errors.New("provider unavailable")})
	degraded, err := failing.Retrieve(ctx, RetrievalRequest{
		OperationID: "eligibility-retrieval-degraded",
		TenantID:    tenantID, ContinuityIDs: []string{continuityID},
		Query: "ELIGIBILITY-VECTOR guidance", Limit: 5, Mode: RetrievalVector,
		EligibilityAsOf: asOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, degraded.Memories, []string{currentID})
	if !degraded.Degraded || degraded.Effective != RetrievalLexical || !degraded.EligibilityAsOf.Equal(asOf) {
		t.Fatalf("unexpected degraded result: %#v", degraded)
	}
}

func TestConversationEligibilitySnapshot(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-context-snapshot"
	defaultsContinuityID := createEligibilityContinuity(t, store, tenantID, "global_defaults")
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: defaultsContinuityID,
		Kind: "global_default", Key: "reply_language", Lifecycle: "active",
		Content: "Reply in Chinese unless the current request requires another language.",
	})

	t.Run("workspace", func(t *testing.T) {
		repoRoot := "/fixtures/eligibility-context-snapshot"
		continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		retriever := &recordingMemoryRetriever{result: RetrievalResult{Memories: []Memory{{
			ID: "11111111-1111-1111-1111-111111111111", Content: "Use the current governed deployment procedure.",
		}}}}
		prepared, err := NewServiceWithRetriever(store, tenantID, retriever).PrepareContext(ctx, PrepareContextRequest{
			OperationID: "eligibility-workspace-snapshot",
			Workspace:   WorkspaceAnchor{RepoRoot: repoRoot},
			Task:        "How should deployment run?",
			MaxItems:    4,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertEligibilityDeliverySnapshot(t, store, tenantID, prepared.DeliveryID, retriever, continuityID)
		for _, expected := range []string{"Reply in Chinese", "governed deployment procedure"} {
			if !strings.Contains(prepared.Context, expected) {
				t.Fatalf("workspace context omitted %q: %s", expected, prepared.Context)
			}
		}
		for _, forbidden := range []string{"eligibility_as_of", "valid_from", "valid_until"} {
			if strings.Contains(prepared.Context, forbidden) {
				t.Fatalf("workspace context exposed technical field %q: %s", forbidden, prepared.Context)
			}
		}
	})

	t.Run("conversation", func(t *testing.T) {
		anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "eligibility-context-snapshot"}
		resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
		if err != nil {
			t.Fatal(err)
		}
		retriever := &recordingMemoryRetriever{result: RetrievalResult{Memories: []Memory{{
			ID: "22222222-2222-2222-2222-222222222222", Content: "The current appointment is Saturday at 10:00.",
		}}}}
		prepared, err := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{
			Retriever: retriever, MemoryLimit: 4,
		}).PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
			OperationID: "eligibility-conversation-snapshot",
			Anchor:      anchor,
			Message:     "When is the appointment?",
		})
		if err != nil {
			t.Fatal(err)
		}
		assertEligibilityDeliverySnapshot(t, store, tenantID, prepared.DeliveryID, retriever, resolution.ContinuityID)
		for _, expected := range []string{"Reply in Chinese", "Saturday at 10:00"} {
			if !strings.Contains(prepared.Context, expected) {
				t.Fatalf("conversation context omitted %q: %s", expected, prepared.Context)
			}
		}
	})
}

func TestBridgeEligibility(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-bridge"
	snapshot, err := store.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := snapshot.AsOf.Add(-time.Microsecond)
	futureAt := snapshot.AsOf.Add(time.Hour)

	conversationID := createEligibilityContinuity(t, store, tenantID, "conversation")
	linkedConversationID := createEligibilityContinuity(t, store, tenantID, "conversation")
	workspaceID := createEligibilityContinuity(t, store, tenantID, "workspace")
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: conversationID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-BRIDGE current release procedure",
	})
	expiredID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: conversationID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-BRIDGE expired release procedure", ValidUntil: &expiredAt,
	})
	futureID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: conversationID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-BRIDGE future release procedure", ValidFrom: &futureAt,
	})
	archivedID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: conversationID, Kind: "fact", Lifecycle: "archived",
		Content: "ELIGIBILITY-BRIDGE archived release procedure",
	})

	promoted, err := store.PromoteConversationMemory(
		ctx, tenantID, "eligibility-bridge-promote-current", conversationID, workspaceID,
		"conversation:release", "/fixtures/eligibility-bridge", []string{currentID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(promoted.MemoryEffects) != 1 || promoted.MemoryEffects[0].SourceMemoryID != currentID {
		t.Fatalf("unexpected current promotion: %#v", promoted)
	}
	for index, memoryID := range []string{expiredID, futureID, archivedID} {
		if _, err := store.PromoteConversationMemory(
			ctx, tenantID, fmt.Sprintf("eligibility-bridge-promote-ineligible-%d", index),
			conversationID, workspaceID, "conversation:release", "/fixtures/eligibility-bridge",
			[]string{memoryID},
		); err == nil {
			t.Fatalf("ineligible memory %s was promoted", memoryID)
		}
	}

	exported, err := store.ExportWorkspaceMemory(
		ctx, tenantID, "eligibility-bridge-export-current", conversationID,
		"conversation:release", []string{currentID}, "Release context", "web_chat",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.ExportBody, "current release procedure") {
		t.Fatalf("current export omitted selected content: %#v", exported)
	}
	for index, memoryID := range []string{expiredID, futureID, archivedID} {
		if _, err := store.ExportWorkspaceMemory(
			ctx, tenantID, fmt.Sprintf("eligibility-bridge-export-ineligible-%d", index),
			conversationID, "conversation:release", []string{memoryID}, "Release context", "web_chat",
		); err == nil {
			t.Fatalf("ineligible memory %s was exported", memoryID)
		}
	}

	if _, err := store.pool.Exec(ctx, `
UPDATE governed_memories SET valid_until = $1 WHERE id = $2::uuid`, snapshot.AsOf, currentID); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ExportWorkspaceMemory(
		ctx, tenantID, "eligibility-bridge-export-current", conversationID,
		"conversation:release", []string{currentID}, "Release context", "web_chat",
	)
	if err != nil {
		t.Fatalf("persisted export replay reevaluated expired source memory: %v", err)
	}
	if !replayed.Replayed || replayed.ID != exported.ID || replayed.ExportBody != exported.ExportBody {
		t.Fatalf("export replay changed persisted result: first=%#v replay=%#v", exported, replayed)
	}

	linkedCurrentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: linkedConversationID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-LINK current linked procedure",
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: linkedConversationID, Kind: "fact", Lifecycle: "active",
		Content: "ELIGIBILITY-LINK expired linked procedure", ValidUntil: &expiredAt,
	})
	if _, err := store.LinkConversationContinuities(
		ctx, tenantID, "eligibility-bridge-link", conversationID, linkedConversationID,
		"conversation:release", "conversation:release-linked",
	); err != nil {
		t.Fatal(err)
	}
	linked, err := store.SearchEligibleConversationMemoryAt(
		ctx, tenantID, conversationID, "ELIGIBILITY-LINK procedure", 10, snapshot.AsOf,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertMemoryIDSet(t, linked, []string{linkedCurrentID})

	history, err := store.ListGovernedMemories(ctx, tenantID, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsGovernedMemory(history, expiredID, "ELIGIBILITY-BRIDGE expired release procedure") ||
		!containsGovernedMemory(history, archivedID, "ELIGIBILITY-BRIDGE archived release procedure") {
		t.Fatalf("authorized history lost expired/archive content: %#v", history)
	}
}

func TestSourceEligibility(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "eligibility-source"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	snapshot, err := store.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := snapshot.AsOf.Add(-time.Microsecond)
	futureAt := snapshot.AsOf.Add(time.Hour)
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Key: "release.current",
		Lifecycle: "active", Content: "ELIGIBILITY-SOURCE current release procedure",
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Key: "release.expired",
		Lifecycle: "active", Content: "ELIGIBILITY-SOURCE expired release procedure", ValidUntil: &expiredAt,
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Key: "release.future",
		Lifecycle: "active", Content: "ELIGIBILITY-SOURCE future release procedure", ValidFrom: &futureAt,
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Key: "release.archived",
		Lifecycle: "archived", Content: "ELIGIBILITY-SOURCE archived release procedure",
	})

	current, err := store.ListActiveMemoriesByKey(ctx, tenantID, continuityID, "release.current", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].ID != currentID {
		t.Fatalf("current keyed fact missing: %#v", current)
	}
	for _, key := range []string{"release.expired", "release.future", "release.archived"} {
		memories, err := store.ListActiveMemoriesByKey(ctx, tenantID, continuityID, key, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(memories) != 0 {
			t.Fatalf("ineligible key %s entered current source candidates: %#v", key, memories)
		}
	}

	matched, err := store.BeginSourceMatch(ctx, tenantID, continuityID, SourceMatchBeginRequest{
		OperationID: "eligibility-source-match", SourceRef: "fixture:eligibility-source-match",
		SourceContent: "The release procedure changed.", ProviderName: "test-provider", RequestedModel: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateIDs(t, matched.CandidateSet, []string{currentID})

	formation, err := store.BeginSourceFormation(
		ctx, tenantID, continuityID, sourceFormationBeginRequest("eligibility-source-formation", sourceFormationTestDocument),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateIDs(t, formation.ActiveSnapshot, []string{currentID})
}

type eligibilityMemorySeed struct {
	TenantID     string
	ContinuityID string
	Kind         string
	Key          string
	Lifecycle    string
	Content      string
	ValidFrom    *time.Time
	ValidUntil   *time.Time
}

func createEligibilityContinuity(t *testing.T, store *Store, tenantID, line string) string {
	t.Helper()
	var continuityID string
	if err := store.pool.QueryRow(context.Background(), `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ($1, $2, 'active')
RETURNING id::text`, tenantID, line).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	return continuityID
}

func seedEligibilityBoundarySet(t *testing.T, store *Store, tenantID, continuityID string, asOf time.Time, prefix string) []string {
	t.Helper()
	before := asOf.Add(-time.Microsecond)
	after := asOf.Add(time.Microsecond)
	currentID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s current", prefix),
	})
	startsNowID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s starts now", prefix), ValidFrom: &asOf,
	})
	endsAfterID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s ends after", prefix), ValidFrom: &before, ValidUntil: &after,
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s future", prefix), ValidFrom: &after,
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s expires now", prefix), ValidUntil: &asOf,
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "archived",
		Content: fmt.Sprintf("ELIGIBILITY-BOUNDARY %s archived", prefix),
	})
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "deleted",
		Content: "[redacted]",
	})
	return []string{currentID, startsNowID, endsAfterID}
}

func seedEligibilityMemory(t *testing.T, store *Store, seed eligibilityMemorySeed) string {
	t.Helper()
	operationID := fmt.Sprintf("eligibility-seed-%s-%s-%d", seed.TenantID, seed.Lifecycle, time.Now().UnixNano())
	var memoryID string
	if err := store.pool.QueryRow(context.Background(), `
WITH observation AS (
  INSERT INTO observations (
    tenant_id, continuity_id, operation_id, observation_kind, content, source_ref, memory_key
  ) VALUES (
    $1, $2::uuid, $3, 'source_update', $4, 'fixture:memory-eligibility', $5
  )
  RETURNING id
)
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, memory_key,
  lifecycle_status, content, valid_from, valid_until
)
SELECT $1, $2::uuid, id, $6, $5, $7, $4, $8, $9
FROM observation
RETURNING id::text`,
		seed.TenantID, seed.ContinuityID, operationID, seed.Content, seed.Key,
		seed.Kind, seed.Lifecycle, seed.ValidFrom, seed.ValidUntil,
	).Scan(&memoryID); err != nil {
		t.Fatal(err)
	}
	if seed.Lifecycle == "active" && seed.Content != "[redacted]" {
		if _, err := store.pool.Exec(context.Background(), `
INSERT INTO memory_search_documents (
  memory_id, tenant_id, continuity_id, content, search_document
) VALUES (
  $1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4)
)`, memoryID, seed.TenantID, seed.ContinuityID, seed.Content); err != nil {
			t.Fatal(err)
		}
	}
	return memoryID
}

func assertMemoryIDSet(t *testing.T, memories []Memory, want []string) {
	t.Helper()
	got := make([]string, len(memories))
	for index, memory := range memories {
		got[index] = memory.ID
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("memory IDs=%v want %v", got, want)
	}
}

func assertEligibilityDeliverySnapshot(t *testing.T, store *Store, tenantID, deliveryID string, retriever *recordingMemoryRetriever, continuityID string) {
	t.Helper()
	if len(retriever.requests) != 1 {
		t.Fatalf("retriever calls=%d", len(retriever.requests))
	}
	request := retriever.requests[0]
	if request.EligibilityAsOf.IsZero() {
		t.Fatal("retrieval request omitted eligibility_as_of")
	}
	if len(request.ContinuityIDs) == 0 || !containsString(request.ContinuityIDs, continuityID) {
		t.Fatalf("retrieval request scope=%v want continuity %s", request.ContinuityIDs, continuityID)
	}
	var stored time.Time
	if err := store.pool.QueryRow(context.Background(), `
SELECT eligibility_as_of
FROM memory_deliveries
WHERE tenant_id = $1 AND id = $2::uuid`, tenantID, deliveryID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !stored.Equal(request.EligibilityAsOf) {
		t.Fatalf("delivery eligibility_as_of=%s retrieval=%s", stored, request.EligibilityAsOf)
	}
}

func containsGovernedMemory(memories []GovernedMemory, memoryID, content string) bool {
	for _, memory := range memories {
		if memory.ID == memoryID && memory.Content == content {
			return true
		}
	}
	return false
}

func assertSourceCandidateIDs(t *testing.T, candidates []SourceMatchCandidate, want []string) {
	t.Helper()
	got := make([]string, len(candidates))
	for index, candidate := range candidates {
		got[index] = candidate.MemoryID
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("source candidate IDs=%v want %v", got, want)
	}
}
