# Release And Database Compatibility Qualification

Date: 2026-07-22

Reality case: `I07-release-database-compatibility`

Status: `runtime-qualified`

Source revision: `45dbdee9a311322249f3d37bbc8e6551e7e8a285`

GitHub Actions run: [`29928032496`](https://github.com/samekind/Vermory/actions/runs/29928032496)

Evidence level: `public`

The normalized machine-readable result is retained in the
[I07 compatibility snapshot](snapshots/2026-07-22-release-database-compatibility.json).

## Qualified Contract

This exact binary declares the inclusive PostgreSQL schema interval `24..24`.
It performs a restricted schema compatibility preflight before constructing a
provider, opening the authentication pool, creating an HTTP listener, or
performing an application write.

The qualified decisions are:

| Database state | Decision | Exit | Service boundary |
|---|---|---:|---|
| schema `23` | `migration_required` | `1` | no listener |
| schema `24` | `compatible` | `0` | authenticated service listens |
| synthetic schema `25` | `binary_too_old` | `1` | no listener |
| unreadable version contract | `preflight_failed` | `1` | no listener |

The exact case contains 12 hard gates. All 12 passed in the local and
protected CI evidence described below.

## PostgreSQL 17 Local Evidence

The dedicated PostgreSQL 17.10 cluster ran on Apple ARM64 with its data and
Unix socket on the external volume:

```text
server: PostgreSQL 17.10 (Homebrew), aarch64-apple-darwin25.4.0
socket: /Volumes/JSData/.cache/vermory/postgres/socket
port: 55439
```

The three fixture databases returned these admin-path schema versions:

| Database | Applied schema | Business state (`observations:governed_memories:memory_deliveries`) |
|---|---:|---:|
| `vermory_i07_old_45dbdee` | `23` | `0:0:0` |
| `vermory_i07_current_45dbdee` | `24` | `0:0:0` |
| `vermory_i07_future_45dbdee` | `25` | `0:0:0` |

The exact binary at this revision produced stable JSON for all three states:

```text
schema 23 -> status=migration_required, exit=1
schema 24 -> status=compatible, exit=0
schema 25 -> status=binary_too_old, exit=1
```

No migration or application write was performed by the compatibility command.

## Restricted Runtime Role

The provisioned role was `vermory_i07_runtime_45dbdee`, a login role with
`NOSUPERUSER` and `NOBYPASSRLS`. Under that role:

```text
SELECT current_user, vermory_auth.schema_version() -> vermory_i07_runtime_45dbdee | 24
SELECT FROM public.goose_db_version                         -> permission denied
UPDATE public.goose_db_version                             -> permission denied
```

The schema inspection function is `STABLE`, `SECURITY DEFINER`, fixes
`search_path` to `pg_catalog`, and has no `PUBLIC` execute privilege. The
runtime role receives only the bounded function capability through the
explicit runtime-role provisioning path.

The exact local hashes are retained here so the result is tied to the tested
implementation and fixtures:

| Artifact | SHA-256 |
|---|---|
| exact local binary | `3c835c35091aa8eeb08a46fa12a8ed72b275b73579a186a6d6baf835d9dfe510` |
| I07 case contract | `64252dd27d84a9e6550d642d8f4174eec75aa0e6c519806e79f154e9569bc7c0` |
| migration `00024_schema_compatibility_boundary.sql` | `f19584226de626eecfb67570b7841768ee6444ab525d7cc9a9b717e61876366a` |

## Real `serve` Boundary

Using the restricted runtime role and schema `24`, the service listened on
`127.0.0.1:18797`; an unauthenticated request to `/v1/defaults` returned
`401`. The process then stopped cleanly and released the port.

The following negative runs used an intentionally invalid provider name. If
compatibility preflight had occurred too late, provider construction would
have produced a different error. Instead each run stopped at the preflight
boundary and left no listener:

| Run | Result | Listener |
|---|---|---|
| old schema `23` | `preflight_failed`, exit `1` | absent |
| future schema `25` | `preflight_failed`, exit `1` | absent |
| current schema with unreadable version role | `preflight_failed`, exit `1` | absent |

Ports `18797`, `18798`, `18799`, and `18800` were all free after the replay.
The three fixture databases remained at `0:0:0`, so the negative controls did
not write application state.

## Protected CI Evidence

The exact implementation head passed the protected CI run with these relevant
jobs:

| Job | ID | Result |
|---|---:|---|
| complete test and integration suite | `88950141037` | success |
| native Linux lifecycle, AMD64 | `88950140926` | success |
| native Linux lifecycle, ARM64 | `88950140995` | success |
| DEB package, AMD64 | `88950141022` | success |
| RPM package, AMD64 | `88950141063` | success |
| DEB package, ARM64 | `88950140992` | success |
| RPM package, ARM64 | `88950141176` | success |
| OIDC signed snapshot | `88951335584` | success |

The test job also passed the repository policy, PostgreSQL 18 full test suite,
runtime and reality race tests, `go vet`, actionlint, release binary build,
OpenClaw integration checks, Hermes checks, release manifest checks, and clean
diff verification. The package jobs checked that package lifecycle scripts
contain no database migration invocation or credential material. The service
unit has no migration pre-start hook.

## Independent Snapshot Verification

The GitHub artifact API reported artifact `8532953742` with digest
`sha256:16905ab13988fd2ad56835ae5c488a1a8cb5da5a1dfa168271167d0659e3806b`.
An independent download to the external volume produced the same SHA-256.
The signed payload manifest contained 12 entries: eight platform payloads,
the OpenClaw artifact, the Hermes archive and its checksum sidecar, and
`checksums.txt`. The downloaded archive also carried the manifest itself and
the Sigstore bundle used to verify it.

The local extracted manifest and bundle hashes were:

| Artifact | SHA-256 |
|---|---|
| `release-manifest.sha256` | `b243a330bbc272343d13229d4ae70751bfb523083136eb167d4a48cfd5ef213b` |
| `checksums.txt` | `73fc49531066a20e3d2f13e773f72ab7410f6ea8a3d789cedddabed2f0c251eb` |
| `release-manifest.sigstore.json` | `061d671c246ab5623b281f255c2882c7228b364ef1ad0a57f6cc280086671683` |

Cosign `v3.0.6` independently verified the manifest with:

```text
workflow identity:
https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge
OIDC issuer:
https://token.actions.githubusercontent.com
transparency log index: 2218281092
```

The positive verification passed. A modified manifest failed signature
verification, and the same unmodified manifest failed when checked against the
wrong `release.yml` workflow identity. These negative results are retained in
the external evidence workspace and summarized in the machine-readable snapshot.

## Rollback Boundary

This qualification intentionally does not claim automatic PostgreSQL down
migration. Binary-only rollback is valid only while the database schema remains
inside the older binary's declared support interval. Once a migration moves the
database outside that interval, recovery requires a verified PostgreSQL backup
or PITR restore point, followed by runtime-role provisioning and compatibility
inspection. Swapping a binary release pointer does not reverse a database
migration.

## Claim Boundary

I07 qualifies deterministic schema compatibility preflight, restricted version
inspection, fail-closed service startup, migration-free package/service
lifecycle, and the documented binary rollback/PITR boundary for the exact
schema-24 release profile.

It does not qualify a universal automatic down-migration mechanism, zero-downtime
database upgrades, long-duration uptime or an SLA, APT/DNF repository delivery,
tagged release publication, or rollback across arbitrary future schemas.
