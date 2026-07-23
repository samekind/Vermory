# Release Packaging Evidence

Date: 2026-07-14

Validated implementation revision: `0300a73fb9be0356b37703943273914a80a9273e`

Validated pull-request merge revision: `3fcde7cff62d51447cf3db635fc7f9920c9ea073`

## Scope

This evidence covers version metadata, four-platform Go archives, SHA-256
checksums, the independent OpenClaw package, pull-request snapshot artifacts,
and the manual/tag publication boundary. It validates a packaging and delivery
slice; it does not claim that the overall Vermory platform or a final public
release is complete.

The implementation is split across these revisions:

| Revision | Change |
|---|---|
| `eeec694` | shared version metadata, `version` command, root version flag, and MCP version source |
| `5def61c` | GoReleaser four-target archives and checksums |
| `90ff5da` | pull-request snapshot and manual/tag workflows |
| `e966671` | deterministic uploaded payload set without run-time `metadata.json` |
| `0300a73` | removal of a random token-digest test false positive exposed by repeated CI |

## Release Contract

GoReleaser `v2.17.0` builds with `CGO_ENABLED=0` and `-trimpath` for:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
```

Every `tar.gz` contains exactly:

```text
LICENSE
README.md
README.zh-CN.md
vermory
```

The binary mode is `0755`; documentation and license modes are `0644`. Archive
owners are normalized to `root/root`, and archive modification times use the
commit time. `checksums.txt` uses SHA-256. The OpenClaw integration remains a
separate `vermory-openclaw-0.1.0.tgz`.

Pull-request CI and manual dispatch use snapshot mode and cannot publish. The
tag job alone has `contents: write`; a `v*` tag runs the real GoReleaser release
and attaches the OpenClaw package to the matching draft GitHub Release.

## Local Verification

The full local gate used the PostgreSQL Unix-socket test database and ran:

```text
VERMORY_TEST_DATABASE_URL=postgresql:///vermory_test?host=/tmp go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL=postgresql:///vermory_test?host=/tmp go test -race -p 1 -count=1 <runtime package set>
VERMORY_TEST_DATABASE_URL=postgresql:///vermory_test?host=/tmp go test -count=1 -race ./internal/reality
go vet ./...
go mod tidy plus zero go.mod/go.sum diff
actionlint 1.7.7 for both workflows
GoReleaser 2.17.0 configuration validation
GoReleaser snapshot release twice
Node 24.18.0 and pnpm 11.12.0 OpenClaw install/check/pack twice
git diff --check
```

The database-backed suite passed every package. The runtime race set passed for
authn, runtime, webchat, identity CLI, operator CLI, MCP server, root command,
and provider packages. Reality race passed separately. OpenClaw passed 43 tests,
typecheck, build, and package creation.

The final local snapshot version output was:

```json
{"version":"0.0.0-SNAPSHOT-0300a73","revision":"0300a73fb9be0356b37703943273914a80a9273e","build_date":"2026-07-14T03:00:26Z","go_version":"go1.26.5"}
```

Two clean local builds of the same revision produced identical archive and
OpenClaw package bytes:

```text
33c07a28a3506b5157bb85d4e4cf155fcc36c706cea03391f8e700655d3b278d  vermory_0.0.0-SNAPSHOT-0300a73_darwin_amd64.tar.gz
74eab66059aa0ed485ab6d10a42accbb7ff8e56700a9a4c5078a30ee3b34bb30  vermory_0.0.0-SNAPSHOT-0300a73_darwin_arm64.tar.gz
ae2a103becb27b03747b79e9512bf14828d341b4bb316d6c62729640bc1cb3bc  vermory_0.0.0-SNAPSHOT-0300a73_linux_amd64.tar.gz
58e862afc561d68d7dd7ff47d7f2e79f1db2d60c0840c37d310760824df0539e  vermory_0.0.0-SNAPSHOT-0300a73_linux_arm64.tar.gz
e8b83836ed7141eab192af3e337ea9a421f1e8c37b44932c634c654697eec244  vermory-openclaw-0.1.0.tgz
```

`go version -m` reported `CGO_ENABLED=0`, `-trimpath=true`, and the correct
GOOS/GOARCH tuple for all four binaries. The local Darwin arm64 binary executed
on the host and returned the injected metadata shown above.

## Remote Pull-Request Artifact

GitHub Actions run
[`29302444695`](https://github.com/jstar0/Vermory/actions/runs/29302444695)
executed the protected `test` job against PostgreSQL 18 with Go 1.25.7,
Node 24, pnpm 11.12.0, and GoReleaser 2.17.0. Attempt 1 completed all 19 main
steps successfully in 3m35s, including snapshot construction, OpenClaw pack,
artifact upload, and clean-diff verification.

The pull-request event builds GitHub's synthetic merge revision rather than the
branch head. The downloaded artifact therefore used snapshot version
`0.0.0-SNAPSHOT-3fcde7c` while the Actions API correctly associated the workflow
run with branch head `0300a73fb9be0356b37703943273914a80a9273e`.

Attempt 1 artifact:

```text
artifact id: 8299044216
artifact name: vermory-pr-snapshot-3fcde7cff62d51447cf3db635fc7f9920c9ea073
Actions transport digest: sha256:22d2e58abdfa0a417c9e750908afcf7f21601149afa9a4e68aecd4d4077eaf86
retention expiry: 2026-07-21T03:04:06Z
```

The downloaded payload contained six files and no run-time metadata file:

```text
55e2b4a4e944abfff6547d73c17458fec621093e33cf9c87ae0bf900d33367c5  checksums.txt
e2833e6a5d5cbaf72af244b7c1650532fd81b0dfcb435c84f2462dd1906c3950  vermory-openclaw-0.1.0.tgz
21b7483851cbc025dbfd3fe81720fedb9280b405317be23c6d5a44bbe4462022  vermory_0.0.0-SNAPSHOT-3fcde7c_darwin_amd64.tar.gz
e37fd09226cecee41d955ba99b3ca7c81cac330f3faba62ebead162879cb8109  vermory_0.0.0-SNAPSHOT-3fcde7c_darwin_arm64.tar.gz
892cdbd519d7b3461ca963649ce7567dad5b57a88b4954ee66f7df8558d6aa17  vermory_0.0.0-SNAPSHOT-3fcde7c_linux_amd64.tar.gz
e72bcd02ccf9415569b1c08f0ac3282b131489082ab9b1b63bda7ad174357d77  vermory_0.0.0-SNAPSHOT-3fcde7c_linux_arm64.tar.gz
```

`shasum -a 256 -c checksums.txt` passed for all four Go archives. Every archive
contained the frozen four-file inventory. The Ubuntu-built Darwin arm64 binary
then executed on the local Apple Silicon host and reported:

```json
{"version":"0.0.0-SNAPSHOT-3fcde7c","revision":"3fcde7cff62d51447cf3db635fc7f9920c9ea073","build_date":"2026-07-14T03:00:32Z","go_version":"go1.25.7"}
```

`go version -m` independently confirmed `CGO_ENABLED=0`, `-trimpath=true`,
`GOOS=darwin`, and `GOARCH=arm64` for that downloaded binary.

## Reproducibility Boundary

Attempt 2 reran the same workflow and synthetic merge revision. It again
completed all 19 main steps successfully, this time in 3m52s. The replacement
artifact was:

```text
artifact id: 8299121733
artifact name: vermory-pr-snapshot-3fcde7cff62d51447cf3db635fc7f9920c9ea073
Actions transport digest: sha256:a8d2380ce62dd51cebf0824cb9c6f24f48643efa32cda18e9463aa65c90de1c4
retention expiry: 2026-07-21T03:09:01Z
```

The two Actions transport digests differ because GitHub creates a new ZIP
container for each upload. After downloading and extracting both attempts, a
sorted SHA-256 manifest of all six release payload files had zero differences.
The four Go archives, `checksums.txt`, and OpenClaw `.tgz` are therefore
byte-identical across repeated Ubuntu workflow runs.

The repeated-run process also exposed a pre-existing random false positive in
`TestPostgresTokenLifecycleStoresOnlyDigestAndReplaysSafely`. The test split a
base64url token on every underscore; a legal secret beginning with `_` produced
an empty substring, and every digest contains the empty string. The database
still held the correct 32-byte SHA-256 digest. The assertion now compares the
PostgreSQL `bytea` directly with the computed digest and verifies that it is not
equal to the decoded raw secret. Before push, the old assertion failed under a
1000-run replay; after revision `0300a73`, 1000 normal repetitions, 200 race
repetitions, the full database suite, and the complete runtime race set passed.

Local macOS and remote Linux `pnpm pack` outputs have different compressed
`.tgz` byte hashes, but their decompressed tar stream hash is identical:

```text
9e9ad0eab2a5484984637522c2c23926bf526790bee7c05ee1275d2b816a53dd
```

Their file trees, file bytes, modes, owners, paths, and normalized 1985 package
timestamps are also identical. This is a gzip container-platform difference,
not a package-content difference. Release publication is centralized on the
Ubuntu workflow, so byte reproducibility is evaluated between repeated Ubuntu
runs, while local verification additionally checks semantic package identity.

## Publication And Claim Boundary

Evidence revision `85445979f6d79683f7304f2f9df47a908add56a9` passed
the protected `test` job in GitHub Actions run
[`29303019839`](https://github.com/jstar0/Vermory/actions/runs/29303019839)
after the document and README links were pushed.

After the pull-request runs:

```text
Draft PR 1: CLEAN and MERGEABLE
required check: test
Git tags: none
GitHub Releases: none
```

This slice proves repeatable packaging, checksums, Actions delivery, and a
least-privilege tag publication path. It does not prove or provide artifact
signing, notarization, Homebrew, Docker images, Windows builds, npm registry
publication, a published GitHub Release, production scale, sealed evaluation,
or final open-source release acceptance.
