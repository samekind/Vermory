# Long-Running Client Operation Design

Date: 2026-07-23

Status: frozen for implementation

## Goal

Vermory already qualifies bounded client turns:

```text
prepare governed context
-> one model or agent run
-> complete or fail
```

That contract cannot safely represent a multi-hour coding or tool-using agent
whose process, network connection, client host, or Vermory service may restart.
W36 adds a durable operation protocol without creating a second workflow
engine:

```text
prepare leased operation
-> tool work
-> heartbeat or bounded checkpoint
-> restart or timeout
-> exact replay or fenced reclaim
-> complete, fail, or cancel
-> ordinary governed write-back rules
```

The authoritative object remains the conversation turn. PostgreSQL remains the
only native semantic and operation authority.

## Frozen Case W36

The case models a long-running OpenClaw software-maintenance run. It includes:

- an exact prepare replay after an uncertain response;
- several tool steps with monotonic progress checkpoints;
- a simulated plugin and Vermory process restart;
- an expired lease reclaimed by a new attempt;
- late heartbeat, checkpoint, tool-result, completion, and failure writes from
  the obsolete attempt;
- a current-attempt completion;
- a second operation whose cancel races with completion;
- the same operation and anchor names under another tenant;
- an unrelated continuity in the same tenant;
- an unchanged Hermes bounded turn as a compatibility control.

The result of the coding task is not the qualification target. W36 qualifies
the lifecycle, authority, isolation, and semantic side effects around that
task.

## Protocol Boundary

Two protocols coexist:

- `bounded_v1` is the existing prepare/complete/fail contract. It has no lease
  or checkpoint and remains available for existing Hermes and Web Chat paths.
- `leased_v1` is the new long-running contract. Every mutable request after
  prepare carries the server-issued attempt ID and lease generation.

The protocol is explicit on the stored turn. A bounded client cannot silently
enter leased mode, and a leased turn cannot be completed through the bounded
path.

Attempt identity and generation are fencing inputs, not authentication. The
ordinary API credential still determines tenant and role.

## State Model

Turn terminal states are:

- `completed`;
- `failed`;
- `cancelled`.

Lease expiry is not a terminal state. An expired current attempt may still
finish if no later attempt has reclaimed the row. Reclaim is the authoritative
event that increments the generation and fences the old attempt.

The leased turn stores:

- current server-issued attempt ID;
- monotonically increasing generation starting at one;
- last heartbeat time and lease expiry;
- latest checkpoint sequence, canonical JSON object, fingerprint, and update
  time;
- cancellation code and bounded diagnostic message when cancelled.

Only the latest checkpoint is retained. W36 does not introduce unbounded
checkpoint history.

## Prepare And Reclaim Contract

Preparing a new exact operation creates the user observation, turn, generation
one, attempt ID, and lease in one PostgreSQL transaction.

Preparing an existing operation under the same tenant, continuity, request
fingerprint, and protocol behaves as follows:

- terminal turn: return the terminal receipt as an idempotent replay;
- current lease or expired lease not yet reclaimed: return the same attempt and
  checkpoint as an exact replay;
- explicit reclaim request with an expired lease: lock the row, issue a new
  attempt ID, increment generation, retain the latest checkpoint and original
  delivery, and return the new fenced receipt;
- mismatched continuity, request, or protocol: reject without mutation.

Ordinary prepare replay never steals a live lease. Reclaim is explicit and
requires the previous lease to be expired at the database decision point.

The first prepare records one immutable governed-context delivery. Resume and
reclaim return that same delivery and context rather than recomputing context
against a later memory snapshot.

## Heartbeat Contract

A heartbeat carries operation ID, anchor, attempt ID, and generation. Under a
locked row it:

- verifies tenant, continuity, protocol, status, attempt, and generation;
- renews the lease for the server-defined bounded interval;
- creates no observation, delivery, checkpoint, audit content containing the
  request body, formation request, memory, or projection;
- returns an idempotent operation receipt.

A heartbeat from an obsolete attempt or terminal operation is rejected.

## Checkpoint Contract

A checkpoint additionally carries a positive monotonic sequence and a bounded
JSON object. The canonical encoded object is limited to 32 KiB and screened for
credential-like content before persistence.

For the current attempt:

- a higher sequence replaces the latest checkpoint and renews the lease;
- the same sequence and fingerprint is an idempotent replay;
- the same sequence with different content is drift and is rejected;
- a lower sequence is stale and is rejected.

Checkpoint content is client-owned progress data. It is never:

- an observation or governed memory;
- input to formation;
- indexed lexically or semantically;
- included in a model context packet;
- treated as proof that a tool action succeeded.

## Terminal Mutation Contract

Completion, failure, and cancellation lock the same turn row and validate the
current attempt and generation before inspecting terminal replay.

Completion alone:

- validates the original delivery;
- creates one assistant observation;
- records answer and model fingerprints;
- marks the turn completed;
- enqueues ordinary same-continuity conversation formation.

Failure and cancellation:

- record only bounded diagnostic status;
- create no assistant observation;
- enqueue no formation;
- create no memory or projection effect.

After reclaim, every mutation from the old generation is rejected even if the
new attempt has already terminated. After cancellation, a late completion is
rejected and cannot be interpreted as a replay.

Concurrent completion, failure, and cancellation serialize through the row
lock. Exactly one terminal transition wins. An exact replay by that winning
attempt is idempotent; a different terminal payload is rejected.

## Tool-Result Contract

The existing OpenClaw tool-result path remains semantic evidence and therefore
is separate from checkpoints. For a leased turn it must carry and validate the
current attempt ID and generation. A stale attempt cannot append tool evidence
after reclaim or cancellation.

Tool-result formation, review, deletion, and sensitivity rules remain those of
W23.

## HTTP Contract

Authenticated client routes add:

```text
POST /v1/client-operations/prepare
POST /v1/client-operations/reclaim
POST /v1/client-operations/heartbeat
POST /v1/client-operations/checkpoint
POST /v1/client-operations/complete
POST /v1/client-operations/fail
POST /v1/client-operations/cancel
```

The request names the channel and thread anchor but never the tenant. Prepare
and reclaim return the immutable delivery, current attempt, generation, lease
expiry, and latest checkpoint. Mutation routes return the current operation
receipt without exposing internal fingerprints or failure diagnostics from
another tenant.

All routes require the existing authenticated client role. Governance remains
operator-only and is not added to the long-running protocol.

## OpenClaw Integration

The maintained OpenClaw plugin uses `leased_v1`:

- `before_prompt_build` prepares or resumes the exact run operation;
- the plugin keeps current attempt metadata only as a performance cache;
- repeating `before_prompt_build` after plugin restart reconstructs the cache
  from PostgreSQL using the same run ID and prompt;
- successful allowlisted tool hooks record semantic tool results with fencing
  and advance a bounded progress checkpoint;
- `agent_end` completes or fails with the current fencing identity;
- stale-write or persistence failure remains fail-open for the visible agent
  result but is explicit in logs and produces no false Vermory receipt.

The integration does not persist API credentials or checkpoint data under
`~/.codex` or another client home. Runtime fixtures use the external Vermory
cache root.

Hermes remains on `bounded_v1` in W36. This is a compatibility control, not a
claim that Hermes long-running recovery has been qualified.

## Migration And Release Contract

Schema 25 extends `conversation_turns`; it does not add an independent generic
operation table. Migration constraints enforce protocol/state/checkpoint
coherence, tenant RLS remains enabled, and runtime-role grants require no new
table authority.

The binary supports exact schema 25. Schema compatibility tests, package
lifecycle tests, installer preflight, rollback behavior, reset, dump/restore,
and release manifests must move together with the migration.

## Hard Failures

- a stale generation renews, checkpoints, records a tool result, completes,
  fails, or cancels a reclaimed operation;
- a live lease is reclaimed;
- resume recomputes or changes the original delivery;
- checkpoint content enters observations, formation, memory, search, context,
  or provider input;
- cancellation can still create an assistant observation or formation work;
- a terminal race produces more than one semantic effect;
- restart loses the operation, attempt, generation, delivery, checkpoint, or
  terminal state;
- another tenant or continuity can resume or mutate the operation;
- a leased turn can bypass fencing through a bounded endpoint;
- the legacy bounded client contract regresses;
- credentials, raw tool objects, or unbounded checkpoint history are stored.

## Acceptance

W36 is qualified only when:

- H-017 and this case are frozen before schema implementation;
- deterministic tests first demonstrate the old contract cannot satisfy the
  W36 hard gates;
- migration, constraint, RLS, reset, dump/restore, and exact schema tests pass;
- store and HTTP tests cover replay, drift, reclaim, every stale mutation,
  terminal races, isolation, and semantic side-effect counts;
- PostgreSQL and Vermory process restart preserve exact delivery and checkpoint;
- the maintained OpenClaw plugin passes restart/resume and fenced tool-loop
  tests;
- a real OpenClaw runtime performs prepare, multiple checkpoints, restart or
  replay, and terminal write-back against the authenticated service;
- Hermes bounded-turn tests remain green;
- full Go, race, vet, integration-module, package, and protected-delivery checks
  pass on the exact pushed revision;
- failed and superseded runs remain recorded rather than rewritten away.

W36 does not claim distributed scheduling, offline conflict-free replication,
mobile background execution, arbitrary workflow orchestration, or automatic
promotion of operation progress into memory.
