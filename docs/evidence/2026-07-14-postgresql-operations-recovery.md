# PostgreSQL Operations And Recovery Evidence

Date: 2026-07-14

Revision under test: `b8185976fb1be8495868792b02927c20a71709d0`

## Scope

This evidence covers the `I02-postgresql-operations-recovery` contract:

- idempotent migration replay at schema 9;
- native PostgreSQL custom-format dump and restore;
- source/target authoritative-state equivalence;
- independent target runtime-role provisioning;
- restored active/revoked token behavior;
- projection loss and rebuild from active governed memories;
- RLS filter omission and cross-tenant foreign-key rejection after restore;
- bounded database outage with no false success receipt and same-process pool recovery.

No external LLM was used. The HTTP authentication probe used Vermory's mock provider because model output cannot prove restore correctness, authorization, deletion, or isolation.

## Environment

```text
Go: go1.26.5 darwin/arm64
PostgreSQL server: 18.4
pg_dump: 18.4
pg_restore: 18.4
source database: vermory_ops_i02_source_20260713222124
target database: vermory_ops_i02_target_20260713222124
```

Both databases were dedicated synthetic databases on the same local PostgreSQL cluster. The restore therefore proves native logical dump portability and role re-provisioning within PostgreSQL 18.4; it does not claim cross-host disaster recovery, streaming replication, point-in-time recovery, or high availability.

## Deterministic Runtime Acceptance

The automated acceptance was executed with:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestOperationsRecovery' -v
```

Result: `PASS`.

The test proves:

- applying migrations twice remains at schema 9;
- the authority fingerprint is unchanged by projection loss and rebuild;
- only active governed memories return to the projection;
- active token metadata authenticates and revoked token metadata does not;
- the restricted runtime role validates after close and reopen;
- a dedicated database with `ALLOW_CONNECTIONS false` and terminated sessions returns an error and a zero receipt for the interrupted operation;
- after connections are re-enabled, the same runtime pool persists a new request without process restart;
- the interrupted operation leaves zero `conversation_turns` rows.

## Native Dump And Restore

The source was dumped with native PostgreSQL tooling:

```bash
pg_dump \
  --format=custom \
  --no-owner \
  --no-acl \
  --file=/secure/path/vermory.dump \
  "$VERMORY_ADMIN_DATABASE_URL"
```

`--no-owner` prevents source object ownership from becoming a target prerequisite. `--no-acl` prevents grants to source-cluster roles from being replayed. The target runtime role was created and granted separately after restore.

| Check | Result |
|---|---:|
| `pg_dump` exit | `0` |
| dump bytes | `79,812` |
| dump SHA-256 | `8de114b4bfe58683d093b8c59a65b21e793a3b7c690935880d7a9e58639fc779` |
| `pg_restore --exit-on-error` exit | `0` |
| target migration replay exit | `0` |
| source schema version | `9` |
| target schema version | `9` |

PostgreSQL custom format is compressed and structured, but it is not encrypted. Operators must encrypt and access-control the artifact during storage and transfer.

## Authority Equivalence

The authoritative fingerprint includes continuity spaces and bindings, observations, governed memories, deliveries, conversation turns, durable bridge tables, token digest metadata, and goose migration history. It intentionally excludes `memory_search_documents` because that table is a disposable projection.

| Check | Source | Restored target |
|---|---:|---:|
| authority fingerprint | `74fdfd024e6cd2a0301cf0aa2c71a947` | `74fdfd024e6cd2a0301cf0aa2c71a947` |
| active memories | `3` | `3` |
| superseded memories | `1` | `1` |
| deleted memories | `1` | `1` |
| active token metadata | `1` | `1` |
| revoked token metadata | `1` | `1` |
| bridge operations | `1` | `1` |
| completed turns | `1` | `1` |
| RLS tables | `12` | `12` |
| RLS policies | `12` | `12` |
| tenant-aware foreign keys | `22` | `22` |
| tenant-FK catalog fingerprint | `483b9f267fd47ac76673dc7641fbab08` | `483b9f267fd47ac76673dc7641fbab08` |

## Projection Recovery

The restored target began with three active projection documents. The projection was then deleted deliberately and rebuilt through the release binary:

```bash
./bin/vermory database rebuild-projections \
  --database-url "$VERMORY_ADMIN_DATABASE_URL"
```

| State | Projection rows | Authority fingerprint |
|---|---:|---:|
| after restore | `3` | `74fdfd024e6cd2a0301cf0aa2c71a947` |
| after projection loss | `0` | `74fdfd024e6cd2a0301cf0aa2c71a947` |
| after rebuild | `3` | `74fdfd024e6cd2a0301cf0aa2c71a947` |

Exact target checks after rebuild returned:

```json
{
  "active_projection": 1,
  "stale_projection": 0,
  "deleted_projection": 0
}
```

The rebuild is transactional and derives documents only from `governed_memories.lifecycle_status = 'active'`.

## Restored Runtime Boundary

The target runtime role was created after restore and provisioned with `database grant-runtime`.

```json
{
  "can_login": true,
  "superuser": false,
  "bypass_rls": false,
  "owned_served_tables": 0,
  "can_execute_auth": true,
  "can_read_auth_table": false,
  "can_read_legacy_table": false
}
```

Filter-omission probes under the restored runtime role returned:

```json
{
  "missing_context_before": 0,
  "tenant_a_visible": "ops-a",
  "tenant_b_visible": "ops-b",
  "missing_context_after": 0
}
```

An attempted tenant-B binding to a tenant-A continuity was rejected by the restored tenant-aware foreign key.

The authenticated loopback server then ran against the restored target runtime DSN:

- restored active token: HTTP `200`, `in_progress` receipt, turn and continuity IDs present;
- restored revoked token: HTTP `401`;
- revoked-token operation rows: `0`;
- stale or deleted facts in the active-token context: `0`.

Raw token values, token digests, runtime passwords, and connection strings were not printed into this report or committed artifacts.

## Operator Boundary

- Raw token secrets never enter PostgreSQL and cannot be recovered from a dump.
- Token digests are sensitive authentication material. Backup disclosure requires incident review and normally token revocation/reissue.
- PostgreSQL roles and passwords are cluster-level state and require separate bootstrap or infrastructure automation.
- Restore acceptance requires runtime-role validation, RLS probes, token lifecycle checks, and projection rebuild; a successful `pg_restore` exit alone is insufficient.
- Uninstalling Vermory or a client integration does not implicitly delete the PostgreSQL database or backup artifacts.

## Cleanup

After evidence capture, both dedicated `vermory_ops_i02_*` databases, the dedicated runtime role, the temporary dump, and the temporary release binary were removed. The loopback listener on port `8792` was stopped and verified free. The shared `vermory_test` database and unrelated evidence resources were not removed.

## Release Gate

Fresh verification after the recovery and embedded-migration changes passed:

```text
go test -p 1 -count=1 ./...: pass
go test -race on authn/runtime/webchat/identitycli/operatorcli/cmd/provider: pass
go vet ./...: pass
go mod tidy with zero go.mod/go.sum diff: pass
Darwin release build SHA-256: aacd8a3e90023b34898cfb044b7cdb01f453ad30390087181e5e55e8e662790e
OpenClaw check: pass
OpenClaw pack dry-run: pass
git diff --check: pass
Linux manifest hashes match generated artifacts: pass
```
