# PostgreSQL HA And PITR Qualification Design

Date: 2026-07-16

Status: execution boundary under the active Vermory goal

## Goal

Prove that a production-shaped Vermory deployment can preserve its PostgreSQL
authority contract across two failures that the existing logical
backup/restore evidence does not cover:

1. immediate loss of a writable primary followed by promotion of a streaming
   standby; and
2. physical point-in-time recovery to a named WAL position before a later
   destructive transaction.

This is an operations qualification. It does not add a Vermory-owned consensus
system, failover controller, backup format, or database proxy.

## Why This Slice Comes Next

W15 retained a real public-benchmark regression instead of tuning it away.
That quality result remains open work. The next implementation slice should
therefore close a separate explicit platform risk without changing retrieval
against the same public labels.

The repository already proves logical dump/restore, runtime connection
recovery, projection rebuild, tenant RLS, and Linux binaries. It does not prove
physical replication, primary promotion, WAL replay to a target point, or the
security consequences of restoring historical authority. Those are concrete
deployment boundaries in the active goal and can be tested without inventing
new memory semantics.

## Non-Goals

- No automatic leader election or split-brain prevention service.
- No Patroni, etcd, Consul, Kubernetes operator, or cloud-specific database
  dependency.
- No universal RPO, RTO, throughput, or durability claim.
- No cross-region or network-partition claim.
- No claim that same-host PostgreSQL processes equal independent physical
  machines.
- No destructive testing against the user's existing PostgreSQL service.
- No silent reuse of a PITR-restored instance as current production state.
- No change to the lexical retrieval default or to W15 benchmark results.

## Frozen Reality Case

Add `I03-postgresql-ha-pitr` as a public synthetic operations case. Synthetic
input is appropriate because real outage credentials and user database files
must not enter the repository.

The case freezes these pressures:

- `physical_streaming_replication`;
- `primary_immediate_failure`;
- `standby_promotion`;
- `multi_host_runtime_reconnect`;
- `wal_archive`;
- `pitr_target_lsn`;
- `historical_deletion_resurrection`;
- `historical_token_resurrection`;
- `post_recovery_projection_rebuild`;
- `post_recovery_credential_governance`.

Expected current behavior and forbidden behavior are recorded before the
profile harness is implemented. The fixture contains no passwords, raw API
tokens, host-specific data directories, or user database names.

## Dedicated Test Topology

The qualification creates only dedicated PostgreSQL 18 clusters under one
profile root:

```text
profile-root/
  primary/
  standby/
  pitr-base/
  pitr-restored/
  wal-archive/
  logs/
  artifacts/
```

The real evidence run places this root on `/Volumes/JSData` rather than the
nearly full system volume. Automated tests may use `t.TempDir()` when space is
sufficient.

Each cluster listens only on `127.0.0.1` with a dynamically allocated port and
sets `unix_socket_directories=''`. TCP-only loopback avoids platform Unix socket
path-length limits while keeping the disposable profile unreachable from
non-loopback interfaces. The generated `pg_hba.conf` trusts only local loopback
connections inside these dedicated disposable clusters. No existing Homebrew
service is stopped or reconfigured.

The harness requires PostgreSQL 18 tools through
`VERMORY_POSTGRES_BIN_DIR`. It invokes `initdb`, `pg_ctl`, `pg_basebackup`,
`pg_controldata`, and `psql` directly and records tool versions without
recording connection secrets.

## Primary And Standby Contract

The primary enables:

```text
wal_level=replica
max_wal_senders>=4
max_replication_slots>=4
hot_standby=on
archive_mode=on
archive_command=<copy each completed WAL file once into wal-archive>
```

The standby is created with `pg_basebackup -R -X stream`. Before failover, the
harness waits until the standby replay LSN is at or beyond the primary flush
LSN containing the last accepted pre-failover Vermory operation.

Vermory connects through a multi-host PostgreSQL DSN with
`target_session_attrs=read-write` and a bounded connect timeout. The same
`pgxpool.Pool` and the same service instance are retained during failure. The
profile must not replace the service with a new process merely to make the
post-promotion request succeed.

The failure is injected with `pg_ctl stop -m immediate` against the dedicated
primary. During the transition:

- a request may fail with a bounded database error;
- no failed request may return a non-zero successful receipt;
- no failed operation ID may appear as a committed turn or memory row.

After `pg_ctl promote`, the harness waits for the standby to report
`pg_is_in_recovery() = false` and read-write transaction status. The existing
Vermory pool must then complete a new authenticated request without process
restart. The promoted cluster must retain:

- current continuity and governed memory state;
- active and revoked token metadata;
- tenant-aware foreign keys and all RLS policies;
- the pre-failover committed operation exactly once;
- zero rows for the transition failure operation;
- successful persistence of one post-failover operation.

## PITR Timeline

The physical backup and WAL archive use an explicit authority timeline:

```text
T0  base backup starts from the primary
T1  active fact A and active token A exist
T2  fact B is committed and standby/archive durability is confirmed
    -> record target LSN after T2
T3  fact B is deleted and token A is revoked
T4  a later fact C is committed
```

The restore copies the physical base backup into a new dedicated data
directory, configures `restore_command`, creates `recovery.signal`, and sets
`recovery_target_lsn` to the exact LSN captured after T2. It uses
`recovery_target_action=promote` and a separate loopback port.

The restored cluster is accepted only when:

- recovery reaches the requested LSN and exits recovery;
- fact A and fact B are present in their T2 lifecycle state;
- the T3 deletion, T3 token revocation, and T4 fact C are absent;
- schema, RLS, tenant foreign keys, and runtime role attributes are valid;
- authoritative counts and a target-state fingerprint match the frozen T2
  snapshot;
- projection tables are explicitly reset and rebuilt from restored authority;
- exact and paraphrased current-fact probes include restored active facts and
  exclude facts that did not exist at T2.

## Historical-State Security Boundary

PITR is expected to restore historical truth, not current truth. If deletion or
token revocation occurred after the target LSN, the restored cluster will
correctly contain the earlier active fact or token metadata. This is not a
Vermory deletion failure and must not be hidden.

The restored instance therefore starts in an operator-quarantined state. The
qualification requires an explicit post-recovery governance phase before the
runtime is considered serviceable:

1. inspect the target timestamp/LSN and restored authority fingerprint;
2. reset and rebuild disposable projections;
3. revoke the historically restored application token;
4. verify the old raw token now receives `401`;
5. issue a new token and verify authenticated access;
6. record that later legitimate deletions or corrections must be replayed or
   re-applied according to the incident plan before external traffic resumes.

The evidence must distinguish `historical_state_restored` from
`current_state_reconciled`.

## Profile Harness

Add an opt-in integration profile under `internal/runtime`. The default test
suite never creates PostgreSQL clusters. The profile runs only when:

```text
VERMORY_HA_PITR_PROFILE=1
VERMORY_POSTGRES_BIN_DIR=/path/to/postgresql-18/bin
VERMORY_HA_PITR_ROOT=/dedicated/profile/root
```

The harness is test-owned orchestration, not a runtime dependency. It may add
small reusable report types under `internal/runtime` when needed, but process
control helpers remain in `_test.go` files unless a real operator command needs
them.

The profile writes atomic JSON checkpoints after cluster initialization,
replication catch-up, promotion, target-LSN capture, PITR recovery, projection
rebuild, and credential re-governance. A repeated run with the same run ID and
matching request fingerprint may reuse terminal checkpoints; conflicting reuse
is rejected.

All PostgreSQL process logs remain outside Git. Committed evidence contains
only versions, ports if useful, LSNs, durations, counts, fingerprints, hashes,
exit statuses, and non-secret failure categories.

## Hard Gates

- PostgreSQL server and client tools are version 18.x.
- Primary and standby system identifiers match before promotion.
- Standby replay reaches the required pre-failover LSN.
- Immediate primary failure produces no false successful receipt.
- The same Vermory pool and service recover after standby promotion.
- Pre-failover committed data appears exactly once after promotion.
- Post-failover writes persist and remain tenant-isolated.
- The promoted standby is read-write and no longer in recovery.
- WAL archive contains every segment required for the target restore.
- PITR reaches the exact target LSN and excludes all later transactions.
- Restored authority matches the frozen T2 target fingerprint.
- Projection rebuild is derived only from restored active authority.
- Restored historical token access is blocked before service acceptance.
- A newly issued token works after re-governance; the historical token fails.
- No unrelated PostgreSQL process or user data directory is modified.

## Measured Outputs

The report records:

- PostgreSQL and Vermory revision identifiers;
- primary, standby, and restored system identifiers;
- primary flush, standby replay, target, and restore LSNs;
- base-backup and WAL archive byte counts and SHA-256 inventory digest;
- failover detection, promotion, reconnect, and PITR durations;
- authoritative row counts and fingerprints at T2, promoted standby, and PITR;
- RLS, foreign-key, token, turn, memory, and projection assertions;
- every injected failure and whether it produced a receipt or row;
- final hard-gate status and explicit non-claims.

Timings describe one same-host qualification profile. They are not published as
universal SLOs.

## Delivery Boundary

W16 is complete only after:

- the frozen I03 case validates;
- opt-in HA and PITR profile tests pass from a clean dedicated root;
- committed evidence records failures as well as successes;
- backup/recovery documentation explains historical deletion and credential
  resurrection;
- the full local release gates pass;
- a protected Draft PR head passes CI and its GitHub artifact is independently
  verified.

The overall Vermory goal remains active after W16 for genuine external sealed
evaluation, long-duration retention, active-backlog dimensional migration,
artifact signing, and final release acceptance.
