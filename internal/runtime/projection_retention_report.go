package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const projectionRetentionReportVersion = 1

var projectionRetentionSafeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type ProjectionRetentionReport struct {
	Version                int                            `json:"version"`
	RunID                  string                         `json:"run_id"`
	CaseSHA256             string                         `json:"case_sha256"`
	ImplementationRevision string                         `json:"implementation_revision"`
	RequestFingerprint     string                         `json:"request_fingerprint"`
	PostgreSQLVersion      string                         `json:"postgresql_version"`
	PGVectorVersion        string                         `json:"pgvector_version"`
	StartedAt              time.Time                      `json:"started_at"`
	CompletedAt            time.Time                      `json:"completed_at"`
	Environment            ProjectionRetentionEnvironment `json:"environment"`
	Policy                 ProjectionRetentionPolicy      `json:"policy"`
	Counts                 ProjectionRetentionCounts      `json:"counts"`
	Epochs                 []ProjectionRetentionEpoch     `json:"epochs"`
	Profiles               []ProjectionRetentionProfile   `json:"profiles"`
	Receipts               []ProjectionPruneReceipt       `json:"receipts"`
	Restart                ProjectionRetentionRestart     `json:"restart"`
	Queries                ProjectionRetentionQueries     `json:"queries"`
	Rebuild                ProjectionRetentionRebuild     `json:"rebuild"`
	Provider               ProjectionRetentionProvider    `json:"provider"`
	HardGates              map[string]bool                `json:"hard_gates"`
	Failures               []ProjectionRetentionFailure   `json:"failures"`
	NonClaims              []string                       `json:"non_claims"`
}

type ProjectionRetentionEnvironment struct {
	OS            string `json:"os"`
	CPU           string `json:"cpu"`
	MemoryGiB     int    `json:"memory_gib"`
	SchemaVersion int    `json:"schema_version"`
}

type ProjectionRetentionPolicy struct {
	Cutoff           time.Time `json:"cutoff"`
	RetainTailEvents int       `json:"retain_tail_events"`
}

type ProjectionRetentionCounts struct {
	InitialCurrent     int `json:"initial_current"`
	GeneratedEvents    int `json:"generated_events"`
	PrunedEvents       int `json:"pruned_events"`
	RetainedEvents     int `json:"retained_events"`
	FinalCurrent       int `json:"final_current"`
	LexicalRows        int `json:"lexical_rows"`
	IncumbentVectors   int `json:"incumbent_vectors"`
	DimensionalVectors int `json:"dimensional_vectors"`
}

type ProjectionRetentionEpoch struct {
	Epoch      int    `json:"epoch"`
	TenantID   string `json:"tenant_id"`
	Generated  int    `json:"generated"`
	Pruned     int64  `json:"pruned"`
	Retained   int64  `json:"retained"`
	Floor      int64  `json:"floor"`
	SlowCursor int64  `json:"slow_cursor"`
}

type ProjectionRetentionProfile struct {
	TenantID    string `json:"tenant_id"`
	ProfileID   string `json:"profile_id"`
	Status      string `json:"status"`
	LastEventID int64  `json:"last_event_id"`
	Floor       int64  `json:"floor"`
	Lag         int64  `json:"lag"`
	VectorCount int64  `json:"vector_count"`
}

type ProjectionRetentionRestart struct {
	FailureCode             string `json:"failure_code"`
	DeletedEventsRolledBack bool   `json:"deleted_events_rolled_back"`
	FloorRolledBack         bool   `json:"floor_rolled_back"`
	ReceiptRolledBack       bool   `json:"receipt_rolled_back"`
	SamePoolRecovered       bool   `json:"same_pool_recovered"`
	RecoveryDurationMS      int64  `json:"recovery_duration_ms"`
}

type ProjectionRetentionQueries struct {
	Successful          int `json:"successful"`
	CrossScopeResults   int `json:"cross_scope_results"`
	LexicalDegradations int `json:"lexical_degradations"`
	P50MS               int `json:"p50_ms"`
	P95MS               int `json:"p95_ms"`
	P99MS               int `json:"p99_ms"`
}

type ProjectionRetentionRebuild struct {
	FutureSubscriberZeroCalls bool `json:"future_subscriber_zero_calls"`
	ResetRequiredRebuild      bool `json:"reset_required_rebuild"`
	AuthorityIDHashEquivalent bool `json:"authority_id_hash_equivalent"`
	DeletedMemoryAbsent       bool `json:"deleted_memory_absent"`
}

type ProjectionRetentionProvider struct {
	BaseURL                  string `json:"base_url"`
	Model                    string `json:"model"`
	Dimensions               int    `json:"dimensions"`
	Requests                 int    `json:"requests"`
	DurationMS               int64  `json:"duration_ms"`
	ProjectionResponseSHA256 string `json:"projection_response_sha256"`
	QueryResponseSHA256      string `json:"query_response_sha256"`
}

type ProjectionRetentionFailure struct {
	Phase   string `json:"phase"`
	Attempt int    `json:"attempt"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Retried bool   `json:"retried"`
}

type ProjectionRetentionArtifactPaths struct {
	JSON     string
	Markdown string
}

func ReadProjectionRetentionReport(path string) (ProjectionRetentionReport, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return ProjectionRetentionReport{}, fmt.Errorf("read projection retention report: %w", err)
	}
	var report ProjectionRetentionReport
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return ProjectionRetentionReport{}, fmt.Errorf("decode projection retention report: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ProjectionRetentionReport{}, fmt.Errorf("decode projection retention trailing data: %v", err)
	}
	if err := ValidateProjectionRetentionReport(report); err != nil {
		return ProjectionRetentionReport{}, err
	}
	return report, nil
}

func WriteProjectionRetentionReport(root string, report ProjectionRetentionReport) (ProjectionRetentionArtifactPaths, bool, error) {
	if err := ValidateProjectionRetentionReport(report); err != nil {
		return ProjectionRetentionArtifactPaths{}, false, err
	}
	dir := filepath.Join(root, report.RunID)
	paths := ProjectionRetentionArtifactPaths{
		JSON: filepath.Join(dir, "report.json"), Markdown: filepath.Join(dir, "report.md"),
	}
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("encode projection retention report: %w", err)
	}
	jsonData = append(jsonData, '\n')
	markdown := []byte(renderProjectionRetentionMarkdown(report))
	if marker := projectionRetentionSecretMarker(string(markdown)); marker != "" {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("projection retention report contains secret-shaped value %q", marker)
	}
	if existing, err := os.ReadFile(paths.JSON); err == nil {
		existingMarkdown, markdownErr := os.ReadFile(paths.Markdown)
		if string(existing) == string(jsonData) && markdownErr == nil && string(existingMarkdown) == string(markdown) {
			return paths, true, nil
		}
		return ProjectionRetentionArtifactPaths{}, false, errors.New("conflicting report replay for run_id " + report.RunID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("read existing projection retention report: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("create projection retention report directory: %w", err)
	}
	jsonTemp, err := writeProjectionRetentionTemp(dir, "report-json-*", jsonData)
	if err != nil {
		return ProjectionRetentionArtifactPaths{}, false, err
	}
	defer os.Remove(jsonTemp)
	markdownTemp, err := writeProjectionRetentionTemp(dir, "report-markdown-*", markdown)
	if err != nil {
		return ProjectionRetentionArtifactPaths{}, false, err
	}
	defer os.Remove(markdownTemp)
	if err := os.Rename(jsonTemp, paths.JSON); err != nil {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("commit projection retention JSON: %w", err)
	}
	if err := os.Rename(markdownTemp, paths.Markdown); err != nil {
		return ProjectionRetentionArtifactPaths{}, false, fmt.Errorf("commit projection retention Markdown: %w", err)
	}
	return paths, false, nil
}

func ValidateProjectionRetentionReport(report ProjectionRetentionReport) error {
	if report.Version != projectionRetentionReportVersion {
		return fmt.Errorf("projection retention report version must be %d", projectionRetentionReportVersion)
	}
	if !projectionRetentionSafeName.MatchString(report.RunID) {
		return errors.New("projection retention run_id is invalid")
	}
	if !isProjectionRetentionLowerHex(report.CaseSHA256, 64) ||
		!isProjectionRetentionLowerHex(report.ImplementationRevision, 40) {
		return errors.New("projection retention identity hashes are invalid")
	}
	if !strings.HasPrefix(report.PostgreSQLVersion, "18.") || report.PGVectorVersion == "" {
		return errors.New("projection retention database versions are invalid")
	}
	if report.StartedAt.IsZero() || report.CompletedAt.Before(report.StartedAt) {
		return errors.New("projection retention timestamps are invalid")
	}
	if report.RequestFingerprint != projectionRetentionRequestFingerprint(report) {
		return errors.New("projection retention request fingerprint is invalid")
	}
	if report.Environment.SchemaVersion != 17 || report.Environment.OS == "" ||
		report.Environment.CPU == "" || report.Environment.MemoryGiB <= 0 {
		return errors.New("projection retention environment is invalid")
	}
	if report.Policy.Cutoff.IsZero() || report.Policy.RetainTailEvents != 1000 {
		return errors.New("projection retention policy is invalid")
	}
	if report.Counts.InitialCurrent != 20000 || report.Counts.GeneratedEvents != 173600 ||
		report.Counts.FinalCurrent != 20000 || report.Counts.LexicalRows != 20000 ||
		report.Counts.IncumbentVectors != 20000 || report.Counts.DimensionalVectors != 20000 ||
		report.Counts.PrunedEvents < 0 || report.Counts.RetainedEvents != 4000 ||
		report.Counts.PrunedEvents+report.Counts.RetainedEvents != report.Counts.GeneratedEvents {
		return errors.New("projection retention counts are invalid")
	}
	if len(report.Epochs) == 0 || len(report.Profiles) == 0 || len(report.Receipts) == 0 {
		return errors.New("projection retention trajectory is incomplete")
	}
	for _, epoch := range report.Epochs {
		if epoch.Epoch <= 0 || strings.TrimSpace(epoch.TenantID) == "" || epoch.Generated < 0 ||
			epoch.Pruned < 0 || epoch.Retained < 0 || epoch.Floor < 0 || epoch.SlowCursor < 0 {
			return errors.New("projection retention epoch is invalid")
		}
	}
	for _, profile := range report.Profiles {
		if profile.TenantID == "" || !IsSupportedRetrievalProfileID(profile.ProfileID) ||
			profile.LastEventID < 0 || profile.Floor < 0 || profile.Lag < 0 || profile.VectorCount < 0 {
			return errors.New("projection retention profile state is invalid")
		}
		if profile.Status != "idle" && profile.Status != "running" &&
			profile.Status != "failed" && profile.Status != ProjectionStatusRebuildRequired {
			return errors.New("projection retention profile status is invalid")
		}
	}
	receiptKeys := map[string]struct{}{}
	lastFloor := map[string]int64{}
	for _, receipt := range report.Receipts {
		key := receipt.TenantID + "\x00" + receipt.OperationID
		if _, exists := receiptKeys[key]; exists {
			return errors.New("projection retention receipts contain duplicates")
		}
		receiptKeys[key] = struct{}{}
		if receipt.ID == "" || receipt.TenantID == "" || receipt.OperationID == "" ||
			!isProjectionRetentionLowerHex(receipt.RequestFingerprint, 64) || receipt.Cutoff.IsZero() ||
			receipt.RetainTailEvents < 0 || receipt.SafeCursorEventID < 0 ||
			receipt.PreviousFloorEventID != lastFloor[receipt.TenantID] ||
			receipt.NewFloorEventID < receipt.PreviousFloorEventID || receipt.DeletedEvents < 0 ||
			(receipt.Result != "pruned" && receipt.Result != "noop") || receipt.CreatedAt.IsZero() || receipt.Replayed {
			return errors.New("projection retention receipt is invalid")
		}
		lastFloor[receipt.TenantID] = receipt.NewFloorEventID
	}
	if report.Restart.FailureCode == "" || !report.Restart.DeletedEventsRolledBack ||
		!report.Restart.FloorRolledBack || !report.Restart.ReceiptRolledBack ||
		!report.Restart.SamePoolRecovered || report.Restart.RecoveryDurationMS < 0 {
		return errors.New("projection retention restart evidence is invalid")
	}
	if report.Queries.Successful != 320 || report.Queries.CrossScopeResults != 0 ||
		report.Queries.LexicalDegradations < 0 || report.Queries.P50MS < 0 ||
		report.Queries.P50MS > report.Queries.P95MS || report.Queries.P95MS > report.Queries.P99MS {
		return errors.New("projection retention query evidence is invalid")
	}
	if !report.Rebuild.FutureSubscriberZeroCalls || !report.Rebuild.ResetRequiredRebuild ||
		!report.Rebuild.AuthorityIDHashEquivalent || !report.Rebuild.DeletedMemoryAbsent {
		return errors.New("projection retention rebuild evidence is invalid")
	}
	if report.Provider.BaseURL != "https://api.siliconflow.cn/v1" ||
		report.Provider.Model != "BAAI/bge-m3" || report.Provider.Dimensions != 1024 ||
		report.Provider.Requests != 2 || report.Provider.DurationMS < 0 ||
		!isProjectionRetentionLowerHex(report.Provider.ProjectionResponseSHA256, 64) ||
		!isProjectionRetentionLowerHex(report.Provider.QueryResponseSHA256, 64) {
		return errors.New("projection retention provider evidence is invalid")
	}
	if len(report.HardGates) != 14 {
		return fmt.Errorf("projection retention hard gate count=%d want 14", len(report.HardGates))
	}
	for name, passed := range report.HardGates {
		if strings.TrimSpace(name) == "" || !passed {
			return fmt.Errorf("projection retention hard gate %q did not pass", name)
		}
	}
	for _, failure := range report.Failures {
		if failure.Phase == "" || failure.Attempt <= 0 || failure.Code == "" || failure.Message == "" {
			return errors.New("projection retention failure records are incomplete")
		}
	}
	if len(report.NonClaims) == 0 {
		return errors.New("projection retention non-claims are missing")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode projection retention report for secret scan: %w", err)
	}
	if marker := projectionRetentionSecretMarker(string(encoded)); marker != "" {
		return fmt.Errorf("projection retention report contains secret-shaped value %q", marker)
	}
	return nil
}

func projectionRetentionRequestFingerprint(report ProjectionRetentionReport) string {
	request := struct {
		Version                int    `json:"version"`
		RunID                  string `json:"run_id"`
		CaseSHA256             string `json:"case_sha256"`
		ImplementationRevision string `json:"implementation_revision"`
		PostgreSQLVersion      string `json:"postgresql_version"`
		Cutoff                 string `json:"cutoff"`
		RetainTailEvents       int    `json:"retain_tail_events"`
	}{
		Version: report.Version, RunID: report.RunID, CaseSHA256: report.CaseSHA256,
		ImplementationRevision: report.ImplementationRevision, PostgreSQLVersion: report.PostgreSQLVersion,
		Cutoff: report.Policy.Cutoff.UTC().Format(time.RFC3339Nano), RetainTailEvents: report.Policy.RetainTailEvents,
	}
	payload, _ := json.Marshal(request)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func renderProjectionRetentionMarkdown(report ProjectionRetentionReport) string {
	var output strings.Builder
	fmt.Fprintln(&output, "# Projection Event Retention Qualification")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Run: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Implementation: `%s`\n", report.ImplementationRevision)
	fmt.Fprintf(&output, "- PostgreSQL / pgvector: `%s / %s`\n", report.PostgreSQLVersion, report.PGVectorVersion)
	fmt.Fprintf(&output, "- Generated / pruned / retained events: `%d / %d / %d`\n", report.Counts.GeneratedEvents, report.Counts.PrunedEvents, report.Counts.RetainedEvents)
	fmt.Fprintf(&output, "- Current authority / lexical / incumbent / dimensional: `%d / %d / %d / %d`\n", report.Counts.FinalCurrent, report.Counts.LexicalRows, report.Counts.IncumbentVectors, report.Counts.DimensionalVectors)
	fmt.Fprintf(&output, "- Query p50/p95/p99: `%d / %d / %d ms`\n", report.Queries.P50MS, report.Queries.P95MS, report.Queries.P99MS)
	fmt.Fprintf(&output, "- Provider: `%s` / `%s` / `%d` dimensions / `%d` requests\n", report.Provider.BaseURL, report.Provider.Model, report.Provider.Dimensions, report.Provider.Requests)
	fmt.Fprintln(&output, "- Hard gates: `14 / 14` PASS")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Profile States")
	fmt.Fprintln(&output)
	for _, profile := range report.Profiles {
		fmt.Fprintf(&output, "- `%s` / `%s`: status=`%s`, cursor=`%d`, floor=`%d`, lag=`%d`, vectors=`%d`\n",
			profile.TenantID, profile.ProfileID, profile.Status, profile.LastEventID, profile.Floor, profile.Lag, profile.VectorCount)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Restart And Rebuild")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Restart failure: `%s`; delete/floor/receipt rollback=`%t/%t/%t`; same pool=`%t`\n",
		report.Restart.FailureCode, report.Restart.DeletedEventsRolledBack, report.Restart.FloorRolledBack,
		report.Restart.ReceiptRolledBack, report.Restart.SamePoolRecovered)
	fmt.Fprintf(&output, "- Future subscriber blocked with `%s`: `%t`\n", ProjectionFailureRebuildRequired, report.Rebuild.FutureSubscriberZeroCalls)
	fmt.Fprintf(&output, "- Reset requires rebuild / authority equivalent / deleted absent: `%t / %t / %t`\n",
		report.Rebuild.ResetRequiredRebuild, report.Rebuild.AuthorityIDHashEquivalent, report.Rebuild.DeletedMemoryAbsent)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Failures Preserved")
	fmt.Fprintln(&output)
	for _, failure := range report.Failures {
		fmt.Fprintf(&output, "- `%s` attempt %d: `%s` - %s (retried=%t)\n", failure.Phase, failure.Attempt, failure.Code, failure.Message, failure.Retried)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Hard Gates")
	fmt.Fprintln(&output)
	keys := make([]string, 0, len(report.HardGates))
	for key := range report.HardGates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&output, "- `%s`: PASS\n", key)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Non-Claims")
	fmt.Fprintln(&output)
	for _, nonClaim := range report.NonClaims {
		fmt.Fprintf(&output, "- %s\n", nonClaim)
	}
	return output.String()
}

func writeProjectionRetentionTemp(dir, pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create projection retention temporary artifact: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", fmt.Errorf("protect projection retention temporary artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", fmt.Errorf("write projection retention temporary artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync projection retention temporary artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close projection retention temporary artifact: %w", err)
	}
	return path, nil
}

func projectionRetentionSecretMarker(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"postgresql://", "password=", "sk-", "authorization:", "bearer "} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func isProjectionRetentionLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
