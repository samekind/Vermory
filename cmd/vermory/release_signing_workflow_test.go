package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIWorkflowKeepsOIDCOutOfTestJobAndSignsCompleteSnapshot(t *testing.T) {
	workflow := readWorkflow(t, "ci.yml")
	parts := strings.SplitN(workflow, "jobs:", 2)
	if len(parts) != 2 {
		t.Fatal("CI workflow has no jobs section")
	}
	if strings.Contains(parts[0], "id-token: write") {
		t.Fatal("CI workflow-level permissions expose OIDC to the test job")
	}

	signing := workflowSection(t, workflow, "  sign-snapshot:", "")
	for _, required := range []string{
		"needs: [test, linux-service-lifecycle, linux-service-lifecycle-arm64, linux-package-install]",
		"id-token: write",
		"github.event.pull_request.head.repo.full_name == github.repository",
		"SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}",
		"ref: ${{ env.SOURCE_SHA }}",
		"Verify snapshot source revision",
		`test "$(git rev-parse HEAD)" = "$SOURCE_SHA"`,
		"name: vermory-pr-snapshot-${{ env.SOURCE_SHA }}",
		"scripts/release-manifest.sh create dist",
		"cosign sign-blob",
		"cosign verify-blob",
		"--certificate-identity",
		"--certificate-oidc-issuer",
		"release-manifest.sigstore.json",
		"sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6",
		"cosign-release: v3.0.6",
	} {
		if !strings.Contains(signing, required) {
			t.Fatalf("sign-snapshot is missing %q", required)
		}
	}

	lifecycle := workflowSection(t, workflow, "  linux-service-lifecycle:", "\n  linux-service-lifecycle-arm64:")
	for _, required := range []string{
		"runs-on: ubuntu-latest",
		"SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}",
		"I05_QUALIFICATION: github-hosted-ubuntu-systemd-amd64",
		"I05_EXPECTED_MACHINE: x86_64",
		"ref: ${{ env.SOURCE_SHA }}",
		"Verify lifecycle source revision",
		"deploy/linux/run-i05-acceptance.sh",
		`.qualification == "github-hosted-ubuntu-systemd-amd64"`,
		`.runtime.machine == "x86_64"`,
		"vermory-i05-linux-service-amd64-${{ env.SOURCE_SHA }}",
	} {
		if !strings.Contains(lifecycle, required) {
			t.Fatalf("linux-service-lifecycle is missing %q", required)
		}
	}

	armLifecycle := workflowSection(t, workflow, "  linux-service-lifecycle-arm64:", "\n  linux-package-install:")
	for _, required := range []string{
		"runs-on: ubuntu-24.04-arm",
		"SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}",
		"I05_QUALIFICATION: github-hosted-ubuntu-systemd-arm64",
		"I05_EXPECTED_MACHINE: aarch64",
		"ref: ${{ env.SOURCE_SHA }}",
		"Verify native ARM64 lifecycle source and architecture",
		`test "$(uname -m)" = "$I05_EXPECTED_MACHINE"`,
		"deploy/linux/run-i05-acceptance.sh",
		"vermory-i05-linux-service-arm64-${{ env.SOURCE_SHA }}",
	} {
		if !strings.Contains(armLifecycle, required) {
			t.Fatalf("linux-service-lifecycle-arm64 is missing %q", required)
		}
	}

	packageInstall := workflowSection(t, workflow, "  linux-package-install:", "\n  sign-snapshot:")
	for _, required := range []string{
		"runs-on: ${{ matrix.runner }}",
		"format: deb",
		"format: rpm",
		"runner: ubuntu-latest",
		"runner: ubuntu-24.04-arm",
		"arch: amd64",
		"arch: arm64",
		"deploy/linux/run-i06-package-acceptance.sh",
		"vermory-i06-linux-package-${{ matrix.format }}-${{ matrix.arch }}-${{ env.SOURCE_SHA }}",
	} {
		if !strings.Contains(packageInstall, required) {
			t.Fatalf("linux-package-install is missing %q", required)
		}
	}

	testJob := workflowSection(t, workflow, "  test:", "\n  sign-snapshot:")
	if strings.Contains(testJob, "id-token: write") {
		t.Fatal("test job has OIDC signing permission")
	}
	if strings.Contains(testJob, "Upload release snapshot") {
		t.Fatal("test job still uploads an unsigned release snapshot")
	}
}

func TestReleaseWorkflowSignsManualAndTaggedPayloadManifests(t *testing.T) {
	workflow := readWorkflow(t, "release.yml")
	jobs := []struct {
		name string
		next string
	}{
		{name: "snapshot", next: "\n  publish:"},
		{name: "publish"},
	}
	for _, job := range jobs {
		section := workflowSection(t, workflow, "  "+job.name+":", job.next)
		for _, required := range []string{
			"id-token: write",
			"scripts/release-manifest.sh create dist",
			"cosign sign-blob",
			"cosign verify-blob",
			"release-manifest.sha256",
			"release-manifest.sigstore.json",
			"dist/*.deb",
			"dist/*.rpm",
		} {
			if !strings.Contains(section, required) {
				t.Fatalf("release %s job is missing %q", job.name, required)
			}
		}
	}
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func workflowSection(t *testing.T, workflow, start, nextPrefix string) string {
	t.Helper()
	startIndex := strings.Index(workflow, start)
	if startIndex < 0 {
		t.Fatalf("workflow section %q is missing", start)
	}
	rest := workflow[startIndex+len(start):]
	if nextPrefix == "" {
		return rest
	}
	endIndex := strings.Index(rest, nextPrefix)
	if endIndex < 0 {
		return rest
	}
	return rest[:endIndex]
}
