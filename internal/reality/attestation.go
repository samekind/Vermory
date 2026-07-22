package reality

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Attestation struct {
	Version              int               `json:"version"`
	EvaluatorID          string            `json:"evaluator_id"`
	EvaluatorKeyID       string            `json:"evaluator_key_id,omitempty"`
	SuiteVersion         string            `json:"suite_version"`
	SuiteProfile         string            `json:"suite_profile,omitempty"`
	ProtocolVersion      string            `json:"protocol_version,omitempty"`
	SubmissionDigest     string            `json:"submission_digest,omitempty"`
	ImplementationDigest string            `json:"implementation_digest"`
	RunID                string            `json:"run_id,omitempty"`
	RunAt                time.Time         `json:"run_at"`
	HardGatesPass        bool              `json:"hard_gates_pass"`
	Counts               map[string]int    `json:"counts"`
	HardGateResults      map[string]string `json:"hard_gate_results,omitempty"`
	FailureCategories    []string          `json:"failure_categories,omitempty"`
	ResultDigest         string            `json:"result_digest,omitempty"`
	Signature            string            `json:"signature"`
}

type attestationWire struct {
	Version              *int               `json:"version"`
	EvaluatorID          *string            `json:"evaluator_id"`
	EvaluatorKeyID       *string            `json:"evaluator_key_id,omitempty"`
	SuiteVersion         *string            `json:"suite_version"`
	SuiteProfile         *string            `json:"suite_profile,omitempty"`
	ProtocolVersion      *string            `json:"protocol_version,omitempty"`
	SubmissionDigest     *string            `json:"submission_digest,omitempty"`
	ImplementationDigest *string            `json:"implementation_digest"`
	RunID                *string            `json:"run_id,omitempty"`
	RunAt                *time.Time         `json:"run_at"`
	HardGatesPass        *bool              `json:"hard_gates_pass"`
	Counts               *map[string]int    `json:"counts"`
	HardGateResults      *map[string]string `json:"hard_gate_results,omitempty"`
	FailureCategories    []string           `json:"failure_categories,omitempty"`
	ResultDigest         *string            `json:"result_digest,omitempty"`
	Signature            *string            `json:"signature"`
}

type unsignedAttestation struct {
	Version              int            `json:"version"`
	EvaluatorID          string         `json:"evaluator_id"`
	SuiteVersion         string         `json:"suite_version"`
	ImplementationDigest string         `json:"implementation_digest"`
	RunAt                time.Time      `json:"run_at"`
	HardGatesPass        bool           `json:"hard_gates_pass"`
	Counts               map[string]int `json:"counts"`
	FailureCategories    []string       `json:"failure_categories,omitempty"`
}

type unsignedAttestationV2 struct {
	Version              int               `json:"version"`
	EvaluatorID          string            `json:"evaluator_id"`
	EvaluatorKeyID       string            `json:"evaluator_key_id"`
	SuiteVersion         string            `json:"suite_version"`
	SuiteProfile         string            `json:"suite_profile"`
	ProtocolVersion      string            `json:"protocol_version"`
	SubmissionDigest     string            `json:"submission_digest"`
	ImplementationDigest string            `json:"implementation_digest"`
	RunID                string            `json:"run_id"`
	RunAt                time.Time         `json:"run_at"`
	HardGatesPass        bool              `json:"hard_gates_pass"`
	Counts               map[string]int    `json:"counts"`
	HardGateResults      map[string]string `json:"hard_gate_results"`
	FailureCategories    []string          `json:"failure_categories,omitempty"`
	ResultDigest         string            `json:"result_digest"`
}

func VerifyAttestation(data []byte, publicKey ed25519.PublicKey) (Attestation, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Attestation{}, fmt.Errorf("invalid Ed25519 public key length %d", len(publicKey))
	}

	var wire attestationWire
	if err := decodeStrictJSON(data, &wire); err != nil {
		return Attestation{}, fmt.Errorf("decode attestation: %w", err)
	}
	attestation, err := validateAttestationWire(wire, publicKey)
	if err != nil {
		return Attestation{}, err
	}
	signature, err := base64.StdEncoding.DecodeString(attestation.Signature)
	if err != nil {
		return Attestation{}, fmt.Errorf("decode signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return Attestation{}, fmt.Errorf("invalid Ed25519 signature length %d", len(signature))
	}
	payload, err := canonicalAttestationPayload(attestation)
	if err != nil {
		return Attestation{}, err
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return Attestation{}, errors.New("attestation signature verification failed")
	}
	return attestation, nil
}

func validateAttestationWire(wire attestationWire, publicKey ed25519.PublicKey) (Attestation, error) {
	if wire.Version == nil {
		return Attestation{}, errors.New("version is required")
	}
	if *wire.Version != 1 && *wire.Version != 2 {
		return Attestation{}, fmt.Errorf("attestation version %d is unsupported", *wire.Version)
	}
	if wire.EvaluatorID == nil || strings.TrimSpace(*wire.EvaluatorID) == "" {
		return Attestation{}, errors.New("evaluator_id is required")
	}
	if wire.SuiteVersion == nil || strings.TrimSpace(*wire.SuiteVersion) == "" {
		return Attestation{}, errors.New("suite_version is required")
	}
	if wire.ImplementationDigest == nil || !validSHA256(*wire.ImplementationDigest) {
		return Attestation{}, errors.New("implementation_digest is required and must be a lowercase SHA-256 digest")
	}
	if wire.RunAt == nil || wire.RunAt.IsZero() {
		return Attestation{}, errors.New("run_at is required")
	}
	if wire.HardGatesPass == nil {
		return Attestation{}, errors.New("hard_gates_pass is required")
	}
	if wire.Counts == nil || len(*wire.Counts) == 0 {
		return Attestation{}, errors.New("counts is required and must not be empty")
	}
	for key, value := range *wire.Counts {
		if strings.TrimSpace(key) == "" || value < 0 {
			return Attestation{}, errors.New("counts keys must be non-empty and values must be non-negative")
		}
	}
	if wire.Signature == nil || strings.TrimSpace(*wire.Signature) == "" {
		return Attestation{}, errors.New("signature is required")
	}
	attestation := Attestation{
		Version:              *wire.Version,
		EvaluatorID:          *wire.EvaluatorID,
		SuiteVersion:         *wire.SuiteVersion,
		ImplementationDigest: *wire.ImplementationDigest,
		RunAt:                *wire.RunAt,
		HardGatesPass:        *wire.HardGatesPass,
		Counts:               *wire.Counts,
		FailureCategories:    wire.FailureCategories,
		Signature:            *wire.Signature,
	}
	if *wire.Version == 1 {
		if wire.EvaluatorKeyID != nil || wire.SuiteProfile != nil || wire.ProtocolVersion != nil ||
			wire.SubmissionDigest != nil || wire.RunID != nil || wire.HardGateResults != nil || wire.ResultDigest != nil {
			return Attestation{}, errors.New("version 1 attestation must not contain version 2 fields")
		}
		return attestation, nil
	}
	if !validEvaluationToken(*wire.EvaluatorID) {
		return Attestation{}, errors.New("evaluator_id must be a lowercase versioned token")
	}
	if wire.EvaluatorKeyID == nil || !validSHA256(*wire.EvaluatorKeyID) {
		return Attestation{}, errors.New("evaluator_key_id is required and must be a lowercase SHA-256 digest")
	}
	if *wire.EvaluatorKeyID != EvaluatorKeyID(publicKey) {
		return Attestation{}, errors.New("evaluator_key_id does not match the supplied public key")
	}
	if wire.SuiteProfile == nil || !validEvaluationToken(*wire.SuiteProfile) {
		return Attestation{}, errors.New("suite_profile is required and must be a lowercase versioned token")
	}
	if !validEvaluationToken(*wire.SuiteVersion) {
		return Attestation{}, errors.New("suite_version must be a lowercase versioned token")
	}
	if wire.ProtocolVersion == nil || *wire.ProtocolVersion != ExternalEvaluationProtocolVersion {
		return Attestation{}, fmt.Errorf("protocol_version must be %q", ExternalEvaluationProtocolVersion)
	}
	if wire.SubmissionDigest == nil || !validSHA256(*wire.SubmissionDigest) {
		return Attestation{}, errors.New("submission_digest is required and must be a lowercase SHA-256 digest")
	}
	if wire.RunID == nil || !validEvaluationToken(*wire.RunID) {
		return Attestation{}, errors.New("run_id is required and must be a lowercase versioned token")
	}
	if !isUTC(*wire.RunAt) {
		return Attestation{}, errors.New("run_at must use UTC")
	}
	if wire.HardGateResults == nil || len(*wire.HardGateResults) == 0 {
		return Attestation{}, errors.New("hard_gate_results is required and must not be empty")
	}
	if wire.ResultDigest == nil || !validSHA256(*wire.ResultDigest) {
		return Attestation{}, errors.New("result_digest is required and must be a lowercase SHA-256 digest")
	}
	if err := validateV2Counts(*wire.Counts, *wire.HardGateResults, *wire.HardGatesPass, wire.FailureCategories); err != nil {
		return Attestation{}, err
	}
	attestation.EvaluatorKeyID = *wire.EvaluatorKeyID
	attestation.SuiteProfile = *wire.SuiteProfile
	attestation.ProtocolVersion = *wire.ProtocolVersion
	attestation.SubmissionDigest = *wire.SubmissionDigest
	attestation.RunID = *wire.RunID
	attestation.HardGateResults = *wire.HardGateResults
	attestation.ResultDigest = *wire.ResultDigest
	return attestation, nil
}

func canonicalAttestationPayload(attestation Attestation) ([]byte, error) {
	if attestation.Version == 2 {
		return json.Marshal(unsignedAttestationV2{
			Version:              attestation.Version,
			EvaluatorID:          attestation.EvaluatorID,
			EvaluatorKeyID:       attestation.EvaluatorKeyID,
			SuiteVersion:         attestation.SuiteVersion,
			SuiteProfile:         attestation.SuiteProfile,
			ProtocolVersion:      attestation.ProtocolVersion,
			SubmissionDigest:     attestation.SubmissionDigest,
			ImplementationDigest: attestation.ImplementationDigest,
			RunID:                attestation.RunID,
			RunAt:                attestation.RunAt,
			HardGatesPass:        attestation.HardGatesPass,
			Counts:               attestation.Counts,
			HardGateResults:      attestation.HardGateResults,
			FailureCategories:    attestation.FailureCategories,
			ResultDigest:         attestation.ResultDigest,
		})
	}
	return json.Marshal(unsignedAttestation{
		Version:              attestation.Version,
		EvaluatorID:          attestation.EvaluatorID,
		SuiteVersion:         attestation.SuiteVersion,
		ImplementationDigest: attestation.ImplementationDigest,
		RunAt:                attestation.RunAt,
		HardGatesPass:        attestation.HardGatesPass,
		Counts:               attestation.Counts,
		FailureCategories:    attestation.FailureCategories,
	})
}

func VerifyAttestationForSubmission(data []byte, publicKey ed25519.PublicKey, submissionData []byte) (Attestation, error) {
	submission, err := ParseEvaluationSubmission(submissionData)
	if err != nil {
		return Attestation{}, fmt.Errorf("verify evaluation submission: %w", err)
	}
	attestation, err := VerifyAttestation(data, publicKey)
	if err != nil {
		return Attestation{}, err
	}
	if attestation.Version != 2 {
		return Attestation{}, errors.New("a submission-bound attestation must use version 2")
	}
	submissionDigest, err := EvaluationSubmissionDigest(submission)
	if err != nil {
		return Attestation{}, err
	}
	if attestation.SubmissionDigest != submissionDigest {
		return Attestation{}, errors.New("submission_digest does not match the supplied submission")
	}
	if attestation.ImplementationDigest != submission.Implementation.ArtifactSHA256 {
		return Attestation{}, errors.New("implementation_digest does not match the supplied submission")
	}
	if attestation.ProtocolVersion != submission.ProtocolVersion {
		return Attestation{}, errors.New("protocol_version does not match the supplied submission")
	}
	if attestation.SuiteProfile != submission.SuiteProfile {
		return Attestation{}, errors.New("suite_profile does not match the supplied submission")
	}
	if attestation.RunAt.Before(submission.CreatedAt) || attestation.RunAt.After(submission.ExpiresAt) {
		return Attestation{}, errors.New("attestation run_at is outside the submission validity interval")
	}
	return attestation, nil
}

func EvaluatorKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:])
}

func validateV2Counts(counts map[string]int, hardGates map[string]string, hardGatesPass bool, failureCategories []string) error {
	required := []string{
		"cases",
		"passed",
		"failed",
		"hard_gates",
		"hard_gates_passed",
		"hard_gates_failed",
		"hard_gates_not_run",
	}
	for _, key := range required {
		if _, ok := counts[key]; !ok {
			return fmt.Errorf("counts.%s is required", key)
		}
	}
	if counts["cases"] != counts["passed"]+counts["failed"] {
		return errors.New("case counts are inconsistent")
	}
	if counts["cases"] == 0 {
		return errors.New("counts.cases must be positive")
	}
	statusCounts := map[string]int{"pass": 0, "fail": 0, "not_run": 0}
	for key, status := range hardGates {
		if !validEvaluationToken(key) {
			return fmt.Errorf("hard_gate_results contains invalid key %q", key)
		}
		if _, ok := statusCounts[status]; !ok {
			return fmt.Errorf("hard_gate_results.%s has unsupported status %q", key, status)
		}
		statusCounts[status]++
	}
	if counts["hard_gates"] != len(hardGates) ||
		counts["hard_gates_passed"] != statusCounts["pass"] ||
		counts["hard_gates_failed"] != statusCounts["fail"] ||
		counts["hard_gates_not_run"] != statusCounts["not_run"] {
		return errors.New("hard gate counts are inconsistent")
	}
	shouldPass := statusCounts["fail"] == 0 && statusCounts["not_run"] == 0
	if hardGatesPass != shouldPass {
		return errors.New("hard_gates_pass is inconsistent with hard_gate_results")
	}
	if (counts["failed"] > 0 || !shouldPass) && len(failureCategories) == 0 {
		return errors.New("failure_categories is required when cases or hard gates fail")
	}
	seen := make(map[string]struct{}, len(failureCategories))
	for _, category := range failureCategories {
		if !validEvaluationToken(category) {
			return fmt.Errorf("failure_categories contains invalid token %q", category)
		}
		if _, exists := seen[category]; exists {
			return fmt.Errorf("failure_categories contains duplicate %q", category)
		}
		seen[category] = struct{}{}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
