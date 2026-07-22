package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const qualifiedPackageSource = "0123456789abcdef0123456789abcdef01234567"

type qualifiedPackageFixture struct {
	format  string
	arch    string
	machine string
	name    string
}

func TestAssembleQualifiedPackagesUsesAcceptedBytesAndRebuildsChecksums(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	evidence := writeQualifiedPackageEvidence(t)
	runQualifiedPackageAssembler(t, evidence, dist, true)

	for _, fixture := range qualifiedPackageFixtures() {
		payload, err := os.ReadFile(filepath.Join(dist, fixture.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(payload) != "qualified:"+fixture.name+"\n" {
			t.Fatalf("%s does not contain the accepted package bytes", fixture.name)
		}
	}

	checksums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(checksums)), "\n"); len(lines) != 8 {
		t.Fatalf("checksums entries=%d, want 8: %s", len(lines), checksums)
	}
	runReleaseManifestScript(t, "create", dist, true)
	runReleaseManifestScript(t, "verify", dist, true)
}

func TestAssembleQualifiedPackagesRejectsBytesThatDoNotMatchAcceptanceReport(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	evidence := writeQualifiedPackageEvidence(t)
	fixture := qualifiedPackageFixtures()[3]
	path := filepath.Join(
		evidence,
		"vermory-i06-linux-package-"+fixture.format+"-"+fixture.arch+"-"+qualifiedPackageSource,
		fixture.name,
	)
	if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runQualifiedPackageAssembler(t, evidence, dist, false)
}

func writeQualifiedPackageEvidence(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, fixture := range qualifiedPackageFixtures() {
		directory := filepath.Join(
			root,
			"vermory-i06-linux-package-"+fixture.format+"-"+fixture.arch+"-"+qualifiedPackageSource,
		)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		payload := []byte("qualified:" + fixture.name + "\n")
		if err := os.WriteFile(filepath.Join(directory, fixture.name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		report := map[string]any{
			"version":    "1",
			"case_id":    "I06-linux-native-packages",
			"source_sha": qualifiedPackageSource,
			"package": map[string]any{
				"format":       fixture.format,
				"architecture": fixture.arch,
				"machine":      fixture.machine,
				"file":         fixture.name,
				"sha256":       hex.EncodeToString(digest[:]),
				"native":       true,
			},
			"hard_gates": qualifiedPackageHardGates(),
		}
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "report.json"), append(encoded, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func qualifiedPackageFixtures() []qualifiedPackageFixture {
	return []qualifiedPackageFixture{
		{format: "deb", arch: "amd64", machine: "x86_64", name: "vermory_0.0.0_linux_amd64.deb"},
		{format: "rpm", arch: "amd64", machine: "x86_64", name: "vermory_0.0.0_linux_amd64.rpm"},
		{format: "deb", arch: "arm64", machine: "aarch64", name: "vermory_0.0.0_linux_arm64.deb"},
		{format: "rpm", arch: "arm64", machine: "aarch64", name: "vermory_0.0.0_linux_arm64.rpm"},
	}
}

func qualifiedPackageHardGates() map[string]bool {
	return map[string]bool{
		"exact_source_head":            true,
		"expected_format":              true,
		"native_architecture":          true,
		"executable_binary":            true,
		"restricted_service_identity":  true,
		"hardened_systemd_unit":        true,
		"environment_example":          true,
		"protected_environment_absent": true,
		"service_not_activated":        true,
		"scripts_without_sudo":         true,
		"scripts_without_migrations":   true,
		"scripts_without_credentials":  true,
		"true_removal_guard":           true,
		"package_owned_files_removed":  true,
		"operator_state_preserved":     true,
		"report_credential_free":       true,
	}
}

func runQualifiedPackageAssembler(t *testing.T, evidence, dist string, wantSuccess bool) {
	t.Helper()
	script := filepath.Join("..", "..", "scripts", "assemble-qualified-packages.sh")
	command := exec.Command("bash", script, evidence, dist, qualifiedPackageSource)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("qualified package assembly failed: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("qualified package assembly unexpectedly passed:\n%s", output)
	}
}
