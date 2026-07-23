package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"vermory/internal/retrievalablation"
)

func TestRetrievalAblationCommandIsRegisteredWithRequiredFlags(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "retrieval-ablation" {
			continue
		}
		for _, flagName := range []string{
			"database-url", "corpus", "run-id", "output-dir", "embedding-base-url",
			"embedding-api-key-env", "embedding-model", "embedding-dimensions", "implementation-revision",
		} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("retrieval-ablation must expose --%s", flagName)
			}
		}
		return
	}
	t.Fatal("expected retrieval-ablation command")
}

func TestRetrievalAblationCommandRequiresConfiguredAPIKey(t *testing.T) {
	command := newRetrievalAblationCommand()
	command.SetArgs([]string{
		"--database-url", "postgresql:///test",
		"--corpus", "corpus.json",
		"--run-id", "run",
		"--output-dir", "artifacts/run",
		"--embedding-base-url", "https://api.siliconflow.cn/v1",
		"--embedding-api-key-env", "W08_MISSING_KEY",
		"--embedding-model", "BAAI/bge-m3",
		"--embedding-dimensions", "1024",
		"--implementation-revision", "abc123",
	})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "W08_MISSING_KEY") {
		t.Fatalf("missing API key error=%v", err)
	}
}

func TestRetrievalAblationCommandExecutesWithoutPrintingSecret(t *testing.T) {
	t.Setenv("W08_TEST_KEY", "test-secret-value")
	original := executeRetrievalAblation
	t.Cleanup(func() { executeRetrievalAblation = original })
	var captured retrievalablation.Options
	executeRetrievalAblation = func(_ context.Context, options retrievalablation.Options, outputDir string) (retrievalablation.Report, retrievalablation.ArtifactPaths, bool, error) {
		captured = options
		return retrievalablation.Report{
			RunID: "run", Conditions: []retrievalablation.ConditionReport{{Name: retrievalablation.ConditionLexical, Metrics: retrievalablation.Aggregate{QueryCount: 24}}},
			HardGates: retrievalablation.HardGateReport{Pass: true}, QualificationStatus: "measured",
		}, retrievalablation.ArtifactPaths{JSON: outputDir + "/report.json", Markdown: outputDir + "/report.md"}, false, nil
	}
	command := newRetrievalAblationCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"--database-url", "postgresql:///test",
		"--corpus", "corpus.json",
		"--run-id", "run",
		"--output-dir", "artifacts/run",
		"--embedding-base-url", "https://api.siliconflow.cn/v1",
		"--embedding-api-key-env", "W08_TEST_KEY",
		"--embedding-model", "BAAI/bge-m3",
		"--embedding-dimensions", "1024",
		"--implementation-revision", "abc123",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if captured.EmbeddingAPIKey != "test-secret-value" || captured.EmbeddingModel != "BAAI/bge-m3" {
		t.Fatalf("command options mismatch: %#v", captured)
	}
	for _, required := range []string{"run=run", "queries=24", "hard_gates=pass", "qualification=measured", "report="} {
		if !strings.Contains(output.String(), required) {
			t.Fatalf("command output missing %q: %s", required, output.String())
		}
	}
	if strings.Contains(output.String(), "test-secret-value") {
		t.Fatalf("command output leaked API key: %s", output.String())
	}
}
