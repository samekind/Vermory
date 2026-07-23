package macos_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type w38PublicSnapshot struct {
	EvidenceVersion            int                 `json:"evidence_version"`
	CaseID                     string              `json:"case_id"`
	Status                     string              `json:"status"`
	ImplementationRevision     string              `json:"implementation_revision"`
	CaseSHA256                 string              `json:"case_sha256"`
	ImplementationBinarySHA256 string              `json:"implementation_binary_sha256"`
	AuthoritativeReportSHA256  string              `json:"authoritative_report_sha256"`
	RawInventorySHA256         string              `json:"raw_inventory_sha256"`
	RawEvidenceFileCount       int                 `json:"raw_evidence_file_count"`
	HardGateCount              int                 `json:"hard_gate_count"`
	HardGates                  []w38PublicHardGate `json:"hard_gates"`
	RetainedFailures           []struct {
		Attempt string `json:"attempt"`
		Result  string `json:"result"`
		Reason  string `json:"reason"`
	} `json:"retained_failures"`
	Model struct {
		RequestedWebChatModel        string `json:"requested_webchat_model"`
		ObservedWebChatProviderModel string `json:"observed_webchat_provider_model"`
		RankingClaim                 bool   `json:"ranking_claim"`
	} `json:"model"`
	EvidenceIntegrity struct {
		FailedAttemptsRetained    bool `json:"failed_attempts_retained"`
		KnownSecretMatches        int  `json:"known_secret_matches_in_raw_evidence"`
		RuntimeCredentialsRemoved bool `json:"temporary_runtime_credentials_removed"`
		PublicPathsNormalized     bool `json:"public_paths_normalized"`
		ChecksumBound             bool `json:"checksum_bound"`
	} `json:"evidence_integrity"`
}

type w38PublicHardGate struct {
	Number      int    `json:"number"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func TestRunW38OpenClawWebChatKeepsRuntimeExternalAndUnprivileged(t *testing.T) {
	payload, err := os.ReadFile("run-w38-openclaw-webchat.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, required := range []string{
		"/Volumes/JSData/",
		"OPENCLAW_STATE_DIR",
		"OPENCLAW_CONFIG_PATH",
		"OPENCLAW_BUNDLED_PLUGINS_DIR",
		"plugins inspect vermory --runtime --json",
		".plugin.status == \"loaded\"",
		"sessionMode:\"none\"",
		"grok-vermory-environment",
		"gateway run",
		"hold_runtime",
		"VERMORY_API_TOKEN",
		"VERMORY_OPERATOR_API_TOKEN",
		"schema_version == 25",
		"unlink \"$postgres_socket_link\"",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("W38 runner omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"sudo",
		"NewAPI",
		"newapi",
		"gateway install",
		"~/Library",
		"$HOME/Library",
		"set -x",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("W38 runner contains forbidden behavior %q", forbidden)
		}
	}
	info, err := os.Stat("run-w38-openclaw-webchat.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("W38 runner mode=%o want 755", info.Mode().Perm())
	}
}

func TestW38PublicEvidenceIsExactCredentialFreeAndChecksumBound(t *testing.T) {
	casePayload, err := os.ReadFile("../../runtime/cases/W38-official-openclaw-webchat/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		ID        string   `json:"id"`
		HardGates []string `json:"hard_gates"`
	}
	if err := json.Unmarshal(casePayload, &manifest); err != nil {
		t.Fatal(err)
	}

	snapshotPayload, err := os.ReadFile("../../docs/evidence/snapshots/2026-07-23-official-openclaw-webchat.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot w38PublicSnapshot
	if err := json.Unmarshal(snapshotPayload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.EvidenceVersion != 1 || snapshot.CaseID != manifest.ID || snapshot.Status != "runtime-qualified" {
		t.Fatalf("unexpected W38 public identity: %#v", snapshot)
	}
	if snapshot.ImplementationRevision != "6811b117292484872e0e3c89379edbf664efaa06" {
		t.Fatalf("unexpected W38 implementation revision %q", snapshot.ImplementationRevision)
	}
	caseDigest := sha256.Sum256(casePayload)
	if snapshot.CaseSHA256 != hex.EncodeToString(caseDigest[:]) {
		t.Fatalf("W38 case digest=%q want %q", snapshot.CaseSHA256, hex.EncodeToString(caseDigest[:]))
	}
	for name, value := range map[string]string{
		"binary":        snapshot.ImplementationBinarySHA256,
		"report":        snapshot.AuthoritativeReportSHA256,
		"raw inventory": snapshot.RawInventorySHA256,
	} {
		if len(value) != 64 {
			t.Errorf("W38 %s digest has length %d", name, len(value))
		}
	}
	if snapshot.RawEvidenceFileCount != 49 {
		t.Fatalf("W38 raw evidence files=%d want 49", snapshot.RawEvidenceFileCount)
	}
	if snapshot.HardGateCount != 28 || len(snapshot.HardGates) != 28 || len(manifest.HardGates) != 28 {
		t.Fatalf("W38 hard gates snapshot=%d/%d case=%d", snapshot.HardGateCount, len(snapshot.HardGates), len(manifest.HardGates))
	}
	for index, gate := range snapshot.HardGates {
		if gate.Number != index+1 || gate.Status != "passed" || gate.Description != manifest.HardGates[index] {
			t.Errorf("W38 hard gate %d drifted: %#v", index+1, gate)
		}
	}
	if snapshot.Model.RequestedWebChatModel != "grok-4.5" || snapshot.Model.ObservedWebChatProviderModel != "grok-4.5-build-free" || snapshot.Model.RankingClaim {
		t.Fatalf("unexpected W38 model boundary: %#v", snapshot.Model)
	}
	if len(snapshot.RetainedFailures) != 9 || !snapshot.EvidenceIntegrity.FailedAttemptsRetained ||
		snapshot.EvidenceIntegrity.KnownSecretMatches != 0 || !snapshot.EvidenceIntegrity.RuntimeCredentialsRemoved ||
		!snapshot.EvidenceIntegrity.PublicPathsNormalized || !snapshot.EvidenceIntegrity.ChecksumBound {
		t.Fatalf("incomplete W38 evidence integrity: failures=%d integrity=%#v", len(snapshot.RetainedFailures), snapshot.EvidenceIntegrity)
	}

	for _, forbidden := range []string{
		"/Volumes/JSData",
		"/Users/",
		"postgresql://",
		"postgres://",
		"http://127.",
		"https://",
		"sk-",
		`"token":`,
		`"password":`,
		`"database_url":`,
		`"proxy_url":`,
	} {
		if strings.Contains(string(snapshotPayload), forbidden) {
			t.Errorf("W38 public snapshot contains forbidden material %q", forbidden)
		}
	}

	reportPayload, err := os.ReadFile("../../docs/evidence/2026-07-23-official-openclaw-webchat.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"28 / 28 PASS",
		"grok-4.5-build-free",
		"post-delete probe answered `UNKNOWN`.",
		"Known runtime and Grok authentication values matched zero raw evidence files.",
	} {
		if !strings.Contains(string(reportPayload), required) {
			t.Errorf("W38 evidence report omitted %q", required)
		}
	}
	for _, forbidden := range []string{"/Volumes/JSData", "/Users/", "postgresql://", "sk-"} {
		if strings.Contains(string(reportPayload), forbidden) {
			t.Errorf("W38 evidence report contains forbidden material %q", forbidden)
		}
	}
}
