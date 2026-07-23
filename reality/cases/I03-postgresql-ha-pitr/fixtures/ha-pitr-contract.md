# PostgreSQL HA and PITR qualification fixture

All cluster names, ports, roles, tenants, continuity identifiers, facts, and
token metadata used by this case are synthetic and dedicated to one disposable
qualification run.

## Authority boundary

PostgreSQL remains authoritative for continuity bindings, observations,
governed memory lifecycle, delivery history, bridge state, token digest
metadata, revocation state, and migration history. Search documents and vector
rows are disposable projections derived from governed active memory.

Vermory does not own leader election, replication consensus, WAL transport, or
physical backup formats. The qualification uses PostgreSQL 18 streaming
replication, native WAL archiving, physical base backup, standby promotion, and
target-LSN recovery.

## Dedicated topology

The run creates one primary, one streaming standby, one physical point-in-time
base backup, one restored cluster, one WAL archive, private socket directories,
and process logs inside a dedicated profile root. Each server listens only on
loopback and its dedicated socket. No existing database service or user data
directory is modified.

## Failover trajectory

1. Initialize the dedicated primary and apply the complete Vermory migration
   set.
2. Create one replication role and one non-owner runtime role with RLS
   enforcement.
3. Seed an active fact, an active application token, and a revoked control
   token.
4. Create the physical point-in-time base backup and a streaming standby.
5. Commit a second active fact and one authenticated pre-failover turn.
6. Wait until standby replay reaches the primary flush position for the last
   committed operation.
7. Stop the primary in immediate mode and attempt one bounded request. It may
   fail, but it must not return a successful receipt or create a row.
8. Promote the standby and wait until it is read-write.
9. Reuse the same Vermory runtime store, authentication pool, and HTTP handler.
   A new authenticated request must succeed without process restart.
10. Verify the pre-failover and post-promotion operations exactly once, the
    transition operation zero times, and tenant isolation under the promoted
    primary.

## Point-in-time timeline

The recovery timeline is fixed:

```text
T0  physical base backup starts
T1  fact A and token A are active
T2  fact B is committed and durable
    capture the target WAL position after T2
T3  fact B is deleted and token A is revoked
T4  fact C is committed
```

The restored cluster replays archived WAL from the physical base backup to the
captured T2 position and promotes. It must contain fact A, fact B, and active
token-A metadata. It must not contain the T3 deletion, T3 revocation, or T4
fact C.

## Historical-state quarantine

Point-in-time recovery restores historical truth, not automatically reconciled
current truth. A deletion or token revocation that occurred after the target
position is expected to be absent from the historical restore. The restored
instance therefore remains quarantined until an operator:

1. verifies the target position and authority fingerprint;
2. resets and rebuilds disposable projections;
3. revokes historically restored token-A metadata;
4. verifies the old raw token is rejected;
5. issues a new token and verifies authenticated access;
6. records which later legitimate corrections or deletions must be replayed
   before external traffic resumes.

## Required exclusions

- no automatic leader-election or split-brain-prevention claim;
- no cross-region or independent-machine claim from same-host processes;
- no raw token, login password, private connection string, or user data path in
  fixtures or committed evidence;
- no successful receipt for an uncommitted transition request;
- no projection row treated as authoritative state;
- no PITR-restored historical state described as reconciled current state;
- no universal RPO, RTO, latency, or durability claim from one profile run;
- no change to the lexical retrieval default or retained W15 benchmark result.
