# PostgreSQL Operations And Recovery Design

Date: 2026-07-14

Status: approved execution boundary under the active Vermory goal

## Goal

Prove that PostgreSQL remains Vermory's authoritative state across migration replays, native backup/restore, disposable projection loss, runtime-role re-provisioning, database connectivity failure, and Linux deployment targets.

This is an operations and evidence slice. It does not introduce a second persistence engine, a proprietary backup format, or a new memory semantic model.

## Non-Goals

- No HTTP backup endpoint;
- no token export or raw-secret backup;
- no automatic migration from `serve`;
- no database deletion as an uninstall action;
- no claim that a successful dump restore proves distributed HA or point-in-time recovery;
- no benchmark score substitution for restore correctness.

## Authority And Backup Boundary

PostgreSQL is authoritative for:

- continuity spaces and bindings;
- observations and governed memory lifecycle;
- delivery and conversation-turn history required by deletion policy;
- durable bridge operations, events, and effects;
- token digest metadata and revocation state;
- migration history.

The following are disposable or reproducible:

- `memory_search_documents` search projection;
- embedding/vector indexes added by later retrieval slices;
- generated HTTP context packets;
- optional mem0, MemOS, or Supermemory adapter state.

Backups use PostgreSQL's native `pg_dump`/`pg_restore` tools. PostgreSQL custom-format dumps are not encrypted by the format itself, so operators must encrypt and access-control the artifact during storage and transfer. Raw token secrets are never in PostgreSQL and therefore cannot be recovered from a dump; a restore must reissue or revoke tokens according to the operator's incident policy.

## Recovery Contract

Given a source database at schema 9 and a target empty database:

1. migrate the source to the current schema with admin credentials;
2. seed governed continuity, lifecycle, bridge, and token metadata;
3. dump the source with PostgreSQL custom format;
4. restore the dump into the target;
5. recreate the non-owner runtime role and run `database grant-runtime`;
6. run `serve` against the target runtime DSN;
7. authenticate using a token whose digest was restored and whose status is active;
8. compare authoritative fingerprints and deterministic lifecycle counts;
9. drop and rebuild `memory_search_documents` from governed active memories;
10. verify recall, deletion exclusion, RLS filter omission, and cross-tenant FK rejection again.

The target is accepted only when authoritative fingerprints match and all hard gates pass. A projection may be empty before rebuild but must be equivalent after rebuild.

## Failure Contract

- `serve` never connects with admin credentials when the runtime role is unavailable;
- an unavailable auth database returns a bounded service error and no anonymous fallback;
- a missing tenant context returns zero rows or a closed error;
- a connection returned to the pool has no prior tenant GUC;
- after the database becomes available, a new authenticated request can complete without process restart;
- an in-progress turn may become failed, but the system never claims a successful persistence receipt for a write it did not commit.

## Linux Contract

The release build must produce:

```text
vermory-linux-amd64
vermory-linux-arm64
```

Both builds use `CGO_ENABLED=0`, report Linux ELF metadata, and pass a Linux process startup probe. The probe must show the binary can print CLI help and validate the `serve` configuration boundary. Database-backed runtime execution is additionally covered on an available Linux runtime; when emulation is used, the evidence must state that explicitly.

## Hard Gates

- migration replay reaches schema 9 without applying a duplicate migration;
- custom dump restores without SQL errors;
- source and target authoritative fingerprints are identical;
- restored token authentication succeeds and revoked token authentication fails;
- runtime role is non-owner, non-superuser, and non-`BYPASSRLS` on target;
- all 12 RLS policies and tenant-aware FK constraints exist on target;
- projection rebuild restores active recall and does not restore deleted/superseded content;
- direct filter-omission query never returns another tenant;
- runtime database outage does not produce a false successful receipt;
- both Linux architecture artifacts pass ELF/startup checks.

## Evidence Separation

Deterministic evidence includes migration versions, dump/restore exit codes, row fingerprints, role attributes, RLS catalog inventory, token lifecycle results, projection counts, and HTTP status/receipt assertions.

Model output is optional behavioral evidence only. It cannot establish restore equivalence, deletion, isolation, or authorization.
