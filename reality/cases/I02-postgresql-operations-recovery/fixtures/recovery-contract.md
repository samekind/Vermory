# PostgreSQL operations and recovery fixture

All database names, role names, continuity identifiers, memory identifiers, and facts used by this case are synthetic and dedicated to the case run.

## Authority boundary

PostgreSQL is the authoritative store for continuity bindings, observations, governed memory lifecycle, delivery history, bridges, token digest metadata, revocation state, and migration history. Search documents and later embedding indexes are disposable projections derived from governed active memories.

Native PostgreSQL custom-format dumps are used for backup and restore. The format does not encrypt the artifact, so the operator must encrypt and restrict the dump during storage and transfer. Raw token secrets are never stored in PostgreSQL or copied into evidence.

## Recovery trajectory

1. Apply the complete migration set twice and retain one current schema version without duplicate effects.
2. Seed two isolated tenants with active, superseded, and deleted governed memories, durable bridge records, one active token digest, and one revoked token digest.
3. Capture a native custom-format dump from a dedicated source database.
4. Restore into an empty dedicated target database without restoring object ownership.
5. Recreate and provision a non-owner runtime role separately from the restored database objects.
6. Compare deterministic fingerprints of every authoritative table while excluding disposable projection rows from the authority check.
7. Clear the search projection, rebuild it from governed active memories, and verify that deleted and superseded content remains excluded.
8. Run filter-omission and cross-tenant reference probes again under the target runtime role.
9. Make the database unavailable during an authenticated request and verify that no successful persistence receipt is emitted.
10. Restore database availability and verify that a new authenticated request succeeds without restarting the Vermory process.

## Portability trajectory

Build static Linux amd64 and arm64 binaries from the same revision. Each artifact must have matching ELF architecture metadata, print CLI help inside Linux, and reject unsafe server configuration before attempting to listen. Any emulation used for architecture execution must be recorded rather than presented as native hardware evidence.

## Required exclusions

- no admin connection fallback in the runtime server;
- no proprietary Vermory backup format;
- no projection row treated as authoritative state;
- no deleted or superseded memory restored into active recall;
- no raw token, login password, private connection string, or user database copied into fixtures or evidence;
- no successful write receipt for an uncommitted operation;
- no claim of high availability, point-in-time recovery, or native architecture execution from this single restore case.
