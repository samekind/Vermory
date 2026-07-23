package webchat

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vermory/internal/authn"
	"vermory/internal/provider"
	"vermory/internal/runtime"
)

func TestClientOperationHTTPCompletesFencedLifecycle(t *testing.T) {
	handler, store := testHandler(t, provider.Mock{Output: "unused"})
	if _, err := runtime.NewGlobalDefaultsService(store, "local").Set(context.Background(), runtime.SetGlobalDefaultRequest{
		OperationID: "http-leased-default",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese.",
	}); err != nil {
		t.Fatal(err)
	}
	preparedResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/prepare", `{
  "operation_id":"http-leased-complete",
  "channel":"openclaw",
  "thread_id":"agent:main:http-leased",
  "message":"Continue the long-running task."
}`)
	if preparedResponse.Code != http.StatusOK {
		t.Fatalf("prepare returned %d: %s", preparedResponse.Code, preparedResponse.Body.String())
	}
	var prepared runtime.PreparedConversationTurn
	decodeResponse(t, preparedResponse, &prepared)
	if prepared.Protocol != runtime.ClientOperationLeasedV1 || prepared.AttemptID == "" || prepared.LeaseGeneration != 1 || prepared.Context == "" {
		t.Fatalf("unexpected prepared operation: %#v", prepared)
	}

	staleHeartbeat := performJSON(t, handler, http.MethodPost, "/v1/client-operations/heartbeat", fmt.Sprintf(`{
  "operation_id":"%s","channel":"openclaw","thread_id":"agent:main:http-leased",
  "attempt_id":"%s","lease_generation":2
}`, prepared.OperationID, prepared.AttemptID))
	if staleHeartbeat.Code != http.StatusConflict || !strings.Contains(staleHeartbeat.Body.String(), "operation_conflict") {
		t.Fatalf("stale heartbeat returned %d: %s", staleHeartbeat.Code, staleHeartbeat.Body.String())
	}

	checkpointResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/checkpoint", fmt.Sprintf(`{
  "operation_id":"%s","channel":"openclaw","thread_id":"agent:main:http-leased",
  "attempt_id":"%s","lease_generation":1,"sequence":1,
  "checkpoint":{"phase":"tool_loop","completed_steps":3}
}`, prepared.OperationID, prepared.AttemptID))
	if checkpointResponse.Code != http.StatusOK {
		t.Fatalf("checkpoint returned %d: %s", checkpointResponse.Code, checkpointResponse.Body.String())
	}
	var checkpoint runtime.ChatTurnReceipt
	decodeResponse(t, checkpointResponse, &checkpoint)
	if checkpoint.CheckpointSequence != 1 || !strings.Contains(string(checkpoint.Checkpoint), "tool_loop") {
		t.Fatalf("unexpected checkpoint receipt: %#v", checkpoint)
	}

	completeBody := fmt.Sprintf(`{
  "operation_id":"%s","channel":"openclaw","thread_id":"agent:main:http-leased",
  "attempt_id":"%s","lease_generation":1,
  "answer":"Completed after three tool steps.","model":"openclaw/http-test"
}`, prepared.OperationID, prepared.AttemptID)
	completedResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/complete", completeBody)
	if completedResponse.Code != http.StatusOK {
		t.Fatalf("complete returned %d: %s", completedResponse.Code, completedResponse.Body.String())
	}
	var completed runtime.ChatTurnReceipt
	decodeResponse(t, completedResponse, &completed)
	if completed.Status != runtime.ChatTurnCompleted || completed.AssistantObservationID == "" {
		t.Fatalf("unexpected completion receipt: %#v", completed)
	}

	replayedResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/complete", completeBody)
	if replayedResponse.Code != http.StatusOK {
		t.Fatalf("completion replay returned %d: %s", replayedResponse.Code, replayedResponse.Body.String())
	}
	var replayed runtime.ChatTurnReceipt
	decodeResponse(t, replayedResponse, &replayed)
	if !replayed.Replayed || replayed.AssistantObservationID != completed.AssistantObservationID {
		t.Fatalf("completion replay drifted: %#v", replayed)
	}
}

func TestClientOperationHTTPCancelRejectsLateCompletion(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	preparedResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/prepare", `{
  "operation_id":"http-leased-cancel",
  "channel":"openclaw",
  "thread_id":"agent:main:http-cancel",
  "message":"Start a cancellable long-running task."
}`)
	var prepared runtime.PreparedConversationTurn
	decodeResponse(t, preparedResponse, &prepared)
	cancelBody := fmt.Sprintf(`{
  "operation_id":"%s","channel":"openclaw","thread_id":"agent:main:http-cancel",
  "attempt_id":"%s","lease_generation":1,
  "cancellation_code":"user_cancelled","cancellation_message":"Stopped before activation."
}`, prepared.OperationID, prepared.AttemptID)
	cancelledResponse := performJSON(t, handler, http.MethodPost, "/v1/client-operations/cancel", cancelBody)
	if cancelledResponse.Code != http.StatusOK {
		t.Fatalf("cancel returned %d: %s", cancelledResponse.Code, cancelledResponse.Body.String())
	}

	late := performJSON(t, handler, http.MethodPost, "/v1/client-operations/complete", fmt.Sprintf(`{
  "operation_id":"%s","channel":"openclaw","thread_id":"agent:main:http-cancel",
  "attempt_id":"%s","lease_generation":1,
  "answer":"late answer","model":"openclaw/late"
}`, prepared.OperationID, prepared.AttemptID))
	if late.Code != http.StatusConflict || !strings.Contains(late.Body.String(), "already cancelled") {
		t.Fatalf("late completion returned %d: %s", late.Code, late.Body.String())
	}
}

func TestAuthenticatedClientOperationUsesPrincipalTenant(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-leased": principal("identity-leased", authn.RoleClient),
	}}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "unused"}, "test-model", authenticator)
	response := performAuthenticatedJSON(t, handler, "client-leased", http.MethodPost, "/v1/client-operations/prepare", `{
  "operation_id":"authenticated-leased",
  "channel":"openclaw",
  "thread_id":"agent:main:authenticated-leased",
  "message":"Prepare under the authenticated tenant."
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated leased prepare returned %d: %s", response.Code, response.Body.String())
	}
	resolution, err := store.ResolveConversation(context.Background(), "identity-leased", runtime.ConversationAnchor{
		Channel: "openclaw", ThreadID: "agent:main:authenticated-leased",
	})
	if err != nil || resolution.Status != runtime.ResolutionResolved {
		t.Fatalf("leased operation did not use principal tenant: resolution=%#v err=%v", resolution, err)
	}
}
