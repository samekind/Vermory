package webchat

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vermory/internal/provider"
	"vermory/internal/runtime"
)

func TestHermesCompletionSchedulesFormationWithoutCrossingOpenClawReviewScope(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	tenantID := "hermes-auto-formation"
	service := runtime.NewConversationService(store, tenantID, nil, "", runtime.ConversationServiceConfig{})
	handler := NewHandler(service)
	sessionKey := "profile:personal:shared-key"

	preparedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/prepare", `{
  "operation_id":"hermes-auto-complete",
  "session_key":"`+sessionKey+`",
  "message":"The printer room is C-204."
}`)
	if preparedResponse.Code != http.StatusOK {
		t.Fatalf("Hermes prepare returned %d: %s", preparedResponse.Code, preparedResponse.Body.String())
	}
	completedResponse := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/complete", `{
  "operation_id":"hermes-auto-complete",
  "session_key":"`+sessionKey+`",
  "answer":"Acknowledged.",
  "model":"deepseek-ai/DeepSeek-V4-Flash"
}`)
	if completedResponse.Code != http.StatusOK {
		t.Fatalf("Hermes complete returned %d: %s", completedResponse.Code, completedResponse.Body.String())
	}
	var completed runtime.ChatTurnReceipt
	decodeResponse(t, completedResponse, &completed)
	schedule, err := store.ConversationFormationSchedule(context.Background(), tenantID, completed.ContinuityID)
	if err != nil || schedule.State != runtime.ConversationFormationPending {
		t.Fatalf("Hermes completion schedule=%#v err=%v", schedule, err)
	}

	formationOutput := fmt.Sprintf(`{"candidates":[{"decision":"new","memory_key":"printer.room.current","source_observation_id":%q,"quote":"The printer room is C-204.","occurrence":1,"content":"The printer room is C-204.","reason":"Explicit room."}],"reason":"One candidate."}`, completed.UserObservationID)
	formation := runtime.NewSourceFormationService(store, tenantID, provider.Mock{Output: formationOutput}, "fixture-provider", "fixture-model")
	worker, err := runtime.NewConversationFormationWorker(store, formation, runtime.ConversationFormationWorkerOptions{
		TenantID: tenantID, LeaseDuration: time.Minute, RetryDelay: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := worker.RunOnce(context.Background()); err != nil || !result.Found || result.FormationStatus != runtime.SourceFormationCompleted {
		t.Fatalf("Hermes formation result=%#v err=%v", result, err)
	}

	if response := performJSON(t, handler, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"openclaw-same-key",
  "session_key":"`+sessionKey+`",
  "message":"Review OpenClaw memory."
}`); response.Code != http.StatusOK {
		t.Fatalf("OpenClaw prepare returned %d: %s", response.Code, response.Body.String())
	}
	openClawInbox := performJSON(t, handler, http.MethodGet, "/v1/memories/candidates?channel=openclaw&thread_id=profile%3Apersonal%3Ashared-key", "")
	if openClawInbox.Code != http.StatusOK {
		t.Fatalf("OpenClaw inbox returned %d: %s", openClawInbox.Code, openClawInbox.Body.String())
	}
	var inbox runtime.ConversationReviewInbox
	decodeResponse(t, openClawInbox, &inbox)
	if len(inbox.Candidates) != 0 {
		t.Fatalf("Hermes candidate crossed into OpenClaw inbox: %#v", inbox)
	}

	failedPrepare := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/prepare", `{
  "operation_id":"hermes-auto-failed",
  "session_key":"profile:personal:failed",
  "message":"This turn will fail."
}`)
	if failedPrepare.Code != http.StatusOK {
		t.Fatalf("failed Hermes prepare returned %d: %s", failedPrepare.Code, failedPrepare.Body.String())
	}
	var failedPrepared runtime.PreparedConversationTurn
	decodeResponse(t, failedPrepare, &failedPrepared)
	failed := performJSON(t, handler, http.MethodPost, "/v1/integrations/hermes/turns/fail", `{
  "operation_id":"hermes-auto-failed",
  "session_key":"profile:personal:failed",
  "failure_code":"hermes_empty_output",
  "failure_message":"No visible answer."
}`)
	if failed.Code != http.StatusOK {
		t.Fatalf("Hermes fail returned %d: %s", failed.Code, failed.Body.String())
	}
	if _, err := store.ConversationFormationSchedule(context.Background(), tenantID, failedPrepared.ContinuityID); err == nil {
		t.Fatal("failed Hermes turn created an automatic formation schedule")
	}
}
