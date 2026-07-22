# macOS Authenticated Service Lifecycle Design

Status: frozen for execution

Date: 2026-07-23

## Purpose

Qualify the durable user-level lifecycle of the authenticated Vermory service
on a real Mac mini. The service must remain loopback-only and unprivileged,
preserve PostgreSQL-authoritative state, reject incompatible binaries before
replacement, recover automatically from a failed candidate startup, and allow
an operator to roll back explicitly to the immediately preceding compatible
installation.

Runtime case: `I11-macos-authenticated-service-lifecycle`.

## Installation Contract

The existing authenticated installer remains the only installation entry
point. It stages the candidate binary, runner, and mode-`0600` environment file
inside the user's application-support directory. Before replacing any live
file, the candidate must:

- report a valid Vermory version;
- pass the read-only `database compatibility` check with the runtime DSN;
- retain a loopback listen address;
- leave migration and PostgreSQL role changes to the operator path.

If a complete current installation exists, the installer copies it into one
protected rollback slot before activation. A partial current installation is
an error and must not be silently treated as a first install.

## Activation And Recovery Contract

The LaunchAgent always points to stable installed paths. Installation replaces
files atomically, reloads the user LaunchAgent, and accepts the candidate only
after both the root endpoint and unauthenticated session boundary are healthy.

If candidate activation or health verification fails and a rollback slot
exists, the installer restores the previous binary, runner, and environment,
reloads the LaunchAgent, verifies the restored service, and exits nonzero. The
failed candidate is not reported as installed.

## Explicit Rollback Contract

The rollback command performs the previous binary's read-only database
compatibility check before changing live files. An incompatible rollback is
rejected without stopping or replacing the current service. A compatible
rollback atomically restores all three installed files, reloads the same
LaunchAgent, verifies health, and consumes the rollback slot.

Rollback never performs a database down migration. PostgreSQL remains the
semantic authority and existing tenant, continuity, memory, token, audit, and
projection state must remain intact.

## Storage And Security

- No local or remote `sudo`.
- No LaunchDaemon or system-wide install.
- Secrets remain only in a mode-`0600` environment file and never enter the
  plist, command output, normalized evidence, or repository.
- The workstation repository, build cache, temporary files, downloads, and
  evidence remain under `/Volumes/JSData`.
- Mac mini runtime files remain in an isolated user-owned I11 root.

## Real-Host Acceptance

The accepted Mac mini trajectory is:

```text
fresh isolated database and user LaunchAgent
-> install accepted base binary
-> create governed state and authenticated access
-> reject an incompatible candidate with no live-file change
-> install compatible candidate
-> verify restart and state preservation
-> simulate a compatible candidate health failure and automatic restoration
-> reinstall compatible candidate
-> explicit rollback to base
-> verify service, authentication, governed state, and loopback binding
```

OpenClaw and Hermes remain separately qualified clients. I11 verifies that
their Vermory backend can survive this lifecycle; it does not require a model
call and does not inherit or replace client-specific qualification.

## Non-Claims

I11 does not qualify macOS system-wide installation, automatic schema
downgrade, zero-downtime restart, long-duration SLA, public Internet exposure,
model availability, OpenClaw transcript retention, Hermes transcript
retention, or arbitrary historical rollback.
