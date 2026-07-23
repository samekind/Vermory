package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const dimensionalMigrationReportVersion = 1

var dimensionalMigrationSafeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type DimensionalMigrationReport struct {
	Version                int                             `json:"version"`
	RunID                  string                          `json:"run_id"`
	CaseSHA256             string                          `json:"case_sha256"`
	ImplementationRevision string                          `json:"implementation_revision"`
	RequestFingerprint     string                          `json:"request_fingerprint"`
	PostgreSQLVersion      string                          `json:"postgresql_version"`
	PGVectorVersion        string                          `json:"pgvector_version"`
	StartedAt              time.Time                       `json:"started_at"`
	CompletedAt            time.Time                       `json:"completed_at"`
	Environment            DimensionalMigrationEnvironment `json:"environment"`
	Profiles               DimensionalMigrationProfiles    `json:"profiles"`
	Counts                 DimensionalMigrationCounts      `json:"counts"`
	Snapshot               DimensionalMigrationSnapshot    `json:"snapshot"`
	Workload               DimensionalMigrationWorkload    `json:"workload"`
	Restart                DimensionalMigrationRestart     `json:"restart"`
	Queries                DimensionalMigrationQueries     `json:"queries"`
	Reset                  DimensionalMigrationReset       `json:"reset"`
	Provider               DimensionalMigrationProvider    `json:"provider"`
	HardGates              map[string]bool                 `json:"hard_gates"`
	Failures               []DimensionalMigrationFailure   `json:"failures"`
	NonClaims              []string                        `json:"non_claims"`
}

type DimensionalMigrationEnvironment struct {
	OS            string `json:"os"`
	CPU           string `json:"cpu"`
	MemoryGiB     int    `json:"memory_gib"`
	SchemaVersion int    `json:"schema_version"`
}

type DimensionalMigrationProfiles struct {
	IncumbentID         string          `json:"incumbent_id"`
	IncumbentClass      ProjectionClass `json:"incumbent_class"`
	IncumbentDimensions int             `json:"incumbent_dimensions"`
	IncumbentLifecycle  string          `json:"incumbent_lifecycle"`
	CandidateID         string          `json:"candidate_id"`
	CandidateClass      ProjectionClass `json:"candidate_class"`
	CandidateDimensions int             `json:"candidate_dimensions"`
	CandidateLifecycle  string          `json:"candidate_lifecycle"`
}

type DimensionalMigrationCounts struct {
	InitialActive         int `json:"initial_active"`
	Revisions             int `json:"revisions"`
	Deleted               int `json:"deleted"`
	NewFacts              int `json:"new_facts"`
	TailEvents            int `json:"tail_events"`
	FinalActive           int `json:"final_active"`
	LexicalRows           int `json:"lexical_rows"`
	IncumbentFinalVectors int `json:"incumbent_final_vectors"`
	CandidateFinalVectors int `json:"candidate_final_vectors"`
}

type DimensionalMigrationSnapshot struct {
	IncumbentProjected      int   `json:"incumbent_projected"`
	CandidateScanned        int   `json:"candidate_scanned"`
	CandidateProjected      int   `json:"candidate_projected"`
	CandidateSkippedChanged int   `json:"candidate_skipped_changed"`
	CandidateFinalLag       int64 `json:"candidate_final_lag"`
}

type DimensionalMigrationWorkload struct {
	QueryClients                            int   `json:"query_clients"`
	QuerySamples                            int   `json:"query_samples"`
	WriterDurationMS                        int64 `json:"writer_duration_ms"`
	AuthorityCompletedWhileCandidateBlocked bool  `json:"authority_completed_while_candidate_blocked"`
}

type DimensionalMigrationRestart struct {
	FailureCode              string `json:"failure_code"`
	PartialRows              int    `json:"partial_rows"`
	InterruptedCursorAdvance int64  `json:"interrupted_cursor_advance"`
	RecoveryDurationMS       int64  `json:"recovery_duration_ms"`
	SamePoolsRecovered       bool   `json:"same_pools_recovered"`
}

type DimensionalMigrationQueries struct {
	Successful              int `json:"successful"`
	BoundedDatabaseFailures int `json:"bounded_database_failures"`
	CrossScopeResults       int `json:"cross_scope_results"`
	P50MS                   int `json:"p50_ms"`
	P95MS                   int `json:"p95_ms"`
	P99MS                   int `json:"p99_ms"`
}

type DimensionalMigrationReset struct {
	CandidateRowsAfterReset int  `json:"candidate_rows_after_reset"`
	IncumbentRowsUnchanged  bool `json:"incumbent_rows_unchanged"`
	AuthorityUnchanged      bool `json:"authority_unchanged"`
	CandidateRebuilt        bool `json:"candidate_rebuilt"`
}

type DimensionalMigrationProvider struct {
	BaseURL                  string `json:"base_url"`
	Model                    string `json:"model"`
	Dimensions               int    `json:"dimensions"`
	Requests                 int    `json:"requests"`
	DurationMS               int64  `json:"duration_ms"`
	ProjectionResponseSHA256 string `json:"projection_response_sha256"`
	QueryResponseSHA256      string `json:"query_response_sha256"`
}

type DimensionalMigrationFailure struct {
	Phase   string `json:"phase"`
	Attempt int    `json:"attempt"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Retried bool   `json:"retried"`
}

type DimensionalMigrationArtifactPaths struct {
	JSON     string
	Markdown string
}

func ReadDimensionalMigrationReport(path string) (DimensionalMigrationReport, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return DimensionalMigrationReport{}, fmt.Errorf("read dimensional migration report: %w", err)
	}
	var report DimensionalMigrationReport
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return DimensionalMigrationReport{}, fmt.Errorf("decode dimensional migration report: %w", err)
	}
	if err := ValidateDimensionalMigrationReport(report); err != nil {
		return DimensionalMigrationReport{}, err
	}
	return report, nil
}

func WriteDimensionalMigrationReport(root string, report DimensionalMigrationReport) (DimensionalMigrationArtifactPaths, bool, error) {
	if err := ValidateDimensionalMigrationReport(report); err != nil {
		return DimensionalMigrationArtifactPaths{}, false, err
	}
	dir := filepath.Join(root, report.RunID)
	paths := DimensionalMigrationArtifactPaths{
		JSON: filepath.Join(dir, "report.json"), Markdown: filepath.Join(dir, "report.md"),
	}
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return DimensionalMigrationArtifactPaths{}, false, fmt.Errorf("encode dimensional migration report: %w", err)
	}
	jsonData = append(jsonData, '\n')
	markdown := []byte(renderDimensionalMigrationMarkdown(report))
	if existing, err := os.ReadFile(paths.JSON); err == nil {
		existingMarkdown, markdownErr := os.ReadFile(paths.Markdown)
		if string(existing) == string(jsonData) && markdownErr == nil && string(existingMarkdown) == string(markdown) {
			return paths, true, nil
		}
		return DimensionalMigrationArtifactPaths{}, false, errors.New("conflicting report replay for run_id " + report.RunID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return DimensionalMigrationArtifactPaths{}, false, fmt.Errorf("read existing dimensional migration report: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DimensionalMigrationArtifactPaths{}, false, fmt.Errorf("create dimensional migration report directory: %w", err)
	}
	jsonTemp, err := writeDimensionalMigrationTemp(dir, "report-json-*", jsonData)
	if err != nil {
		return DimensionalMigrationArtifactPaths{}, false, err
	}
	defer os.Remove(jsonTemp)
	markdownTemp, err := writeDimensionalMigrationTemp(dir, "report-markdown-*", markdown)
	if err != nil {
		return DimensionalMigrationArtifactPaths{}, false, err
	}
	defer os.Remove(markdownTemp)
	if err := os.Rename(jsonTemp, paths.JSON); err != nil {
		return DimensionalMigrationArtifactPaths{}, false, fmt.Errorf("commit dimensional migration JSON: %w", err)
	}
	if err := os.Rename(markdownTemp, paths.Markdown); err != nil {
		return DimensionalMigrationArtifactPaths{}, false, fmt.Errorf("commit dimensional migration Markdown: %w", err)
	}
	return paths, false, nil
}

func ValidateDimensionalMigrationReport(report DimensionalMigrationReport) error {
	if report.Version != dimensionalMigrationReportVersion {
		return fmt.Errorf("dimensional migration report version must be %d", dimensionalMigrationReportVersion)
	}
	if !dimensionalMigrationSafeName.MatchString(report.RunID) {
		return errors.New("dimensional migration run_id is invalid")
	}
	if !isDimensionalMigrationLowerHex(report.CaseSHA256, 64) || !isDimensionalMigrationLowerHex(report.ImplementationRevision, 40) {
		return errors.New("dimensional migration identity hashes are invalid")
	}
	if !strings.HasPrefix(report.PostgreSQLVersion, "18.") || report.PGVectorVersion == "" {
		return errors.New("dimensional migration database versions are invalid")
	}
	if report.StartedAt.IsZero() || report.CompletedAt.Before(report.StartedAt) {
		return errors.New("dimensional migration timestamps are invalid")
	}
	if report.RequestFingerprint != dimensionalMigrationRequestFingerprint(report) {
		return errors.New("dimensional migration request fingerprint is invalid")
	}
	if report.Environment.SchemaVersion != 16 || report.Profiles.IncumbentID != ProductionRetrievalProfileID ||
		report.Profiles.IncumbentClass != ProjectionClass1024 || report.Profiles.IncumbentDimensions != 1024 ||
		report.Profiles.IncumbentLifecycle != "active" || report.Profiles.CandidateID != DimensionalMigrationRetrievalProfileID ||
		report.Profiles.CandidateClass != ProjectionClass2560 || report.Profiles.CandidateDimensions != 2560 ||
		report.Profiles.CandidateLifecycle != "candidate" {
		return errors.New("dimensional migration profile contract is invalid")
	}
	if report.Queries.P50MS < 0 || report.Queries.P50MS > report.Queries.P95MS || report.Queries.P95MS > report.Queries.P99MS {
		return errors.New("dimensional migration latency percentiles are invalid")
	}
	if len(report.HardGates) != 12 {
		return fmt.Errorf("dimensional migration hard gate count=%d want 12", len(report.HardGates))
	}
	for name, passed := range report.HardGates {
		if strings.TrimSpace(name) == "" || !passed {
			return fmt.Errorf("dimensional migration hard gate %q did not pass", name)
		}
	}
	for _, failure := range report.Failures {
		if failure.Phase == "" || failure.Attempt <= 0 || failure.Code == "" || failure.Message == "" {
			return errors.New("dimensional migration failure records are incomplete")
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode dimensional migration report for secret scan: %w", err)
	}
	if marker := dimensionalMigrationSecretMarker(string(encoded)); marker != "" {
		return fmt.Errorf("dimensional migration report contains secret-shaped value %q", marker)
	}
	return nil
}

func dimensionalMigrationRequestFingerprint(report DimensionalMigrationReport) string {
	request := struct {
		Version                int    `json:"version"`
		RunID                  string `json:"run_id"`
		CaseSHA256             string `json:"case_sha256"`
		ImplementationRevision string `json:"implementation_revision"`
		PostgreSQLVersion      string `json:"postgresql_version"`
		IncumbentID            string `json:"incumbent_id"`
		CandidateID            string `json:"candidate_id"`
		IncumbentDimensions    int    `json:"incumbent_dimensions"`
		CandidateDimensions    int    `json:"candidate_dimensions"`
	}{
		Version: report.Version, RunID: report.RunID, CaseSHA256: report.CaseSHA256,
		ImplementationRevision: report.ImplementationRevision, PostgreSQLVersion: report.PostgreSQLVersion,
		IncumbentID: report.Profiles.IncumbentID, CandidateID: report.Profiles.CandidateID,
		IncumbentDimensions: report.Profiles.IncumbentDimensions, CandidateDimensions: report.Profiles.CandidateDimensions,
	}
	data, _ := json.Marshal(request)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func renderDimensionalMigrationMarkdown(report DimensionalMigrationReport) string {
	var output strings.Builder
	fmt.Fprintln(&output, "# Active-Backlog Dimensional Migration Qualification")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Run ID: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Implementation revision: `%s`\n", report.ImplementationRevision)
	fmt.Fprintf(&output, "- PostgreSQL/pgvector: `%s` / `%s`\n", report.PostgreSQLVersion, report.PGVectorVersion)
	fmt.Fprintf(&output, "- Incumbent: `%s` / `%s` / `%d` dimensions\n", report.Profiles.IncumbentID, report.Profiles.IncumbentLifecycle, report.Profiles.IncumbentDimensions)
	fmt.Fprintf(&output, "- Candidate: `%s` / `%s` / `%d` dimensions\n", report.Profiles.CandidateID, report.Profiles.CandidateLifecycle, report.Profiles.CandidateDimensions)
	fmt.Fprintf(&output, "- Initial/final active: `%d` / `%d`\n", report.Counts.InitialActive, report.Counts.FinalActive)
	fmt.Fprintf(&output, "- Tail events: `%d`\n", report.Counts.TailEvents)
	fmt.Fprintf(&output, "- Query p50/p95/p99: `%d / %d / %d ms`\n", report.Queries.P50MS, report.Queries.P95MS, report.Queries.P99MS)
	fmt.Fprintln(&output, "- Hard gates: PASS")
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

func writeDimensionalMigrationTemp(dir, pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create dimensional migration temporary artifact: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", fmt.Errorf("protect dimensional migration temporary artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", fmt.Errorf("write dimensional migration temporary artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync dimensional migration temporary artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close dimensional migration temporary artifact: %w", err)
	}
	return path, nil
}

func dimensionalMigrationSecretMarker(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"postgresql://", "password=", "sk-", "authorization:", "bearer "} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func isDimensionalMigrationLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
