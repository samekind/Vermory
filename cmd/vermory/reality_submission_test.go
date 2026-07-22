package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/reality"
)

func TestRealitySubmissionCommandsCreateAndVerifyExactArtifact(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "vermory_linux_amd64.tar.gz")
	if err := os.WriteFile(artifact, []byte("external evaluator target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	submissionPath := filepath.Join(dir, "submission.json")

	create := newRootCommand()
	createOutput := new(bytes.Buffer)
	create.SetOut(createOutput)
	create.SetErr(createOutput)
	create.SetArgs([]string{
		"reality-submission-create",
		"--artifact", artifact,
		"--artifact-uri", "https://example.test/releases/vermory_linux_amd64.tar.gz",
		"--source-revision", strings.Repeat("a", 40),
		"--submission-id", "cli-submission-1",
		"--suite-profile", "core-continuity-v1",
		"--interface", "workspace_mcp_stdio_v1",
		"--interface", "authenticated_web_chat_http_v1",
		"--platform", "linux_amd64",
		"--created-at", "2026-07-23T08:00:00Z",
		"--valid-for", "168h",
		"--nonce", strings.Repeat("1", 64),
		"--output", submissionPath,
	})
	if err := create.Execute(); err != nil {
		t.Fatalf("create submission: %v\n%s", err, createOutput.String())
	}
	data, err := os.ReadFile(submissionPath)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := reality.ParseEvaluationSubmission(data)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Implementation.ArtifactName != filepath.Base(artifact) || submission.Nonce != strings.Repeat("1", 64) {
		t.Fatalf("unexpected submission: %#v", submission)
	}

	verify := newRootCommand()
	verifyOutput := new(bytes.Buffer)
	verify.SetOut(verifyOutput)
	verify.SetErr(verifyOutput)
	verify.SetArgs([]string{"reality-submission-verify", "--input", submissionPath, "--artifact", artifact})
	if err := verify.Execute(); err != nil {
		t.Fatalf("verify submission: %v\n%s", err, verifyOutput.String())
	}
	if !strings.Contains(verifyOutput.String(), "artifact_verified=true") {
		t.Fatalf("unexpected verify output: %s", verifyOutput.String())
	}

	if err := os.WriteFile(artifact, []byte("mutated evaluator target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutated := newRootCommand()
	mutated.SetArgs([]string{"reality-submission-verify", "--input", submissionPath, "--artifact", artifact})
	if err := mutated.Execute(); err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("expected mutated artifact rejection, got %v", err)
	}
}

func TestRealitySubmissionAndAttestationCommandSurface(t *testing.T) {
	root := newRootCommand()
	commands := map[string]*struct{}{}
	for _, command := range root.Commands() {
		commands[command.Name()] = nil
	}
	for _, name := range []string{"reality-submission-create", "reality-submission-verify", "reality-attestation-verify"} {
		if _, ok := commands[name]; !ok {
			t.Fatalf("expected %s command", name)
		}
	}
	if _, ok := commands["reality-attestation-sign"]; ok {
		t.Fatal("production CLI must not expose an external evaluation attestation signer")
	}
	attestation, _, err := root.Find([]string{"reality-attestation-verify"})
	if err != nil {
		t.Fatal(err)
	}
	if attestation.Flags().Lookup("submission") == nil {
		t.Fatal("version 2 attestation verification must expose --submission")
	}
}

func TestRealitySubmissionCreateRefusesToOverwriteArtifact(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "vermory_linux_amd64.tar.gz")
	if err := os.WriteFile(artifact, []byte("keep this artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := newRootCommand()
	command.SetArgs([]string{
		"reality-submission-create",
		"--artifact", artifact,
		"--artifact-uri", "https://example.test/releases/vermory_linux_amd64.tar.gz",
		"--source-revision", strings.Repeat("a", 40),
		"--submission-id", "overwrite-control",
		"--suite-profile", "core-continuity-v1",
		"--interface", "workspace_mcp_stdio_v1",
		"--platform", "linux_amd64",
		"--output", artifact,
	})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "must not overwrite") {
		t.Fatalf("expected overwrite rejection, got %v", err)
	}
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep this artifact\n" {
		t.Fatalf("artifact changed: %q", data)
	}
}
