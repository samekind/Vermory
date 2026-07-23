# PostgreSQL HA And PITR Qualification Evidence

Date: 2026-07-16

Revision under test: `423caeba93086c92af653d38112b4ea1b8f2dce4`

Formal run ID: `postgresql-ha-pitr-20260716-v1`

## Scope

This evidence covers the frozen `I03-postgresql-ha-pitr` operations case:

- PostgreSQL 18 physical streaming replication;
- immediate loss of the dedicated writable primary;
- promotion of a streamed standby;
- recovery through the same authenticated Web Chat handler, runtime store, and authentication pool;
- exact-LSN point-in-time recovery from a physical base backup and WAL archive;
- explicit separation between historical state restoration and current state reconciliation;
- rebuild of disposable lexical/vector projection state;
- re-revocation of a historically restored token before accepting traffic;
- RLS, tenant-aware foreign keys, token lifecycle, and zero false-receipt gates after recovery.

No embedding or chat model was used to judge these gates. The Web Chat path used Vermory's deterministic mock provider because a model response cannot prove WAL replay, row persistence, RLS, deletion boundaries, or credential revocation.

## Environment

```text
Go test host: darwin/arm64
PostgreSQL server and tools: 18.4
Topology: three dedicated same-host PostgreSQL processes
Network: dynamic TCP ports bound only to 127.0.0.1
Unix sockets: disabled for the profile
Authority: PostgreSQL
```

The profile root was isolated under `/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr`. It did not inspect, stop, copy, or modify any existing PostgreSQL service or user data directory.

## Formal Command

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/postgresql-ha-pitr-20260716-v1 \
VERMORY_HA_PITR_RUN_ID=postgresql-ha-pitr-20260716-v1 \
VERMORY_HA_PITR_IMPLEMENTATION_REVISION=423caeba93086c92af653d38112b4ea1b8f2dce4 \
go test -count=1 ./internal/operationsprofile \
  -run '^TestPostgreSQLHAPITRProfile$' -v
```

The first invocation completed the profile and wrote deterministic JSON and Markdown reports. A second invocation with the same identity replayed the completed report in `0.03s` without creating PostgreSQL clusters.

## Failover Result

The primary migrated to schema 15, created a restricted runtime role, seeded two tenant-isolated continuities, issued active and revoked control tokens, created a physical PITR base backup, and created a standby with `pg_basebackup -R -X stream`.

Before failure, the standby replay LSN reached the primary flush LSN:

| Measurement | Result |
|---|---:|
| primary system identifier | `7662866837654329699` |
| standby system identifier | `7662866837654329699` |
| primary flush LSN | `0/5000000` |
| standby replay LSN | `0/5000000` |
| pre-failover operation rows | `1` |
| transition operation rows | `0` |
| post-promotion operation rows | `1` |
| promotion time | `171 ms` |
| same-handler reconnect time | `234 ms` |

The failure was injected with `pg_ctl stop -m immediate` against only the dedicated primary. The transition Web Chat request returned `503`, a zero-value receipt, and zero `conversation_turns` rows. After promotion, the same handler object, runtime store, runtime pool, authentication pool, and token authenticator completed a new authenticated turn. The promoted server was out of recovery and read-write.

The measured times describe one local same-host run. They are not availability or latency SLOs.

## Exact-LSN Recovery Result

The authority timeline was:

```text
T0  physical base backup
T1  fact A active; token A active
T2  fact B active; authenticated pre-failover turn committed
    target LSN captured: 0/402ACE0
T3  fact B deleted; token A revoked
T4  fact C committed
```

The restore copied the T0 base into a fresh data directory and used:

```text
restore_command = 'cp <dedicated-wal-archive>/%f %p'
recovery_target_lsn = '0/402ACE0'
recovery_target_timeline = 'current'
recovery_target_inclusive = on
recovery_target_action = promote
```

PostgreSQL logged:

```text
recovery stopping after WAL location (LSN) "0/402ACE0"
selected new timeline ID: 3
```

The replay position after promotion was `0/402CCD8`, and the authoritative T2 fingerprint matched byte-for-byte at the logical row level:

```text
T2 fingerprint:       4995748eaa075f985141f93ad82cdc058b2bb753ebfbbd87a61d5ad4e4a15103
restored fingerprint: 4995748eaa075f985141f93ad82cdc058b2bb753ebfbbd87a61d5ad4e4a15103
```

The restored authority contained fact A and fact B as active, did not contain fact C, did not contain the T3 deletion observation, and did not contain the T3 token revocation. This is the expected semantics of restoring historical truth.

## Projection And Credential Reconciliation

Before the restored instance was accepted for use:

1. both supported vector profile generations were reset for both tenants;
2. the lexical search projection was rebuilt from restored active governed memories;
3. exact and paraphrased probes returned restored fact B;
4. the post-target fact C had zero authority and projection rows;
5. the historically restored token A was shown to authenticate while the instance remained quarantined;
6. token A was revoked again with a new idempotent operation;
7. an authenticated Web Chat request with the old raw token returned `401` and no receipt;
8. a newly issued operator token completed an authenticated Web Chat request.

The final restored catalog contained `19` tenant-isolation RLS policies and `36` tenant-aware foreign keys. The restricted runtime role still validated after PITR.

## Physical Inventory

| Artifact | Measurement |
|---|---:|
| physical base backup bytes | `43,641,232` |
| measured WAL archive bytes | `83,886,830` |
| WAL inventory SHA-256 | `d0828cb4f58d1ddcae450e14a7f401b3a3fa92fc85a5ef1828143dc0b9c45080` |
| committed report JSON SHA-256 | `00596dc52ad6e3ac569e27387f912a8bcc0a2e1ce38245427622fc9c039c5999` |
| generated report Markdown SHA-256 | `6d3ce0fd0a39d3a7356b2645c78b98de33ca006e54798e091bfb440dbd06eace` |

The committed raw report snapshot is [2026-07-16-postgresql-ha-pitr.json](snapshots/2026-07-16-postgresql-ha-pitr.json).

## Failure Ledger

Failures were retained rather than removed from the final report:

| Phase | Attempt | Code | Resolution |
|---|---:|---|---|
| harness setup | 1 | `profile_root_permission_denied` | moved the dedicated root below a writable JSData workspace path |
| harness setup | 2 | `unix_socket_path_too_long` | changed the profile to loopback TCP-only and disabled Unix sockets |
| HA profile | 1 | `promoted_archive_command_non_idempotent` | made the archive command return success when a WAL file already exists |
| failover injection | 1 | `transition_database_unavailable` | expected failure retained; the operation produced zero receipt and zero rows |

## Cleanup

All three allocated TCP ports were verified closed after the formal run. No process command line referenced the formal profile root after cleanup. PostgreSQL logs and data directories remain outside Git for local audit; only the non-secret report snapshot and this evidence document are committed.

## Non-Claims

- Same-host PostgreSQL processes are not cross-host HA evidence.
- Vermory does not provide automatic leader election, split-brain prevention, a database proxy, or a consensus system.
- The measured timings are not universal SLOs.
- PITR does not automatically make restored historical state safe for current traffic.
- This qualification does not change the lexical retrieval default or resolve the W15 LongMemEval-S quality regression.
