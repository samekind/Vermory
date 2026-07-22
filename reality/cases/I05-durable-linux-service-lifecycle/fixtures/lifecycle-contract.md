# Durable Linux service lifecycle qualification fixture

This case contains only synthetic Linux deployment and PostgreSQL recovery
state. It does not contain a user database, provider credential, API token,
private host route, private repository, or personal filesystem path.

## Installed service

Vermory is installed as a systemd system service under a dedicated non-login
service identity. Release payloads are root-owned and versioned. The `current`
and `previous` pointers are changed only by a root-required installer or
explicit rollback action. The service defaults to a loopback listener.

The unit does not contain a PostgreSQL connection string or provider secret.
It loads a root-owned private environment file, and `vermory serve` reads its
restricted runtime database configuration from that environment. The admin
database identity is never available to the service.

## Release lifecycle

Installation requires a release archive, normalized release ID, and expected
SHA-256. Archive traversal, digest mismatch, a missing executable, and a
conflicting existing release ID are rejected before activation.

An upgrade atomically moves the active pointer after installing immutable
bytes. The new service must pass a bounded authenticated-boundary probe. If it
fails, the installer restores and verifies the former release, returns a
failure status, and retains the failed release for diagnosis. An explicit
rollback similarly restores its starting release if the requested previous
release cannot become healthy.

Binary rollback does not undo forward PostgreSQL migrations. Any release that
breaks backward schema compatibility requires a separate migration and rollback
decision.

## Backup and restore

Backups use PostgreSQL custom format with `--no-owner` and `--no-acl`. Database
access is selected through a libpq service name. Passwords may be supplied by a
private `PGPASSFILE`; they do not appear in process arguments, backup metadata,
logs, or evidence.

Each backup has private file permissions, a SHA-256 sidecar, and normalized
metadata. PostgreSQL custom format is not encrypted, so the operator must place
it on encrypted, access-controlled storage.

Restore accepts only a checksum-matched backup and an empty target database.
Cluster roles are not restored from the dump. The target runtime role is
separately provisioned, migrations are replayed, and disposable projections are
rebuilt from active PostgreSQL authority before service traffic resumes.

## Required proof

The protected real-Linux run must demonstrate initial service install, current
authenticated state, successful upgrade, failed-upgrade automatic rollback,
explicit rollback in both directions, private native backup, empty-target
restore, runtime-role re-provisioning, projection rebuild, and authenticated
state after service restart on the restored target.

The run may use an ephemeral protected Ubuntu host to prove actual systemd and
Linux process behavior. It does not claim long-duration uptime, a cloud SLA,
native Linux ARM64 service execution, DEB/RPM packaging, or automatic database
migration rollback.
