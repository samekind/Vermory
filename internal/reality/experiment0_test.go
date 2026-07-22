package reality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExperiment0ReportsFrozenPublicCoverage(t *testing.T) {
	report := BuildExperiment0(Experiment0Options{
		RunID:    "experiment-0-test",
		CaseRoot: "../../reality/cases",
	})

	if !report.Pass || !report.PublicValidation.Pass {
		t.Fatalf("expected public evidence to pass: %#v", report)
	}
	if len(report.PublicValidation.Results) != 21 {
		t.Fatalf("expected twenty-one cases, got %d", len(report.PublicValidation.Results))
	}
	for _, result := range report.PublicValidation.Results {
		if result.LockSHA256 == "" {
			t.Fatalf("case %s has no fixture lock hash", result.CaseID)
		}
	}
	if len(report.ContinuityCoverage[string(LineWorkspace)]) != 6 || len(report.ContinuityCoverage[string(LineConversation)]) != 12 {
		t.Fatalf("unexpected continuity coverage: %#v", report.ContinuityCoverage)
	}
	if len(report.ContinuityCoverage[string(LineBridge)]) != 7 {
		t.Fatalf("expected seven bridge cases, got %#v", report.ContinuityCoverage)
	}
	if len(report.PressureCoverage["explicit_deletion"]) != 1 {
		t.Fatalf("expected deletion pressure coverage: %#v", report.PressureCoverage)
	}
	if report.EvidenceLevels[string(EvidencePublic)] != 21 || report.SealedStatus != "unavailable" {
		t.Fatalf("unexpected evidence status: levels=%#v sealed=%q", report.EvidenceLevels, report.SealedStatus)
	}
	if !containsText(report.Limitations, "target discovery coverage remains incomplete") {
		t.Fatalf("expected incomplete-target limitation: %#v", report.Limitations)
	}
	if got := report.HypothesisSignals["H-013"]; len(got) != 1 || got[0] != "I01-authenticated-multitenant-rls" {
		t.Fatalf("unexpected RLS hypothesis signal: %#v", got)
	}
	if got := report.HypothesisSignals["H-014"]; len(got) != 2 || got[0] != "I02-postgresql-operations-recovery" || got[1] != "I03-postgresql-ha-pitr" {
		t.Fatalf("unexpected operations recovery hypothesis signal: %#v", got)
	}
	if got := report.HypothesisSignals["hermes_client_seed"]; len(got) != 1 || got[0] != "H01-hermes-linked-sessions" {
		t.Fatalf("unexpected Hermes client seed: %#v", got)
	}
	if got := report.HypothesisSignals["bridge_seed"]; len(got) != 3 ||
		got[0] != "B01-conversation-workspace-promotion" ||
		got[1] != "B02-linked-conversations-workspace-rebind" ||
		got[2] != "B03-three-client-conversation-bridge" {
		t.Fatalf("unexpected bridge seed: %#v", got)
	}
	if got := report.HypothesisSignals["H-005"]; len(got) != 4 ||
		got[0] != "C01-device-maintenance-continuity" ||
		got[1] != "F03-verified-tool-outcome-formation" ||
		got[2] != "S01-deletion-and-source-injection" ||
		got[3] != "W01-synapseloom-continuity" {
		t.Fatalf("unexpected revision hypothesis signal: %#v", got)
	}
	if got := report.HypothesisSignals["H-008"]; len(got) != 3 ||
		got[0] != "C01-device-maintenance-continuity" ||
		got[1] != "F03-verified-tool-outcome-formation" ||
		got[2] != "S01-deletion-and-source-injection" {
		t.Fatalf("unexpected lifecycle hypothesis signal: %#v", got)
	}
	if got := report.HypothesisSignals["H-007"]; len(got) != 4 ||
		got[0] != "C02-housing-viewing-validity" ||
		got[1] != "G01-language-default-local-override" ||
		got[2] != "S01-deletion-and-source-injection" ||
		got[3] != "W03-workspace-workaround-validity" {
		t.Fatalf("unexpected retention hypothesis signal: %#v", got)
	}
}

func TestBuildExperiment0CarriesEveryValidationFailure(t *testing.T) {
	root := t.TempDir()
	cloneCaseTree(t, "../../reality/testdata/valid-public", filepath.Join(root, "unfrozen"))
	report := BuildExperiment0(Experiment0Options{RunID: "failed", CaseRoot: root})

	if report.Pass || report.PublicValidation.Pass {
		t.Fatalf("expected validation failure: %#v", report)
	}
	if !hasViolation(report.PublicValidation, "fixture_lock_missing") {
		t.Fatalf("expected fixture lock failure to carry forward: %#v", report.PublicValidation)
	}
}

func TestBuildExperiment0SeparatesWithheldAndExternallyAttestedEvidence(t *testing.T) {
	root := t.TempDir()
	dir := cloneCaseTree(t, "../../reality/testdata/valid-public", filepath.Join(root, "withheld"))
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"evidence_level": "public"`, `"evidence_level": "withheld_local"`, 1))
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FreezeCase(dir); err != nil {
		t.Fatal(err)
	}
	attestation := Attestation{HardGatesPass: true, Counts: map[string]int{"cases": 3, "passed": 3}}

	report := BuildExperiment0(Experiment0Options{RunID: "mixed", CaseRoot: root, SealedAttestation: &attestation})
	if !report.Pass || report.EvidenceLevels[string(EvidenceWithheldLocal)] != 1 || report.EvidenceLevels["sealed"] != 3 {
		t.Fatalf("unexpected mixed evidence report: %#v", report)
	}
	if report.SealedStatus != "attested" {
		t.Fatalf("expected attested sealed status, got %q", report.SealedStatus)
	}
}

func TestBuildExperiment0DoesNotCountLegacyDefinitionsAsRealTrajectories(t *testing.T) {
	report := BuildExperiment0(Experiment0Options{RunID: "baseline", CaseRoot: "../../reality/cases"})
	if report.Baselines["memos_three_client_round_packs"] != "translated_proxy" {
		t.Fatalf("legacy packs must remain translated proxies: %#v", report.Baselines)
	}
	if report.Baselines["legacy_contextmesh_self_case"] != "inspired_case" {
		t.Fatalf("legacy self-case must remain an inspired case: %#v", report.Baselines)
	}
	if !report.Pass {
		t.Fatalf("target seed counts must not become an automatic failure: %#v", report)
	}
}

func TestWriteExperiment0Artifacts(t *testing.T) {
	report := BuildExperiment0(Experiment0Options{RunID: "write-test", CaseRoot: "../../reality/cases"})
	artifacts, err := WriteExperiment0Artifacts(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(artifacts.JSONPath, filepath.Join("experiment-0", "write-test", "report.json")) {
		t.Fatalf("unexpected JSON path %q", artifacts.JSONPath)
	}
	data, err := os.ReadFile(artifacts.JSONPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Experiment0Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RunID != "write-test" || !decoded.Pass {
		t.Fatalf("unexpected persisted report: %#v", decoded)
	}
	markdown, err := os.ReadFile(artifacts.MarkdownPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "# Vermory Experiment 0: write-test") || !strings.Contains(string(markdown), "W01-synapseloom-continuity") {
		t.Fatalf("unexpected markdown:\n%s", markdown)
	}
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
