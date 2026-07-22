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
