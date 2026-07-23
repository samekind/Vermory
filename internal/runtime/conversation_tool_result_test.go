package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestConversationToolResultRequiresExactPreparedTurnIdentity(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-identity", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:device"}
	prepared, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-device-1",
		Anchor:      anchor,
		Message:     "Check the keyboard diagnostic state.",
	})
	if err != nil {
		t.Fatal(err)
	}

	request := RecordConversationToolResultRequest{
		OperationID: "openclaw:run-device-1",
		Anchor:      anchor,
		RunID:       "run-device-1",
		ToolName:    "device.keyboard_diagnostic",
		ToolCallID:  "call-keyboard-1",
		Content:     "Keyboard diagnostic processed 1,333,470 events.",
	}
	first, err := service.RecordToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.TurnID != prepared.ID || first.ObservationID == "" || first.Replayed {
		t.Fatalf("unexpected first tool receipt: %#v", first)
	}
	second, err := service.RecordToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.ObservationID != first.ObservationID || !second.Replayed {
		t.Fatalf("tool replay was not idempotent: first=%#v second=%#v", first, second)
	}

	conflict := request
	conflict.Content = "Keyboard diagnostic processed a different count."
	if _, err := service.RecordToolResult(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("conflicting tool replay was accepted: %v", err)
	}

	wrongRun := request
	wrongRun.ToolCallID = "call-keyboard-wrong-run"
	wrongRun.RunID = "run-device-other"
	if _, err := service.RecordToolResult(context.Background(), wrongRun); err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("wrong run was accepted: %v", err)
	}

	wrongSession := request
	wrongSession.ToolCallID = "call-keyboard-wrong-session"
	wrongSession.Anchor.ThreadID = "agent:main:unrelated"
	if _, err := service.RecordToolResult(context.Background(), wrongSession); err == nil {
		t.Fatal("cross-session tool result was accepted")
	}
}

func TestConversationToolResultExactReplaySurvivesTurnCompletion(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-completed-replay", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:completed-replay"}
	_, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-completed-replay",
		Anchor:      anchor,
		Message:     "Read the durable device status.",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := RecordConversationToolResultRequest{
		OperationID: "openclaw:run-completed-replay",
		Anchor:      anchor,
		RunID:       "run-completed-replay",
		ToolName:    "read",
		ToolCallID:  "call-completed-replay",
		Content:     "Free capacity is 412 GB.",
	}
	first, err := service.RecordToolResult(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteExternalTurn(context.Background(), CompleteExternalConversationTurnRequest{
		OperationID: request.OperationID,
		Anchor:      anchor,
		Answer:      "The device has 412 GB free.",
		Model:       "fixture/model",
	}); err != nil {
		t.Fatal(err)
	}

	replayed, err := service.RecordToolResult(context.Background(), request)
	if err != nil {
		t.Fatalf("exact replay after completion failed: %v", err)
	}
	if !replayed.Replayed || replayed.ObservationID != first.ObservationID {
		t.Fatalf("unexpected completed replay receipt: first=%#v replayed=%#v", first, replayed)
	}
	conflict := request
	conflict.Content = "Free capacity changed after the accepted call."
	if _, err := service.RecordToolResult(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("conflicting replay was accepted after completion: %v", err)
	}

	newCall := request
	newCall.ToolCallID = "call-after-completion"
	if _, err := service.RecordToolResult(context.Background(), newCall); err == nil || !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("new tool result was accepted after completion: %v", err)
	}
}

func TestConversationToolResultRejectsTurnWithoutPreparedDelivery(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "tool-result-unprepared"
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:unprepared"}
	resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginConversationTurn(ctx, tenantID, resolution.ContinuityID, ChatTurnRequest{
		OperationID: "openclaw:run-unprepared",
		Anchor:      anchor,
		Message:     "This turn has no prepared delivery.",
	}); err != nil {
		t.Fatal(err)
	}
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	_, err = service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID: "openclaw:run-unprepared",
		Anchor:      anchor,
		RunID:       "run-unprepared",
		ToolName:    "device.check",
		ToolCallID:  "call-unprepared",
		Content:     "This result must not be accepted.",
	})
	if err == nil || !strings.Contains(err.Error(), "prepared delivery") {
		t.Fatalf("unprepared turn accepted a tool result: %v", err)
	}
}

func TestConversationToolResultRejectsSensitiveAndOversizedContentBeforePersistence(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-screening", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:screening"}
	_, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-screening",
		Anchor:      anchor,
		Message:     "Inspect the device state.",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := RecordConversationToolResultRequest{
		OperationID: "openclaw:run-screening",
		Anchor:      anchor,
		RunID:       "run-screening",
		ToolName:    "device.debug",
	}

	sensitive := base
	sensitive.ToolCallID = "call-sensitive"
	sensitive.Content = "api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST"
	if _, err := service.RecordToolResult(context.Background(), sensitive); err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("sensitive tool result was accepted: %v", err)
	}

	oversized := base
	oversized.ToolCallID = "call-oversized"
	oversized.Content = strings.Repeat("x", maxConversationToolResultBytes+1)
	if _, err := service.RecordToolResult(context.Background(), oversized); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("oversized tool result was accepted: %v", err)
	}

	var toolObservations, metadataRows int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM observations WHERE tenant_id = $1 AND observation_kind = 'tool_result'`,
		"tool-result-screening",
	).Scan(&toolObservations); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM conversation_tool_results WHERE tenant_id = $1`,
		"tool-result-screening",
	).Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if toolObservations != 0 || metadataRows != 0 {
		t.Fatalf("rejected tool result reached storage: observations=%d metadata=%d", toolObservations, metadataRows)
	}
}

func TestConversationToolResultEnforcesPerTurnCountAndByteLimits(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-limits", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:limits"}
	_, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-limits",
		Anchor:      anchor,
		Message:     "Run the bounded checks.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxConversationToolResultsPerTurn; index++ {
		content := "ok"
		if index < maxConversationToolResultTotalBytes/maxConversationToolResultBytes {
			content = strings.Repeat("x", maxConversationToolResultBytes)
		}
		_, err := service.RecordToolResult(context.Background(), RecordConversationToolResultRequest{
			OperationID: "openclaw:run-limits",
			Anchor:      anchor,
			RunID:       "run-limits",
			ToolName:    "device.check",
			ToolCallID:  "call-" + strings.Repeat("x", index) + "-bounded",
			Content:     content,
		})
		if index == maxConversationToolResultTotalBytes/maxConversationToolResultBytes {
			if err == nil || !strings.Contains(err.Error(), "total content") {
				t.Fatalf("total byte limit was not enforced at index %d: %v", index, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("bounded result %d failed: %v", index, err)
		}
	}
	t.Fatal("test did not reach the total byte limit")
}

func TestConversationToolResultEnforcesPerTurnCountLimit(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-count", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:count"}
	_, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-count",
		Anchor:      anchor,
		Message:     "Run the count-bounded checks.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxConversationToolResultsPerTurn; index++ {
		_, err := service.RecordToolResult(context.Background(), RecordConversationToolResultRequest{
			OperationID: "openclaw:run-count",
			Anchor:      anchor,
			RunID:       "run-count",
			ToolName:    "device.check",
			ToolCallID:  "count-call-" + strings.Repeat("x", index+1),
			Content:     "bounded result",
		})
		if err != nil {
			t.Fatalf("tool result %d failed: %v", index, err)
		}
	}
	_, err = service.RecordToolResult(context.Background(), RecordConversationToolResultRequest{
		OperationID: "openclaw:run-count",
		Anchor:      anchor,
		RunID:       "run-count",
		ToolName:    "device.check",
		ToolCallID:  "count-call-overflow",
		Content:     "one too many",
	})
	if err == nil || !strings.Contains(err.Error(), "too many tool results") {
		t.Fatalf("tool result count limit was not enforced: %v", err)
	}
}

func TestResetForTestClearsConversationToolResults(t *testing.T) {
	store := openTestStore(t)
	service := NewConversationService(store, "tool-result-reset", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:reset"}
	_, err := service.PrepareExternalTurn(context.Background(), ExternalConversationTurnRequest{
		OperationID: "openclaw:run-reset",
		Anchor:      anchor,
		Message:     "Record one result before reset.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordToolResult(context.Background(), RecordConversationToolResultRequest{
		OperationID: "openclaw:run-reset",
		Anchor:      anchor,
		RunID:       "run-reset",
		ToolName:    "device.check",
		ToolCallID:  "call-reset",
		Content:     "reset fixture result",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.pool.QueryRow(context.Background(), `SELECT count(*) FROM conversation_tool_results`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("conversation tool results survived reset: %d", count)
	}
}
