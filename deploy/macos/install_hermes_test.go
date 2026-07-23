package macos_test

import (
	"os"
	"strings"
	"testing"
)

func TestInstallHermesIsPinnedAndUnprivileged(t *testing.T) {
	payload, err := os.ReadFile("install-hermes.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, required := range []string{
		"VERMORY_HERMES_COMMIT",
		"VERMORY_HERMES_INSTALLER_SHA256",
		"UV_CACHE_DIR=\"$root/cache/uv\"",
		"UV_PYTHON_INSTALL_DIR=\"$root/python\"",
		"npm_config_cache=\"$root/cache/npm\"",
		"https://hermes-agent.nousresearch.com/install.sh",
		"Hermes installer checksum mismatch",
		"--commit \"$commit\"",
		"for stage in repository venv python-deps path config complete",
		"--stage \"$stage\"",
		"--skip-setup",
		"--skip-browser",
		"--no-skills",
		"git -C \"$install_dir\" rev-parse HEAD",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("Hermes installer omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"sudo ",
		"node-deps",
		"curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash",
		"git checkout --",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("Hermes installer contains unsafe behavior %q", forbidden)
		}
	}
	info, err := os.Stat("install-hermes.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("Hermes installer mode=%o want 755", info.Mode().Perm())
	}
}
