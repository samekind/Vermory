# Native Linux Package Qualification

Date: 2026-07-22

Reality case: `I06-linux-native-packages`

Status: `runtime-qualified`

Source revision: `4a20942b60f3198de25ba53091d23a40e499341e`

GitHub Actions run: [`29923986517`](https://github.com/samekind/Vermory/actions/runs/29923986517)

Evidence level: `public`

The normalized machine-readable result is retained in the
[I06 package snapshot](snapshots/2026-07-22-linux-native-packages.json).

## Qualified Contract

The exact pull-request head completed four independent package legs:

| Format | Go architecture | Native machine | Job |
|---|---|---|---|
| DEB | `amd64` | `x86_64` | [`88936300805`](https://github.com/samekind/Vermory/actions/runs/29923986517/job/88936300805) |
| RPM | `amd64` | `x86_64` | [`88936300759`](https://github.com/samekind/Vermory/actions/runs/29923986517/job/88936300759) |
| DEB | `arm64` | `aarch64` | [`88936300802`](https://github.com/samekind/Vermory/actions/runs/29923986517/job/88936300802) |
| RPM | `arm64` | `aarch64` | [`88936300792`](https://github.com/samekind/Vermory/actions/runs/29923986517/job/88936300792) |

Each leg followed this trajectory:

```text
exact pull-request head on native runner
-> GoReleaser package build
-> DEB or RPM metadata and architecture check
-> native package installation
-> installed binary revision and machine check
-> service identity, systemd unit, and non-activation checks
-> true package removal
-> package-owned file removal and operator-state preservation
-> normalized report plus the exact accepted package bytes
```

The dependent signing job then downloaded all four acceptance artifacts,
validated each report and package digest, replaced independently built package
copies with the accepted bytes, regenerated `checksums.txt`, created the
12-entry complete release manifest, and signed that manifest through GitHub
OIDC. The package bytes that passed installation are therefore the package
bytes covered by the signed manifest.

## Package Results

| Format | Architecture | Accepted and signed package SHA-256 | Hard gates |
|---|---|---|---|
| DEB | AMD64 | `41021b3faa8fd2d6653e7ae97cceba8c2170f73ca16774caf5775e38ae741a57` | `16/16` |
| DEB | ARM64 | `b64537e1b4e0644635479159dccc7259bc0ea06d90e3d8fded2d467ccb15a77e` | `16/16` |
| RPM | AMD64 | `7117c84cd9a3a4a8205007ecfdaa616a9ae893105f4c1395d84f647c934bf0b6` | `16/16` |
| RPM | ARM64 | `e2a344ceb6b4f1ad50800472a3af379f7a0b592c453f980ea299c06884a6e548` | `16/16` |

Every package installed `/usr/bin/vermory`, a hardened systemd unit, Linux
documentation, and a non-secret environment example. The installed binary
returned help, reported the exact source revision, and matched the native
runner architecture. The package created a non-root, non-login `vermory`
identity.

Installation did not create `/etc/vermory/vermory.env`, enable or start the
service, run a database migration, or carry a credential. Removal deleted
package-owned files while preserving an operator-created environment file and
the stable service identity. Package scripts contain no internal `sudo` call;
the GitHub runner established the one external root boundary required for
native package installation.

## Artifact Integrity

Each acceptance artifact contained exactly `report.json` and the package file.
Its independently downloaded ZIP SHA-256 matched GitHub's artifact digest.
The report package hash matched both the package inside that acceptance
artifact and the same package inside the signed snapshot.

| Leg | Artifact ID | Artifact ZIP SHA-256 | Report SHA-256 |
|---|---:|---|---|
| DEB AMD64 | `8531128039` | `7a0513d4f49aa781014548470fb04157d98215cbc9b36b77bb0c6584cab36f05` | `ef1db3ad91c6fd50242c747343890c67bf78e4857ccd28bd813342793d6482fd` |
| DEB ARM64 | `8531138244` | `d59107182de520ed0497b454f5bd23433961d0ff837239fc58c6b940907251a0` | `3349175109e676100e7b97004fcdd94be8debae29ad398d792b7408d91e23e9e` |
| RPM AMD64 | `8531127894` | `0b684d144c3e5b59abe698dffe0ddf2d49bb2ab6341ccdcd745a15ee0b20994c` | `c7814e5d4a99e4d584de062ee36ea6c24f98cd78363d414a15d3df63324d239f` |
| RPM ARM64 | `8531147789` | `d5139cd36153986e86d49cd1e76a8a0ff7f015d3cd530ea392efa7bd084f81c0` | `bb93d89dfb28eac3265b4b8dbf559e93e1361c28e38794186d0a0644665ddd92` |

## Signed Snapshot

Signing job [`88937403534`](https://github.com/samekind/Vermory/actions/runs/29923986517/job/88937403534)
produced artifact `8531264312`, named
`vermory-pr-snapshot-4a20942b60f3198de25ba53091d23a40e499341e`.
Its GitHub artifact digest and independently downloaded ZIP SHA-256 both equal
`6728bfa703d01c89634b9364b79378010d65cdc700312d01c61e01ee6dee5164`.

The manifest had 12 entries:

- four Go release archives;
- four exact packages accepted by the native package jobs;
- GoReleaser `checksums.txt` with eight verified entries;
- the OpenClaw package;
- the Hermes archive;
- the Hermes checksum sidecar.

All payload hashes and the Hermes sidecar verified. Manifest SHA-256 was
`af1f20906ed2e77fd58a15aefd594ae4361842c0459befac116251958835f0c5`.
Cosign `v3.0.6` verified the identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`
and issuer `https://token.actions.githubusercontent.com`. The Sigstore bundle
included a transparency-log promise and proof at log index `2218182904`.
A modified manifest and the wrong `release.yml` identity were both rejected.

The same exact-head run independently passed the complete test job and the two
AMD64/ARM64 I05 systemd lifecycle jobs before signing was allowed to start.

## Hard Gates

All four package legs passed the same 16 gates:

1. exact source head;
2. expected package format;
3. native architecture;
4. executable installed binary;
5. restricted service identity;
6. hardened systemd unit;
7. non-secret environment example;
8. protected environment absent after installation;
9. service not enabled or started;
10. package scripts without `sudo`;
11. package scripts without migrations;
12. package scripts without credentials;
13. true-removal guard present;
14. package-owned files removed;
15. operator state preserved;
16. normalized report credential-free.

## Retained Failures

Run [`29922576223`](https://github.com/samekind/Vermory/actions/runs/29922576223)
proved that RPM invokes maintainer scripts through `/bin/sh`: both RPM legs
rejected Bash-only `set -o pipefail`. The scripts were converted to POSIX shell
before a new run was accepted.

Run [`29922904137`](https://github.com/samekind/Vermory/actions/runs/29922904137)
was green, but it was rejected as final qualification evidence. External
artifact review found that its installed RPM/ARM64 SHA-256
`d588aac827312a9323e7f1e4088b602c3a47805249df2d109d71598aa0717c2d`
did not equal the independently rebuilt signed RPM/ARM64 SHA-256
`88a4aff69f04149a331a19435f32c5611a715b2c9235e43a13140e3fd07926ff`.
The signing pipeline was changed to consume exact accepted package bytes, and
only the later run is counted as qualified.

## Claim Boundary

I06 qualifies four exact-head DEB/RPM package artifacts installed and removed
on native AMD64/ARM64 runners and then incorporated byte-for-byte into one
verified OIDC-signed pull-request snapshot.

It does not qualify an APT or DNF repository, repository metadata/signing or
retention, tagged release publication, distribution-specific DNF upgrade
behavior, automatic database migration, PostgreSQL migration rollback,
long-duration uptime/SLA, or the complete Vermory platform.
