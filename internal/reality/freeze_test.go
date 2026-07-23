package reality

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFreezeCaseWritesDeterministicLock(t *testing.T) {
	dir := cloneCaseTree(t, "../../reality/testdata/valid-public", filepath.Join(t.TempDir(), "valid-public"))
	first, err := FreezeCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FreezeCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.LockSHA256 != second.LockSHA256 {
		t.Fatalf("freeze must be deterministic: %s != %s", first.LockSHA256, second.LockSHA256)
	}
	if first.CaseID != "valid-public" {
		t.Fatalf("expected case id valid-public, got %q", first.CaseID)
	}
	if _, err := os.Stat(filepath.Join(dir, "fixture-lock.json")); err != nil {
		t.Fatalf("expected fixture lock: %v", err)
	}
}

func TestValidateRootDetectsPostFreezeMutation(t *testing.T) {
	root := t.TempDir()
	dir := cloneCaseTree(t, "../../reality/testdata/valid-public", filepath.Join(root, "valid-public"))
	if _, err := FreezeCase(dir); err != nil {
		t.Fatal(err)
	}
	appendFixture(t, filepath.Join(dir, "fixtures", "source.md"), "mutated")
	report := ValidateRoot(root)
	if report.Pass || !hasViolation(report, "fixture_lock_mismatch") {
		t.Fatalf("expected mutation failure: %#v", report)
	}
}

func TestValidateRootRejectsLockedSymlinkOutsideCase(t *testing.T) {
	root := t.TempDir()
	dir := cloneCaseTree(t, "../../reality/testdata/valid-public", filepath.Join(root, "valid-public"))
	if _, err := FreezeCase(dir); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	content := []byte("outside evidence")
	if err := os.WriteFile(outside, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.md")); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, fixtureLockFilename)
	var lock FixtureLock
	if err := decodeStrictJSONFile(lockPath, &lock); err != nil {
		t.Fatal(err)
	}
	lock.Files = append(lock.Files, LockedFile{Path: "escape.md", SHA256: hashBytes(content), Bytes: int64(len(content))})
	data, err := marshalIndented(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	report := ValidateRoot(root)
	if report.Pass || !hasViolation(report, "fixture_lock_mismatch") {
		t.Fatalf("expected escaped symlink rejection: %#v", report)
	}
}

func TestMemoryEligibilityRealityCases(t *testing.T) {
	report := ValidateRoot("../../reality/cases")
	if !report.Pass {
		t.Fatalf("reality case root did not validate: %#v", report)
	}
	want := map[string]map[string]bool{
		"C02-housing-viewing-validity": {
			"temporal_validity": true,
			"exact_boundary":    true,
		},
		"W03-workspace-workaround-validity": {
			"temporal_validity":   true,
			"archive_not_current": true,
			"explicit_forgetting": true,
		},
	}
	for _, result := range report.Results {
		required, ok := want[result.CaseID]
		if !ok {
			continue
		}
		if !result.Pass || result.EvidenceLevel != EvidencePublic || result.LockSHA256 == "" {
			t.Fatalf("case %s is not frozen public evidence: %#v", result.CaseID, result)
		}
		for _, pressure := range result.Pressures {
			delete(required, pressure)
		}
		if len(required) != 0 {
			t.Fatalf("case %s lacks required pressures: %#v", result.CaseID, required)
		}
		delete(want, result.CaseID)
	}
	if len(want) != 0 {
		t.Fatalf("memory eligibility cases were not found: %#v", want)
	}
}

func cloneCaseTree(t *testing.T, source, destination string) string {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return destination
}

func appendFixture(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func hasViolation(report ValidationReport, code string) bool {
	for _, result := range report.Results {
		for _, violation := range result.Violations {
			if violation.Code == code {
				return true
			}
		}
	}
	return false
}
