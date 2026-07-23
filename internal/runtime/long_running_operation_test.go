package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLeasedConversationOperationReclaimsAndFencesStaleAttempt(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	tenantID := "leased-reclaim"
	_, err := NewGlobalDefaultsService(store, tenantID).Set(ctx, SetGlobalDefaultRequest{
		OperationID: "leased-reclaim-default",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese.",
	})
	requireNoError(t, err)
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{OperationLeaseDuration: 2 * time.Minute})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:leased-reclaim"}
	request := LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-reclaim-run",
		Anchor:      anchor,
		Message:     "Continue the release repair from the latest durable checkpoint.",
	}

	prepared, err := service.PrepareLeasedOperation(ctx, request)
	requireNoError(t, err)
	if prepared.Protocol != ClientOperationLeasedV1 || prepared.Status != ChatTurnInProgress || prepared.AttemptID == "" || prepared.LeaseGeneration != 1 || prepared.LeaseExpiresAt == nil {
		t.Fatalf("unexpected leased prepare receipt: %#v", prepared)
	}
	if prepared.DeliveryID == "" || prepared.Context == "" {
		t.Fatalf("leased prepare omitted immutable delivery: %#v", prepared)
	}

	replayed, err := service.PrepareLeasedOperation(ctx, request)
	requireNoError(t, err)
	if !replayed.Replayed || replayed.ID != prepared.ID || replayed.DeliveryID != prepared.DeliveryID || replayed.Context != prepared.Context || replayed.AttemptID != prepared.AttemptID || replayed.LeaseGeneration != 1 {
		t.Fatalf("exact leased prepare replay drifted: first=%#v replay=%#v", prepared, replayed)
	}
	if _, err := service.ReclaimLeasedOperation(ctx, request); err == nil || !strings.Contains(err.Error(), "live lease") {
		t.Fatalf("live lease was reclaimed: %v", err)
	}
	heartbeat, err := service.HeartbeatLeasedOperation(ctx, HeartbeatLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
	})
	requireNoError(t, err)
	if heartbeat.AttemptID != prepared.AttemptID || heartbeat.LeaseGeneration != prepared.LeaseGeneration || heartbeat.LeaseExpiresAt == nil {
		t.Fatalf("current heartbeat changed operation identity: %#v", heartbeat)
	}

	checkpointRequest := CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            1,
		Checkpoint:                          json.RawMessage(`{"phase":"tests_passed","next":"package"}`),
	}
	checkpoint, err := service.CheckpointLeasedOperation(ctx, checkpointRequest)
	requireNoError(t, err)
	if checkpoint.CheckpointSequence != 1 || !strings.Contains(string(checkpoint.Checkpoint), "tests_passed") {
		t.Fatalf("checkpoint was not retained: %#v", checkpoint)
	}
	toolResult, err := service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID:     request.OperationID,
		Anchor:          anchor,
		RunID:           "leased-reclaim-run",
		ToolName:        "release.verify",
		ToolCallID:      "current-tool-call",
		Content:         "Release verification passed before the process restart.",
		AttemptID:       prepared.AttemptID,
		LeaseGeneration: prepared.LeaseGeneration,
	})
	requireNoError(t, err)
	if toolResult.ObservationID == "" {
		t.Fatalf("current leased attempt did not record fenced tool evidence: %#v", toolResult)
	}
	checkpointReplay, err := service.CheckpointLeasedOperation(ctx, checkpointRequest)
	requireNoError(t, err)
	if !checkpointReplay.Replayed || checkpointReplay.CheckpointSequence != 1 {
		t.Fatalf("checkpoint replay was not idempotent: %#v", checkpointReplay)
	}
	drift := checkpointRequest
	drift.Checkpoint = json.RawMessage(`{"phase":"different"}`)
	if _, err := service.CheckpointLeasedOperation(ctx, drift); err == nil || !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("same checkpoint sequence accepted drift: %v", err)
	}
	checkpointTwo := checkpointRequest
	checkpointTwo.Sequence = 2
	checkpointTwo.Checkpoint = json.RawMessage(`{"phase":"package_built","next":"publish"}`)
	advanced, err := service.CheckpointLeasedOperation(ctx, checkpointTwo)
	requireNoError(t, err)
	if advanced.CheckpointSequence != 2 {
		t.Fatalf("higher checkpoint sequence did not advance: %#v", advanced)
	}
	if _, err := service.CheckpointLeasedOperation(ctx, checkpointRequest); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("checkpoint sequence regression was accepted: %v", err)
	}

	if _, err := store.pool.Exec(ctx, `
UPDATE conversation_turns SET lease_expires_at = now() - interval '1 second'
WHERE tenant_id = $1 AND operation_id = $2`, "leased-reclaim", request.OperationID); err != nil {
		t.Fatal(err)
	}
	nonStealingReplay, err := service.PrepareLeasedOperation(ctx, request)
	requireNoError(t, err)
	if nonStealingReplay.AttemptID != prepared.AttemptID || nonStealingReplay.LeaseGeneration != 1 {
		t.Fatalf("ordinary prepare stole expired lease: %#v", nonStealingReplay)
	}

	reclaimed, err := service.ReclaimLeasedOperation(ctx, request)
	requireNoError(t, err)
	if reclaimed.AttemptID == prepared.AttemptID || reclaimed.LeaseGeneration != 2 || reclaimed.DeliveryID != prepared.DeliveryID || reclaimed.Context != prepared.Context || reclaimed.CheckpointSequence != 2 {
		t.Fatalf("reclaim did not preserve durable state and fence the attempt: first=%#v reclaimed=%#v", prepared, reclaimed)
	}

	if _, err := service.HeartbeatLeasedOperation(ctx, HeartbeatLeasedConversationOperationRequest{LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor)}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("obsolete heartbeat was accepted: %v", err)
	}
	if _, err := service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID:     request.OperationID,
		Anchor:          anchor,
		RunID:           "leased-reclaim-run",
		ToolName:        "release.verify",
		ToolCallID:      "obsolete-tool-call",
		Content:         "This stale result must not become an observation.",
		AttemptID:       prepared.AttemptID,
		LeaseGeneration: prepared.LeaseGeneration,
	}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("obsolete tool result was accepted: %v", err)
	}
	if _, err := service.CompleteLeasedOperation(ctx, CompleteLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Answer:                              "obsolete answer",
		Model:                               "openclaw/obsolete",
	}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("obsolete completion was accepted: %v", err)
	}

	completed, err := service.CompleteLeasedOperation(ctx, CompleteLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(reclaimed.ChatTurnReceipt, anchor),
		Answer:                              "The release repair completed after restart.",
		Model:                               "openclaw/runtime",
	})
	requireNoError(t, err)
	if completed.Status != ChatTurnCompleted || completed.AssistantObservationID == "" || completed.LeaseGeneration != 2 || completed.LeaseExpiresAt != nil {
		t.Fatalf("current attempt did not complete exactly once: %#v", completed)
	}

	var assistantCount, scheduleCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE tenant_id=$1 AND operation_id=$2`, "leased-reclaim", request.OperationID+":assistant").Scan(&assistantCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id=$1 AND continuity_id=$2::uuid`, "leased-reclaim", prepared.ContinuityID).Scan(&scheduleCount); err != nil {
		t.Fatal(err)
	}
	if assistantCount != 1 || scheduleCount != 1 {
		t.Fatalf("completion semantic effects mismatch: assistants=%d schedules=%d", assistantCount, scheduleCount)
	}
}

func TestLeasedConversationCancellationRejectsLateCompletionWithoutSemanticWriteback(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-cancel", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:leased-cancel"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-cancel-run",
		Anchor:      anchor,
		Message:     "Run the guarded migration until cancelled.",
	})
	requireNoError(t, err)

	cancelled, err := service.CancelLeasedOperation(ctx, CancelLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		CancellationCode:                    "user_cancelled",
		CancellationMessage:                 "The operator cancelled before activation.",
	})
	requireNoError(t, err)
	if cancelled.Status != ChatTurnCancelled || cancelled.CancellationCode != "user_cancelled" || cancelled.AssistantObservationID != "" {
		t.Fatalf("unexpected cancellation receipt: %#v", cancelled)
	}
	cancelledReplay, err := service.CancelLeasedOperation(ctx, CancelLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		CancellationCode:                    "user_cancelled",
		CancellationMessage:                 "The operator cancelled before activation.",
	})
	requireNoError(t, err)
	if !cancelledReplay.Replayed || cancelledReplay.ID != cancelled.ID {
		t.Fatalf("cancellation replay changed terminal state: %#v", cancelledReplay)
	}
	if _, err := service.CancelLeasedOperation(ctx, CancelLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		CancellationCode:                    "different_cancel",
		CancellationMessage:                 "Different terminal payload.",
	}); err == nil || !strings.Contains(err.Error(), "another cancelled") {
		t.Fatalf("changed cancellation replay was accepted: %v", err)
	}
	if _, err := service.CompleteLeasedOperation(ctx, CompleteLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Answer:                              "late success",
		Model:                               "openclaw/late",
	}); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("late completion after cancellation was accepted: %v", err)
	}

	var assistantCount, scheduleCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE tenant_id=$1 AND operation_id=$2`, "leased-cancel", prepared.OperationID+":assistant").Scan(&assistantCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id=$1 AND continuity_id=$2::uuid`, "leased-cancel", prepared.ContinuityID).Scan(&scheduleCount); err != nil {
		t.Fatal(err)
	}
	if assistantCount != 0 || scheduleCount != 0 {
		t.Fatalf("cancelled operation produced semantic effects: assistants=%d schedules=%d", assistantCount, scheduleCount)
	}
}

func TestLeasedCheckpointDoesNotEnterSemanticSurfaces(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-checkpoint", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:checkpoint-isolation"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:checkpoint-isolation",
		Anchor:      anchor,
		Message:     "Continue the normal task.",
	})
	requireNoError(t, err)
	if _, err := service.CheckpointLeasedOperation(ctx, CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            1,
		Checkpoint:                          json.RawMessage(`{"api_token":"W36_SYNTHETIC_SECRET_MUST_NOT_PERSIST"}`),
	}); err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("sensitive checkpoint was accepted: %v", err)
	}
	if _, err := service.CheckpointLeasedOperation(ctx, CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            1,
		Checkpoint:                          json.RawMessage(`{"data":"` + strings.Repeat("x", maxConversationCheckpointBytes) + `"}`),
	}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized checkpoint was accepted: %v", err)
	}
	marker := "W36_CHECKPOINT_ONLY_7f93"
	_, err = service.CheckpointLeasedOperation(ctx, CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            1,
		Checkpoint:                          json.RawMessage(`{"cursor":"` + marker + `"}`),
	})
	requireNoError(t, err)

	queries := []string{
		`SELECT count(*) FROM observations WHERE tenant_id=$1 AND content LIKE '%' || $2 || '%'`,
		`SELECT count(*) FROM governed_memories WHERE tenant_id=$1 AND content LIKE '%' || $2 || '%'`,
		`SELECT count(*) FROM memory_search_documents WHERE tenant_id=$1 AND content LIKE '%' || $2 || '%'`,
		`SELECT count(*) FROM memory_deliveries WHERE tenant_id=$1 AND context_body LIKE '%' || $2 || '%'`,
		`SELECT count(*) FROM source_formation_items WHERE tenant_id=$1 AND (content LIKE '%' || $2 || '%' OR quote LIKE '%' || $2 || '%')`,
	}
	for _, query := range queries {
		var count int
		if err := store.pool.QueryRow(ctx, query, "leased-checkpoint", marker).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("checkpoint marker entered semantic surface for query %q", query)
		}
	}
}

func TestLeasedConversationFailureHasNoSemanticWriteback(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-failure", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:leased-failure"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-failure-run",
		Anchor:      anchor,
		Message:     "This long-running operation will fail safely.",
	})
	requireNoError(t, err)
	failure := FailLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		FailureCode:                         "agent_runtime_error",
		FailureMessage:                      "The agent stopped before a visible answer.",
	}
	failed, err := service.FailLeasedOperation(ctx, failure)
	requireNoError(t, err)
	if failed.Status != ChatTurnFailed || failed.FailureCode != failure.FailureCode || failed.AssistantObservationID != "" {
		t.Fatalf("unexpected leased failure receipt: %#v", failed)
	}
	replayed, err := service.FailLeasedOperation(ctx, failure)
	requireNoError(t, err)
	if !replayed.Replayed || replayed.ID != failed.ID {
		t.Fatalf("leased failure replay drifted: %#v", replayed)
	}
	var assistantCount, scheduleCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE tenant_id=$1 AND operation_id=$2`, "leased-failure", prepared.OperationID+":assistant").Scan(&assistantCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id=$1 AND continuity_id=$2::uuid`, "leased-failure", prepared.ContinuityID).Scan(&scheduleCount); err != nil {
		t.Fatal(err)
	}
	if assistantCount != 0 || scheduleCount != 0 {
		t.Fatalf("failed operation produced semantic effects: assistants=%d schedules=%d", assistantCount, scheduleCount)
	}
}

func TestLeasedOperationResumesAfterStoreReopen(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	tenantID := "leased-reopen"
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:leased-reopen"}
	request := LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-reopen-run",
		Anchor:      anchor,
		Message:     "Resume from PostgreSQL after the service process restarts.",
	}
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	prepared, err := service.PrepareLeasedOperation(ctx, request)
	requireNoError(t, err)
	checkpoint, err := service.CheckpointLeasedOperation(ctx, CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            4,
		Checkpoint:                          json.RawMessage(`{"phase":"resume","completed_steps":4}`),
	})
	requireNoError(t, err)

	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	reopened, err := OpenStore(ctx, databaseURL)
	requireNoError(t, err)
	t.Cleanup(reopened.Close)
	restartedService := NewConversationService(reopened, tenantID, nil, "", ConversationServiceConfig{})
	resumed, err := restartedService.PrepareLeasedOperation(ctx, request)
	requireNoError(t, err)
	if !resumed.Replayed || resumed.ID != prepared.ID || resumed.DeliveryID != prepared.DeliveryID || resumed.Context != prepared.Context ||
		resumed.AttemptID != prepared.AttemptID || resumed.LeaseGeneration != prepared.LeaseGeneration ||
		resumed.CheckpointSequence != checkpoint.CheckpointSequence || string(resumed.Checkpoint) != string(checkpoint.Checkpoint) {
		t.Fatalf("store reopen lost leased operation state: before=%#v after=%#v", checkpoint, resumed)
	}
}

func TestLeasedOperationConcurrentCancelAndCompleteHasOneTerminalEffect(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-terminal-race", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:terminal-race"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-terminal-race",
		Anchor:      anchor,
		Message:     "Race cancellation against completion.",
	})
	requireNoError(t, err)
	completion := CompleteLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Answer:                              "The current attempt completed.",
		Model:                               "openclaw/race",
	}
	cancellation := CancelLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		CancellationCode:                    "operator_cancelled",
		CancellationMessage:                 "The operator cancelled concurrently.",
	}

	start := make(chan struct{})
	type result struct {
		receipt ChatTurnReceipt
		err     error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		receipt, err := service.CompleteLeasedOperation(ctx, completion)
		results <- result{receipt: receipt, err: err}
	}()
	go func() {
		defer workers.Done()
		<-start
		receipt, err := service.CancelLeasedOperation(ctx, cancellation)
		results <- result{receipt: receipt, err: err}
	}()
	close(start)
	workers.Wait()
	close(results)

	var successes []ChatTurnReceipt
	for current := range results {
		if current.err == nil {
			successes = append(successes, current.receipt)
		}
	}
	if len(successes) != 1 || (successes[0].Status != ChatTurnCompleted && successes[0].Status != ChatTurnCancelled) {
		t.Fatalf("terminal race did not produce one winner: %#v", successes)
	}
	var assistantCount, scheduleCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM observations WHERE tenant_id=$1 AND operation_id=$2`, "leased-terminal-race", prepared.OperationID+":assistant").Scan(&assistantCount); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id=$1 AND continuity_id=$2::uuid`, "leased-terminal-race", prepared.ContinuityID).Scan(&scheduleCount); err != nil {
		t.Fatal(err)
	}
	if successes[0].Status == ChatTurnCompleted {
		if assistantCount != 1 || scheduleCount != 1 {
			t.Fatalf("completed race winner effects mismatch: assistants=%d schedules=%d", assistantCount, scheduleCount)
		}
	} else if assistantCount != 0 || scheduleCount != 0 {
		t.Fatalf("cancelled race winner produced semantic effects: assistants=%d schedules=%d", assistantCount, scheduleCount)
	}
}

func TestLeasedOperationRejectsCrossScopeAndBoundedMutation(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-scope", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:scope-a"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-scope-run",
		Anchor:      anchor,
		Message:     "Keep this operation in scope A.",
	})
	requireNoError(t, err)

	otherTenant := NewConversationService(store, "leased-scope-other", nil, "", ConversationServiceConfig{})
	if _, err := otherTenant.HeartbeatLeasedOperation(ctx, HeartbeatLeasedConversationOperationRequest{LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor)}); err == nil {
		t.Fatal("another tenant mutated the leased operation")
	}
	wrongAnchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:scope-b"}
	if _, err := service.HeartbeatLeasedOperation(ctx, HeartbeatLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: LeasedConversationOperationMutation{
			OperationID: prepared.OperationID,
			Anchor:      wrongAnchor, AttemptID: prepared.AttemptID, LeaseGeneration: prepared.LeaseGeneration,
		},
	}); err == nil {
		t.Fatal("another continuity mutated the leased operation")
	}
	if _, err := service.CompleteExternalTurn(ctx, CompleteExternalConversationTurnRequest{
		OperationID: prepared.OperationID,
		Anchor:      anchor,
		Answer:      "bounded bypass",
		Model:       "legacy/client",
	}); err == nil || !strings.Contains(err.Error(), "leased") {
		t.Fatalf("bounded completion bypassed leased fencing: %v", err)
	}
}

func TestResetForTestClearsLongRunningOperationState(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewConversationService(store, "leased-reset", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:leased-reset"}
	prepared, err := service.PrepareLeasedOperation(ctx, LeasedConversationOperationRequest{
		OperationID: "openclaw:leased-reset-run",
		Anchor:      anchor,
		Message:     "Create leased operation state that test reset must remove.",
	})
	requireNoError(t, err)
	_, err = service.CheckpointLeasedOperation(ctx, CheckpointLeasedConversationOperationRequest{
		LeasedConversationOperationMutation: leasedMutation(prepared.ChatTurnReceipt, anchor),
		Sequence:                            1,
		Checkpoint:                          json.RawMessage(`{"phase":"reset_pending"}`),
	})
	requireNoError(t, err)
	_, err = service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID:     prepared.OperationID,
		Anchor:          anchor,
		RunID:           "leased-reset-run",
		ToolName:        "release.verify",
		ToolCallID:      "reset-tool-call",
		Content:         "The reset fixture recorded one current fenced tool result.",
		AttemptID:       prepared.AttemptID,
		LeaseGeneration: prepared.LeaseGeneration,
	})
	requireNoError(t, err)

	requireNoError(t, store.ResetForTest(ctx))
	var turns, tools, observations int
	requireNoError(t, store.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM conversation_turns),
  (SELECT count(*) FROM conversation_tool_results),
  (SELECT count(*) FROM observations)`).Scan(&turns, &tools, &observations))
	if turns != 0 || tools != 0 || observations != 0 {
		t.Fatalf("test reset retained leased operation state: turns=%d tools=%d observations=%d", turns, tools, observations)
	}
}

func leasedMutation(receipt ChatTurnReceipt, anchor ConversationAnchor) LeasedConversationOperationMutation {
	return LeasedConversationOperationMutation{
		OperationID:     receipt.OperationID,
		Anchor:          anchor,
		AttemptID:       receipt.AttemptID,
		LeaseGeneration: receipt.LeaseGeneration,
	}
}
