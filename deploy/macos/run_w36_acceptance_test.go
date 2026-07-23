package macos_test

import (
	"os"
	"strings"
	"testing"
)

func TestRunW36AcceptanceUsesRealServiceAndNoPrivilegedOrLocalHomeState(t *testing.T) {
	payload, err := os.ReadFile("run-w36-acceptance.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, required := range []string{
		"$binary\" serve",
		"/v1/client-operations/prepare",
		"/v1/client-operations/reclaim",
		"/v1/client-operations/checkpoint",
		"/v1/client-operations/complete",
		"/v1/client-operations/cancel",
		"stale heartbeat was not fenced",
		"stale completion was not fenced",
		"temporary PostgreSQL socket path is too long",
		"pg_dump",
		"pg_restore",
		"openclaw-terminal-after-restore.json",
		"restored_token_authenticated:true",
		"run-w36-openclaw-client.mjs",
		"/v1/integrations/hermes/turns/complete",
		"status:\"runtime-qualified\"",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("W36 runner omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"sudo",
		"NewAPI",
		"newapi",
		"~/.codex",
		"set -x",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("W36 runner contains forbidden behavior %q", forbidden)
		}
	}
	for _, name := range []string{"run-w36-acceptance.sh", "run-w36-openclaw-client.mjs"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode=%o want 755", name, info.Mode().Perm())
		}
	}
}
