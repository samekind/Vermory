# Release And Database Compatibility Design

Date: 2026-07-22

Status: approved execution boundary under the active Vermory goal

## Goal

Make release/database compatibility a deterministic deployment boundary. A
Vermory binary must reject an unsupported PostgreSQL schema before it opens a
network listener or performs an application write, and operators must be able
to inspect the same decision as stable JSON before changing a deployment.

This slice does not add automatic migration to `serve`, package installation,
or service startup. PostgreSQL remains the only semantic authority.

## Failure Being Addressed

`serve` currently avoids automatic migration, but it discovers an old or future
schema only when a later table, function, or role check fails. That failure is
too late and too ambiguous for package upgrades, binary rollback, or unattended
service supervision. A future binary must not be allowed to start against an
old schema, and an old binary must not be allowed to start against a schema it
does not understand.

## Compatibility Contract

The release embeds one inclusive schema support interval:

```text
minimum_supported_schema <= current_schema <= maximum_supported_schema
```

The deterministic decisions are:

| Condition | Decision | Service action |
|---|---|---|
| current schema is inside the interval | `compatible` | startup may continue |
| current schema is below the interval | `migration_required` | refuse startup |
| current schema is above the interval | `binary_too_old` | refuse startup |
| version cannot be read through the restricted contract | `preflight_failed` | refuse startup |

The first qualified release supports schema 24 exactly. Migration 24 adds only
the restricted schema-version inspection boundary; it does not change memory
semantics. Later releases may intentionally widen the interval when tests prove
backward compatibility. The interval must never be inferred from package or
semantic version strings.

## Database Permission Boundary

Migration 24 creates `vermory_auth.schema_version()`, a `STABLE`,
`SECURITY DEFINER` function with a fixed `pg_catalog` search path. It returns
only the highest applied Goose version.

- `PUBLIC` receives no execute privilege.
- the application runtime role receives execute privilege only through
  `database grant-runtime`;
- the runtime role receives no `SELECT`, `INSERT`, `UPDATE`, or `DELETE`
  privilege on `public.goose_db_version`;
- the operator/admin path may continue to read the migration table directly;
- neither path exposes a DSN or credential in its report or error.

An installation upgrading from schema 23 performs the operator sequence before
enabling the new service:

```text
backup
-> vermory database compatibility
-> vermory database migrate
-> vermory database grant-runtime
-> vermory database compatibility with the runtime DSN
-> start or restart service
```

The pre-migration inspection uses the admin path because schema 23 predates the
restricted inspection function. The service remains fail closed until migration
and runtime-role provisioning are complete.

## CLI Contract

`vermory database compatibility --database-url <dsn>` performs no migration
and no application write. It emits one JSON document:

```json
{
  "status": "compatible",
  "schema_version": 24,
  "minimum_supported_schema": 24,
  "maximum_supported_schema": 24,
  "binary_revision": "<build revision>",
  "migration_required": false
}
```

The command exits successfully only for `compatible`. For
`migration_required` and `binary_too_old`, it still writes the complete JSON
report and then returns a typed incompatibility error. Automation must use the
JSON status rather than parse free-form SQL errors.

## Serve Contract

`serve` performs the restricted compatibility preflight after opening its
runtime pool but before:

- constructing a model provider;
- building a retriever;
- opening the authentication pool;
- creating an HTTP listener;
- writing application state.

An incompatible or unreadable schema returns a bounded typed error. The error
contains the decision and supported interval but never the database URL,
password, provider secret, SQL statement, or raw PostgreSQL error.

`serve` still validates that the database identity is a non-owner,
non-superuser, non-`BYPASSRLS` runtime role after compatibility succeeds.

## Migration And Package Contract

- `database migrate` remains an explicit admin command.
- DEB/RPM installation, upgrade, and removal never invoke it.
- systemd does not invoke it through `ExecStartPre` or another hidden hook.
- migration replay remains idempotent.
- a package upgrade may leave the service stopped or failing closed until the
  operator completes the documented database sequence.

## Rollback And Recovery Contract

Vermory does not promise general automatic down migrations.

A binary-only rollback is allowed only when the database schema is inside the
older binary's declared support interval. The older binary performs the same
preflight and refuses to listen when the schema is too new.

When a migration moves the database outside the older binary's interval, the
rollback path is restoration of a pre-migration PostgreSQL backup or PITR to a
verified restore point, followed by runtime-role re-provisioning and
compatibility inspection. Package pointer rollback alone is not database
rollback.

## Evidence Plan

Deterministic evidence uses dedicated PostgreSQL databases and restricted login
roles. It covers:

1. schema 24 returns `compatible`;
2. schema 23 returns `migration_required` through the admin inspection path;
3. a synthetic applied schema 25 returns `binary_too_old`;
4. the schema function is executable by the provisioned runtime role;
5. that role cannot read or mutate `goose_db_version`;
6. `serve` against schema 24 reaches the listening boundary only with the
   restricted role;
7. old, future, or unreadable schema state produces no listener and no
   application write;
8. output and errors pass credential scans;
9. migration replay remains stable at schema 24;
10. package scripts remain migration-free.

The old and future schema databases are synthetic operational fixtures. They
prove compatibility decisions and process boundaries, not application-data
rollback or long-term uptime.

## Non-Claims

This slice does not qualify:

- a universal down-migration mechanism;
- logical restoration across every future schema change;
- zero-downtime database upgrades;
- APT or DNF repository publication;
- tagged release publication;
- long-term uptime or SLA.
