package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDimensionalMigrationReportWritesDeterministicArtifactsAndReplays(t *testing.T) {
	report := validDimensionalMigrationReportFixture()
	report.RequestFingerprint = dimensionalMigrationRequestFingerprint(report)
	root := t.TempDir()
	paths, replayed, err := WriteDimensionalMigrationReport(root, report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first dimensional migration report was marked replayed")
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
		"# Active-Backlog Dimensional Migration Qualification",
		report.RunID,
		report.Profiles.CandidateID,
		"postgres_restart_during_candidate_embedding",
		"Hard gates: PASS",
		"not an embedding-model ranking",
	} {
		if !strings.Contains(string(markdownBefore), required) {
			t.Fatalf("markdown missing %q:\n%s", required, markdownBefore)
		}
	}

	replayedPaths, replayed, err := WriteDimensionalMigrationReport(root, report)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || replayedPaths != paths {
		t.Fatalf("unexpected replay: replayed=%t paths=%#v want=%#v", replayed, replayedPaths, paths)
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
	if filepath.Base(paths.JSON) != "report.json" || filepath.Base(paths.Markdown) != "report.md" {
		t.Fatalf("unexpected report paths: %#v", paths)
	}
}

func TestDimensionalMigrationReportRejectsConflictFailedGateAndSecret(t *testing.T) {
	report := validDimensionalMigrationReportFixture()
	report.RequestFingerprint = dimensionalMigrationRequestFingerprint(report)
	root := t.TempDir()
	if _, _, err := WriteDimensionalMigrationReport(root, report); err != nil {
		t.Fatal(err)
	}

	conflict := report
	conflict.Counts.CandidateFinalVectors++
	conflict.RequestFingerprint = dimensionalMigrationRequestFingerprint(conflict)
	if conflict.RequestFingerprint != report.RequestFingerprint {
		t.Fatal("measured output changed the immutable request fingerprint")
	}
	if _, _, err := WriteDimensionalMigrationReport(root, conflict); err == nil || !strings.Contains(err.Error(), "conflicting report replay") {
		t.Fatalf("expected conflicting replay rejection, got %v", err)
	}

	failed := report
	failed.HardGates = cloneDimensionalMigrationGates(report.HardGates)
	failed.HardGates["same_pools_recovered"] = false
	failed.RequestFingerprint = dimensionalMigrationRequestFingerprint(failed)
	if err := ValidateDimensionalMigrationReport(failed); err == nil || !strings.Contains(err.Error(), "hard gate") {
		t.Fatalf("expected failed hard-gate rejection, got %v", err)
	}

	secret := report
	secret.NonClaims = append(append([]string(nil), report.NonClaims...), "postgresql://operator@example.invalid/database")
	secret.RequestFingerprint = dimensionalMigrationRequestFingerprint(secret)
	if err := ValidateDimensionalMigrationReport(secret); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("expected secret rejection, got %v", err)
	}

	nonMonotonic := report
	nonMonotonic.Queries.P50MS = 10
	nonMonotonic.Queries.P95MS = 7
	nonMonotonic.Queries.P99MS = 5
	if err := ValidateDimensionalMigrationReport(nonMonotonic); err == nil || !strings.Contains(err.Error(), "latency percentiles") {
		t.Fatalf("expected non-monotonic latency rejection, got %v", err)
	}
}

func validDimensionalMigrationReportFixture() DimensionalMigrationReport {
	started := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	hardGates := map[string]bool{
		"schema_classes_isolated":               true,
		"authority_writes_provider_independent": true,
		"incumbent_served_during_backlog":       true,
		"restart_committed_no_partial_vector":   true,
		"same_pools_recovered":                  true,
		"tail_event_count_exact":                true,
		"physical_classes_converged":            true,
		"ineligible_rows_absent":                true,
		"candidate_reset_isolated":              true,
		"real_provider_2560_dimensions":         true,
		"retrieval_audits_profile_scoped":       true,
		"default_profile_unchanged":             true,
	}
	return DimensionalMigrationReport{
		Version:                1,
		RunID:                  "active-backlog-dimension-test",
		CaseSHA256:             strings.Repeat("a", 64),
		ImplementationRevision: strings.Repeat("1", 40),
		PostgreSQLVersion:      "18.4",
		PGVectorVersion:        "0.8.5",
		StartedAt:              started,
		CompletedAt:            started.Add(30 * time.Second),
		Environment: DimensionalMigrationEnvironment{
			OS: "Darwin arm64", CPU: "Apple M4 Pro", MemoryGiB: 48, SchemaVersion: 16,
		},
		Profiles: DimensionalMigrationProfiles{
			IncumbentID: ProductionRetrievalProfileID, IncumbentClass: ProjectionClass1024,
			IncumbentDimensions: 1024, IncumbentLifecycle: "active",
			CandidateID: DimensionalMigrationRetrievalProfileID, CandidateClass: ProjectionClass2560,
			CandidateDimensions: 2560, CandidateLifecycle: "candidate",
		},
		Counts: DimensionalMigrationCounts{
			InitialActive: 20000, Revisions: 2000, Deleted: 500, NewFacts: 500,
			TailEvents: 5000, FinalActive: 20000, LexicalRows: 20000,
			IncumbentFinalVectors: 20000, CandidateFinalVectors: 20000,
		},
		Snapshot: DimensionalMigrationSnapshot{
			IncumbentProjected: 20000, CandidateScanned: 20000, CandidateProjected: 19500,
			CandidateSkippedChanged: 500, CandidateFinalLag: 0,
		},
		Workload: DimensionalMigrationWorkload{
			QueryClients: 16, QuerySamples: 320, WriterDurationMS: 1400,
			AuthorityCompletedWhileCandidateBlocked: true,
		},
		Restart: DimensionalMigrationRestart{
			FailureCode: "postgres_restart_during_candidate_embedding", PartialRows: 0,
			InterruptedCursorAdvance: 0, RecoveryDurationMS: 900, SamePoolsRecovered: true,
		},
		Queries: DimensionalMigrationQueries{
			Successful: 319, BoundedDatabaseFailures: 1, CrossScopeResults: 0,
			P50MS: 4, P95MS: 20, P99MS: 80,
		},
		Reset: DimensionalMigrationReset{
			CandidateRowsAfterReset: 0, IncumbentRowsUnchanged: true,
			AuthorityUnchanged: true, CandidateRebuilt: true,
		},
		Provider: DimensionalMigrationProvider{
			BaseURL: "https://api.siliconflow.cn/v1", Model: "Qwen/Qwen3-Embedding-4B",
			Dimensions: 2560, Requests: 2, DurationMS: 350,
			ProjectionResponseSHA256: strings.Repeat("b", 64), QueryResponseSHA256: strings.Repeat("c", 64),
		},
		HardGates: hardGates,
		Failures: []DimensionalMigrationFailure{{
			Phase: "candidate_restart", Attempt: 1, Code: "postgres_restart_during_candidate_embedding",
			Message: "the dedicated PostgreSQL cluster was stopped after candidate embedding started", Retried: true,
		}},
		NonClaims: []string{
			"not an embedding-model ranking",
			"not an automatic profile promotion",
			"not a long-duration retention result",
			"not cross-host HA evidence",
			"not final release acceptance",
		},
	}
}

func cloneDimensionalMigrationGates(input map[string]bool) map[string]bool {
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
