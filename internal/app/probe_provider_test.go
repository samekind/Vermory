package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeProviderMockWritesPerModelArtifacts(t *testing.T) {
	root := t.TempDir()

	report, err := ProbeProvider(context.Background(), ProbeProviderOptions{
		ArtifactRoot: root,
		Provider:     "mock",
		RunID:        "probe-mock",
		Prompt:       "Only reply OK",
		Models:       []string{"model-a", "model-b"},
		MaxTokens:    32,
	})
	if err != nil {
		t.Fatalf("ProbeProvider returned error: %v", err)
	}

	if report.ProviderMode != "mock" {
		t.Fatalf("expected mock mode, got %q", report.ProviderMode)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 probe results, got %d", len(report.Results))
	}
	if report.Results[0].Status != "ok" || report.Results[1].Status != "ok" {
		t.Fatalf("expected successful probe statuses, got %#v", report.Results)
	}
	if !strings.Contains(report.Results[0].OutputPreview, "Only reply OK") {
		t.Fatalf("expected preview to include prompt, got %q", report.Results[0].OutputPreview)
	}
	if _, err := os.Stat(filepath.Join(root, "provider-probes", "probe-mock", "model-a", "output.md")); err != nil {
		t.Fatalf("expected output artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "provider-probes", "probe-mock", "report.md")); err != nil {
		t.Fatalf("expected report artifact: %v", err)
	}
	reportMarkdown, err := os.ReadFile(filepath.Join(root, "provider-probes", "probe-mock", "report.md"))
	if err != nil {
		t.Fatalf("read report artifact: %v", err)
	}
	if !strings.HasPrefix(string(reportMarkdown), "# Vermory Provider Probe Report\n") {
		t.Fatalf("unexpected report title:\n%s", reportMarkdown)
	}
}

func TestProbeProviderRequiresModels(t *testing.T) {
	_, err := ProbeProvider(context.Background(), ProbeProviderOptions{
		ArtifactRoot: t.TempDir(),
		Provider:     "mock",
	})
	if err == nil {
		t.Fatal("expected missing models error")
	}
}
