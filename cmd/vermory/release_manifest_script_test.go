package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseManifestScriptCreatesAndVerifiesCompletePayloadSet(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	runReleaseManifestScript(t, "create", dist, true)

	first, err := os.ReadFile(filepath.Join(dist, "release-manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(first)), "\n")
	if len(lines) != 12 {
		t.Fatalf("manifest entries=%d, want 12: %s", len(lines), first)
	}
	for index := 1; index < len(lines); index++ {
		if lines[index-1][66:] > lines[index][66:] {
			t.Fatalf("manifest is not sorted: %s", first)
		}
	}

	runReleaseManifestScript(t, "verify", dist, true)
	runReleaseManifestScript(t, "create", dist, true)
	second, err := os.ReadFile(filepath.Join(dist, "release-manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("manifest changed across identical creation:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestReleaseManifestScriptRejectsMissingOrModifiedPayload(t *testing.T) {
	t.Run("missing sidecar", func(t *testing.T) {
		dist := writeReleaseManifestPayloads(t)
		if err := os.Remove(filepath.Join(dist, "vermory-hermes-0.1.0.tar.gz.sha256")); err != nil {
			t.Fatal(err)
		}
		runReleaseManifestScript(t, "create", dist, false)
	})

	t.Run("modified payload", func(t *testing.T) {
		dist := writeReleaseManifestPayloads(t)
		runReleaseManifestScript(t, "create", dist, true)
		path := filepath.Join(dist, "vermory-openclaw-0.1.0.tgz")
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("tampered"); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		runReleaseManifestScript(t, "verify", dist, false)
	})
}

func TestReleaseManifestScriptRejectsPartialRepositoryQualificationSet(t *testing.T) {
	dist := writeReleaseManifestPayloads(t)
	if err := os.WriteFile(
		filepath.Join(dist, "vermory-repository-apt-amd64-"+qualifiedRepositorySource+".tar.gz"),
		[]byte("partial repository set\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runReleaseManifestScript(t, "create", dist, false)
}

func writeReleaseManifestPayloads(t *testing.T) string {
	t.Helper()
	dist := t.TempDir()
	files := []string{
		"checksums.txt",
		"vermory-openclaw-0.1.0.tgz",
		"vermory-hermes-0.1.0.tar.gz",
		"vermory-hermes-0.1.0.tar.gz.sha256",
		"vermory_0.0.0_darwin_amd64.tar.gz",
		"vermory_0.0.0_darwin_arm64.tar.gz",
		"vermory_0.0.0_linux_amd64.tar.gz",
		"vermory_0.0.0_linux_arm64.tar.gz",
		"vermory_0.0.0_linux_amd64.deb",
		"vermory_0.0.0_linux_arm64.deb",
		"vermory_0.0.0_linux_amd64.rpm",
		"vermory_0.0.0_linux_arm64.rpm",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dist, name), []byte("fixture:"+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dist
}

func runReleaseManifestScript(t *testing.T, mode, dist string, wantSuccess bool) {
	t.Helper()
	script := filepath.Join("..", "..", "scripts", "release-manifest.sh")
	command := exec.Command("bash", script, mode, dist)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("release manifest %s failed: %v\n%s", mode, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("release manifest %s unexpectedly passed:\n%s", mode, output)
	}
}
