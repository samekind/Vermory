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
	"strings"
	"time"
)

const memoryEligibilityReportVersion = 1

var (
	memoryEligibilitySafeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	memoryEligibilityVector   = regexp.MustCompile(`(-?[0-9]+(\.[0-9]+)?,){8}`)
)

var memoryEligibilityHardGateNames = []string{
	"schema 18 adds tenant-isolated temporal eligibility archive and immutable operation audit",
	"working input does not silently create durable or global memory",
	"one request uses one PostgreSQL-derived eligibility timestamp across every context source",
	"scheduled facts are absent before valid_from and eligible at the boundary",
	"facts are eligible before valid_until and absent exactly at the boundary",
	"expiry preserves inspectable history while removing current reuse",
	"archive preserves inspectable history and removes lexical vector and delivery eligibility",
	"forget redacts current scheduled expired archived and superseded targets without deleting unrelated guidance",
	"G01 local override leaves the Global Default unchanged",
	"workspace and conversation durable facts remain available across client restart and client change",
	"lexical shadow vector bridge Web Chat OpenClaw and MCP enforce identical eligibility",
	"projection rebuild and PostgreSQL restore preserve effective results without reviving forgotten content",
	"validity and archive operations are scoped idempotent conflict rejecting and content-free in audit",
	"concurrent forget wins over validity and archive without resurrection or false receipt",
	"provider outage degrades safely without stale deleted or cross-scope exposure",
	"real Web Chat Grok and MCP Codex or Grok artifacts complete tasks using only eligible memory",
}

type MemoryEligibilityReport struct {
	Version                int                                     `json:"version"`
	RunID                  string                                  `json:"run_id"`
	ProfileID              string                                  `json:"profile_id"`
	Mode                   string                                  `json:"mode"`
	CaseSHA256             string                                  `json:"case_sha256"`
	SchemaVersion          int                                     `json:"schema_version"`
	ImplementationRevision string                                  `json:"implementation_revision"`
	RequestFingerprint     string                                  `json:"request_fingerprint"`
	PostgreSQLVersion      string                                  `json:"postgresql_version"`
	PGVectorVersion        string                                  `json:"pgvector_version"`
	StartedAt              time.Time                               `json:"started_at"`
	CompletedAt            time.Time                               `json:"completed_at"`
	Corpus                 MemoryEligibilityCorpusCounts           `json:"corpus"`
	Queries                MemoryEligibilityQueryMetrics           `json:"queries"`
	Baselines              []MemoryEligibilityBaselineOutcome      `json:"baselines"`
	Metrics                MemoryEligibilityOutcomeMetrics         `json:"metrics"`
	Operations             MemoryEligibilityOperationEvidence      `json:"operations"`
	RestartRestore         MemoryEligibilityRestartRestoreEvidence `json:"restart_restore"`
	Projection             MemoryEligibilityProjectionEvidence     `json:"projection"`
	RealClients            []MemoryEligibilityClientEvidence       `json:"real_clients"`
	Provider               MemoryEligibilityProviderEvidence       `json:"provider"`
	HardGates              []MemoryEligibilityHardGate             `json:"hard_gates"`
	Failures               []MemoryEligibilityFailure              `json:"failures"`
	NonClaims              []string                                `json:"non_claims"`
}

type MemoryEligibilityCorpusCounts struct {
	Tenants             int `json:"tenants"`
	Continuities        int `json:"continuities"`
	Total               int `json:"total"`
	CurrentOpenEnded    int `json:"current_open_ended"`
	Scheduled           int `json:"scheduled"`
	Expired             int `json:"expired"`
	Archived            int `json:"archived"`
	Superseded          int `json:"superseded"`
	Deleted             int `json:"deleted"`
	ActiveLifecycle     int `json:"active_lifecycle"`
	ArchivedLifecycle   int `json:"archived_lifecycle"`
	SupersededLifecycle int `json:"superseded_lifecycle"`
	DeletedLifecycle    int `json:"deleted_lifecycle"`
}

type MemoryEligibilityQueryMetrics struct {
	Clients    int `json:"clients"`
	PerClient  int `json:"per_client"`
	Total      int `json:"total"`
	Successful int `json:"successful"`
	P50MS      int `json:"p50_ms"`
	P95MS      int `json:"p95_ms"`
	P99MS      int `json:"p99_ms"`
}

type MemoryEligibilityBaselineOutcome struct {
	Condition              string `json:"condition"`
	TaskCount              int    `json:"task_count"`
	TaskSuccess            int    `json:"task_success"`
	CurrentFactHits        int    `json:"current_fact_hits"`
	ScheduledPrematureUse  int    `json:"scheduled_premature_use"`
	ExpiredMisuse          int    `json:"expired_misuse"`
	ArchivedMisuse         int    `json:"archived_misuse"`
	DeletionResidue        int    `json:"deletion_residue"`
	GlobalDefaultPollution int    `json:"global_default_pollution"`
	ScopeLeakage           int    `json:"scope_leakage"`
	ContextTokens          int    `json:"context_tokens"`
	DeliveredMemories      int    `json:"delivered_memories"`
}

type MemoryEligibilityOutcomeMetrics struct {
	CurrentExpected        int `json:"current_expected"`
	CurrentReturned        int `json:"current_returned"`
	CurrentRecallBPS       int `json:"current_recall_bps"`
	ScheduledPrematureUse  int `json:"scheduled_premature_use"`
	ExpiredMisuse          int `json:"expired_misuse"`
	ArchivedMisuse         int `json:"archived_misuse"`
	DeletionResidue        int `json:"deletion_residue"`
	GlobalDefaultPollution int `json:"global_default_pollution"`
	ScopeLeakage           int `json:"scope_leakage"`
}

type MemoryEligibilityOperationEvidence struct {
	ReplayCount           int                                 `json:"replay_count"`
	ConflictRejectedCount int                                 `json:"conflict_rejected_count"`
	RaceCount             int                                 `json:"race_count"`
	ForgetWins            int                                 `json:"forget_wins"`
	FalseReceipts         int                                 `json:"false_receipts"`
	AuditContentResidue   int                                 `json:"audit_content_residue"`
	Receipts              []MemoryEligibilityOperationReceipt `json:"receipts"`
}

type MemoryEligibilityOperationReceipt struct {
	OperationID      string `json:"operation_id"`
	Kind             string `json:"kind"`
	Result           string `json:"result"`
	Replayed         bool   `json:"replayed"`
	ConflictRejected bool   `json:"conflict_rejected"`
	RaceOutcome      string `json:"race_outcome,omitempty"`
}

type MemoryEligibilityRestartRestoreEvidence struct {
	RestartRecovered   bool   `json:"restart_recovered"`
	RestoreEquivalent  bool   `json:"restore_equivalent"`
	ForgottenAbsent    bool   `json:"forgotten_absent"`
	BeforeSHA256       string `json:"before_sha256"`
	AfterRestartSHA256 string `json:"after_restart_sha256"`
	AfterRestoreSHA256 string `json:"after_restore_sha256"`
}

type MemoryEligibilityProjectionEvidence struct {
	LexicalRows       int    `json:"lexical_rows"`
	VectorRows        int    `json:"vector_rows"`
	RebuildEquivalent bool   `json:"rebuild_equivalent"`
	OutageDegraded    bool   `json:"outage_degraded"`
	DegradationReason string `json:"degradation_reason"`
	StaleResults      int    `json:"stale_results"`
	ArchivedResults   int    `json:"archived_results"`
	DeletedResults    int    `json:"deleted_results"`
	CrossScopeResults int    `json:"cross_scope_results"`
	BeforeSHA256      string `json:"before_sha256"`
	RebuildSHA256     string `json:"rebuild_sha256"`
	RestoreSHA256     string `json:"restore_sha256"`
}

type MemoryEligibilityClientEvidence struct {
	Surface        string `json:"surface"`
	Client         string `json:"client"`
	ClientVersion  string `json:"client_version"`
	Model          string `json:"model"`
	Completed      bool   `json:"completed"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	FailureCode    string `json:"failure_code,omitempty"`
}

type MemoryEligibilityProviderEvidence struct {
	Claimed                  bool   `json:"claimed"`
	BaseURL                  string `json:"base_url,omitempty"`
	Model                    string `json:"model,omitempty"`
	Dimensions               int    `json:"dimensions,omitempty"`
	Requests                 int    `json:"requests"`
	DurationMS               int64  `json:"duration_ms"`
	ProjectionResponseSHA256 string `json:"projection_response_sha256,omitempty"`
	QueryResponseSHA256      string `json:"query_response_sha256,omitempty"`
}

type MemoryEligibilityHardGate struct {
	Ordinal int    `json:"ordinal"`
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
}

type MemoryEligibilityFailure struct {
	Sequence int       `json:"sequence"`
	At       time.Time `json:"at"`
	Phase    string    `json:"phase"`
	Attempt  int       `json:"attempt"`
	Code     string    `json:"code"`
	Message  string    `json:"message"`
	Retried  bool      `json:"retried"`
}

type MemoryEligibilityArtifactPaths struct {
	JSON     string
	Markdown string
}

func ReadMemoryEligibilityReport(path string) (MemoryEligibilityReport, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return MemoryEligibilityReport{}, fmt.Errorf("read memory eligibility report: %w", err)
	}
	var report MemoryEligibilityReport
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return MemoryEligibilityReport{}, fmt.Errorf("decode memory eligibility report: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return MemoryEligibilityReport{}, fmt.Errorf("decode memory eligibility trailing data: %v", err)
	}
	if err := ValidateMemoryEligibilityReport(report); err != nil {
		return MemoryEligibilityReport{}, err
	}
	return report, nil
}

func WriteMemoryEligibilityReport(root string, report MemoryEligibilityReport) (MemoryEligibilityArtifactPaths, bool, error) {
	if err := ValidateMemoryEligibilityReport(report); err != nil {
		return MemoryEligibilityArtifactPaths{}, false, err
	}
	dir := filepath.Join(root, report.RunID)
	paths := MemoryEligibilityArtifactPaths{
		JSON: filepath.Join(dir, "report.json"), Markdown: filepath.Join(dir, "report.md"),
	}
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("encode memory eligibility report: %w", err)
	}
	jsonData = append(jsonData, '\n')
	markdown := []byte(renderMemoryEligibilityMarkdown(report))
	if marker := memoryEligibilityUnsafeMarker(string(markdown)); marker != "" {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("memory eligibility Markdown contains unsafe value %q", marker)
	}
	if existing, err := os.ReadFile(paths.JSON); err == nil {
		existingMarkdown, markdownErr := os.ReadFile(paths.Markdown)
		if string(existing) == string(jsonData) && markdownErr == nil && string(existingMarkdown) == string(markdown) {
			return paths, true, nil
		}
		return MemoryEligibilityArtifactPaths{}, false, errors.New("conflicting report replay for run_id " + report.RunID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("read existing memory eligibility report: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("create memory eligibility report directory: %w", err)
	}
	jsonTemp, err := writeMemoryEligibilityTemp(dir, "report-json-*", jsonData)
	if err != nil {
		return MemoryEligibilityArtifactPaths{}, false, err
	}
	defer os.Remove(jsonTemp)
	markdownTemp, err := writeMemoryEligibilityTemp(dir, "report-markdown-*", markdown)
	if err != nil {
		return MemoryEligibilityArtifactPaths{}, false, err
	}
	defer os.Remove(markdownTemp)
	if err := os.Rename(jsonTemp, paths.JSON); err != nil {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("commit memory eligibility JSON: %w", err)
	}
	if err := os.Rename(markdownTemp, paths.Markdown); err != nil {
		return MemoryEligibilityArtifactPaths{}, false, fmt.Errorf("commit memory eligibility Markdown: %w", err)
	}
	return paths, false, nil
}

func ValidateMemoryEligibilityReport(report MemoryEligibilityReport) error {
	if report.Version != memoryEligibilityReportVersion || report.ProfileID != "memory-eligibility-retention-v1" {
		return errors.New("memory eligibility report identity is invalid")
	}
	if !memoryEligibilitySafeName.MatchString(report.RunID) || (report.Mode != "mini" && report.Mode != "formal") {
		return errors.New("memory eligibility run identity is invalid")
	}
	if !memoryEligibilityLowerHex(report.CaseSHA256, 64) || !memoryEligibilityLowerHex(report.ImplementationRevision, 40) ||
		report.SchemaVersion != 18 {
		return errors.New("memory eligibility source identity is invalid")
	}
	if !strings.HasPrefix(report.PostgreSQLVersion, "18.") || strings.TrimSpace(report.PGVectorVersion) == "" {
		return errors.New("memory eligibility database versions are invalid")
	}
	if report.StartedAt.IsZero() || report.CompletedAt.Before(report.StartedAt) {
		return errors.New("memory eligibility timestamps are invalid")
	}
	if report.RequestFingerprint != memoryEligibilityRequestFingerprint(report) {
		return errors.New("memory eligibility request fingerprint is invalid")
	}
	if err := validateMemoryEligibilityCorpus(report.Mode, report.Corpus); err != nil {
		return err
	}
	if err := validateMemoryEligibilityQueries(report.Mode, report.Queries); err != nil {
		return err
	}
	if err := validateMemoryEligibilityBaselines(report.Baselines); err != nil {
		return err
	}
	if err := validateMemoryEligibilityMetrics(report.Metrics); err != nil {
		return err
	}
	if err := validateMemoryEligibilityOperations(report.Operations); err != nil {
		return err
	}
	if err := validateMemoryEligibilityRecovery(report.RestartRestore); err != nil {
		return err
	}
	if err := validateMemoryEligibilityProjection(report.Corpus, report.Projection); err != nil {
		return err
	}
	if err := validateMemoryEligibilityClients(report.RealClients); err != nil {
		return err
	}
	if err := validateMemoryEligibilityProvider(report.Mode, report.Provider); err != nil {
		return err
	}
	if len(report.HardGates) != len(memoryEligibilityHardGateNames) {
		return fmt.Errorf("memory eligibility hard gate count=%d want %d", len(report.HardGates), len(memoryEligibilityHardGateNames))
	}
	for index, expected := range memoryEligibilityHardGateNames {
		gate := report.HardGates[index]
		if gate.Ordinal != index+1 || gate.Name != expected || !gate.Passed {
			return fmt.Errorf("memory eligibility hard gate %d is invalid", index+1)
		}
	}
	if len(report.Failures) == 0 {
		return errors.New("memory eligibility failure ledger is missing")
	}
	lastFailureAt := time.Time{}
	for index, failure := range report.Failures {
		if failure.Sequence != index+1 || failure.At.IsZero() || failure.At.Before(lastFailureAt) ||
			strings.TrimSpace(failure.Phase) == "" || failure.Attempt <= 0 ||
			strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.Message) == "" {
			return errors.New("memory eligibility failure ledger is invalid")
		}
		lastFailureAt = failure.At
	}
	if len(report.NonClaims) < 4 {
		return errors.New("memory eligibility non-claims are incomplete")
	}
	seenNonClaims := map[string]struct{}{}
	for _, nonClaim := range report.NonClaims {
		if strings.TrimSpace(nonClaim) == "" {
			return errors.New("memory eligibility non-claim is empty")
		}
		if _, exists := seenNonClaims[nonClaim]; exists {
			return errors.New("memory eligibility non-claims contain duplicates")
		}
		seenNonClaims[nonClaim] = struct{}{}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode memory eligibility report for safety scan: %w", err)
	}
	if marker := memoryEligibilityUnsafeMarker(string(encoded)); marker != "" {
		return fmt.Errorf("memory eligibility report contains unsafe value %q", marker)
	}
	return nil
}

func validateMemoryEligibilityCorpus(mode string, counts MemoryEligibilityCorpusCounts) error {
	values := []int{
		counts.Tenants, counts.Continuities, counts.Total, counts.CurrentOpenEnded, counts.Scheduled,
		counts.Expired, counts.Archived, counts.Superseded, counts.Deleted, counts.ActiveLifecycle,
		counts.ArchivedLifecycle, counts.SupersededLifecycle, counts.DeletedLifecycle,
	}
	for _, value := range values {
		if value < 0 {
			return errors.New("memory eligibility corpus contains negative counts")
		}
	}
	effectiveTotal := counts.CurrentOpenEnded + counts.Scheduled + counts.Expired + counts.Archived + counts.Superseded + counts.Deleted
	lifecycleTotal := counts.ActiveLifecycle + counts.ArchivedLifecycle + counts.SupersededLifecycle + counts.DeletedLifecycle
	if counts.Tenants == 0 || counts.Continuities < counts.Tenants || counts.Total == 0 ||
		effectiveTotal != counts.Total || lifecycleTotal != counts.Total ||
		counts.ActiveLifecycle != counts.CurrentOpenEnded+counts.Scheduled+counts.Expired ||
		counts.ArchivedLifecycle != counts.Archived || counts.SupersededLifecycle != counts.Superseded ||
		counts.DeletedLifecycle != counts.Deleted {
		return errors.New("memory eligibility corpus arithmetic is invalid")
	}
	if mode == "formal" && (counts.Tenants != 4 || counts.Continuities != 20 || counts.Total != 10000 ||
		counts.CurrentOpenEnded != 4000 || counts.Scheduled != 1500 || counts.Expired != 1500 ||
		counts.Archived != 1000 || counts.Superseded != 1000 || counts.Deleted != 1000) {
		return errors.New("memory eligibility formal corpus does not match the frozen profile")
	}
	if mode == "mini" && counts.Total >= 10000 {
		return errors.New("memory eligibility mini corpus is not miniature")
	}
	return nil
}

func validateMemoryEligibilityQueries(mode string, queries MemoryEligibilityQueryMetrics) error {
	if queries.Clients <= 0 || queries.PerClient <= 0 || queries.Total != queries.Clients*queries.PerClient ||
		queries.Successful != queries.Total || queries.P50MS < 0 || queries.P50MS > queries.P95MS ||
		queries.P95MS > queries.P99MS {
		return errors.New("memory eligibility query evidence is invalid")
	}
	if mode == "formal" && queries.Total != 320 {
		return errors.New("memory eligibility formal query count must be 320")
	}
	return nil
}

func validateMemoryEligibilityBaselines(baselines []MemoryEligibilityBaselineOutcome) error {
	expected := []string{"no_context", "full_history", "lifecycle_only", "vermory_eligibility"}
	if len(baselines) != len(expected) {
		return errors.New("memory eligibility baseline set is incomplete")
	}
	for index, baseline := range baselines {
		if baseline.Condition != expected[index] || baseline.TaskCount <= 0 || baseline.TaskSuccess < 0 ||
			baseline.TaskSuccess > baseline.TaskCount || baseline.CurrentFactHits < 0 ||
			baseline.ScheduledPrematureUse < 0 || baseline.ExpiredMisuse < 0 || baseline.ArchivedMisuse < 0 ||
			baseline.DeletionResidue < 0 || baseline.GlobalDefaultPollution < 0 || baseline.ScopeLeakage < 0 ||
			baseline.ContextTokens < 0 || baseline.DeliveredMemories < 0 {
			return errors.New("memory eligibility baseline outcome is invalid")
		}
	}
	qualified := baselines[len(baselines)-1]
	if qualified.TaskSuccess != qualified.TaskCount || qualified.ScheduledPrematureUse != 0 ||
		qualified.ExpiredMisuse != 0 || qualified.ArchivedMisuse != 0 || qualified.DeletionResidue != 0 ||
		qualified.GlobalDefaultPollution != 0 || qualified.ScopeLeakage != 0 {
		return errors.New("memory eligibility qualified baseline contains a hard-gate failure")
	}
	return nil
}

func validateMemoryEligibilityMetrics(metrics MemoryEligibilityOutcomeMetrics) error {
	if metrics.CurrentExpected <= 0 || metrics.CurrentReturned < 0 || metrics.CurrentReturned > metrics.CurrentExpected ||
		metrics.CurrentRecallBPS != metrics.CurrentReturned*10000/metrics.CurrentExpected ||
		metrics.ScheduledPrematureUse != 0 || metrics.ExpiredMisuse != 0 || metrics.ArchivedMisuse != 0 ||
		metrics.DeletionResidue != 0 || metrics.GlobalDefaultPollution != 0 || metrics.ScopeLeakage != 0 {
		return errors.New("memory eligibility outcome metrics are invalid")
	}
	return nil
}

func validateMemoryEligibilityOperations(operations MemoryEligibilityOperationEvidence) error {
	if operations.ReplayCount < 0 || operations.ConflictRejectedCount < 0 || operations.RaceCount <= 0 ||
		operations.ForgetWins != operations.RaceCount || operations.FalseReceipts != 0 || operations.AuditContentResidue != 0 ||
		len(operations.Receipts) == 0 {
		return errors.New("memory eligibility operation summary is invalid")
	}
	seen := map[string]struct{}{}
	replays := 0
	conflicts := 0
	for _, receipt := range operations.Receipts {
		if strings.TrimSpace(receipt.OperationID) == "" || strings.TrimSpace(receipt.Kind) == "" || strings.TrimSpace(receipt.Result) == "" {
			return errors.New("memory eligibility operation receipt is incomplete")
		}
		if _, exists := seen[receipt.OperationID]; exists {
			return errors.New("memory eligibility operation receipts contain duplicates")
		}
		seen[receipt.OperationID] = struct{}{}
		if receipt.Replayed {
			replays++
		}
		if receipt.ConflictRejected {
			conflicts++
		}
		if receipt.RaceOutcome != "" && receipt.RaceOutcome != "forget_won" {
			return errors.New("memory eligibility race outcome is invalid")
		}
	}
	if operations.ReplayCount != replays || operations.ConflictRejectedCount != conflicts {
		return errors.New("memory eligibility operation receipt arithmetic is invalid")
	}
	return nil
}

func validateMemoryEligibilityRecovery(recovery MemoryEligibilityRestartRestoreEvidence) error {
	if !recovery.RestartRecovered || !recovery.RestoreEquivalent || !recovery.ForgottenAbsent ||
		!memoryEligibilityLowerHex(recovery.BeforeSHA256, 64) ||
		recovery.AfterRestartSHA256 != recovery.BeforeSHA256 || recovery.AfterRestoreSHA256 != recovery.BeforeSHA256 {
		return errors.New("memory eligibility restart and restore evidence is invalid")
	}
	return nil
}

func validateMemoryEligibilityProjection(corpus MemoryEligibilityCorpusCounts, projection MemoryEligibilityProjectionEvidence) error {
	if projection.LexicalRows < corpus.CurrentOpenEnded || projection.LexicalRows > corpus.ActiveLifecycle ||
		projection.VectorRows < corpus.CurrentOpenEnded || projection.VectorRows > corpus.ActiveLifecycle ||
		!projection.RebuildEquivalent || !projection.OutageDegraded || projection.DegradationReason != "provider_unavailable" ||
		projection.StaleResults != 0 || projection.ArchivedResults != 0 || projection.DeletedResults != 0 || projection.CrossScopeResults != 0 ||
		!memoryEligibilityLowerHex(projection.BeforeSHA256, 64) || projection.RebuildSHA256 != projection.BeforeSHA256 ||
		projection.RestoreSHA256 != projection.BeforeSHA256 {
		return errors.New("memory eligibility projection evidence is invalid")
	}
	return nil
}

func validateMemoryEligibilityClients(clients []MemoryEligibilityClientEvidence) error {
	if len(clients) < 2 {
		return errors.New("memory eligibility real-client evidence is incomplete")
	}
	seen := map[string]struct{}{}
	webChatGrokCompleted := false
	mcpGrokCompleted := false
	mcpCodexCompleted := false
	for _, client := range clients {
		key := client.Surface + "\x00" + client.Client
		if strings.TrimSpace(client.Surface) == "" || strings.TrimSpace(client.Client) == "" ||
			strings.TrimSpace(client.ClientVersion) == "" || strings.TrimSpace(client.Model) == "" ||
			!memoryEligibilityLowerHex(client.EvidenceSHA256, 64) || !memoryEligibilityLowerHex(client.ArtifactSHA256, 64) {
			return errors.New("memory eligibility real-client hash evidence is missing")
		}
		if _, exists := seen[key]; exists {
			return errors.New("memory eligibility real-client evidence contains duplicates")
		}
		seen[key] = struct{}{}
		if client.Completed && client.Surface == "web_chat" && client.Client == "grok-cli" {
			webChatGrokCompleted = true
		}
		if client.Completed && client.Surface == "mcp_workspace" && client.Client == "grok-cli" {
			mcpGrokCompleted = true
		}
		if client.Completed && client.Surface == "mcp_workspace" && client.Client == "codex-cli" {
			mcpCodexCompleted = true
		}
		if client.Completed && client.FailureCode != "" {
			return errors.New("memory eligibility completed client has a failure code")
		}
	}
	if !webChatGrokCompleted || !mcpGrokCompleted || !mcpCodexCompleted {
		return errors.New("memory eligibility real-client surfaces are incomplete")
	}
	return nil
}

func validateMemoryEligibilityProvider(mode string, provider MemoryEligibilityProviderEvidence) error {
	if mode == "mini" {
		if provider.Claimed || provider.BaseURL != "" || provider.Model != "" || provider.Dimensions != 0 ||
			provider.Requests != 0 || provider.DurationMS != 0 || provider.ProjectionResponseSHA256 != "" || provider.QueryResponseSHA256 != "" {
			return errors.New("memory eligibility mini report made a direct-provider claim")
		}
		return nil
	}
	if !provider.Claimed || provider.BaseURL != "https://api.siliconflow.cn/v1" || provider.Model != "BAAI/bge-m3" ||
		provider.Dimensions != 1024 || provider.Requests != 2 || provider.DurationMS < 0 ||
		!memoryEligibilityLowerHex(provider.ProjectionResponseSHA256, 64) || !memoryEligibilityLowerHex(provider.QueryResponseSHA256, 64) {
		return errors.New("memory eligibility direct-provider evidence is invalid")
	}
	return nil
}

func memoryEligibilityRequestFingerprint(report MemoryEligibilityReport) string {
	request := struct {
		Version                int                               `json:"version"`
		RunID                  string                            `json:"run_id"`
		ProfileID              string                            `json:"profile_id"`
		Mode                   string                            `json:"mode"`
		CaseSHA256             string                            `json:"case_sha256"`
		SchemaVersion          int                               `json:"schema_version"`
		ImplementationRevision string                            `json:"implementation_revision"`
		PostgreSQLVersion      string                            `json:"postgresql_version"`
		Corpus                 MemoryEligibilityCorpusCounts     `json:"corpus"`
		Provider               MemoryEligibilityProviderEvidence `json:"provider"`
	}{
		Version: report.Version, RunID: report.RunID, ProfileID: report.ProfileID, Mode: report.Mode,
		CaseSHA256: report.CaseSHA256, SchemaVersion: report.SchemaVersion,
		ImplementationRevision: report.ImplementationRevision, PostgreSQLVersion: report.PostgreSQLVersion,
		Corpus: report.Corpus, Provider: report.Provider,
	}
	payload, _ := json.Marshal(request)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func renderMemoryEligibilityMarkdown(report MemoryEligibilityReport) string {
	var output strings.Builder
	fmt.Fprintln(&output, "# Memory Eligibility And Retention Qualification")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Run / profile / mode: `%s / %s / %s`\n", report.RunID, report.ProfileID, report.Mode)
	fmt.Fprintf(&output, "- Revision / schema: `%s / %d`\n", report.ImplementationRevision, report.SchemaVersion)
	fmt.Fprintf(&output, "- PostgreSQL / pgvector: `%s / %s`\n", report.PostgreSQLVersion, report.PGVectorVersion)
	fmt.Fprintf(&output, "- Corpus: `%d` facts across `%d` tenants and `%d` continuities\n", report.Corpus.Total, report.Corpus.Tenants, report.Corpus.Continuities)
	fmt.Fprintf(&output, "- Queries: `%d`; p50/p95/p99=`%d/%d/%d ms`\n", report.Queries.Total, report.Queries.P50MS, report.Queries.P95MS, report.Queries.P99MS)
	fmt.Fprintln(&output, "- Hard gates: `16 / 16` PASS")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Corpus")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Effective current/scheduled/expired/archived/superseded/deleted: `%d/%d/%d/%d/%d/%d`\n",
		report.Corpus.CurrentOpenEnded, report.Corpus.Scheduled, report.Corpus.Expired,
		report.Corpus.Archived, report.Corpus.Superseded, report.Corpus.Deleted)
	fmt.Fprintf(&output, "- Lifecycle active/archived/superseded/deleted: `%d/%d/%d/%d`\n",
		report.Corpus.ActiveLifecycle, report.Corpus.ArchivedLifecycle,
		report.Corpus.SupersededLifecycle, report.Corpus.DeletedLifecycle)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Baselines")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Condition | Tasks | Success | Current hits | Scheduled | Expired | Archived | Deleted | Default | Scope | Tokens | Memories |")
	fmt.Fprintln(&output, "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, baseline := range report.Baselines {
		fmt.Fprintf(&output, "| `%s` | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			baseline.Condition, baseline.TaskCount, baseline.TaskSuccess, baseline.CurrentFactHits,
			baseline.ScheduledPrematureUse, baseline.ExpiredMisuse, baseline.ArchivedMisuse,
			baseline.DeletionResidue, baseline.GlobalDefaultPollution, baseline.ScopeLeakage,
			baseline.ContextTokens, baseline.DeliveredMemories)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Operations And Recovery")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Replay/conflict/race/forget-wins: `%d/%d/%d/%d`\n",
		report.Operations.ReplayCount, report.Operations.ConflictRejectedCount,
		report.Operations.RaceCount, report.Operations.ForgetWins)
	fmt.Fprintf(&output, "- Restart/restore/forgotten-absent: `%t/%t/%t`\n",
		report.RestartRestore.RestartRecovered, report.RestartRestore.RestoreEquivalent,
		report.RestartRestore.ForgottenAbsent)
	fmt.Fprintf(&output, "- Projection lexical/vector/rebuild/outage: `%d/%d/%t/%s`\n",
		report.Projection.LexicalRows, report.Projection.VectorRows,
		report.Projection.RebuildEquivalent, report.Projection.DegradationReason)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Real Clients")
	fmt.Fprintln(&output)
	for _, client := range report.RealClients {
		fmt.Fprintf(&output, "- `%s` via `%s %s` / `%s`: completed=`%t`, evidence=`%s`, artifact=`%s`\n",
			client.Surface, client.Client, client.ClientVersion, client.Model, client.Completed,
			client.EvidenceSHA256, client.ArtifactSHA256)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Direct Provider")
	fmt.Fprintln(&output)
	if report.Provider.Claimed {
		fmt.Fprintf(&output, "- `%s` / `%s` / `%d` dimensions / `%d` requests\n",
			report.Provider.BaseURL, report.Provider.Model, report.Provider.Dimensions, report.Provider.Requests)
	} else {
		fmt.Fprintln(&output, "- No direct-provider claim in this miniature profile.")
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Failure Ledger")
	fmt.Fprintln(&output)
	for _, failure := range report.Failures {
		fmt.Fprintf(&output, "- `%d` `%s` attempt %d: `%s` - %s (retried=%t)\n",
			failure.Sequence, failure.Phase, failure.Attempt, failure.Code, failure.Message, failure.Retried)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Hard Gates")
	fmt.Fprintln(&output)
	for _, gate := range report.HardGates {
		fmt.Fprintf(&output, "- `%02d` %s: PASS\n", gate.Ordinal, gate.Name)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Non-Claims")
	fmt.Fprintln(&output)
	for _, nonClaim := range report.NonClaims {
		fmt.Fprintf(&output, "- %s\n", nonClaim)
	}
	return output.String()
}

func writeMemoryEligibilityTemp(dir, pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create memory eligibility temporary artifact: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", fmt.Errorf("protect memory eligibility temporary artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", fmt.Errorf("write memory eligibility temporary artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync memory eligibility temporary artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close memory eligibility temporary artifact: %w", err)
	}
	return path, nil
}

func memoryEligibilityUnsafeMarker(value string) string {
	lower := strings.ToLower(strings.ReplaceAll(value, `\"`, `"`))
	for _, marker := range []string{
		"postgresql://", "password=", "sk-", "authorization:", "bearer ",
		`"choices":[`, `"embedding":[`, `"raw_provider_body"`,
	} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	if memoryEligibilityVector.MatchString(lower) {
		return "numeric-vector"
	}
	return ""
}

func memoryEligibilityLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
