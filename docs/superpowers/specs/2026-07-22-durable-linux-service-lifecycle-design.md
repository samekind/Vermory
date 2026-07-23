# Durable Linux Service Lifecycle Design

Date: 2026-07-22

Status: frozen execution contract

Case: `I05-durable-linux-service-lifecycle`

## 1. Purpose

This slice qualifies Vermory as an installed Linux service rather than only a
portable ELF process. The operator-facing lifecycle is:

```text
verified Linux release archive
-> versioned installation
-> restricted systemd service
-> authenticated loopback probe
-> verified upgrade
-> failed-upgrade automatic rollback
-> explicit operator rollback
-> native PostgreSQL backup
-> empty-target restore
-> runtime role and projection rebuild
-> restored authenticated probe
```

The case extends, but does not replace, `I02-postgresql-operations-recovery`.
I02 already proves authoritative fingerprint equivalence and PostgreSQL
recovery semantics. I05 proves the missing package, process-manager, release
pointer, secret-file, and service-to-restored-database lifecycle on real Linux.

No LLM is used because model output cannot prove package integrity, process
identity, rollback, backup correctness, RLS, or restored authentication.

## 2. Deployment Profile

The production-shaped default is a system-level systemd service:

- a dedicated `vermory` system user and group;
- root-owned immutable releases under `/opt/vermory/releases/<release-id>`;
- `current` and `previous` symlinks controlled only by the installer;
- a root-readable `0600` environment file under `/etc/vermory`;
- a root-owned unit under `/etc/systemd/system`;
- loopback HTTP unless the operator separately configures a TLS pair;
- no admin database credential in the service process;
- no `sudo` invocation inside Vermory's scripts.

The operator may invoke the root-required installer once through the host's
normal privilege workflow. The scripts themselves require effective UID zero
and never trigger multiple privilege prompts or silently elevate.

## 3. Configuration Boundary

The service reads its restricted runtime configuration from environment
variables. At minimum:

```text
VERMORY_DATABASE_URL
VERMORY_LISTEN
VERMORY_PROVIDER
```

`serve` accepts those variables as defaults while preserving explicit CLI flag
precedence. The unit contains only `ExecStart=/opt/vermory/current/vermory
serve`; it does not interpolate a connection string into the process command
line or journal.

The environment file must be root-owned and inaccessible to group or other
users. Provider keys remain indirect: provider configuration names an API-key
environment variable, while the key value exists only in the protected service
environment. The installer must not print the environment file.

## 4. Install And Upgrade Contract

Every install requires an archive path, a normalized release ID, and the
expected SHA-256. Before activation the installer must:

1. compare the archive digest in constant-format hexadecimal;
2. reject absolute or parent-traversing archive members;
3. require exactly one executable `vermory` payload at archive root;
4. install into a new root-owned versioned release directory;
5. refuse a different payload under an existing release ID;
6. install or verify the hardened systemd unit;
7. atomically update release symlinks;
8. restart the service and require a bounded authenticated-boundary probe.

A normal upgrade moves the old `current` target to `previous`, activates the
new release, and restarts the service. If the new service does not become
healthy, the installer restores the old `current` target, restarts it, verifies
recovery, and exits nonzero. Failed bytes remain in their immutable release
directory for diagnosis; they are not reported as activated.

An explicit rollback swaps `current` and `previous`, restarts, and probes. If
the selected previous release fails, the rollback script restores the starting
release and exits nonzero.

Binary rollback does not reverse PostgreSQL migrations. A release requiring a
non-backward-compatible schema change needs its own migration hypothesis and
cannot rely on this generic pointer rollback.

## 5. systemd Boundary

The unit must run as the dedicated service identity and include at least:

- `NoNewPrivileges=true`;
- `PrivateTmp=true`;
- `ProtectSystem=strict`;
- `ProtectHome=true`;
- kernel, control-group, hostname, and clock protections;
- `RestrictSUIDSGID=true`;
- a bounded address-family set;
- `Restart=on-failure` with restart limits;
- an explicit environment file and absolute executable path.

The acceptance run inspects effective systemd properties, process UID, active
release target, listener address, and journal. A running process alone is not a
pass.

## 6. Backup Contract

Vermory continues to use PostgreSQL native custom-format backups. The backup
script accepts a libpq service name rather than a plaintext DSN. Passwords may
reside in an operator-controlled `PGPASSFILE`; neither passwords nor connection
URLs enter arguments, metadata, logs, or Git.

The script:

- uses `pg_dump --format=custom --no-owner --no-acl`;
- writes through a private temporary file and publishes atomically;
- creates a SHA-256 sidecar and JSON metadata;
- records the backup ID, bytes, digest, schema version, and PostgreSQL versions;
- creates all output files with owner-only permissions;
- refuses to overwrite an existing backup ID.

The PostgreSQL custom format is not encryption. The target backup directory
must be encrypted and access-controlled by the deployment environment.

## 7. Restore Contract

Restore requires a checksum-matched backup and a separately provisioned empty
target database exposed through a libpq service name. It must refuse a target
that already contains Vermory authority tables.

After `pg_restore --exit-on-error --no-owner --no-acl`, the operator workflow:

1. replays current migrations through an admin connection;
2. grants the boundary to an existing restricted `LOGIN`, non-superuser,
   non-`BYPASSRLS` runtime role;
3. rebuilds disposable projections from active PostgreSQL authority;
4. restarts the service with the target runtime service configuration;
5. proves a restored active token authenticates and a governed value is
   visible;
6. confirms service logs and public evidence contain no raw token or database
   credential.

Raw API token secrets are not recoverable from PostgreSQL. The acceptance run
keeps one synthetic token only long enough to prove that its restored digest
authenticates; production incident policy may instead require revocation and
reissue after restore.

## 8. Real Linux Acceptance

The protected repository workflow uses a clean Ubuntu systemd host and a
dedicated PostgreSQL 18 service. It builds two separately versioned Linux AMD64
archives from the exact pull-request head plus one synthetic failing archive.

The run must prove:

1. first install starts under the dedicated UID and returns HTTP `401` for an
   unauthenticated protected endpoint;
2. an operator token can read one synthetic governed default;
3. upgrade changes the release ID and binary digest while preserving the same
   database state;
4. a failing release triggers automatic rollback and the prior service becomes
   healthy again;
5. explicit rollback selects the earlier release and a second explicit rollback
   returns to the later release;
6. backup files are private and checksum-valid;
7. restore into an empty database succeeds, grants the runtime role, and
   rebuilds projections;
8. the service restarts against the restored runtime configuration and the same
   synthetic token/default probe succeeds;
9. the unit, process arguments, journal, normalized report, and committed
   evidence contain no database password or raw token;
10. all files are tied to the exact protected head and failed upgrade evidence
    is retained.

The Ubuntu host is real Linux AMD64 process-manager evidence, but it is an
ephemeral protected runner. The case does not claim measured month-long uptime,
a specific cloud SLA, native Linux ARM64 systemd execution, distribution package
manager integration, automatic unattended upgrades, or universal PostgreSQL
topology support.

## 9. Public Evidence Boundary

Committed evidence contains normalized versions, release IDs, hashes, counts,
systemd properties, HTTP statuses, backup metadata, and hard-gate results. Raw
journal output, environment files, database service files, token values,
passwords, database dumps, and private runner internals remain outside Git.
