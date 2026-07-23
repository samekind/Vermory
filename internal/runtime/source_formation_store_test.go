package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const sourceFormationTestDocument = `# Deployment Operations Revision

Primary production region remains us-east-1.
Production deployments now retry at most 5 times.
Rollback approval requires two maintainers.
`

func TestSourceFormationStoreCompletesAtomicBatch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "formation-batch"
	repoRoot := "/fixtures/formation-batch"
	service := NewGovernanceService(store, tenantID)
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	region := addSourceMatchFact(t, service, repoRoot, "formation-region", "deploy.region.primary", "Production deploys to us-east-1.", "fixture:region")
	retry := addSourceMatchFact(t, service, repoRoot, "formation-retry", "deploy.retry.max", "Production deployments retry at most 3 times.", "fixture:retry")
	addSourceMatchFact(t, service, repoRoot, "formation-slsa", "release.attestation.format", "Production releases publish a signed SLSA provenance statement.", "fixture:slsa")
	otherService := NewGovernanceService(store, "formation-other")
	if _, err := otherService.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, otherService, repoRoot, "formation-other-secret", "deploy.secret.mode", "Production deployments use static cloud credentials.", "fixture:other")

	request := sourceFormationBeginRequest("formation-batch-run", sourceFormationTestDocument)
	begin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if begin.Status != SourceFormationPending || begin.Replayed || len(begin.ActiveSnapshot) != 3 {
		t.Fatalf("unexpected formation begin: %#v", begin)
	}
	for _, candidate := range begin.ActiveSnapshot {
		if strings.Contains(candidate.Content, "static cloud credentials") {
			t.Fatalf("cross-tenant fact entered formation snapshot: %#v", begin.ActiveSnapshot)
		}
	}

	completed, err := store.CompleteSourceFormation(ctx, tenantID, begin.ID, []byte(sourceFormationTestDocument), SourceFormationCompletion{
		Status:                 SourceFormationCompleted,
		ResolvedModel:          "resolved-test-model",
		ProviderOutput:         `{"candidates":[],"reason":"fixture output"}`,
		ProviderArtifactSHA256: strings.Repeat("a", 64),
		Reason:                 "three durable facts",
		Items: []SourceFormationProviderItem{
			{
				Decision:   SourceFormationUnchanged,
				MemoryKey:  "deploy.region.primary",
				Quote:      "Primary production region remains us-east-1.",
				Occurrence: 1,
				Content:    "Production deploys to us-east-1.",
				Reason:     "same region",
			},
			{
				Decision:   SourceFormationUpdate,
				MemoryKey:  "deploy.retry.max",
				Quote:      "Production deployments now retry at most 5 times.",
				Occurrence: 1,
				Content:    "Production deployments retry at most 5 times.",
				Reason:     "retry limit changed",
			},
			{
				Decision:   SourceFormationNew,
				MemoryKey:  "deploy.rollback.approvals",
				Quote:      "Rollback approval requires two maintainers.",
				Occurrence: 1,
				Content:    "Rollback approval requires two maintainers.",
				Reason:     "new rollback rule",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != SourceFormationCompleted || len(completed.Items) != 3 {
		t.Fatalf("unexpected completed formation: %#v", completed)
	}
	unchanged := formationItemByKey(t, completed.Items, "deploy.region.primary")
	if unchanged.Decision != SourceFormationUnchanged || unchanged.TargetMemoryID != region.Memory.MemoryID || unchanged.CandidateMemoryID != "" || unchanged.ObservationID == "" {
		t.Fatalf("unexpected unchanged item: %#v", unchanged)
	}
	updated := formationItemByKey(t, completed.Items, "deploy.retry.max")
	if updated.Decision != SourceFormationUpdate || updated.TargetMemoryID != retry.Memory.MemoryID || updated.CandidateMemoryID == "" || updated.CandidateStatus != "proposed" {
		t.Fatalf("unexpected update item: %#v", updated)
	}
	created := formationItemByKey(t, completed.Items, "deploy.rollback.approvals")
	if created.Decision != SourceFormationNew || created.TargetMemoryID != "" || created.CandidateMemoryID == "" || created.CandidateStatus != "proposed" {
		t.Fatalf("unexpected new item: %#v", created)
	}
	for _, item := range completed.Items {
		if item.ByteStart < 0 || item.ByteEnd <= item.ByteStart || item.ByteEnd-item.ByteStart != len(item.Quote) {
			t.Fatalf("formation item has invalid byte span: %#v", item)
		}
	}
	assertSourceCandidateSearch(t, store, tenantID, resolution.ContinuityID, "Production deployments retry at most 3 times.", true)
	assertSourceCandidateSearch(t, store, tenantID, resolution.ContinuityID, "Production deployments retry at most 5 times.", false)
	assertSourceCandidateSearch(t, store, tenantID, resolution.ContinuityID, "Rollback approval requires two maintainers.", false)

	replayed, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.ID != begin.ID || len(replayed.Items) != 3 {
		t.Fatalf("formation replay did not return stored batch: %#v", replayed)
	}
	inspected, err := store.InspectSourceFormation(ctx, tenantID, resolution.ContinuityID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.ID != begin.ID || inspected.ProviderOutput == "" || len(inspected.Items) != 3 {
		t.Fatalf("unexpected formation inspection: %#v", inspected)
	}
}

func TestSourceFormationStorePersistsAbstentionAndFailsInvalidBatchAtomically(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "formation-invalid"
	repoRoot := "/fixtures/formation-invalid"
	service := NewGovernanceService(store, tenantID)
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "formation-invalid-retry", "deploy.retry.max", "Production deployments retry at most 3 times.", "fixture:retry")

	abstainDocument := "The fallback policy remains undecided.\n"
	abstainBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, sourceFormationBeginRequest("formation-abstain", abstainDocument))
	if err != nil {
		t.Fatal(err)
	}
	abstained, err := store.CompleteSourceFormation(ctx, tenantID, abstainBegin.ID, []byte(abstainDocument), SourceFormationCompletion{
		Status:         SourceFormationAbstained,
		ResolvedModel:  "test-model",
		ProviderOutput: `{"candidates":[],"reason":"nothing safe"}`,
		Reason:         "nothing safe to retain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if abstained.Status != SourceFormationAbstained || len(abstained.Items) != 0 {
		t.Fatalf("unexpected formation abstention: %#v", abstained)
	}

	invalidDocument := "Retry at most 5 times.\nRollback approval requires two maintainers.\n"
	invalidBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, sourceFormationBeginRequest("formation-duplicate", invalidDocument))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.CompleteSourceFormation(ctx, tenantID, invalidBegin.ID, []byte(invalidDocument), SourceFormationCompletion{
		Status:        SourceFormationCompleted,
		ResolvedModel: "test-model",
		Reason:        "duplicate fixture",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "deploy.retry.max", Quote: "Retry at most 5 times.", Occurrence: 1, Content: "Production deployments retry at most 5 times.", Reason: "update"},
			{Decision: SourceFormationNew, MemoryKey: "deploy.retry.max", Quote: "Rollback approval requires two maintainers.", Occurrence: 1, Content: "Rollback approval requires two maintainers.", Reason: "duplicate"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != SourceFormationFailed || failed.FailureCode != "duplicate_memory_key" || len(failed.Items) != 0 {
		t.Fatalf("invalid batch did not fail atomically: %#v", failed)
	}
	var effects int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM observations
WHERE tenant_id = $1 AND operation_id LIKE 'source-formation:%'`, tenantID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 0 {
		t.Fatalf("invalid formation batch created observations: %d", effects)
	}
}

func TestSourceFormationStoreFailsOnSnapshotDriftAndExpiresPendingReplay(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "formation-drift"
	repoRoot := "/fixtures/formation-drift"
	service := NewGovernanceService(store, tenantID)
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "formation-drift-retry", "deploy.retry.max", "Retry at most 3 times.", "fixture:retry")
	document := "Retry at most 5 times.\n"

	driftRequest := sourceFormationBeginRequest("formation-drift-run", document)
	driftBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, driftRequest)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "formation-drift-new", "deploy.region.primary", "Deploy to us-east-1.", "fixture:region")
	drifted, err := store.CompleteSourceFormation(ctx, tenantID, driftBegin.ID, []byte(document), SourceFormationCompletion{
		Status:        SourceFormationCompleted,
		ResolvedModel: "test-model",
		Reason:        "retry changed",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "deploy.retry.max", Quote: "Retry at most 5 times.", Occurrence: 1, Content: "Retry at most 5 times.", Reason: "update"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if drifted.Status != SourceFormationFailed || drifted.FailureCode != "active_snapshot_changed" || len(drifted.Items) != 0 {
		t.Fatalf("snapshot drift did not fail closed: %#v", drifted)
	}
	if _, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, driftRequest); err == nil || !strings.Contains(err.Error(), "active snapshot has changed") {
		t.Fatalf("changed snapshot replayed old formation: %v", err)
	}

	expiryRequest := sourceFormationBeginRequest("formation-expiry-run", document)
	expiryBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, expiryRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE source_formation_runs
SET created_at = now() - interval '10 minutes'
WHERE id = $1::uuid`, expiryBegin.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, expiryRequest)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status != SourceFormationFailed || expired.FailureCode != "pending_expired" || !expired.Replayed {
		t.Fatalf("pending formation did not expire: %#v", expired)
	}
}

func TestSourceFormationStoreRejectsConflictingReplayAndInvalidSpansAtomically(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "formation-invalid-spans"
	repoRoot := "/fixtures/formation-invalid-spans"
	service := NewGovernanceService(store, tenantID)
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "formation-span-retry", "deploy.retry.max", "Retry at most 3 times.", "fixture:retry")

	document := "Retry at most 5 times.\nRollback approval requires two maintainers.\n"
	request := sourceFormationBeginRequest("formation-conflicting-replay", document)
	if _, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, request); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.SourceRef = "fixture:changed-logical-request"
	if _, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, changed); err == nil || !strings.Contains(err.Error(), "another logical source formation") {
		t.Fatalf("conflicting operation replay was accepted: %v", err)
	}

	missingBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, sourceFormationBeginRequest("formation-missing-occurrence", document))
	if err != nil {
		t.Fatal(err)
	}
	missing, err := store.CompleteSourceFormation(ctx, tenantID, missingBegin.ID, []byte(document), SourceFormationCompletion{
		Status:        SourceFormationCompleted,
		ResolvedModel: "test-model",
		Reason:        "invalid occurrence fixture",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "deploy.retry.max", Quote: "Retry at most 5 times.", Occurrence: 2, Content: "Retry at most 5 times.", Reason: "update"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != SourceFormationFailed || missing.FailureCode != "quote_occurrence_not_found" {
		t.Fatalf("missing quote occurrence did not fail closed: %#v", missing)
	}

	overlapBegin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, sourceFormationBeginRequest("formation-overlap", document))
	if err != nil {
		t.Fatal(err)
	}
	overlap, err := store.CompleteSourceFormation(ctx, tenantID, overlapBegin.ID, []byte(document), SourceFormationCompletion{
		Status:        SourceFormationCompleted,
		ResolvedModel: "test-model",
		Reason:        "overlap fixture",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "deploy.retry.max", Quote: "Retry at most 5 times.", Occurrence: 1, Content: "Retry at most 5 times.", Reason: "update"},
			{Decision: SourceFormationNew, MemoryKey: "deploy.retry.fragment", Quote: "at most 5 times", Occurrence: 1, Content: "Retry fragment is five.", Reason: "overlap"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if overlap.Status != SourceFormationFailed || overlap.FailureCode != "overlapping_source_spans" {
		t.Fatalf("overlapping source spans did not fail closed: %#v", overlap)
	}

	var effects int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM observations
WHERE tenant_id = $1 AND operation_id LIKE 'source-formation:%'`, tenantID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 0 {
		t.Fatalf("invalid source spans created observations: %d", effects)
	}
}

func TestSourceFormationForgetRedactsAuditAndTerminatesPendingRun(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "formation-redaction"
	repoRoot := "/fixtures/formation-redaction"
	service := NewGovernanceService(store, tenantID)
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	oldContent := "Use FORMATION-OLD-SECRET for production signing."
	newContent := "Use FORMATION-NEW-OIDC for production signing."
	old := addSourceMatchFact(t, service, repoRoot, "formation-redaction-old", "release.signing.mode", oldContent, "fixture:formation-old-secret")
	document := newContent + "\n"
	beginRequest := sourceFormationBeginRequest("formation-redaction-run", document)
	beginRequest.SourceRef = "fixture:formation-new-secret"
	begin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, beginRequest)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteSourceFormation(ctx, tenantID, begin.ID, []byte(document), SourceFormationCompletion{
		Status:         SourceFormationCompleted,
		ResolvedModel:  "test-model",
		ProviderOutput: `{"items":[{"reason":"FORMATION-OLD-SECRET becomes FORMATION-NEW-OIDC"}]}`,
		Reason:         "FORMATION-OLD-SECRET becomes FORMATION-NEW-OIDC",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "release.signing.mode", Quote: newContent, Occurrence: 1, Content: newContent, Reason: "FORMATION-OLD-SECRET becomes FORMATION-NEW-OIDC"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := formationItemByKey(t, completed.Items, "release.signing.mode")
	if _, err := service.AcceptCandidate(ctx, repoRoot, candidate.CandidateMemoryID, "formation-redaction-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Forget(ctx, repoRoot, candidate.CandidateMemoryID, "formation-redaction-forget-new"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Forget(ctx, repoRoot, old.Memory.MemoryID, "formation-redaction-forget-old"); err != nil {
		t.Fatal(err)
	}

	inspected, err := store.InspectSourceFormation(ctx, tenantID, resolution.ContinuityID, beginRequest.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	encodedSnapshot, err := json.Marshal(inspected.ActiveSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	values := []string{inspected.SourceRef, inspected.ProviderOutput, inspected.Reason, string(encodedSnapshot)}
	for _, item := range inspected.Items {
		values = append(values, item.Quote, item.Content, item.Reason)
	}
	for _, value := range values {
		for _, forbidden := range []string{"FORMATION-OLD-SECRET", "FORMATION-NEW-OIDC", "fixture:formation-old-secret", "fixture:formation-new-secret"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("forgotten source formation content survived in %q", value)
			}
		}
	}
	if inspected.SourceRef != "[redacted]" || inspected.ProviderOutput != "[redacted]" || inspected.Reason != "[redacted]" {
		t.Fatalf("source formation audit was not redacted consistently: %#v", inspected)
	}
	if len(inspected.Items) != 1 || inspected.Items[0].Quote != "[redacted]" || inspected.Items[0].Content != "[redacted]" || inspected.Items[0].Reason != "[redacted]" {
		t.Fatalf("source formation item was not redacted consistently: %#v", inspected.Items)
	}

	pendingTarget := addSourceMatchFact(t, service, repoRoot, "formation-pending-target", "deploy.pending.secret", "PENDING-FORMATION-SECRET", "fixture:pending-formation-secret")
	pendingDocument := "Replace the pending deployment secret.\n"
	pendingRequest := sourceFormationBeginRequest("formation-pending-run", pendingDocument)
	pending, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, pendingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Forget(ctx, repoRoot, pendingTarget.Memory.MemoryID, "formation-pending-forget"); err != nil {
		t.Fatal(err)
	}
	late, err := store.CompleteSourceFormation(ctx, tenantID, pending.ID, []byte(pendingDocument), SourceFormationCompletion{
		Status:         SourceFormationCompleted,
		ResolvedModel:  "test-model",
		ProviderOutput: `{"reason":"PENDING-FORMATION-SECRET"}`,
		Reason:         "PENDING-FORMATION-SECRET",
		Items: []SourceFormationProviderItem{
			{Decision: SourceFormationUpdate, MemoryKey: "deploy.pending.secret", Quote: "Replace the pending deployment secret.", Occurrence: 1, Content: "Use a replacement secret.", Reason: "PENDING-FORMATION-SECRET"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if late.Status != SourceFormationFailed || late.FailureCode != "referenced_memory_deleted" || !late.Replayed {
		t.Fatalf("pending source formation was not terminated by deletion: %#v", late)
	}
	pendingInspection, err := store.InspectSourceFormation(ctx, tenantID, resolution.ContinuityID, pendingRequest.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	pendingSnapshot, err := json.Marshal(pendingInspection.ActiveSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{pendingInspection.ProviderOutput, pendingInspection.Reason, string(pendingSnapshot)} {
		if strings.Contains(value, "PENDING-FORMATION-SECRET") {
			t.Fatalf("late provider completion restored forgotten content: %#v", pendingInspection)
		}
	}
}

func sourceFormationBeginRequest(operationID, document string) SourceFormationBeginRequest {
	return SourceFormationBeginRequest{
		OperationID:    operationID,
		SourceRef:      "fixture:" + operationID,
		SourceSHA256:   sourceMatchSHA256([]byte(document)),
		SourceBytes:    len([]byte(document)),
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	}
}

func formationItemByKey(t *testing.T, items []SourceFormationItemReceipt, key string) SourceFormationItemReceipt {
	t.Helper()
	for _, item := range items {
		if item.MemoryKey == key {
			return item
		}
	}
	t.Fatalf("formation item %q not found: %#v", key, items)
	return SourceFormationItemReceipt{}
}

func TestSourceFormationPendingExpiryConstantCoversProviderDeadline(t *testing.T) {
	if sourceFormationPendingExpiry <= defaultSourceMatchProviderTimeout {
		t.Fatalf("pending expiry %s must exceed provider timeout %s", sourceFormationPendingExpiry, defaultSourceMatchProviderTimeout)
	}
	if sourceFormationPendingExpiry != defaultSourceMatchProviderTimeout+time.Minute {
		t.Fatalf("unexpected formation pending expiry: %s", sourceFormationPendingExpiry)
	}
}

func TestConversationFormationStoreBindsManifestAndEvidence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "conversation-formation-store"
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "store-formation"}
	conversation := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	turn := persistFormationConversationTurn(
		t,
		conversation,
		anchor,
		"store-formation-turn",
		"The maintenance visit is Saturday at 10:00.",
		"Acknowledged.",
	)
	resolution, err := store.ResolveConversation(ctx, tenantID, anchor)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := store.SelectConversationFormationObservations(ctx, tenantID, resolution.ContinuityID, []string{turn.UserObservationID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	manifest := sourceFormationManifestFromObservations(observations)
	begin, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, SourceFormationBeginRequest{
		OperationID:    "store-conversation-formation",
		SourceRef:      "conversation:" + resolution.ContinuityID + "@1-1",
		SourceSHA256:   sourceFormationInputManifestFingerprint(manifest),
		SourceBytes:    len([]byte(observations[0].Content)),
		InputKind:      SourceFormationInputConversation,
		InputManifest:  manifest,
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteConversationFormation(ctx, tenantID, begin.ID, SourceFormationCompletion{
		Status:        SourceFormationCompleted,
		ResolvedModel: "test-model",
		Reason:        "One exact fact.",
		Items: []SourceFormationProviderItem{{
			Decision:            SourceFormationNew,
			MemoryKey:           "maintenance.visit.current",
			SourceObservationID: turn.UserObservationID,
			Quote:               "The maintenance visit is Saturday at 10:00.",
			Occurrence:          1,
			Content:             "The maintenance visit is Saturday at 10:00.",
			Reason:              "Explicit schedule.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.InputKind != SourceFormationInputConversation || len(completed.Items) != 1 ||
		completed.Items[0].EvidenceObservationID != turn.UserObservationID || completed.Items[0].CandidateMemoryID == "" {
		t.Fatalf("conversation formation store did not bind evidence: %#v", completed)
	}

	if _, err := store.BeginSourceFormation(ctx, tenantID, resolution.ContinuityID, sourceFormationBeginRequest(
		"store-document-on-conversation",
		"The maintenance visit is Saturday at 10:00.",
	)); err == nil || !strings.Contains(err.Error(), "workspace continuity") {
		t.Fatalf("document formation attached to conversation continuity: %v", err)
	}
}
