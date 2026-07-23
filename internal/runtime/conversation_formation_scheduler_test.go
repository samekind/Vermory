package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/provider"

	"github.com/jackc/pgx/v5"
)

func TestConversationFormationScheduleMigrationIsTenantScopedAndPreservesParentOnRunDeletion(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if version, err := store.SchemaVersion(ctx); err != nil || version != MaximumSupportedSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	for _, column := range []string{
		"tenant_id", "continuity_id", "requested_through_sequence",
		"processed_through_sequence", "schedule_state", "lease_token",
		"lease_expires_at", "attempt_count", "next_attempt_at",
		"window_start_sequence", "window_end_sequence", "window_observation_ids",
		"window_fingerprint", "active_operation_id", "last_run_id", "last_status",
		"last_failure_code", "created_at", "updated_at",
	} {
		var exists bool
		if err := store.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name = 'conversation_formation_schedules'
    AND column_name = $1
)`, column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("conversation_formation_schedules column %q is missing", column)
		}
	}

	var rlsEnabled bool
	var policyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT c.relrowsecurity,
       (SELECT count(*) FROM pg_policy policy WHERE policy.polrelid = c.oid)
FROM pg_class c
WHERE c.oid = 'public.conversation_formation_schedules'::regclass`,
	).Scan(&rlsEnabled, &policyCount); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled || policyCount != 1 {
		t.Fatalf("schedule RLS enabled=%t policies=%d", rlsEnabled, policyCount)
	}

	resolution, err := store.ResolveOrCreateConversation(ctx, "schedule-migration", ConversationAnchor{
		Channel: "openclaw", ThreadID: "migration",
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	if err := store.pool.QueryRow(ctx, `
INSERT INTO source_formation_runs (
  tenant_id, continuity_id, operation_id, request_fingerprint,
  source_ref, source_sha256, source_bytes, active_snapshot,
  active_snapshot_fingerprint, provider_name, requested_model,
  status, completed_at
) VALUES (
  'schedule-migration', $1::uuid, 'schedule-migration-run', repeat('a', 64),
  'fixture:schedule', repeat('b', 64), 1, '[]'::jsonb,
  repeat('c', 64), 'fixture', 'fixture-model', 'completed', now()
)
RETURNING id::text`, resolution.ContinuityID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO conversation_formation_schedules (
  tenant_id, continuity_id, requested_through_sequence,
  processed_through_sequence, schedule_state, last_run_id, last_status
) VALUES ('schedule-migration', $1::uuid, 1, 1, 'idle', $2::uuid, 'completed')`,
		resolution.ContinuityID, runID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `DELETE FROM source_formation_runs WHERE id = $1::uuid`, runID); err != nil {
		t.Fatalf("deleting the optional last run must not delete or invalidate its schedule: %v", err)
	}
	var tenantID, continuityID string
	var lastRunID *string
	if err := store.pool.QueryRow(ctx, `
SELECT tenant_id, continuity_id::text, last_run_id::text
FROM conversation_formation_schedules
WHERE tenant_id = 'schedule-migration'`,
	).Scan(&tenantID, &continuityID, &lastRunID); err != nil {
		t.Fatal(err)
	}
	if tenantID != "schedule-migration" || continuityID != resolution.ContinuityID || lastRunID != nil {
		t.Fatalf("run deletion changed schedule identity: tenant=%q continuity=%q last_run=%v", tenantID, continuityID, lastRunID)
	}

	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}
	var schedules int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_formation_schedules`).Scan(&schedules); err != nil {
		t.Fatal(err)
	}
	if schedules != 0 {
		t.Fatalf("conversation formation schedules survived reset: %d", schedules)
	}
}

func TestCompletedConversationTurnEnqueuesOnceAndFailedTurnDoesNotEnqueue(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewConversationService(store, "schedule-turns", nil, "", ConversationServiceConfig{})
	completedAnchor := ConversationAnchor{Channel: "openclaw", ThreadID: "completed"}
	completed := persistFormationConversationTurn(
		t, service, completedAnchor, "schedule-completed",
		"The submission bundle is thesis-defense-v7.zip.", "Acknowledged.",
	)

	schedule, err := store.ConversationFormationSchedule(ctx, "schedule-turns", completed.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	var userSequence int64
	if err := store.pool.QueryRow(ctx, `SELECT observation_seq FROM observations WHERE id = $1::uuid`, completed.UserObservationID).Scan(&userSequence); err != nil {
		t.Fatal(err)
	}
	if schedule.State != ConversationFormationPending || schedule.RequestedThroughSequence != userSequence || schedule.ProcessedThroughSequence != 0 {
		t.Fatalf("unexpected schedule after completion: %#v", schedule)
	}

	replay, err := service.CompleteExternalTurn(ctx, CompleteExternalConversationTurnRequest{
		OperationID: completed.OperationID,
		Anchor:      completedAnchor,
		Answer:      completed.Answer,
		Model:       completed.Model,
	})
	if err != nil || !replay.Replayed {
		t.Fatalf("completion replay=%#v err=%v", replay, err)
	}
	var scheduleRows int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM conversation_formation_schedules
WHERE tenant_id = 'schedule-turns' AND continuity_id = $1::uuid`, completed.ContinuityID).Scan(&scheduleRows); err != nil {
		t.Fatal(err)
	}
	if scheduleRows != 1 {
		t.Fatalf("completion replay created %d schedule rows", scheduleRows)
	}

	failedAnchor := ConversationAnchor{Channel: "hermes", ThreadID: "failed"}
	prepared, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "schedule-failed",
		Anchor:      failedAnchor,
		Message:     "This turn never produced a visible answer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FailExternalTurn(ctx, FailExternalConversationTurnRequest{
		OperationID: prepared.OperationID,
		Anchor:      failedAnchor,
		FailureCode: "provider_error",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConversationFormationSchedule(ctx, "schedule-turns", prepared.ContinuityID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed turn created a formation schedule: %v", err)
	}

	claim, found, err := store.ClaimConversationFormation(ctx, "schedule-turns", time.Minute, time.Second)
	if err != nil || !found {
		t.Fatalf("claim found=%t err=%v", found, err)
	}
	if len(claim.Observations) != 1 || claim.Observations[0].ID != completed.UserObservationID || claim.Observations[0].Kind != ObservationKindUserMessage {
		t.Fatalf("automatic formation included anything other than the completed user observation: %#v", claim)
	}
}

func TestCompletedTurnSchedulesUserAndToolResultsButExcludesAssistantAndFailedTurns(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewConversationService(store, "schedule-tool-results", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "tool-results"}
	prepared, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "openclaw:schedule-tool-success",
		Anchor:      anchor,
		Message:     "Check current storage.",
	})
	if err != nil {
		t.Fatal(err)
	}
	tool, err := service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID: prepared.OperationID,
		Anchor:      anchor,
		RunID:       "schedule-tool-success",
		ToolName:    "device.storage_check",
		ToolCallID:  "call-storage-success",
		Content:     "Storage has 87 GB available and is 82 percent used.",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.CompleteExternalTurn(ctx, CompleteExternalConversationTurnRequest{
		OperationID: prepared.OperationID,
		Anchor:      anchor,
		Answer:      "Storage check completed.",
		Model:       "fixture-client-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	var toolSequence int64
	if err := store.pool.QueryRow(ctx, `SELECT observation_seq FROM observations WHERE id = $1::uuid`, tool.ObservationID).Scan(&toolSequence); err != nil {
		t.Fatal(err)
	}
	schedule, err := store.ConversationFormationSchedule(ctx, "schedule-tool-results", completed.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.RequestedThroughSequence != toolSequence {
		t.Fatalf("schedule stopped before the final eligible tool result: %#v tool_seq=%d", schedule, toolSequence)
	}
	claim, found, err := store.ClaimConversationFormation(ctx, "schedule-tool-results", time.Minute, time.Second)
	if err != nil || !found {
		t.Fatalf("tool formation claim found=%t err=%v", found, err)
	}
	if len(claim.Observations) != 2 || claim.Observations[0].Kind != ObservationKindUserMessage || claim.Observations[1].Kind != ObservationKindToolResult {
		t.Fatalf("claim did not contain exactly user and tool evidence: %#v", claim.Observations)
	}
	for _, observation := range claim.Observations {
		if observation.ID == completed.AssistantObservationID {
			t.Fatalf("assistant observation entered automatic formation: %#v", claim.Observations)
		}
	}

	failedAnchor := ConversationAnchor{Channel: "openclaw", ThreadID: "failed-tool-result"}
	failedPrepared, err := service.PrepareExternalTurn(ctx, ExternalConversationTurnRequest{
		OperationID: "openclaw:schedule-tool-failed",
		Anchor:      failedAnchor,
		Message:     "Attempt the removal.",
	})
	if err != nil {
		t.Fatal(err)
	}
	failedTool, err := service.RecordToolResult(ctx, RecordConversationToolResultRequest{
		OperationID: failedPrepared.OperationID,
		Anchor:      failedAnchor,
		RunID:       "schedule-tool-failed",
		ToolName:    "device.remove_bundle",
		ToolCallID:  "call-failed-turn",
		Content:     "A partial result that must not be formed after the turn fails.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FailExternalTurn(ctx, FailExternalConversationTurnRequest{
		OperationID: failedPrepared.OperationID,
		Anchor:      failedAnchor,
		FailureCode: "openclaw_agent_error",
	}); err != nil {
		t.Fatal(err)
	}
	formation := NewSourceFormationService(store, "schedule-tool-results", provider.Mock{Output: `{"candidates":[],"reason":"none"}`}, "fixture", "fixture")
	if _, err := formation.FormConversation(ctx, ConversationFormationRequest{
		OperationID:    "failed-tool-result-formation",
		Anchor:         failedAnchor,
		ObservationIDs: []string{failedTool.ObservationID},
	}); err == nil || !strings.Contains(err.Error(), "eligible") {
		t.Fatalf("failed-turn tool result entered formation: %v", err)
	}
}

func TestConversationFormationClaimIsBoundedAndTenantScoped(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewConversationService(store, "schedule-bounds", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "fifty-one"}
	for index := 0; index < 51; index++ {
		persistFormationConversationTurn(t, service, anchor,
			fmt.Sprintf("schedule-bound-%02d", index),
			fmt.Sprintf("Durable fact number %02d.", index), "Acknowledged.")
	}
	claim, found, err := store.ClaimConversationFormation(ctx, "schedule-bounds", time.Minute, time.Second)
	if err != nil || !found {
		t.Fatalf("bounded claim found=%t err=%v", found, err)
	}
	if len(claim.Observations) != conversationFormationWindowLimit {
		t.Fatalf("claim observations=%d want %d", len(claim.Observations), conversationFormationWindowLimit)
	}
	if claim.Schedule.WindowEndSequence >= claim.Schedule.RequestedThroughSequence {
		t.Fatalf("50-observation claim consumed an unbounded request: %#v", claim.Schedule)
	}
	if other, found, err := store.ClaimConversationFormation(ctx, "schedule-other-tenant", time.Minute, time.Second); err != nil || found || other.Schedule.ContinuityID != "" {
		t.Fatalf("tenant worker observed another tenant: claim=%#v found=%t err=%v", other, found, err)
	}

	byteService := NewConversationService(store, "schedule-bytes", nil, "", ConversationServiceConfig{})
	byteAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "byte-bound"}
	for index := 0; index < 4; index++ {
		persistFormationConversationTurn(t, byteService, byteAnchor,
			fmt.Sprintf("schedule-byte-%d", index), strings.Repeat(string(rune('a'+index)), 20_000), "Acknowledged.")
	}
	byteClaim, found, err := store.ClaimConversationFormation(ctx, "schedule-bytes", time.Minute, time.Second)
	if err != nil || !found {
		t.Fatalf("byte-bounded claim found=%t err=%v", found, err)
	}
	totalBytes := 0
	for _, observation := range byteClaim.Observations {
		totalBytes += len([]byte(observation.Content))
	}
	if totalBytes > conversationFormationWindowBytes || len(byteClaim.Observations) != 3 {
		t.Fatalf("byte-bounded claim observations=%d bytes=%d", len(byteClaim.Observations), totalBytes)
	}
}

func TestConversationFormationLeaseRecoveryRejectsChangedWindow(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewConversationService(store, "schedule-window-change", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "window-change"}
	turn := persistFormationConversationTurn(t, service, anchor, "schedule-window-change-turn", "The deadline is Tuesday at 18:00.", "Acknowledged.")
	claim, found, err := store.ClaimConversationFormation(ctx, "schedule-window-change", 10*time.Millisecond, time.Hour)
	if err != nil || !found {
		t.Fatalf("initial claim found=%t err=%v", found, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE observations SET content = 'The deadline changed without governance.' WHERE id = $1::uuid`, turn.UserObservationID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if recovered, found, err := store.ClaimConversationFormation(ctx, "schedule-window-change", time.Minute, time.Hour); err != nil || found || recovered.Schedule.ContinuityID != "" {
		t.Fatalf("changed running window was reclaimed: claim=%#v found=%t err=%v", recovered, found, err)
	}
	schedule, err := store.ConversationFormationSchedule(ctx, "schedule-window-change", claim.Schedule.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.State != ConversationFormationRetryWait || schedule.LastFailureCode != "window_changed" || schedule.ProcessedThroughSequence != 0 {
		t.Fatalf("changed window was not returned safely to retry: %#v", schedule)
	}
}

func TestConversationFormationRetryAndLeaseRecoveryKeepCorrectCursorSemantics(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewConversationService(store, "schedule-retry", nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "retry"}
	persistFormationConversationTurn(t, service, anchor, "schedule-retry-turn", "The upload bundle is thesis-defense-v7.zip.", "Acknowledged.")

	first, found, err := store.ClaimConversationFormation(ctx, "schedule-retry", 10*time.Millisecond, time.Millisecond)
	if err != nil || !found {
		t.Fatalf("first claim found=%t err=%v", found, err)
	}
	time.Sleep(20 * time.Millisecond)
	recovered, found, err := store.ClaimConversationFormation(ctx, "schedule-retry", time.Minute, time.Millisecond)
	if err != nil || !found {
		t.Fatalf("recovered claim found=%t err=%v", found, err)
	}
	if recovered.Schedule.ActiveOperationID != first.Schedule.ActiveOperationID || recovered.Schedule.LeaseToken == first.Schedule.LeaseToken {
		t.Fatalf("lease recovery did not preserve operation and rotate lease: first=%#v recovered=%#v", first.Schedule, recovered.Schedule)
	}

	retried, err := store.RetryConversationFormationClaim(ctx, "schedule-retry", recovered, SourceFormationReceipt{}, "provider_timeout", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ProcessedThroughSequence != 0 || retried.State != ConversationFormationRetryWait || retried.LastFailureCode != "provider_timeout" {
		t.Fatalf("retry advanced or lost state: %#v", retried)
	}
	time.Sleep(5 * time.Millisecond)
	second, found, err := store.ClaimConversationFormation(ctx, "schedule-retry", time.Minute, time.Millisecond)
	if err != nil || !found {
		t.Fatalf("second attempt found=%t err=%v", found, err)
	}
	if second.Schedule.ActiveOperationID == first.Schedule.ActiveOperationID || second.Schedule.AttemptCount != 2 {
		t.Fatalf("retry did not create a distinct attempt: first=%#v second=%#v", first.Schedule, second.Schedule)
	}
}

func TestConversationFormationWorkerReplaysCompletedRunAfterLeaseExpiry(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "schedule-worker-replay"
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "worker-replay"}
	persistFormationConversationTurn(t, service, anchor, "schedule-worker-replay-turn", "The office is B-412.", "Acknowledged.")
	claim, found, err := store.ClaimConversationFormation(ctx, tenantID, 10*time.Millisecond, time.Millisecond)
	if err != nil || !found {
		t.Fatalf("claim found=%t err=%v", found, err)
	}

	llm := &sourceFormationTestProvider{response: provider.GenerateResponse{
		Output: `{"candidates":[],"reason":"No durable candidate selected."}`,
		Model:  "fixture-model",
	}}
	formation := NewSourceFormationService(store, tenantID, llm, "fixture-provider", "fixture-model")
	receipt, err := formation.FormConversationContinuity(ctx, claim.Schedule.ContinuityID, claim.Schedule.ActiveOperationID, []string{claim.Observations[0].ID})
	if err != nil || receipt.Status != SourceFormationAbstained {
		t.Fatalf("formation receipt=%#v err=%v", receipt, err)
	}
	time.Sleep(20 * time.Millisecond)
	worker, err := NewConversationFormationWorker(store, formation, ConversationFormationWorkerOptions{
		TenantID: tenantID, LeaseDuration: time.Minute, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || !result.Replayed || result.FormationRunID != receipt.ID || result.ProcessedThroughSequence == 0 || len(llm.calls) != 1 {
		t.Fatalf("worker did not replay and finish the existing run: result=%#v calls=%d", result, len(llm.calls))
	}
}

func TestConversationFormationWorkerFailureDoesNotAdvanceCursor(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "schedule-worker-failure"
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "hermes", ThreadID: "worker-failure"}
	turn := persistFormationConversationTurn(t, service, anchor, "schedule-worker-failure-turn", "The printer room is C-204.", "Acknowledged.")

	llm := &sourceFormationTestProvider{err: errors.New("provider unavailable")}
	formation := NewSourceFormationService(store, tenantID, llm, "fixture-provider", "fixture-model")
	worker, err := NewConversationFormationWorker(store, formation, ConversationFormationWorkerOptions{
		TenantID: tenantID, LeaseDuration: time.Minute, RetryDelay: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.RunOnce(ctx)
	if err == nil || !result.Found || result.FormationStatus != SourceFormationFailed || result.FailureCode != "provider_error" {
		t.Fatalf("provider failure result=%#v err=%v", result, err)
	}
	schedule, err := store.ConversationFormationSchedule(ctx, tenantID, turn.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ProcessedThroughSequence != 0 || schedule.State != ConversationFormationRetryWait || schedule.LastStatus != SourceFormationFailed || schedule.LastFailureCode != "provider_error" {
		t.Fatalf("provider failure advanced or lost schedule state: %#v", schedule)
	}
}

func TestConversationFormationWorkerStopsCreatingAttemptsAtConfiguredLimit(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "schedule-worker-attempt-limit"
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	turn := persistFormationConversationTurn(t, service, ConversationAnchor{Channel: "openclaw", ThreadID: "attempt-limit"},
		"schedule-worker-attempt-limit-turn", "The submission bundle is thesis-defense-v7.zip.", "Acknowledged.")
	llm := &sourceFormationTestProvider{err: errors.New("provider unavailable")}
	formation := NewSourceFormationService(store, tenantID, llm, "fixture-provider", "fixture-model")
	worker, err := NewConversationFormationWorker(store, formation, ConversationFormationWorkerOptions{
		TenantID: tenantID, LeaseDuration: time.Minute, RetryDelay: time.Millisecond, MaxAttempts: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if result, err := worker.RunOnce(ctx); err == nil || !result.Found {
			t.Fatalf("attempt %d result=%#v err=%v", attempt+1, result, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if result, err := worker.RunOnce(ctx); err != nil || result.Found {
		t.Fatalf("attempt beyond limit result=%#v err=%v", result, err)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("provider calls=%d want 2", len(llm.calls))
	}
	schedule, err := store.ConversationFormationSchedule(ctx, tenantID, turn.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.AttemptCount != 2 || schedule.ProcessedThroughSequence != 0 || schedule.State != ConversationFormationRetryWait {
		t.Fatalf("attempt-limited schedule lost retry state: %#v", schedule)
	}
}

func TestConversationFormationCompletionLeavesNewerTurnPending(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "schedule-newer-turn"
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	anchor := ConversationAnchor{Channel: "openclaw", ThreadID: "newer-turn"}
	persistFormationConversationTurn(t, service, anchor, "schedule-newer-turn-one", "The bundle is thesis-defense-v7.zip.", "Acknowledged.")
	claim, found, err := store.ClaimConversationFormation(ctx, tenantID, time.Minute, time.Second)
	if err != nil || !found {
		t.Fatalf("claim found=%t err=%v", found, err)
	}
	second := persistFormationConversationTurn(t, service, anchor, "schedule-newer-turn-two", "The deadline is Wednesday at 12:00.", "Acknowledged.")

	llm := &sourceFormationTestProvider{response: provider.GenerateResponse{
		Output: `{"candidates":[],"reason":"No durable candidate selected."}`,
		Model:  "fixture-model",
	}}
	formation := NewSourceFormationService(store, tenantID, llm, "fixture-provider", "fixture-model")
	observationIDs := make([]string, len(claim.Observations))
	for index, observation := range claim.Observations {
		observationIDs[index] = observation.ID
	}
	receipt, err := formation.FormConversationContinuity(ctx, claim.Schedule.ContinuityID, claim.Schedule.ActiveOperationID, observationIDs)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteConversationFormationClaim(ctx, tenantID, claim, receipt)
	if err != nil {
		t.Fatal(err)
	}
	var secondSequence int64
	if err := store.pool.QueryRow(ctx, `SELECT observation_seq FROM observations WHERE id = $1::uuid`, second.UserObservationID).Scan(&secondSequence); err != nil {
		t.Fatal(err)
	}
	if completed.State != ConversationFormationPending || completed.ProcessedThroughSequence != claim.Schedule.WindowEndSequence || completed.RequestedThroughSequence != secondSequence {
		t.Fatalf("newer completed turn was lost after worker completion: %#v", completed)
	}
}

func TestConversationFormationConcurrentClaimHasSingleWinner(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "schedule-concurrent-claim"
	service := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	persistFormationConversationTurn(t, service, ConversationAnchor{Channel: "openclaw", ThreadID: "concurrent"},
		"schedule-concurrent-turn", "The bundle is thesis-defense-v7.zip.", "Acknowledged.")

	start := make(chan struct{})
	type claimResult struct {
		found bool
		err   error
	}
	results := make(chan claimResult, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, found, err := store.ClaimConversationFormation(ctx, tenantID, time.Minute, time.Second)
			results <- claimResult{found: found, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	winners := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.found {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent claim winners=%d want 1", winners)
	}
}
