package macos_test

import (
	"os"
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
}
