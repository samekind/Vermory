package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestI09CaseFreezesRepositoryLifecycleBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "runtime", "cases", "I09-linux-repository-lifecycle", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var specification struct {
		Version                       string   `json:"version"`
		ID                            string   `json:"id"`
		BaseSourceSHA                 string   `json:"base_source_sha"`
		BaseQualificationVersion      string   `json:"base_qualification_version"`
		CandidateQualificationVersion string   `json:"candidate_qualification_version"`
		HardGateCount                 int      `json:"hard_gate_count"`
		HardGates                     []string `json:"hard_gates"`
		QualificationBoundaries       []string `json:"qualification_boundaries"`
	}
	if err := json.Unmarshal(data, &specification); err != nil {
		t.Fatal(err)
	}
	if specification.Version != "1" || specification.ID != "I09-linux-repository-lifecycle" {
		t.Fatalf("unexpected I09 identity: %+v", specification)
	}
	if len(specification.BaseSourceSHA) != 40 || specification.BaseQualificationVersion == "" || specification.CandidateQualificationVersion == "" {
		t.Fatalf("I09 version anchors are incomplete: %+v", specification)
	}
	if specification.HardGateCount != 26 || len(specification.HardGates) != 26 {
		t.Fatalf("I09 hard gates=%d/%d, want 26/26", specification.HardGateCount, len(specification.HardGates))
	}
	if len(specification.QualificationBoundaries) != 5 {
		t.Fatalf("I09 qualification boundaries=%d, want 5", len(specification.QualificationBoundaries))
	}
	joined := strings.Join(append(specification.HardGates, specification.QualificationBoundaries...), "\n")
	for _, required := range []string{
		"exact pull-request head",
		"frozen accepted base source revision",
		"qualification-only semantic versions",
		"normal package-manager upgrade",
		"explicit package-manager rollback",
		"preserves operator configuration and the service identity",
		"same ephemeral qualification key",
		"no stable production repository signing key",
		"no database schema migration",
		"RPM package payload signatures are not qualified",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("I09 contract is missing %q", required)
		}
	}
}

func TestI09SnapshotVersionOverridePreservesOrdinarySnapshotDefault(t *testing.T) {
	configuration := readRepositoryScript(t, ".goreleaser.yaml")
	for _, required := range []string{
		"snapshot:",
		"VERMORY_SNAPSHOT_VERSION",
		"envOrDefault",
		".ShortCommit",
	} {
		if !strings.Contains(configuration, required) {
			t.Fatalf("GoReleaser snapshot configuration is missing %q", required)
		}
	}
}

func TestI09ScriptsRequireNativeUpgradeRollbackAndRetentionSemantics(t *testing.T) {
	packages := readRepositoryScript(t, "scripts", "build-i09-versioned-packages.sh")
	repository := readRepositoryScript(t, "scripts", "build-i09-lifecycle-repository.sh")
	acceptance := readRepositoryScript(t, "deploy", "linux", "run-i09-repository-lifecycle-acceptance.sh")

	for _, required := range []string{
		"git worktree add --detach",
		"VERMORY_SNAPSHOT_VERSION",
		"goreleaser release",
		"linux_${architecture}",
		"base_qualification_version",
		"candidate_qualification_version",
	} {
		if !strings.Contains(packages, required) {
			t.Fatalf("I09 package builder is missing %q", required)
		}
	}
	for _, required := range []string{
		"--multiversion",
		"createrepo_c",
		"base-snapshot",
		"full-snapshot",
		"same-ephemeral-key",
		"private signing key material",
	} {
		if !strings.Contains(repository, required) {
			t.Fatalf("I09 repository builder is missing %q", required)
		}
	}
	for _, required := range []string{
		"--allow-downgrades",
		"dnf",
		"downgrade",
		"upgrade",
		"repo_gpgcheck=1",
		"signed-by=",
		"operator-owned-i09",
		"service_identity_stable",
		"base_package_retained",
		"candidate_reupgrade",
	} {
		if !strings.Contains(acceptance, required) {
			t.Fatalf("I09 lifecycle acceptance is missing %q", required)
		}
	}
	if strings.Contains(packages, "sudo ") || strings.Contains(repository, "sudo ") || strings.Contains(acceptance, "sudo ") {
		t.Fatal("I09 scripts must not invoke sudo internally")
	}
}

func TestCIWorkflowRequiresI09NativeLifecycleBeforeSigning(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	versionedPackages := workflowSection(t, workflow, "  linux-versioned-packages:", "\n  linux-repository-lifecycle-apt:")
	aptLifecycle := workflowSection(t, workflow, "  linux-repository-lifecycle-apt:", "\n  linux-repository-lifecycle-dnf:")
	dnfLifecycle := workflowSection(t, workflow, "  linux-repository-lifecycle-dnf:", "\n  sign-snapshot:")
	signing := workflowSection(t, workflow, "  sign-snapshot:", "")

	for _, required := range []string{
		"scripts/build-i09-versioned-packages.sh",
		"vermory-i09-versioned-packages-${{ matrix.arch }}-${{ env.SOURCE_SHA }}",
		"goreleaser/goreleaser-action@",
		"install-only: true",
	} {
		if !strings.Contains(versionedPackages, required) {
			t.Fatalf("linux-versioned-packages is missing %q", required)
		}
	}
	for _, section := range []struct {
		name string
		body string
	}{
		{name: "APT lifecycle", body: aptLifecycle},
		{name: "DNF lifecycle", body: dnfLifecycle},
	} {
		for _, required := range []string{
			"scripts/build-i09-lifecycle-repository.sh",
			"deploy/linux/run-i09-repository-lifecycle-acceptance.sh",
			"(.hard_gates | length == 26)",
			"vermory-i09-linux-repository-lifecycle-",
		} {
			if !strings.Contains(section.body, required) {
				t.Fatalf("%s is missing %q", section.name, required)
			}
		}
	}
	for _, required := range []string{
		"linux-versioned-packages",
		"linux-repository-lifecycle-apt",
		"linux-repository-lifecycle-dnf",
	} {
		if !strings.Contains(signing, required) {
			t.Fatalf("sign-snapshot dependency is missing %q", required)
		}
	}
}
