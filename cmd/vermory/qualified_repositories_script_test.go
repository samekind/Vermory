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

const qualifiedRepositorySource = "89abcdef0123456789abcdef0123456789abcdef"

type qualifiedRepositoryFixture struct {
	kind    string
	format  string
	arch    string
	machine string
}

func TestI08CaseFreezesRepositoryQualificationBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "I08-linux-package-repository", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var specification struct {
		Version                 string   `json:"version"`
		ID                      string   `json:"id"`
		HardGateCount           int      `json:"hard_gate_count"`
		HardGates               []string `json:"hard_gates"`
		QualificationBoundaries []string `json:"qualification_boundaries"`
	}
	if err := json.Unmarshal(data, &specification); err != nil {
		t.Fatal(err)
	}
	if specification.Version != "1" || specification.ID != "I08-linux-package-repository" {
		t.Fatalf("unexpected I08 identity: %+v", specification)
	}
	if specification.HardGateCount != 18 || len(specification.HardGates) != 18 {
		t.Fatalf("I08 hard gates=%d/%d, want 18/18", specification.HardGateCount, len(specification.HardGates))
	}
	if len(specification.QualificationBoundaries) != 4 {
		t.Fatalf("I08 qualification boundaries=%d, want 4", len(specification.QualificationBoundaries))
	}
	joined := strings.Join(append(specification.HardGates, specification.QualificationBoundaries...), "\n")
	for _, required := range []string{
		"exact package bytes accepted by I06",
		"native package manager",
		"tampered repository metadata",
		"no private signing key material",
		"ephemeral qualification material",
		"no stable production repository signing key",
		"no public or long-lived hosted repository",
		"RPM package payload signatures are not qualified",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("I08 contract is missing %q", required)
		}
	}
}

func TestAssembleQualifiedRepositoriesUsesAcceptedBundlesAndExtendsReleaseManifest(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	evidence := writeQualifiedRepositoryEvidence(t)
	runQualifiedRepositoryAssembler(t, evidence, dist, true)

	for _, fixture := range qualifiedRepositoryFixtures() {
		name := qualifiedRepositoryBundleName(fixture)
		payload, err := os.ReadFile(filepath.Join(dist, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(payload) != "qualified:"+name+"\n" {
			t.Fatalf("%s does not contain accepted repository bytes", name)
		}
	}

	runReleaseManifestScript(t, "create", dist, true)
	manifest, err := os.ReadFile(filepath.Join(dist, "release-manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(manifest)), "\n"); len(lines) != 16 {
		t.Fatalf("manifest entries=%d, want 16: %s", len(lines), manifest)
	}
	runReleaseManifestScript(t, "verify", dist, true)
}

func TestAssembleQualifiedRepositoriesRejectsModifiedAcceptedBundle(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	evidence := writeQualifiedRepositoryEvidence(t)
	fixture := qualifiedRepositoryFixtures()[2]
	path := filepath.Join(
		evidence,
		"vermory-i08-linux-repository-"+fixture.kind+"-"+fixture.arch+"-"+qualifiedRepositorySource,
		qualifiedRepositoryBundleName(fixture),
	)
	if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runQualifiedRepositoryAssembler(t, evidence, dist, false)
}

func TestLinuxRepositoryScriptsRequireNativeSignedRepositorySemantics(t *testing.T) {
	builder := readRepositoryScript(t, "scripts", "build-linux-repository.sh")
	acceptance := readRepositoryScript(t, "deploy", "linux", "run-i08-repository-acceptance.sh")
	for _, required := range []string{
		"dpkg-scanpackages",
		"apt-ftparchive",
		"--clearsign",
		"createrepo_c",
		"--compress-type gz",
		"repomd.xml.asc",
		"I06-linux-native-packages",
		"ephemeral-ci-qualification",
	} {
		if !strings.Contains(builder, required) {
			t.Fatalf("repository builder is missing %q", required)
		}
	}
	for _, required := range []string{
		"signed-by=",
		"repo_gpgcheck=1",
		"file://",
		"download vermory",
		"tampered",
		"installed_revision_bound",
		"private_signing_key_absent",
	} {
		if !strings.Contains(acceptance, required) {
			t.Fatalf("repository acceptance is missing %q", required)
		}
	}
	if strings.Contains(builder, "sudo ") || strings.Contains(acceptance, "sudo ") {
		t.Fatal("I08 scripts must not invoke sudo internally")
	}
}

func writeQualifiedRepositoryEvidence(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, fixture := range qualifiedRepositoryFixtures() {
		directory := filepath.Join(
			root,
			"vermory-i08-linux-repository-"+fixture.kind+"-"+fixture.arch+"-"+qualifiedRepositorySource,
		)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		name := qualifiedRepositoryBundleName(fixture)
		payload := []byte("qualified:" + name + "\n")
		if err := os.WriteFile(filepath.Join(directory, name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		report := map[string]any{
			"version":    "1",
			"case_id":    "I08-linux-package-repository",
			"source_sha": qualifiedRepositorySource,
			"repository": map[string]any{
				"kind":                        fixture.kind,
				"package_format":              fixture.format,
				"architecture":                fixture.arch,
				"machine":                     fixture.machine,
				"native":                      true,
				"package_manager":             fixture.kind,
				"transport":                   "file://",
				"metadata_signature_enforced": true,
				"key_scope":                   "ephemeral-ci-qualification",
			},
			"bundle": map[string]any{
				"file":   name,
				"sha256": hex.EncodeToString(digest[:]),
			},
			"hard_gates": qualifiedRepositoryHardGates(),
			"qualification_boundaries": map[string]bool{
				"stable_production_signing_key": false,
				"public_hosted_repository":      false,
				"cross_version_lifecycle":       false,
				"rpm_payload_signature":         false,
			},
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

func qualifiedRepositoryFixtures() []qualifiedRepositoryFixture {
	return []qualifiedRepositoryFixture{
		{kind: "apt", format: "deb", arch: "amd64", machine: "x86_64"},
		{kind: "dnf", format: "rpm", arch: "amd64", machine: "x86_64"},
		{kind: "apt", format: "deb", arch: "arm64", machine: "aarch64"},
		{kind: "dnf", format: "rpm", arch: "arm64", machine: "aarch64"},
	}
}

func qualifiedRepositoryBundleName(fixture qualifiedRepositoryFixture) string {
	return "vermory-repository-" + fixture.kind + "-" + fixture.arch + "-" + qualifiedRepositorySource + ".tar.gz"
}

func qualifiedRepositoryHardGates() map[string]bool {
	return map[string]bool{
		"exact_source_head":               true,
		"exact_i06_package_bytes":         true,
		"expected_repository_kind":        true,
		"native_architecture":             true,
		"native_package_manager":          true,
		"file_repository_only":            true,
		"native_repository_metadata":      true,
		"metadata_package_digest_bound":   true,
		"repository_signature_valid":      true,
		"client_signature_enforced":       true,
		"downloaded_package_digest_bound": true,
		"installed_revision_bound":        true,
		"service_not_activated":           true,
		"tampered_metadata_rejected":      true,
		"private_signing_key_absent":      true,
		"ephemeral_key_scope_declared":    true,
		"repository_bundle_hashed":        true,
		"report_credential_free":          true,
	}
}

func readRepositoryScript(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func runQualifiedRepositoryAssembler(t *testing.T, evidence, dist string, wantSuccess bool) {
	t.Helper()
	script := filepath.Join("..", "..", "scripts", "assemble-qualified-repositories.sh")
	command := exec.Command("bash", script, evidence, dist, qualifiedRepositorySource)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("qualified repository assembly failed: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("qualified repository assembly unexpectedly passed:\n%s", output)
	}
}
