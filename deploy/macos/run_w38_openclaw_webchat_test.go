package macos_test

import (
	"os"
	"strings"
	"testing"
)

func TestRunW38OpenClawWebChatKeepsRuntimeExternalAndUnprivileged(t *testing.T) {
	payload, err := os.ReadFile("run-w38-openclaw-webchat.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, required := range []string{
		"/Volumes/JSData/",
		"OPENCLAW_STATE_DIR",
		"OPENCLAW_CONFIG_PATH",
		"OPENCLAW_BUNDLED_PLUGINS_DIR",
		"plugins inspect vermory --runtime --json",
		".plugin.status == \"loaded\"",
		"sessionMode:\"none\"",
		"grok-vermory-environment",
		"gateway run",
		"hold_runtime",
		"VERMORY_API_TOKEN",
		"VERMORY_OPERATOR_API_TOKEN",
		"schema_version == 25",
		"unlink \"$postgres_socket_link\"",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("W38 runner omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"sudo",
		"NewAPI",
		"newapi",
		"gateway install",
		"~/Library",
		"$HOME/Library",
		"set -x",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("W38 runner contains forbidden behavior %q", forbidden)
		}
	}
	info, err := os.Stat("run-w38-openclaw-webchat.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("W38 runner mode=%o want 755", info.Mode().Perm())
	}
}
