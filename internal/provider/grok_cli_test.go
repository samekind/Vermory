package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGrokCLIProviderRunsIsolatedSingleTurnAndCapturesJSON(t *testing.T) {
	dir := t.TempDir()
	capturePath := filepath.Join(dir, "arguments.txt")
	promptCapturePath := filepath.Join(dir, "prompt.txt")
	commandPath := filepath.Join(dir, "grok")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
prompt_file=''
previous=''
for argument in "$@"; do
  if [ "$previous" = '--prompt-file' ]; then
    prompt_file=$argument
    break
  fi
  previous=$argument
done
test -n "$prompt_file"
cp "$prompt_file" %q
printf '%%s\n' '{"text":"grok response","usage":{"input_tokens":2718,"cache_read_input_tokens":7285,"output_tokens":97,"reasoning_tokens":11,"total_tokens":10111},"modelUsage":{"grok-4.5":{}}}'
`, capturePath, promptCapturePath)
	if err := os.WriteFile(commandPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake grok command: %v", err)
	}

	client := NewGrokCLI(GrokCLIConfig{Command: commandPath})
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:         "grok-4.5",
		System:        "system instructions",
		Prompt:        "finish the task",
		ContextPacket: "confirmed context packet",
		MaxTokens:     64,
		JSONSchema:    `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if resp.Output != "grok response" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
	if resp.Model != "grok-4.5" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if !json.Valid(resp.RawArtifact) {
		t.Fatalf("raw artifact should be the Grok JSON response, got %q", resp.RawArtifact)
	}
	if resp.Usage == nil || *resp.Usage != (TokenUsage{
		InputTokens:       2718,
		CachedInputTokens: 7285,
		OutputTokens:      97,
		ReasoningTokens:   11,
		TotalTokens:       10111,
	}) {
		t.Fatalf("unexpected Grok usage: %#v", resp.Usage)
	}

	arguments, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read captured arguments: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(arguments)), "\n")
	var promptPath string
	foundMaxTurns := false
	foundToolAllowlist := false
	foundToolDenylist := false
	foundJSONSchema := false
	for index, line := range lines {
		if line == "--prompt-file" && index+1 < len(lines) {
			promptPath = lines[index+1]
		}
		if line == "--max-turns" {
			if index+1 >= len(lines) || lines[index+1] != "1" {
				t.Fatalf("expected Grok max turns 1, got %q", strings.Join(lines, " "))
			}
			foundMaxTurns = true
		}
		if line == "--tools" {
			if index+1 >= len(lines) || lines[index+1] != "todo_write" {
				t.Fatalf("expected Grok no-tool allowlist sentinel, got %q", strings.Join(lines, " "))
			}
			foundToolAllowlist = true
		}
		if line == "--disallowed-tools" {
			if index+1 >= len(lines) || lines[index+1] != "todo_write,update_goal,search_tool,use_tool,CallMcpTool,Agent" {
				t.Fatalf("expected complete Grok tool denylist, got %q", strings.Join(lines, " "))
			}
			foundToolDenylist = true
		}
		if line == "--json-schema" {
			if index+1 >= len(lines) || !json.Valid([]byte(lines[index+1])) {
				t.Fatalf("expected valid JSON schema after --json-schema, got %q", strings.Join(lines, " "))
			}
			foundJSONSchema = true
		}
	}
	if !foundMaxTurns {
		t.Fatalf("expected --max-turns in Grok arguments: %q", strings.Join(lines, " "))
	}
	if !foundToolAllowlist || !foundToolDenylist {
		t.Fatalf("expected no-tool allowlist and denylist in Grok arguments: %q", strings.Join(lines, " "))
	}
	if !foundJSONSchema {
		t.Fatalf("expected --json-schema in Grok arguments: %q", strings.Join(lines, " "))
	}
	for _, want := range []string{
		"--verbatim",
		"--no-memory",
		"--disable-web-search",
		"--no-plan",
		"--no-subagents",
		"--max-turns",
		"--tools",
		"--disallowed-tools",
		"--permission-mode",
		"dontAsk",
		"--output-format",
		"json",
		"--json-schema",
		"--model",
		"grok-4.5",
		"--prompt-file",
	} {
		if !strings.Contains(string(arguments), want) {
			t.Fatalf("expected Grok invocation to contain %q, got %q", want, arguments)
		}
	}
	for _, forbidden := range []string{"system instructions", "confirmed context packet", "finish the task"} {
		if strings.Contains(string(arguments), forbidden) {
			t.Fatalf("Grok process arguments leaked prompt content %q: %q", forbidden, arguments)
		}
	}
	prompt, err := os.ReadFile(promptCapturePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"system instructions", "confirmed context packet", "finish the task"} {
		if !strings.Contains(string(prompt), required) {
			t.Fatalf("prompt file omitted %q: %q", required, prompt)
		}
	}
	if promptPath == "" {
		t.Fatalf("Grok invocation omitted prompt path: %q", arguments)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("temporary prompt file survived provider call: path=%s err=%v", promptPath, err)
	}
}

func TestGrokCLIProviderCapturesStdoutThroughARegularFile(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "grok")
	script := `#!/bin/sh
if [ -f /dev/stdout ]; then
  printf '%s\n' '{"text":"stable file-backed response","modelUsage":{"grok-4.5":{}}}'
else
  printf '%s\n' '{"text":"","modelUsage":{"grok-4.5":{}}}'
fi
`
	if err := os.WriteFile(commandPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake grok command: %v", err)
	}

	client := NewGrokCLI(GrokCLIConfig{Command: commandPath})
	response, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "grok-4.5",
		Prompt: "test stable stdout capture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Output != "stable file-backed response" {
		t.Fatalf("unexpected output: %#v", response)
	}
}

func TestGrokCLIProviderUsesShellParentForCLIStability(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "grok")
	script := `#!/bin/sh
parent=$(ps -p "$PPID" -o comm=)
case "$parent" in
  *sh) printf '%s\n' '{"text":"shell-parent response","modelUsage":{"grok-4.5":{}}}' ;;
  *) printf '%s\n' '{"text":"","stopReason":"Cancelled","modelUsage":{"grok-4.5":{}}}' ;;
esac
`
	if err := os.WriteFile(commandPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake grok command: %v", err)
	}

	client := NewGrokCLI(GrokCLIConfig{Command: commandPath})
	response, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "grok-4.5",
		Prompt: "test shell parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Output != "shell-parent response" {
		t.Fatalf("unexpected output: %#v", response)
	}
}

func TestGrokCLIProviderCancellationKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "grok")
	childPath := filepath.Join(dir, "child.pid")
	script := fmt.Sprintf(`#!/bin/sh
sleep 30 &
child=$!
group=$(ps -o pgid= -p $$ | tr -d ' ')
printf '%%s %%s\n' "$child" "$group" > %q
mv %q %q
wait "$child"
`, childPath+".tmp", childPath+".tmp", childPath)
	if err := os.WriteFile(commandPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := NewGrokCLI(GrokCLIConfig{Command: commandPath}).Generate(ctx, GenerateRequest{
			Model:  "grok-4.5",
			Prompt: "block until cancellation",
		})
		result <- err
	}()

	var childPID int
	var processGroupID int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(childPath)
		if err == nil {
			fields := strings.Fields(string(raw))
			if len(fields) != 2 {
				t.Fatalf("unexpected child process record %q", raw)
			}
			childPID, err = strconv.Atoi(fields[0])
			if err != nil {
				t.Fatal(err)
			}
			processGroupID, err = strconv.Atoi(fields[1])
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 || processGroupID == 0 {
		t.Fatal("fake Grok child did not start")
	}
	cancel()
	if err := <-result; err == nil {
		t.Fatal("canceled Grok provider returned success")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		childErr := syscall.Kill(childPID, 0)
		groupErr := syscall.Kill(-processGroupID, 0)
		if childErr == syscall.ESRCH && groupErr == syscall.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Grok child process %d or process group %d survived context cancellation", childPID, processGroupID)
}
