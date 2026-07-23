package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vermory/internal/provider"
)

func TestEvalSelfCaseMockWritesPlatformRunArtifacts(t *testing.T) {
	root := t.TempDir()

	report, err := EvalSelfCase(context.Background(), EvalSelfCaseOptions{
		ArtifactRoot: root,
		Provider:     "mock",
		Model:        "mock-model",
		RunID:        "eval-test",
	})
	if err != nil {
		t.Fatalf("EvalSelfCase returned error: %v", err)
	}

	if report.ProviderMode != "mock" {
		t.Fatalf("expected mock provider mode, got %q", report.ProviderMode)
	}
	if len(report.Results) != 4 {
		t.Fatalf("expected 4 baseline results, got %d", len(report.Results))
	}
	if _, err := os.Stat(filepath.Join(root, "platform-runs", "eval-test", "report.md")); err != nil {
		t.Fatalf("expected report artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "platform-runs", "eval-test", "contextmesh_packet", "packet.md")); err != nil {
		t.Fatalf("expected contextmesh packet artifact: %v", err)
	}
}

func TestBuildProviderSelectsLocallyAuthenticatedGrokCLI(t *testing.T) {
	llm, mode, name, model, err := buildProvider(EvalSelfCaseOptions{Provider: "grok-cli"})
	if err != nil {
		t.Fatalf("buildProvider returned error: %v", err)
	}
	if _, ok := llm.(*provider.GrokCLI); !ok {
		t.Fatalf("expected GrokCLI provider, got %T", llm)
	}
	if mode != "real" || name != "grok-cli" || model != "grok-4.5" {
		t.Fatalf("unexpected Grok provider identity: mode=%q name=%q model=%q", mode, name, model)
	}
}
