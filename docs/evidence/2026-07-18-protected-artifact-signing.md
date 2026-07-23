# Protected Artifact Signing Qualification

Date: 2026-07-18

Reality case: `I04-protected-artifact-signing`

Source revision: `e4bb83ba72989f6cd60d1af7ef61e50481c13b6a`

Protected synthetic merge: `efe3ad6d532a28b3efed10c00de1e300bdc947fa`

GitHub Actions run: `29641606056`

Evidence level: `public`

## Qualified Contract

This run qualifies one protected pull-request snapshot with the following
contract:

```text
ordinary untrusted-code test job without OIDC authority
-> complete deterministic manifest for every release payload
-> separate same-repository post-test signing job
-> GitHub OIDC keyless Cosign signature
-> exact workflow identity and issuer verification
-> modified-manifest rejection
-> wrong-workflow-identity rejection
-> independent cross-host verification on an ARM64 Mac mini
```

The signed subject is `release-manifest.sha256`, not GitHub's transport ZIP.
The manifest covers all eight release payload files.

## Protected CI

| Gate | Job | Result |
|---|---:|---:|
| PostgreSQL, full Go suite, runtime race, Reality race, vet, module drift | `test` / `88072944118` | pass |
| OpenClaw install/check/package | `test` | pass |
| Hermes isolated tests and deterministic package | `test` | pass |
| Four Go archives and complete manifest | `test` | pass |
| Separate trusted signing job after `test` | `sign-snapshot` / `88073327492` | pass |
| Pinned Cosign install | `v3.0.6` | pass |
| Exact identity and issuer verification | `sign-snapshot` | pass |
| Modified manifest rejected | `sign-snapshot` | pass |
| Wrong workflow identity rejected | `sign-snapshot` | pass |
| Signed artifact upload | artifact `8428840638` | pass |

The repository-level workflow permission remains `contents: read`. The `test`
job has no `id-token: write`. Only `sign-snapshot`, after `needs: test`, receives
`contents: read` and `id-token: write`, and it runs only for same-repository pull
requests or repository pushes. Fork pull-request code does not receive Vermory
signing authority.

After qualification, `main` branch protection was updated so both `test` and
`sign-snapshot` are strict required checks bound to the GitHub Actions app.

## Source And Synthetic Merge Binding

The protected artifact name contains the pull-request synthetic merge revision:

```text
vermory-pr-snapshot-efe3ad6d532a28b3efed10c00de1e300bdc947fa
```

GitHub commit evidence reported:

```text
first parent:  2fe75531c7d8e85b6d7fadf39e2b42dd70ebaac7
second parent: e4bb83ba72989f6cd60d1af7ef61e50481c13b6a
```

The second parent exactly equals the source branch head. GitHub reported the
synthetic merge commit verification as valid.

## Complete Release Payload Manifest

Artifact metadata:

| Field | Value |
|---|---|
| Artifact ID | `8428840638` |
| Stored bytes | `21,813,069` |
| GitHub artifact SHA-256 | `f22fd6aff9c84e412762db86dfc6caaedb3fd7d87ab49757ff9bf40f7e9f674f` |
| Retention | seven days |

The extracted artifact contains two verification files plus exactly eight
manifest payloads:

```text
checksums.txt
vermory-hermes-0.1.0.tar.gz
vermory-hermes-0.1.0.tar.gz.sha256
vermory-openclaw-0.1.0.tgz
vermory_0.0.0-SNAPSHOT-efe3ad6_darwin_amd64.tar.gz
vermory_0.0.0-SNAPSHOT-efe3ad6_darwin_arm64.tar.gz
vermory_0.0.0-SNAPSHOT-efe3ad6_linux_amd64.tar.gz
vermory_0.0.0-SNAPSHOT-efe3ad6_linux_arm64.tar.gz
release-manifest.sha256
release-manifest.sigstore.json
```

`release-manifest.sha256` contains eight sorted records. Every payload passed
independent SHA-256 verification. `checksums.txt` independently verified the
four Go archives, and the Hermes sidecar independently verified the Hermes
archive.

## Sigstore Identity

Expected certificate identity:

```text
https://github.com/jstar0/Vermory/.github/workflows/ci.yml@refs/pull/1/merge
```

Expected OIDC issuer:

```text
https://token.actions.githubusercontent.com
```

The bundle media type is:

```text
application/vnd.dev.sigstore.bundle.v0.3+json
```

The bundle contains one transparency-log entry with both an inclusion promise
and inclusion proof. The recorded Rekor log index is `2194452343`.

Positive verification returned `Verified OK`. Appending one newline to a copy
of the manifest produced an invalid-signature rejection. Verifying the original
manifest against `release.yml@refs/pull/1/merge` produced an identity mismatch
and showed the actual SAN as `ci.yml@refs/pull/1/merge`.

## Independent Mac Mini Verification

The workstation and Mac mini were not on the same LAN. Delivery used the
existing Qingdao reverse-management SSH tunnel. No direct LAN connection,
`sudo`, system-wide installation, or Mac mini NewAPI route was used.

The GitHub artifact was streamed directly to an isolated W24 directory on the
Mac mini without persisting the ZIP in the workstation repository. The remote
ZIP byte count and SHA-256 exactly matched GitHub artifact metadata.

The Mac mini had no preinstalled Cosign. The official Sigstore `v3.0.6`
`darwin/arm64` binary and official checksum file were streamed into the
isolated evidence tools directory. Both the GitHub release asset digest and
`cosign_checksums.txt` reported:

```text
5fadd012ae6381a6a29ff86a7d39aa873878852f1073fc90b15995961ecfb084
```

The isolated binary reported `v3.0.6`, commit
`f1ad3ee952313be5d74a49d67ba0aa8d0d5e351f`, and platform
`darwin/arm64`.

The isolated W24 root retains 23 hashed files. Its evidence manifest SHA-256 is:

```text
cc919863e7ba9da6887c91a5d4559ac20a30e2a68238cbbfeaaa42152bc2fd70
```

Independent package inspection showed:

- every Go archive contains `vermory`, `LICENSE`, `README.md`, and
  `README.zh-CN.md`;
- the Darwin ARM64 binary reports revision
  `efe3ad6d532a28b3efed10c00de1e300bdc947fa`;
- the OpenClaw package contains the compiled JavaScript, declarations,
  `openclaw.plugin.json`, and `package.json`;
- the Hermes archive contains only the documented provider package, lockfile,
  metadata, readme, and license.

## Retained Failures

The run retained these implementation or infrastructure failures:

1. the first Reality regression run expected sixteen public cases after I04
   raised the authoritative count to seventeen;
2. the first local actionlint execution failed while `sum.golang.org` returned
   `unexpected EOF`; the verified retry passed without disabling `GOSUMDB`;
3. the first branch-protection update used `PUT` for a subresource requiring
   `PATCH` and returned HTTP 404 without changing protection;
4. the first attempt to generate a remote JSON summary failed because nested
   shell quoting removed JSON string quotes; the public snapshot and final
   remote copy use the reviewed repository JSON instead.

See the [failure ledger](snapshots/2026-07-18-w24-failure-ledger.json).

## Privacy And Publication Gates

- no private signing key exists in the repository, workflow, artifact, or Mac
  mini evidence;
- no GitHub OIDC token is retained;
- no provider key, database credential, or full environment is present;
- no tag was created;
- no GitHub Release was created;
- the pull request remains a draft.

## Claim Boundary

This evidence qualifies the protected pull-request signing path for one exact
source head and synthetic merge. It proves complete payload-manifest signing,
exact GitHub workflow identity verification, negative controls, and independent
Mac mini verification.

It does not qualify manual `workflow_dispatch` execution, tagged release
publication, macOS notarization, package-manager distribution, container-image
signing, SLSA provenance, external sealed evaluation, or the complete Vermory
platform.

Machine-readable summary:
[2026-07-18-w24-protected-artifact-signing.json](snapshots/2026-07-18-w24-protected-artifact-signing.json)
