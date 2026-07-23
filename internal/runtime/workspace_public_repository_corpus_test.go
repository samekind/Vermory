package runtime

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/resolver"
)

type workspacePublicRepositoryCorpusCase struct {
	Version                      string                            `json:"version"`
	ID                           string                            `json:"id"`
	RealityCaseID                string                            `json:"reality_case_id"`
	TenantID                     string                            `json:"tenant_id"`
	OtherTenantID                string                            `json:"other_tenant_id"`
	FilesystemNamespace          string                            `json:"filesystem_namespace"`
	AlternateFilesystemNamespace string                            `json:"alternate_filesystem_namespace"`
	SourceAuthorization          string                            `json:"source_authorization"`
	MinimumRepositoryCount       int                               `json:"minimum_repository_count"`
	Repositories                 []workspacePublicRepositorySource `json:"repositories"`
	OtherTenantMarker            string                            `json:"other_tenant_marker"`
	HardGateCount                int                               `json:"hard_gate_count"`
	HardGates                    []string                          `json:"hard_gates"`
	NonClaims                    []string                          `json:"non_claims"`
}

type workspacePublicRepositorySource struct {
	ID         string `json:"id"`
	Ecosystem  string `json:"ecosystem"`
	URL        string `json:"url"`
	Revision   string `json:"revision"`
	Directory  string `json:"directory"`
	NestedPath string `json:"nested_path"`
	Marker     string `json:"marker"`
}

func TestW37RealRepositoryCorpusCaseIsFrozen(t *testing.T) {
	manifest := loadWorkspacePublicRepositoryCorpusCase(t)
	if manifest.Version != "1" || manifest.ID != "W37-real-repository-corpus" ||
		manifest.RealityCaseID != "W05-trusted-workspace-attachment" {
		t.Fatalf("unexpected W37 identity: %#v", manifest)
	}
	if manifest.MinimumRepositoryCount < 6 || len(manifest.Repositories) < manifest.MinimumRepositoryCount {
		t.Fatalf("W37 corpus is too small: minimum=%d actual=%d", manifest.MinimumRepositoryCount, len(manifest.Repositories))
	}
	if manifest.TenantID == "" || manifest.OtherTenantID == "" || manifest.TenantID == manifest.OtherTenantID {
		t.Fatalf("W37 tenant isolation contract is invalid: %#v", manifest)
	}
	if manifest.FilesystemNamespace == "" || manifest.AlternateFilesystemNamespace == "" ||
		manifest.FilesystemNamespace == manifest.AlternateFilesystemNamespace {
		t.Fatalf("W37 namespace contract is invalid: %#v", manifest)
	}
	if manifest.SourceAuthorization != "public_https_read_only" {
		t.Fatalf("W37 source authorization is invalid: %q", manifest.SourceAuthorization)
	}
	if manifest.HardGateCount != 18 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("W37 hard-gate count drifted: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	assertUniqueNonEmptyStrings(t, "hard gate", manifest.HardGates)
	assertUniqueNonEmptyStrings(t, "non-claim", manifest.NonClaims)

	ids := make([]string, 0, len(manifest.Repositories))
	urls := make([]string, 0, len(manifest.Repositories))
	directories := make([]string, 0, len(manifest.Repositories))
	markers := make([]string, 0, len(manifest.Repositories))
	for _, repository := range manifest.Repositories {
		if repository.Ecosystem == "" || !validRelativeCorpusPath(repository.Directory) || !validRelativeCorpusPath(repository.NestedPath) {
			t.Fatalf("W37 repository source is incomplete or path-dependent: %#v", repository)
		}
		if !strings.HasPrefix(repository.URL, "https://github.com/") || !strings.HasSuffix(repository.URL, ".git") {
			t.Fatalf("W37 repository URL is not a public GitHub HTTPS source: %q", repository.URL)
		}
		if len(repository.Revision) != 40 {
			t.Fatalf("W37 repository revision is not an exact commit: %#v", repository)
		}
		if _, err := hex.DecodeString(repository.Revision); err != nil {
			t.Fatalf("W37 repository revision is not hexadecimal: %#v", repository)
		}
		ids = append(ids, repository.ID)
		urls = append(urls, repository.URL)
		directories = append(directories, repository.Directory)
		markers = append(markers, repository.Marker)
	}
	assertUniqueNonEmptyStrings(t, "repository id", ids)
	assertUniqueNonEmptyStrings(t, "repository URL", urls)
	assertUniqueNonEmptyStrings(t, "repository directory", directories)
	assertUniqueNonEmptyStrings(t, "repository marker", markers)
}

func TestW37RealRepositoryCorpusAcceptance(t *testing.T) {
	corpusRoot := strings.TrimSpace(os.Getenv("VERMORY_W37_CORPUS_ROOT"))
	if corpusRoot == "" {
		t.Skip("VERMORY_W37_CORPUS_ROOT is not set")
	}
	manifest := loadWorkspacePublicRepositoryCorpusCase(t)
	absoluteCorpusRoot, err := filepath.Abs(corpusRoot)
	if err != nil {
		t.Fatalf("resolve W37 corpus root: %v", err)
	}
	canonicalCorpusRoot, err := filepath.EvalSymlinks(absoluteCorpusRoot)
	if err != nil {
		t.Fatalf("canonicalize W37 corpus root: %v", err)
	}

	type acceptedRepository struct {
		source workspacePublicRepositorySource
		root   string
	}
	accepted := make([]acceptedRepository, 0, len(manifest.Repositories))
	commonFingerprints := make(map[string]string, len(manifest.Repositories))
	for _, source := range manifest.Repositories {
		root, err := filepath.EvalSymlinks(filepath.Join(canonicalCorpusRoot, source.Directory))
		if err != nil {
			t.Fatalf("canonicalize W37 repository %s: %v", source.ID, err)
		}
		if got := gitOutput(t, root, "rev-parse", "HEAD"); got != source.Revision {
			t.Fatalf("W37 repository %s revision drifted: got=%s want=%s", source.ID, got, source.Revision)
		}
		if got := gitOutput(t, root, "config", "--get", "remote.origin.url"); got != source.URL {
			t.Fatalf("W37 repository %s origin drifted: got=%q want=%q", source.ID, got, source.URL)
		}

		rootAttachment := probeWorkspace(t, root, manifest.FilesystemNamespace)
		nestedAttachment := probeWorkspace(t, filepath.Join(root, source.NestedPath), manifest.FilesystemNamespace)
		if rootAttachment.RepoRoot != root || nestedAttachment.RepoRoot != root {
			t.Fatalf("W37 repository %s did not resolve its canonical root: root=%#v nested=%#v", source.ID, rootAttachment, nestedAttachment)
		}
		if rootAttachment.GitCommonFingerprint != nestedAttachment.GitCommonFingerprint {
			t.Fatalf("W37 repository %s nested probe changed Git common evidence", source.ID)
		}
		if rootAttachment.Fingerprint == nestedAttachment.Fingerprint {
			t.Fatalf("W37 repository %s root and nested receipts collapsed", source.ID)
		}
		if previous, exists := commonFingerprints[rootAttachment.GitCommonFingerprint]; exists {
			t.Fatalf("W37 repositories %s and %s collided on Git common evidence", previous, source.ID)
		}
		commonFingerprints[rootAttachment.GitCommonFingerprint] = source.ID

		encoded, err := resolver.EncodeWorkspaceAttachment(nestedAttachment)
		if err != nil {
			t.Fatalf("encode W37 repository %s attachment: %v", source.ID, err)
		}
		decoded, err := resolver.DecodeWorkspaceAttachment(encoded)
		if err != nil {
			t.Fatalf("decode W37 repository %s attachment: %v", source.ID, err)
		}
		if decoded != nestedAttachment {
			t.Fatalf("W37 repository %s attachment round trip drifted: got=%#v want=%#v", source.ID, decoded, nestedAttachment)
		}
		accepted = append(accepted, acceptedRepository{source: source, root: root})
	}

	store := openTestStore(t)
	ctx := context.Background()
	governance := NewGovernanceServiceWithNamespace(store, manifest.TenantID, manifest.FilesystemNamespace)
	continuityIDs := make(map[string]string, len(accepted))
	for _, repository := range accepted {
		anchor := WorkspaceAnchor{RepoRoot: repository.root, FilesystemNamespace: manifest.FilesystemNamespace}
		resolution, err := governance.ConfirmWorkspaceAnchor(ctx, anchor)
		requireNoError(t, err)
		if previous, exists := continuityIDs[resolution.ContinuityID]; exists {
			t.Fatalf("W37 repositories %s and %s shared continuity %s", previous, repository.source.ID, resolution.ContinuityID)
		}
		continuityIDs[resolution.ContinuityID] = repository.source.ID
		_, err = governance.AddSource(ctx, repository.root, GovernanceWriteRequest{
			OperationID: "w37-seed-" + repository.source.ID,
			MemoryKey:   "continuation_marker",
			Content:     repository.source.Marker,
			SourceRef:   "public-git:" + repository.source.URL + "@" + repository.source.Revision,
		})
		requireNoError(t, err)
	}

	service := NewService(store, manifest.TenantID)
	for _, repository := range accepted {
		anchor := WorkspaceAnchor{RepoRoot: repository.root, FilesystemNamespace: manifest.FilesystemNamespace}
		operationID := "w37-prepare-" + repository.source.ID
		prepared := requireWorkspaceFact(t, service, operationID, anchor, repository.source.Marker)
		for _, other := range accepted {
			if other.source.ID != repository.source.ID {
				requireNotContains(t, prepared.Context, other.source.Marker)
			}
		}
		replayed := requireWorkspaceFact(t, service, operationID, anchor, repository.source.Marker)
		if replayed.DeliveryID != prepared.DeliveryID || replayed.Context != prepared.Context {
			t.Fatalf("W37 repository %s prepare replay drifted: first=%#v replay=%#v", repository.source.ID, prepared, replayed)
		}
	}

	first := accepted[0]
	firstAnchor := WorkspaceAnchor{RepoRoot: first.root, FilesystemNamespace: manifest.FilesystemNamespace}
	assertWorkspaceAbstains(t, service, "w37-alternate-namespace", WorkspaceAnchor{
		RepoRoot: first.root, FilesystemNamespace: manifest.AlternateFilesystemNamespace,
	})

	otherGovernance := NewGovernanceServiceWithNamespace(store, manifest.OtherTenantID, manifest.FilesystemNamespace)
	otherResolution, err := otherGovernance.ConfirmWorkspaceAnchor(ctx, firstAnchor)
	requireNoError(t, err)
	if otherResolution.ContinuityID == "" {
		t.Fatal("W37 other tenant did not receive a continuity")
	}
	if _, exists := continuityIDs[otherResolution.ContinuityID]; exists {
		t.Fatalf("W37 other tenant reused target continuity: %s", otherResolution.ContinuityID)
	}
	_, err = otherGovernance.AddSource(ctx, first.root, GovernanceWriteRequest{
		OperationID: "w37-seed-other-tenant",
		MemoryKey:   "continuation_marker",
		Content:     manifest.OtherTenantMarker,
		SourceRef:   "synthetic:W37:other-tenant",
	})
	requireNoError(t, err)
	otherPrepared := requireWorkspaceFact(t, NewService(store, manifest.OtherTenantID), "w37-other-tenant-prepare", firstAnchor, manifest.OtherTenantMarker)
	requireNotContains(t, otherPrepared.Context, first.source.Marker)
	targetPrepared := requireWorkspaceFact(t, service, "w37-target-after-other-tenant", firstAnchor, first.source.Marker)
	requireNotContains(t, targetPrepared.Context, manifest.OtherTenantMarker)
}

func loadWorkspacePublicRepositoryCorpusCase(t *testing.T) workspacePublicRepositoryCorpusCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W37-real-repository-corpus", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest workspacePublicRepositoryCorpusCase
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W37 case contains trailing JSON data: %v", err)
	}
	return manifest
}

func assertUniqueNonEmptyStrings(t *testing.T, label string, values []string) {
	t.Helper()
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("W37 %s cannot be empty", label)
		}
		if _, exists := seen[value]; exists {
			t.Fatalf("W37 %s is duplicated: %q", label, value)
		}
		seen[value] = struct{}{}
	}
}

func validRelativeCorpusPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	cleaned := filepath.Clean(value)
	return cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, ".."+string(filepath.Separator))
}
