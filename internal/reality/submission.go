package reality

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ExternalEvaluationProtocolVersion = "vermory.external-evaluation/v1"

	EvaluationDatabaseEphemeralPostgres = "evaluator_ephemeral_postgresql"
	EvaluationProviderOwnedProxy        = "evaluator_owned_proxy"
	EvaluationNetworkDenyExceptProvider = "deny_except_evaluator_provider_proxy"
	EvaluationTelemetryDisabled         = "disabled"
	EvaluationResultEvaluatorControlled = "evaluator_controlled"
)

type EvaluationSubmission struct {
	Version         int                         `json:"version"`
	ProtocolVersion string                      `json:"protocol_version"`
	SubmissionID    string                      `json:"submission_id"`
	Nonce           string                      `json:"nonce"`
	CreatedAt       time.Time                   `json:"created_at"`
	ExpiresAt       time.Time                   `json:"expires_at"`
	SuiteProfile    string                      `json:"suite_profile"`
	Implementation  EvaluationImplementation    `json:"implementation"`
	Interfaces      []string                    `json:"interfaces"`
	Platforms       []string                    `json:"platforms"`
	Execution       EvaluationExecutionBoundary `json:"execution"`
}

type EvaluationImplementation struct {
	SourceRevision    string `json:"source_revision"`
	ArtifactName      string `json:"artifact_name"`
	ArtifactURI       string `json:"artifact_uri"`
	ArtifactSHA256    string `json:"artifact_sha256"`
	ArtifactSizeBytes int64  `json:"artifact_size_bytes"`
}

type EvaluationExecutionBoundary struct {
	Database       string `json:"database"`
	Provider       string `json:"provider"`
	Network        string `json:"network"`
	Telemetry      string `json:"telemetry"`
	ResultArtifact string `json:"result_artifact"`
}

func GenerateEvaluationSubmissionNonce() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate submission nonce: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func BuildEvaluationSubmission(submission EvaluationSubmission, artifactPath string) (EvaluationSubmission, error) {
	file, err := os.Open(artifactPath)
	if err != nil {
		return EvaluationSubmission{}, fmt.Errorf("open implementation artifact: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return EvaluationSubmission{}, fmt.Errorf("stat implementation artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return EvaluationSubmission{}, errors.New("implementation artifact must be a non-empty regular file")
	}
	name := filepath.Base(artifactPath)
	if submission.Implementation.ArtifactName != "" && submission.Implementation.ArtifactName != name {
		return EvaluationSubmission{}, fmt.Errorf("artifact_name %q does not match artifact path %q", submission.Implementation.ArtifactName, name)
	}
	hash := sha256.New()
	bytesRead, err := io.Copy(hash, file)
	if err != nil {
		return EvaluationSubmission{}, fmt.Errorf("hash implementation artifact: %w", err)
	}
	if bytesRead != info.Size() {
		return EvaluationSubmission{}, errors.New("implementation artifact changed while it was being hashed")
	}

	submission.Implementation.ArtifactName = name
	submission.Implementation.ArtifactSHA256 = hex.EncodeToString(hash.Sum(nil))
	submission.Implementation.ArtifactSizeBytes = info.Size()
	sort.Strings(submission.Interfaces)
	sort.Strings(submission.Platforms)
	if err := ValidateEvaluationSubmission(submission); err != nil {
		return EvaluationSubmission{}, err
	}
	if err := VerifyEvaluationSubmissionArtifact(submission, artifactPath); err != nil {
		return EvaluationSubmission{}, fmt.Errorf("re-verify implementation artifact: %w", err)
	}
	return submission, nil
}

func ValidateEvaluationSubmission(submission EvaluationSubmission) error {
	if submission.Version != 1 {
		return fmt.Errorf("submission version %d is unsupported", submission.Version)
	}
	if submission.ProtocolVersion != ExternalEvaluationProtocolVersion {
		return fmt.Errorf("protocol_version must be %q", ExternalEvaluationProtocolVersion)
	}
	if !validEvaluationToken(submission.SubmissionID) {
		return errors.New("submission_id must be a lowercase versioned token")
	}
	if !validSHA256(submission.Nonce) {
		return errors.New("nonce must be 32 bytes encoded as lowercase hexadecimal")
	}
	if submission.CreatedAt.IsZero() || !isUTC(submission.CreatedAt) {
		return errors.New("created_at is required and must use UTC")
	}
	if submission.ExpiresAt.IsZero() || !isUTC(submission.ExpiresAt) || !submission.ExpiresAt.After(submission.CreatedAt) {
		return errors.New("expires_at is required, must use UTC, and must be after created_at")
	}
	if !validEvaluationToken(submission.SuiteProfile) {
		return errors.New("suite_profile must be a lowercase versioned token")
	}
	if !validSourceRevision(submission.Implementation.SourceRevision) {
		return errors.New("implementation.source_revision must be a lowercase 40- or 64-character hexadecimal revision")
	}
	if !validArtifactName(submission.Implementation.ArtifactName) {
		return errors.New("implementation.artifact_name must be a safe filename")
	}
	if err := validateArtifactURI(submission.Implementation.ArtifactURI, submission.Implementation.ArtifactName); err != nil {
		return err
	}
	if !validSHA256(submission.Implementation.ArtifactSHA256) {
		return errors.New("implementation.artifact_sha256 must be a lowercase SHA-256 digest")
	}
	if submission.Implementation.ArtifactSizeBytes <= 0 {
		return errors.New("implementation.artifact_size_bytes must be positive")
	}
	if err := validateSortedTokens("interfaces", submission.Interfaces); err != nil {
		return err
	}
	if err := validateSortedTokens("platforms", submission.Platforms); err != nil {
		return err
	}
	if submission.Execution.Database != EvaluationDatabaseEphemeralPostgres {
		return fmt.Errorf("execution.database must be %q", EvaluationDatabaseEphemeralPostgres)
	}
	if submission.Execution.Provider != EvaluationProviderOwnedProxy {
		return fmt.Errorf("execution.provider must be %q", EvaluationProviderOwnedProxy)
	}
	if submission.Execution.Network != EvaluationNetworkDenyExceptProvider {
		return fmt.Errorf("execution.network must be %q", EvaluationNetworkDenyExceptProvider)
	}
	if submission.Execution.Telemetry != EvaluationTelemetryDisabled {
		return fmt.Errorf("execution.telemetry must be %q", EvaluationTelemetryDisabled)
	}
	if submission.Execution.ResultArtifact != EvaluationResultEvaluatorControlled {
		return fmt.Errorf("execution.result_artifact must be %q", EvaluationResultEvaluatorControlled)
	}
	return nil
}

func ParseEvaluationSubmission(data []byte) (EvaluationSubmission, error) {
	var submission EvaluationSubmission
	if err := decodeStrictJSON(data, &submission); err != nil {
		return EvaluationSubmission{}, fmt.Errorf("decode evaluation submission: %w", err)
	}
	if err := ValidateEvaluationSubmission(submission); err != nil {
		return EvaluationSubmission{}, err
	}
	return submission, nil
}

func MarshalEvaluationSubmission(submission EvaluationSubmission) ([]byte, error) {
	if err := ValidateEvaluationSubmission(submission); err != nil {
		return nil, err
	}
	return marshalIndented(submission)
}

func WriteEvaluationSubmission(path string, submission EvaluationSubmission) error {
	data, err := MarshalEvaluationSubmission(submission)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func EvaluationSubmissionDigest(submission EvaluationSubmission) (string, error) {
	if err := ValidateEvaluationSubmission(submission); err != nil {
		return "", err
	}
	data, err := json.Marshal(submission)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func VerifyEvaluationSubmissionArtifact(submission EvaluationSubmission, artifactPath string) error {
	if err := ValidateEvaluationSubmission(submission); err != nil {
		return err
	}
	file, err := os.Open(artifactPath)
	if err != nil {
		return fmt.Errorf("open implementation artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat implementation artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != submission.Implementation.ArtifactSizeBytes {
		return errors.New("implementation artifact size does not match submission")
	}
	if filepath.Base(artifactPath) != submission.Implementation.ArtifactName {
		return errors.New("implementation artifact filename does not match submission")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash implementation artifact: %w", err)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != submission.Implementation.ArtifactSHA256 {
		return fmt.Errorf("implementation artifact SHA-256 is %s, expected %s", actual, submission.Implementation.ArtifactSHA256)
	}
	return nil
}

func validateArtifactURI(value, artifactName string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("implementation.artifact_uri must be an HTTPS URI without credentials, query parameters, or fragments")
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || path.Base(decodedPath) != artifactName {
		return errors.New("implementation.artifact_uri path must end with artifact_name")
	}
	return nil
}

func validateSortedTokens(field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	for index, value := range values {
		if !validEvaluationToken(value) {
			return fmt.Errorf("%s contains invalid token %q", field, value)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("%s must be sorted and contain unique values", field)
		}
	}
	return nil
}

func validEvaluationToken(value string) bool {
	if value == "" || len(value) > 128 || value != strings.ToLower(value) {
		return false
	}
	for index, char := range value {
		allowed := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || strings.ContainsRune("._:/-", char)
		if !allowed || index == 0 && !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func validSourceRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	if value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validArtifactName(value string) bool {
	if value == "" || len(value) > 255 || filepath.Base(value) != value || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._+-", char)) {
			return false
		}
	}
	return true
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}
