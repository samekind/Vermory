package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"vermory/internal/brand"

	"github.com/spf13/cobra"
)

func TestVersionCommandUsesStableBuildMetadata(t *testing.T) {
	originalVersion, originalRevision, originalBuildDate := brand.Version, brand.Revision, brand.BuildDate
	brand.Version = "0.1.0-alpha.1"
	brand.Revision = "abc123"
	brand.BuildDate = "2026-07-14T00:00:00Z"
	t.Cleanup(func() {
		brand.Version, brand.Revision, brand.BuildDate = originalVersion, originalRevision, originalBuildDate
	})

	command := newRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("version output is not JSON: %v\n%s", err, output.String())
	}
	for key, want := range map[string]string{
		"version":    "0.1.0-alpha.1",
		"revision":   "abc123",
		"build_date": "2026-07-14T00:00:00Z",
	} {
		if got[key] != want {
			t.Fatalf("version field %s = %#v, want %q", key, got[key], want)
		}
	}
	if value, ok := got["go_version"].(string); !ok || value == "" {
		t.Fatalf("missing go_version: %#v", got)
	}
}

func TestRootVersionFlagUsesBrandVersion(t *testing.T) {
	originalVersion := brand.Version
	brand.Version = "0.1.0-alpha.1"
	t.Cleanup(func() { brand.Version = originalVersion })

	command := newRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "0.1.0-alpha.1") {
		t.Fatalf("root version output did not use brand version: %q", output.String())
	}
}

func TestRealityAttestationCLIExposesVerifyButNoSignCommand(t *testing.T) {
	verifyFound := false
	for _, command := range newRootCommand().Commands() {
		switch command.Name() {
		case "reality-attestation-verify":
			verifyFound = true
		case "reality-attestation-sign":
			t.Fatal("local sealed attestation signing must not be exposed")
		}
	}
	if !verifyFound {
		t.Fatal("expected reality-attestation-verify command")
	}
}

func TestExperiment0CLIIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() == "experiment-0" {
			return
		}
	}
	t.Fatal("expected experiment-0 command")
}

func TestProviderCommandsAdvertiseGrokCLI(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "eval-self-case" && command.Name() != "eval-casebook" && command.Name() != "eval-matrix" && command.Name() != "probe-provider" && command.Name() != "acceptance-report" && command.Name() != "benchmark-longmemeval" {
			continue
		}
		flag := command.Flags().Lookup("provider")
		if flag == nil || !strings.Contains(flag.Usage, "grok-cli") {
			t.Fatalf("command %q must advertise grok-cli provider support", command.Name())
		}
	}
}

func TestBenchmarkLongMemEvalCommandIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "benchmark-longmemeval" {
			continue
		}
		for _, flagName := range []string{
			"database-url",
			"source-dataset",
			"qualification",
			"execution",
			"artifact-root",
			"provider",
			"base-url",
			"api-key-env",
			"model",
			"run-id",
			"implementation-revision",
		} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("benchmark-longmemeval must expose --%s", flagName)
			}
		}
		return
	}
	t.Fatal("expected benchmark-longmemeval command")
}

func TestBenchmarkLongMemEvalRetrievalCommandIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "benchmark-longmemeval-retrieval" {
			continue
		}
		for _, flagName := range []string{
			"database-url",
			"source-dataset",
			"qualification",
			"execution",
			"artifact-root",
			"run-id",
			"implementation-revision",
			"resume",
			"vector-profile",
			"embedding-api-key-env",
		} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("benchmark-longmemeval-retrieval must expose --%s", flagName)
			}
		}
		return
	}
	t.Fatal("expected benchmark-longmemeval-retrieval command")
}

func TestBenchmarkLongMemEvalQACommandIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "benchmark-longmemeval-qa" {
			continue
		}
		for _, flagName := range []string{
			"source-dataset",
			"retrieval-results",
			"qualification",
			"execution",
			"artifact-root",
			"run-id",
			"implementation-revision",
			"phase",
			"reader-command",
			"reader-base-url",
			"reader-api-key-env",
			"judge-command",
			"judge-base-url",
			"judge-api-key-env",
			"resume",
		} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("benchmark-longmemeval-qa must expose --%s", flagName)
			}
		}
		return
	}
	t.Fatal("expected benchmark-longmemeval-qa command")
}

func TestMCPStdioCommandIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() != "mcp-stdio" {
			continue
		}
		for _, flagName := range []string{"database-url", "tenant-id", "workspace-attachment"} {
			if command.Flags().Lookup(flagName) == nil {
				t.Fatalf("mcp-stdio must expose --%s", flagName)
			}
		}
		return
	}
	t.Fatal("expected mcp-stdio command")
}

func TestOperatorCommandsAreRegistered(t *testing.T) {
	names := map[string]bool{}
	for _, command := range newRootCommand().Commands() {
		names[command.Name()] = true
	}
	for _, want := range []string{"workspace", "memory"} {
		if !names[want] {
			t.Fatalf("expected root command %q", want)
		}
	}
}

func TestIdentityAndDatabaseCommandsAreRegistered(t *testing.T) {
	names := map[string]bool{}
	for _, command := range newRootCommand().Commands() {
		names[command.Name()] = true
	}
	for _, want := range []string{"identity", "database"} {
		if !names[want] {
			t.Fatalf("expected root command %q", want)
		}
	}
}

func TestAuthenticatedServeCommandIsRegistered(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() == "serve" {
			return
		}
	}
	t.Fatal("expected authenticated serve command")
}

func TestOperatorMemoryForgetHasNoFreeTextFlag(t *testing.T) {
	root := newRootCommand()
	for _, parent := range root.Commands() {
		if parent.Name() != "memory" {
			continue
		}
		for _, child := range parent.Commands() {
			if child.Name() != "forget" {
				continue
			}
			if child.Flags().Lookup("reason") != nil || child.Flags().Lookup("content") != nil {
				t.Fatal("forget command must not accept free-text deletion content")
			}
			return
		}
	}
	t.Fatal("expected memory forget command")
}

func TestMemoryEligibilityCommandsRegistered(t *testing.T) {
	root := newRootCommand()
	for _, parent := range root.Commands() {
		if parent.Name() != "memory" {
			continue
		}
		commands := map[string]*cobra.Command{}
		for _, child := range parent.Commands() {
			commands[child.Name()] = child
		}
		setValidity := commands["set-validity"]
		archive := commands["archive"]
		if setValidity == nil || archive == nil {
			t.Fatalf("memory eligibility commands missing: set-validity=%v archive=%v", setValidity != nil, archive != nil)
		}
		for _, flag := range []string{"continuity-id", "operation-id", "memory-id", "valid-from", "valid-until"} {
			if setValidity.Flags().Lookup(flag) == nil {
				t.Fatalf("set-validity is missing --%s", flag)
			}
		}
		for _, flag := range []string{"continuity-id", "operation-id", "memory-id"} {
			if archive.Flags().Lookup(flag) == nil {
				t.Fatalf("archive is missing --%s", flag)
			}
		}
		for _, command := range []*cobra.Command{setValidity, archive} {
			if command.Flags().Lookup("content") != nil || command.Flags().Lookup("reason") != nil {
				t.Fatalf("%s must not accept free text", command.Name())
			}
		}
		return
	}
	t.Fatal("expected memory command")
}

func TestOperatorSourceRevisionCommandIsRegistered(t *testing.T) {
	root := newRootCommand()
	for _, parent := range root.Commands() {
		if parent.Name() != "memory" {
			continue
		}
		for _, child := range parent.Commands() {
			if child.Name() != "revise-source" {
				continue
			}
			for _, flag := range []string{"repo-root", "operation-id", "memory-id", "source-ref", "content"} {
				if child.Flags().Lookup(flag) == nil {
					t.Fatalf("revise-source is missing --%s", flag)
				}
			}
			return
		}
	}
	t.Fatal("expected memory revise-source command")
}

func TestOperatorSourceMatchCommandsAreRegistered(t *testing.T) {
	root := newRootCommand()
	for _, parent := range root.Commands() {
		if parent.Name() != "memory" {
			continue
		}
		found := map[string]bool{}
		for _, child := range parent.Commands() {
			found[child.Name()] = true
			if child.Name() != "match-source" {
				continue
			}
			for _, flag := range []string{"repo-root", "operation-id", "source-ref", "content", "provider", "model", "base-url", "api-key-env", "grok-command", "disable-thinking"} {
				if child.Flags().Lookup(flag) == nil {
					t.Fatalf("match-source is missing --%s", flag)
				}
			}
		}
		if !found["match-source"] || !found["inspect-source-match"] {
			t.Fatalf("source match commands are missing: %#v", found)
		}
		return
	}
	t.Fatal("expected memory source match commands")
}

func TestOperatorSourceFormationCommandsAreRegistered(t *testing.T) {
	root := newRootCommand()
	for _, parent := range root.Commands() {
		if parent.Name() != "memory" {
			continue
		}
		found := map[string]bool{}
		for _, child := range parent.Commands() {
			found[child.Name()] = true
			expectedFlags := map[string][]string{
				"form-document":     {"repo-root", "operation-id", "source-file", "source-ref", "provider", "model", "base-url", "api-key-env", "grok-command", "disable-thinking"},
				"form-conversation": {"operation-id", "channel", "thread-id", "observation-id", "recent-user-observations", "provider", "model", "base-url", "api-key-env", "grok-command", "disable-thinking"},
			}[child.Name()]
			for _, flag := range expectedFlags {
				if child.Flags().Lookup(flag) == nil {
					t.Fatalf("%s is missing --%s", child.Name(), flag)
				}
			}
		}
		if !found["form-document"] || !found["form-conversation"] || !found["inspect-source-formation"] {
			t.Fatalf("source formation commands are missing: %#v", found)
		}
		return
	}
	t.Fatal("expected memory source formation commands")
}
