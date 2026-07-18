# Cursor Agent Real-Client Qualification Attempt

Date: 2026-07-19

Status: blocked by external client account

## Scope

W25 executes the frozen
[`W04-canonical-repository-cross-client`](../../reality/cases/W04-canonical-repository-cross-client/manifest.json)
contract against the real `cursor-agent` CLI. Cursor is treated as a distinct
external coding client, not as implementation delegation and not as a substitute
for Codex or Grok evidence.

The intended task is:

```text
exact confirmed workspace
-> Cursor loads Vermory MCP over stdio
-> prepare_context delivers current accepted facts
-> Cursor creates and verifies continuity-report.md
-> commit_observation is replayed idempotently
-> PostgreSQL retains one proposed memory and no active agent memory
```

The real generation step did not reach MCP. This document records a failed
qualification, not a client pass.

## Frozen Runtime

| Field | Value |
|---|---|
| Source revision | `b2173fdd128eb02b7fda96c826dce04b0dca017a` |
| Disposable client workspace revision | `2fe75531c7d8e85b6d7fadf39e2b42dd70ebaac7` |
| Cursor Agent | `2026.07.13-7fe37d2` |
| Requested model | `gpt-5.3-codex` |
| Vermory binary SHA-256 | `c52a72195494f0f296080a975ecbc543cabf6bf067f90f7eaabfcb98d59acad9` |
| Database | dedicated PostgreSQL 18 on the Mac mini |
| MCP transport | project-local stdio over the existing SSH management path |
| Stable evidence | `$HOME/.vermory/evidence/W25-cursor-agent/20260719-b2173fd-v2` |

No `sudo` was used. PostgreSQL remained local to the Mac mini; no database
port or credential was exposed to the workstation.

## Seed State

Two exact workspace continuities were confirmed before the client run.

The canonical continuity contained three current facts:

- `https://github.com/samekind/Vermory` is canonical;
- the organization repository is independent rather than a GitHub fork;
- the current marker is `samekind-w25-current`.

The former `jstar0/Vermory` canonical fact remained present only as one
superseded row. A separate same-name continuity retained the active synthetic
marker `distractor-w25-only`.

The fresh seed ledger reported:

```json
{
  "canonical_current": 3,
  "canonical_superseded": 1,
  "stale_active": 0,
  "distractor_current": 1,
  "pre_client_deliveries": 0,
  "pre_client_observations": 0
}
```

## MCP Discovery

After explicit project-server approval and correction of the remote URI quoting,
the real Cursor CLI reported:

```text
vermory-w25: ready
```

It listed exactly:

```text
commit_observation (content, delivery_id, operation_id, source_ref)
prepare_context (cwd, max_items, operation_id, repo_root, task)
```

The client surface exposes no tenant, binding, authority, acceptance,
correction, deletion, supersession, target-memory, or rebind argument.

## Real Client Result

The final attempt used `--print`, `--output-format stream-json`,
`--trust`, `--auto-review`, `--sandbox enabled`, and
`--approve-mcps`. It did not use `--force` or `--yolo`.

Cursor initialized the session and recorded the requested model, then exited
`1` with:

```text
ActionRequiredError: You have an unpaid invoice
```

The error occurred before `prepare_context`. The final PostgreSQL ledger was:

```json
{
  "deliveries": 0,
  "agent_results": 0,
  "proposed_memories": 0,
  "active_agent_memories": 0,
  "canonical_delivered": 0,
  "marker_delivered": 0,
  "stale_delivered": 0,
  "distractor_delivered": 0,
  "source_ref_matches": 0
}
```

No `continuity-report.md` was written. This is an account-level client
failure, not a Vermory delivery pass and not evidence that Cursor consumed
governed context.

## Preserved Attempts

Seven attempts remain in the retained failure ledger:

1. a minimal probe observed the account error, followed by an outer zsh
   reserved-variable harness failure;
2. the corrected minimal probe exited `1` with the account error;
3. the first full runner found the MCP server but lacked explicit approval;
4. the approved server initially failed because the remote PostgreSQL URI was
   not quoted for zsh;
5. the corrected MCP server became ready, but generation remained account
   blocked;
6. the first final runner added exact artifact, replay, and PostgreSQL hard
   gates; MCP remained ready and generation remained account blocked with zero
   authoritative effects;
7. the current runner derived fresh operation IDs, passed the zero-effect
   preflight, and again remained account blocked with zero authoritative
   effects.

The runner now refuses to overwrite run IDs, removes project-local MCP
configuration, excludes raw login identity, requires exact artifact content,
requires a replay receipt, and requires a complete PostgreSQL ledger before
setting `pass=true`. Fixture tests prove incomplete ledger evidence cannot
pass.

## Integrity

The normalized Mac mini evidence root contains 85 files covered by
`checksums.sha256`. The checksum manifest SHA-256 is:

```text
bb18d5d816dba0996a28af0bce6969074f77bcf75c60de1dea95542d9df09d29
```

Independent verification passed `85 / 85` entries. The privacy report found
zero credential-token shapes, private-key markers, email addresses, personal
home paths, raw login-status files, and environment dumps.

## Qualification Boundary

W25 is not qualified. A later fresh run must satisfy every gate in
[`Cursor Agent Real-Client Qualification Design`](../superpowers/specs/2026-07-19-cursor-agent-real-client-design.md),
including both real MCP calls, the exact artifact, idempotent replay, one
proposed memory, zero active agent memory, stale and distractor exclusion, and
privacy/checksum verification.

A Codex pass, Grok pass, fake Cursor test, MCP discovery result, or account/model
listing cannot replace that run.
