package linux_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nativePackageCase struct {
	Version       string   `json:"version"`
	ID            string   `json:"id"`
	HardGateCount int      `json:"hard_gate_count"`
	HardGates     []string `json:"hard_gates"`
}

func TestLinuxServiceUnitKeepsRuntimeSecretsOutOfArguments(t *testing.T) {
	unit := readFile(t, "vermory.service")
	requireContains(t, unit,
		"User=@SERVICE_USER@",
		"Group=@SERVICE_GROUP@",
		"EnvironmentFile=@ENVIRONMENT_FILE@",
		"ExecStart=@INSTALL_ROOT@/current/vermory serve",
		"NoNewPrivileges=true",
		"PrivateTmp=true",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"RestrictSUIDSGID=true",
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6",
		"Restart=on-failure",
	)
	for _, forbidden := range []string{"postgresql://", "--database-url", "sudo ", "Environment=VERMORY_DATABASE_URL"} {
		if strings.Contains(unit, forbidden) {
			t.Fatalf("systemd unit contains forbidden value %q", forbidden)
		}
	}
}

func TestLinuxInstallerHasVersionedAtomicRollbackBoundary(t *testing.T) {
	script := readFile(t, "install-system-service.sh")
	requireContains(t, script,
		"effective UID 0",
		"sha256sum",
		"archive contains an unsafe path",
		"releases/$RELEASE_ID",
		"current",
		"previous",
		"useradd --system",
		"groupadd --system",
		"systemctl daemon-reload",
		"systemctl reset-failed",
		"automatic rollback restored",
		"VERMORY_HEALTH_ATTEMPTS",
	)
	if strings.Contains(script, "sudo ") {
		t.Fatal("installer must not invoke sudo internally")
	}
}

func TestLinuxRollbackRestoresStartingReleaseOnFailure(t *testing.T) {
	script := readFile(t, "rollback-system-service.sh")
	requireContains(t, script,
		"effective UID 0",
		"current",
		"previous",
		"rollback restored starting release",
		"systemctl restart",
		"systemctl reset-failed",
	)
	if strings.Contains(script, "sudo ") {
		t.Fatal("rollback must not invoke sudo internally")
	}
}

func TestLinuxBackupUsesPrivateNativePostgreSQLArtifact(t *testing.T) {
	script := readFile(t, "backup-postgresql.sh")
	requireContains(t, script,
		"umask 077",
		"service=$PG_SERVICE",
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"sha256sum",
		"schema_version",
		"LC_ALL=C pg_dump --version",
		`^pg_dump \(PostgreSQL\)`,
		`^[0-9]+([.][0-9]+)*$`,
		"refusing to overwrite backup",
	)
	for _, forbidden := range []string{"postgresql://", "DATABASE_URL", "sudo "} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("backup script contains forbidden value %q", forbidden)
		}
	}
}

func TestLinuxRestoreRejectsUnsafeTargetAndRebuildsRuntime(t *testing.T) {
	script := readFile(t, "restore-postgresql.sh")
	requireContains(t, script,
		"sha256sum --check",
		"target database is not empty",
		"pg_restore",
		"--exit-on-error",
		"--no-owner",
		"--no-acl",
		"database migrate",
		"database grant-runtime",
		"database rebuild-projections",
		"service=$TARGET_PG_SERVICE",
	)
	for _, forbidden := range []string{"postgresql://", "DATABASE_URL", "sudo "} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("restore script contains forbidden value %q", forbidden)
		}
	}
}

func TestLinuxDeploymentAssetsShipInEveryReleaseArchive(t *testing.T) {
	config := readFile(t, filepath.Join("..", "..", ".goreleaser.yaml"))
	requireContains(t, config,
		"src: deploy/linux/README.md",
		"src: deploy/linux/vermory.service",
		"src: deploy/linux/install-system-service.sh",
		"src: deploy/linux/rollback-system-service.sh",
		"src: deploy/linux/backup-postgresql.sh",
		"src: deploy/linux/restore-postgresql.sh",
	)
	for _, name := range []string{
		"install-system-service.sh",
		"rollback-system-service.sh",
		"backup-postgresql.sh",
		"restore-postgresql.sh",
	} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode=%#o want 0755", name, info.Mode().Perm())
		}
	}
}

func TestLinuxAcceptanceExercisesTheCompleteI05Boundary(t *testing.T) {
	script := readFile(t, "run-i05-acceptance.sh")
	requireContains(t, script,
		"SOURCE_SHA",
		"I05_QUALIFICATION",
		"I05_EXPECTED_MACHINE",
		"uname -m",
		"-buildvcs=false",
		"build_release i05-v1",
		"build_release i05-v2",
		"build_failing_release i05-failing",
		"automatic rollback restored i05-v2",
		"rollback_release",
		"backup-postgresql.sh",
		"restore-postgresql.sh",
		"tampered backup passed checksum verification",
		"restore accepted a non-empty target",
		"restored token or governed default probe failed",
		"unit_process_journal_secret_free",
		`chmod 0644 "$EVIDENCE_DIRECTORY/report.json"`,
		`chmod 0755 "$EVIDENCE_DIRECTORY"`,
	)
	if strings.Contains(script, `qualification: "github-hosted-ubuntu-systemd-amd64"`) {
		t.Fatal("acceptance report must use the runner-qualified architecture instead of a hard-coded AMD64 label")
	}
	if strings.Contains(script, "sudo ") {
		t.Fatal("acceptance script must execute inside one externally established root boundary")
	}
	if strings.Contains(script, `git -C "$REPOSITORY_ROOT"`) {
		t.Fatal("root acceptance must not refresh the repository index")
	}
	reportMode := strings.Index(script, `chmod 0644 "$EVIDENCE_DIRECTORY/report.json"`)
	credentialScan := strings.Index(script, `grep -Fq "$secret" "$EVIDENCE_DIRECTORY/report.json"`)
	directoryMode := strings.Index(script, `chmod 0755 "$EVIDENCE_DIRECTORY"`)
	if reportMode < 0 || credentialScan <= reportMode || directoryMode <= credentialScan {
		t.Fatal("normalized evidence must remain root-confined until its credential scan passes")
	}
}

func TestI06LinuxNativePackageCaseIsFrozen(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "I06-linux-native-packages", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest nativePackageCase
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1" || manifest.ID != "I06-linux-native-packages" ||
		manifest.HardGateCount != 16 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("unexpected I06 contract: %#v", manifest)
	}
	seen := make(map[string]struct{}, len(manifest.HardGates))
	for _, gate := range manifest.HardGates {
		if strings.TrimSpace(gate) == "" {
			t.Fatal("I06 hard gate cannot be empty")
		}
		if _, exists := seen[gate]; exists {
			t.Fatalf("I06 hard gate is duplicated: %q", gate)
		}
		seen[gate] = struct{}{}
	}
}

func TestLinuxNativePackagesKeepInstallationNonActivating(t *testing.T) {
	config := readFile(t, filepath.Join("..", "..", ".goreleaser.yaml"))
	requireContains(t, config,
		"nfpms:",
		"id: vermory-linux-packages",
		"package_name: vermory",
		"- deb",
		"- rpm",
		"src: deploy/linux/package/vermory.service",
		"dst: /usr/lib/systemd/system/vermory.service",
		"src: deploy/linux/package/vermory.env.example",
		"dst: /usr/share/vermory/vermory.env.example",
		"preinstall: deploy/linux/package/preinstall.sh",
		"postinstall: deploy/linux/package/postinstall.sh",
		"preremove: deploy/linux/package/preremove-deb.sh",
		"preremove: deploy/linux/package/preremove-rpm.sh",
		"postremove: deploy/linux/package/postremove.sh",
	)

	unit := readFile(t, filepath.Join("package", "vermory.service"))
	requireContains(t, unit,
		"ConditionPathExists=/etc/vermory/vermory.env",
		"User=vermory",
		"Group=vermory",
		"EnvironmentFile=/etc/vermory/vermory.env",
		"ExecStart=/usr/bin/vermory serve",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
	)

	for _, name := range []string{"preinstall.sh", "postinstall.sh", "preremove-deb.sh", "preremove-rpm.sh", "postremove.sh"} {
		script := readFile(t, filepath.Join("package", name))
		requireContains(t, script, "#!/bin/sh")
		for _, forbidden := range []string{"sudo ", "database migrate", "systemctl enable", "systemctl start", "systemctl restart"} {
			if strings.Contains(script, forbidden) {
				t.Fatalf("package script %s contains forbidden action %q", name, forbidden)
			}
		}
		for _, nonPOSIX := range []string{"[[", "pipefail", "${EUID", "<<<"} {
			if strings.Contains(script, nonPOSIX) {
				t.Fatalf("package script %s contains non-POSIX syntax %q", name, nonPOSIX)
			}
		}
	}

	postinstall := readFile(t, filepath.Join("package", "postinstall.sh"))
	requireContains(t, postinstall, "systemctl daemon-reload")
	for _, name := range []string{"preremove-deb.sh", "preremove-rpm.sh"} {
		requireContains(t, readFile(t, filepath.Join("package", name)), "systemctl disable --now vermory.service")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func requireContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Fatalf("required contract %q is missing", value)
		}
	}
}
