package operationsprofile

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
	"strconv"
	"strings"
	"time"
)

const reportVersion = 1

var safeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Report struct {
	Version                int             `json:"version"`
	RunID                  string          `json:"run_id"`
	ImplementationRevision string          `json:"implementation_revision"`
	RequestFingerprint     string          `json:"request_fingerprint"`
	PostgreSQLVersion      string          `json:"postgresql_version"`
	StartedAt              time.Time       `json:"started_at"`
	CompletedAt            time.Time       `json:"completed_at"`
	Topology               TopologyReport  `json:"topology"`
	Failover               FailoverReport  `json:"failover"`
	PITR                   PITRReport      `json:"pitr"`
	Security               SecurityReport  `json:"security"`
	HardGates              map[string]bool `json:"hard_gates"`
	Failures               []FailureRecord `json:"failures"`
	NonClaims              []string        `json:"non_claims"`
}

type TopologyReport struct {
	PrimarySystemID  string `json:"primary_system_id"`
	StandbySystemID  string `json:"standby_system_id"`
	PromotedSystemID string `json:"promoted_system_id"`
	RestoredSystemID string `json:"restored_system_id"`
	SameHost         bool   `json:"same_host"`
}

type FailoverReport struct {
	PrimaryFlushLSN     string `json:"primary_flush_lsn"`
	StandbyReplayLSN    string `json:"standby_replay_lsn"`
	DetectionDurationMS int64  `json:"detection_duration_ms"`
	PromotionDurationMS int64  `json:"promotion_duration_ms"`
	ReconnectDurationMS int64  `json:"reconnect_duration_ms"`
	PreFailoverRows     int    `json:"pre_failover_rows"`
	TransitionRows      int    `json:"transition_rows"`
	PostPromotionRows   int    `json:"post_promotion_rows"`
	SameHandler         bool   `json:"same_handler"`
	SameRuntimeStore    bool   `json:"same_runtime_store"`
	SameAuthPool        bool   `json:"same_auth_pool"`
	SameRuntimePool     bool   `json:"same_runtime_pool"`
	PromotedReadWrite   bool   `json:"promoted_read_write"`
}

type PITRReport struct {
	TargetLSN               string `json:"target_lsn"`
	RestoredReplayLSN       string `json:"restored_replay_lsn"`
	T2Fingerprint           string `json:"t2_fingerprint"`
	RestoredFingerprint     string `json:"restored_fingerprint"`
	BaseBackupBytes         int64  `json:"base_backup_bytes"`
	WALArchiveBytes         int64  `json:"wal_archive_bytes"`
	WALInventorySHA256      string `json:"wal_inventory_sha256"`
	DurationMS              int64  `json:"duration_ms"`
	HistoricalStateRestored bool   `json:"historical_state_restored"`
	ProjectionRebuilt       bool   `json:"projection_rebuilt"`
}

type SecurityReport struct {
	HistoricalTokenInitiallyActive bool `json:"historical_token_initially_active"`
	HistoricalTokenRevoked         bool `json:"historical_token_revoked"`
	HistoricalTokenRejected        bool `json:"historical_token_rejected"`
	NewTokenAccepted               bool `json:"new_token_accepted"`
	CurrentStateReconciled         bool `json:"current_state_reconciled"`
	RLSPolicies                    int  `json:"rls_policies"`
	TenantForeignKeys              int  `json:"tenant_foreign_keys"`
}

type FailureRecord struct {
	Phase   string `json:"phase"`
	Attempt int    `json:"attempt"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Retried bool   `json:"retried"`
}

type ArchiveEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Checkpoint struct {
	Version            int               `json:"version"`
	RunID              string            `json:"run_id"`
	RequestFingerprint string            `json:"request_fingerprint"`
	Phase              string            `json:"phase"`
	Status             string            `json:"status"`
	Measurements       map[string]string `json:"measurements"`
	Failures           []FailureRecord   `json:"failures"`
}

type ArtifactPaths struct {
	JSON     string
	Markdown string
}

func WriteReport(root string, report Report) (ArtifactPaths, bool, error) {
	if err := ValidateReport(report); err != nil {
		return ArtifactPaths{}, false, err
	}
	dir := filepath.Join(root, report.RunID)
	paths := ArtifactPaths{
		JSON:     filepath.Join(dir, "report.json"),
		Markdown: filepath.Join(dir, "report.md"),
	}
	jsonData, err := marshalIndented(report)
	if err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("encode report: %w", err)
	}
	markdown := []byte(renderMarkdown(report))

	if existing, err := os.ReadFile(paths.JSON); err == nil {
		existingMarkdown, markdownErr := os.ReadFile(paths.Markdown)
		if string(existing) == string(jsonData) && markdownErr == nil && string(existingMarkdown) == string(markdown) {
			return paths, true, nil
		}
		return ArtifactPaths{}, false, errors.New("conflicting report replay for run_id " + report.RunID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ArtifactPaths{}, false, fmt.Errorf("read existing report: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("create report directory: %w", err)
	}
	jsonTemp, err := writeTemp(dir, "report-json-*", jsonData)
	if err != nil {
		return ArtifactPaths{}, false, err
	}
	defer os.Remove(jsonTemp)
	markdownTemp, err := writeTemp(dir, "report-markdown-*", markdown)
	if err != nil {
		return ArtifactPaths{}, false, err
	}
	defer os.Remove(markdownTemp)
	if err := os.Rename(jsonTemp, paths.JSON); err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("commit JSON report: %w", err)
	}
	if err := os.Rename(markdownTemp, paths.Markdown); err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("commit Markdown report: %w", err)
	}
	return paths, false, nil
}

func WriteCheckpoint(root string, checkpoint Checkpoint) error {
	if err := validateCheckpoint(checkpoint); err != nil {
		return err
	}
	data, err := marshalIndented(checkpoint)
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	dir := filepath.Join(root, "checkpoints")
	path := filepath.Join(dir, checkpoint.Phase+".json")
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == string(data) {
			return nil
		}
		return errors.New("conflicting checkpoint replay for phase " + checkpoint.Phase)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing checkpoint: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}
	temp, err := writeTemp(dir, checkpoint.Phase+"-*", data)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("commit checkpoint: %w", err)
	}
	return nil
}

func InventoryDigest(entries []ArchiveEntry) string {
	sorted := append([]ArchiveEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].Size != sorted[j].Size {
			return sorted[i].Size < sorted[j].Size
		}
		return sorted[i].SHA256 < sorted[j].SHA256
	})
	hash := sha256.New()
	for _, entry := range sorted {
		hash.Write([]byte(entry.Path))
		hash.Write([]byte{0})
		hash.Write([]byte(strconv.FormatInt(entry.Size, 10)))
		hash.Write([]byte{0})
		hash.Write([]byte(entry.SHA256))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func ValidateReport(report Report) error {
	if report.Version != reportVersion {
		return fmt.Errorf("report version must be %d", reportVersion)
	}
	if !safeNamePattern.MatchString(report.RunID) {
		return errors.New("run_id is invalid")
	}
	if !isLowerHex(report.ImplementationRevision, 40) {
		return errors.New("implementation_revision must be a lowercase 40-character Git digest")
	}
	if !strings.HasPrefix(report.PostgreSQLVersion, "18.") {
		return errors.New("postgresql_version must be 18.x")
	}
	if report.StartedAt.IsZero() || report.CompletedAt.Before(report.StartedAt) {
		return errors.New("report timestamps are invalid")
	}
	if !isLowerHex(report.RequestFingerprint, 64) || report.RequestFingerprint != reportRequestFingerprint(report) {
		return errors.New("request_fingerprint is invalid")
	}
	if report.Topology.PrimarySystemID == "" || report.Topology.PrimarySystemID != report.Topology.StandbySystemID || report.Topology.PrimarySystemID != report.Topology.PromotedSystemID || report.Topology.PrimarySystemID != report.Topology.RestoredSystemID {
		return errors.New("topology system identifiers do not match")
	}
	if report.Failover.PrimaryFlushLSN == "" || report.Failover.StandbyReplayLSN == "" ||
		!report.Failover.SameHandler || !report.Failover.SameRuntimeStore || !report.Failover.SameAuthPool ||
		!report.Failover.SameRuntimePool || !report.Failover.PromotedReadWrite {
		return errors.New("failover evidence is incomplete")
	}
	if report.PITR.TargetLSN == "" || report.PITR.RestoredReplayLSN == "" || report.PITR.T2Fingerprint != report.PITR.RestoredFingerprint || !isLowerHex(report.PITR.T2Fingerprint, 64) || !isLowerHex(report.PITR.WALInventorySHA256, 64) || !report.PITR.HistoricalStateRestored || !report.PITR.ProjectionRebuilt {
		return errors.New("PITR evidence is incomplete")
	}
	if !report.Security.HistoricalTokenInitiallyActive || !report.Security.HistoricalTokenRevoked || !report.Security.HistoricalTokenRejected || !report.Security.NewTokenAccepted || !report.Security.CurrentStateReconciled {
		return errors.New("security re-governance evidence is incomplete")
	}
	if len(report.HardGates) == 0 {
		return errors.New("hard gates are required")
	}
	for name, passed := range report.HardGates {
		if strings.TrimSpace(name) == "" || !passed {
			return fmt.Errorf("hard gate %q did not pass", name)
		}
	}
	for _, failure := range report.Failures {
		if failure.Phase == "" || failure.Attempt <= 0 || failure.Code == "" || failure.Message == "" {
			return errors.New("failure records must retain phase, attempt, code, and message")
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode report for secret scan: %w", err)
	}
	if marker := secretMarker(string(encoded)); marker != "" {
		return fmt.Errorf("report contains secret-shaped value %q", marker)
	}
	return nil
}

func reportRequestFingerprint(report Report) string {
	request := struct {
		Version                int    `json:"version"`
		RunID                  string `json:"run_id"`
		ImplementationRevision string `json:"implementation_revision"`
		PostgreSQLVersion      string `json:"postgresql_version"`
	}{
		Version: report.Version, RunID: report.RunID, ImplementationRevision: report.ImplementationRevision,
		PostgreSQLVersion: report.PostgreSQLVersion,
	}
	data, _ := json.Marshal(request)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validateCheckpoint(checkpoint Checkpoint) error {
	if checkpoint.Version != reportVersion || !safeNamePattern.MatchString(checkpoint.RunID) || !safeNamePattern.MatchString(checkpoint.Phase) {
		return errors.New("checkpoint identity is invalid")
	}
	if !isLowerHex(checkpoint.RequestFingerprint, 64) {
		return errors.New("checkpoint request_fingerprint is invalid")
	}
	if checkpoint.Status != "completed" && checkpoint.Status != "failed" {
		return errors.New("checkpoint status must be completed or failed")
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode checkpoint for secret scan: %w", err)
	}
	if marker := secretMarker(string(data)); marker != "" {
		return fmt.Errorf("checkpoint contains secret-shaped value %q", marker)
	}
	return nil
}

func renderMarkdown(report Report) string {
	var output strings.Builder
	fmt.Fprintln(&output, "# PostgreSQL HA And PITR Qualification")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Run ID: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Implementation revision: `%s`\n", report.ImplementationRevision)
	fmt.Fprintf(&output, "- PostgreSQL: `%s`\n", report.PostgreSQLVersion)
	fmt.Fprintf(&output, "- Target LSN: `%s`\n", report.PITR.TargetLSN)
	fmt.Fprintf(&output, "- Failover detection/promotion/reconnect: `%d / %d / %d ms`\n", report.Failover.DetectionDurationMS, report.Failover.PromotionDurationMS, report.Failover.ReconnectDurationMS)
	fmt.Fprintf(&output, "- PITR duration: `%d ms`\n", report.PITR.DurationMS)
	fmt.Fprintln(&output, "- Hard gates: PASS")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Failures Preserved")
	fmt.Fprintln(&output)
	if len(report.Failures) == 0 {
		fmt.Fprintln(&output, "- None.")
	} else {
		for _, failure := range report.Failures {
			fmt.Fprintf(&output, "- `%s` attempt %d: `%s` - %s (retried=%t)\n", failure.Phase, failure.Attempt, failure.Code, failure.Message, failure.Retried)
		}
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Historical-State Security Boundary")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "A PITR restore remains under historical-state quarantine until projections and credentials are re-governed. Historical state restored is not current state reconciled.")
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

func marshalIndented(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeTemp(dir, pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temporary artifact: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", fmt.Errorf("protect temporary artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", fmt.Errorf("write temporary artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary artifact: %w", err)
	}
	return path, nil
}

func secretMarker(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"postgresql://", "password=", "vmt_", "sk-", "authorization:"} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func isLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
