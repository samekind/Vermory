package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"vermory/internal/retrievalablation"
)

func TestRetrievalProfileComparisonCommandIsRegisteredWithFrozenFlags(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "retrieval-profile-compare" {
			continue
		}
		for _, flagName := range []string{
			"database-url", "corpus", "run-id", "output-dir",
			"embedding-api-key-env", "implementation-revision",
		} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("retrieval-profile-compare must expose --%s", flagName)
			}
		}
		for _, forbidden := range []string{"embedding-model", "embedding-base-url", "profile-id"} {
			if command.Flags().Lookup(forbidden) != nil {
				t.Fatalf("retrieval-profile-compare must not expose mutable --%s", forbidden)
			}
		}
		return
	}
	t.Fatal("expected retrieval-profile-compare command")
}

func TestRetrievalProfileComparisonCommandExecutesWithoutPrintingSecret(t *testing.T) {
	t.Setenv("PROFILE_TEST_KEY", "profile-secret-value")
	original := executeRetrievalProfileComparison
	t.Cleanup(func() { executeRetrievalProfileComparison = original })
	var captured retrievalablation.ProfileComparisonRunOptions
	executeRetrievalProfileComparison = func(_ context.Context, options retrievalablation.ProfileComparisonRunOptions, outputDir string) (retrievalablation.ProfileComparison, retrievalablation.ArtifactPaths, bool, error) {
		captured = options
		return retrievalablation.ProfileComparison{
			RunID: "run", Profiles: []retrievalablation.ProfileComparisonReport{
				{Metrics: retrievalablation.Aggregate{QueryCount: 18}},
			},
			HardGates:           retrievalablation.ProfileComparisonHardGates{Pass: true},
			Decision:            retrievalablation.ProfilePromotionDecision{Status: "keep_candidate"},
			QualificationStatus: "measured",
		}, retrievalablation.ArtifactPaths{JSON: outputDir + "/report.json", Markdown: outputDir + "/report.md"}, false, nil
	}

	command := newRetrievalProfileComparisonCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"--database-url", "postgresql:///test",
		"--corpus", "corpus.json",
		"--run-id", "run",
		"--output-dir", "artifacts/run",
		"--embedding-api-key-env", "PROFILE_TEST_KEY",
		"--implementation-revision", "abc123",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if captured.EmbeddingAPIKey != "profile-secret-value" {
		t.Fatalf("command did not pass the configured key")
	}
	for _, required := range []string{"run=run", "queries=18", "hard_gates=pass", "decision=keep_candidate", "report="} {
		if !strings.Contains(output.String(), required) {
			t.Fatalf("command output missing %q: %s", required, output.String())
		}
	}
	if strings.Contains(output.String(), "profile-secret-value") {
		t.Fatalf("command output leaked API key: %s", output.String())
	}
}
