package reality

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildEvaluationSubmissionBindsExactArtifact(t *testing.T) {
	artifactPath := filepath.Join(t.TempDir(), "vermory_linux_amd64.tar.gz")
	if err := os.WriteFile(artifactPath, []byte("exact release artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	submission := validEvaluationSubmission()
	submission.Implementation.ArtifactName = ""
	submission.Implementation.ArtifactSHA256 = ""
	submission.Implementation.ArtifactSizeBytes = 0
	built, err := BuildEvaluationSubmission(submission, artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if built.Implementation.ArtifactName != filepath.Base(artifactPath) {
		t.Fatalf("artifact name = %q", built.Implementation.ArtifactName)
	}
	if built.Implementation.ArtifactSHA256 != "cce84875c88eea43f1986bb30390b6b3bd55132b4cb77e2ed46116d3912479cb" {
		t.Fatalf("artifact digest = %q", built.Implementation.ArtifactSHA256)
	}
	if built.Implementation.ArtifactSizeBytes != 23 {
		t.Fatalf("artifact size = %d", built.Implementation.ArtifactSizeBytes)
	}
}

func TestEvaluationSubmissionDigestIsCanonicalAndBindsFields(t *testing.T) {
	submission := validEvaluationSubmission()
	data, err := MarshalEvaluationSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	formatted := append([]byte(" \n\t"), data...)
	formatted = append(formatted, '\n')
	parsed, err := ParseEvaluationSubmission(formatted)
	if err != nil {
		t.Fatal(err)
	}

	first, err := EvaluationSubmissionDigest(submission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EvaluationSubmissionDigest(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical digest drifted: %s != %s", first, second)
	}

	parsed.Implementation.ArtifactSHA256 = strings.Repeat("c", 64)
	changed, err := EvaluationSubmissionDigest(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("implementation mutation did not change submission digest")
	}
}

func TestParseEvaluationSubmissionRejectsUnknownFields(t *testing.T) {
	data, err := MarshalEvaluationSubmission(validEvaluationSubmission())
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["expected_answers"] = []string{"must never enter a submission"}
	unknown, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseEvaluationSubmission(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}
}

func TestValidateEvaluationSubmissionRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EvaluationSubmission)
		want   string
	}{
		{
			name: "credential-bearing artifact URI",
			mutate: func(value *EvaluationSubmission) {
				value.Implementation.ArtifactURI = "https://token@example.test/vermory.tar.gz"
			},
			want: "artifact_uri",
		},
		{
			name: "artifact URI query",
			mutate: func(value *EvaluationSubmission) {
				value.Implementation.ArtifactURI = "https://example.test/vermory.tar.gz?token=secret"
			},
			want: "artifact_uri",
		},
		{
			name: "invalid nonce",
			mutate: func(value *EvaluationSubmission) {
				value.Nonce = "known-demo-value"
			},
			want: "nonce",
		},
		{
			name: "expired before creation",
			mutate: func(value *EvaluationSubmission) {
				value.ExpiresAt = value.CreatedAt.Add(-time.Second)
			},
			want: "expires_at",
		},
		{
			name: "duplicate interface",
			mutate: func(value *EvaluationSubmission) {
				value.Interfaces = []string{"workspace_mcp_stdio_v1", "workspace_mcp_stdio_v1"}
			},
			want: "interfaces",
		},
		{
			name: "unsorted platform",
			mutate: func(value *EvaluationSubmission) {
				value.Platforms = []string{"linux_arm64", "linux_amd64"}
			},
			want: "platforms",
		},
		{
			name: "implementation callback boundary",
			mutate: func(value *EvaluationSubmission) {
				value.Execution.Network = "implementation_callback_allowed"
			},
			want: "execution.network",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validEvaluationSubmission()
			test.mutate(&value)
			if err := ValidateEvaluationSubmission(value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q rejection, got %v", test.want, err)
			}
		})
	}
}

func TestVerifyAttestationV2BindsExactSubmission(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	submission := validEvaluationSubmission()
	submissionData, err := MarshalEvaluationSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	attestation := validV2Attestation(t, submission, publicKey)
	signed := signV2Attestation(t, attestation, privateKey)

	verified, err := VerifyAttestationForSubmission(signed, publicKey, submissionData)
	if err != nil {
		t.Fatal(err)
	}
	if verified.RunID != attestation.RunID || verified.SubmissionDigest != attestation.SubmissionDigest {
		t.Fatalf("unexpected bound attestation: %#v", verified)
	}
}

func TestVerifyAttestationV2RejectsBindingAndCountFailures(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	submission := validEvaluationSubmission()
	submissionData, err := MarshalEvaluationSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Attestation)
		want   string
	}{
		{
			name: "submission digest",
			mutate: func(value *Attestation) {
				value.SubmissionDigest = strings.Repeat("d", 64)
			},
			want: "submission_digest",
		},
		{
			name: "implementation digest",
			mutate: func(value *Attestation) {
				value.ImplementationDigest = strings.Repeat("e", 64)
			},
			want: "implementation_digest",
		},
		{
			name: "suite profile",
			mutate: func(value *Attestation) {
				value.SuiteProfile = "another-suite"
			},
			want: "suite_profile",
		},
		{
			name: "run outside validity",
			mutate: func(value *Attestation) {
				value.RunAt = submission.ExpiresAt.Add(time.Second)
			},
			want: "validity interval",
		},
		{
			name: "case count mismatch",
			mutate: func(value *Attestation) {
				value.Counts["passed"] = 11
			},
			want: "case counts",
		},
		{
			name: "hard gate mismatch",
			mutate: func(value *Attestation) {
				value.HardGateResults["deletion_residue"] = "fail"
			},
			want: "hard gate counts",
		},
		{
			name: "false pass",
			mutate: func(value *Attestation) {
				value.HardGateResults["deletion_residue"] = "fail"
				value.Counts["hard_gates_passed"] = 5
				value.Counts["hard_gates_failed"] = 1
			},
			want: "hard_gates_pass",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attestation := validV2Attestation(t, submission, publicKey)
			test.mutate(&attestation)
			signed := signV2Attestation(t, attestation, privateKey)
			if _, err := VerifyAttestationForSubmission(signed, publicKey, submissionData); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q rejection, got %v", test.want, err)
			}
		})
	}
}

func TestVerifyAttestationV2RejectsWrongEvaluatorKeyID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	submission := validEvaluationSubmission()
	submissionData, err := MarshalEvaluationSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	attestation := validV2Attestation(t, submission, otherPublicKey)
	signed := signV2Attestation(t, attestation, privateKey)
	if _, err := VerifyAttestationForSubmission(signed, publicKey, submissionData); err == nil || !strings.Contains(err.Error(), "evaluator_key_id") {
		t.Fatalf("expected evaluator key ID rejection, got %v", err)
	}
}

func validEvaluationSubmission() EvaluationSubmission {
	return EvaluationSubmission{
		Version:         1,
		ProtocolVersion: ExternalEvaluationProtocolVersion,
		SubmissionID:    "vermory-head-20260723",
		Nonce:           strings.Repeat("1", 64),
		CreatedAt:       time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC),
		ExpiresAt:       time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC),
		SuiteProfile:    "core-continuity-v1",
		Implementation: EvaluationImplementation{
			SourceRevision:    strings.Repeat("a", 40),
			ArtifactName:      "vermory_linux_amd64.tar.gz",
			ArtifactURI:       "https://github.com/samekind/Vermory/releases/download/v0.1.0/vermory_linux_amd64.tar.gz",
			ArtifactSHA256:    strings.Repeat("b", 64),
			ArtifactSizeBytes: 1024,
		},
		Interfaces: []string{
			"authenticated_web_chat_http_v1",
			"operator_cli_v1",
			"workspace_mcp_stdio_v1",
		},
		Platforms: []string{"linux_amd64", "linux_arm64"},
		Execution: EvaluationExecutionBoundary{
			Database:       EvaluationDatabaseEphemeralPostgres,
			Provider:       EvaluationProviderOwnedProxy,
			Network:        EvaluationNetworkDenyExceptProvider,
			Telemetry:      EvaluationTelemetryDisabled,
			ResultArtifact: EvaluationResultEvaluatorControlled,
		},
	}
}

func validV2Attestation(t *testing.T, submission EvaluationSubmission, publicKey ed25519.PublicKey) Attestation {
	t.Helper()
	submissionDigest, err := EvaluationSubmissionDigest(submission)
	if err != nil {
		t.Fatal(err)
	}
	return Attestation{
		Version:              2,
		EvaluatorID:          "independent-evaluator",
		EvaluatorKeyID:       EvaluatorKeyID(publicKey),
		SuiteVersion:         "private-core-2026-07-r1",
		SuiteProfile:         submission.SuiteProfile,
		ProtocolVersion:      submission.ProtocolVersion,
		SubmissionDigest:     submissionDigest,
		ImplementationDigest: submission.Implementation.ArtifactSHA256,
		RunID:                "run-20260723-001",
		RunAt:                submission.CreatedAt.Add(time.Hour),
		HardGatesPass:        true,
		Counts: map[string]int{
			"cases":              12,
			"passed":             12,
			"failed":             0,
			"hard_gates":         6,
			"hard_gates_passed":  6,
			"hard_gates_failed":  0,
			"hard_gates_not_run": 0,
		},
		HardGateResults: map[string]string{
			"cross_scope_leakage": "pass",
			"deletion_residue":    "pass",
			"source_authority":    "pass",
			"stale_misuse":        "pass",
			"tenant_isolation":    "pass",
			"wrong_attachment":    "pass",
		},
		ResultDigest: strings.Repeat("f", 64),
	}
}

func signV2Attestation(t *testing.T, attestation Attestation, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	payload, err := canonicalAttestationPayload(attestation)
	if err != nil {
		t.Fatal(err)
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	data, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
