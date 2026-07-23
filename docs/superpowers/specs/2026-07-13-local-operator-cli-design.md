# Local Operator CLI Design

**Status:** approved for planning

**Date:** 2026-07-13

## Purpose

The workspace MCP slice deliberately gives an AI client only two normal-flow
actions: retrieve governed workspace context and write back an
`agent_result` proposal. It cannot create a workspace binding, promote its
own output, replace a current fact, or forget a fact.

This design adds the missing trusted local path for those actions. It is an
operator CLI for the person running a local Vermory deployment, not a new
model-facing protocol and not an administration UI.

The intended local flow is:

```text
operator confirms workspace
-> AI client retrieves active workspace context over MCP
-> AI client writes a proposed result
-> operator inspects the scoped state
-> operator records a source fact, corrects one named fact, or forgets one named fact
-> the next client request receives only the eligible active state
```

## Scope

This slice adds commands under the existing `vermory` binary. Every command
opens the configured PostgreSQL database, runs migrations, and requires a
server-owned `--tenant-id`. It never accepts tenant identity from an MCP
caller or another model-facing transport.

### Commands

| Command | Required inputs | Effect |
|---|---|---|
| `workspace confirm` | `--database-url`, `--tenant-id`, `--repo-root` | Explicitly creates or confirms one workspace binding. Repeating the same root returns the existing continuity. |
| `workspace inspect` | `--database-url`, `--tenant-id`, `--repo-root` | Returns the exact workspace resolution and its continuity ID when confirmed. An unknown root returns `needs_confirmation`; it does not create state. |
| `memory inspect` | `--database-url`, `--tenant-id`, `--repo-root` | Lists scoped governed memories with ID, lifecycle, content, and revision relation so an operator can choose an explicit target. |
| `memory add-source` | `--database-url`, `--tenant-id`, `--repo-root`, `--operation-id`, `--content`, `--source-ref` | Records a trusted source observation and creates one active fact. It does not infer or replace another fact. |
| `memory correct` | `--database-url`, `--tenant-id`, `--repo-root`, `--operation-id`, `--memory-id`, `--content` | Records a trusted user correction and requires the named active fact to be superseded atomically. |
| `memory forget` | `--database-url`, `--tenant-id`, `--repo-root`, `--operation-id`, `--memory-id` | Records an explicit forget request and redacts the named fact and its origin content atomically. |

`--operation-id` is required for mutating memory commands. It is the durable
idempotency key: a retry with the same ID returns the original receipt, and a
reused ID against another workspace fails rather than duplicating authority.

Commands print concise, parseable receipts to stdout. Errors and diagnostics
remain on stderr. The receipts include continuity, observation, memory, and
lifecycle identifiers that are necessary for the next explicit operator
action, but context delivery remains semantic-only for MCP consumers.

## Authority And Safety Rules

- A normal MCP client retains exactly `prepare_context` and
  `commit_observation`; no trusted governance tool is added to MCP.
- `workspace confirm` may confirm only the supplied normalized root. It never
  guesses that a rename, mirror, worktree, or different path belongs to an
  existing continuity. Explicit `rebind` remains a later bridge action.
- Mutating commands resolve the workspace first. A missing or unconfirmed
  root is an error for mutation and creates no observation or memory.
- `memory add-source` produces active authority but never silently
  supersedes another fact. `memory correct` must name an active fact to
  supersede. `memory forget` must name the fact to redact.
- `memory forget` has no free-text reason. It writes a bounded operator-action
  observation rather than storing a new operator sentence that could repeat
  the deleted fact in audit history.
- All writes use the existing single PostgreSQL transaction that stores the
  observation, performs the lifecycle transition or redaction, and updates
  the lexical projection. The CLI does not edit tables directly.
- Inspect operations are scoped to one resolved workspace and tenant. They do
  not search or expose other continuities.
- PostgreSQL remains authoritative. Rebuilding the lexical projection after a
  correction or forget must preserve only the current active facts and must
  not revive redacted content.

## Design Choices Considered

### Recommended: local trusted CLI

The command surface is explicit, works before HTTP or UI exists, and keeps
ordinary AI clients unprivileged. It directly enables real local use of the
workspace slice and has a small, deterministic test surface.

### Deferred: HTTP administration API

An HTTP endpoint would need authentication, authorization, transport
security, session ownership, and deployment policy before it could safely
grant governance authority. Those concerns are legitimate but do not belong
in this local vertical slice.

### Rejected: governance MCP tools

Exposing confirmation, correction, or deletion as normal MCP tools would let
a model grant authority to its own output or alter its own scope. That
contradicts Vermory's authority boundary.

## Implementation Shape

The CLI will use a small trusted runtime facade rather than exposing raw SQL
from `cmd/vermory`. The facade owns the configured tenant and composes the
existing store operations:

```text
CLI command
-> trusted runtime facade
-> resolve or confirm workspace
-> PostgreSQL authority transaction
-> concise receipt
```

The facade is limited to workspace confirmation, scoped inspection, and the
three explicit memory mutations. It does not attempt to define a generic
policy engine, role system, bridge model, or cross-line abstraction.

## Acceptance

The slice is accepted only when fresh tests establish all of the following:

1. An unknown root inspected through the CLI remains `needs_confirmation` and
   creates no continuity.
2. Explicit confirmation is idempotent and lets MCP context preparation use
   the same workspace continuity.
3. A trusted source fact becomes active and reaches the next scoped context
   delivery, while an agent write-back remains proposed.
4. A correction names and supersedes exactly one active fact; the next
   context excludes the old fact.
5. Forgetting a named fact redacts authoritative content, removes its
   projection, and survives an explicit projection rebuild under exact and
   paraphrased probes.
6. Commands cannot operate on an unresolved root, another tenant, or a
   memory outside the resolved workspace.
7. MCP still discovers exactly two normal-flow tools after the CLI is added.
8. A real local replay uses the CLI to govern a non-sensitive workspace
   fixture, then uses a registered MCP client to retrieve the resulting
   context and write back a proposal. Its artifact and redacted ledger are
   preserved separately from public fixtures.

## Non-Goals

- HTTP, UI, authentication, roles, remote multi-user administration, and
  browser management.
- Conversation-backed continuity, Global Defaults, or their governance
  controls.
- `promote`, `link`, `export`, `adopt`, `rebind`, `split`, or `merge` bridge
  operations.
- Automatic promotion of model output or language-based supersession.
- Embedding, vector retrieval, mem0, MemOS, Supermemory, or provider work.
