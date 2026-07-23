package runtime

import (
	"strings"
	"testing"
)

func TestWorkspaceAnchorNormalizesRepoRoot(t *testing.T) {
	anchor, err := (WorkspaceAnchor{RepoRoot: "/work/acorn/../acorn/"}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if anchor.RepoRoot != "/work/acorn" {
		t.Fatalf("expected normalized root, got %q", anchor.RepoRoot)
	}
}

func TestPrepareContextRequestRejectsMissingOperationID(t *testing.T) {
	req := PrepareContextRequest{
		Workspace: WorkspaceAnchor{RepoRoot: "/work/acorn"},
		Task:      "Continue checkout work.",
	}
	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "operation_id") {
		t.Fatalf("expected operation_id validation error, got %v", err)
	}
}

func TestCommitObservationRequestRejectsUnsupportedKind(t *testing.T) {
	req := CommitObservationRequest{
		OperationID: "writeback-1",
		Kind:        "unknown",
		Content:     "Completed the task.",
	}
	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind validation error, got %v", err)
	}
}

func TestCommitObservationRequestRejectsAgentResultSupersession(t *testing.T) {
	req := CommitObservationRequest{
		OperationID:        "writeback-1",
		Kind:               ObservationKindAgentResult,
		Content:            "Use checkout_eta_v3.",
		SupersedesMemoryID: "e6fb79d6-f2cc-48a1-8fe2-595df8f5b316",
	}
	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "supersedes_memory_id") {
		t.Fatalf("expected supersession validation error, got %v", err)
	}
}

func TestSourceCandidateRequiresStableMemoryKeyAndSource(t *testing.T) {
	request := CommitObservationRequest{
		OperationID: "source-candidate-1",
		Kind:        ObservationKindSourceCandidate,
		Content:     "Use OIDC keyless signing.",
	}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "memory_key") {
		t.Fatalf("missing source candidate key was accepted: %v", err)
	}
	request.MemoryKey = "release.signing.mode"
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "source_ref") {
		t.Fatalf("missing source candidate reference was accepted: %v", err)
	}
	request.SourceRef = "repo:deploy/production.yaml@sha-new"
	request.SupersedesMemoryID = "e6fb79d6-f2cc-48a1-8fe2-595df8f5b316"
	if err := request.Validate(); err != nil {
		t.Fatalf("valid source candidate was rejected: %v", err)
	}
}

func TestMemoryKeyIsRejectedForUnkeyedObservationKinds(t *testing.T) {
	request := CommitObservationRequest{
		OperationID: "agent-result-key",
		Kind:        ObservationKindAgentResult,
		MemoryKey:   "release.signing.mode",
		Content:     "Agent output cannot claim a stable source key.",
	}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "memory_key") {
		t.Fatalf("agent result accepted a source memory key: %v", err)
	}
}

func TestPrepareContextRequestClampsMaxItems(t *testing.T) {
	req := PrepareContextRequest{
		OperationID: "prepare-1",
		Workspace:   WorkspaceAnchor{RepoRoot: "/work/acorn"},
		Task:        "Continue checkout work.",
		MaxItems:    100,
	}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	if req.MaxItems != maxContextItems {
		t.Fatalf("expected max items %d, got %d", maxContextItems, req.MaxItems)
	}
}
