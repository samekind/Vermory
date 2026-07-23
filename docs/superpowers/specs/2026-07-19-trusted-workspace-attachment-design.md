# Trusted Workspace Attachment Design

Status: frozen for implementation

Date: 2026-07-19

## 1. Purpose

This slice makes workspace attachment independent of model output. Entering a
recognized checkout should reconnect to its governed workspace continuity
across coding clients. Entering an unknown or ambiguous checkout should abstain
instead of attaching by basename, repository title, remote URL, or semantic
similarity.

The frozen reality case is
`reality/cases/W05-trusted-workspace-attachment`.

## 2. Existing Failure

The MCP `prepare_context` tool currently asks the model to send `repo_root`.
The remote server then treats that untrusted string as the lookup anchor. A
separate pure resolver can also derive `workspace:<basename>` when no governed
candidate exists. These behaviors cannot distinguish same-name repositories,
same path text on different hosts, worktrees, moved checkouts, mirrors, or
clones, and they put attachment selection on the wrong side of the model trust
boundary.

## 3. Identity Contract

A V1 trusted workspace attachment contains:

- a schema version;
- an opaque filesystem namespace ID provisioned outside the model;
- the canonical local Git checkout root found by a trusted client-side probe;
- the actual cwd, which may be nested under that root;
- a Git common-directory fingerprint for diagnostics and explicit adopt
  review, never for automatic merging;
- an attachment fingerprint over the normalized identity fields.

The authoritative lookup key is the pair:

```text
filesystem namespace ID + canonical repository root
```

Path text alone is not globally unique. The namespace ID represents one local
filesystem identity domain and must be shared by trusted adapters on that
machine so Codex, Grok, Cursor, and other clients see the same binding. It is
not a tenant, user identity, secret, model parameter, or repository identity.

The probe uses local Git metadata only to find the root. Git common-directory
or remote identity may help an operator review a proposed adopt, but neither
automatically grants continuity.

## 4. Local And Remote Runtime Boundary

For a local MCP process:

```text
trusted client adapter
-> inspect local cwd with git
-> normalize and fingerprint attachment
-> start Vermory MCP with that attachment
-> exact governed lookup or abstention
```

For stdio over SSH:

```text
trusted workstation adapter probes local checkout
-> passes bounded attachment at remote MCP process startup
-> Mac mini validates attachment structure and fingerprint
-> PostgreSQL resolves exact governed binding or abstains
```

The remote host never claims to inspect the workstation filesystem. The MCP
tool call itself carries task text and delivery options only. It does not expose
`repo_root`, namespace, tenant, continuity ID, confirmation, adopt, rebind, or
other governance controls to the model.

Startup configuration is server-owned. Starting a different MCP process with a
different attachment is an operator or trusted-adapter action, not an in-band
model action.

## 5. Resolution Rules

The server normalizes and verifies the startup attachment, then queries the
authoritative PostgreSQL binding for the exact tenant, namespace, and root.

- One exact confirmed binding returns `resolved` and its continuity ID.
- No exact confirmed binding returns `needs_confirmation`.
- Invalid, incomplete, or internally inconsistent attachment data fails MCP
  startup.
- Conflicting confirmation is rejected by a database uniqueness constraint.
- An explicit continuity ID never bypasses the exact binding lookup.
- Legacy path-only bindings remain available to legacy operator workflows but
  are not an automatic fallback for namespaced remote attachments.

The pure resolver must follow the same conservative rule: a supplied root with
no governed candidate is unresolved, never a basename-derived identity.

## 6. Worktree, Move, Mirror, And Clone Semantics

A Git worktree has a different root from the primary checkout. It abstains
until `AdoptWorkspaceAnchor` explicitly adds its namespaced root as an alias.
The shared Git common-directory fingerprint is review evidence only.

A moved checkout abstains at its new root until `RebindWorkspace` retires the
old namespaced binding and confirms the new one. Rebind retains the continuity
ID and audit record.

A mirror, fork, clone, restored copy, or repository with the same remote URL
abstains unless an operator explicitly adopts or rebinds it. Vermory does not
infer which relationship the user intends.

The same path text in another filesystem namespace is a different anchor and
cannot reuse the first namespace's binding.

## 7. Governance And Storage

PostgreSQL remains the only semantic authority. The existing
`continuity_bindings` relation gains a filesystem namespace field. Existing
rows migrate to the empty legacy namespace; new trusted attachments require a
non-empty namespace. The confirmed-anchor uniqueness constraint covers tenant,
namespace, and normalized root.

`AdoptWorkspaceAnchor` and `RebindWorkspace` accept optional existing and new
namespace IDs. Empty IDs preserve current path-only operator behavior. New
namespaced aliases use the same durable bridge and reversal machinery rather
than implicit identity migration.

Models may prepare context and write proposed observations after resolution.
They cannot confirm a workspace, create an alias, rebind a path, select a
tenant, or activate their own memory.

## 8. Acceptance Gates

The slice passes only when all of these hold:

1. A nested cwd is probed to its canonical Git root without model input.
2. The attachment encoding is deterministic, bounded, normalized, and rejects
   fingerprint mismatch or path escape.
3. Codex, Grok, Cursor, or another coding client using the same attachment
   resolves the same continuity.
4. Same basename in another path and identical path text in another namespace
   do not resolve to the governed continuity.
5. An unknown root, worktree, moved path, mirror, or clone returns
   `needs_confirmation` until an explicit governance action.
6. Namespaced adopt and rebind preserve continuity and remain reversible.
7. `prepare_context` exposes no workspace path, namespace, tenant, binding,
   adopt, or rebind parameter.
8. The remote stdio-over-SSH path uses a workstation-generated attachment and
   Mac mini PostgreSQL without server-side filesystem inference.
9. A real coding client receives governed context, creates a deterministic
   artifact, and writes one proposed observation with idempotent replay.
10. Cross-tenant and cross-continuity leakage remain zero, and all failed
    attempts, checksums, and privacy scans are retained.

## 9. Non-Claims

This slice does not automatically identify arbitrary repository copies, merge
worktrees, rank models, qualify every client, replace explicit bridge review,
or complete Vermory. It proves one conservative workspace-attachment contract
that later adapters can reuse.
