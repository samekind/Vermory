package webchat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"vermory/internal/provider"
	"vermory/internal/runtime"
)

func TestChatTurnReturnsPersistentReceipt(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "assistant response"})
	response := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"http-turn-1",
  "channel":"web_chat",
  "thread_id":"matter-http",
  "message":"hello"
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var receipt runtime.ChatTurnReceipt
	decodeResponse(t, response, &receipt)
	if receipt.Status != runtime.ChatTurnCompleted || receipt.Answer != "assistant response" || receipt.ContinuityID == "" || receipt.DeliveryID == "" {
		t.Fatalf("unexpected chat receipt: %#v", receipt)
	}
}

func TestChatTurnRejectsMissingAnchorAndAuthorityFields(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	missingThread := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"missing-thread",
  "channel":"web_chat",
  "message":"hello"
}`)
	if missingThread.Code != http.StatusBadRequest {
		t.Fatalf("missing thread returned %d: %s", missingThread.Code, missingThread.Body.String())
	}

	forbidden := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"forbidden-field",
  "channel":"web_chat",
  "thread_id":"matter-http",
  "message":"hello",
  "tenant_id":"attacker",
  "continuity_id":"attacker",
  "provider":"attacker"
}`)
	if forbidden.Code != http.StatusBadRequest {
		t.Fatalf("authority fields were accepted: %d %s", forbidden.Code, forbidden.Body.String())
	}
}

func TestChatTurnRejectsTrailingJSONAndOversizedBody(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	trailing := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", `{"operation_id":"one","channel":"web_chat","thread_id":"matter","message":"hello"} {}`)
	if trailing.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON returned %d: %s", trailing.Code, trailing.Body.String())
	}

	large := `{"operation_id":"large","channel":"web_chat","thread_id":"matter","message":"` + strings.Repeat("x", int(maxRequestBodyBytes)) + `"}`
	oversized := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", large)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body returned %d: %s", oversized.Code, oversized.Body.String())
	}
}

func TestOpenClawTurnEndpointsUseServerOwnedAnchorAndLifecycle(t *testing.T) {
	handler, store := testHandler(t, nil)
	prepareBody := `{
  "operation_id":"openclaw:http-run-1",
  "session_key":"agent:main:home-maintenance-a",
  "message":"What is the current maintenance arrangement?"
}`
	preparedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", prepareBody)
	if preparedResponse.Code != http.StatusOK {
		t.Fatalf("prepare returned %d: %s", preparedResponse.Code, preparedResponse.Body.String())
	}
	var prepared runtime.PreparedConversationTurn
	decodeResponse(t, preparedResponse, &prepared)
	if prepared.Status != runtime.ChatTurnInProgress || prepared.ID == "" || prepared.DeliveryID == "" {
		t.Fatalf("unexpected prepare receipt: %#v", prepared)
	}

	resolution, err := store.ResolveConversation(context.Background(), "local", runtime.ConversationAnchor{
		Channel:  "openclaw",
		ThreadID: "agent:main:home-maintenance-a",
	})
	if err != nil || resolution.Status != runtime.ResolutionResolved {
		t.Fatalf("server-owned OpenClaw anchor was not persisted: resolution=%#v err=%v", resolution, err)
	}

	replayedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", prepareBody)
	if replayedResponse.Code != http.StatusOK {
		t.Fatalf("prepare replay returned %d: %s", replayedResponse.Code, replayedResponse.Body.String())
	}
	var replayed runtime.PreparedConversationTurn
	decodeResponse(t, replayedResponse, &replayed)
	if !replayed.Replayed || replayed.ID != prepared.ID || replayed.DeliveryID != prepared.DeliveryID {
		t.Fatalf("unexpected prepare replay: first=%#v replay=%#v", prepared, replayed)
	}

	completedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/complete", `{
  "operation_id":"openclaw:http-run-1",
  "session_key":"agent:main:home-maintenance-a",
  "answer":"The current appointment is Saturday at 10:00.",
  "model":"grok-cli/grok-4.5"
}`)
	if completedResponse.Code != http.StatusOK {
		t.Fatalf("complete returned %d: %s", completedResponse.Code, completedResponse.Body.String())
	}
	var completed runtime.ChatTurnReceipt
	decodeResponse(t, completedResponse, &completed)
	if completed.Status != runtime.ChatTurnCompleted || completed.AssistantObservationID == "" {
		t.Fatalf("unexpected completion receipt: %#v", completed)
	}

	failedPrepare := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"openclaw:http-run-2",
  "session_key":"agent:main:home-maintenance-a",
  "message":"This run will fail."
}`)
	if failedPrepare.Code != http.StatusOK {
		t.Fatalf("failed-turn prepare returned %d: %s", failedPrepare.Code, failedPrepare.Body.String())
	}
	failedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/fail", `{
  "operation_id":"openclaw:http-run-2",
  "session_key":"agent:main:home-maintenance-a",
  "failure_code":"openclaw_agent_error",
  "failure_message":"provider failed before a visible answer"
}`)
	if failedResponse.Code != http.StatusOK {
		t.Fatalf("fail returned %d: %s", failedResponse.Code, failedResponse.Body.String())
	}
	var failed runtime.ChatTurnReceipt
	decodeResponse(t, failedResponse, &failed)
	if failed.Status != runtime.ChatTurnFailed || failed.FailureCode != "openclaw_agent_error" || failed.AssistantObservationID != "" {
		t.Fatalf("unexpected failure receipt: %#v", failed)
	}
}

func TestHermesTurnEndpointsUseHermesAnchorAndStayIsolatedFromOpenClaw(t *testing.T) {
	handler, store := testHandler(t, nil)
	const sessionKey = "profile:personal:thread-a"

	hermesPrepare := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/prepare", `{
  "operation_id":"hermes:http-run-1",
  "session_key":"profile:personal:thread-a",
  "message":"Continue the current household task."
}`)
	if hermesPrepare.Code != http.StatusOK {
		t.Fatalf("Hermes prepare returned %d: %s", hermesPrepare.Code, hermesPrepare.Body.String())
	}
	var prepared runtime.PreparedConversationTurn
	decodeResponse(t, hermesPrepare, &prepared)
	if prepared.Status != runtime.ChatTurnInProgress || prepared.DeliveryID == "" {
		t.Fatalf("unexpected Hermes prepare receipt: %#v", prepared)
	}

	hermesResolution, err := store.ResolveConversation(context.Background(), "local", runtime.ConversationAnchor{
		Channel:  "hermes",
		ThreadID: sessionKey,
	})
	if err != nil || hermesResolution.Status != runtime.ResolutionResolved {
		t.Fatalf("Hermes anchor was not persisted: resolution=%#v err=%v", hermesResolution, err)
	}

	openClawPrepare := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"openclaw:same-key",
  "session_key":"profile:personal:thread-a",
  "message":"Continue the current household task."
}`)
	if openClawPrepare.Code != http.StatusOK {
		t.Fatalf("OpenClaw control prepare returned %d: %s", openClawPrepare.Code, openClawPrepare.Body.String())
	}
	openClawResolution, err := store.ResolveConversation(context.Background(), "local", runtime.ConversationAnchor{
		Channel:  "openclaw",
		ThreadID: sessionKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if openClawResolution.ContinuityID == hermesResolution.ContinuityID {
		t.Fatalf("Hermes and OpenClaw anchors were merged: hermes=%s openclaw=%s", hermesResolution.ContinuityID, openClawResolution.ContinuityID)
	}

	hermesComplete := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/complete", `{
  "operation_id":"hermes:http-run-1",
  "session_key":"profile:personal:thread-a",
  "answer":"The task remains scheduled for Saturday.",
  "model":"hermes/test-model"
}`)
	if hermesComplete.Code != http.StatusOK {
		t.Fatalf("Hermes complete returned %d: %s", hermesComplete.Code, hermesComplete.Body.String())
	}
	var completed runtime.ChatTurnReceipt
	decodeResponse(t, hermesComplete, &completed)
	if completed.Status != runtime.ChatTurnCompleted || completed.AssistantObservationID == "" {
		t.Fatalf("unexpected Hermes completion receipt: %#v", completed)
	}
}

func TestOpenClawTurnEndpointsRejectClientAuthorityAndConflictingReplay(t *testing.T) {
	handler, _ := testHandler(t, nil)
	for name, body := range map[string]string{
		"tenant":     `{"operation_id":"openclaw:forbidden-tenant","session_key":"agent:main:a","message":"hello","tenant_id":"attacker"}`,
		"continuity": `{"operation_id":"openclaw:forbidden-continuity","session_key":"agent:main:a","message":"hello","continuity_id":"attacker"}`,
		"channel":    `{"operation_id":"openclaw:forbidden-channel","session_key":"agent:main:a","message":"hello","channel":"attacker"}`,
		"status":     `{"operation_id":"openclaw:forbidden-status","session_key":"agent:main:a","message":"hello","status":"completed"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("authority field was accepted: %d %s", response.Code, response.Body.String())
			}
		})
	}

	path := "/v1/integrations/openclaw/turns/prepare"
	first := performJSON(t, handler, http.MethodPost, path, `{"operation_id":"openclaw:conflict","session_key":"agent:main:a","message":"first"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("initial prepare failed: %d %s", first.Code, first.Body.String())
	}
	conflict := performJSON(t, handler, http.MethodPost, path, `{"operation_id":"openclaw:conflict","session_key":"agent:main:a","message":"different"}`)
	if conflict.Code != http.StatusBadRequest {
		t.Fatalf("conflicting replay returned %d: %s", conflict.Code, conflict.Body.String())
	}
	missingIdentity := performJSON(t, handler, http.MethodPost, path, `{"operation_id":"openclaw:missing","message":"hello"}`)
	if missingIdentity.Code != http.StatusBadRequest {
		t.Fatalf("missing session key returned %d: %s", missingIdentity.Code, missingIdentity.Body.String())
	}
	large := `{"operation_id":"openclaw:large","session_key":"agent:main:a","message":"` + strings.Repeat("x", int(maxRequestBodyBytes)) + `"}`
	oversized := performJSON(t, handler, http.MethodPost, path, large)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body returned %d: %s", oversized.Code, oversized.Body.String())
	}
}

func TestGlobalDefaultsEndpointsUseServerOwnedTenantAndDurableState(t *testing.T) {
	handler, store := testHandler(t, provider.Mock{Output: "unused"})
	setResponse := performJSON(t, handler, http.MethodPost, "/v1/defaults/set", `{
  "operation_id":"http-default-set",
  "key":"reply_language",
  "content":"Default user-facing replies to Chinese unless the active task explicitly requests another language."
}`)
	if setResponse.Code != http.StatusOK {
		t.Fatalf("set returned %d: %s", setResponse.Code, setResponse.Body.String())
	}
	var created runtime.GlobalDefaultMutationReceipt
	decodeResponse(t, setResponse, &created)
	if created.ContinuityID == "" || created.MemoryID == "" || created.MemoryStatus != "active" {
		t.Fatalf("unexpected set receipt: %#v", created)
	}

	forbidden := performJSON(t, handler, http.MethodPost, "/v1/defaults/set", `{
  "operation_id":"http-default-forbidden-tenant",
  "key":"output_format",
  "content":"Use Markdown.",
  "tenant_id":"attacker"
}`)
	if forbidden.Code != http.StatusBadRequest {
		t.Fatalf("request-owned tenant was accepted: %d %s", forbidden.Code, forbidden.Body.String())
	}

	restarted := NewHandler(
		runtime.NewConversationService(store, "local", provider.Mock{Output: "unused"}, "test-model", runtime.ConversationServiceConfig{}),
		runtime.NewGlobalDefaultsService(store, "local"),
	)
	inspectRequest := httptest.NewRequest(http.MethodGet, "/v1/defaults", nil)
	inspectResponse := httptest.NewRecorder()
	restarted.ServeHTTP(inspectResponse, inspectRequest)
	if inspectResponse.Code != http.StatusOK {
		t.Fatalf("inspect returned %d: %s", inspectResponse.Code, inspectResponse.Body.String())
	}
	var inspection runtime.GlobalDefaultsInspection
	decodeResponse(t, inspectResponse, &inspection)
	if inspection.ContinuityID != created.ContinuityID || len(inspection.Defaults) != 1 || inspection.Defaults[0].ID != created.MemoryID {
		t.Fatalf("restarted handler lost durable default: %#v", inspection)
	}

	correctResponse := performJSON(t, restarted, http.MethodPost, "/v1/defaults/correct", `{
  "operation_id":"http-default-correct",
  "memory_id":"`+created.MemoryID+`",
  "content":"Default user-facing replies to Chinese."
}`)
	if correctResponse.Code != http.StatusOK {
		t.Fatalf("correct returned %d: %s", correctResponse.Code, correctResponse.Body.String())
	}
	var corrected runtime.GlobalDefaultMutationReceipt
	decodeResponse(t, correctResponse, &corrected)
	if corrected.MemoryID == created.MemoryID || corrected.MemoryStatus != "active" {
		t.Fatalf("unexpected correction receipt: %#v", corrected)
	}

	forgetResponse := performJSON(t, restarted, http.MethodPost, "/v1/defaults/forget", `{
  "operation_id":"http-default-forget",
  "memory_id":"`+corrected.MemoryID+`"
}`)
	if forgetResponse.Code != http.StatusOK {
		t.Fatalf("forget returned %d: %s", forgetResponse.Code, forgetResponse.Body.String())
	}
	var forgotten runtime.GlobalDefaultMutationReceipt
	decodeResponse(t, forgetResponse, &forgotten)
	if forgotten.MemoryStatus != "deleted" || forgotten.MemoryID != corrected.MemoryID {
		t.Fatalf("unexpected forget receipt: %#v", forgotten)
	}
}

func TestBridgeEndpointsExposeDurableServerOwnedGovernance(t *testing.T) {
	handler, store := testHandler(t, provider.Mock{Output: "unused"})
	ctx := context.Background()
	conversationA, memoryA := seedBridgeConversationMemory(t, store, "bridge-http-a", runtime.ConversationAnchor{Channel: "web_chat", ThreadID: "release-a"}, "Use checkout_eta_v2 for the staged checkout release.")
	conversationB, _ := seedBridgeConversationMemory(t, store, "bridge-http-b", runtime.ConversationAnchor{Channel: "openclaw_dm", ThreadID: "release-b"}, "Run the checkout smoke suite before rollout.")
	workspaceID, err := store.ConfirmWorkspaceBinding(ctx, "local", "/fixtures/http-bridge-workspace")
	if err != nil {
		t.Fatal(err)
	}
	governance := runtime.NewGovernanceService(store, "local")
	workspaceMemory, err := governance.AddSource(ctx, "/fixtures/http-bridge-workspace", runtime.GovernanceWriteRequest{
		OperationID: "http-bridge-workspace-source",
		Content:     "Run the checkout smoke suite before rollout.",
		SourceRef:   "fixture:http:bridge",
	})
	if err != nil {
		t.Fatal(err)
	}

	promote := performJSON(t, handler, http.MethodPost, "/v1/bridges/promote", `{
  "operation_id":"http-bridge-promote",
  "source_channel":"web_chat",
  "source_thread_id":"release-a",
  "target_repo_root":"/fixtures/http-bridge-workspace",
  "memory_ids":["`+memoryA+`"]
}`)
	if promote.Code != http.StatusOK {
		t.Fatalf("promote returned %d: %s", promote.Code, promote.Body.String())
	}
	var promoted runtime.BridgeReceipt
	decodeResponse(t, promote, &promoted)
	if promoted.Action != runtime.BridgeActionPromote || promoted.SourceContinuityID != conversationA || promoted.TargetContinuityID != workspaceID {
		t.Fatalf("unexpected promote receipt: %#v", promoted)
	}

	forbidden := performJSON(t, handler, http.MethodPost, "/v1/bridges/promote", `{
  "operation_id":"http-bridge-forbidden",
  "source_channel":"web_chat",
  "source_thread_id":"release-a",
  "target_repo_root":"/fixtures/http-bridge-workspace",
  "memory_ids":["`+memoryA+`"],
  "tenant_id":"attacker"
}`)
	if forbidden.Code != http.StatusBadRequest {
		t.Fatalf("request-owned tenant was accepted: %d %s", forbidden.Code, forbidden.Body.String())
	}

	link := performJSON(t, handler, http.MethodPost, "/v1/bridges/link", `{
  "operation_id":"http-bridge-link",
  "primary_channel":"web_chat",
  "primary_thread_id":"release-a",
  "linked_channel":"openclaw_dm",
  "linked_thread_id":"release-b"
}`)
	if link.Code != http.StatusOK {
		t.Fatalf("link returned %d: %s", link.Code, link.Body.String())
	}
	var linked runtime.BridgeReceipt
	decodeResponse(t, link, &linked)
	if linked.Action != runtime.BridgeActionLink || linked.TargetContinuityID != conversationB {
		t.Fatalf("unexpected link receipt: %#v", linked)
	}

	exportedResponse := performJSON(t, handler, http.MethodPost, "/v1/bridges/export", `{
  "operation_id":"http-bridge-export",
  "repo_root":"/fixtures/http-bridge-workspace",
  "memory_ids":["`+workspaceMemory.Memory.MemoryID+`"],
  "title":"Release handoff",
  "target_profile":"team_handoff"
}`)
	if exportedResponse.Code != http.StatusOK {
		t.Fatalf("export returned %d: %s", exportedResponse.Code, exportedResponse.Body.String())
	}
	var exported runtime.BridgeReceipt
	decodeResponse(t, exportedResponse, &exported)
	if exported.Action != runtime.BridgeActionExport || !strings.Contains(exported.ExportBody, "smoke suite") {
		t.Fatalf("unexpected export receipt: %#v", exported)
	}

	_, err = store.ConfirmWorkspaceBinding(ctx, "local", "/fixtures/http-adopt-original")
	if err != nil {
		t.Fatal(err)
	}
	adoptResponse := performJSON(t, handler, http.MethodPost, "/v1/bridges/adopt", `{
  "operation_id":"http-bridge-adopt",
  "existing_repo_root":"/fixtures/http-adopt-original",
  "new_repo_root":"/fixtures/http-adopt-alias"
}`)
	if adoptResponse.Code != http.StatusOK {
		t.Fatalf("adopt returned %d: %s", adoptResponse.Code, adoptResponse.Body.String())
	}

	_, err = store.ConfirmWorkspaceBinding(ctx, "local", "/fixtures/http-rebind-old")
	if err != nil {
		t.Fatal(err)
	}
	rebindResponse := performJSON(t, handler, http.MethodPost, "/v1/bridges/rebind", `{
  "operation_id":"http-bridge-rebind",
  "old_repo_root":"/fixtures/http-rebind-old",
  "new_repo_root":"/fixtures/http-rebind-new"
}`)
	if rebindResponse.Code != http.StatusOK {
		t.Fatalf("rebind returned %d: %s", rebindResponse.Code, rebindResponse.Body.String())
	}

	restarted := NewHandlerWithGovernance(
		runtime.NewConversationService(store, "local", provider.Mock{Output: "unused"}, "test-model", runtime.ConversationServiceConfig{}),
		runtime.NewGlobalDefaultsService(store, "local"),
		runtime.NewBridgeService(store, "local"),
	)
	inspectRequest := httptest.NewRequest(http.MethodGet, "/v1/bridges/"+promoted.ID, nil)
	inspectResponse := httptest.NewRecorder()
	restarted.ServeHTTP(inspectResponse, inspectRequest)
	if inspectResponse.Code != http.StatusOK {
		t.Fatalf("inspect returned %d: %s", inspectResponse.Code, inspectResponse.Body.String())
	}
	var inspected runtime.BridgeReceipt
	decodeResponse(t, inspectResponse, &inspected)
	if inspected.ID != promoted.ID || len(inspected.Events) != 1 {
		t.Fatalf("restarted handler lost bridge state: %#v", inspected)
	}

	reverse := performJSON(t, restarted, http.MethodPost, "/v1/bridges/reverse", `{
  "operation_id":"http-bridge-promote-reverse",
  "bridge_id":"`+promoted.ID+`"
}`)
	if reverse.Code != http.StatusOK {
		t.Fatalf("reverse returned %d: %s", reverse.Code, reverse.Body.String())
	}
	var reversed runtime.BridgeReceipt
	decodeResponse(t, reverse, &reversed)
	if reversed.Status != runtime.BridgeStatusReversed || len(reversed.Events) != 2 {
		t.Fatalf("unexpected reverse receipt: %#v", reversed)
	}
}

func TestMemoryGovernanceAndInspectionUseExactConversation(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "fact for matter A"})
	turnResponse := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"govern-turn",
  "channel":"web_chat",
  "thread_id":"matter-a",
  "message":"produce a fact"
}`)
	var turn runtime.ChatTurnReceipt
	decodeResponse(t, turnResponse, &turn)

	confirmResponse := performJSON(t, handler, http.MethodPost, "/v1/memories/confirm", `{
  "operation_id":"govern-confirm",
  "channel":"web_chat",
  "thread_id":"matter-a",
  "observation_id":"`+turn.AssistantObservationID+`"
}`)
	if confirmResponse.Code != http.StatusOK {
		t.Fatalf("confirm failed: %d %s", confirmResponse.Code, confirmResponse.Body.String())
	}
	var confirmed runtime.MemoryReceipt
	decodeResponse(t, confirmResponse, &confirmed)
	var confirmationJSON map[string]any
	decodeResponse(t, confirmResponse, &confirmationJSON)
	if _, ok := confirmationJSON["memory_id"]; !ok {
		t.Fatalf("confirmation receipt does not use stable JSON fields: %s", confirmResponse.Body.String())
	}
	if _, ok := confirmationJSON["MemoryID"]; ok {
		t.Fatalf("confirmation receipt exposed Go field names: %s", confirmResponse.Body.String())
	}

	inspectA := httptest.NewRecorder()
	handler.ServeHTTP(inspectA, httptest.NewRequest(http.MethodGet, "/v1/conversations/inspect?channel=web_chat&thread_id=matter-a", nil))
	if inspectA.Code != http.StatusOK || !strings.Contains(inspectA.Body.String(), "fact for matter A") {
		t.Fatalf("inspect A failed: %d %s", inspectA.Code, inspectA.Body.String())
	}
	inspectB := httptest.NewRecorder()
	handler.ServeHTTP(inspectB, httptest.NewRequest(http.MethodGet, "/v1/conversations/inspect?channel=web_chat&thread_id=matter-b", nil))
	if inspectB.Code != http.StatusNotFound || strings.Contains(inspectB.Body.String(), "fact for matter A") {
		t.Fatalf("inspect crossed continuity: %d %s", inspectB.Code, inspectB.Body.String())
	}

	forgetResponse := performJSON(t, handler, http.MethodPost, "/v1/memories/forget", `{
  "operation_id":"govern-forget",
  "channel":"web_chat",
  "thread_id":"matter-a",
  "memory_id":"`+confirmed.MemoryID+`"
}`)
	if forgetResponse.Code != http.StatusOK || strings.Contains(forgetResponse.Body.String(), "fact for matter A") {
		t.Fatalf("forget failed or disclosed content: %d %s", forgetResponse.Code, forgetResponse.Body.String())
	}
}

func TestProviderFailureReturnsPersistedBadGatewayReceipt(t *testing.T) {
	handler, _ := testHandler(t, failingProvider{})
	body := `{"operation_id":"provider-failure","channel":"web_chat","thread_id":"matter","message":"hello"}`
	first := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", body)
	second := performJSON(t, handler, http.MethodPost, "/v1/chat/turn", body)
	if first.Code != http.StatusBadGateway || second.Code != http.StatusBadGateway {
		t.Fatalf("provider failure statuses: first=%d second=%d", first.Code, second.Code)
	}
	if strings.Contains(first.Body.String(), "database") || strings.Contains(first.Body.String(), "provider unavailable internal detail") {
		t.Fatalf("failure response exposed internal detail: %s", first.Body.String())
	}
}

type failingProvider struct{}

func (failingProvider) Generate(context.Context, provider.GenerateRequest) (provider.GenerateResponse, error) {
	return provider.GenerateResponse{}, errProviderUnavailable
}

var errProviderUnavailable = &providerError{"provider unavailable internal detail"}

type providerError struct{ message string }

func (e *providerError) Error() string { return e.message }

func testHandler(t *testing.T, llm provider.Provider) (http.Handler, *runtime.Store) {
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
	service := runtime.NewConversationService(store, "local", llm, "test-model", runtime.ConversationServiceConfig{})
	defaults := runtime.NewGlobalDefaultsService(store, "local")
	bridges := runtime.NewBridgeService(store, "local")
	return NewHandlerWithGovernance(service, defaults, bridges), store
}

func seedBridgeConversationMemory(t *testing.T, store *runtime.Store, operationPrefix string, anchor runtime.ConversationAnchor, content string) (string, string) {
	t.Helper()
	ctx := context.Background()
	resolution, err := store.ResolveOrCreateConversation(ctx, "local", anchor)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := store.CommitObservation(ctx, "local", resolution.ContinuityID, runtime.CommitObservationRequest{
		OperationID: operationPrefix + ":message",
		Kind:        runtime.ObservationKindUserMessage,
		Content:     content,
		SourceRef:   "fixture:http:conversation",
	})
	if err != nil {
		t.Fatal(err)
	}
	memory, err := store.ConfirmConversationObservation(ctx, "local", resolution.ContinuityID, observation.ObservationID, operationPrefix+":confirm")
	if err != nil {
		t.Fatal(err)
	}
	return resolution.ContinuityID, memory.MemoryID
}

func performJSON(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}
