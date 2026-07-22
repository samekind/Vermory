package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type workspacePhysicalCrossHostCase struct {
	Version                   string   `json:"version"`
	ID                        string   `json:"id"`
	RealityCaseID             string   `json:"reality_case_id"`
	SourceHost                string   `json:"source_host"`
	TargetHost                string   `json:"target_host"`
	TenantID                  string   `json:"tenant_id"`
	OtherTenantID             string   `json:"other_tenant_id"`
	SourceFilesystemNamespace string   `json:"source_filesystem_namespace"`
	TargetFilesystemNamespace string   `json:"target_filesystem_namespace"`
	CurrentFact               string   `json:"current_fact"`
	OtherTenantFact           string   `json:"other_tenant_fact"`
	ArtifactPath              string   `json:"artifact_path"`
	ArtifactContent           string   `json:"artifact_content"`
	HardGateCount             int      `json:"hard_gate_count"`
	HardGates                 []string `json:"hard_gates"`
}

type workspacePhysicalCrossHostEvidence struct {
	CaseID      string `json:"case_id"`
	Status      string `json:"status"`
	SourceHead  string `json:"source_head"`
	ProtectedCI struct {
		Conclusion string `json:"conclusion"`
	} `json:"protected_ci"`
	SignedSnapshot struct {
		PositiveVerification      string `json:"positive_verification"`
		ModifiedManifestRejection string `json:"modified_manifest_rejection"`
		WrongIdentityRejection    string `json:"wrong_identity_rejection"`
		DarwinARM64Revision       string `json:"darwin_arm64_revision"`
	} `json:"signed_snapshot"`
	MCP struct {
		Protocol                    string   `json:"protocol"`
		Tools                       []string `json:"tools"`
		ModelVisibleAuthorityFields int      `json:"model_visible_authority_fields"`
		GrokDoctorHealthy           bool     `json:"grok_doctor_healthy"`
	} `json:"mcp"`
	RuntimeProbe struct {
		Client         string `json:"client"`
		MemoryStatus   string `json:"memory_status"`
		ExactReplay    bool   `json:"exact_replay"`
		ArtifactExact  bool   `json:"artifact_content_exact"`
		ProjectionRows int    `json:"proposed_projection_rows"`
	} `json:"runtime_probe"`
	GrokAttempt struct {
		Status       string `json:"status"`
		FailureClass string `json:"failure_class"`
		PrepareCalls int    `json:"prepare_calls"`
		CommitCalls  int    `json:"commit_calls"`
		Deliveries   int    `json:"deliveries"`
		Observations int    `json:"observations"`
		Artifacts    int    `json:"artifacts"`
	} `json:"grok_attempt"`
	Reversal struct {
		BridgeStatus          string `json:"bridge_status"`
		SourceBinding         string `json:"source_binding"`
		TargetBinding         string `json:"target_binding"`
		OtherTargetBinding    string `json:"other_tenant_target_binding"`
		SourceStatus          string `json:"source_status"`
		TargetStatus          string `json:"target_status"`
		TargetDeliveryIDEmpty bool   `json:"target_delivery_id_empty"`
		TargetDeliveryRows    int    `json:"target_delivery_rows"`
	} `json:"reversal"`
	HardGateSummary struct {
		Passed          int  `json:"passed"`
		ExternalBlocked int  `json:"external_blocked"`
		PlatformFailed  int  `json:"platform_failed"`
		Qualified32Of32 bool `json:"qualified_32_of_32"`
	} `json:"hard_gate_summary"`
	HardGates []struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Client string `json:"client"`
	} `json:"hard_gates"`
}

func TestW34PhysicalCrossHostWorkspaceContinuityCaseIsFrozen(t *testing.T) {
	manifest := loadWorkspacePhysicalCrossHostCase(t)
	if manifest.Version != "1" || manifest.ID != "W34-physical-cross-host-workspace-continuity" ||
		manifest.RealityCaseID != "W05-trusted-workspace-attachment" {
		t.Fatalf("unexpected W34 identity: %#v", manifest)
	}
	if manifest.SourceHost == "" || manifest.TargetHost == "" || manifest.SourceHost == manifest.TargetHost {
		t.Fatalf("W34 requires two distinct physical hosts: %#v", manifest)
	}
	if manifest.TenantID == "" || manifest.OtherTenantID == "" || manifest.TenantID == manifest.OtherTenantID {
		t.Fatalf("W34 tenant isolation contract is invalid: %#v", manifest)
	}
	if manifest.SourceFilesystemNamespace == "" || manifest.TargetFilesystemNamespace == "" ||
		manifest.SourceFilesystemNamespace == manifest.TargetFilesystemNamespace {
		t.Fatalf("W34 filesystem namespace contract is invalid: %#v", manifest)
	}
	if strings.TrimSpace(manifest.CurrentFact) == "" || strings.TrimSpace(manifest.OtherTenantFact) == "" ||
		manifest.CurrentFact == manifest.OtherTenantFact {
		t.Fatalf("W34 governed fact controls are invalid: %#v", manifest)
	}
	if filepath.IsAbs(manifest.ArtifactPath) || filepath.Clean(manifest.ArtifactPath) != manifest.ArtifactPath ||
		strings.HasPrefix(manifest.ArtifactPath, "..") || strings.TrimSpace(manifest.ArtifactContent) == "" {
		t.Fatalf("W34 deterministic artifact contract is invalid: %#v", manifest)
	}
	if manifest.HardGateCount != 32 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("W34 hard-gate count drifted: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	seen := make(map[string]struct{}, len(manifest.HardGates))
	for _, gate := range manifest.HardGates {
		if strings.TrimSpace(gate) == "" {
			t.Fatal("W34 hard gate cannot be empty")
		}
		if _, exists := seen[gate]; exists {
			t.Fatalf("W34 hard gate is duplicated: %q", gate)
		}
		seen[gate] = struct{}{}
	}
}

func TestW34PhysicalCrossHostEvidencePreservesClientBoundary(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(root, "docs", "evidence", "snapshots", "2026-07-22-physical-cross-host-workspace-continuity.json")
	payload, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var evidence workspacePhysicalCrossHostEvidence
	if err := json.Unmarshal(payload, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.CaseID != "W34-physical-cross-host-workspace-continuity" ||
		evidence.Status != "physical-runtime-qualified-grok-generation-external-blocked" ||
		evidence.SourceHead == "" || evidence.ProtectedCI.Conclusion != "success" {
		t.Fatalf("unexpected W34 evidence identity: %#v", evidence)
	}
	if evidence.SignedSnapshot.DarwinARM64Revision != evidence.SourceHead ||
		evidence.SignedSnapshot.PositiveVerification != "pass" ||
		evidence.SignedSnapshot.ModifiedManifestRejection != "pass" ||
		evidence.SignedSnapshot.WrongIdentityRejection != "pass" {
		t.Fatalf("W34 signed snapshot is not independently qualified: %#v", evidence.SignedSnapshot)
	}
	if evidence.MCP.Protocol != "2025-06-18" || len(evidence.MCP.Tools) != 2 ||
		evidence.MCP.ModelVisibleAuthorityFields != 0 || !evidence.MCP.GrokDoctorHealthy {
		t.Fatalf("unexpected W34 MCP evidence: %#v", evidence.MCP)
	}
	if evidence.RuntimeProbe.Client != "independent-go-mcp-sdk" || evidence.RuntimeProbe.MemoryStatus != "proposed" ||
		!evidence.RuntimeProbe.ExactReplay || !evidence.RuntimeProbe.ArtifactExact || evidence.RuntimeProbe.ProjectionRows != 0 {
		t.Fatalf("W34 runtime probe evidence drifted: %#v", evidence.RuntimeProbe)
	}
	if evidence.GrokAttempt.Status != "external_blocked" || evidence.GrokAttempt.FailureClass != "authentication_expired" ||
		evidence.GrokAttempt.PrepareCalls != 0 || evidence.GrokAttempt.CommitCalls != 0 ||
		evidence.GrokAttempt.Deliveries != 0 || evidence.GrokAttempt.Observations != 0 || evidence.GrokAttempt.Artifacts != 0 {
		t.Fatalf("W34 Grok failure was blurred into a runtime pass: %#v", evidence.GrokAttempt)
	}
	if evidence.Reversal.BridgeStatus != "reversed" || evidence.Reversal.SourceBinding != "confirmed" ||
		evidence.Reversal.TargetBinding != "retired" || evidence.Reversal.OtherTargetBinding != "confirmed" ||
		evidence.Reversal.SourceStatus != "resolved" || evidence.Reversal.TargetStatus != "needs_confirmation" ||
		!evidence.Reversal.TargetDeliveryIDEmpty || evidence.Reversal.TargetDeliveryRows != 0 {
		t.Fatalf("W34 reversal evidence drifted: %#v", evidence.Reversal)
	}
	if evidence.HardGateSummary.Passed != 28 || evidence.HardGateSummary.ExternalBlocked != 4 ||
		evidence.HardGateSummary.PlatformFailed != 0 || evidence.HardGateSummary.Qualified32Of32 || len(evidence.HardGates) != 32 {
		t.Fatalf("W34 hard-gate summary is invalid: summary=%#v gates=%d", evidence.HardGateSummary, len(evidence.HardGates))
	}
	blocked := map[int]bool{23: true, 24: true, 25: true, 27: true}
	seen := make(map[int]bool, len(evidence.HardGates))
	for _, gate := range evidence.HardGates {
		if gate.ID < 1 || gate.ID > 32 || seen[gate.ID] {
			t.Fatalf("invalid W34 hard-gate id: %#v", gate)
		}
		seen[gate.ID] = true
		if blocked[gate.ID] && gate.Status != "external_blocked" {
			t.Fatalf("Grok gate %d was incorrectly qualified: %#v", gate.ID, gate)
		}
		if !blocked[gate.ID] && gate.Status != "pass" {
			t.Fatalf("runtime gate %d unexpectedly failed: %#v", gate.ID, gate)
		}
		if (gate.ID == 26 || gate.ID == 28 || gate.ID == 29) && gate.Client != "independent-go-mcp-sdk" {
			t.Fatalf("SDK runtime gate %d lost client provenance: %#v", gate.ID, gate)
		}
	}

	markdown, err := os.ReadFile(filepath.Join(root, "docs", "evidence", "2026-07-22-physical-cross-host-workspace-continuity.md"))
	if err != nil {
		t.Fatal(err)
	}
	publicEvidence := strings.ToLower(string(append(payload, markdown...)))
	for _, forbidden := range []string{"postgresql://", "/users/", "frp.", "refresh_token", "access_token", "private key"} {
		if strings.Contains(publicEvidence, forbidden) {
			t.Fatalf("W34 public evidence contains forbidden material %q", forbidden)
		}
	}
}

func loadWorkspacePhysicalCrossHostCase(t *testing.T) workspacePhysicalCrossHostCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W34-physical-cross-host-workspace-continuity", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest workspacePhysicalCrossHostCase
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W34 case contains trailing JSON data: %v", err)
	}
	return manifest
}
