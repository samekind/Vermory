package macos_test

import (
	"os"
	"strings"
	"testing"
)

func TestRunW19FormalUsesKeychainAndExactHead(t *testing.T) {
	payload, err := os.ReadFile("run-w19-formal.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, required := range []string{
		"security find-generic-password -a \"$user_name\" -s vermory-siliconflow -w",
		"git -C \"$repo_root\" status --porcelain",
		"VERMORY_W19_FORMAL_PROFILE=1",
		"VERMORY_IMPLEMENTATION_REVISION=\"$revision\"",
		"-run TestMemoryEligibilityFormalProfile -v -timeout 60m",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("formal runner omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"set -x",
		"NewAPI",
		"newapi",
		"-w \"$api_key\"",
		"printf \"%s\\n\" \"$api_key\"",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("formal runner contains unsafe behavior %q", forbidden)
		}
	}
	info, err := os.Stat("run-w19-formal.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("formal runner mode=%o want 755", info.Mode().Perm())
	}
}
