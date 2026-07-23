package runtime_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"vermory/internal/mcpserver"
	"vermory/internal/runtime"
	"vermory/internal/webchat"

	"github.com/jackc/pgx/v5/pgxpool"
)

type productionRetrievalCase struct {
	ID        string                     `json:"id"`
	Profile   productionRetrievalProfile `json:"profile"`
	Workspace struct {
		TenantID         string `json:"tenant_id"`
		RepoRoot         string `json:"repo_root"`
		SemanticQuery    string `json:"semantic_query"`
		SemanticExpected string `json:"semantic_expected"`
		ExactQuery       string `json:"exact_query"`
		ActiveFacts      []struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		} `json:"active_facts"`
		Superseded struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		} `json:"superseded"`
		Deleted struct {
			Key     string `json:"key"`
			Content string `json:"content"`
		} `json:"deleted"`
		Proposed       string `json:"proposed"`
		CrossWorkspace struct {
			RepoRoot string `json:"repo_root"`
			Key      string `json:"key"`
			Content  string `json:"content"`
		} `json:"cross_workspace"`
		CrossTenant struct {
			TenantID string `json:"tenant_id"`
			RepoRoot string `json:"repo_root"`
			Key      string `json:"key"`
			Content  string `json:"content"`
		} `json:"cross_tenant"`
	} `json:"workspace"`
	Conversation struct {
		TenantID string                          `json:"tenant_id"`
		Primary  productionRetrievalConversation `json:"primary"`
		Linked   productionRetrievalConversation `json:"linked"`
		Unlinked productionRetrievalConversation `json:"unlinked"`
		Query    string                          `json:"query"`
		Expected string                          `json:"expected"`
	} `json:"conversation"`
	HardGates struct {
		WorkspaceForbidden    []string `json:"workspace_forbidden"`
		ConversationForbidden []string `json:"conversation_forbidden"`
	} `json:"hard_gates"`
}

type productionRetrievalProfile struct {
	ID         string `json:"id"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

type productionRetrievalConversation struct {
	Channel  string `json:"channel"`
	ThreadID string `json:"thread_id"`
	Fact     string `json:"fact"`
}

func TestProductionRetrievalAcceptance(t *testing.T) {
	manifest := loadProductionRetrievalCase(t)
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := runtime.OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	seeded := seedProductionRetrievalCase(t, store, manifest)
	embedder := productionRetrievalEmbedder{}
	runProductionWorkerCurrent(t, store, manifest.Workspace.TenantID, embedder, retrievalProfile(manifest))
	runProductionWorkerCurrent(t, store, manifest.Workspace.CrossTenant.TenantID, embedder, retrievalProfile(manifest))
	status, err := store.RetrievalProjectionStatus(ctx, manifest.Workspace.TenantID, manifest.Profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Lag != 0 || status.VectorCount != 10 || status.Status != "idle" {
		t.Fatalf("unexpected active-only projection status: %#v", status)
	}

	coordinator, err := runtime.NewRetrievalCoordinator(store, embedder, retrievalProfile(manifest))
	if err != nil {
		t.Fatal(err)
	}
	lexicalHandler := mcpserver.New(runtime.NewService(store, manifest.Workspace.TenantID), mcpserver.Config{TenantID: manifest.Workspace.TenantID})
	vectorHandler := mcpserver.New(runtime.NewServiceWithRetriever(store, manifest.Workspace.TenantID, productionModeRetriever{delegate: coordinator, mode: runtime.RetrievalVector}), mcpserver.Config{TenantID: manifest.Workspace.TenantID})
	shadowHandler := mcpserver.New(runtime.NewServiceWithRetriever(store, manifest.Workspace.TenantID, productionModeRetriever{delegate: coordinator, mode: runtime.RetrievalShadow}), mcpserver.Config{TenantID: manifest.Workspace.TenantID})

	lexicalSemantic := prepareProductionMCP(t, lexicalHandler, "w09-lexical-semantic", manifest.Workspace.RepoRoot, manifest.Workspace.SemanticQuery)
	if strings.Contains(lexicalSemantic.Context, manifest.Workspace.SemanticExpected) {
		t.Fatalf("lexical default unexpectedly solved the frozen semantic paraphrase: %s", lexicalSemantic.Context)
	}
	vectorSemantic := prepareProductionMCP(t, vectorHandler, "w09-vector-semantic", manifest.Workspace.RepoRoot, manifest.Workspace.SemanticQuery)
	assertContainsAll(t, vectorSemantic.Context, []string{manifest.Workspace.SemanticExpected})
	assertContainsNone(t, vectorSemantic.Context, manifest.HardGates.WorkspaceForbidden)

	lexicalExact := prepareProductionMCP(t, lexicalHandler, "w09-lexical-exact", manifest.Workspace.RepoRoot, manifest.Workspace.ExactQuery)
	shadowExact := prepareProductionMCP(t, shadowHandler, "w09-shadow-exact", manifest.Workspace.RepoRoot, manifest.Workspace.ExactQuery)
	if shadowExact.Context != lexicalExact.Context {
		t.Fatalf("shadow changed lexical context bytes:\nlexical=%q\nshadow=%q", lexicalExact.Context, shadowExact.Context)
	}
	vectorExact := prepareProductionMCP(t, vectorHandler, "w09-vector-exact", manifest.Workspace.RepoRoot, manifest.Workspace.ExactQuery)
	for _, expected := range manifest.Workspace.ActiveFacts[1:5] {
		assertContainsAll(t, vectorExact.Context, []string{expected.Content})
	}
	assertContainsNone(t, vectorExact.Context, manifest.HardGates.WorkspaceForbidden)
	assertShadowAudit(t, pool, manifest.Workspace.TenantID, "workspace-retrieval:w09-shadow-exact")

	conversationRetriever := productionModeRetriever{delegate: coordinator, mode: runtime.RetrievalVector}
	conversationService := runtime.NewConversationService(store, manifest.Conversation.TenantID, nil, "", runtime.ConversationServiceConfig{Retriever: conversationRetriever})
	httpHandler := webchat.NewHandlerWithGovernance(
		conversationService,
		runtime.NewGlobalDefaultsService(store, manifest.Conversation.TenantID),
		runtime.NewBridgeService(store, manifest.Conversation.TenantID),
	)
	preparedConversation := prepareProductionOpenClaw(t, httpHandler, "w09-conversation-vector", manifest.Conversation.Primary.ThreadID, manifest.Conversation.Query)
	assertContainsAll(t, preparedConversation.Context, []string{manifest.Conversation.Expected})
	assertContainsNone(t, preparedConversation.Context, manifest.HardGates.ConversationForbidden)

	outageCoordinator, err := runtime.NewRetrievalCoordinator(store, productionErrorEmbedder{}, retrievalProfile(manifest))
	if err != nil {
		t.Fatal(err)
	}
	outageHandler := mcpserver.New(runtime.NewServiceWithRetriever(store, manifest.Workspace.TenantID, productionModeRetriever{delegate: outageCoordinator, mode: runtime.RetrievalVector}), mcpserver.Config{TenantID: manifest.Workspace.TenantID})
	outage := prepareProductionMCP(t, outageHandler, "w09-provider-outage", manifest.Workspace.RepoRoot, manifest.Workspace.ExactQuery)
	if outage.Context != lexicalExact.Context {
		t.Fatalf("provider outage changed lexical fallback bytes:\nlexical=%q\noutage=%q", lexicalExact.Context, outage.Context)
	}
	assertRetrievalFailure(t, pool, manifest.Workspace.TenantID, "workspace-retrieval:w09-provider-outage", "embedding_unavailable")

	governance := runtime.NewGovernanceService(store, manifest.Workspace.TenantID)
	if _, err := governance.AddSource(ctx, manifest.Workspace.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-lag-source",
		MemoryKey:   "release.freeze.window",
		Content:     "发布冻结窗口在 21:45 开始。",
		SourceRef:   "fixture:w09-lag",
	}); err != nil {
		t.Fatal(err)
	}
	lagLexical := prepareProductionMCP(t, lexicalHandler, "w09-lag-lexical", manifest.Workspace.RepoRoot, "21:45")
	lagVector := prepareProductionMCP(t, vectorHandler, "w09-lag-vector", manifest.Workspace.RepoRoot, "21:45")
	if lagVector.Context != lagLexical.Context {
		t.Fatalf("projection lag changed lexical fallback bytes: lexical=%q vector=%q", lagLexical.Context, lagVector.Context)
	}
	assertRetrievalFailure(t, pool, manifest.Workspace.TenantID, "workspace-retrieval:w09-lag-vector", "projection_lag")
	runProductionWorkerCurrent(t, store, manifest.Workspace.TenantID, embedder, retrievalProfile(manifest))

	temporary, err := governance.AddSource(ctx, manifest.Workspace.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-late-source",
		MemoryKey:   "release.temporary.override",
		Content:     "临时发布覆盖码是 TEMP-OVERRIDE-991。",
		SourceRef:   "fixture:w09-late",
	})
	if err != nil {
		t.Fatal(err)
	}
	blocking := &productionBlockingEmbedder{started: make(chan struct{}, 1), release: make(chan struct{})}
	worker, err := runtime.NewProjectionWorker(store, blocking, runtime.ProjectionWorkerOptions{
		TenantID:  manifest.Workspace.TenantID,
		Profile:   retrievalProfile(manifest),
		BatchSize: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	workerDone := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(ctx)
		workerDone <- runErr
	}()
	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("late-completion worker did not reach embedding")
	}
	if _, err := governance.Forget(ctx, manifest.Workspace.RepoRoot, temporary.Memory.MemoryID, "w09-late-delete"); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	if err := <-workerDone; err == nil || !strings.Contains(err.Error(), "authority_changed") {
		t.Fatalf("late completion did not lose to deletion: %v", err)
	}
	runProductionWorkerCurrent(t, store, manifest.Workspace.TenantID, embedder, retrievalProfile(manifest))
	var temporaryVectorCount int
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2 AND memory_id = $3::uuid`, manifest.Workspace.TenantID, manifest.Profile.ID, temporary.Memory.MemoryID).Scan(&temporaryVectorCount); err != nil {
		t.Fatal(err)
	}
	if temporaryVectorCount != 0 {
		t.Fatalf("late completion restored deleted vector row: %d", temporaryVectorCount)
	}

	beforeAuthority := productionAuthorityFingerprint(t, pool)
	beforeRebuild, err := coordinator.Retrieve(ctx, runtime.RetrievalRequest{
		OperationID:   "w09-before-rebuild",
		TenantID:      manifest.Workspace.TenantID,
		ContinuityIDs: []string{seeded.workspaceContinuityID},
		Query:         manifest.Workspace.SemanticQuery,
		Limit:         6,
		Mode:          runtime.RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ResetVectorProjection(ctx, manifest.Workspace.TenantID, manifest.Profile.ID); err != nil {
		t.Fatal(err)
	}
	if afterReset := productionAuthorityFingerprint(t, pool); afterReset != beforeAuthority {
		t.Fatalf("vector reset changed authority: before=%s after=%s", beforeAuthority, afterReset)
	}
	rebuildProductionWorkerCurrent(t, store, manifest.Workspace.TenantID, embedder, retrievalProfile(manifest))
	afterRebuild, err := coordinator.Retrieve(ctx, runtime.RetrievalRequest{
		OperationID:   "w09-after-rebuild",
		TenantID:      manifest.Workspace.TenantID,
		ContinuityIDs: []string{seeded.workspaceContinuityID},
		Query:         manifest.Workspace.SemanticQuery,
		Limit:         6,
		Mode:          runtime.RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(memoryIDs(beforeRebuild.Memories), memoryIDs(afterRebuild.Memories)) {
		t.Fatalf("rebuild changed vector result IDs: before=%#v after=%#v", memoryIDs(beforeRebuild.Memories), memoryIDs(afterRebuild.Memories))
	}
	if afterAuthority := productionAuthorityFingerprint(t, pool); afterAuthority != beforeAuthority {
		t.Fatalf("projection replay changed authority: before=%s after=%s", beforeAuthority, afterAuthority)
	}
}

type productionSeededState struct {
	workspaceContinuityID string
}

func seedProductionRetrievalCase(t *testing.T, store *runtime.Store, manifest productionRetrievalCase) productionSeededState {
	t.Helper()
	ctx := context.Background()
	governance := runtime.NewGovernanceService(store, manifest.Workspace.TenantID)
	if _, err := governance.ConfirmWorkspace(ctx, manifest.Workspace.RepoRoot); err != nil {
		t.Fatal(err)
	}
	resolution, err := store.ResolveWorkspace(ctx, manifest.Workspace.TenantID, runtime.WorkspaceAnchor{RepoRoot: manifest.Workspace.RepoRoot})
	if err != nil {
		t.Fatal(err)
	}
	old, err := governance.AddSource(ctx, manifest.Workspace.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-superseded-source",
		MemoryKey:   manifest.Workspace.Superseded.Key,
		Content:     manifest.Workspace.Superseded.Content,
		SourceRef:   "fixture:w09-superseded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := governance.ReviseSource(ctx, manifest.Workspace.RepoRoot, old.Memory.MemoryID, runtime.GovernanceWriteRequest{
		OperationID: "w09-current-rollback-source",
		Content:     manifest.Workspace.SemanticExpected,
		SourceRef:   "fixture:w09-current",
	}); err != nil {
		t.Fatal(err)
	}
	for _, fact := range manifest.Workspace.ActiveFacts {
		if fact.Content == manifest.Workspace.SemanticExpected {
			continue
		}
		if _, err := governance.AddSource(ctx, manifest.Workspace.RepoRoot, runtime.GovernanceWriteRequest{
			OperationID: "w09-active-" + fact.Key,
			MemoryKey:   fact.Key,
			Content:     fact.Content,
			SourceRef:   "fixture:w09-active",
		}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := governance.AddSource(ctx, manifest.Workspace.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-deleted-source",
		MemoryKey:   manifest.Workspace.Deleted.Key,
		Content:     manifest.Workspace.Deleted.Content,
		SourceRef:   "fixture:w09-deleted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := governance.Forget(ctx, manifest.Workspace.RepoRoot, deleted.Memory.MemoryID, "w09-delete-source"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitGovernedObservation(ctx, manifest.Workspace.TenantID, resolution.ContinuityID, runtime.CommitObservationRequest{
		OperationID: "w09-proposed-agent-result",
		Kind:        runtime.ObservationKindAgentResult,
		Content:     manifest.Workspace.Proposed,
		SourceRef:   "fixture:w09-proposed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := governance.ConfirmWorkspace(ctx, manifest.Workspace.CrossWorkspace.RepoRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := governance.AddSource(ctx, manifest.Workspace.CrossWorkspace.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-cross-workspace",
		MemoryKey:   manifest.Workspace.CrossWorkspace.Key,
		Content:     manifest.Workspace.CrossWorkspace.Content,
		SourceRef:   "fixture:w09-cross-workspace",
	}); err != nil {
		t.Fatal(err)
	}
	otherGovernance := runtime.NewGovernanceService(store, manifest.Workspace.CrossTenant.TenantID)
	if _, err := otherGovernance.ConfirmWorkspace(ctx, manifest.Workspace.CrossTenant.RepoRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := otherGovernance.AddSource(ctx, manifest.Workspace.CrossTenant.RepoRoot, runtime.GovernanceWriteRequest{
		OperationID: "w09-cross-tenant",
		MemoryKey:   manifest.Workspace.CrossTenant.Key,
		Content:     manifest.Workspace.CrossTenant.Content,
		SourceRef:   "fixture:w09-cross-tenant",
	}); err != nil {
		t.Fatal(err)
	}

	primary, _ := confirmProductionConversation(t, store, manifest.Conversation.TenantID, manifest.Conversation.Primary, "w09-conversation-primary")
	linked, _ := confirmProductionConversation(t, store, manifest.Conversation.TenantID, manifest.Conversation.Linked, "w09-conversation-linked")
	_, _ = confirmProductionConversation(t, store, manifest.Conversation.TenantID, manifest.Conversation.Unlinked, "w09-conversation-unlinked")
	if _, err := runtime.NewBridgeService(store, manifest.Conversation.TenantID).LinkConversations(ctx, runtime.LinkConversationsRequest{
		OperationID: "w09-conversation-link",
		Primary:     runtime.ConversationAnchor{Channel: primary.Channel, ThreadID: primary.ThreadID},
		Linked:      runtime.ConversationAnchor{Channel: linked.Channel, ThreadID: linked.ThreadID},
	}); err != nil {
		t.Fatal(err)
	}
	return productionSeededState{workspaceContinuityID: resolution.ContinuityID}
}

func confirmProductionConversation(t *testing.T, store *runtime.Store, tenantID string, definition productionRetrievalConversation, operationID string) (runtime.ConversationResolution, string) {
	t.Helper()
	ctx := context.Background()
	anchor := runtime.ConversationAnchor{Channel: definition.Channel, ThreadID: definition.ThreadID}
	resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := store.CommitObservation(ctx, tenantID, resolution.ContinuityID, runtime.CommitObservationRequest{
		OperationID: operationID + ":message",
		Kind:        runtime.ObservationKindUserMessage,
		Content:     definition.Fact,
		SourceRef:   "fixture:w09-conversation",
	})
	if err != nil {
		t.Fatal(err)
	}
	memory, err := store.ConfirmConversationObservation(ctx, tenantID, resolution.ContinuityID, observation.ObservationID, operationID+":confirm")
	if err != nil {
		t.Fatal(err)
	}
	return resolution, memory.MemoryID
}

func runProductionWorkerCurrent(t *testing.T, store *runtime.Store, tenantID string, embedder runtime.Embedder, profile runtime.RetrievalProfile) {
	t.Helper()
	worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
		TenantID:  tenantID,
		Profile:   profile,
		BatchSize: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 10; attempt++ {
		result, err := worker.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Lag == 0 {
			return
		}
	}
	t.Fatal("projection worker did not reach a current cursor")
}

func rebuildProductionWorkerCurrent(t *testing.T, store *runtime.Store, tenantID string, embedder runtime.Embedder, profile runtime.RetrievalProfile) {
	t.Helper()
	worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
		TenantID: tenantID, Profile: profile, SnapshotPageSize: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.RebuildCurrent(context.Background())
	if err != nil || result.Lag != 0 || result.Status != "idle" {
		t.Fatalf("projection snapshot rebuild did not reach current: result=%#v err=%v", result, err)
	}
}

type productionRetrievalEmbedder struct{}

func (productionRetrievalEmbedder) Embed(_ context.Context, content string) ([]float32, error) {
	vector := make([]float32, 1024)
	switch {
	case strings.Contains(content, "deploy/prod/release.yaml") && strings.Contains(content, "REL-SIG-409"):
		for _, index := range []int{1, 2, 3, 4} {
			vector[index] = 0.5
		}
	case strings.Contains(content, "生产回滚") || strings.Contains(content, "事故回滚") || strings.Contains(content, "撤回"):
		vector[0] = 1
	case strings.Contains(content, "deploy/prod/release.yaml"):
		vector[1] = 1
	case strings.Contains(content, "--canary-percent=10"):
		vector[2] = 1
	case strings.Contains(content, "REL-SIG-409"):
		vector[3] = 1
	case strings.Contains(content, "deepseek-ai/DeepSeek-V4-Flash"):
		vector[4] = 1
	case strings.Contains(content, "数据库迁移") || strings.Contains(content, "数据库变更"):
		vector[10] = 1
	default:
		digest := sha256.Sum256([]byte(content))
		vector[100+int(digest[0])%900] = 1
	}
	return vector, nil
}

type productionErrorEmbedder struct{}

func (productionErrorEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("synthetic provider detail must remain bounded")
}

type productionBlockingEmbedder struct {
	started chan struct{}
	release chan struct{}
}

func (embedder *productionBlockingEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	select {
	case embedder.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-embedder.release:
		return productionRetrievalEmbedder{}.Embed(ctx, content)
	}
}

type productionModeRetriever struct {
	delegate runtime.MemoryRetriever
	mode     runtime.RetrievalMode
}

func (retriever productionModeRetriever) Retrieve(ctx context.Context, request runtime.RetrievalRequest) (runtime.RetrievalResult, error) {
	request.Mode = retriever.mode
	return retriever.delegate.Retrieve(ctx, request)
}

func retrievalProfile(manifest productionRetrievalCase) runtime.RetrievalProfile {
	return runtime.RetrievalProfile{
		ID:              manifest.Profile.ID,
		BaseURL:         manifest.Profile.BaseURL,
		Model:           manifest.Profile.Model,
		Dimensions:      manifest.Profile.Dimensions,
		ProjectionClass: runtime.ProjectionClass1024,
	}
}

func prepareProductionMCP(t *testing.T, handler *mcpserver.Handler, operationID, repoRoot, query string) mcpserver.PrepareContextOutput {
	t.Helper()
	_, output, err := handler.PrepareContext(context.Background(), nil, mcpserver.PrepareContextInput{
		OperationID: operationID,
		RepoRoot:    repoRoot,
		Task:        query,
		MaxItems:    12,
	})
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func prepareProductionOpenClaw(t *testing.T, handler http.Handler, operationID, sessionKey, message string) runtime.PreparedConversationTurn {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"operation_id": operationID,
		"session_key":  sessionKey,
		"message":      message,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/integrations/openclaw/turns/prepare", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("OpenClaw prepare failed: %d %s", response.Code, response.Body.String())
	}
	var prepared runtime.PreparedConversationTurn
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	return prepared
}

func assertShadowAudit(t *testing.T, pool *pgxpool.Pool, tenantID, operationID string) {
	t.Helper()
	var requested, effective string
	var lexicalIDs, vectorIDs, deliveredIDs []string
	var degraded bool
	if err := pool.QueryRow(context.Background(), `
SELECT requested_mode, effective_mode, lexical_memory_ids::text[], vector_memory_ids::text[], delivered_memory_ids::text[], degraded
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&requested, &effective, &lexicalIDs, &vectorIDs, &deliveredIDs, &degraded); err != nil {
		t.Fatal(err)
	}
	if requested != "shadow" || effective != "shadow" || degraded || len(vectorIDs) == 0 || !reflect.DeepEqual(lexicalIDs, deliveredIDs) {
		t.Fatalf("unexpected shadow audit: requested=%s effective=%s lexical=%#v vector=%#v delivered=%#v degraded=%v", requested, effective, lexicalIDs, vectorIDs, deliveredIDs, degraded)
	}
}

func assertRetrievalFailure(t *testing.T, pool *pgxpool.Pool, tenantID, operationID, failureCode string) {
	t.Helper()
	var effective, gotFailure string
	var degraded bool
	if err := pool.QueryRow(context.Background(), `
SELECT effective_mode, degraded, failure_code
FROM memory_retrieval_runs
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&effective, &degraded, &gotFailure); err != nil {
		t.Fatal(err)
	}
	if effective != "lexical" || !degraded || gotFailure != failureCode {
		t.Fatalf("unexpected retrieval fallback audit: effective=%s degraded=%v failure=%s", effective, degraded, gotFailure)
	}
}

func productionAuthorityFingerprint(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var fingerprint string
	if err := pool.QueryRow(context.Background(), `
WITH authoritative_rows AS (
  SELECT 'continuity_spaces' AS table_name, to_jsonb(row_data)::text AS row_data FROM continuity_spaces row_data
  UNION ALL SELECT 'continuity_bindings', to_jsonb(row_data)::text FROM continuity_bindings row_data
  UNION ALL SELECT 'conversation_bindings', to_jsonb(row_data)::text FROM conversation_bindings row_data
  UNION ALL SELECT 'observations', to_jsonb(row_data)::text FROM observations row_data
  UNION ALL SELECT 'governed_memories', to_jsonb(row_data)::text FROM governed_memories row_data
  UNION ALL SELECT 'memory_deliveries', to_jsonb(row_data)::text FROM memory_deliveries row_data
  UNION ALL SELECT 'conversation_turns', to_jsonb(row_data)::text FROM conversation_turns row_data
  UNION ALL SELECT 'bridge_operations', to_jsonb(row_data)::text FROM bridge_operations row_data
  UNION ALL SELECT 'bridge_events', to_jsonb(row_data)::text FROM bridge_events row_data
  UNION ALL SELECT 'bridge_memory_effects', to_jsonb(row_data)::text FROM bridge_memory_effects row_data
  UNION ALL SELECT 'conversation_links', to_jsonb(row_data)::text FROM conversation_links row_data
  UNION ALL SELECT 'source_match_decisions', to_jsonb(row_data)::text FROM source_match_decisions row_data
  UNION ALL SELECT 'source_formation_runs', to_jsonb(row_data)::text FROM source_formation_runs row_data
  UNION ALL SELECT 'source_formation_items', to_jsonb(row_data)::text FROM source_formation_items row_data
)
SELECT encode(digest(convert_to(COALESCE(string_agg(table_name || ':' || row_data, E'\n' ORDER BY table_name, row_data), ''), 'UTF8'), 'sha256'), 'hex')
FROM authoritative_rows`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func memoryIDs(memories []runtime.Memory) []string {
	ids := make([]string, len(memories))
	for index, memory := range memories {
		ids[index] = memory.ID
	}
	return ids
}

func assertContainsAll(t *testing.T, content string, expected []string) {
	t.Helper()
	for _, item := range expected {
		if !strings.Contains(content, item) {
			t.Fatalf("context does not contain %q: %s", item, content)
		}
	}
}

func assertContainsNone(t *testing.T, content string, forbidden []string) {
	t.Helper()
	for _, item := range forbidden {
		if strings.Contains(content, item) {
			t.Fatalf("context contains forbidden %q: %s", item, content)
		}
	}
}

func loadProductionRetrievalCase(t *testing.T) productionRetrievalCase {
	t.Helper()
	path := filepath.Join("..", "..", "runtime", "cases", "W09-production-retrieval-runtime", "case.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest productionRetrievalCase
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "W09-production-retrieval-runtime" || manifest.Profile.ID != runtime.ProductionRetrievalProfileID {
		t.Fatalf("unexpected W09 manifest: %#v", manifest)
	}
	return manifest
}
