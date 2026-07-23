package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProbeProviderCommandFailsWhenProviderRejectsModel(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":30001,"message":"account unavailable"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("VERMORY_TEST_PROVIDER_KEY", "test-key")

	artifactRoot := t.TempDir()
	command := newRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"probe-provider",
		"--provider", "openai-compatible",
		"--base-url", server.URL,
		"--api-key-env", "VERMORY_TEST_PROVIDER_KEY",
		"--models", "test-model",
		"--run-id", "rejected-probe",
		"--artifact-root", artifactRoot,
	})

	err := command.Execute()
	if err == nil {
		t.Fatal("expected rejected provider probe to return an error")
	}
	if !strings.Contains(err.Error(), "provider probe failed for 1 model(s): test-model") {
		t.Fatalf("unexpected command error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
	if !strings.Contains(output.String(), "model=test-model status=error") {
		t.Fatalf("command output does not retain failed model status:\n%s", output.String())
	}
	if _, err := os.Stat(filepath.Join(artifactRoot, "provider-probes", "rejected-probe", "report.json")); err != nil {
		t.Fatalf("failed probe report was not retained: %v", err)
	}
}
