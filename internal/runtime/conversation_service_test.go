package runtime

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"vermory/internal/provider"
)

type recordingProvider struct {
	calls  []provider.GenerateRequest
	output string
	err    error
}

func (p *recordingProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.calls = append(p.calls, request)
	if p.err != nil {
		return provider.GenerateResponse{}, p.err
	}
	return provider.GenerateResponse{Output: p.output, Model: "test-model"}, nil
}

func TestConversationServiceIncludesPriorTurnsOnTheSameThread(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "assistant answer"}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "matter-1"}

	first, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "turn-1",
		Anchor:      anchor,
		Message:     "first user message",
	})
	requireNoError(t, err)
	if first.Status != ChatTurnCompleted {
		t.Fatalf("first turn did not complete: %#v", first)
	}
	_, err = service.Chat(ctx, ChatTurnRequest{
		OperationID: "turn-2",
		Anchor:      anchor,
		Message:     "second user message",
	})
	requireNoError(t, err)

	if len(llm.calls) != 2 {
		t.Fatalf("expected two provider calls, got %d", len(llm.calls))
	}
	packet := llm.calls[1].ContextPacket
	for _, expected := range []string{"Recent conversation:", "User: first user message", "Assistant: assistant answer"} {
		if !strings.Contains(packet, expected) {
			t.Fatalf("second turn packet is missing %q: %s", expected, packet)
		}
	}
	if strings.Contains(packet, "second user message") {
		t.Fatalf("current user message was duplicated into history: %s", packet)
	}
}

func TestConversationConsumerAppliesGlobalDefaultWithoutPersistingLocalOverride(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defaults := NewGlobalDefaultsService(store, "local")
	created, err := defaults.Set(ctx, SetGlobalDefaultRequest{
		OperationID: "conversation-global-language-set",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese unless the active task explicitly requests another language.",
	})
	requireNoError(t, err)
	llm := &recordingProvider{output: "English table deliverable"}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})

	_, err = service.Chat(ctx, ChatTurnRequest{
		OperationID: "conversation-local-english-override",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "mcm-table-task"},
		Message:     "For this task only, produce the table-facing deliverable in English.",
	})
	requireNoError(t, err)
	if len(llm.calls) != 1 {
		t.Fatalf("expected one provider call, got %d", len(llm.calls))
	}
	requireContains(t, llm.calls[0].ContextPacket, "Global defaults:\nDefault user-facing replies to Chinese")
	requireContains(t, llm.calls[0].Prompt, "For this task only")

	inspection, err := defaults.Inspect(ctx)
	requireNoError(t, err)
	if len(inspection.Defaults) != 1 || inspection.Defaults[0].ID != created.MemoryID || inspection.Defaults[0].LifecycleStatus != "active" {
		t.Fatalf("local override mutated the global default: %#v", inspection)
	}
	requireContains(t, inspection.Defaults[0].Content, "Chinese")
	requireNotContains(t, inspection.Defaults[0].Content, "English")

	llm.output = "新的中文回答"
	_, err = service.Chat(ctx, ChatTurnRequest{
		OperationID: "conversation-new-chinese-task",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "unrelated-chinese-task"},
		Message:     "请解释一个新的无关问题。",
	})
	requireNoError(t, err)
	requireContains(t, llm.calls[1].ContextPacket, "Default user-facing replies to Chinese")
	requireNotContains(t, llm.calls[1].ContextPacket, "table-facing deliverable in English")

	_, err = defaults.Forget(ctx, ForgetGlobalDefaultRequest{
		OperationID: "conversation-global-language-forget",
		MemoryID:    created.MemoryID,
	})
	requireNoError(t, err)
	_, err = service.Chat(ctx, ChatTurnRequest{
		OperationID: "conversation-after-global-forget",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "after-default-delete"},
		Message:     "继续一个新的任务。",
	})
	requireNoError(t, err)
	requireNotContains(t, llm.calls[2].ContextPacket, "Default user-facing replies to Chinese")
}

func TestConversationLinkedGovernedMemorySharesWithoutPoolingRawHistoryAndReverses(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	primaryAnchor := ConversationAnchor{Channel: "openclaw_dm", ThreadID: "thesis-submission"}
	linkedAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "thesis-submission"}
	siblingAnchor := ConversationAnchor{Channel: "email_forward", ThreadID: "thesis-submission"}
	unrelatedAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "literature-plan"}
	primary, _ := confirmConversationMemoryForBridge(t, store, "local", primaryAnchor, "link-primary-deadline", "The thesis submission package is due on 18 July at 17:00.")
	linked, _ := confirmConversationMemoryForBridge(t, store, "local", linkedAnchor, "link-child-style", "References use GB/T 7714-2015 numeric style.")
	_, _ = confirmConversationMemoryForBridge(t, store, "local", siblingAnchor, "link-sibling-room", "The thesis defense room is C204.")
	_, _ = confirmConversationMemoryForBridge(t, store, "local", unrelatedAnchor, "link-unrelated-plan", "Create a literature-reading plan for next semester.")
	_, err := store.CommitObservation(ctx, "local", primary.ContinuityID, CommitObservationRequest{
		OperationID: "link-primary-raw-history",
		Kind:        ObservationKindUserMessage,
		Content:     "PRIMARY_RAW_HISTORY must stay in the OpenClaw thread.",
		SourceRef:   conversationUserSourceRef,
	})
	requireNoError(t, err)
	_, err = store.CommitObservation(ctx, "local", linked.ContinuityID, CommitObservationRequest{
		OperationID: "link-child-raw-history",
		Kind:        ObservationKindUserMessage,
		Content:     "LINKED_LOCAL_HISTORY belongs to the Web Chat thread.",
		SourceRef:   conversationUserSourceRef,
	})
	requireNoError(t, err)

	llm := &recordingProvider{output: "ok"}
	chat := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-before",
		Anchor:      linkedAnchor,
		Message:     "When is the thesis submission package due?",
	})
	requireNoError(t, err)
	requireNotContains(t, llm.calls[0].ContextPacket, "18 July at 17:00")

	bridges := NewBridgeService(store, "local")
	linkedBridge, err := bridges.LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "bridge-link-thesis-channels",
		Primary:     primaryAnchor,
		Linked:      linkedAnchor,
	})
	requireNoError(t, err)
	if linkedBridge.Action != BridgeActionLink || linkedBridge.Status != BridgeStatusActive || linkedBridge.SourceContinuityID != primary.ContinuityID || linkedBridge.TargetContinuityID != linked.ContinuityID {
		t.Fatalf("unexpected link receipt: %#v", linkedBridge)
	}
	_, err = bridges.LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "bridge-link-thesis-email",
		Primary:     primaryAnchor,
		Linked:      siblingAnchor,
	})
	requireNoError(t, err)

	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-after-child",
		Anchor:      linkedAnchor,
		Message:     "When is the thesis submission package due now?",
	})
	requireNoError(t, err)
	childPacket := llm.calls[1].ContextPacket
	requireContains(t, childPacket, "Governed memory:\nThe thesis submission package is due on 18 July at 17:00.")
	requireContains(t, childPacket, "LINKED_LOCAL_HISTORY")
	requireNotContains(t, childPacket, "PRIMARY_RAW_HISTORY")
	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-after-sibling",
		Anchor:      linkedAnchor,
		Message:     "Which room is the thesis defense in?",
	})
	requireNoError(t, err)
	requireContains(t, llm.calls[2].ContextPacket, "C204")

	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-after-primary",
		Anchor:      primaryAnchor,
		Message:     "Which reference style should the thesis use?",
	})
	requireNoError(t, err)
	primaryPacket := llm.calls[3].ContextPacket
	requireContains(t, primaryPacket, "GB/T 7714-2015")
	requireContains(t, primaryPacket, "PRIMARY_RAW_HISTORY")
	requireNotContains(t, primaryPacket, "LINKED_LOCAL_HISTORY")

	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-unrelated",
		Anchor:      unrelatedAnchor,
		Message:     "When is the thesis submission package due?",
	})
	requireNoError(t, err)
	unrelatedPacket := llm.calls[4].ContextPacket
	requireNotContains(t, unrelatedPacket, "18 July at 17:00")
	requireNotContains(t, unrelatedPacket, "GB/T 7714-2015")

	_, err = bridges.LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "bridge-link-reject-nested",
		Primary:     linkedAnchor,
		Linked:      unrelatedAnchor,
	})
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("linked child became a nested primary: %v", err)
	}

	reversed, err := bridges.Reverse(ctx, ReverseBridgeRequest{OperationID: "bridge-link-thesis-reverse", BridgeID: linkedBridge.ID})
	requireNoError(t, err)
	if reversed.Status != BridgeStatusReversed {
		t.Fatalf("link was not reversed: %#v", reversed)
	}
	_, err = chat.Chat(ctx, ChatTurnRequest{
		OperationID: "link-after-reverse",
		Anchor:      linkedAnchor,
		Message:     "When is the thesis submission package due after separation?",
	})
	requireNoError(t, err)
	requireNotContains(t, llm.calls[5].ContextPacket, "18 July at 17:00")
	requireNotContains(t, llm.calls[5].ContextPacket, "C204")
}

func TestConversationPreparationUsesInjectedRetrieverWithAuthorizedLinkedScope(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "conversation-retriever"
	primaryAnchor := ConversationAnchor{Channel: "openclaw_dm", ThreadID: "release-primary"}
	childAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "release-child"}
	primary, _ := confirmConversationMemoryForBridge(t, store, tenantID, primaryAnchor, "conversation-retriever-primary", "Primary governed fact.")
	child, _ := confirmConversationMemoryForBridge(t, store, tenantID, childAnchor, "conversation-retriever-child", "Child governed fact.")
	if _, err := NewBridgeService(store, tenantID).LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "conversation-retriever-link",
		Primary:     primaryAnchor,
		Linked:      childAnchor,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := &recordingMemoryRetriever{result: RetrievalResult{
		Memories:  []Memory{{ID: "33333333-3333-3333-3333-333333333333", Content: "Semantic deployment decision is current."}},
		Effective: RetrievalVector,
		AuditID:   "44444444-4444-4444-4444-444444444444",
	}}
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{Retriever: retriever, MemoryLimit: 3})
	prepared, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "conversation-semantic-prepare",
		Anchor:      childAnchor,
		Message:     "What is the deployment decision?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retriever.requests) != 1 {
		t.Fatalf("retriever calls=%d", len(retriever.requests))
	}
	request := retriever.requests[0]
	if request.OperationID != "conversation-retrieval:conversation-semantic-prepare" || request.TenantID != tenantID || request.Query != "What is the deployment decision?" || request.Limit != 3 {
		t.Fatalf("unexpected conversation retrieval request: %#v", request)
	}
	wantScope := []string{primary.ContinuityID, child.ContinuityID}
	sort.Strings(wantScope)
	if !reflect.DeepEqual(request.ContinuityIDs, wantScope) {
		t.Fatalf("conversation retrieval scope=%#v want %#v", request.ContinuityIDs, wantScope)
	}
	requireContains(t, prepared.Context, "Semantic deployment decision is current.")
	for _, internal := range []string{"vector", "44444444-4444-4444-4444-444444444444", "audit"} {
		requireNotContains(t, prepared.Context, internal)
	}
}

func TestConversationExternalTurnPreparesGovernedContextWithoutRawHistory(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	primaryAnchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:home-maintenance-a"}
	linkedAnchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:home-maintenance-b"}
	primary, _ := confirmConversationMemoryForBridge(t, store, "local", primaryAnchor, "external-primary-appointment", "The plumbing inspection is Saturday at 10:00.")
	linked, _ := confirmConversationMemoryForBridge(t, store, "local", linkedAnchor, "external-linked-access", "The technician must check in with the concierge.")
	_, err := store.CommitObservation(ctx, "local", primary.ContinuityID, CommitObservationRequest{
		OperationID: "external-primary-raw",
		Kind:        ObservationKindUserMessage,
		Content:     "PRIMARY_RAW_CHATTER about rain must stay local.",
		SourceRef:   conversationUserSourceRef,
	})
	requireNoError(t, err)
	_, err = store.CommitObservation(ctx, "local", linked.ContinuityID, CommitObservationRequest{
		OperationID: "external-linked-raw",
		Kind:        ObservationKindUserMessage,
		Content:     "LINKED_RAW_CHATTER about lunch is already in OpenClaw.",
		SourceRef:   conversationUserSourceRef,
	})
	requireNoError(t, err)
	_, err = NewBridgeService(store, "local").LinkConversations(ctx, LinkConversationsRequest{
		OperationID: "external-link",
		Primary:     primaryAnchor,
		Linked:      linkedAnchor,
	})
	requireNoError(t, err)
	_, err = NewGlobalDefaultsService(store, "local").Set(ctx, SetGlobalDefaultRequest{
		OperationID: "external-default",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese unless the current task explicitly requests another language.",
	})
	requireNoError(t, err)

	service := NewConversationService(store, "local", nil, "", ConversationServiceConfig{})
	request := ExternalConversationTurnRequest{
		OperationID: "openclaw:run-prepare-1",
		Anchor:      linkedAnchor,
		Message:     "What is the current maintenance arrangement?",
	}
	prepared, err := service.PrepareExternalTurn(ctx, request)
	requireNoError(t, err)
	if prepared.Status != ChatTurnInProgress || prepared.DeliveryID == "" || prepared.Context == "" {
		t.Fatalf("unexpected prepared turn: %#v", prepared)
	}
	for _, expected := range []string{
		"Global defaults:",
		"Default user-facing replies to Chinese",
		"Governed memory:",
		"Saturday at 10:00",
		"check in with the concierge",
	} {
		requireContains(t, prepared.Context, expected)
	}
	for _, forbidden := range []string{"Recent conversation:", "PRIMARY_RAW_CHATTER", "LINKED_RAW_CHATTER", request.Message} {
		requireNotContains(t, prepared.Context, forbidden)
	}

	replayed, err := service.PrepareExternalTurn(ctx, request)
	requireNoError(t, err)
	if !replayed.Replayed || replayed.ID != prepared.ID || replayed.DeliveryID != prepared.DeliveryID || replayed.Context != prepared.Context {
		t.Fatalf("prepare replay changed persisted turn: first=%#v replay=%#v", prepared, replayed)
	}
	var userObservationCount int
	err = store.pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE tenant_id = $1 AND operation_id = $2`, "local", request.OperationID).Scan(&userObservationCount)
	requireNoError(t, err)
	if userObservationCount != 1 {
		t.Fatalf("prepare replay wrote %d user observations", userObservationCount)
	}

	conflictingMessage := request
	conflictingMessage.Message = "different message"
	if _, err := service.PrepareExternalTurn(ctx, conflictingMessage); err == nil {
		t.Fatal("prepare accepted a conflicting message for the same operation")
	}
	conflictingAnchor := request
	conflictingAnchor.Anchor = ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:unrelated-c"}
	if _, err := service.PrepareExternalTurn(ctx, conflictingAnchor); err == nil {
		t.Fatal("prepare accepted a conflicting anchor for the same operation")
	}
}

func TestConversationExternalTurnCompletesAndFailsIdempotently(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "local", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:external-lifecycle"}
	prepared, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "openclaw:run-complete",
		Anchor:      anchor,
		Message:     "State the current appointment.",
	})
	requireNoError(t, err)

	completion := CompleteExternalConversationTurnRequest{
		OperationID: prepared.OperationID,
		Anchor:      anchor,
		Answer:      "The current appointment is Saturday at 10:00.",
		Model:       "grok-cli/grok-4.5",
	}
	completed, err := service.CompleteExternalTurn(ctx, completion)
	requireNoError(t, err)
	if completed.Status != ChatTurnCompleted || completed.AssistantObservationID == "" || completed.Answer != completion.Answer || completed.Model != completion.Model {
		t.Fatalf("unexpected completed turn: %#v", completed)
	}
	replayed, err := service.CompleteExternalTurn(ctx, completion)
	requireNoError(t, err)
	if !replayed.Replayed || replayed.ID != completed.ID || replayed.AssistantObservationID != completed.AssistantObservationID {
		t.Fatalf("completion replay changed turn: first=%#v replay=%#v", completed, replayed)
	}
	wrongAnchor := completion
	wrongAnchor.Anchor = ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:other"}
	if _, err := service.CompleteExternalTurn(ctx, wrongAnchor); err == nil {
		t.Fatal("completion accepted another continuity")
	}
	unknown := completion
	unknown.OperationID = "openclaw:missing"
	if _, err := service.CompleteExternalTurn(ctx, unknown); err == nil {
		t.Fatal("completion accepted an unprepared operation")
	}

	failedPreparation, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "openclaw:run-fail",
		Anchor:      anchor,
		Message:     "This run will fail.",
	})
	requireNoError(t, err)
	failure := FailExternalConversationTurnRequest{
		OperationID:    failedPreparation.OperationID,
		Anchor:         anchor,
		FailureCode:    "openclaw_agent_error",
		FailureMessage: "provider failed before a visible answer",
	}
	failed, err := service.FailExternalTurn(ctx, failure)
	requireNoError(t, err)
	if failed.Status != ChatTurnFailed || failed.FailureCode != failure.FailureCode || failed.AssistantObservationID != "" {
		t.Fatalf("unexpected failed turn: %#v", failed)
	}
	failedReplay, err := service.FailExternalTurn(ctx, failure)
	requireNoError(t, err)
	if !failedReplay.Replayed || failedReplay.ID != failed.ID || failedReplay.AssistantObservationID != "" {
		t.Fatalf("failure replay changed turn: first=%#v replay=%#v", failed, failedReplay)
	}
}

func TestConversationServiceDoesNotCrossThreadBoundary(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "answer"}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()

	_, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "isolated-1",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "matter-a"},
		Message:     "private fact alpha",
	})
	requireNoError(t, err)
	_, err = service.Chat(ctx, ChatTurnRequest{
		OperationID: "isolated-2",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "matter-b"},
		Message:     "unrelated question",
	})
	requireNoError(t, err)

	if strings.Contains(llm.calls[1].ContextPacket, "private fact alpha") {
		t.Fatalf("conversation context leaked across threads: %s", llm.calls[1].ContextPacket)
	}
}

func TestConversationServiceReplaysCompletedTurnWithoutCallingProviderAgain(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "stable answer"}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	request := ChatTurnRequest{
		OperationID: "idempotent-turn",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "matter-replay"},
		Message:     "one request",
	}

	first, err := service.Chat(context.Background(), request)
	requireNoError(t, err)
	second, err := service.Chat(context.Background(), request)
	requireNoError(t, err)

	if len(llm.calls) != 1 {
		t.Fatalf("replayed turn called provider %d times", len(llm.calls))
	}
	if first.ID != second.ID || second.Answer != "stable answer" || !second.Replayed {
		t.Fatalf("unexpected replay receipts: first=%#v second=%#v", first, second)
	}
}

func TestConversationServicePersistsFailedTurnWithoutAssistantObservation(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{err: errors.New("provider unavailable")}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	request := ChatTurnRequest{
		OperationID: "failed-turn",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "matter-failure"},
		Message:     "retain this user input",
	}

	first, err := service.Chat(context.Background(), request)
	requireNoError(t, err)
	second, err := service.Chat(context.Background(), request)
	requireNoError(t, err)
	if first.Status != ChatTurnFailed || second.Status != ChatTurnFailed || !second.Replayed {
		t.Fatalf("failed turn was not persisted: first=%#v second=%#v", first, second)
	}
	if len(llm.calls) != 1 {
		t.Fatalf("failed replay called provider %d times", len(llm.calls))
	}

	resolution, err := store.ResolveOrCreateConversation(context.Background(), "local", request.Anchor)
	requireNoError(t, err)
	recent, err := store.ListRecentConversationObservations(context.Background(), "local", resolution.ContinuityID, "", 12)
	requireNoError(t, err)
	if len(recent) != 1 || recent[0].Kind != ObservationKindUserMessage || recent[0].Content != request.Message {
		t.Fatalf("provider failure wrote an assistant observation: %#v", recent)
	}
}

func TestConversationGovernanceRequiresExplicitConfirmationForAssistantOutput(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "The durable fact is ALPHA-17."}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "matter-confirm"}

	turn, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "confirm-turn",
		Anchor:      anchor,
		Message:     "Report the durable fact.",
	})
	requireNoError(t, err)
	resolution, err := store.ResolveOrCreateConversation(ctx, "local", anchor)
	requireNoError(t, err)
	before, err := store.SearchActiveMemory(ctx, "local", resolution.ContinuityID, "ALPHA-17", 5)
	requireNoError(t, err)
	if len(before) != 0 {
		t.Fatalf("assistant output became active before confirmation: %#v", before)
	}

	confirmed, err := service.Confirm(ctx, ConfirmConversationMemoryRequest{
		OperationID:   "confirm-observation",
		Anchor:        anchor,
		ObservationID: turn.AssistantObservationID,
	})
	requireNoError(t, err)
	if confirmed.Status != "active" {
		t.Fatalf("confirmation did not create active memory: %#v", confirmed)
	}
	after, err := store.SearchActiveMemory(ctx, "local", resolution.ContinuityID, "ALPHA-17", 5)
	requireNoError(t, err)
	if len(after) != 1 || after[0].Content != "The durable fact is ALPHA-17." {
		t.Fatalf("confirmed memory is not retrievable: %#v", after)
	}
}

func TestConversationGovernanceRejectsObservationFromAnotherThread(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "thread-a fact"}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()

	turn, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "cross-thread-turn",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "thread-a"},
		Message:     "produce a fact",
	})
	requireNoError(t, err)
	_, err = service.Confirm(ctx, ConfirmConversationMemoryRequest{
		OperationID:   "cross-thread-confirm",
		Anchor:        ConversationAnchor{Channel: "web_chat", ThreadID: "thread-b"},
		ObservationID: turn.AssistantObservationID,
	})
	if err == nil {
		t.Fatal("confirmation must reject an observation from another conversation")
	}
}

func TestConversationGovernanceCorrectsAndForgetsTargetedMemory(t *testing.T) {
	store := openTestStore(t)
	llm := &recordingProvider{output: "Use release flag old_mode."}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "matter-lifecycle"}

	turn, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "lifecycle-turn",
		Anchor:      anchor,
		Message:     "Which release flag should be used?",
	})
	requireNoError(t, err)
	confirmed, err := service.Confirm(ctx, ConfirmConversationMemoryRequest{
		OperationID:   "lifecycle-confirm",
		Anchor:        anchor,
		ObservationID: turn.AssistantObservationID,
	})
	requireNoError(t, err)
	corrected, err := service.Correct(ctx, CorrectConversationMemoryRequest{
		OperationID: "lifecycle-correct",
		Anchor:      anchor,
		MemoryID:    confirmed.MemoryID,
		Content:     "Use release flag new_mode.",
	})
	requireNoError(t, err)
	if corrected.Memory.Status != "active" {
		t.Fatalf("correction did not create active memory: %#v", corrected)
	}

	resolution, err := store.ResolveOrCreateConversation(ctx, "local", anchor)
	requireNoError(t, err)
	matches, err := store.SearchActiveMemory(ctx, "local", resolution.ContinuityID, "release flag", 5)
	requireNoError(t, err)
	if len(matches) != 1 || strings.Contains(matches[0].Content, "old_mode") || !strings.Contains(matches[0].Content, "new_mode") {
		t.Fatalf("correction lifecycle is wrong: %#v", matches)
	}

	forgotten, err := service.Forget(ctx, ForgetConversationMemoryRequest{
		OperationID: "lifecycle-forget",
		Anchor:      anchor,
		MemoryID:    corrected.Memory.MemoryID,
	})
	requireNoError(t, err)
	if forgotten.Memory.Status != "deleted" {
		t.Fatalf("forget did not delete targeted memory: %#v", forgotten)
	}
	requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
	matches, err = store.SearchActiveMemory(ctx, "local", resolution.ContinuityID, "new_mode", 5)
	requireNoError(t, err)
	if len(matches) != 0 {
		t.Fatalf("forgotten memory returned after rebuild: %#v", matches)
	}
}

func TestConversationForgetRedactsOriginHistoryTurnReplayAndDelivery(t *testing.T) {
	store := openTestStore(t)
	const secret = "ORCHID-7419"
	llm := &recordingProvider{output: "The temporary recovery code is " + secret + "."}
	service := NewConversationService(store, "local", llm, "test-model", ConversationServiceConfig{})
	ctx := context.Background()
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "matter-delete"}
	request := ChatTurnRequest{OperationID: "delete-turn", Anchor: anchor, Message: "Show the temporary code."}

	turn, err := service.Chat(ctx, request)
	requireNoError(t, err)
	confirmed, err := service.Confirm(ctx, ConfirmConversationMemoryRequest{
		OperationID:   "delete-confirm",
		Anchor:        anchor,
		ObservationID: turn.AssistantObservationID,
	})
	requireNoError(t, err)
	llm.output = "The governed recovery code is " + secret + "."
	followup, err := service.Chat(ctx, ChatTurnRequest{
		OperationID: "delete-followup",
		Anchor:      anchor,
		Message:     "What recovery information is retained?",
	})
	requireNoError(t, err)
	if !strings.Contains(followup.Answer, secret) {
		t.Fatalf("test setup did not persist a downstream secret echo: %#v", followup)
	}
	_, err = service.Forget(ctx, ForgetConversationMemoryRequest{
		OperationID: "delete-forget",
		Anchor:      anchor,
		MemoryID:    confirmed.MemoryID,
	})
	requireNoError(t, err)

	replayed, err := service.Chat(ctx, request)
	requireNoError(t, err)
	if strings.Contains(replayed.Answer, secret) {
		t.Fatalf("replayed answer retained deleted content: %#v", replayed)
	}
	inspection, err := service.Inspect(ctx, anchor)
	requireNoError(t, err)
	encoded := inspectionText(inspection)
	if strings.Contains(encoded, secret) {
		t.Fatalf("inspection retained deleted content: %s", encoded)
	}
	if !strings.Contains(encoded, "[redacted]") {
		t.Fatalf("inspection did not retain a redacted marker: %s", encoded)
	}

	var deliveryBody string
	err = store.pool.QueryRow(ctx, `
SELECT context_body FROM memory_deliveries WHERE id = $1::uuid`, followup.DeliveryID).Scan(&deliveryBody)
	requireNoError(t, err)
	if strings.Contains(deliveryBody, secret) {
		t.Fatalf("delivery retained deleted content: %s", deliveryBody)
	}
}

func inspectionText(inspection ConversationInspection) string {
	var parts []string
	for _, observation := range inspection.Observations {
		parts = append(parts, observation.Content)
	}
	for _, memory := range inspection.Memories {
		parts = append(parts, memory.Content)
	}
	return strings.Join(parts, "\n")
}
