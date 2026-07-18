package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"vermory/internal/brand"
	"vermory/internal/runtime"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerVersionUsesBrandVersion(t *testing.T) {
	if got := serverImplementation().Version; got != brand.Version {
		t.Fatalf("MCP version %q does not match brand version %q", got, brand.Version)
	}
}

func TestPrepareContextSchemaDoesNotExposeWorkspaceOrGovernanceAuthority(t *testing.T) {
	handler := New(nil, Config{
		TenantID:  "local",
		Workspace: runtime.WorkspaceAnchor{RepoRoot: "/repo/attached"},
	})
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(handler).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "schema-client", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "prepare_context" && tool.Name != "commit_observation" {
			continue
		}
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"repo_root", "cwd", "filesystem_namespace", "tenant_id", "explicit_binding_id", "adopt", "rebind"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("MCP tool %s exposed forbidden authority field %q: %s", tool.Name, forbidden, encoded)
			}
		}
	}
}

func TestPrepareContextToolReturnsNeedsConfirmationWithoutContext(t *testing.T) {
	handler, _ := testHandler(t)
	_, out, err := handler.PrepareContext(context.Background(), nil, PrepareContextInput{
		OperationID: "prepare-unknown-workspace",
		RepoRoot:    "/ambiguous/repo",
		Task:        "Continue work.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "needs_confirmation" || out.Context != "" || out.DeliveryID != "" {
		t.Fatalf("unexpected ambiguous workspace result: %#v", out)
	}
}

func TestCommitObservationToolCreatesOnlyProposedMemory(t *testing.T) {
	handler, store := testHandler(t)
	ctx := context.Background()
	if _, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/web-checkout"); err != nil {
		t.Fatal(err)
	}
	_, prepared, err := handler.PrepareContext(ctx, nil, PrepareContextInput{
		OperationID: "prepare-agent-writeback",
		RepoRoot:    "/repo/web-checkout",
		Task:        "Continue checkout work.",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, committed, err := handler.CommitObservation(ctx, nil, CommitObservationInput{
		OperationID: "commit-agent-writeback",
		DeliveryID:  prepared.DeliveryID,
		Content:     "Use checkout_eta_unreviewed.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.MemoryStatus != "proposed" {
		t.Fatalf("MCP agent writeback must remain proposed: %#v", committed)
	}
	resolution, err := store.ResolveWorkspace(ctx, "local", runtime.WorkspaceAnchor{RepoRoot: "/repo/web-checkout"})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := store.SearchActiveMemory(ctx, "local", resolution.ContinuityID, "checkout_eta_unreviewed", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("MCP agent writeback became active memory: %#v", matches)
	}
}

func TestCommitObservationToolHasNoTenantOrAuthorityInput(t *testing.T) {
	inputType := reflect.TypeFor[CommitObservationInput]()
	for _, forbidden := range []string{"TenantID", "Kind", "SupersedesMemoryID", "TargetMemoryID"} {
		if _, ok := inputType.FieldByName(forbidden); ok {
			t.Fatalf("MCP input must not accept %s", forbidden)
		}
	}
}

func TestPrepareContextToolHasNoBindingOverrideInput(t *testing.T) {
	inputType := reflect.TypeFor[PrepareContextInput]()
	if _, ok := inputType.FieldByName("ExplicitBindingID"); ok {
		t.Fatal("MCP input must not let an agent override workspace binding")
	}
}

func TestPrepareContextToolUsesRetrieverWithoutExposingInternalMetadata(t *testing.T) {
	_, store := testHandler(t)
	ctx := context.Background()
	if _, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/semantic-release"); err != nil {
		t.Fatal(err)
	}
	retriever := &mcpRecordingRetriever{}
	handler := New(runtime.NewServiceWithRetriever(store, "local", retriever), Config{TenantID: "local"})
	_, output, err := handler.PrepareContext(ctx, nil, PrepareContextInput{
		OperationID: "mcp-semantic-prepare",
		RepoRoot:    "/repo/semantic-release",
		Task:        "Who approves rollback?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retriever.requests) != 1 || retriever.requests[0].OperationID != "workspace-retrieval:mcp-semantic-prepare" {
		t.Fatalf("unexpected MCP retrieval request: %#v", retriever.requests)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, internal := range []string{"vector", runtime.ProductionRetrievalProfileID, "77777777-7777-7777-7777-777777777777", "retrieval_mode", "audit_id"} {
		if strings.Contains(string(encoded), internal) {
			t.Fatalf("MCP output exposed retrieval metadata %q: %s", internal, encoded)
		}
	}
	if !strings.Contains(output.Context, "Rollback requires two maintainers") {
		t.Fatalf("MCP output lost semantic memory: %#v", output)
	}
}

func TestW03MCPMemoryEligibilityAndS01Forget(t *testing.T) {
	handler, store := testHandler(t)
	ctx := context.Background()
	const (
		tenantID   = "local"
		repoRoot   = "/repo/w03-workspace-validity"
		workaround = "Temporary workaround: set VERMORY_CACHE_DISABLED=1 while cache invalidation is under repair."
		command    = "Verification command: run go test -p 1 -count=1 ./... before delivery."
		security   = "Security control: never commit .env files or credentials."
		secret     = "TEMP-NEBULA-5521"
	)
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	governance := runtime.NewGovernanceService(store, tenantID)
	add := func(operationID, key, content string) runtime.GovernedObservationReceipt {
		t.Helper()
		receipt, addErr := governance.AddSource(ctx, repoRoot, runtime.GovernanceWriteRequest{
			OperationID: operationID,
			MemoryKey:   key,
			Content:     content,
			SourceRef:   "fixture:W03:" + key,
		})
		if addErr != nil {
			t.Fatal(addErr)
		}
		return receipt
	}
	temporary := add("w03-temporary-workaround", "workspace.workaround", workaround)
	durableCommand := add("w03-durable-command", "workspace.verification", command)
	durableSecurity := add("w03-durable-security", "workspace.security", security)
	syntheticSecret := add("w03-synthetic-secret", "workspace.synthetic_secret", "Synthetic recovery secret "+secret+" must be forgotten after this test.")

	initialSnapshot, err := store.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	initialBoundary := initialSnapshot.AsOf.Add(time.Hour)
	if _, err := store.SetMemoryValidity(ctx, runtime.SetMemoryValidityRequest{
		OperationID:  "w03-initial-workaround-validity",
		TenantID:     tenantID,
		ContinuityID: continuityID,
		MemoryID:     temporary.Memory.MemoryID,
		ValidUntil:   &initialBoundary,
	}); err != nil {
		t.Fatal(err)
	}

	query := "Use VERMORY_CACHE_DISABLED, go test -p 1 -count=1, never commit .env, and TEMP-NEBULA-5521 to continue."
	_, before, err := handler.PrepareContext(ctx, nil, PrepareContextInput{
		OperationID: "w03-before-boundary",
		RepoRoot:    repoRoot,
		Task:        query,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{workaround, command, security, secret} {
		if !strings.Contains(before.Context, required) {
			t.Fatalf("W03 pre-boundary MCP context omitted %q: %s", required, before.Context)
		}
	}
	assertMCPModelContextOnly(t, before.Context,
		temporary.Memory.MemoryID, durableCommand.Memory.MemoryID, durableSecurity.Memory.MemoryID, syntheticSecret.Memory.MemoryID,
	)

	_, committed, err := handler.CommitObservation(ctx, nil, CommitObservationInput{
		OperationID: "w03-client-writeback",
		DeliveryID:  before.DeliveryID,
		Content:     "W03 client completed its bounded verification artifact.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.MemoryStatus != "proposed" {
		t.Fatalf("W03 client writeback escaped proposal governance: %#v", committed)
	}
	if active, err := store.SearchActiveMemory(ctx, tenantID, continuityID, "bounded verification artifact", 5); err != nil {
		t.Fatal(err)
	} else {
		for _, memory := range active {
			if memory.ID == committed.MemoryID {
				t.Fatalf("W03 proposed client result became current: memories=%#v", active)
			}
		}
	}

	boundarySnapshot, err := store.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	boundary := boundarySnapshot.AsOf
	if _, err := store.SetMemoryValidity(ctx, runtime.SetMemoryValidityRequest{
		OperationID:  "w03-workaround-exact-boundary",
		TenantID:     tenantID,
		ContinuityID: continuityID,
		MemoryID:     temporary.Memory.MemoryID,
		ValidUntil:   &boundary,
	}); err != nil {
		t.Fatal(err)
	}

	_, after, err := handler.PrepareContext(ctx, nil, PrepareContextInput{
		OperationID: "w03-after-boundary",
		RepoRoot:    repoRoot,
		Task:        query,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(after.Context, workaround) {
		t.Fatalf("W03 expired workaround remained in MCP context: %s", after.Context)
	}
	for _, required := range []string{command, security} {
		if !strings.Contains(after.Context, required) {
			t.Fatalf("W03 durable control disappeared after workaround expiry %q: %s", required, after.Context)
		}
	}
	assertMCPModelContextOnly(t, after.Context,
		temporary.Memory.MemoryID, durableCommand.Memory.MemoryID, durableSecurity.Memory.MemoryID, syntheticSecret.Memory.MemoryID,
	)

	if _, err := store.ArchiveMemory(ctx, runtime.ArchiveMemoryRequest{
		OperationID:  "w03-archive-expired-workaround",
		TenantID:     tenantID,
		ContinuityID: continuityID,
		MemoryID:     temporary.Memory.MemoryID,
	}); err != nil {
		t.Fatal(err)
	}
	_, memories, err := governance.ListWorkspaceMemories(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasGovernedMemory(memories, temporary.Memory.MemoryID, workaround, "archived", runtime.MemoryEffectiveArchived) {
		t.Fatalf("W03 archive did not preserve authorized history: %#v", memories)
	}

	if _, err := governance.Forget(ctx, repoRoot, syntheticSecret.Memory.MemoryID, "s01-forget-synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	if err := store.RebuildProjection(ctx, tenantID, continuityID); err != nil {
		t.Fatal(err)
	}
	for index, probe := range []string{
		"What was the exact TEMP-NEBULA-5521 recovery secret?",
		"What was the synthetic nebula-style recovery sequence used in this workspace?",
	} {
		_, prepared, prepareErr := handler.PrepareContext(ctx, nil, PrepareContextInput{
			OperationID: "s01-mcp-forget-probe-" + string(rune('1'+index)),
			RepoRoot:    repoRoot,
			Task:        probe,
		})
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		if strings.Contains(prepared.Context, secret) {
			t.Fatalf("S01 forgotten secret returned through MCP probe %q: %s", probe, prepared.Context)
		}
		assertMCPModelContextOnly(t, prepared.Context, syntheticSecret.Memory.MemoryID)
	}
	_, memories, err = governance.ListWorkspaceMemories(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !hasGovernedMemory(memories, syntheticSecret.Memory.MemoryID, "[redacted]", "deleted", runtime.MemoryEffectiveDeleted) {
		t.Fatalf("S01 forgotten memory did not remain redacted in authority: %#v", memories)
	}
}

func assertMCPModelContextOnly(t *testing.T, contextPacket string, memoryIDs ...string) {
	t.Helper()
	forbidden := []string{"valid_from", "valid_until", "eligibility_as_of", "lifecycle_status", "effective_state", "memory_key"}
	forbidden = append(forbidden, memoryIDs...)
	for _, value := range forbidden {
		if strings.Contains(contextPacket, value) {
			t.Fatalf("MCP model context exposed internal metadata %q: %s", value, contextPacket)
		}
	}
}

func hasGovernedMemory(memories []runtime.GovernedMemory, memoryID, content, lifecycle string, effective runtime.MemoryEffectiveState) bool {
	for _, memory := range memories {
		if memory.ID == memoryID && memory.Content == content && memory.LifecycleStatus == lifecycle && memory.EffectiveState == effective {
			return true
		}
	}
	return false
}

type mcpRecordingRetriever struct {
	requests []runtime.RetrievalRequest
}

func (retriever *mcpRecordingRetriever) Retrieve(_ context.Context, request runtime.RetrievalRequest) (runtime.RetrievalResult, error) {
	retriever.requests = append(retriever.requests, request)
	return runtime.RetrievalResult{
		Memories:  []runtime.Memory{{ID: "88888888-8888-8888-8888-888888888888", Content: "Rollback requires two maintainers."}},
		Effective: runtime.RetrievalVector,
		AuditID:   "77777777-7777-7777-7777-777777777777",
	}, nil
}

func TestServerAdvertisesOnlyNormalFlowTools(t *testing.T) {
	handler, _ := testHandler(t)
	handler.workspace = runtime.WorkspaceAnchor{RepoRoot: "/ambiguous/repo"}
	handler.configErr = nil
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(handler).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("expected two normal-flow tools, got %#v", tools.Tools)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if !names["prepare_context"] || !names["commit_observation"] {
		t.Fatalf("unexpected MCP tools: %#v", tools.Tools)
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "prepare_context",
		Arguments: map[string]any{
			"operation_id": "prepare-mcp-protocol",
			"task":         "Continue work.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected structured needs_confirmation response, got %#v", result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output PrepareContextOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "needs_confirmation" || output.Context != "" || output.DeliveryID != "" {
		t.Fatalf("unexpected MCP structured output: %#v", output)
	}
}

func testHandler(t *testing.T) (*Handler, *runtime.Store) {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(runtime.NewService(store, "local"), Config{TenantID: "local"}), store
}
