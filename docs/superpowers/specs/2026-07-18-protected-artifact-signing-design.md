# Protected Artifact Signing Design

Status: frozen implementation design

Date: 2026-07-18

Reality case: `I04-protected-artifact-signing`

## Purpose

W24 closes the gap between deterministic release checksums and an
identity-authenticated protected artifact. The signed subject is a complete
release payload manifest, not the GitHub transport ZIP and not only the four Go
archives already covered by GoReleaser's `checksums.txt`.

## Signed Payload Contract

`release-manifest.sha256` contains exactly eight sorted records:

1. four `vermory_*_{darwin,linux}_{amd64,arm64}.tar.gz` archives;
2. `checksums.txt`;
3. `vermory-openclaw-*.tgz`;
4. `vermory-hermes-*.tar.gz`;
5. `vermory-hermes-*.tar.gz.sha256`.

The manifest generator fails if any payload class is missing or duplicated.
Verification fails if any listed byte changes. The manifest and Sigstore bundle
are uploaded beside the payloads.

## Trust Boundary

The ordinary `test` job keeps `contents: read` and has no OIDC authority. It may
execute pull-request code, tests, package scripts, and build tools without being
able to mint the Vermory signing identity.

A separate `sign-snapshot` job:

- depends on successful `test`;
- runs only for repository pushes or same-repository pull requests;
- has only `contents: read` and `id-token: write`;
- rebuilds the deterministic release payload from the same Git revision;
- creates the complete manifest;
- signs it with keyless Sigstore;
- verifies the exact workflow identity and GitHub OIDC issuer;
- runs modified-manifest and wrong-identity negative controls;
- uploads the only accepted signed snapshot artifact.

Fork pull requests do not run the signing job. A skipped signing job is not
evidence of signed delivery.

## Identity Contract

The expected certificate identity is exact:

```text
${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/.github/workflows/<workflow>@${GITHUB_REF}
```

The issuer is exact:

```text
https://token.actions.githubusercontent.com
```

For pull-request snapshots, GitHub API evidence additionally proves that the
signed synthetic merge's second parent equals the source branch HEAD. This
prevents a valid workflow certificate from being used to claim a different
source revision.

## Release Workflow

Manual snapshots and `v*` tag publication use the same manifest and keyless
signature contract in `release.yml`. Tag publication attaches the manifest and
bundle to the draft GitHub Release along with OpenClaw and Hermes packages.

W24 executes and qualifies only the protected pull-request snapshot path. The
manual and tag paths are configured and statically validated but remain
unqualified until separately executed.

## Verification

Protected CI must establish:

- deterministic manifest generation and self-verification;
- missing payload rejection;
- modified payload rejection;
- exact keyless signature verification;
- modified manifest rejection;
- wrong workflow identity rejection;
- no long-lived private key or signing secret;
- signed artifact upload only after all prior gates pass.

Mac mini verification must establish:

- GitHub artifact transport size and digest;
- all eight payload hashes;
- Sigstore bundle verification against exact identity and issuer;
- modified-manifest and wrong-identity rejection;
- synthetic merge signature and second-parent binding;
- package and credential privacy checks;
- a checksum manifest over retained evidence.

## Non-Claims

W24 does not claim:

- macOS notarization;
- Homebrew, npm, PyPI, container, or OS-package publication;
- a published GitHub Release or tag;
- an external sealed evaluator;
- cross-host PostgreSQL HA;
- final open-source release acceptance.
