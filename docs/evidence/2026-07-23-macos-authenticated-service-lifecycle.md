# macOS Authenticated Service Lifecycle Qualification

Date: 2026-07-23

Reality case: `I11-macos-authenticated-service-lifecycle`

Status: `runtime-qualified`

Qualified source revision: `e8d2bd34ee63e4cb923287baaed1275061ef5e8b`

Frozen base revision: `989bea77eeab56ee8ef87ccf9034269958897231`

GitHub Actions run: [`29967482446`](https://github.com/samekind/Vermory/actions/runs/29967482446)

Test job: [`89082029034`](https://github.com/samekind/Vermory/actions/runs/29967482446/job/89082029034)

Signing job: [`89082583992`](https://github.com/samekind/Vermory/actions/runs/29967482446/job/89082583992)

Evidence level: `public`

The normalized machine-readable result is retained in the
[I11 lifecycle snapshot](snapshots/2026-07-23-macos-authenticated-service-lifecycle.json).
The credential-free runtime report and execution log remain outside Git on the
operator-controlled evidence hosts.

## Qualified Contract

An OIDC-signed Darwin ARM64 candidate completed this isolated trajectory on the
ARM64 Mac mini with macOS 26.5.1, launchd, and PostgreSQL 18.3:

```text
signed exact-revision candidate
-> version and read-only database compatibility preflight
-> unprivileged user LaunchAgent install on loopback
-> authenticated governed state creation
-> incompatible candidate rejected without changing the live service
-> exact candidate upgrade with one complete rollback slot
-> incompatible rollback rejected without stopping the candidate
-> restart with governed state retained
-> compatible explicit rollback to the frozen base
-> candidate reinstall
-> unhealthy replacement rejected with automatic complete restoration
-> OpenClaw and Hermes backend lifecycle probes
-> credential scan and checksum-bound report
-> service, database, role, runtime, plist, and listener cleanup
```

The run used the deterministic mock provider because I11 qualifies deployment,
database compatibility, recovery, and security behavior rather than model
quality. No provider credential was required.

## Runtime Boundary

| Field | Accepted value |
|---|---|
| Host class | user-owned Mac mini |
| Operating system | macOS `26.5.1` |
| Architecture | `arm64` |
| PostgreSQL | `18.3` Homebrew client and local server |
| Service manager | launchd user domain |
| Privilege | unprivileged, no `sudo` |
| Listener | loopback-only `127.0.0.1:18811` |
| Runtime provider | deterministic mock |
| Runtime run ID | `i11-20260723-e8d2bd3-v6` |

The workstation and Mac mini were not assumed to share a LAN. The exact runner
bundle and candidate were transferred over the existing restricted FRP SSH
route into user-owned paths. No system-wide installation, LaunchDaemon, remote
package-manager action, or Mac mini NewAPI route was used.

## Protected Artifact Integrity

The source revision first passed every required CI leg, including the full
database suite, race tests, OpenClaw and Hermes checks, native AMD64/ARM64
service and package jobs, and APT/DNF repository lifecycle jobs. Only then did
the independent `sign-snapshot` job receive OIDC permission and sign the
release manifest.

| Field | Value |
|---|---|
| Signed artifact ID | `8548456074` |
| Artifact name | `vermory-pr-snapshot-e8d2bd34ee63e4cb923287baaed1275061ef5e8b` |
| Stored bytes | `66,380,867` |
| GitHub artifact SHA-256 | `3c51c7b846eb669a7bb35356b538352d8f5eeabfac0b915916afa1034f3ce5c8` |
| Manifest entries | `16` |
| `release-manifest.sha256` file SHA-256 | `05e0b8f7251e9c9ce6c8df9f8085e06724c798cd78d9cb4764f2fe9c8aad1817` |
| Sigstore bundle SHA-256 | `31567d83ae7ec09dc140a85b6e8b9ea845f566e71f8217fc2cf8099ac4806384` |
| Darwin ARM64 archive SHA-256 | `ecc0620538527ee3145a5f569bf93c3eb04c12e68288b227459648e268517a77` |
| Candidate binary SHA-256 | `6fbe9057986a36ef989ad0804c2f7f673af2d185cfe6bfb1b471a50c0489478d` |
| Exact-tree runner bundle SHA-256 | `0aaa07103b93234799bf75b69584d1389ebbbd740850fccbe7e8c846dc026215` |
| Final runtime report SHA-256 | `6927f71866e416665a6a1e0b5fdb38fe4f505e9e3f3d70d3d7945ebb7d171e7e` |

`scripts/release-manifest.sh verify` accepted every payload. Cosign v3.0.6
returned `Verified OK` for workflow identity
`https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge`
and issuer `https://token.actions.githubusercontent.com`. The extracted binary
reported the exact qualified revision before it was transferred. Local and
remote binary hashes matched.

The runner bundle is not presented as a separately signed release payload. It
was created from the exact qualified Git tree, checksum-bound before and after
transport, and used only to execute the frozen public I11 contract against the
signed candidate.

## Lifecycle And State Result

The base binary migrated an isolated database to schema 24, granted a restricted
runtime role, installed the first authenticated service, and created one active
governed default plus one digest-backed operator token. The baseline authority
counts were:

```text
observations | active governed memories | search documents | active tokens
1            | 1                         | 1                | 1
```

The candidate passed version and database compatibility before any live-file
replacement. A deliberately incompatible candidate then failed preflight and
left the live binary unchanged. The accepted candidate replaced the stable
binary, runner, and mode-0600 environment through atomic paths while retaining
the complete base installation in one mode-0700 rollback slot.

An incompatible rollback binary was rejected before the running candidate was
stopped or replaced. A compatible explicit rollback restored the complete base
installation and consumed the rollback slot. The candidate was then installed
again. A deliberately unhealthy replacement failed activation, restored the
previous complete candidate installation automatically, and verified the
restored service before returning failure.

The final authority counts were `3|1|1|1`. The two additional observations came
from the isolated OpenClaw and Hermes backend prepare probes; the governed
default, search document, and active token remained singular and current.

## Security And Cleanup Result

| Check | Result |
|---|---:|
| Environment file mode | `0600` |
| Protected current and rollback environment files | `2` |
| Credential matches in LaunchAgent plist | `0` |
| Credential matches in non-secret runtime files | `0` |
| Unauthenticated session response | `401` |
| Authenticated root and session health | pass |
| OpenClaw backend prepare probe | pass |
| Hermes backend prepare probe | pass |

After report creation and process exit, independent host inspection found:

```text
LaunchAgent plist: absent
launchd service: unloaded
I11 database: absent
I11 runtime role: absent
I11 runtime root: absent
TCP listener on 18811: absent
```

The report checksum matched on the Mac mini and after transfer to the external
evidence disk.

## Hard Gates

All 20 frozen gates passed:

1. candidate version preflight;
2. candidate read-only database compatibility preflight;
3. incompatible candidate made no live change;
4. unprivileged user LaunchAgent with no `sudo`;
5. loopback-only listener;
6. protected environment and credential-free plist;
7. complete protected rollback slot;
8. partial current installation rejected;
9. atomic replacement through stable paths;
10. candidate root health returned `200`;
11. unauthenticated session returned `401`;
12. failed candidate activation restored the previous installation;
13. automatic restoration verified the previous service;
14. explicit rollback performed database compatibility preflight;
15. incompatible rollback left the candidate running;
16. compatible explicit rollback restored binary, runner, and environment;
17. successful explicit rollback consumed the rollback slot;
18. authoritative tenant, continuity, memory, audit, token, and projection state survived lifecycle changes;
19. the Mac mini runtime remained isolated and provider-independent;
20. the normalized report was checksum-bound and credential-free.

## Retained Failure

The preceding exact-head v5 run on revision
`3cc50b98dc85b2fbb19452e122de4bc90b946949` completed its 20 recorded runtime
gates, but post-run inspection found an unloaded I11 plist still present in
`~/Library/LaunchAgents`. Inspection also found the same residue from the
earlier v4 debug run. Neither run is accepted as final I11 qualification.

Revision `e8d2bd34ee63e4cb923287baaed1275061ef5e8b` added a regression assertion
and removed the plist in the runner cleanup path. The focused test failed before
the fix, passed after it, and v6 independently proved zero service, database,
role, runtime, plist, and listener residue. The rejected v5 report remains in
the operator's external-disk failure archive with SHA-256
`5d97f931ce0b04e5b35f5df0eb10c9f36a114d7e37e89e069ab97ea79d167213`.

## Claim Boundary

I11 qualifies one frozen base-to-candidate authenticated user-service lifecycle
on this ARM64 Mac mini: exact artifact binding, read-only compatibility
preflight, unprivileged loopback startup, upgrade, restart, incompatible-change
rejection, explicit rollback, failed-activation restoration, governed-state
preservation, credential scanning, and residue-free cleanup.

It does not qualify:

- PostgreSQL down migration or arbitrary historical-version rollback;
- zero-downtime restart or a long-duration service-level objective;
- database backup, restore, high availability, or disaster recovery;
- model availability or model quality;
- public Internet exposure, macOS notarization, or package-manager delivery;
- a system-wide LaunchDaemon or privileged installation;
- real OpenClaw or Hermes client behavior on this run.

The OpenClaw and Hermes checks in I11 prove only that their authenticated Vermory
backend endpoints remained usable after the lifecycle. Their real-client
qualifications remain grounded in their separate client evidence.
