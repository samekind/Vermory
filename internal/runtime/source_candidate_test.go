package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSourceCandidateLifecycleKeepsAuthorityUntilExplicitAcceptance(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	local := NewGovernanceService(store, "local")
	other := NewGovernanceService(store, "other-tenant")
	repoRoot := "/fixtures/release-control"

	localWorkspace, err := local.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := other.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	oldFact := "Production releases use a macOS keychain certificate."
	newFact := "Production releases use GitHub Actions OIDC keyless signing."
	independentFact := "The deployment API timeout is 800 ms."
	otherTenantFact := "Production releases use static cloud credentials."

	old, err := local.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "w05-signing-old",
		MemoryKey:   "release.signing.mode",
		Content:     oldFact,
		SourceRef:   "repo:deploy/production.yaml@sha-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	independent, err := local.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "w05-timeout",
		MemoryKey:   "deploy.api.timeout",
		Content:     independentFact,
		SourceRef:   "repo:deploy/runtime.yaml@sha-stable",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherFact, err := other.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "w05-other-signing",
		MemoryKey:   "release.signing.mode",
		Content:     otherTenantFact,
		SourceRef:   "repo:deploy/production.yaml@other-tenant",
	})
	if err != nil {
		t.Fatal(err)
	}

	proposal := GovernanceWriteRequest{
		OperationID: "w05-signing-candidate-1",
		MemoryKey:   "release.signing.mode",
		Content:     newFact,
		SourceRef:   "repo:deploy/production.yaml@sha-new",
	}
	first, err := local.ProposeSourceCandidate(ctx, repoRoot, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if first.Disposition != SourceCandidateReplacement ||
		first.TargetMemoryID != old.Memory.MemoryID ||
		first.Candidate.Status != "proposed" ||
		first.Candidate.MemoryID == "" {
		t.Fatalf("unexpected source candidate: %#v", first)
	}
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, oldFact, true)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, newFact, false)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, independentFact, true)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, otherTenantFact, false)
	assertSourceCandidateSearch(t, store, "other-tenant", otherWorkspace.ContinuityID, otherTenantFact, true)

	replay, err := local.ProposeSourceCandidate(ctx, repoRoot, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Candidate.MemoryID != first.Candidate.MemoryID || replay.TargetMemoryID != first.TargetMemoryID {
		t.Fatalf("proposal replay changed identity: first=%#v replay=%#v", first, replay)
	}
	conflict := proposal
	conflict.Content = "Production releases use a different signing mode."
	if _, err := local.ProposeSourceCandidate(ctx, repoRoot, conflict); err == nil || !strings.Contains(err.Error(), "another logical source candidate") {
		t.Fatalf("conflicting proposal replay was accepted: %v", err)
	}

	rejected, err := local.RejectCandidate(ctx, repoRoot, first.Candidate.MemoryID, "w05-reject-candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Memory.Status != "rejected" || rejected.Memory.MemoryID != first.Candidate.MemoryID {
		t.Fatalf("unexpected rejection: %#v", rejected)
	}
	rejectReplay, err := local.RejectCandidate(ctx, repoRoot, first.Candidate.MemoryID, "w05-reject-candidate-1")
	if err != nil || !rejectReplay.Memory.Replayed {
		t.Fatalf("rejection replay failed: receipt=%#v err=%v", rejectReplay, err)
	}
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, oldFact, true)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, newFact, false)

	secondProposal := proposal
	secondProposal.OperationID = "w05-signing-candidate-2"
	second, err := local.ProposeSourceCandidate(ctx, repoRoot, secondProposal)
	if err != nil {
		t.Fatal(err)
	}
	if second.TargetMemoryID != old.Memory.MemoryID || second.Candidate.MemoryID == first.Candidate.MemoryID {
		t.Fatalf("second proposal did not target the current fact: %#v", second)
	}
	if _, err := other.AcceptCandidate(ctx, repoRoot, second.Candidate.MemoryID, "w05-other-accept"); err == nil {
		t.Fatal("other tenant accepted the local candidate")
	}
	if _, err := local.ConfirmWorkspace(ctx, "/fixtures/release-control-other"); err != nil {
		t.Fatal(err)
	}
	if _, err := local.AcceptCandidate(ctx, "/fixtures/release-control-other", second.Candidate.MemoryID, "w05-other-workspace-accept"); err == nil {
		t.Fatal("another workspace accepted the candidate")
	}
	if _, err := local.RejectCandidate(ctx, repoRoot, second.Candidate.MemoryID, "w05-reject-candidate-1"); err == nil {
		t.Fatal("rejection operation_id was reused for another candidate")
	}

	accepted, err := local.AcceptCandidate(ctx, repoRoot, second.Candidate.MemoryID, "w05-accept-candidate-2")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Memory.Status != "active" || accepted.Memory.MemoryID != second.Candidate.MemoryID {
		t.Fatalf("unexpected acceptance: %#v", accepted)
	}
	acceptReplay, err := local.AcceptCandidate(ctx, repoRoot, second.Candidate.MemoryID, "w05-accept-candidate-2")
	if err != nil || !acceptReplay.Memory.Replayed {
		t.Fatalf("acceptance replay failed: receipt=%#v err=%v", acceptReplay, err)
	}
	if _, err := local.AcceptCandidate(ctx, repoRoot, first.Candidate.MemoryID, "w05-accept-rejected"); err == nil {
		t.Fatal("rejected candidate became active")
	}

	memories, err := store.ListGovernedMemories(ctx, "local", localWorkspace.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateMemory(t, memories, old.Memory.MemoryID, "release.signing.mode", "superseded", "")
	assertSourceCandidateMemory(t, memories, first.Candidate.MemoryID, "release.signing.mode", "rejected", old.Memory.MemoryID)
	assertSourceCandidateMemory(t, memories, second.Candidate.MemoryID, "release.signing.mode", "active", old.Memory.MemoryID)
	assertSourceCandidateMemory(t, memories, independent.Memory.MemoryID, "deploy.api.timeout", "active", "")
	if err := store.RebuildProjection(ctx, "local", localWorkspace.ContinuityID); err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, oldFact, false)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, newFact, true)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, independentFact, true)

	unchangedRequest := secondProposal
	unchangedRequest.OperationID = "w05-signing-unchanged"
	unchanged, err := local.ProposeSourceCandidate(ctx, repoRoot, unchangedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Disposition != SourceCandidateUnchanged || unchanged.Candidate.MemoryID != "" || unchanged.Observation.ObservationID == "" {
		t.Fatalf("identical source content created a candidate: %#v", unchanged)
	}
	unchangedReplay, err := local.ProposeSourceCandidate(ctx, repoRoot, unchangedRequest)
	if err != nil || !unchangedReplay.Replayed || unchangedReplay.Observation.ObservationID != unchanged.Observation.ObservationID {
		t.Fatalf("unchanged replay failed: receipt=%#v err=%v", unchangedReplay, err)
	}

	newCandidate, err := local.ProposeSourceCandidate(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "w05-attestation-new",
		MemoryKey:   "release.attestation.format",
		Content:     "Publish a signed in-toto attestation.",
		SourceRef:   "repo:deploy/attestation.yaml@sha-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if newCandidate.Disposition != SourceCandidateNew || newCandidate.TargetMemoryID != "" || newCandidate.Candidate.Status != "proposed" {
		t.Fatalf("unexpected new source candidate: %#v", newCandidate)
	}
	if _, err := local.AcceptCandidate(ctx, repoRoot, newCandidate.Candidate.MemoryID, "w05-accept-attestation"); err != nil {
		t.Fatal(err)
	}
	if _, err := local.AcceptCandidate(ctx, repoRoot, newCandidate.Candidate.MemoryID, "w05-accept-candidate-2"); err == nil {
		t.Fatal("acceptance operation_id was reused for another candidate")
	}
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, newCandidate.Candidate.MemoryID, false)
	assertSourceCandidateSearch(t, store, "local", localWorkspace.ContinuityID, "Publish a signed in-toto attestation.", true)

	if otherFact.Memory.MemoryID == "" {
		t.Fatal("other-tenant fixture did not persist")
	}
}

func TestSourceCandidateAcceptanceFailsAfterTargetChanges(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "local")
	repoRoot := "/fixtures/source-target-race"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	original, err := service.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "target-race-original",
		MemoryKey:   "release.signing.mode",
		Content:     "Use signing mode A.",
		SourceRef:   "fixture:target-race:a",
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := service.ProposeSourceCandidate(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "target-race-candidate",
		MemoryKey:   "release.signing.mode",
		Content:     "Use signing mode B.",
		SourceRef:   "fixture:target-race:b",
	})
	if err != nil {
		t.Fatal(err)
	}
	emergency, err := service.ReviseSource(ctx, repoRoot, original.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "target-race-emergency",
		Content:     "Use signing mode C.",
		SourceRef:   "fixture:target-race:c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptCandidate(ctx, repoRoot, candidate.Candidate.MemoryID, "target-race-accept"); err == nil || !strings.Contains(err.Error(), "no longer") {
		t.Fatalf("candidate replaced a no-longer-current target: %v", err)
	}
	memories, err := store.ListGovernedMemories(ctx, "local", resolution.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateMemory(t, memories, candidate.Candidate.MemoryID, "release.signing.mode", "proposed", original.Memory.MemoryID)
	assertSourceCandidateMemory(t, memories, emergency.Memory.MemoryID, "release.signing.mode", "active", original.Memory.MemoryID)
	assertSourceCandidateSearch(t, store, "local", resolution.ContinuityID, "Use signing mode B.", false)
	assertSourceCandidateSearch(t, store, "local", resolution.ContinuityID, "Use signing mode C.", true)
}

func TestNewSourceCandidateAcceptanceFailsWhenKeyBecomesActive(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "local")
	repoRoot := "/fixtures/new-source-race"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := service.ProposeSourceCandidate(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "new-source-candidate",
		MemoryKey:   "release.attestation.format",
		Content:     "Publish a signed in-toto attestation.",
		SourceRef:   "fixture:new-source:candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "new-source-active",
		MemoryKey:   "release.attestation.format",
		Content:     "Publish a signed SLSA attestation.",
		SourceRef:   "fixture:new-source:active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptCandidate(ctx, repoRoot, candidate.Candidate.MemoryID, "new-source-accept"); err == nil || !strings.Contains(err.Error(), "now has an active fact") {
		t.Fatalf("new candidate activated over a newly current key: %v", err)
	}
	memories, err := store.ListGovernedMemories(ctx, "local", resolution.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceCandidateMemory(t, memories, candidate.Candidate.MemoryID, "release.attestation.format", "proposed", "")
	assertSourceCandidateMemory(t, memories, active.Memory.MemoryID, "release.attestation.format", "active", "")
}

func TestSourceCandidateFormationAbstainsOnAmbiguousActiveKey(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	service := NewGovernanceService(store, "local")
	repoRoot := "/fixtures/ambiguous-source"
	resolution, err := service.ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for index, content := range []string{"Use signer A.", "Use signer B."} {
		_, err := service.AddSource(ctx, repoRoot, GovernanceWriteRequest{
			OperationID: "ambiguous-source-" + string(rune('a'+index)),
			MemoryKey:   "release.signer",
			Content:     content,
			SourceRef:   "fixture:ambiguous",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := service.ProposeSourceCandidate(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "ambiguous-candidate",
		MemoryKey:   "release.signer",
		Content:     "Use signer C.",
		SourceRef:   "fixture:ambiguous:new",
	}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous source key did not abstain: %v", err)
	}
	memories, err := store.ListGovernedMemories(ctx, "local", resolution.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range memories {
		if memory.LifecycleStatus == "proposed" {
			t.Fatalf("ambiguous formation wrote a candidate: %#v", memories)
		}
	}
}

func assertSourceCandidateMemory(t *testing.T, memories []GovernedMemory, id, key, status, supersedes string) {
	t.Helper()
	for _, memory := range memories {
		if memory.ID == id {
			if memory.MemoryKey != key || memory.LifecycleStatus != status || memory.SupersedesMemoryID != supersedes {
				t.Fatalf("memory %s mismatch: %#v", id, memory)
			}
			return
		}
	}
	t.Fatalf("memory %s not found: %#v", id, memories)
}

func assertSourceCandidateSearch(t *testing.T, store *Store, tenantID, continuityID, query string, want bool) {
	t.Helper()
	matches, err := store.SearchActiveMemory(context.Background(), tenantID, continuityID, query, 12)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, match := range matches {
		if match.Content == query {
			found = true
			break
		}
	}
	if found != want {
		t.Fatalf("search %q returned %#v, want exact=%t", query, matches, want)
	}
}
