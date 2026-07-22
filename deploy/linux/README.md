# Vermory on Linux with systemd

This directory contains the production-shaped Linux service lifecycle for a
Vermory release archive. It installs immutable, versioned release directories,
runs `vermory serve` under a dedicated system identity, rolls back an unhealthy
upgrade, and provides native PostgreSQL backup and empty-target restore tools.

The scripts require an operator-controlled root invocation but never invoke
`sudo` themselves. They target a systemd system service, not a per-user unit.

## Requirements

- Linux with systemd, Bash, GNU coreutils, GNU tar, curl, and shadow-utils;
- PostgreSQL client tools matching the server major version;
- a reachable PostgreSQL database with the `vector` extension available;
- one admin database identity for migrations and grants;
- one existing `LOGIN NOSUPERUSER NOBYPASSRLS` role for the service;
- a root-owned `0600` service environment file.

The service listens on `127.0.0.1:8788` by default. Put a separately governed
TLS reverse proxy in front of it for remote access, or configure a TLS pair
before selecting a non-loopback `VERMORY_LISTEN` value.

## Prepare PostgreSQL

Keep the admin connection outside the service environment. Apply migrations
and grant the serving boundary explicitly:

```bash
./vermory database migrate --database-url "service=vermory_admin"
./vermory database grant-runtime \
  --database-url "service=vermory_admin" \
  --role vermory_runtime
./vermory database rebuild-projections \
  --database-url "service=vermory_admin"
```

`service=vermory_admin` is a libpq service selected from an operator-controlled
`PGSERVICEFILE`; its password can be supplied through a private `PGPASSFILE`.
The running service must receive only its restricted runtime connection.

## Prepare the service environment

Create `/etc/vermory/vermory.env` as root with mode `0600`:

```text
VERMORY_DATABASE_URL=postgresql://vermory_runtime:REDACTED@127.0.0.1:5432/vermory?sslmode=require
VERMORY_LISTEN=127.0.0.1:8788
VERMORY_PROVIDER=mock
```

For a real provider, add its server-owned model configuration and key variable
to this protected file. Do not put an admin database identity here. The unit
loads this file without copying its values into `ExecStart` or command-line
arguments.

## DEB and RPM packages

Vermory release snapshots and tagged releases include native DEB and RPM files
for AMD64 and ARM64. Verify the selected package through the signed complete
release manifest before installation, then use the host package manager:

```bash
sudo -- apt install ./vermory_0.1.0_linux_amd64.deb
# or
sudo -- dnf install ./vermory_0.1.0_linux_amd64.rpm
```

The package installs `/usr/bin/vermory`, a hardened
`/usr/lib/systemd/system/vermory.service`, the `vermory` non-login service
identity, Linux documentation, and a non-secret environment example at
`/usr/share/vermory/vermory.env.example`. It deliberately does not create
`/etc/vermory/vermory.env`, run migrations, enable the unit, start the service,
or carry database credentials.

Complete the database and environment steps above, then activate the service
explicitly:

```bash
sudo -- install -d -o root -g root -m 0755 /etc/vermory
sudo -- install -o root -g root -m 0600 \
  /usr/share/vermory/vermory.env.example \
  /etc/vermory/vermory.env
sudo -- systemctl enable --now vermory.service
```

Replace every deployment-specific value before activation. Package upgrades do
not migrate PostgreSQL or restart the service; perform the release-specific
database compatibility check first and restart explicitly. Package removal
stops a currently managed service and removes package-owned files while
preserving `/etc/vermory` and the stable service identity.

The repository currently produces signed package artifacts, not an APT or DNF
repository. Repository metadata, repository signing, and retention remain a
separate delivery boundary.

The exact DEB/RPM artifacts accepted on native AMD64/ARM64 runners and then
included byte-for-byte in a signed pull-request snapshot are recorded in the
[I06 native package evidence](../../docs/evidence/2026-07-22-linux-native-packages.md).
That qualification does not extend to a package repository or tagged release.

## Initial install and upgrade

Verify the release archive against the signed Vermory release manifest first.
Then pass the same archive digest to the installer:

```bash
sha256sum vermory_0.1.0_linux_amd64.tar.gz
sudo -- ./deploy/linux/install-system-service.sh \
  ./vermory_0.1.0_linux_amd64.tar.gz \
  0.1.0 \
  EXPECTED_SHA256
```

The installer validates the archive before extraction, creates the dedicated
service user and group, installs into `/opt/vermory/releases/0.1.0`, renders the
hardened unit, atomically activates `current`, and requires HTTP `401` from an
unauthenticated protected endpoint. That status proves the service is reachable
and its authentication boundary is active without exposing a token.

Run the same command with a new archive and release ID to upgrade. If the new
release does not pass the bounded probe, the installer restores the former
`current` and `previous` pointers, restarts the former release, verifies it, and
returns nonzero. The failed immutable release remains available for diagnosis.

Binary pointer rollback does not reverse PostgreSQL migrations. A release with
a non-backward-compatible migration requires a release-specific database plan.

## Explicit rollback

The rollback command swaps `current` and `previous`, so invoking it a second
time returns to the release from which the first rollback started:

```bash
sudo -- ./deploy/linux/rollback-system-service.sh
```

If the selected previous release is unhealthy, the script restores and probes
the starting release before returning failure.

## Backup

Create a PostgreSQL custom-format backup through an admin libpq service:

```bash
PGSERVICEFILE=/secure/path/pg_service.conf \
PGPASSFILE=/secure/path/pgpass \
./deploy/linux/backup-postgresql.sh \
  vermory_admin \
  /encrypted/backups/vermory \
  2026-07-22T180000Z
```

The command writes a private `.dump`, a SHA-256 sidecar, and normalized JSON
metadata without putting a password or connection URL in process arguments.
It refuses to overwrite an existing backup ID.

PostgreSQL custom format provides structured restore support, not encryption.
Store the files on encrypted, access-controlled storage and apply an external
retention policy.

## Restore

Provision a separate empty target database and an existing restricted runtime
role. The restore script rejects a checksum mismatch, a non-empty target, a
missing role, a superuser role, or a role with `BYPASSRLS`:

```bash
PGSERVICEFILE=/secure/path/pg_service.conf \
PGPASSFILE=/secure/path/pgpass \
./deploy/linux/restore-postgresql.sh \
  /encrypted/backups/vermory/2026-07-22T180000Z.dump \
  vermory_restore_admin \
  vermory_restore_runtime \
  ./vermory
```

After `pg_restore`, the script replays current migrations, grants the runtime
boundary, and rebuilds disposable lexical projections from active PostgreSQL
authority. Update the protected service environment to the restored runtime
connection and restart the service only after these steps succeed.

API token digests are restored, but raw token secrets are not recoverable from
PostgreSQL. Incident policy may require revocation and reissue after a restore.

## Overrides

The install and rollback scripts support these deployment-specific overrides:

```text
VERMORY_INSTALL_ROOT
VERMORY_ENVIRONMENT_FILE
VERMORY_SYSTEMD_UNIT
VERMORY_SERVICE_NAME
VERMORY_SERVICE_USER
VERMORY_SERVICE_GROUP
VERMORY_UNIT_TEMPLATE
VERMORY_HEALTH_URL
VERMORY_HEALTH_ATTEMPTS
VERMORY_HEALTH_INTERVAL
```

Defaults are `/opt/vermory`, `/etc/vermory/vermory.env`,
`/etc/systemd/system/vermory.service`, and `vermory.service`.
