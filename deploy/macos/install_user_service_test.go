package macos_test

import (
	"os"
	"strings"
	"testing"
)

func TestInstallUserServiceSupportsIndependentUnprivilegedInstances(t *testing.T) {
	script, err := os.ReadFile("install-user-service.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, required := range []string{
		`VERMORY_INSTALL_BINARY`,
		`VERMORY_LOG_BASENAME`,
		`VERMORY_LAUNCHD_LABEL`,
		`VERMORY_TENANT_ID`,
		`VERMORY_LISTEN`,
		`"gui/$(id -u)"`,
		`VERMORY_INSTALL_BINARY must be inside HOME`,
		`invalid VERMORY_LAUNCHD_LABEL`,
		`VERMORY_INSTALL_BINARY must not contain dot path segments`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("user service installer omitted %q", required)
		}
	}
	for _, forbidden := range []string{"sudo ", "/Library/LaunchDaemons"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("user service installer contains privileged behavior %q", forbidden)
		}
	}
}
