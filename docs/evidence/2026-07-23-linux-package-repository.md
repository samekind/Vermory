# Native Linux Package Repository Qualification

Date: 2026-07-23

Reality case: `I08-linux-package-repository`

Status: `runtime-qualified`

Source revision: `858e5afc05b8136fd81d46a88164f799f2925197`

GitHub Actions run: [`29953896649`](https://github.com/samekind/Vermory/actions/runs/29953896649)

Evidence level: `public`

The normalized machine-readable result is retained in the
[I08 repository snapshot](snapshots/2026-07-23-linux-package-repository.json).

## Qualified Contract

The exact pull-request head completed four independent repository legs after
the corresponding I06 package legs had accepted the package bytes:

| Repository | Architecture | Native machine | Job |
|---|---|---|---|
| APT | `amd64` | `x86_64` | [`89038308876`](https://github.com/samekind/Vermory/actions/runs/29953896649/job/89038308876) |
| APT | `arm64` | `aarch64` | [`89038308994`](https://github.com/samekind/Vermory/actions/runs/29953896649/job/89038308994) |
| DNF | `amd64` | `x86_64` | [`89038308886`](https://github.com/samekind/Vermory/actions/runs/29953896649/job/89038308886) |
| DNF | `arm64` | `aarch64` | [`89038308883`](https://github.com/samekind/Vermory/actions/runs/29953896649/job/89038308883) |

Each leg followed this trajectory:

```text
exact pull-request head
-> download the exact I06-qualified package artifact
-> verify the I06 report and package SHA-256
-> build native APT or DNF repository metadata
-> sign repository metadata with an ephemeral qualification key
-> configure the native package manager with the bundled public key
-> disable unrelated repositories
-> download and install Vermory from file://
-> verify downloaded bytes and installed source revision
-> verify the service was not enabled or started
-> modify repository metadata
-> require the package manager itself to reject the tampered repository
-> emit a credential-free report and hashed repository bundle
```

APT used signed `InRelease` and `Release.gpg` metadata with a dedicated
`signed-by` keyring. DNF used signed `repomd.xml` metadata with
`repo_gpgcheck=1`; the accepted tamper probe used an independent repository
identifier, a fresh cache and persistence directory, `metadata_expire=0`,
`skip_if_unavailable=0`, and `--refresh`. A successful command could therefore
not be explained by a cached valid repository or by silently skipping an
unavailable invalid repository.

## Repository Results

| Repository | Architecture | Accepted package SHA-256 | Repository bundle SHA-256 | Hard gates |
|---|---|---|---|---|
| APT | AMD64 | `b481f16cef527695849583c69fef62a73c1cb27f6920d470bc4e734df80d3cb6` | `e3d6313fa2e60700b363dddb630e3128e2b1be8306a8d74665ab68eeddea4d3f` | `18/18` |
| APT | ARM64 | `e3f783e56f2e3fc07f0ddaafc942f8dd80345d81a233b0be763577341c722b47` | `7a8deb3e1cfc693fa180776a730661773f4741242e90bf880fb3a8a67953ced4` | `18/18` |
| DNF | AMD64 | `849df49aa64518f36664769d7479d70ac7230e115fcc27db42353d0496bd276e` | `58bd985cea6a0d10524ded38da6ed65e06f07f227b30f43042671f7abf0fddd4` | `18/18` |
| DNF | ARM64 | `3761499c01361f24b1d243d2eecbf7b44be6fd5c3baa87a8230e0fc6ac317745` | `59a72b68af208f2d0b4bacf2f4603d7c7f74769081fb4a34db7d137426390cbc` | `18/18` |

The package hashes equal the I06 acceptance artifacts and the package entries
in the final release manifest. The package manager download hashes also equal
those values. Every installed binary reported source revision
`858e5afc05b8136fd81d46a88164f799f2925197` on its native architecture.

Each bundle contains native repository metadata, one accepted package, one
public key, and `repository.json`. No private signing key is present. The four
public-key fingerprints are intentionally different because each leg uses a
short-lived CI qualification key rather than claiming a production repository
key lifecycle.

## Artifact Integrity

The four acceptance artifacts were independently downloaded to the external
evidence cache. Each downloaded ZIP SHA-256 equals the digest reported by
GitHub. The report hash and repository bundle hash were then calculated from
the extracted bytes.

| Leg | Artifact ID | Artifact ZIP SHA-256 | Report SHA-256 |
|---|---:|---|---|
| APT AMD64 | `8543259023` | `9b53387e35b175c57c88923dd8f3a349bb9825557e630d71361b39d483e86b3f` | `cb7c978dad9d4d7d254244aa05ea0848bc71e27622efb4f6d602c34d2135fda3` |
| APT ARM64 | `8543258420` | `2a570f603f2bc26f11605d428a77690275f96b75917cddec84c787c91a1fe38b` | `f43d66e0c947fed87da5a1d3671b5cfc06c62d9ac0f2d92bb8cd015e87d1b4bf` |
| DNF AMD64 | `8543267443` | `c4dfa6a7f21096e365b01c18c8fc05ebfeed7a32a1fbc00cdcb14929aab84d3b` | `5df4f252d7de8a11d65780fe8756d57db384d787ff498b70f78fa88d8d32d8a3` |
| DNF ARM64 | `8543260060` | `bf907c14833c81ba00d5ea76e8d27444d1fb7d9564a0c2a170ec7fb14cfd6650` | `a981b7bb6062e7f8917e28996e9f5e75ea98246129f5299ae6f501a9fdfbc3cc` |

## Signed Snapshot

Signing job [`89038852826`](https://github.com/samekind/Vermory/actions/runs/29953896649/job/89038852826)
produced artifact `8543470072`, named
`vermory-pr-snapshot-858e5afc05b8136fd81d46a88164f799f2925197`.
Its GitHub artifact digest and independently downloaded ZIP SHA-256 both equal
`8ccd4ea3de9fb23f166c510d4e924f3f63d7caa982332372f9cfe17b4d588e67`.

The manifest has 16 entries:

- four Go release archives;
- four exact DEB/RPM packages accepted by I06;
- four exact APT/DNF repository bundles accepted by I08;
- GoReleaser `checksums.txt` with eight verified entries;
- the OpenClaw package;
- the Hermes archive;
- the Hermes checksum sidecar.

All 16 payload hashes verified. Manifest SHA-256 is
`f151a384d94ce2dc317dfd90787cbfaf11e0a56dfc9f7e0c2596503d58eee5a9`,
and the Sigstore bundle SHA-256 is
`5b23c71810632195405c121a30de578d7a92e59e71242bcf7a75c00cecf86d74`.

Official Cosign `v3.0.6` independently verified the identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`
and issuer `https://token.actions.githubusercontent.com`. The bundle media type
is `application/vnd.dev.sigstore.bundle.v0.3+json`; it contains a transparency
log inclusion promise and proof at log index `2219773507`. A modified manifest
failed signature verification, and the unchanged manifest failed verification
when the expected identity was changed to `release.yml`.

The same exact-head run also passed the full test job, two native systemd and
PostgreSQL lifecycle jobs, and all four I06 package jobs before signing could
start.

## Hard Gates

All four repository legs passed the same 18 gates:

1. exact source head;
2. exact I06 package bytes;
3. expected repository kind;
4. native architecture;
5. native package manager;
6. `file://` repository transport with unrelated repositories disabled;
7. native repository metadata;
8. package digest bound into repository metadata;
9. valid repository metadata signature;
10. client-side signature enforcement;
11. downloaded package digest bound to the accepted bytes;
12. installed binary revision bound to the source head;
13. service not enabled or started;
14. tampered metadata rejected by the package manager;
15. private signing key absent from the bundle;
16. ephemeral qualification-key scope declared;
17. repository bundle bound to a SHA-256 digest;
18. normalized report credential-free.

## Retained Failures

The accepted result was reached through six retained failed runs rather than
by rewriting the history as a single successful attempt:

| Run | Source head | Failure retained |
|---|---|---|
| [`29951831006`](https://github.com/samekind/Vermory/actions/runs/29951831006) | `f6723f710a27ebb8aa107dfc3266ca21024bf843` | APT download-only output was searched at the wrong cache level; DNF checkout hit container ownership protection. |
| [`29952252432`](https://github.com/samekind/Vermory/actions/runs/29952252432) | `2ba06ad6d2df48154deefd7488ba6eb27a6b2c48` | APT still used the wrong download location; DNF assumed a fixed compressed metadata filename. |
| [`29952545596`](https://github.com/samekind/Vermory/actions/runs/29952545596) | `87b358efe616b2517daac8691bb37e47c2d6e89b` | APT passed on both architectures; DNF repository construction still assumed the wrong primary metadata format. |
| [`29952883835`](https://github.com/samekind/Vermory/actions/runs/29952883835) | `1ead952f75475b010fe720122c340beef33ad8fe` | DNF metadata signing and loading passed, but DNF5 rejected the legacy `install --downloadonly --downloaddir` invocation. |
| [`29953121695`](https://github.com/samekind/Vermory/actions/runs/29953121695) | `eb073aa896f7ab0dd66ae33d48041ddd7357a1f3` | `dnf download` fetched the package, but the evidence script looked in the wrong output location. |
| [`29953440464`](https://github.com/samekind/Vermory/actions/runs/29953440464) | `eafe4b1be8266f28d96b7050d9089e0d691675c9` | Both DNF installations passed, but the tampered repository probe reused state and returned success instead of proving rejection. |

The final fix gave the tampered repository its own identifier, forced metadata
refresh, disabled unavailable-repository skipping, and isolated the cache and
persistence directories. Both native DNF legs then rejected tampered metadata.

## Claim Boundary

I08 qualifies four exact-head APT/DNF repository bundles for AMD64/ARM64. Each
bundle contains the exact package bytes previously accepted by I06, is consumed
through the native package manager from `file://`, enforces signed repository
metadata, rejects modified metadata, and is incorporated byte-for-byte into a
verified OIDC-signed pull-request snapshot.

It does not qualify a stable production repository signing key, a public or
long-lived hosted repository, mirrors, retention, cross-version upgrade,
downgrade or rollback behavior, tagged publication, or the complete Vermory
platform. RPM payload OpenPGP signatures are not qualified; the DNF result is
specifically repository metadata signature enforcement.
