package reality

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifyAttestationAcceptsValidEd25519Signature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attestation := validTestAttestation()
	data := signTestAttestation(t, attestation, privateKey)

	verified, err := VerifyAttestation(data, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if verified.EvaluatorID != attestation.EvaluatorID || !verified.HardGatesPass {
		t.Fatalf("unexpected verified attestation: %#v", verified)
	}
}

func TestVerifyAttestationRejectsModifiedPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attestation := validTestAttestation()
	signed := signTestAttestation(t, attestation, privateKey)
	if err := json.Unmarshal(signed, &attestation); err != nil {
		t.Fatal(err)
	}
	attestation.Counts["passed"]++
	modified, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyAttestation(modified, publicKey)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature failure, got %v", err)
	}
}

func TestVerifyAttestationRequiresIdentityAndRunFields(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	base := signTestAttestation(t, validTestAttestation(), privateKey)
	fields := []string{"evaluator_id", "suite_version", "implementation_digest", "run_at", "hard_gates_pass"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal(base, &payload); err != nil {
				t.Fatal(err)
			}
			delete(payload, field)
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyAttestation(data, publicKey); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("expected missing %s rejection, got %v", field, err)
			}
		})
	}
}

func TestVerifyAttestationRejectsUnknownFieldsAndFutureVersions(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := signTestAttestation(t, validTestAttestation(), privateKey)
	var payload map[string]any
	if err := json.Unmarshal(signed, &payload); err != nil {
		t.Fatal(err)
	}
	payload["local_sealed_case"] = true
	unknown, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAttestation(unknown, publicKey); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}

	attestation := validTestAttestation()
	attestation.Version = 3
	future := signTestAttestation(t, attestation, privateKey)
	if _, err := VerifyAttestation(future, publicKey); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected future-version rejection, got %v", err)
	}
}

func TestVerifyAttestationV1RejectsUnsignedV2Fields(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := signTestAttestation(t, validTestAttestation(), privateKey)
	var payload map[string]any
	if err := json.Unmarshal(signed, &payload); err != nil {
		t.Fatal(err)
	}
	payload["submission_digest"] = strings.Repeat("a", 64)
	modified, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAttestation(modified, publicKey); err == nil || !strings.Contains(err.Error(), "version 2 fields") {
		t.Fatalf("expected unsigned v2 field rejection, got %v", err)
	}
}

func validTestAttestation() Attestation {
	return Attestation{
		Version:              1,
		EvaluatorID:          "external-evaluator-1",
		SuiteVersion:         "sealed-suite-2026-07",
		ImplementationDigest: strings.Repeat("a", 64),
		RunAt:                time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC),
		HardGatesPass:        true,
		Counts:               map[string]int{"cases": 12, "passed": 12},
	}
}

func signTestAttestation(t *testing.T, attestation Attestation, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	unsigned := struct {
		Version              int            `json:"version"`
		EvaluatorID          string         `json:"evaluator_id"`
		SuiteVersion         string         `json:"suite_version"`
		ImplementationDigest string         `json:"implementation_digest"`
		RunAt                time.Time      `json:"run_at"`
		HardGatesPass        bool           `json:"hard_gates_pass"`
		Counts               map[string]int `json:"counts"`
		FailureCategories    []string       `json:"failure_categories,omitempty"`
	}{
		Version:              attestation.Version,
		EvaluatorID:          attestation.EvaluatorID,
		SuiteVersion:         attestation.SuiteVersion,
		ImplementationDigest: attestation.ImplementationDigest,
		RunAt:                attestation.RunAt,
		HardGatesPass:        attestation.HardGatesPass,
		Counts:               attestation.Counts,
		FailureCategories:    attestation.FailureCategories,
	}
	payload, err := json.Marshal(unsigned)
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
