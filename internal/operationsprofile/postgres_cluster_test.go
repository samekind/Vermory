package operationsprofile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProfileConfigRequiresExplicitOptIn(t *testing.T) {
	for _, value := range []string{"", "true", "yes", "0"} {
		t.Run(value, func(t *testing.T) {
			_, err := loadProfileConfigFromEnv(func(key string) string {
				if key == "VERMORY_HA_PITR_PROFILE" {
					return value
				}
				return ""
			})
			if !errors.Is(err, errProfileDisabled) {
				t.Fatalf("opt-in %q returned %v", value, err)
			}
		})
	}
}

func TestLoadProfileConfigRequiresPostgreSQL18Tools(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	binDir := t.TempDir()
	writeVersionTool(t, binDir, "postgres", "postgres (PostgreSQL) 17.9")
	writeVersionTool(t, binDir, "pg_basebackup", "pg_basebackup (PostgreSQL) 18.4")

	_, err := loadProfileConfigFromEnv(profileTestEnv(root, binDir))
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL 18") {
		t.Fatalf("expected PostgreSQL 17 rejection, got %v", err)
	}

	writeVersionTool(t, binDir, "postgres", "postgres (PostgreSQL) 18.4")
	config, err := loadProfileConfigFromEnv(profileTestEnv(root, binDir))
	if err != nil {
		t.Fatal(err)
	}
	if config.Root != root || config.BinDir != binDir || config.PostgreSQLVersion != "18.4" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestClusterPathsRemainInsideDedicatedRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "profile")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureContainedPath(root, filepath.Join(root, "primary")); err != nil {
		t.Fatal(err)
	}
	if err := ensureContainedPath(root, filepath.Join(base, "outside")); err == nil || !strings.Contains(err.Error(), "outside dedicated root") {
		t.Fatalf("expected outside path rejection, got %v", err)
	}

	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureContainedPath(root, filepath.Join(link, "data")); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestRenderPrimaryConfigUsesLoopbackAndDedicatedArchive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	archive := filepath.Join(root, "wal-archive")
	config, err := renderPrimaryConfig(55432, archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"listen_addresses = '127.0.0.1'",
		"unix_socket_directories = ''",
		"port = 55432",
		"wal_level = replica",
		"max_wal_senders = 4",
		"max_replication_slots = 4",
		"archive_mode = on",
		archive,
		"test -f \"" + archive + "/%f\" || cp \"%p\" \"" + archive + "/%f\"",
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("primary config missing %q:\n%s", required, config)
		}
	}
	if strings.Contains(config, "listen_addresses = '*'") {
		t.Fatalf("primary config exposed non-loopback listener:\n%s", config)
	}
}

func TestRenderStandbyAndPITRConfigsContainExactRecoveryBoundary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	standby, err := renderStandbyConfig(55433)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"listen_addresses = '127.0.0.1'", "unix_socket_directories = ''", "port = 55433", "hot_standby = on"} {
		if !strings.Contains(standby, required) {
			t.Fatalf("standby config missing %q:\n%s", required, standby)
		}
	}

	pitr, err := renderPITRConfig(
		55434,
		filepath.Join(root, "wal-archive"),
		"0/30001A0",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"unix_socket_directories = ''",
		"recovery_target_lsn = '0/30001A0'",
		"recovery_target_timeline = 'current'",
		"recovery_target_inclusive = on",
		"recovery_target_action = promote",
		filepath.Join(root, "wal-archive"),
	} {
		if !strings.Contains(pitr, required) {
			t.Fatalf("PITR config missing %q:\n%s", required, pitr)
		}
	}
}

func TestPostgreSQLClusterHarnessSmoke(t *testing.T) {
	config, err := loadProfileConfigFromEnv(os.Getenv)
	if errors.Is(err, errProfileDisabled) {
		t.Skip("VERMORY_HA_PITR_PROFILE=1 is required")
	}
	if err != nil {
		t.Fatal(err)
	}
	harness := newClusterHarness(t, config)
	harness.runReplicationSmoke(t)
}

func profileTestEnv(root, binDir string) func(string) string {
	values := map[string]string{
		"VERMORY_HA_PITR_PROFILE":  "1",
		"VERMORY_POSTGRES_BIN_DIR": binDir,
		"VERMORY_HA_PITR_ROOT":     root,
		"VERMORY_HA_PITR_RUN_ID":   "profile-test",
	}
	return func(key string) string { return values[key] }
}

func writeVersionTool(t *testing.T, binDir, name, output string) {
	t.Helper()
	path := filepath.Join(binDir, name)
	content := "#!/bin/sh\nprintf '%s\\n' '" + strings.ReplaceAll(output, "'", "'\\''") + "'\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}
