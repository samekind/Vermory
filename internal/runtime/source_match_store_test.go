package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSourceMatchStoreCreatesAuditedProposedCandidate(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	local := NewGovernanceService(store, "match-local")
	other := NewGovernanceService(store, "match-other")
	repoRoot := "/fixtures/source-match-store"
	resolution, err := local.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	signing := addSourceMatchFact(t, local, repoRoot, "match-signing", "release.signing.mode", "Use a macOS keychain certificate.", "fixture:signing:old")
	addSourceMatchFact(t, local, repoRoot, "match-timeout", "deploy.api.timeout", "The deployment API timeout is 800 ms.", "fixture:timeout")
	addSourceMatchFact(t, local, repoRoot, "match-attestation", "release.attestation.format", "Publish a signed SLSA provenance statement.", "fixture:attestation")
	addSourceMatchFact(t, other, repoRoot, "match-other-signing", "release.signing.mode", "Use static cloud credentials.", "fixture:other")

	request := SourceMatchBeginRequest{
		OperationID:    "source-match-store-one",
		SourceRef:      "fixture:signing:new",
		SourceContent:  "Use GitHub Actions OIDC keyless signing.",
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	}
	begin, err := store.BeginSourceMatch(ctx, "match-local", resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if begin.Status != SourceMatchPending || begin.ID == "" || begin.Replayed {
		t.Fatalf("unexpected begin receipt: %#v", begin)
	}
	if len(begin.CandidateSet) != 3 {
		t.Fatalf("candidate set crossed scope or lost facts: %#v", begin.CandidateSet)
	}
	assertSourceMatchCandidate(t, begin.CandidateSet, signing.Memory.MemoryID, "release.signing.mode", "fixture:signing:old")
	for _, candidate := range begin.CandidateSet {
		if strings.Contains(candidate.Content, "static cloud credentials") {
			t.Fatalf("cross-tenant fact entered candidate set: %#v", begin.CandidateSet)
		}
	}

	replay, err := store.BeginSourceMatch(ctx, "match-local", resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.ID != begin.ID || replay.CandidateSetFingerprint != begin.CandidateSetFingerprint {
		t.Fatalf("begin replay changed identity: begin=%#v replay=%#v", begin, replay)
	}
	conflict := request
	conflict.SourceContent = "Use a different signing mechanism."
	if _, err := store.BeginSourceMatch(ctx, "match-local", resolution.ContinuityID, conflict); err == nil || !strings.Contains(err.Error(), "another logical source match") {
		t.Fatalf("conflicting replay was accepted: %v", err)
	}

	matched, err := store.CompleteSourceMatch(ctx, "match-local", begin.ID, SourceMatchCompletion{
		Decision:               SourceMatchMatched,
		SelectedMemoryKey:      "release.signing.mode",
		ResolvedModel:          "resolved-test-model",
		ProviderOutput:         `{"decision":"matched","memory_key":"release.signing.mode","reason":"signing changed"}`,
		ProviderArtifactSHA256: strings.Repeat("a", 64),
		Reason:                 "signing changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched.Status != SourceMatchMatched || matched.TargetMemoryID != signing.Memory.MemoryID ||
		matched.ObservationID == "" || matched.CandidateMemoryID == "" || matched.Disposition != SourceCandidateReplacement {
		t.Fatalf("unexpected matched receipt: %#v", matched)
	}
	memories, err := store.ListGovernedMemories(ctx, "match-local", resolution.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateMemory(t, memories, signing.Memory.MemoryID, "release.signing.mode", "active", "")
	assertSourceCandidateMemory(t, memories, matched.CandidateMemoryID, "release.signing.mode", "proposed", signing.Memory.MemoryID)
	assertSourceCandidateSearch(t, store, "match-local", resolution.ContinuityID, request.SourceContent, false)
	assertSourceCandidateSearch(t, store, "match-local", resolution.ContinuityID, "Use a macOS keychain certificate.", true)
	assertSourceCandidateSearch(t, store, "match-local", resolution.ContinuityID, "signing changed", false)

	inspected, err := store.InspectSourceMatch(ctx, "match-local", resolution.ContinuityID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.ID != begin.ID || inspected.Status != SourceMatchMatched || inspected.ProviderOutput == "" {
		t.Fatalf("unexpected inspection: %#v", inspected)
	}
	if _, err := store.InspectSourceMatch(ctx, "match-other", resolution.ContinuityID, request.OperationID); err == nil {
		t.Fatal("cross-tenant source match inspection succeeded")
	}
}

func TestSourceMatchStoreHandlesUnchangedAbstainedAndFailedDecisions(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-terminal")
	repoRoot := "/fixtures/source-match-terminal"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "terminal-timeout", "deploy.api.timeout", "The timeout is 800 ms.", "fixture:timeout")

	unchangedBegin := beginSourceMatchForTest(t, store, "source-match-terminal", resolution.ContinuityID, "terminal-unchanged", "The timeout is 800 ms.")
	unchanged, err := store.CompleteSourceMatch(ctx, "source-match-terminal", unchangedBegin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "deploy.api.timeout",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"deploy.api.timeout","reason":"same fact"}`,
		Reason:            "same fact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Disposition != SourceCandidateUnchanged || unchanged.ObservationID == "" || unchanged.CandidateMemoryID != "" {
		t.Fatalf("unchanged match created a candidate: %#v", unchanged)
	}

	abstainBegin := beginSourceMatchForTest(t, store, "source-match-terminal", resolution.ContinuityID, "terminal-abstain", "No listed fact matches this source.")
	abstained, err := store.CompleteSourceMatch(ctx, "source-match-terminal", abstainBegin.ID, SourceMatchCompletion{
		Decision:       SourceMatchAbstained,
		ResolvedModel:  "test-model",
		ProviderOutput: `{"decision":"abstained","memory_key":"","reason":"no safe target"}`,
		Reason:         "no safe target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if abstained.Status != SourceMatchAbstained || abstained.CandidateMemoryID != "" || abstained.Reason != "no safe target" {
		t.Fatalf("unexpected abstain receipt: %#v", abstained)
	}

	failedBegin := beginSourceMatchForTest(t, store, "source-match-terminal", resolution.ContinuityID, "terminal-failed", "Provider fails for this source.")
	failed, err := store.CompleteSourceMatch(ctx, "source-match-terminal", failedBegin.ID, SourceMatchCompletion{
		Decision:    SourceMatchFailed,
		FailureCode: "provider_timeout",
		Reason:      "provider deadline exceeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != SourceMatchFailed || failed.FailureCode != "provider_timeout" || failed.CandidateMemoryID != "" {
		t.Fatalf("unexpected failed receipt: %#v", failed)
	}
}

func TestSourceMatchReplayRejectsChangedCandidateSnapshot(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-replay-snapshot")
	repoRoot := "/fixtures/source-match-replay-snapshot"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "replay-snapshot-signing", "release.signing.mode", "Use signer A.", "fixture:signer:a")
	request := SourceMatchBeginRequest{
		OperationID:    "replay-snapshot-match",
		SourceRef:      "fixture:signer:b",
		SourceContent:  "Use signer B.",
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	}
	begin, err := store.BeginSourceMatch(ctx, "source-match-replay-snapshot", resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteSourceMatch(ctx, "source-match-replay-snapshot", begin.ID, SourceMatchCompletion{
		Decision:       SourceMatchAbstained,
		ResolvedModel:  "test-model",
		ProviderOutput: `{"decision":"abstained","memory_key":"","reason":"no safe target"}`,
		Reason:         "no safe target",
	}); err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "replay-snapshot-timeout", "deploy.api.timeout", "The timeout is 800 ms.", "fixture:timeout")
	if _, err := store.BeginSourceMatch(ctx, "source-match-replay-snapshot", resolution.ContinuityID, request); err == nil || !strings.Contains(err.Error(), "candidate snapshot has changed") {
		t.Fatalf("changed candidate snapshot replayed an old decision: %v", err)
	}
}

func TestSourceMatchReplayExpiresOrphanedPendingDecision(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-pending-expiry")
	repoRoot := "/fixtures/source-match-pending-expiry"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "pending-expiry-signing", "release.signing.mode", "Use signer A.", "fixture:signer:a")
	request := SourceMatchBeginRequest{
		OperationID:    "pending-expiry-match",
		SourceRef:      "fixture:signer:b",
		SourceContent:  "Use signer B.",
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	}
	begin, err := store.BeginSourceMatch(ctx, "source-match-pending-expiry", resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE source_match_decisions
SET created_at = now() - interval '10 minutes'
WHERE id = $1::uuid`, begin.ID); err != nil {
		t.Fatal(err)
	}
	expired, err := store.BeginSourceMatch(ctx, "source-match-pending-expiry", resolution.ContinuityID, request)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status != SourceMatchFailed || expired.FailureCode != "pending_expired" || !expired.Replayed {
		t.Fatalf("orphaned pending match did not expire: %#v", expired)
	}
}

func TestSourceMatchStoreRejectsInvalidAmbiguousAndDriftedTargets(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-invalid")
	repoRoot := "/fixtures/source-match-invalid"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	addSourceMatchFact(t, service, repoRoot, "invalid-signing", "release.signing.mode", "Use signer A.", "fixture:signing:a")

	outsideBegin := beginSourceMatchForTest(t, store, "source-match-invalid", resolution.ContinuityID, "invalid-outside", "Use signer B.")
	outside, err := store.CompleteSourceMatch(ctx, "source-match-invalid", outsideBegin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "finance.secret",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"finance.secret","reason":"injected"}`,
		Reason:            "injected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if outside.Status != SourceMatchFailed || outside.FailureCode != "selected_key_outside_candidate_set" {
		t.Fatalf("outside key was not rejected: %#v", outside)
	}

	driftBegin := beginSourceMatchForTest(t, store, "source-match-invalid", resolution.ContinuityID, "invalid-drift", "Use signer C.")
	addSourceMatchFact(t, service, repoRoot, "invalid-new-fact", "release.rollout.mode", "Use a staged rollout.", "fixture:rollout")
	drifted, err := store.CompleteSourceMatch(ctx, "source-match-invalid", driftBegin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "release.signing.mode",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"release.signing.mode","reason":"signing changed"}`,
		Reason:            "signing changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if drifted.Status != SourceMatchFailed || drifted.FailureCode != "candidate_set_changed" || drifted.CandidateMemoryID != "" {
		t.Fatalf("drifted set created a candidate: %#v", drifted)
	}

	addSourceMatchFact(t, service, repoRoot, "invalid-duplicate", "release.signing.mode", "Use signer D.", "fixture:signing:d")
	ambiguousBegin := beginSourceMatchForTest(t, store, "source-match-invalid", resolution.ContinuityID, "invalid-ambiguous", "Use signer E.")
	ambiguous, err := store.CompleteSourceMatch(ctx, "source-match-invalid", ambiguousBegin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "release.signing.mode",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"release.signing.mode","reason":"signing changed"}`,
		Reason:            "signing changed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Status != SourceMatchFailed || ambiguous.FailureCode != "selected_key_is_ambiguous" || ambiguous.CandidateMemoryID != "" {
		t.Fatalf("ambiguous key created a candidate: %#v", ambiguous)
	}
}

func TestSourceMatchAuditRedactsForgottenMemoryContent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-redaction")
	repoRoot := "/fixtures/source-match-redaction"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	oldContent := "Use REDACTION-OLD-KEYCHAIN for production signing."
	newContent := "Use REDACTION-NEW-OIDC for production signing."
	old := addSourceMatchFact(t, service, repoRoot, "redaction-old", "release.signing.mode", oldContent, "fixture:redaction:old")
	begin := beginSourceMatchForTest(t, store, "source-match-redaction", resolution.ContinuityID, "redaction-match", newContent)
	matched, err := store.CompleteSourceMatch(ctx, "source-match-redaction", begin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "release.signing.mode",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"release.signing.mode","reason":"REDACTION-OLD-KEYCHAIN becomes REDACTION-NEW-OIDC"}`,
		Reason:            "REDACTION-OLD-KEYCHAIN becomes REDACTION-NEW-OIDC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptCandidate(ctx, repoRoot, matched.CandidateMemoryID, "redaction-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Forget(ctx, repoRoot, matched.CandidateMemoryID, "redaction-forget-new"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Forget(ctx, repoRoot, old.Memory.MemoryID, "redaction-forget-old"); err != nil {
		t.Fatal(err)
	}

	var sourceRef, sourceContent, candidateSet, candidateFingerprint, providerOutput, reason string
	if err := store.pool.QueryRow(ctx, `
SELECT source_ref, source_content, candidate_set::text, candidate_set_fingerprint,
       provider_output, reason
FROM source_match_decisions
WHERE tenant_id = 'source-match-redaction' AND operation_id = 'redaction-match'`).Scan(
		&sourceRef,
		&sourceContent,
		&candidateSet,
		&candidateFingerprint,
		&providerOutput,
		&reason,
	); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{sourceRef, sourceContent, candidateSet, providerOutput, reason} {
		for _, forbidden := range []string{"REDACTION-OLD-KEYCHAIN", "REDACTION-NEW-OIDC", "fixture:redaction:old", "fixture:redaction-match"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("forgotten source match content survived in %q", value)
			}
		}
	}
	if sourceRef != "[redacted]" || sourceContent != "[redacted]" || providerOutput != "[redacted]" || reason != "[redacted]" {
		t.Fatalf("source match audit was not redacted consistently: ref=%q content=%q output=%q reason=%q", sourceRef, sourceContent, providerOutput, reason)
	}
	if len(candidateFingerprint) != 64 || !strings.Contains(candidateSet, "[redacted]") {
		t.Fatalf("candidate set redaction/fingerprint mismatch: set=%s fingerprint=%s", candidateSet, candidateFingerprint)
	}
	inspected, err := store.InspectSourceMatch(ctx, "source-match-redaction", resolution.ContinuityID, "redaction-match")
	if err != nil {
		t.Fatal(err)
	}
	_, canonicalFingerprint, err := canonicalSourceMatchCandidates(inspected.CandidateSet)
	if err != nil {
		t.Fatal(err)
	}
	if candidateFingerprint != canonicalFingerprint {
		t.Fatalf("redacted candidate fingerprint is not canonical: stored=%s canonical=%s", candidateFingerprint, canonicalFingerprint)
	}
}

func TestSourceMatchForgetTerminatesPendingDecisionWithoutProviderRewrite(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "source-match-pending-forget")
	repoRoot := "/fixtures/source-match-pending-forget"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	secret := "PENDING-FORGET-SECRET"
	target := addSourceMatchFact(t, service, repoRoot, "pending-forget-target", "release.signing.mode", secret, "fixture:pending-forget")
	begin := beginSourceMatchForTest(t, store, "source-match-pending-forget", resolution.ContinuityID, "pending-forget-match", "Use a replacement signer.")
	if _, err := service.Forget(ctx, repoRoot, target.Memory.MemoryID, "pending-forget-delete"); err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteSourceMatch(ctx, "source-match-pending-forget", begin.ID, SourceMatchCompletion{
		Decision:          SourceMatchMatched,
		SelectedMemoryKey: "release.signing.mode",
		ResolvedModel:     "test-model",
		ProviderOutput:    `{"decision":"matched","memory_key":"release.signing.mode","reason":"PENDING-FORGET-SECRET"}`,
		Reason:            secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != SourceMatchFailed || completed.FailureCode != "referenced_memory_deleted" || !completed.Replayed {
		t.Fatalf("pending source match was not terminated by deletion: %#v", completed)
	}
	inspected, err := store.InspectSourceMatch(ctx, "source-match-pending-forget", resolution.ContinuityID, "pending-forget-match")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{inspected.SourceContent, inspected.ProviderOutput, inspected.Reason} {
		if strings.Contains(value, secret) {
			t.Fatalf("pending provider completion restored forgotten content: %#v", inspected)
		}
	}
}

func addSourceMatchFact(t *testing.T, service *GovernanceService, repoRoot, operationID, key, content, sourceRef string) GovernedObservationReceipt {
	t.Helper()
	receipt, err := service.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
		OperationID: operationID,
		MemoryKey:   key,
		Content:     content,
		SourceRef:   sourceRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func beginSourceMatchForTest(t *testing.T, store *Store, tenantID, continuityID, operationID, content string) SourceMatchReceipt {
	t.Helper()
	receipt, err := store.BeginSourceMatch(context.Background(), tenantID, continuityID, SourceMatchBeginRequest{
		OperationID:    operationID,
		SourceRef:      "fixture:" + operationID,
		SourceContent:  content,
		ProviderName:   "test-provider",
		RequestedModel: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func assertSourceMatchCandidate(t *testing.T, candidates []SourceMatchCandidate, memoryID, key, sourceRef string) {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.MemoryID == memoryID {
			if candidate.MemoryKey != key || candidate.SourceRef != sourceRef {
				t.Fatalf("candidate mismatch: %#v", candidate)
			}
			return
		}
	}
	t.Fatalf("candidate %s not found: %#v", memoryID, candidates)
}
