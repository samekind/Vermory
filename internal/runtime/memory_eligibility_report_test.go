package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMemoryEligibilityReportWritesDeterministicArtifactsAndReplays(t *testing.T) {
	report := validMemoryEligibilityReport("formal")
	root := t.TempDir()
	paths, replayed, err := WriteMemoryEligibilityReport(root, report)
	if err != nil || replayed {
		t.Fatalf("write memory eligibility report: replayed=%v err=%v", replayed, err)
	}
	jsonFirst, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	markdownFirst, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(jsonFirst), "\n") ||
		!strings.Contains(string(markdownFirst), "16 / 16") ||
		!strings.Contains(string(markdownFirst), "vermory_eligibility") ||
		!strings.Contains(string(markdownFirst), "codex-cli 0.144.3") ||
		!strings.Contains(string(markdownFirst), "provider_unavailable") {
		t.Fatalf("unexpected memory eligibility artifacts:\nJSON=%s\nMarkdown=%s", jsonFirst, markdownFirst)
	}

	pathsReplay, replayed, err := WriteMemoryEligibilityReport(root, report)
	if err != nil || !replayed || pathsReplay != paths {
		t.Fatalf("identical report did not replay: paths=%#v replayed=%v err=%v", pathsReplay, replayed, err)
	}
	jsonReplay, _ := os.ReadFile(pathsReplay.JSON)
	markdownReplay, _ := os.ReadFile(pathsReplay.Markdown)
	if !reflect.DeepEqual(jsonReplay, jsonFirst) || !reflect.DeepEqual(markdownReplay, markdownFirst) {
		t.Fatal("identical report replay changed artifact bytes")
	}

	read, err := ReadMemoryEligibilityReport(paths.JSON)
	if err != nil || !reflect.DeepEqual(read, report) {
		t.Fatalf("read memory eligibility report mismatch: report=%#v err=%v", read, err)
	}
}

func TestMemoryEligibilityReportRejectsConflictFailedGateAndUnsafeEvidence(t *testing.T) {
	root := t.TempDir()
	report := validMemoryEligibilityReport("formal")
	if _, _, err := WriteMemoryEligibilityReport(root, report); err != nil {
		t.Fatal(err)
	}
	conflict := report
	conflict.Queries.P50MS++
	if _, _, err := WriteMemoryEligibilityReport(root, conflict); err == nil ||
		!strings.Contains(err.Error(), "conflicting report replay") {
		t.Fatalf("conflicting report replay was accepted: %v", err)
	}

	failedGate := validMemoryEligibilityReport("formal")
	failedGate.HardGates[0].Passed = false
	if err := ValidateMemoryEligibilityReport(failedGate); err == nil {
		t.Fatal("report with a failed hard gate was accepted")
	}

	unsafeValues := []string{
		"Bearer sk-secret-shaped-value",
		"postgresql://user:password@host/database",
		`{"choices":[{"message":{"content":"raw provider body"}}]}`,
		"vector=[0.1,0.2,0.3,0.4,0.5,0.6,0.7,0.8,0.9]",
	}
	for index, value := range unsafeValues {
		unsafe := validMemoryEligibilityReport("formal")
		unsafe.Failures = append(unsafe.Failures, MemoryEligibilityFailure{
			Sequence: len(unsafe.Failures) + 1,
			At:       unsafe.CompletedAt.Add(time.Duration(index+1) * time.Second),
			Phase:    "provider",
			Attempt:  index + 2,
			Code:     "provider_error",
			Message:  value,
			Retried:  true,
		})
		if err := ValidateMemoryEligibilityReport(unsafe); err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Fatalf("unsafe report value was accepted: value=%q err=%v", value, err)
		}
	}
}

func TestMemoryEligibilityReportRejectsInvalidArithmeticLatencyReceiptsClientsProviderAndOrder(t *testing.T) {
	for name, mutate := range map[string]func(*MemoryEligibilityReport){
		"corpus arithmetic":    func(report *MemoryEligibilityReport) { report.Corpus.Deleted++ },
		"lifecycle arithmetic": func(report *MemoryEligibilityReport) { report.Corpus.ActiveLifecycle-- },
		"query arithmetic":     func(report *MemoryEligibilityReport) { report.Queries.Total-- },
		"latency":              func(report *MemoryEligibilityReport) { report.Queries.P50MS = report.Queries.P95MS + 1 },
		"metrics":              func(report *MemoryEligibilityReport) { report.Metrics.CurrentRecallBPS-- },
		"duplicate receipt": func(report *MemoryEligibilityReport) {
			report.Operations.Receipts = append(report.Operations.Receipts, report.Operations.Receipts[0])
		},
		"missing client hash": func(report *MemoryEligibilityReport) { report.RealClients[0].ArtifactSHA256 = "" },
		"missing Codex client": func(report *MemoryEligibilityReport) {
			report.RealClients = report.RealClients[:2]
		},
		"provider requests": func(report *MemoryEligibilityReport) { report.Provider.Requests = 1 },
		"hard gate order": func(report *MemoryEligibilityReport) {
			report.HardGates[0], report.HardGates[1] = report.HardGates[1], report.HardGates[0]
		},
		"failure chronology": func(report *MemoryEligibilityReport) {
			report.Failures[1].At = report.Failures[0].At.Add(-time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			report := validMemoryEligibilityReport("formal")
			mutate(&report)
			if err := ValidateMemoryEligibilityReport(report); err == nil {
				t.Fatalf("invalid memory eligibility report was accepted: %#v", report)
			}
		})
	}
}

func TestMemoryEligibilityMiniReportAllowsExplicitNoProviderClaim(t *testing.T) {
	report := validMemoryEligibilityReport("mini")
	if err := ValidateMemoryEligibilityReport(report); err != nil {
		t.Fatal(err)
	}
	if report.Provider.Claimed || report.Provider.Requests != 0 {
		t.Fatalf("mini report made a provider claim: %#v", report.Provider)
	}
}

func TestMemoryEligibilityHardGateOrderMatchesFrozenCase(t *testing.T) {
	manifest := loadMemoryEligibilityCase(t)
	if !reflect.DeepEqual(memoryEligibilityHardGateNames, manifest.HardGates) {
		t.Fatalf("report hard-gate order drifted from frozen case:\nreport=%#v\ncase=%#v", memoryEligibilityHardGateNames, manifest.HardGates)
	}
}

func TestMemoryEligibilityArtifactPathsStayUnderRunDirectory(t *testing.T) {
	report := validMemoryEligibilityReport("mini")
	paths, _, err := WriteMemoryEligibilityReport(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(paths.JSON)) != report.RunID || filepath.Base(filepath.Dir(paths.Markdown)) != report.RunID {
		t.Fatalf("memory eligibility artifacts escaped run directory: %#v", paths)
	}
}

func validMemoryEligibilityReport(mode string) MemoryEligibilityReport {
	started := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	formal := mode == "formal"
	corpus := MemoryEligibilityCorpusCounts{
		Tenants: 2, Continuities: 4, Total: 60,
		CurrentOpenEnded: 20, Scheduled: 10, Expired: 10,
		Archived: 8, Superseded: 6, Deleted: 6,
		ActiveLifecycle: 40, ArchivedLifecycle: 8, SupersededLifecycle: 6, DeletedLifecycle: 6,
	}
	queries := MemoryEligibilityQueryMetrics{
		Clients: 4, PerClient: 8, Total: 32, Successful: 32,
		P50MS: 2, P95MS: 5, P99MS: 8,
	}
	if formal {
		corpus = MemoryEligibilityCorpusCounts{
			Tenants: 4, Continuities: 20, Total: 10000,
			CurrentOpenEnded: 4000, Scheduled: 1500, Expired: 1500,
			Archived: 1000, Superseded: 1000, Deleted: 1000,
			ActiveLifecycle: 7000, ArchivedLifecycle: 1000, SupersededLifecycle: 1000, DeletedLifecycle: 1000,
		}
		queries = MemoryEligibilityQueryMetrics{
			Clients: 16, PerClient: 20, Total: 320, Successful: 320,
			P50MS: 4, P95MS: 11, P99MS: 19,
		}
	}

	report := MemoryEligibilityReport{
		Version: 1, RunID: "memory-eligibility-report-test-" + mode,
		ProfileID: "memory-eligibility-retention-v1", Mode: mode,
		CaseSHA256: strings.Repeat("a", 64), SchemaVersion: 18,
		ImplementationRevision: strings.Repeat("b", 40),
		PostgreSQLVersion:      "18.4", PGVectorVersion: "0.8.5",
		StartedAt: started, CompletedAt: started.Add(time.Minute),
		Corpus:  corpus,
		Queries: queries,
		Baselines: []MemoryEligibilityBaselineOutcome{
			{Condition: "no_context", TaskCount: 4, TaskSuccess: 1, CurrentFactHits: 0, ContextTokens: 0, DeliveredMemories: 0},
			{Condition: "full_history", TaskCount: 4, TaskSuccess: 2, CurrentFactHits: 4, ExpiredMisuse: 2, ArchivedMisuse: 1, DeletionResidue: 1, GlobalDefaultPollution: 1, ContextTokens: 1800, DeliveredMemories: 16},
			{Condition: "lifecycle_only", TaskCount: 4, TaskSuccess: 3, CurrentFactHits: 4, ScheduledPrematureUse: 1, ExpiredMisuse: 1, ContextTokens: 900, DeliveredMemories: 9},
			{Condition: "vermory_eligibility", TaskCount: 4, TaskSuccess: 4, CurrentFactHits: 4, ContextTokens: 420, DeliveredMemories: 4},
		},
		Metrics: MemoryEligibilityOutcomeMetrics{
			CurrentExpected: 4000, CurrentReturned: 4000, CurrentRecallBPS: 10000,
		},
		Operations: MemoryEligibilityOperationEvidence{
			ReplayCount: 3, ConflictRejectedCount: 3, RaceCount: 2, ForgetWins: 2,
			FalseReceipts: 0, AuditContentResidue: 0,
			Receipts: []MemoryEligibilityOperationReceipt{
				{OperationID: "validity-replay", Kind: "set_validity", Result: "updated", Replayed: true, ConflictRejected: true},
				{OperationID: "archive-replay", Kind: "archive", Result: "archived", Replayed: true, ConflictRejected: true},
				{OperationID: "forget-race", Kind: "forget", Result: "deleted", Replayed: true, ConflictRejected: true, RaceOutcome: "forget_won"},
			},
		},
		RestartRestore: MemoryEligibilityRestartRestoreEvidence{
			RestartRecovered: true, RestoreEquivalent: true, ForgottenAbsent: true,
			BeforeSHA256: strings.Repeat("c", 64), AfterRestartSHA256: strings.Repeat("c", 64), AfterRestoreSHA256: strings.Repeat("c", 64),
		},
		Projection: MemoryEligibilityProjectionEvidence{
			LexicalRows: corpus.CurrentOpenEnded, VectorRows: corpus.CurrentOpenEnded,
			RebuildEquivalent: true, OutageDegraded: true, DegradationReason: "provider_unavailable",
			StaleResults: 0, ArchivedResults: 0, DeletedResults: 0, CrossScopeResults: 0,
			BeforeSHA256: strings.Repeat("d", 64), RebuildSHA256: strings.Repeat("d", 64), RestoreSHA256: strings.Repeat("d", 64),
		},
		RealClients: []MemoryEligibilityClientEvidence{
			{Surface: "web_chat", Client: "grok-cli", ClientVersion: "0.2.101", Model: "grok-4.5", Completed: true, EvidenceSHA256: strings.Repeat("e", 64), ArtifactSHA256: strings.Repeat("f", 64)},
			{Surface: "mcp_workspace", Client: "grok-cli", ClientVersion: "0.2.101", Model: "grok-4.5", Completed: true, EvidenceSHA256: strings.Repeat("1", 64), ArtifactSHA256: strings.Repeat("2", 64)},
			{Surface: "mcp_workspace", Client: "codex-cli", ClientVersion: "0.144.3", Model: "gpt-5.5", Completed: true, EvidenceSHA256: strings.Repeat("5", 64), ArtifactSHA256: strings.Repeat("6", 64)},
		},
		Provider:  MemoryEligibilityProviderEvidence{Claimed: formal},
		HardGates: passingMemoryEligibilityHardGates(),
		Failures: []MemoryEligibilityFailure{
			{Sequence: 1, At: started.Add(10 * time.Second), Phase: "provider", Attempt: 1, Code: "duplicate_flag", Message: "Grok rejected a duplicate verbatim flag", Retried: true},
			{Sequence: 2, At: started.Add(20 * time.Second), Phase: "degradation", Attempt: 1, Code: "provider_unavailable", Message: "the deterministic provider outage degraded to eligible lexical serving", Retried: true},
		},
		NonClaims: []string{
			"qualification workload is not a universal capacity claim",
			"no model ranking claim",
			"no automatic promotion of model output",
			"no cross-region high availability claim",
			"no sealed external evaluation claim",
		},
	}
	if formal {
		report.Provider = MemoryEligibilityProviderEvidence{
			Claimed: true, BaseURL: "https://api.siliconflow.cn/v1", Model: "BAAI/bge-m3",
			Dimensions: 1024, Requests: 2, DurationMS: 100,
			ProjectionResponseSHA256: strings.Repeat("3", 64), QueryResponseSHA256: strings.Repeat("4", 64),
		}
	}
	report.RequestFingerprint = memoryEligibilityRequestFingerprint(report)
	return report
}

func passingMemoryEligibilityHardGates() []MemoryEligibilityHardGate {
	gates := make([]MemoryEligibilityHardGate, len(memoryEligibilityHardGateNames))
	for index, name := range memoryEligibilityHardGateNames {
		gates[index] = MemoryEligibilityHardGate{Ordinal: index + 1, Name: name, Passed: true}
	}
	return gates
}
