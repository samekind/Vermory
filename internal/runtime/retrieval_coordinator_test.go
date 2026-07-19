package runtime

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRetrievalRequestNormalizesFrozenModesAndScope(t *testing.T) {
	request := RetrievalRequest{
		OperationID:   " retrieval-op ",
		TenantID:      " tenant-a ",
		ContinuityIDs: []string{"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		Query:         " semantic release decision ",
		Limit:         0,
		Mode:          RetrievalShadow,
	}
	normalized, err := request.normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.OperationID != "retrieval-op" || normalized.TenantID != "tenant-a" || normalized.Query != "semantic release decision" {
		t.Fatalf("request strings were not normalized: %#v", normalized)
	}
	if normalized.Limit != defaultContextItems {
		t.Fatalf("default limit=%d want %d", normalized.Limit, defaultContextItems)
	}
	wantIDs := []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}
	if !reflect.DeepEqual(normalized.ContinuityIDs, wantIDs) {
		t.Fatalf("continuity IDs=%#v want %#v", normalized.ContinuityIDs, wantIDs)
	}

	request.Limit = maxContextItems + 10
	normalized, err = request.normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Limit != maxContextItems {
		t.Fatalf("capped limit=%d want %d", normalized.Limit, maxContextItems)
	}
}

func TestRetrievalRequestRejectsInvalidModesAndAmbiguousInputs(t *testing.T) {
	valid := RetrievalRequest{
		OperationID:   "retrieval-op",
		TenantID:      "tenant-a",
		ContinuityIDs: []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		Query:         "release decision",
		Limit:         5,
		Mode:          RetrievalVector,
	}
	for name, mutate := range map[string]func(*RetrievalRequest){
		"mode":       func(request *RetrievalRequest) { request.Mode = "hybrid" },
		"operation":  func(request *RetrievalRequest) { request.OperationID = "" },
		"tenant":     func(request *RetrievalRequest) { request.TenantID = "" },
		"continuity": func(request *RetrievalRequest) { request.ContinuityIDs = nil },
		"duplicate": func(request *RetrievalRequest) {
			request.ContinuityIDs = append(request.ContinuityIDs, request.ContinuityIDs[0])
		},
		"query":          func(request *RetrievalRequest) { request.Query = "" },
		"negative limit": func(request *RetrievalRequest) { request.Limit = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			request := valid
			request.ContinuityIDs = append([]string(nil), valid.ContinuityIDs...)
			mutate(&request)
			if _, err := request.normalized(); err == nil {
				t.Fatalf("invalid request was accepted: %#v", request)
			}
		})
	}

	lexical := valid
	lexical.Mode = RetrievalLexical
	lexical.OperationID = ""
	if _, err := lexical.normalized(); err != nil {
		t.Fatalf("lexical retrieval unexpectedly required an audit operation ID: %v", err)
	}
}

func TestResolveLinkedConversationContinuityIDsUsesDurableLinkAuthority(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-linked-scope"
	primaryAnchor := ConversationAnchor{Channel: "openclaw_dm", ThreadID: "release-primary"}
	childAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "release-child"}
	siblingAnchor := ConversationAnchor{Channel: "email_forward", ThreadID: "release-sibling"}
	unrelatedAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "unrelated"}
	primary, err := store.ResolveOrCreateConversation(ctx, tenantID, primaryAnchor)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.ResolveOrCreateConversation(ctx, tenantID, childAnchor)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := store.ResolveOrCreateConversation(ctx, tenantID, siblingAnchor)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := store.ResolveOrCreateConversation(ctx, tenantID, unrelatedAnchor)
	if err != nil {
		t.Fatal(err)
	}
	bridges := NewBridgeService(store, tenantID)
	if _, err := bridges.LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "retrieval-link-child",
		Primary:     primaryAnchor,
		Linked:      childAnchor,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridges.LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "retrieval-link-sibling",
		Primary:     primaryAnchor,
		Linked:      siblingAnchor,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.ResolveLinkedConversationContinuityIDs(ctx, tenantID, child.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{primary.ContinuityID, child.ContinuityID, sibling.ContinuityID}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("linked scope=%#v want %#v", got, want)
	}

	got, err = store.ResolveLinkedConversationContinuityIDs(ctx, tenantID, unrelated.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{unrelated.ContinuityID}) {
		t.Fatalf("unlinked scope=%#v", got)
	}

	workspaceID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, "/fixtures/retrieval-linked-workspace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveLinkedConversationContinuityIDs(ctx, tenantID, workspaceID); err == nil {
		t.Fatal("workspace continuity was accepted as a conversation scope")
	}
}

func TestRetrievalCoordinatorShadowPreservesLexicalIDsAndOrder(t *testing.T) {
	store, continuityID, memories := seedCoordinatorWorkspace(t, "retrieval-shadow")
	insertCurrentCursor(t, store, "retrieval-shadow")
	insertVectorDocument(t, store, "retrieval-shadow", continuityID, memories[0].Memory.MemoryID, testVectorWithFirstValue(1))
	insertVectorDocument(t, store, "retrieval-shadow", continuityID, memories[1].Memory.MemoryID, testVectorWithFirstValue(0))
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	request := RetrievalRequest{
		OperationID:   "retrieval-shadow-op",
		TenantID:      "retrieval-shadow",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalShadow,
	}
	lexical, err := store.SearchActiveMemory(context.Background(), request.TenantID, continuityID, request.Query, request.Limit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Memories, lexical) || result.Effective != RetrievalShadow || result.Degraded || result.AuditID == "" {
		t.Fatalf("shadow changed delivery: result=%#v lexical=%#v", result, lexical)
	}
	assertRetrievalAudit(t, store, request.TenantID, request.OperationID, RetrievalShadow, RetrievalShadow, false, "")
	var auditJSON string
	if err := store.pool.QueryRow(context.Background(), `
SELECT to_jsonb(run)::text
FROM memory_retrieval_runs run
WHERE tenant_id = $1 AND operation_id = $2`, request.TenantID, request.OperationID).Scan(&auditJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditJSON, request.Query) || strings.Contains(auditJSON, "Rollback requires two maintainers") {
		t.Fatalf("retrieval audit stored raw query or content: %s", auditJSON)
	}
}

func TestRetrievalCoordinatorVectorDeliversCurrentAuthorizedProjection(t *testing.T) {
	store, continuityID, memories := seedCoordinatorWorkspace(t, "retrieval-vector")
	insertCurrentCursor(t, store, "retrieval-vector")
	insertVectorDocument(t, store, "retrieval-vector", continuityID, memories[0].Memory.MemoryID, testVectorWithFirstValue(1))
	insertVectorDocument(t, store, "retrieval-vector", continuityID, memories[1].Memory.MemoryID, testVectorWithFirstValue(0))
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID:   "retrieval-vector-op",
		TenantID:      "retrieval-vector",
		ContinuityIDs: []string{continuityID},
		Query:         "paraphrased rollback approval",
		Limit:         1,
		Mode:          RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective != RetrievalVector || result.Degraded || len(result.Memories) != 1 || result.Memories[0].ID != memories[0].Memory.MemoryID {
		t.Fatalf("vector result=%#v want current semantic match", result)
	}
	assertRetrievalAudit(t, store, "retrieval-vector", "retrieval-vector-op", RetrievalVector, RetrievalVector, false, "")
}

func TestRetrievalCoordinatorVectorFallsBackByteExactlyWhenProjectionIsStale(t *testing.T) {
	store, continuityID, memories := seedCoordinatorWorkspace(t, "retrieval-stale")
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	request := RetrievalRequest{
		OperationID:   "retrieval-stale-op",
		TenantID:      "retrieval-stale",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalVector,
	}
	lexical, err := store.SearchActiveMemory(context.Background(), request.TenantID, continuityID, request.Query, request.Limit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Memories, lexical) || result.Effective != RetrievalLexical || !result.Degraded || result.FailureCode != "projection_lag" {
		t.Fatalf("stale projection did not use exact lexical fallback: result=%#v lexical=%#v", result, lexical)
	}
	assertRetrievalAudit(t, store, request.TenantID, request.OperationID, RetrievalVector, RetrievalLexical, true, "projection_lag")

	shadowRequest := request
	shadowRequest.OperationID = "retrieval-stale-shadow-op"
	shadowRequest.Mode = RetrievalShadow
	shadowResult, err := coordinator.Retrieve(context.Background(), shadowRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shadowResult.Memories, lexical) || shadowResult.Effective != RetrievalLexical || !shadowResult.Degraded {
		t.Fatalf("stale shadow changed lexical delivery: result=%#v lexical=%#v", shadowResult, lexical)
	}
	assertRetrievalAudit(t, store, shadowRequest.TenantID, shadowRequest.OperationID, RetrievalShadow, RetrievalLexical, true, "projection_lag")
	_ = memories
}

func TestRetrievalCoordinatorFallsBackForNonIdleProjectionStates(t *testing.T) {
	for _, cursorStatus := range []string{"running", "failed"} {
		t.Run(cursorStatus, func(t *testing.T) {
			tenantID := "retrieval-state-" + cursorStatus
			store, continuityID, memories := seedCoordinatorWorkspace(t, tenantID)
			insertCurrentCursor(t, store, tenantID)
			insertVectorDocument(t, store, tenantID, continuityID, memories[0].Memory.MemoryID, testVectorWithFirstValue(1))
			if _, err := store.pool.Exec(context.Background(), `
UPDATE memory_projection_cursors
SET status = $3
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, ProductionRetrievalProfileID, cursorStatus); err != nil {
				t.Fatal(err)
			}
			embedder := &projectionTestEmbedder{vector: testVectorWithFirstValue(1)}
			coordinator := mustRetrievalCoordinator(t, store, embedder)
			result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
				OperationID: "retrieval-state-" + cursorStatus + "-op",
				TenantID:    tenantID, ContinuityIDs: []string{continuityID},
				Query: "rollback maintainers", Limit: 5, Mode: RetrievalVector,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Effective != RetrievalLexical || !result.Degraded || embedder.calls.Load() != 0 {
				t.Fatalf("non-idle projection was queried: result=%#v calls=%d", result, embedder.calls.Load())
			}
			assertRetrievalAudit(t, store, tenantID, "retrieval-state-"+cursorStatus+"-op", RetrievalVector, RetrievalLexical, true, "projection_not_current")
		})
	}
}

func TestRetrievalCoordinatorFallsBackBelowRetentionFloor(t *testing.T) {
	tenantID := "retrieval-state-rebuild-required"
	store, continuityID, _ := seedCoordinatorWorkspace(t, tenantID)
	floor := latestProjectionEventID(t, store, tenantID)
	setProjectionRetentionFloor(t, store, tenantID, floor)
	if _, err := store.pool.Exec(context.Background(), `DELETE FROM memory_projection_events WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(context.Background(), `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, 0, 'idle')`, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	embedder := &projectionTestEmbedder{vector: testVectorWithFirstValue(1)}
	coordinator := mustRetrievalCoordinator(t, store, embedder)
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID: "retrieval-state-rebuild-required-op",
		TenantID:    tenantID, ContinuityIDs: []string{continuityID},
		Query: "rollback maintainers", Limit: 5, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective != RetrievalLexical || !result.Degraded || embedder.calls.Load() != 0 {
		t.Fatalf("below-floor projection was queried: result=%#v calls=%d", result, embedder.calls.Load())
	}
	assertRetrievalAudit(t, store, tenantID, "retrieval-state-rebuild-required-op", RetrievalVector, RetrievalLexical, true, ProjectionFailureRebuildRequired)
}

func TestRetrievalCoordinatorVectorFallsBackForProviderAndEmptyProjection(t *testing.T) {
	store, continuityID, _ := seedCoordinatorWorkspace(t, "retrieval-fallback")
	insertCurrentCursor(t, store, "retrieval-fallback")
	request := RetrievalRequest{
		OperationID:   "retrieval-provider-fallback-op",
		TenantID:      "retrieval-fallback",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalVector,
	}
	lexical, err := store.SearchActiveMemory(context.Background(), request.TenantID, continuityID, request.Query, request.Limit)
	if err != nil {
		t.Fatal(err)
	}
	providerCoordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{err: errors.New("provider secret")})
	providerResult, err := providerCoordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(providerResult.Memories, lexical) || providerResult.Effective != RetrievalLexical || !providerResult.Degraded || providerResult.FailureCode != "embedding_unavailable" {
		t.Fatalf("provider fallback=%#v lexical=%#v", providerResult, lexical)
	}
	assertRetrievalAudit(t, store, request.TenantID, request.OperationID, RetrievalVector, RetrievalLexical, true, "embedding_unavailable")

	emptyRequest := request
	emptyRequest.OperationID = "retrieval-empty-vector-op"
	emptyCoordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	emptyResult, err := emptyCoordinator.Retrieve(context.Background(), emptyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(emptyResult.Memories, lexical) || emptyResult.Effective != RetrievalLexical || !emptyResult.Degraded || emptyResult.FailureCode != "vector_empty" {
		t.Fatalf("empty vector fallback=%#v lexical=%#v", emptyResult, lexical)
	}
	assertRetrievalAudit(t, store, emptyRequest.TenantID, emptyRequest.OperationID, RetrievalVector, RetrievalLexical, true, "vector_empty")
}

func TestRetrievalCoordinatorRejectsStaleVectorByAuthorityAndHash(t *testing.T) {
	store, continuityID, memories := seedCoordinatorWorkspace(t, "retrieval-authority")
	insertCurrentCursor(t, store, "retrieval-authority")
	insertVectorDocument(t, store, "retrieval-authority", continuityID, memories[0].Memory.MemoryID, testVectorWithFirstValue(1))
	revised, err := NewGovernanceService(store, "retrieval-authority").ReviseSource(context.Background(), "/fixtures/retrieval-authority", memories[0].Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "retrieval-authority-revision",
		Content:     "Rollback requires three maintainers.",
		SourceRef:   "fixture:retrieval-authority-revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(context.Background(), `
UPDATE memory_projection_cursors
SET last_event_id = (SELECT COALESCE(max(event_id), 0) FROM memory_projection_events WHERE tenant_id = $1), status = 'idle'
WHERE tenant_id = $1 AND profile_id = $2`, "retrieval-authority", ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		OperationID:   "retrieval-authority-op",
		TenantID:      "retrieval-authority",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Memories) != 1 || result.Memories[0].ID != revised.Memory.MemoryID || result.Effective != RetrievalLexical || !result.Degraded {
		t.Fatalf("stale authority vector was delivered: %#v", result)
	}
}

func TestRetrievalCoordinatorFiltersActiveVectorWithWrongContentHash(t *testing.T) {
	store, continuityID, memories := seedCoordinatorWorkspace(t, "retrieval-hash")
	insertCurrentCursor(t, store, "retrieval-hash")
	insertVectorDocument(t, store, "retrieval-hash", continuityID, memories[0].Memory.MemoryID, testVectorWithFirstValue(1))
	if _, err := store.pool.Exec(context.Background(), `
UPDATE memory_vector_documents
SET content_sha256 = repeat('b', 64)
WHERE tenant_id = $1 AND profile_id = $2 AND memory_id = $3::uuid`, "retrieval-hash", ProductionRetrievalProfileID, memories[0].Memory.MemoryID); err != nil {
		t.Fatal(err)
	}
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	request := RetrievalRequest{
		OperationID:   "retrieval-hash-op",
		TenantID:      "retrieval-hash",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalVector,
	}
	lexical, err := store.SearchActiveMemory(context.Background(), request.TenantID, continuityID, request.Query, request.Limit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Memories, lexical) || result.Effective != RetrievalLexical || !result.Degraded {
		t.Fatalf("content-hash mismatch was delivered: result=%#v lexical=%#v", result, lexical)
	}
	assertRetrievalAudit(t, store, request.TenantID, request.OperationID, RetrievalVector, RetrievalLexical, true, "vector_empty")
}

func TestRetrievalCoordinatorAuditReplayIsStableAndConflictsAreRejected(t *testing.T) {
	store, continuityID, _ := seedCoordinatorWorkspace(t, "retrieval-audit")
	insertCurrentCursor(t, store, "retrieval-audit")
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	request := RetrievalRequest{
		OperationID:   "retrieval-audit-op",
		TenantID:      "retrieval-audit",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalShadow,
	}
	first, err := coordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Retrieve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.AuditID == "" || second.AuditID != first.AuditID {
		t.Fatalf("audit replay IDs first=%q second=%q", first.AuditID, second.AuditID)
	}
	request.Query = "different request"
	if _, err := coordinator.Retrieve(context.Background(), request); err == nil || !strings.Contains(err.Error(), "audit operation conflict") {
		t.Fatalf("conflicting audit replay was accepted: %v", err)
	}
}

func TestRetrievalCoordinatorLexicalDefaultNeedsNoEmbedderOrAudit(t *testing.T) {
	store, continuityID, _ := seedCoordinatorWorkspace(t, "retrieval-lexical-default")
	coordinator, err := NewRetrievalCoordinator(store, nil, RetrievalProfile{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(context.Background(), RetrievalRequest{
		TenantID:      "retrieval-lexical-default",
		ContinuityIDs: []string{continuityID},
		Query:         "rollback maintainers",
		Limit:         5,
		Mode:          RetrievalLexical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective != RetrievalLexical || result.Degraded || result.AuditID != "" || len(result.Memories) != 1 {
		t.Fatalf("unexpected lexical default result: %#v", result)
	}
	var auditCount int
	if err := store.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_retrieval_runs WHERE tenant_id = $1`, "retrieval-lexical-default").Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatalf("lexical default wrote %d retrieval audits", auditCount)
	}
}

func TestRetrievalCoordinatorRecomputesLinkedConversationAuthorization(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "retrieval-linked-vector"
	primaryAnchor := ConversationAnchor{Channel: "openclaw_dm", ThreadID: "release-primary"}
	childAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "release-child"}
	unlinkedAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "other-thread"}
	primary, primaryMemoryID := confirmConversationMemoryForBridge(t, store, tenantID, primaryAnchor, "linked-vector-primary", "Rollback approval requires two maintainers.")
	child, childMemoryID := confirmConversationMemoryForBridge(t, store, tenantID, childAnchor, "linked-vector-child", "The release window starts at 18:00 UTC.")
	unlinked, unlinkedMemoryID := confirmConversationMemoryForBridge(t, store, tenantID, unlinkedAnchor, "linked-vector-unlinked", "A different rollback also requires two maintainers.")
	if _, err := NewBridgeService(store, tenantID).LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "linked-vector-bridge",
		Primary:     primaryAnchor,
		Linked:      childAnchor,
	}); err != nil {
		t.Fatal(err)
	}
	insertCurrentCursor(t, store, tenantID)
	insertVectorDocument(t, store, tenantID, primary.ContinuityID, primaryMemoryID, testVectorWithFirstValue(1))
	insertVectorDocument(t, store, tenantID, child.ContinuityID, childMemoryID, testVectorWithFirstValue(0))
	insertVectorDocument(t, store, tenantID, unlinked.ContinuityID, unlinkedMemoryID, testVectorWithFirstValue(1))
	coordinator := mustRetrievalCoordinator(t, store, &projectionTestEmbedder{vector: testVectorWithFirstValue(1)})
	request := RetrievalRequest{
		OperationID:   "linked-vector-retrieval",
		TenantID:      tenantID,
		ContinuityIDs: []string{child.ContinuityID, primary.ContinuityID},
		Query:         "Who must approve a rollback?",
		Limit:         1,
		Mode:          RetrievalVector,
	}
	result, err := coordinator.Retrieve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Memories) != 1 || result.Memories[0].ID != primaryMemoryID {
		t.Fatalf("linked vector scope leaked or missed memory: %#v", result)
	}

	request.OperationID = "linked-vector-unauthorized"
	request.ContinuityIDs = append(request.ContinuityIDs, unlinked.ContinuityID)
	if _, err := coordinator.Retrieve(ctx, request); err == nil || !strings.Contains(err.Error(), "stale or unauthorized") {
		t.Fatalf("caller-assembled conversation scope was accepted: %v", err)
	}
}

func mustRetrievalCoordinator(t *testing.T, store *Store, embedder Embedder) *RetrievalCoordinator {
	t.Helper()
	coordinator, err := NewRetrievalCoordinator(store, embedder, RetrievalProfile{
		ID:              ProductionRetrievalProfileID,
		BaseURL:         "https://api.siliconflow.cn/v1",
		Model:           "BAAI/bge-m3",
		Dimensions:      1024,
		ProjectionClass: ProjectionClass1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func seedCoordinatorWorkspace(t *testing.T, tenantID string) (*Store, string, []GovernedObservationReceipt) {
	t.Helper()
	store := openTestStore(t)
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(context.Background(), "/fixtures/"+tenantID); err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, "/fixtures/"+tenantID)
	contents := []string{"Rollback requires two maintainers.", "The release window starts at 18:00 UTC."}
	receipts := make([]GovernedObservationReceipt, 0, len(contents))
	for index, content := range contents {
		receipt, err := governance.AddSource(context.Background(), "/fixtures/"+tenantID, GovernanceWriteRequest{
			OperationID: "coordinator-source-" + tenantID + "-" + string(rune('a'+index)),
			MemoryKey:   "coordinator.fact." + string(rune('a'+index)),
			Content:     content,
			SourceRef:   "fixture:" + tenantID,
		})
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt)
	}
	return store, continuityID, receipts
}

func insertCurrentCursor(t *testing.T, store *Store, tenantID string) {
	t.Helper()
	if _, err := store.pool.Exec(context.Background(), `
INSERT INTO memory_projection_cursors (tenant_id, profile_id, last_event_id, status)
VALUES ($1, $2, (SELECT COALESCE(max(event_id), 0) FROM memory_projection_events WHERE tenant_id = $1), 'idle')
ON CONFLICT (tenant_id, profile_id) DO UPDATE SET
  last_event_id = EXCLUDED.last_event_id, status = 'idle'`, tenantID, ProductionRetrievalProfileID); err != nil {
		t.Fatal(err)
	}
}

func insertVectorDocument(t *testing.T, store *Store, tenantID, continuityID, memoryID string, vector []float32) {
	t.Helper()
	if _, err := store.pool.Exec(context.Background(), `
INSERT INTO memory_vector_documents (
  profile_id, tenant_id, continuity_id, memory_id, content_sha256, embedding
)
SELECT $1, $2, $3::uuid, $4::uuid, encode(digest(convert_to(content, 'UTF8'), 'sha256'), 'hex'), $5::vector
FROM governed_memories
WHERE tenant_id = $2 AND id = $4::uuid`, ProductionRetrievalProfileID, tenantID, continuityID, memoryID, retrievalVectorLiteral(vector)); err != nil {
		t.Fatal(err)
	}
}

func testVectorWithFirstValue(value float32) []float32 {
	vector := make([]float32, 1024)
	vector[0] = value
	return vector
}

func assertRetrievalAudit(t *testing.T, store *Store, tenantID, operationID string, requested, effective RetrievalMode, degraded bool, failureCode string) {
	t.Helper()
	var gotRequested, gotEffective, gotFailure string
	var gotDegraded bool
	if err := store.pool.QueryRow(context.Background(), `
SELECT requested_mode, effective_mode, degraded, failure_code
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&gotRequested, &gotEffective, &gotDegraded, &gotFailure); err != nil {
		t.Fatal(err)
	}
	if gotRequested != string(requested) || gotEffective != string(effective) || gotDegraded != degraded || gotFailure != failureCode {
		t.Fatalf("audit requested=%q effective=%q degraded=%v failure=%q", gotRequested, gotEffective, gotDegraded, gotFailure)
	}
}
