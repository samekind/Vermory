# Open-Source Release Packaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce versioned, checksummed Linux and macOS Vermory archives plus the OpenClaw package through a reproducible snapshot/tag workflow without publishing a release from the current Draft PR.

**Architecture:** Keep application version metadata in `internal/brand` and inject release values through Go linker flags. Use GoReleaser `v2.17.0` for four CGO-free binary targets, archives, and checksums. Pull requests run a snapshot build; a separate tag/manual workflow owns GitHub Release publication and includes the independently packed OpenClaw plugin.

**Tech Stack:** Go 1.25.7, Cobra, GoReleaser 2.17.0, GitHub Actions, Node 24, pnpm 11.12.0.

## Global Constraints

- Supported release targets are `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64`.
- Every archive contains the `vermory` binary, `LICENSE`, `README.md`, and `README.zh-CN.md`.
- Release metadata contains version, revision, build date, and Go runtime version.
- Development builds report `dev`, `unknown`, and `unknown`; release values are linker-injected.
- Archives use `tar.gz`; checksums use SHA-256.
- OpenClaw remains an independent npm-compatible `.tgz`, not embedded in the Go archives.
- Pull requests may create snapshot artifacts only. No current Draft PR step creates a tag or GitHub Release.
- Tag publication requires `contents: write`; normal CI remains `contents: read`.
- Signing, notarization, Homebrew, Docker images, Windows, and package registries are explicit non-goals for this slice.

---

### Task 1: Version Metadata Contract

**Files:**
- Modify: `internal/brand/brand.go`
- Create: `internal/brand/brand_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/mcpserver/server_test.go`

**Interfaces:**
- Produces: mutable linker targets `brand.Version`, `brand.Revision`, and `brand.BuildDate`.
- Produces: `brand.Info()` returning `VersionInfo` with version, revision, build date, and Go version.
- Produces: `vermory version` and `vermory --version` output derived from the same metadata.
- Produces: MCP implementation metadata using `brand.Version` instead of a second hard-coded version.

- [x] **Step 1: Write failing brand and CLI tests**

Require development defaults, complete `Info()` output, stable JSON field names, a registered `version` command, root `--version`, and MCP metadata sourced from `brand.Version`.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./internal/brand ./cmd/vermory ./internal/mcpserver -run 'Test.*Version' -count=1
```

Expected: FAIL because build metadata and the version command do not exist.

- [x] **Step 3: Implement minimal metadata and command**

Use string variables so Go linker `-X` can inject release values. The command writes one JSON object to stdout; Cobra's root version flag uses the same `brand.Version` value.

- [x] **Step 4: Verify GREEN and commit**

Run the focused tests and:

```bash
git add internal/brand cmd/vermory internal/mcpserver
git commit -m "feat: expose release version metadata"
```

### Task 2: Reproducible Snapshot Archives

**Files:**
- Create: `.goreleaser.yaml`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: Task 1 linker variables.
- Produces: four `tar.gz` archives, `checksums.txt`, and GoReleaser metadata under `dist/`.

- [x] **Step 1: Verify missing release configuration**

Run:

```bash
goreleaser check --config .goreleaser.yaml
```

Expected: FAIL because `.goreleaser.yaml` does not exist. Do not accept
GoReleaser's implicit default configuration as a release contract.

- [x] **Step 2: Add minimal GoReleaser configuration**

Configure one `vermory` build from `./cmd/vermory`, `CGO_ENABLED=0`, the four target tuples, `-trimpath`, release linker metadata, `tar.gz` archives with the required files, SHA-256 checksums, and no source archive.

- [x] **Step 3: Run a real snapshot build**

Run:

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

Require four archives, one checksum file, successful host execution of the Darwin arm64 binary, and `go version -m` evidence for every binary.

- [x] **Step 4: Commit**

```bash
git add .goreleaser.yaml .gitignore
git commit -m "build: add multi-architecture release archives"
```

### Task 3: Snapshot And Tag Workflows

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Produces: PR snapshot packaging gate with an uploaded immutable Actions artifact.
- Produces: manual/tag release workflow with GitHub Release publication only for a `v*` tag.
- Produces: OpenClaw `.tgz` beside Go archives and checksums.

- [x] **Step 1: Add PR snapshot packaging**

After tests pass, install GoReleaser `v2.17.0`, run a snapshot, pack OpenClaw, collect the archives/checksums/tgz, and upload them with 7-day retention.

- [x] **Step 2: Add tag/manual release workflow**

`workflow_dispatch` performs a non-publishing snapshot. `push.tags: ['v*']` performs a real GoReleaser release and uploads the OpenClaw package to the matching GitHub Release using the repository token. Use `contents: write` only in this workflow.

- [x] **Step 3: Validate workflow syntax**

Run:

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/ci.yml .github/workflows/release.yml
```

- [x] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "ci: package snapshot and tagged releases"
```

### Task 4: Evidence And Delivery

**Files:**
- Create: `docs/evidence/2026-07-14-release-packaging.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: Draft PR 1 body

**Interfaces:**
- Produces: exact archive inventory, checksums, version outputs, target metadata, workflow revisions, remote run IDs, and non-claims.

- [x] **Step 1: Run full local release gates**

Run the database-backed Go suite, runtime race set, vet, tidy diff, snapshot release, OpenClaw check/package, actionlint, and `git diff --check`.

- [x] **Step 2: Commit and push evidence**

```bash
git add README.md README.zh-CN.md docs/evidence/2026-07-14-release-packaging.md docs/superpowers/plans/2026-07-14-release-packaging.md
git commit -m "docs: record release packaging evidence"
git push
```

- [x] **Step 3: Verify protected remote gates**

Require Draft PR 1 to remain `CLEAN` and `MERGEABLE`, the protected `test` check to pass, snapshot artifacts to exist, and no GitHub Release or tag to be created from the PR run.

- [x] **Step 4: Close this slice without closing the platform goal**

Mark this checklist complete only after evidence is on the remote branch and the protected check passes. Keep the overall Vermory goal active for conflict candidate formation, broader original benchmarks, scale, sealed evaluation, signing/notarization, and final release acceptance.
