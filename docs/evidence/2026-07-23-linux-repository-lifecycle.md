# Cross-Version Linux Repository Lifecycle Qualification

Date: 2026-07-23

Reality case: `I09-linux-repository-lifecycle`

Status: `runtime-qualified`

Frozen base revision: `858e5afc05b8136fd81d46a88164f799f2925197`

Candidate revision: `b2076981351f3e4f2f31c86a1abcd22c04cd71ec`

GitHub Actions run: [`29959050993`](https://github.com/samekind/Vermory/actions/runs/29959050993)

Evidence level: `public`

The normalized machine-readable result is retained in the
[I09 lifecycle snapshot](snapshots/2026-07-23-linux-repository-lifecycle.json).

## Qualified Contract

I09 extends I08 from one accepted repository snapshot to an explicit
cross-version package-manager lifecycle. It does not replace I06 or I08: the
base and candidate packages are built from two exact source revisions, assigned
qualification-only versions, and then exercised through native APT and DNF
resolution on both supported architectures.

Two package-building jobs produced the frozen base and exact candidate package
sets:

| Architecture | Job | Artifact |
|---|---|---|
| AMD64 | [`89055325016`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055325016) | `8545237739` |
| ARM64 | [`89055324992`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055324992) | `8545242357` |

Four independent lifecycle legs then completed the same native trajectory:

| Repository | Architecture | Native machine | Job |
|---|---|---|---|
| APT | `amd64` | `x86_64` | [`89055590116`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055590116) |
| APT | `arm64` | `aarch64` | [`89055590154`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055590154) |
| DNF | `amd64` | `x86_64` | [`89055590156`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055590156) |
| DNF | `arm64` | `aarch64` | [`89055590152`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89055590152) |

Each lifecycle leg followed this path:

```text
build base from the frozen accepted source revision
-> build candidate from the exact pull-request head
-> create a base-only signed repository snapshot
-> install base through the native package manager
-> create operator configuration
-> switch to a signed snapshot retaining base and adding candidate
-> perform a normal upgrade to candidate
-> prove a normal update does not implicitly downgrade candidate
-> explicitly roll back to the retained base version
-> perform a second normal upgrade to candidate
-> remove the package
-> prove operator configuration and service identity remain preserved
```

Both repository snapshots in one leg use the same short-lived qualification
key. APT enforces signed metadata through a dedicated `signed-by` keyring. DNF
enforces signed `repomd.xml` through `repo_gpgcheck=1`. Unrelated repositories
are disabled while resolving Vermory, so package selection cannot be explained
by another configured source.

## Versioned Package Sets

The versions below exist only to make package-manager ordering and explicit
rollback testable. They are not tags, releases, or public version promises.

| Format | Architecture | Base native version | Candidate native version | Base SHA-256 | Candidate SHA-256 |
|---|---|---|---|---|---|
| DEB | AMD64 | `0.0.1~i09.1` | `0.0.2~i09.1` | `1c8de8ed8c8ed515e21aeb209860b4a7308132f80225fa8ec48e01f4d27a6f6a` | `ccb6c65de6ca113e6f1eb3d61fd61677c5bfe2a3b0c7ad58e61d59e4f1831935` |
| DEB | ARM64 | `0.0.1~i09.1` | `0.0.2~i09.1` | `3f6be2e68568479296418cd33d62c6671848098afcc68a797c6e9e87f6d90d0c` | `cbe4232a441df68f5fe45afd6afd1a01f74f39bdeacc3d05ba9dc473f731ccb2` |
| RPM | AMD64 | `0.0.1~i09.1-1` | `0.0.2~i09.1-1` | `53ab7686c605e52e6d9a163183b2a86d2517e2842d526c5785848bb883ba573a` | `4a36b49c8a54f54b4fc0721aff27ecbfac69923f846e0fb936c48b7bb830ee43` |
| RPM | ARM64 | `0.0.1~i09.1-1` | `0.0.2~i09.1-1` | `3bb67f35ad93bcf6cc085ab63894f79dc8a66ec2dd81cd2aa88ca0c2c69cbb2c` | `1119656cce47814e8272d10c41576065288d2dde88b514e61c69ead3f351f552` |

The package-set manifests bind the base to
`858e5afc05b8136fd81d46a88164f799f2925197` and the candidate to
`b2076981351f3e4f2f31c86a1abcd22c04cd71ec`. Native package-manager comparison
selects the candidate as newer in all four format and architecture combinations.

## Lifecycle Results

| Repository | Architecture | Base native version | Candidate native version | Bundle SHA-256 | Hard gates |
|---|---|---|---|---|---|
| APT | AMD64 | `0.0.1~i09.1` | `0.0.2~i09.1` | `f0a5d2dc9bbf0edcbafbe4455a34bd75dab36ee0ce89fae08ea2b078ee83b23a` | `26/26` |
| APT | ARM64 | `0.0.1~i09.1` | `0.0.2~i09.1` | `2bc4bd4f51971cd4105c85a85b58b7ede327c1fcc92e76570e4c908f7c1738e4` | `26/26` |
| DNF | AMD64 | `0.0.1~i09.1-1` | `0.0.2~i09.1-1` | `c58926da135ef6f68f45f05f928c38b30fbdae56656ff087ffd3923cdba28d8e` | `26/26` |
| DNF | ARM64 | `0.0.1~i09.1-1` | `0.0.2~i09.1-1` | `d60610dc8150771aa52effb54eadf3eacf50c26942357f35a8cb9244cdab80cf` | `26/26` |

All four legs installed the frozen base, upgraded normally to the exact
candidate, stayed on candidate when a lower-only repository was presented,
rolled back only through an explicit package-manager request, and upgraded
normally to candidate again. The installed binary reported the expected source
revision at each phase.

Operator configuration and the dedicated non-login service UID/GID remained
stable across upgrade, rollback, re-upgrade, and final package removal. The
service remained inactive and disabled throughout. Package operations did not
run database migrations. Each evidence bundle contains a public qualification
key and both repository snapshots, but no private signing key.

## Artifact Integrity

The six I09 artifacts were downloaded as their original ZIP bytes to the
external evidence cache. Every independently calculated ZIP SHA-256 equals the
digest reported by GitHub.

| Artifact | Artifact ID | ZIP SHA-256 | Inner report SHA-256 |
|---|---:|---|---|
| Versioned packages AMD64 | `8545237739` | `fef65de79dbffc3f64140b6db05a68a3ec9d90d39e324a7effd76f768fb6524d` | `59e3ce5bc4fc760304c33732673bac468973867e234906e911f2ddc0dd415882` |
| Versioned packages ARM64 | `8545242357` | `d34a4878121af3cbc0d6fa0fda1674db2df0a8eee921d04661cec84dd7ed59c7` | `70e99f4ace1ae9432298cb8675cbbfb5a10ae584c0f638d6cafd0e0783b9c81a` |
| APT lifecycle AMD64 | `8545260118` | `026e4ee051ac6f7087123b47973ac74d2bd941bd86aa7a7a327d02981a4f33df` | `7cd2ba5cd657b82b4ef4013fe515bd10e704e68801e7fc3fb4ec501d33b48b07` |
| APT lifecycle ARM64 | `8545263675` | `ac5563f74db8cfb94533001e6fa7975c165692c7eecca15448f8189ed27ff16a` | `127902093764ad767e2dfae063031bac674835a0e6c4a3000e8ad6ee5aba0288` |
| DNF lifecycle AMD64 | `8545257279` | `a90ecdfa405a11a5342ada2ca8ccf12994f5bc977a5eb34ac8d885897b8a7948` | `ae080fff37c977d132fbd7f1ca8577a108b86bf6060da47d41b751746cdbab1e` |
| DNF lifecycle ARM64 | `8545275739` | `d4f7f04f45c97a9d24494ff87ee57ee2947c587c90cbb1136718ae2a7bfbd0f3` | `16c1c346276eaebf8833f93c8f0d378ade9bd3a823cedf9fcaafe960743a4edd` |

The four normalized reports each contain exactly 26 hard gates and all values
are `true`. Their declared bundle hashes equal the independently calculated
hashes above. Bundle-content scans found no private-key marker, API key, bearer
token, or GitHub token pattern.

## Protected Snapshot

The complete accepted run passed `test`, both I05 service jobs, all four I06
package jobs, all four I08 repository jobs, all four I09 lifecycle jobs, and
then signing job
[`89056789539`](https://github.com/samekind/Vermory/actions/runs/29959050993/job/89056789539).

The signing job produced artifact `8545410585`, named
`vermory-pr-snapshot-b2076981351f3e4f2f31c86a1abcd22c04cd71ec`. Its GitHub
artifact digest and independently downloaded ZIP SHA-256 both equal
`7b95b508332ad378fd276b091dd87109c3268b312665b790a76d31d87224fe29`.

The signed `release-manifest.sha256` still contains 16 payload entries: the
eight GoReleaser platform/package outputs, four exact I08 repository bundles,
GoReleaser `checksums.txt`, OpenClaw, Hermes, and the Hermes checksum sidecar.
I09 qualification-only package sets and lifecycle bundles are deliberately not
release payload entries; their four successful jobs are prerequisites that
must pass before `sign-snapshot` can start.

All 16 payload hashes verified. The release manifest SHA-256 is
`d67ae6ab55a316c99f220bceac33aea6f0bb823af81ff87374184cc0f61628ad`,
and the Sigstore bundle SHA-256 is
`2c3b4d817fba936fa58919f29d74c5b3f55b8cac9bb0dcd64be920e2f2d08f72`.

Official Cosign `v3.0.6` independently verified workflow identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`
and issuer `https://token.actions.githubusercontent.com`. A modified manifest
was rejected, and the unchanged manifest was rejected when verification
expected `release.yml` instead of `ci.yml`.

## Hard Gates

All four lifecycle legs passed the same 26 gates:

1. exact candidate pull-request head;
2. frozen base source revision;
3. explicit qualification-only base and candidate versions;
4. native package-manager version ordering;
5. native architecture;
6. base-only repository snapshot;
7. retained base plus candidate in the full snapshot;
8. one ephemeral key across both snapshots in a leg;
9. repository metadata signature enforcement;
10. unrelated repositories disabled;
11. base installation through the native package manager;
12. base binary revision bound to the frozen source;
13. dormant service after base installation;
14. operator configuration creation;
15. normal candidate upgrade;
16. candidate binary revision bound to the exact head;
17. stable service identity;
18. implicit downgrade rejected;
19. explicit rollback;
20. rollback binary revision bound to the frozen source;
21. rollback state preserved;
22. normal candidate re-upgrade;
23. final removal preserves operator state;
24. service always inactive and disabled;
25. private signing key absent;
26. report and bundle SHA-256 binding with no credential material.

## Retained Failures And Infrastructure Delay

The first workflow run
[`29958102641`](https://github.com/samekind/Vermory/actions/runs/29958102641)
retains two distinct failure classes.

Attempt 1 queued all nine initial jobs for more than nine minutes without a
runner or executed step. It was cancelled and rerun while GitHub Status tracked
the incident [Disruption with actions hosted runners](https://stspg.io/hccgdw2k2b1q),
which reported that approximately 3% of hosted-runner jobs were experiencing
start delays exceeding five minutes. This is retained as an infrastructure
incident, not a product pass or failure.

Attempt 2 built both versioned package sets successfully, but all four I09
lifecycle jobs exited immediately after the base installation. The service was
correctly inactive and disabled; however, the shell function combined `set -e`
with bare `systemctl is-active ... && fail` and `systemctl is-enabled ... &&
fail` commands. The expected non-zero result therefore terminated the script
before the explicit failure branch could be evaluated. Commit
`b2076981351f3e4f2f31c86a1abcd22c04cd71ec` changed both checks to explicit
`if` conditions and added a regression contract test. Run `29959050993` then
completed all four full lifecycles and the dependent signing job.

## Claim Boundary

I09 qualifies one frozen base-to-candidate lifecycle for DEB/APT and RPM/DNF on
native AMD64 and ARM64 execution. It proves normal upgrade, rejection of an
implicit downgrade, explicit rollback to a retained older package, normal
re-upgrade, final removal, service dormancy, operator-state preservation,
source-revision binding, repository metadata signature enforcement, and
credential-free evidence for those exact qualification packages.

It does not qualify a stable production repository signing key, a public or
long-lived hosted repository, mirrors, retention guarantees, release tags,
public semantic-version promises, unattended operating-system updates,
database schema migration or rollback, RPM payload OpenPGP signatures, or the
complete Vermory platform.
