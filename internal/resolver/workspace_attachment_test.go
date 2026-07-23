package resolver

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeGitWorkspaceResolvesNestedCWDAndRoundTrips(t *testing.T) {
	repo := initAttachmentTestRepository(t)
	nested := filepath.Join(repo, "internal", "runtime")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	attachment, err := ProbeGitWorkspace(context.Background(), nested, "workstation-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if attachment.RepoRoot != repo || attachment.CWD != nested {
		t.Fatalf("nested cwd was not attached to canonical root: %#v", attachment)
	}
	encoded, err := EncodeWorkspaceAttachment(attachment)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeWorkspaceAttachment(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != attachment {
		t.Fatalf("attachment round trip changed identity: before=%#v after=%#v", attachment, decoded)
	}
}

func TestWorkspaceAttachmentRejectsTamperingAndUnknownFields(t *testing.T) {
	repo := initAttachmentTestRepository(t)
	attachment, err := ProbeGitWorkspace(context.Background(), repo, "workstation-alpha")
	if err != nil {
		t.Fatal(err)
	}
	attachment.GitCommonFingerprint = strings.Repeat("a", 64)
	if _, err := attachment.Normalized(); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("tampered attachment was accepted: %v", err)
	}
	payload := `{"version":1,"filesystem_namespace":"workstation-alpha","repo_root":"/repo","cwd":"/repo","git_common_fingerprint":"` + strings.Repeat("a", 64) + `","fingerprint":"` + strings.Repeat("b", 64) + `","tenant_id":"other"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if _, err := DecodeWorkspaceAttachment(encoded); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown attachment authority field was accepted: %v", err)
	}
}

func TestProbeGitWorkspaceDistinguishesWorktreeRootWithoutAutoIdentity(t *testing.T) {
	repo := initAttachmentTestRepository(t)
	writeAttachmentTestFile(t, filepath.Join(repo, "README.md"), "primary\n")
	runAttachmentGit(t, repo, "add", "README.md")
	runAttachmentGit(t, repo, "-c", "user.name=Vermory Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	worktree := filepath.Join(t.TempDir(), "Vermory-resolver")
	runAttachmentGit(t, repo, "worktree", "add", "-b", "resolver-test", worktree)

	primary, err := ProbeGitWorkspace(context.Background(), repo, "workstation-alpha")
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := ProbeGitWorkspace(context.Background(), worktree, "workstation-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if primary.RepoRoot == secondary.RepoRoot {
		t.Fatalf("worktree root was collapsed into the primary root: primary=%#v secondary=%#v", primary, secondary)
	}
	if primary.GitCommonFingerprint != secondary.GitCommonFingerprint {
		t.Fatalf("worktree diagnostic identity did not retain common Git evidence: primary=%#v secondary=%#v", primary, secondary)
	}
	if primary.Fingerprint == secondary.Fingerprint {
		t.Fatal("different workspace roots received the same attachment fingerprint")
	}
}

func TestNormalizeFilesystemNamespaceRejectsMissingOrUnsafeValue(t *testing.T) {
	for _, value := range []string{"", "../host", strings.Repeat("a", 129), "host with spaces"} {
		if _, err := NormalizeFilesystemNamespace(value, false); err == nil {
			t.Fatalf("unsafe filesystem namespace %q was accepted", value)
		}
	}
	if value, err := NormalizeFilesystemNamespace("", true); err != nil || value != "" {
		t.Fatalf("legacy empty namespace was not preserved: value=%q err=%v", value, err)
	}
}

func initAttachmentTestRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := filepath.Join(t.TempDir(), "Vermory")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runAttachmentGit(t, repo, "init")
	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func runAttachmentGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeAttachmentTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
