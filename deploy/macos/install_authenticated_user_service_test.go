package macos_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuthenticatedUserServiceIsUnprivilegedAndKeepsSecretsOutOfPlist(t *testing.T) {
	installer, err := os.ReadFile("install-authenticated-user-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := os.ReadFile("run-authenticated-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(installer)
	for _, required := range []string{
		"org.vermory.authenticated-web-chat",
		"run-authenticated-service.sh",
		"authenticated service environment must have mode 600",
		"VERMORY_LISTEN must use a loopback address",
		`"gui/$(id -u)"`,
		"/v1/session",
		`session_code" = "401"`,
		`VERSION=$("$INSTALL_BINARY" version`,
		`database compatibility --database-url`,
		`VERMORY_ROLLBACK_DIR`,
		`VERMORY_ROLLBACK_DIR must not contain live service files`,
		`restore_previous_installation`,
		`candidate activation failed; previous installation restored`,
		`candidate activation failed; incomplete first installation removed`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("authenticated installer omitted %q", required)
		}
	}
	for _, forbidden := range []string{"sudo ", "/Library/LaunchDaemons", "database migrate", "grant-runtime", "VERMORY_DATABASE_URL</string>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("authenticated installer contains forbidden behavior %q", forbidden)
		}
	}
	for _, required := range []string{"exec \"$BINARY\" serve", "set -a", `"$(/usr/bin/stat -f '%Lp' "$ENV_FILE")" != "600"`} {
		if !strings.Contains(string(runner), required) {
			t.Fatalf("authenticated runner omitted %q", required)
		}
	}

	rollback, err := os.ReadFile("rollback-authenticated-user-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	rollbackText := string(rollback)
	for _, required := range []string{
		`database compatibility --database-url`,
		`VERMORY_ROLLBACK_DIR`,
		`VERMORY_ROLLBACK_DIR must not contain live service files`,
		`rollback slot is incomplete`,
		`service=$LABEL state=rolled-back`,
	} {
		if !strings.Contains(rollbackText, required) {
			t.Fatalf("authenticated rollback omitted %q", required)
		}
	}
	for _, forbidden := range []string{"sudo ", "/Library/LaunchDaemons", "database migrate", "grant-runtime"} {
		if strings.Contains(rollbackText, forbidden) {
			t.Fatalf("authenticated rollback contains forbidden behavior %q", forbidden)
		}
	}
}

func TestAuthenticatedUserServiceUpgradeRollbackAndAutomaticRestore(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS lifecycle requires macOS paths and plist validation")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	appDir := filepath.Join(home, "Library", "Application Support", "Vermory", "Authenticated")
	installBinary := filepath.Join(appDir, "bin", "vermory")
	environmentPath := filepath.Join(root, "vermory-authenticated.env")
	launchctlPath := filepath.Join(root, "launchctl")
	curlPath := filepath.Join(root, "curl")
	sleepPath := filepath.Join(root, "sleep")
	for _, directory := range []string{home, filepath.Dir(environmentPath)} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(environmentPath, []byte("VERMORY_DATABASE_URL='postgresql://fixture'\nVERMORY_LISTEN='127.0.0.1:18787'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, launchctlPath, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, sleepPath, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, curlPath, `#!/bin/sh
set -eu
url=
for argument in "$@"; do
  url=$argument
done
version=$("$FAKE_INSTALL_BINARY" version)
if [ "$version" = "unhealthy" ]; then
  printf '503'
  exit 0
fi
case "$url" in
  */v1/session) printf '401' ;;
  *) printf '200' ;;
esac
`)

	baseBinary := writeFakeVermoryBinary(t, root, "base", true)
	candidateBinary := writeFakeVermoryBinary(t, root, "candidate", true)
	incompatibleBinary := writeFakeVermoryBinary(t, root, "incompatible", false)
	unhealthyBinary := writeFakeVermoryBinary(t, root, "unhealthy", true)

	commonEnvironment := append(os.Environ(),
		"HOME="+home,
		"VERMORY_APP_DIR="+appDir,
		"LAUNCHCTL="+launchctlPath,
		"CURL="+curlPath,
		"SLEEP="+sleepPath,
		"VERMORY_HEALTH_ATTEMPTS=1",
		"VERMORY_HEALTH_SLEEP_SECONDS=0",
		"FAKE_INSTALL_BINARY="+installBinary,
	)
	runInstaller := func(binary string, wantFailure bool) string {
		t.Helper()
		command := exec.Command("/bin/sh", "install-authenticated-user-service.sh", binary, environmentPath)
		command.Env = commonEnvironment
		output, err := command.CombinedOutput()
		if wantFailure && err == nil {
			t.Fatalf("installer unexpectedly succeeded:\n%s", output)
		}
		if !wantFailure && err != nil {
			t.Fatalf("installer failed: %v\n%s", err, output)
		}
		return string(output)
	}
	installedVersion := func() string {
		t.Helper()
		output, err := exec.Command(installBinary, "version").Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(output))
	}

	runInstaller(baseBinary, false)
	if got := installedVersion(); got != "base" {
		t.Fatalf("first install version=%q want base", got)
	}

	output := runInstaller(incompatibleBinary, true)
	if !strings.Contains(output, "candidate database compatibility preflight failed") {
		t.Fatalf("incompatible candidate failure was not attributed:\n%s", output)
	}
	if got := installedVersion(); got != "base" {
		t.Fatalf("incompatible candidate changed live version=%q", got)
	}

	runInstaller(candidateBinary, false)
	if got := installedVersion(); got != "candidate" {
		t.Fatalf("upgrade version=%q want candidate", got)
	}
	rollbackBinary := filepath.Join(appDir, "rollback", "vermory")
	if got := commandOutput(t, rollbackBinary, "version"); got != "base" {
		t.Fatalf("rollback slot version=%q want base", got)
	}

	rollback := exec.Command("/bin/sh", "rollback-authenticated-user-service.sh")
	rollback.Env = commonEnvironment
	if output, err := rollback.CombinedOutput(); err != nil {
		t.Fatalf("explicit rollback failed: %v\n%s", err, output)
	}
	if got := installedVersion(); got != "base" {
		t.Fatalf("explicit rollback version=%q want base", got)
	}
	if _, err := os.Stat(filepath.Join(appDir, "rollback")); !os.IsNotExist(err) {
		t.Fatalf("successful rollback did not consume slot: %v", err)
	}

	runInstaller(candidateBinary, false)
	output = runInstaller(unhealthyBinary, true)
	if !strings.Contains(output, "candidate activation failed; previous installation restored") {
		t.Fatalf("failed activation did not report automatic restoration:\n%s", output)
	}
	if got := installedVersion(); got != "candidate" {
		t.Fatalf("automatic restoration version=%q want candidate", got)
	}
	if got := commandOutput(t, rollbackBinary, "version"); got != "candidate" {
		t.Fatalf("automatic restoration rollback slot version=%q want candidate", got)
	}
}

func writeFakeVermoryBinary(t *testing.T, root, version string, compatible bool) string {
	t.Helper()
	path := filepath.Join(root, "vermory-"+version)
	compatibilityExit := "0"
	if !compatible {
		compatibilityExit = "1"
	}
	payload := "#!/bin/sh\nset -eu\ncase \"${1:-}\" in\n  version) printf '%s\\n' '" + version + "' ;;\n  database) exit " + compatibilityExit + " ;;\n  *) exit 64 ;;\nesac\n"
	writeExecutable(t, path, payload)
	return path
}

func writeExecutable(t *testing.T, path, payload string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(payload), 0o755); err != nil {
		t.Fatal(err)
	}
}

func commandOutput(t *testing.T, path string, arguments ...string) string {
	t.Helper()
	output, err := exec.Command(path, arguments...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}
