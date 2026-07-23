package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorClientQualificationScriptRequiresCompleteEvidence(t *testing.T) {
	repoRoot := cursorTestRepoRoot(t)
	script := filepath.Join(repoRoot, "scripts", "cursor-client-qualification.sh")
	if output, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("cursor qualification script is not valid bash: %v\n%s", err, output)
	}

	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "--force") || strings.Contains(source, "--yolo") {
		t.Fatal("cursor qualification must not bypass client approvals")
	}
	for _, required := range []string{
		"--ledger-command",
		"artifact_valid",
		"ledger_valid",
		"replay_seen",
		"client_account_blocked",
		"refusing to overwrite run directory",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("cursor qualification script is missing %q", required)
		}
	}

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	cursor := writeCursorFixtureExecutable(t, root, "cursor-agent", scriptLines(
		"#!/bin/sh",
		"if [ \"$1\" = \"--version\" ]; then echo test-cursor-1; exit 0; fi",
		"if [ \"$1\" = \"status\" ]; then echo \"Logged in as fixture@example.invalid\"; exit 0; fi",
		"if [ \"$1\" = \"models\" ]; then echo \"gpt-5.3-codex - Codex fixture\"; exit 0; fi",
		"if [ \"$1\" = \"mcp\" ]; then",
		"  case \"$2\" in",
		"    enable) echo enabled; exit 0 ;;",
		"    disable) exit 0 ;;",
		"    list) echo \"vermory-w25: ready\"; exit 0 ;;",
		"    list-tools)",
		"      echo \"Tools for vermory-w25 (2):\"",
		"      echo \"- commit_observation (content, delivery_id, operation_id, source_ref)\"",
		"      echo \"- prepare_context (cwd, max_items, operation_id, repo_root, task)\"",
		"      exit 0 ;;",
		"  esac",
		"fi",
		"printf '%s\\n' 'canonical_repository=https://github.com/samekind/Vermory' 'continuation_marker=samekind-w25-current' > continuity-report.md",
		"echo '{\"tool\":\"commit_observation\",\"result\":{\"replayed\":true}}'",
	))
	mcp := writeCursorFixtureExecutable(t, root, "mcp-wrapper", scriptLines("#!/bin/sh", "exit 0"))
	ledger := writeCursorFixtureExecutable(t, root, "ledger-wrapper", scriptLines(
		"#!/bin/sh",
		"if [ ! -e \"$0.state\" ]; then printf preflight > \"$0.state\"; echo '{\"deliveries\":0,\"agent_results\":0,\"proposed_memories\":0,\"active_agent_memories\":0,\"canonical_delivered\":0,\"marker_delivered\":0,\"stale_delivered\":0,\"distractor_delivered\":0,\"source_ref_matches\":0}'; exit 0; fi",
		"echo '{\"deliveries\":1,\"agent_results\":1,\"proposed_memories\":1,\"active_agent_memories\":0,\"canonical_delivered\":1,\"marker_delivered\":1,\"stale_delivered\":0,\"distractor_delivered\":0,\"source_ref_matches\":1}'",
	))

	run := exec.Command("bash", script,
		"--cursor-agent", cursor,
		"--workspace", workspace,
		"--mcp-command", mcp,
		"--ledger-command", ledger,
		"--artifact-root", artifacts,
		"--run-id", "fixture-success",
	)
	run.Dir = repoRoot
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("fixture cursor run failed: %v\n%s", err, output)
	}

	summaryPath := filepath.Join(artifacts, "fixture-success", "summary.json")
	summary := decodeCursorSummary(t, summaryPath)
	for _, field := range []string{"pass", "artifact_valid", "ledger_valid", "replay_seen"} {
		if value, ok := summary[field].(bool); !ok || !value {
			t.Fatalf("complete fixture evidence field %s did not pass: %#v", field, summary)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("project MCP configuration was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "fixture-success", "status.raw")); !os.IsNotExist(err) {
		t.Fatalf("raw login identity must not be retained: %v", err)
	}

	before, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	replay := exec.Command("bash", script,
		"--cursor-agent", cursor,
		"--workspace", workspace,
		"--mcp-command", mcp,
		"--ledger-command", ledger,
		"--artifact-root", artifacts,
		"--run-id", "fixture-success",
	)
	replay.Dir = repoRoot
	if err := replay.Run(); err == nil {
		t.Fatal("runner overwrote an existing run id")
	}
	after, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed replay changed retained evidence")
	}
}

func TestCursorClientQualificationRejectsIncompleteLedger(t *testing.T) {
	repoRoot := cursorTestRepoRoot(t)
	script := filepath.Join(repoRoot, "scripts", "cursor-client-qualification.sh")
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	cursor := writeCursorFixtureExecutable(t, root, "cursor-agent", scriptLines(
		"#!/bin/sh",
		"case \"$1\" in",
		"  --version) echo test-cursor-1; exit 0 ;;",
		"  status) echo \"Logged in as fixture@example.invalid\"; exit 0 ;;",
		"  models) echo \"gpt-5.3-codex - Codex fixture\"; exit 0 ;;",
		"  mcp)",
		"    case \"$2\" in",
		"      enable) exit 0 ;;",
		"      disable) exit 0 ;;",
		"      list) echo \"vermory-w25: ready\"; exit 0 ;;",
		"      list-tools)",
		"        echo \"- commit_observation (content, delivery_id, operation_id, source_ref)\"",
		"        echo \"- prepare_context (cwd, max_items, operation_id, repo_root, task)\"",
		"        exit 0 ;;",
		"    esac ;;",
		"esac",
		"printf '%s\\n' 'canonical_repository=https://github.com/samekind/Vermory' 'continuation_marker=samekind-w25-current' > continuity-report.md",
		"echo '{\"replayed\":true}'",
	))
	mcp := writeCursorFixtureExecutable(t, root, "mcp-wrapper", scriptLines("#!/bin/sh", "exit 0"))
	ledger := writeCursorFixtureExecutable(t, root, "ledger-wrapper", scriptLines(
		"#!/bin/sh",
		"if [ ! -e \"$0.state\" ]; then printf preflight > \"$0.state\"; echo '{\"deliveries\":0,\"agent_results\":0,\"proposed_memories\":0,\"active_agent_memories\":0,\"canonical_delivered\":0,\"marker_delivered\":0,\"stale_delivered\":0,\"distractor_delivered\":0,\"source_ref_matches\":0}'; exit 0; fi",
		"echo '{\"deliveries\":1,\"agent_results\":1,\"proposed_memories\":0,\"active_agent_memories\":0,\"canonical_delivered\":1,\"marker_delivered\":1,\"stale_delivered\":0,\"distractor_delivered\":0,\"source_ref_matches\":1}'",
	))

	run := exec.Command("bash", script,
		"--cursor-agent", cursor,
		"--workspace", workspace,
		"--mcp-command", mcp,
		"--ledger-command", ledger,
		"--artifact-root", artifacts,
		"--run-id", "fixture-incomplete",
	)
	run.Dir = repoRoot
	if err := run.Run(); err == nil {
		t.Fatal("runner accepted incomplete PostgreSQL evidence")
	}
	summary := decodeCursorSummary(t, filepath.Join(artifacts, "fixture-incomplete", "summary.json"))
	if summary["pass"] != false || summary["ledger_valid"] != false || summary["failure_class"] != "governance_boundary_failed" {
		t.Fatalf("unexpected incomplete-ledger result: %#v", summary)
	}
}

func writeCursorFixtureExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeCursorSummary(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, data)
	}
	return result
}

func scriptLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func cursorTestRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
