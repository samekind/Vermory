package macos_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallOpenClawServicePreservesGeneratedAuthAndExistingConfig(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS installer requires launchd paths")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	pluginDir := filepath.Join(root, "plugin")
	cliPath := filepath.Join(pluginDir, "node_modules", ".bin", "openclaw")
	lsofPath := filepath.Join(root, "lsof")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "dist", "index.js"), []byte("export default {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte(fakeOpenClawCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lsofPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	appDir := filepath.Join(home, "Library", "Application Support", "Vermory")
	grokWrapper := filepath.Join(appDir, "grok", "grok-vermory-isolated")
	if err := os.MkdirAll(filepath.Dir(grokWrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grokWrapper, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(appDir, "openclaw", "config", "openclaw.json")
	installer := filepath.Join("install-openclaw-service.sh")
	for run := 1; run <= 2; run++ {
		command := exec.Command("/bin/sh", installer, pluginDir)
		command.Env = append(os.Environ(),
			"HOME="+home,
			"VERMORY_APP_DIR="+appDir,
			"VERMORY_OPENCLAW_BASE_URL=http://127.0.0.1:8793",
			`VERMORY_OPENCLAW_TOOL_ALLOWLIST_JSON=["device.storage_check","device.remove_bundle"]`,
			"FAKE_OPENCLAW_ROOT="+root,
			"LSOF="+lsofPath,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("installer run %d failed: %v\n%s", run, err, output)
		}
	}
	for _, marker := range []string{"gateway-stop-called", "gateway-start-called"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err != nil {
			t.Fatalf("installer did not complete the explicit gateway stop/start lifecycle: %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if got := nestedString(config, "gateway", "auth", "token"); got != "generated-token-1" {
		t.Fatalf("gateway token changed across reinstall: %q", got)
	}
	if got := nestedString(config, "channels", "fixture", "marker"); got != "preserve-me" {
		t.Fatalf("unrelated config was overwritten: %q", got)
	}
	if got := nestedString(config, "agents", "defaults", "cliBackends", "fixture-cli", "command"); got != "/fixture/cli" {
		t.Fatalf("CLI backend was overwritten: %q", got)
	}
	if got := nestedString(config, "agents", "defaults", "cliBackends", "grok-cli", "command"); got != grokWrapper {
		t.Fatalf("Grok CLI backend was not registered: %q", got)
	}
	if got := nestedString(config, "agents", "defaults", "cliBackends", "grok-cli", "sessionMode"); got != "none" {
		t.Fatalf("Grok CLI backend session mode = %q, want none", got)
	}
	if got := nestedString(config, "plugins", "entries", "fixture-plugin", "marker"); got != "preserve-plugin" {
		t.Fatalf("unrelated plugin config was overwritten: %q", got)
	}
	if got := nestedString(config, "plugins", "entries", "vermory", "config", "baseUrl"); got != "http://127.0.0.1:8793" {
		t.Fatalf("Vermory plugin config missing: %q", got)
	}
	allowlist := nestedStrings(config, "plugins", "entries", "vermory", "config", "toolAllowlist")
	if strings.Join(allowlist, ",") != "device.storage_check,device.remove_bundle" {
		t.Fatalf("Vermory tool allowlist mismatch: %#v", allowlist)
	}
	if got := nestedNumber(config, "plugins", "entries", "vermory", "hooks", "timeouts", "after_tool_call"); got != 15000 {
		t.Fatalf("after_tool_call timeout = %v, want 15000", got)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestInstallGrokRuntimeUsesIsolatedStateAndPreservesRefreshedAuth(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS installer requires macOS paths")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	appDir := filepath.Join(home, "Library", "Application Support", "Vermory")
	sourceBinary := filepath.Join(root, "grok")
	sourceAuth := filepath.Join(root, "auth.json")
	capture := filepath.Join(root, "capture.json")
	if err := os.WriteFile(sourceBinary, []byte(fakeGrokBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeAuth(t, sourceAuth, "source-one")

	installer := filepath.Join("install-grok-runtime.sh")
	runInstaller := func() string {
		t.Helper()
		command := exec.Command("/bin/sh", installer, sourceBinary, sourceAuth)
		command.Env = append(os.Environ(),
			"HOME="+home,
			"VERMORY_APP_DIR="+appDir,
			"VERMORY_GROK_REQUIRE_SIGNATURE=0",
			"VERMORY_GROK_PROXY_URL=http://127.0.0.1:6152",
			"FAKE_GROK_CAPTURE="+capture,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("install Grok runtime: %v\n%s", err, output)
		} else {
			return string(output)
		}
		return ""
	}
	if output := runInstaller(); !strings.Contains(output, "version=grok test") {
		t.Fatalf("installer did not report the installed Grok version: %q", output)
	}

	installedAuth := filepath.Join(appDir, "grok", "state", "auth.json")
	writeFakeAuth(t, installedAuth, "remote-refresh")
	writeFakeAuth(t, sourceAuth, "source-two")
	runInstaller()
	if got := fakeAuthToken(t, installedAuth); got != "remote-refresh" {
		t.Fatalf("reinstall overwrote refreshed auth: %q", got)
	}

	wrapper := filepath.Join(appDir, "grok", "grok-vermory-isolated")
	command := exec.Command(wrapper, "--output-format", "json", "--single", "fixture prompt", "--model", "grok-4.5")
	command.Env = append(os.Environ(), "FAKE_GROK_CAPTURE="+capture)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run Grok wrapper: %v\n%s", err, output)
	}

	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Home       string   `json:"home"`
		GrokHome   string   `json:"grok_home"`
		Memory     string   `json:"memory"`
		Subagents  string   `json:"subagents"`
		WebFetch   string   `json:"web_fetch"`
		Feedback   string   `json:"feedback"`
		HTTPSProxy string   `json:"https_proxy"`
		Arguments  []string `json:"arguments"`
	}
	if err := json.Unmarshal(data, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Home != filepath.Join(appDir, "grok", "home") || captured.GrokHome != filepath.Join(appDir, "grok", "state") {
		t.Fatalf("wrapper did not isolate Grok state: %#v", captured)
	}
	if captured.Memory != "0" || captured.Subagents != "0" || captured.WebFetch != "0" || captured.Feedback != "0" {
		t.Fatalf("wrapper did not disable ambient Grok capabilities: %#v", captured)
	}
	if captured.HTTPSProxy != "http://127.0.0.1:6152" {
		t.Fatalf("wrapper did not apply the loopback proxy: %#v", captured)
	}
	for _, required := range []string{"--no-memory", "--no-plan", "--no-subagents", "--disable-web-search", "--permission-mode", "dontAsk", "--single", "fixture prompt"} {
		if !containsString(captured.Arguments, required) {
			t.Fatalf("wrapper arguments omitted %q: %#v", required, captured.Arguments)
		}
	}
	if countString(captured.Arguments, "--verbatim") != 1 {
		t.Fatalf("isolated wrapper duplicated --verbatim: %#v", captured.Arguments)
	}
	info, err := os.Stat(installedAuth)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("installed auth mode = %o, want 600", info.Mode().Perm())
	}
}

func writeFakeAuth(t *testing.T, path, token string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"https://auth.example::client": map[string]any{
			"key":           token,
			"refresh_token": "refresh-" + token,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fakeAuthToken(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var auth map[string]map[string]any
	if err := json.Unmarshal(data, &auth); err != nil {
		t.Fatal(err)
	}
	for _, entry := range auth {
		value, _ := entry["key"].(string)
		return value
	}
	return ""
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func countString(values []string, expected string) int {
	count := 0
	for _, value := range values {
		if value == expected {
			count++
		}
	}
	return count
}

func nestedString(value map[string]any, path ...string) string {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	text, _ := current.(string)
	return text
}

func nestedStrings(value map[string]any, path ...string) []string {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	values, ok := current.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil
		}
		result = append(result, text)
	}
	return result
}

func nestedNumber(value map[string]any, path ...string) float64 {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return 0
		}
		current = object[key]
	}
	number, _ := current.(float64)
	return number
}

const fakeOpenClawCLI = `#!/bin/sh
set -eu

CONFIG=${OPENCLAW_CONFIG_PATH:?}
ROOT=${FAKE_OPENCLAW_ROOT:?}

case "$*" in
  "plugins inspect vermory --json")
    test -f "$ROOT/plugin-installed"
    ;;
  "plugins install --link "*)
    /usr/bin/touch "$ROOT/plugin-installed"
    ;;
  "config validate")
    /usr/bin/jq empty "$CONFIG"
    ;;
  "plugins inspect vermory --runtime --json")
    /usr/bin/printf '{"id":"vermory","status":"loaded"}\n'
    ;;
  "gateway install --force --runtime node --port "*)
    count=0
    if test -f "$ROOT/install-count"; then
      count=$(/bin/cat "$ROOT/install-count")
    fi
    count=$((count + 1))
    /usr/bin/printf '%s\n' "$count" > "$ROOT/install-count"
    temp="$CONFIG.fake-new"
    /usr/bin/jq --arg token "generated-token-$count" \
      '.channels.fixture.marker = "preserve-me"
       | .agents.defaults.cliBackends."fixture-cli".command = "/fixture/cli"
       | .plugins.entries."fixture-plugin".marker = "preserve-plugin"
       | if .gateway.auth.token then . else .gateway.auth = {mode:"token", token:$token} end' \
      "$CONFIG" > "$temp"
    /bin/mv "$temp" "$CONFIG"
    ;;
	"gateway stop --json")
		/usr/bin/touch "$ROOT/gateway-stop-called"
		;;
	"gateway start --json")
		test -f "$ROOT/gateway-stop-called"
		/usr/bin/touch "$ROOT/gateway-start-called"
		;;
	"gateway restart"*)
		/usr/bin/printf 'gateway restart must not be used during installation\n' >&2
		exit 65
		;;
  "gateway health --json")
    /usr/bin/printf '{"ok":true}\n'
    ;;
  *)
    /usr/bin/printf 'unexpected fake OpenClaw arguments: %s\n' "$*" >&2
    exit 64
    ;;
esac
`

const fakeGrokBinary = `#!/bin/sh
set -eu

for argument in "$@"; do
  if test "$argument" = "--version"; then
    echo "grok test"
    exit 0
  fi
done

/usr/bin/jq -n \
  --arg home "$HOME" \
  --arg grok_home "$GROK_HOME" \
  --arg memory "$GROK_MEMORY" \
  --arg subagents "$GROK_SUBAGENTS" \
  --arg web_fetch "$GROK_WEB_FETCH" \
  --arg feedback "$GROK_FEEDBACK_ENABLED" \
  --arg https_proxy "${HTTPS_PROXY:-}" \
  --args \
  '{home:$home,grok_home:$grok_home,memory:$memory,subagents:$subagents,web_fetch:$web_fetch,feedback:$feedback,https_proxy:$https_proxy,arguments:$ARGS.positional}' \
  -- "$@" \
  > "$FAKE_GROK_CAPTURE"
echo '{"text":"ok"}'
`
