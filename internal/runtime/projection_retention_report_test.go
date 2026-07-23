package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProjectionRetentionReportWritesDeterministicArtifactsAndReplays(t *testing.T) {
	report := validProjectionRetentionReport()
	root := t.TempDir()
	paths, replayed, err := WriteProjectionRetentionReport(root, report)
	if err != nil || replayed {
		t.Fatalf("write projection retention report: replayed=%v err=%v", replayed, err)
	}
	jsonFirst, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	markdownFirst, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(jsonFirst), "\n") || !strings.Contains(string(markdownFirst), "14 / 14") ||
		!strings.Contains(string(markdownFirst), "projection_rebuild_required") {
		t.Fatalf("unexpected projection retention artifacts:\nJSON=%s\nMarkdown=%s", jsonFirst, markdownFirst)
	}
	pathsReplay, replayed, err := WriteProjectionRetentionReport(root, report)
	if err != nil || !replayed || pathsReplay != paths {
		t.Fatalf("identical report did not replay: paths=%#v replayed=%v err=%v", pathsReplay, replayed, err)
	}
	jsonReplay, _ := os.ReadFile(pathsReplay.JSON)
	markdownReplay, _ := os.ReadFile(pathsReplay.Markdown)
	if !reflect.DeepEqual(jsonReplay, jsonFirst) || !reflect.DeepEqual(markdownReplay, markdownFirst) {
		t.Fatal("identical report replay changed artifact bytes")
	}
	read, err := ReadProjectionRetentionReport(paths.JSON)
	if err != nil || !reflect.DeepEqual(read, report) {
		t.Fatalf("read projection retention report mismatch: report=%#v err=%v", read, err)
	}
}

func TestProjectionRetentionReportRejectsConflictFailedGateAndSecret(t *testing.T) {
	root := t.TempDir()
	report := validProjectionRetentionReport()
	if _, _, err := WriteProjectionRetentionReport(root, report); err != nil {
		t.Fatal(err)
	}
	conflict := report
	conflict.Queries.P50MS++
	if _, _, err := WriteProjectionRetentionReport(root, conflict); err == nil ||
		!strings.Contains(err.Error(), "conflicting report replay") {
		t.Fatalf("conflicting report replay was accepted: %v", err)
	}
	failedGate := validProjectionRetentionReport()
	for name := range failedGate.HardGates {
		failedGate.HardGates[name] = false
		break
	}
	if err := ValidateProjectionRetentionReport(failedGate); err == nil {
		t.Fatal("report with a failed hard gate was accepted")
	}
	secret := validProjectionRetentionReport()
	secret.Failures = append(secret.Failures, ProjectionRetentionFailure{
		Phase: "provider", Attempt: 2, Code: "provider_error",
		Message: "Bearer sk-secret-shaped-value", Retried: true,
	})
	if err := ValidateProjectionRetentionReport(secret); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("secret-shaped report was accepted: %v", err)
	}
}

func TestProjectionRetentionReportRejectsInvalidCountsLatencyAndReceipts(t *testing.T) {
	for name, mutate := range map[string]func(*ProjectionRetentionReport){
		"counts":            func(report *ProjectionRetentionReport) { report.Counts.RetainedEvents++ },
		"latency":           func(report *ProjectionRetentionReport) { report.Queries.P50MS = report.Queries.P95MS + 1 },
		"receipt":           func(report *ProjectionRetentionReport) { report.Receipts = append(report.Receipts, report.Receipts[0]) },
		"case hash":         func(report *ProjectionRetentionReport) { report.CaseSHA256 = "invalid" },
		"provider requests": func(report *ProjectionRetentionReport) { report.Provider.Requests = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			report := validProjectionRetentionReport()
			mutate(&report)
			if err := ValidateProjectionRetentionReport(report); err == nil {
				t.Fatalf("invalid projection retention report was accepted: %#v", report)
			}
		})
	}
}

func validProjectionRetentionReport() ProjectionRetentionReport {
	started := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	report := ProjectionRetentionReport{
		Version: 1, RunID: "projection-retention-report-test",
		CaseSHA256: strings.Repeat("a", 64), ImplementationRevision: strings.Repeat("b", 40),
		PostgreSQLVersion: "18.4", PGVectorVersion: "0.8.5",
		StartedAt: started, CompletedAt: started.Add(time.Minute),
		Environment: ProjectionRetentionEnvironment{OS: "Darwin arm64", CPU: "test", MemoryGiB: 48, SchemaVersion: 17},
		Policy:      ProjectionRetentionPolicy{Cutoff: started.Add(-time.Hour), RetainTailEvents: 1000},
		Counts: ProjectionRetentionCounts{
			InitialCurrent: 20000, GeneratedEvents: 173600, PrunedEvents: 169600,
			RetainedEvents: 4000, FinalCurrent: 20000, LexicalRows: 20000,
			IncumbentVectors: 20000, DimensionalVectors: 20000,
		},
		Epochs: []ProjectionRetentionEpoch{
			{Epoch: 1, TenantID: "tenant-00", Generated: 6400, Pruned: 100, Retained: 6300, Floor: 100, SlowCursor: 100},
		},
		Profiles: []ProjectionRetentionProfile{
			{TenantID: "tenant-00", ProfileID: ProductionRetrievalProfileID, Status: "idle", LastEventID: 200, Floor: 100, Lag: 0, VectorCount: 5000},
			{TenantID: "tenant-00", ProfileID: MigrationRetrievalProfileID, Status: ProjectionStatusRebuildRequired, LastEventID: 100, Floor: 100, Lag: 0, VectorCount: 0},
		},
		Receipts: []ProjectionPruneReceipt{
			{ID: "11111111-1111-1111-1111-111111111111", TenantID: "tenant-00", OperationID: "prune-1",
				RequestFingerprint: strings.Repeat("c", 64), Cutoff: started.Add(-time.Hour), RetainTailEvents: 1000,
				SafeCursorEventID: 100, PreviousFloorEventID: 0, NewFloorEventID: 100,
				DeletedEvents: 100, Result: "pruned", CreatedAt: started.Add(time.Second)},
		},
		Restart: ProjectionRetentionRestart{
			FailureCode: "postgres_immediate_stop", DeletedEventsRolledBack: true,
			FloorRolledBack: true, ReceiptRolledBack: true, SamePoolRecovered: true, RecoveryDurationMS: 120,
		},
		Queries: ProjectionRetentionQueries{Successful: 320, CrossScopeResults: 0, LexicalDegradations: 2, P50MS: 10, P95MS: 20, P99MS: 30},
		Rebuild: ProjectionRetentionRebuild{
			FutureSubscriberZeroCalls: true, ResetRequiredRebuild: true,
			AuthorityIDHashEquivalent: true, DeletedMemoryAbsent: true,
		},
		Provider: ProjectionRetentionProvider{
			BaseURL: "https://api.siliconflow.cn/v1", Model: "BAAI/bge-m3", Dimensions: 1024,
			Requests: 2, DurationMS: 100, ProjectionResponseSHA256: strings.Repeat("d", 64),
			QueryResponseSHA256: strings.Repeat("e", 64),
		},
		HardGates: projectionRetentionPassingGates(),
		Failures: []ProjectionRetentionFailure{
			{Phase: "restart", Attempt: 1, Code: "postgres_immediate_stop", Message: "expected injected restart", Retried: true},
		},
		NonClaims: []string{
			"not months of uninterrupted wall-clock operation",
			"no automatic retention scheduling",
			"no authoritative-memory deletion policy",
			"no cross-host high availability claim",
			"no external sealed evaluation claim",
			"no artifact signing or final release acceptance claim",
		},
	}
	report.RequestFingerprint = projectionRetentionRequestFingerprint(report)
	return report
}

func projectionRetentionPassingGates() map[string]bool {
	return map[string]bool{
		"schema 17 creates tenant-isolated retention and prune audit state":          true,
		"runtime workers can read but cannot mutate retention control tables":        true,
		"no prune passes the slowest incremental cursor":                             true,
		"authority writes and active-profile queries continue during prune attempts": true,
		"interrupted prune deletion floor and audit changes roll back together":      true,
		"the same pool recovers after PostgreSQL restart":                            true,
		"event retention reaches the calibrated bound after catch-up":                true,
		"retention floor is monotonic and idempotent replay is byte-stable":          true,
		"new subscribers below the floor rebuild with zero embedding work":           true,
		"reset after pruning requires rebuild and leaves other profiles unchanged":   true,
		"rebuilds match authority and deleted memories remain absent":                true,
		"cross-tenant pruning and receipt access are blocked":                        true,
		"direct provider projection and query succeed after pruning":                 true,
		"lexical and the incumbent remain default with no promotion":                 true,
	}
}

func TestProjectionRetentionArtifactPathsStayUnderRunDirectory(t *testing.T) {
	report := validProjectionRetentionReport()
	paths, _, err := WriteProjectionRetentionReport(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(paths.JSON)) != report.RunID || filepath.Base(filepath.Dir(paths.Markdown)) != report.RunID {
		t.Fatalf("projection retention artifacts escaped run directory: %#v", paths)
	}
}
