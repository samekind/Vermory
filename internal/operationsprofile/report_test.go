package operationsprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReportCreatesDeterministicArtifactsAndReplays(t *testing.T) {
	report := validReportFixture()
	report.RequestFingerprint = reportRequestFingerprint(report)

	paths, replayed, err := WriteReport(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first report write was marked replayed")
	}
	jsonBefore, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	markdownBefore, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"# PostgreSQL HA And PITR Qualification",
		report.RunID,
		report.ImplementationRevision,
		report.PITR.TargetLSN,
		"historical-state quarantine",
		"transition_database_unavailable",
		"Hard gates: PASS",
	} {
		if !strings.Contains(string(markdownBefore), required) {
			t.Fatalf("markdown missing %q:\n%s", required, markdownBefore)
		}
	}

	replayedPaths, replayed, err := WriteReport(filepath.Dir(filepath.Dir(paths.JSON)), report)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || replayedPaths != paths {
		t.Fatalf("unexpected replay result: replayed=%t paths=%#v want=%#v", replayed, replayedPaths, paths)
	}
	jsonAfter, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	markdownAfter, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonAfter) != string(jsonBefore) || string(markdownAfter) != string(markdownBefore) {
		t.Fatal("matching replay changed report bytes")
	}
}

func TestWriteReportRejectsConflictingRunIDReuse(t *testing.T) {
	root := t.TempDir()
	report := validReportFixture()
	report.RequestFingerprint = reportRequestFingerprint(report)
	if _, _, err := WriteReport(root, report); err != nil {
		t.Fatal(err)
	}

	conflict := report
	conflict.Failover.PostPromotionRows = 2
	conflict.RequestFingerprint = reportRequestFingerprint(conflict)
	if conflict.RequestFingerprint != report.RequestFingerprint {
		t.Fatal("measured output changed the immutable request fingerprint")
	}
	if _, _, err := WriteReport(root, conflict); err == nil || !strings.Contains(err.Error(), "conflicting report replay") {
		t.Fatalf("expected conflicting replay rejection, got %v", err)
	}
}

func TestWriteCheckpointIsAtomicAndRejectsFingerprintDrift(t *testing.T) {
	root := t.TempDir()
	checkpoint := Checkpoint{
		Version:            1,
		RunID:              "postgresql-ha-pitr-test",
		RequestFingerprint: strings.Repeat("a", 64),
		Phase:              "replication_caught_up",
		Status:             "completed",
		Measurements:       map[string]string{"standby_replay_lsn": "0/30001A0"},
		Failures: []FailureRecord{{
			Phase: "standby_wait", Attempt: 1, Code: "replay_lag", Message: "standby had not reached the target yet", Retried: true,
		}},
	}
	if err := WriteCheckpoint(root, checkpoint); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "checkpoints", "replication_caught_up.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCheckpoint(root, checkpoint); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("checkpoint replay changed bytes")
	}

	drift := checkpoint
	drift.RequestFingerprint = strings.Repeat("b", 64)
	if err := WriteCheckpoint(root, drift); err == nil || !strings.Contains(err.Error(), "conflicting checkpoint replay") {
		t.Fatalf("expected checkpoint fingerprint drift rejection, got %v", err)
	}
	changed := checkpoint
	changed.Measurements = map[string]string{"standby_replay_lsn": "0/40001A0"}
	if err := WriteCheckpoint(root, changed); err == nil || !strings.Contains(err.Error(), "conflicting checkpoint replay") {
		t.Fatalf("expected checkpoint payload drift rejection, got %v", err)
	}
}

func TestInventoryDigestSortsPathsAndHashesBytes(t *testing.T) {
	first := []ArchiveEntry{
		{Path: "000000010000000000000002", Size: 20, SHA256: strings.Repeat("b", 64)},
		{Path: "000000010000000000000001", Size: 10, SHA256: strings.Repeat("a", 64)},
	}
	second := []ArchiveEntry{first[1], first[0]}
	digest := InventoryDigest(first)
	if len(digest) != 64 || digest != InventoryDigest(second) {
		t.Fatalf("inventory digest is not stable: %q vs %q", digest, InventoryDigest(second))
	}
	second[0].Size++
	if digest == InventoryDigest(second) {
		t.Fatal("inventory digest ignored entry size change")
	}
}

func TestValidateReportRejectsSecretShapedFieldsAndFailedHardGate(t *testing.T) {
	report := validReportFixture()
	report.RequestFingerprint = reportRequestFingerprint(report)
	if err := ValidateReport(report); err != nil {
		t.Fatal(err)
	}

	secret := report
	secret.NonClaims = append([]string(nil), report.NonClaims...)
	secret.NonClaims = append(secret.NonClaims, "postgresql://operator@example.invalid/database")
	secret.RequestFingerprint = reportRequestFingerprint(secret)
	if err := ValidateReport(secret); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("expected secret-shaped report rejection, got %v", err)
	}

	failed := report
	failed.HardGates = cloneHardGates(report.HardGates)
	failed.HardGates["same_pool_recovered"] = false
	failed.RequestFingerprint = reportRequestFingerprint(failed)
	if err := ValidateReport(failed); err == nil || !strings.Contains(err.Error(), "hard gate") {
		t.Fatalf("expected failed hard-gate rejection, got %v", err)
	}
}

func TestValidateReportRequiresUnchangedRuntimeObjects(t *testing.T) {
	report := validReportFixture()
	report.Failover.SameAuthPool = false
	report.RequestFingerprint = reportRequestFingerprint(report)
	if err := ValidateReport(report); err == nil || !strings.Contains(err.Error(), "failover evidence") {
		t.Fatalf("expected changed auth pool rejection, got %v", err)
	}
}

func validReportFixture() Report {
	started := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	return Report{
		Version:                1,
		RunID:                  "postgresql-ha-pitr-test",
		ImplementationRevision: strings.Repeat("1", 40),
		PostgreSQLVersion:      "18.4",
		StartedAt:              started,
		CompletedAt:            started.Add(45 * time.Second),
		Topology: TopologyReport{
			PrimarySystemID: "7600000000000000001", StandbySystemID: "7600000000000000001",
			PromotedSystemID: "7600000000000000001", RestoredSystemID: "7600000000000000001", SameHost: true,
		},
		Failover: FailoverReport{
			PrimaryFlushLSN: "0/30001A0", StandbyReplayLSN: "0/30001A0",
			DetectionDurationMS: 1200, PromotionDurationMS: 900, ReconnectDurationMS: 1400,
			PreFailoverRows: 1, TransitionRows: 0, PostPromotionRows: 1,
			SameHandler: true, SameRuntimeStore: true, SameAuthPool: true,
			SameRuntimePool: true, PromotedReadWrite: true,
		},
		PITR: PITRReport{
			TargetLSN: "0/30001A0", RestoredReplayLSN: "0/30001A0",
			T2Fingerprint: strings.Repeat("c", 64), RestoredFingerprint: strings.Repeat("c", 64),
			BaseBackupBytes: 1024, WALArchiveBytes: 2048, WALInventorySHA256: strings.Repeat("d", 64),
			DurationMS: 4300, HistoricalStateRestored: true, ProjectionRebuilt: true,
		},
		Security: SecurityReport{
			HistoricalTokenInitiallyActive: true, HistoricalTokenRevoked: true,
			HistoricalTokenRejected: true, NewTokenAccepted: true, CurrentStateReconciled: true,
			RLSPolicies: 15, TenantForeignKeys: 26,
		},
		HardGates: map[string]bool{
			"replication_caught_up":  true,
			"no_false_receipt":       true,
			"same_pool_recovered":    true,
			"target_state_equal":     true,
			"credentials_regoverned": true,
		},
		Failures: []FailureRecord{{
			Phase: "failover", Attempt: 1, Code: "transition_database_unavailable", Message: "request failed before standby promotion", Retried: true,
		}},
		NonClaims: []string{
			"same-host PostgreSQL processes are not cross-host HA evidence",
			"measured timings are not universal SLOs",
			"historical-state quarantine is required before traffic resumes",
		},
	}
}

func cloneHardGates(input map[string]bool) map[string]bool {
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
