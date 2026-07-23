# Durable Linux Service Lifecycle Qualification

Date: 2026-07-22

Reality case: `I05-durable-linux-service-lifecycle`

Status: `runtime-qualified`

Source revision: `e04bd0a0b97d833ee1d7bb593aa63dbbff6a2c7b`

GitHub Actions run: [`29916232568`](https://github.com/samekind/Vermory/actions/runs/29916232568)

Linux lifecycle job: [`88910623425`](https://github.com/samekind/Vermory/actions/runs/29916232568/job/88910623425)

Evidence level: `public`

The original AMD64 machine-readable result is retained in the
[I05 lifecycle snapshot](snapshots/2026-07-22-durable-linux-service-lifecycle.json).
The subsequent native ARM64 result is retained separately in the
[I05 ARM64 lifecycle snapshot](snapshots/2026-07-22-durable-linux-service-lifecycle-arm64.json),
so the earlier evidence is not rewritten.

## Qualified Contract

The exact source head completed this trajectory on a GitHub-hosted Ubuntu 24.04
AMD64 runner with systemd and PostgreSQL 18:

```text
checksummed release archive
-> root-owned versioned release
-> dedicated non-login service identity
-> root-only environment file and restricted PostgreSQL runtime role
-> authenticated loopback service boundary
-> successful upgrade with governed state retained
-> failed upgrade with automatic binary rollback
-> bidirectional explicit rollback
-> private native PostgreSQL custom backup
-> checksum and non-empty-target rejection
-> empty-target restore
-> runtime-role grants and projections rebuilt
-> restored token and governed default consumed through the service
-> unit, process arguments, journal, and normalized report credential scan
```

The lifecycle job passed in 1 minute 35 seconds. No model-quality claim is
needed or made for this operations case.

The same exact-head run also passed the ordinary `test` job `88910623466` and
the dependent OIDC `sign-snapshot` job `88911563651`. The latter is delivery
evidence for the source head; the I05 status itself is grounded in the separate
Linux lifecycle job and normalized report.

## Artifact Integrity

| Field | Value |
|---|---|
| Artifact ID | `8528018011` |
| Artifact name | `vermory-i05-linux-service-e04bd0a0b97d833ee1d7bb593aa63dbbff6a2c7b` |
| Stored bytes | `1,336` |
| GitHub artifact SHA-256 | `abb6af2f1968cfdacb343b031ee7d6acd1f4d7901e56ba6255f1ef10733bd200` |
| Downloaded ZIP SHA-256 | `abb6af2f1968cfdacb343b031ee7d6acd1f4d7901e56ba6255f1ef10733bd200` |
| Extracted `report.json` SHA-256 | `fa98f10f18fd58d8842387b727247eee0821deabb59362c26dc0e3e35a395640` |
| Retention | seven days |

The downloaded ZIP contained exactly one file, `report.json`. Its `source_sha`
exactly matched the workflow head. Independent validation required all 16 named
hard gates to exist and equal `true`; it also checked the release and backup
digests, non-empty backup size, schema version, service boundary, and restored
state. A separate credential scan found no database URL, password, bearer value,
API key, access token, or raw token field.

## Native ARM64 Qualification

Source revision `4be21afb6d829c2c63161475e24b4ba58d8a8204` extended the same
frozen I05 contract to GitHub's native Ubuntu 24.04 ARM64 runner. Protected run
[`29918264712`](https://github.com/samekind/Vermory/actions/runs/29918264712)
completed the native ARM64 job
[`88917132816`](https://github.com/samekind/Vermory/actions/runs/29918264712/job/88917132816)
in 1 minute 30 seconds. The job verified `uname -m == aarch64`, Go host and
target architecture `arm64`, and recorded `runtime.native == true` before the
report could pass.

The ARM64 artifact `8528827014`, named
`vermory-i05-linux-service-arm64-4be21afb6d829c2c63161475e24b4ba58d8a8204`,
was 1,371 stored bytes. Its GitHub digest and independently downloaded ZIP
SHA-256 both equal
`983c9b32c6d0291978a6c52e5676dc8d685b3f5cb62d5eb0a879a56bfebe10cb`.
The extracted report SHA-256 is
`f41fe1d54d035beb3d6cc6509bf0e15cb3d13a67c095feb694be02476bd84fcf`.
It bound all 16 hard gates to the exact source head, PostgreSQL schema version
23, a non-empty native custom backup, the hardened systemd service, restored
authentication/default access, and a clean credential scan.

The same run independently passed AMD64 lifecycle job `88917132835`, full test
job `88917132916`, and dependent signing job `88917909918`. Signed snapshot
artifact `8528914598` had GitHub and downloaded ZIP SHA-256
`5002eafb88d3a6ac49d099e714ae8d09af10434c8a3889abb00ea3dcde1096b5`.
All eight release-manifest payloads verified, and Cosign verification returned
`Verified OK` for the PR `ci.yml` identity and GitHub Actions OIDC issuer. The
signed Linux ARM64 payload is a static AArch64 ELF; this artifact check is
delivery evidence, while the native systemd lifecycle report is the runtime
qualification evidence.

## Service And Release Evidence

The service ran as `vermory-i05:vermory-i05`, listened on
`127.0.0.1:18788`, and returned HTTP `401` without authentication. The recorded
systemd properties were:

| Property | Value |
|---|---|
| `NoNewPrivileges` | `yes` |
| `PrivateTmp` | `yes` |
| `ProtectHome` | `yes` |
| `ProtectSystem` | `strict` |
| `RestrictSUIDSGID` | `yes` |
| `RestrictAddressFamilies` | `AF_INET AF_INET6 AF_UNIX` |

The initial and upgraded binaries had different SHA-256 values. Upgrade to
`i05-v2` preserved governed state. Activating the deliberately failing release
was rejected, restored `i05-v2` automatically, and retained a hash of the
failure log. Two explicit rollback operations then moved `v2 -> v1 -> v2`.

## Backup And Restore Evidence

The native PostgreSQL custom backup was `164,357` bytes with SHA-256
`c4616a775ec06eea2e01c05312af8dd542a434ffa13159968d02ddfd8be8a054`
at schema version `23`. The run rejected a modified dump and rejected restore
into a non-empty target. Restore into an empty database then:

- restored the authoritative PostgreSQL state;
- verified the pre-existing restricted runtime role and re-applied runtime grants;
- replayed migrations and rebuilt disposable projections;
- restarted the systemd service against the restored database;
- authenticated with the restored digest-backed token;
- returned the governed deployment marker through the authenticated API.

The dump format is not encrypted. The run proves private file modes and digest
verification, not backup encryption at rest.

## Hard Gates

All recorded gates passed:

1. exact source head;
2. dedicated service identity;
3. restricted runtime database role;
4. root-owned versioned releases;
5. authenticated loopback boundary;
6. successful upgrade preserved state;
7. failed upgrade rolled back;
8. explicit rollback was bidirectional;
9. native private backup;
10. release or backup digest mismatch rejected;
11. non-empty restore target rejected;
12. empty restore target accepted;
13. runtime role re-provisioned;
14. projections rebuilt;
15. restored token and governed default verified;
16. unit, process, journal, and normalized report remained credential-free.

## Retained Failures

Four exact-head failures remain in the public GitHub Actions history:

1. run `29914962654` exposed unsupported `psql -c` variable interpolation;
2. run `29915243780` exposed Cobra flag defaults overwriting protected service environment defaults;
3. run `29915671095` exposed Ubuntu package suffixes in `pg_dump --version`;
4. run `29915968160` completed the lifecycle but exposed root-only traversal on the sanitized evidence directory.

Each failure produced a scoped fix and a new exact-head run. None is counted as
a pass or removed from the evidence history.

## Claim Boundary

I05 qualifies this exact Ubuntu AMD64 and native ARM64 systemd lifecycle:
installation, service hardening, authenticated loopback startup, upgrade,
binary rollback, native backup, empty-target restore, runtime-role recovery,
projection rebuild, restored
authentication, and credential scans.

It does not qualify long-duration uptime or an SLA, DEB/RPM or
package-repository distribution, cross-host production failover, encrypted
backups, or reversal of PostgreSQL migrations during binary rollback.
