package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	WorkspaceAttachmentVersion = 1
	maxAttachmentEncodedBytes  = 8192
)

var filesystemNamespacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type WorkspaceAttachment struct {
	Version              int    `json:"version"`
	FilesystemNamespace  string `json:"filesystem_namespace"`
	RepoRoot             string `json:"repo_root"`
	CWD                  string `json:"cwd"`
	GitCommonFingerprint string `json:"git_common_fingerprint"`
	Fingerprint          string `json:"fingerprint"`
}

func ProbeGitWorkspace(ctx context.Context, cwd, filesystemNamespace string) (WorkspaceAttachment, error) {
	namespace, err := NormalizeFilesystemNamespace(filesystemNamespace, false)
	if err != nil {
		return WorkspaceAttachment{}, err
	}
	canonicalCWD, err := canonicalDirectory(cwd)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("workspace cwd: %w", err)
	}
	repoRoot, err := gitPath(ctx, canonicalCWD, "--show-toplevel")
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("resolve Git workspace root: %w", err)
	}
	canonicalRoot, err := canonicalDirectory(repoRoot)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("canonicalize Git workspace root: %w", err)
	}
	if !pathContains(canonicalRoot, canonicalCWD) {
		return WorkspaceAttachment{}, fmt.Errorf("workspace cwd is outside the resolved Git root")
	}
	commonDir, err := gitPath(ctx, canonicalCWD, "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	canonicalCommon, err := canonicalDirectory(commonDir)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("canonicalize Git common directory: %w", err)
	}

	attachment := WorkspaceAttachment{
		Version:              WorkspaceAttachmentVersion,
		FilesystemNamespace:  namespace,
		RepoRoot:             canonicalRoot,
		CWD:                  canonicalCWD,
		GitCommonFingerprint: digestParts(namespace, canonicalCommon),
	}
	attachment.Fingerprint = attachmentFingerprint(attachment)
	return attachment.Normalized()
}

func (a WorkspaceAttachment) Normalized() (WorkspaceAttachment, error) {
	if a.Version != WorkspaceAttachmentVersion {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment version %d is unsupported", a.Version)
	}
	namespace, err := NormalizeFilesystemNamespace(a.FilesystemNamespace, false)
	if err != nil {
		return WorkspaceAttachment{}, err
	}
	a.FilesystemNamespace = namespace
	a.RepoRoot, err = normalizeAbsolutePath(a.RepoRoot)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment repo_root: %w", err)
	}
	a.CWD, err = normalizeAbsolutePath(a.CWD)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment cwd: %w", err)
	}
	if !pathContains(a.RepoRoot, a.CWD) {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment cwd is outside repo_root")
	}
	a.GitCommonFingerprint = strings.ToLower(strings.TrimSpace(a.GitCommonFingerprint))
	if !validSHA256(a.GitCommonFingerprint) {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment git_common_fingerprint must be a SHA-256 digest")
	}
	a.Fingerprint = strings.ToLower(strings.TrimSpace(a.Fingerprint))
	if !validSHA256(a.Fingerprint) {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment fingerprint must be a SHA-256 digest")
	}
	expected := attachmentFingerprint(a)
	if a.Fingerprint != expected {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment fingerprint mismatch")
	}
	return a, nil
}

func EncodeWorkspaceAttachment(attachment WorkspaceAttachment) (string, error) {
	normalized, err := attachment.Normalized()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode workspace attachment: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeWorkspaceAttachment(encoded string) (WorkspaceAttachment, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment is required")
	}
	if len(encoded) > maxAttachmentEncodedBytes {
		return WorkspaceAttachment{}, fmt.Errorf("workspace attachment is too large")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("decode workspace attachment: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var attachment WorkspaceAttachment
	if err := decoder.Decode(&attachment); err != nil {
		return WorkspaceAttachment{}, fmt.Errorf("decode workspace attachment JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return WorkspaceAttachment{}, err
	}
	return attachment.Normalized()
}

func NormalizeFilesystemNamespace(value string, allowLegacy bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" && allowLegacy {
		return "", nil
	}
	if !filesystemNamespacePattern.MatchString(value) {
		return "", fmt.Errorf("filesystem_namespace must be 1-128 opaque identifier characters")
	}
	return value, nil
}

func attachmentFingerprint(attachment WorkspaceAttachment) string {
	return digestParts(
		fmt.Sprint(attachment.Version),
		attachment.FilesystemNamespace,
		attachment.RepoRoot,
		attachment.CWD,
		attachment.GitCommonFingerprint,
	)
}

func digestParts(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(hash, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func gitPath(ctx context.Context, cwd string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", cwd, "rev-parse"}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git rev-parse failed: %s", detail)
	}
	value := strings.TrimSpace(string(output))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("git rev-parse returned an invalid path")
	}
	return value, nil
}

func canonicalDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func normalizeAbsolutePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("path must be absolute")
	}
	return filepath.Clean(value), nil
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode workspace attachment trailing data: %w", err)
	}
	return fmt.Errorf("workspace attachment contains trailing JSON")
}
