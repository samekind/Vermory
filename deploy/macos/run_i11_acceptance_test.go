package macos_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestI11AcceptanceRunnerMatchesFrozenContract(t *testing.T) {
	script, err := os.ReadFile("run-i11-acceptance.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, required := range []string{
		"I11-macos-authenticated-service-lifecycle",
		"database compatibility --database-url",
		"candidate database compatibility preflight failed",
		"candidate activation failed; previous installation restored",
		"rollback-authenticated-user-service.sh",
		"/v1/integrations/openclaw/turns/prepare",
		"/v1/integrations/hermes/turns/prepare",
		"runtime_secret_matches",
		"checksum_bound_credential_free_report",
		"shasum -a 256 \"$report\"",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("I11 runner omitted %q", required)
		}
	}
	for _, forbidden := range []string{"sudo ", "/Library/LaunchDaemons", "Mac mini NewAPI", "VERMORY_PROVIDER='grok", "VERMORY_PROVIDER='openai"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("I11 runner contains forbidden behavior %q", forbidden)
		}
	}

	payload, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "I11-macos-authenticated-service-lifecycle", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		HardGateCount int      `json:"hard_gate_count"`
		HardGates     []string `json:"hard_gates"`
	}
	if err := json.Unmarshal(payload, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.HardGateCount != 20 || len(contract.HardGates) != contract.HardGateCount {
		t.Fatalf("I11 hard gates=%d declared=%d", len(contract.HardGates), contract.HardGateCount)
	}
}
