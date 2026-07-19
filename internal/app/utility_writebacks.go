package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"vermory/internal/runtime"
	"vermory/internal/utilityeval"
)

type UtilityWritebackOptions struct {
	ProfilePath  string
	BundlePath   string
	ReportPath   string
	DatabaseURL  string
	EvidencePath string
}

type UtilityWritebackEvidence struct {
	Version        string                       `json:"version"`
	RunID          string                       `json:"run_id"`
	ProfileID      string                       `json:"profile_id"`
	ProfileSHA256  string                       `json:"profile_sha256"`
	ReportSHA256   string                       `json:"report_sha256"`
	Scorer         string                       `json:"scorer"`
	WritebackState string                       `json:"writeback_state"`
	Records        []UtilityWritebackCaseRecord `json:"records"`
}

type UtilityWritebackCaseRecord struct {
	CaseID          string `json:"case_id"`
	DeliveryID      string `json:"delivery_id"`
	OutputSHA256    string `json:"output_sha256"`
	ObservationID   string `json:"observation_id"`
	MemoryID        string `json:"memory_id"`
	MemoryStatus    string `json:"memory_status"`
	FirstReplayed   bool   `json:"first_replayed"`
	ReplayConfirmed bool   `json:"replay_confirmed"`
}

type utilityNativeWriteback struct {
	caseID       string
	deliveryID   string
	output       string
	outputSHA256 string
}

func RecordUtilityWritebacks(ctx context.Context, opts UtilityWritebackOptions) (UtilityWritebackEvidence, error) {
	profile, err := utilityeval.LoadProfile(opts.ProfilePath)
	if err != nil {
		return UtilityWritebackEvidence{}, err
	}
	bundle, err := utilityeval.LoadContextBundle(opts.BundlePath)
	if err != nil {
		return UtilityWritebackEvidence{}, err
	}
	reportBytes, report, err := loadUtilityReport(opts.ReportPath)
	if err != nil {
		return UtilityWritebackEvidence{}, err
	}
	writes, err := validateUtilityWritebackInputs(profile, bundle, report)
	if err != nil {
		return UtilityWritebackEvidence{}, err
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" || strings.TrimSpace(opts.EvidencePath) == "" {
		return UtilityWritebackEvidence{}, fmt.Errorf("utility writeback requires database URL and evidence path")
	}

	store, err := runtime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return UtilityWritebackEvidence{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return UtilityWritebackEvidence{}, err
	}

	evidence := UtilityWritebackEvidence{
		Version:        "1",
		RunID:          report.RunID,
		ProfileID:      profile.ID,
		ProfileSHA256:  profile.SHA256,
		ReportSHA256:   sha256Bytes(reportBytes),
		Scorer:         report.Scorer,
		WritebackState: "proposed",
		Records:        make([]UtilityWritebackCaseRecord, 0, len(writes)),
	}
	for _, write := range writes {
		tenantID := "w27-" + compactIdentifier(write.caseID)
		service := runtime.NewService(store, tenantID)
		request := runtime.CommitObservationRequest{
			OperationID: "w27-writeback:" + report.RunID + ":" + compactIdentifier(write.caseID),
			DeliveryID:  write.deliveryID,
			Kind:        runtime.ObservationKindAgentResult,
			Content:     write.output,
			SourceRef:   "utility-run:" + report.RunID + ":" + write.caseID + ":vermory_native",
		}
		first, err := service.CommitObservation(ctx, request)
		if err != nil {
			return UtilityWritebackEvidence{}, fmt.Errorf("write back %s: %w", write.caseID, err)
		}
		if first.MemoryStatus != profile.NativeWritebackStatus {
			return UtilityWritebackEvidence{}, fmt.Errorf("write back %s status is %q, expected %q", write.caseID, first.MemoryStatus, profile.NativeWritebackStatus)
		}
		replay, err := service.CommitObservation(ctx, request)
		if err != nil {
			return UtilityWritebackEvidence{}, fmt.Errorf("replay write back %s: %w", write.caseID, err)
		}
		if !replay.Replayed || replay.ObservationID != first.ObservationID || replay.MemoryID != first.MemoryID || replay.MemoryStatus != first.MemoryStatus {
			return UtilityWritebackEvidence{}, fmt.Errorf("write back %s replay was not idempotent", write.caseID)
		}
		evidence.Records = append(evidence.Records, UtilityWritebackCaseRecord{
			CaseID:          write.caseID,
			DeliveryID:      write.deliveryID,
			OutputSHA256:    write.outputSHA256,
			ObservationID:   first.ObservationID,
			MemoryID:        first.MemoryID,
			MemoryStatus:    first.MemoryStatus,
			FirstReplayed:   first.Replayed,
			ReplayConfirmed: true,
		})
	}
	if err := writeUtilityWritebackEvidence(opts.EvidencePath, evidence); err != nil {
		return UtilityWritebackEvidence{}, err
	}
	return evidence, nil
}

func validateUtilityWritebackInputs(profile utilityeval.Profile, bundle utilityeval.ContextBundle, report utilityeval.Report) ([]utilityNativeWriteback, error) {
	if bundle.ProfileID != profile.ID || bundle.ProfileSHA256 != profile.SHA256 || bundle.ScorerVersion != profile.ScorerVersion {
		return nil, fmt.Errorf("utility writeback bundle does not match the profile")
	}
	if report.ProfileID != profile.ID || report.ProfileSHA256 != profile.SHA256 || report.Scorer != profile.ScorerVersion {
		return nil, fmt.Errorf("utility writeback report does not match the profile")
	}
	lane, err := utilityProviderLane(profile, report.ProviderName, report.Model)
	if err != nil {
		return nil, err
	}
	if report.ProviderMode != "real" || report.Workers != profile.Workers || report.DisableThinking != lane.DisableThinking || report.Temperature != lane.Temperature {
		return nil, fmt.Errorf("utility writeback report execution parameters do not match the profile")
	}
	if err := validateBundleCases(profile.RealityCaseIDs, bundle.Inputs); err != nil {
		return nil, err
	}
	if err := validateBundleScoringAliases(profile.ScoringAliases, bundle.Inputs); err != nil {
		return nil, err
	}

	bundleByCase := make(map[string]utilityeval.CaseInput, len(bundle.Inputs))
	for _, input := range bundle.Inputs {
		bundleByCase[input.ID] = input
	}
	if len(report.Results) != profile.CallsPerCompleteLane {
		return nil, fmt.Errorf("utility writeback report has %d calls, expected %d", len(report.Results), profile.CallsPerCompleteLane)
	}
	seenCalls := make(map[string]struct{}, profile.CallsPerCompleteLane)
	results := make(map[string]utilityeval.CallResult, len(profile.RealityCaseIDs))
	for _, result := range report.Results {
		input, knownCase := bundleByCase[result.CaseID]
		expectedContext, knownCondition := input.Context[result.Condition]
		if !knownCase || !knownCondition {
			return nil, fmt.Errorf("utility writeback report contains undeclared call %q %q", result.CaseID, result.Condition)
		}
		callID := result.CaseID + "\x00" + string(result.Condition)
		if _, exists := seenCalls[callID]; exists {
			return nil, fmt.Errorf("utility writeback report repeats call %q %q", result.CaseID, result.Condition)
		}
		seenCalls[callID] = struct{}{}
		if result.Status != "completed" {
			return nil, fmt.Errorf("utility writeback report call %q %q is not completed", result.CaseID, result.Condition)
		}
		if result.Context != expectedContext {
			return nil, fmt.Errorf("utility writeback report call %q %q changed its frozen context", result.CaseID, result.Condition)
		}
		if result.Condition != utilityeval.ConditionVermoryNative {
			continue
		}
		if _, exists := results[result.CaseID]; exists {
			return nil, fmt.Errorf("utility writeback report repeats native case %q", result.CaseID)
		}
		results[result.CaseID] = result
	}

	writes := make([]utilityNativeWriteback, 0, len(profile.RealityCaseIDs))
	for _, caseID := range profile.RealityCaseIDs {
		result, ok := results[caseID]
		if !ok || result.Status != "completed" || !result.Score.Success || result.Score.ForbiddenHits != 0 {
			return nil, fmt.Errorf("utility writeback native case %q is not successful", caseID)
		}
		expected := bundleByCase[caseID].Context[utilityeval.ConditionVermoryNative]
		if result.Context.DeliveryID == "" || result.Context != expected {
			return nil, fmt.Errorf("utility writeback native case %q changed its frozen delivery", caseID)
		}
		output, outputSHA256, err := readUtilityOutput(result.OutputURI)
		if err != nil {
			return nil, fmt.Errorf("utility writeback output %s: %w", caseID, err)
		}
		if outputSHA256 != result.OutputSHA256 {
			return nil, fmt.Errorf("utility writeback output %s digest mismatch", caseID)
		}
		writes = append(writes, utilityNativeWriteback{
			caseID:       caseID,
			deliveryID:   result.Context.DeliveryID,
			output:       output,
			outputSHA256: outputSHA256,
		})
	}
	return writes, nil
}

func loadUtilityReport(path string) ([]byte, utilityeval.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, utilityeval.Report{}, err
	}
	var report utilityeval.Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return nil, utilityeval.Report{}, fmt.Errorf("decode utility report: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, utilityeval.Report{}, fmt.Errorf("decode utility report: trailing JSON")
	}
	if err := report.Validate(); err != nil {
		return nil, utilityeval.Report{}, err
	}
	return data, report, nil
}

func readUtilityOutput(uri string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
		return "", "", fmt.Errorf("output URI must be a local file URI")
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	output := strings.TrimSpace(string(data))
	if output == "" {
		return "", "", fmt.Errorf("output is empty")
	}
	return output, sha256Bytes(data), nil
}

func writeUtilityWritebackEvidence(path string, evidence UtilityWritebackEvidence) error {
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".utility-writebacks-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func sha256Bytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
