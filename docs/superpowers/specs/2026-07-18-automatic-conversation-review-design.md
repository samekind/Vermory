# Automatic Conversation Formation And Review Design

Date: 2026-07-18

Status: frozen for implementation

## Goal

Vermory must move from operator-triggered conversation formation to a durable,
fail-open product loop without allowing a model to activate memory:

```text
completed real-client turn
-> durable same-continuity formation request
-> tenant-scoped worker
-> review inbox with exact source evidence
-> explicit accept, reject, correct, or forget
-> later client turn receives only accepted current memory
```

The worker is automatic; governance is not. Provider output remains proposed
and cannot create Global Defaults, bridge continuities, or become current
memory without an explicit user or operator action.

## Frozen Case F02

F02 uses an authorized synthetic thesis-submission trajectory. One OpenClaw
turn states the current upload bundle, a portal deadline, and a faculty office.
Completing the visible turn must enqueue formation without delaying or changing
the OpenClaw answer. A tenant-scoped worker forms three proposed candidates.

The OpenClaw user then:

- lists pending candidates with a short reference, candidate content, and exact
  source quote;
- accepts the bundle and deadline;
- rejects the office fact;
- states a later deadline correction;
- accepts the generated update;
- receives the current bundle and corrected deadline in a fresh turn;
- explicitly forgets the deadline and verifies it is no longer delivered.

A separate Hermes session creates its own schedule and candidate. Its raw
observation and pending candidate must not appear in the OpenClaw review inbox.

## Scheduling Contract

Only a completed conversation turn advances the automatic formation request
for its exact tenant and continuity. Prepare replay, completion replay, failed
turns, assistant observations, and unrelated continuities cannot create a
second logical request.

The durable schedule records:

- the highest user-observation sequence requested for processing;
- the highest sequence terminally processed;
- pending, running, retry-wait, or idle state;
- a bounded lease and attempt count;
- the last formation run and safe failure code.

One schedule row exists per tenant and conversation continuity. A worker claims
only its configured tenant through the restricted runtime role. It processes a
bounded ordered user-observation window, creates a deterministic operation ID,
and advances the cursor only after a completed or abstained formation run.

Provider failure, timeout, malformed output, process termination, or lease
expiry leaves the request retryable. Chat completion remains successful even
when no worker or provider is available.

## Review Contract

The operator API exposes only review-safe candidate data:

- candidate memory ID;
- stable memory key;
- proposed content;
- exact source quote and source observation ID;
- new or update decision;
- optional target memory ID;
- creation time.

It does not expose provider raw output, hidden prompts, database rows, request
fingerprints, or credentials.

The authenticated review routes require operator or owner role. A client token
can persist and consume turns but cannot list, accept, reject, correct, or
forget memory.

The official OpenClaw plugin registers one direct `/vermory` command that
bypasses the LLM. It uses the existing client token for turn persistence and a
separate operator token for review actions. Supported subcommands are:

```text
/vermory memories
/vermory accept <candidate-ref>
/vermory reject <candidate-ref>
/vermory correct <memory-ref> <replacement>
/vermory forget <memory-ref>
```

References are resolved only inside the current OpenClaw session continuity.
Ambiguous or missing prefixes are rejected. The command is never exposed as a
model tool, and the model cannot approve its own candidate.

## Worker Contract

`vermory conversation-formation-worker` runs for one fixed tenant. It accepts
the same direct provider choices as manual formation, including explicit
non-thinking mode, and supports continuous polling or `--once` qualification.

The worker uses PostgreSQL as its only queue and authority. No Redis, second
memory database, or client-owned scheduler is introduced.

## Hard Failures

- chat completion waits for or fails because formation is unavailable;
- a replay creates another schedule or provider call;
- a worker reads another tenant or continuity;
- assistant output enters the formation manifest;
- a candidate becomes active without explicit review;
- a client token performs governance;
- one session can resolve another session's short candidate reference;
- rejected, superseded, or forgotten content is delivered as current;
- ordinary conversation creates a Global Default;
- provider failure advances the processed cursor or loses retryability;
- deletion leaves source, audit, delivery, projection, or review-inbox residue
  on a covered Vermory surface.

## Acceptance

W22 is qualified only when:

- F02 is frozen before implementation and passes deterministic validation;
- migration, RLS, tenant-aware foreign keys, schedule state, lease recovery,
  idempotency, retry, cursor, and reset tests pass in PostgreSQL;
- authenticated HTTP tests prove client/operator separation and candidate
  scope isolation;
- OpenClaw plugin tests prove direct command parsing, short-reference safety,
  separate tokens, bounded responses, and fail-open turn behavior;
- Hermes completed turns enqueue through the same server path without gaining
  a model governance tool;
- a Mac mini real-client run uses a real provider to create candidates, performs
  explicit OpenClaw review, consumes the corrected fact, forgets it, and
  preserves a complete failure ledger;
- final protected CI and release artifacts are independently verified.
