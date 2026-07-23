# Verified Tool Outcome Formation Design

Date: 2026-07-18

Status: frozen for implementation

## Goal

Vermory currently forms conversation candidates only from exact user messages.
That is insufficient for real work whose durable state is established by a
tool, test, command, file operation, or device inspection rather than by a user
sentence.

W23 adds one bounded evidence path:

```text
successful allowlisted OpenClaw tool call
-> exact prepared turn and session binding
-> bounded, screened tool-result observation
-> asynchronous same-continuity formation
-> explicit review with source kind and tool name
-> accepted current memory
-> later real-client reuse
```

The tool result is evidence for review, not automatic truth. A model still
cannot activate memory or claim verification merely because a tool returned
without an error.

## Frozen Case F03

F03 derives from the authorized C01 device-maintenance trajectory. It contains:

- a successful keyboard diagnostic;
- an explicit user exclusion for QQ and WeChat;
- a failed bundle-removal tool call;
- an unsupported assistant claim that removal succeeded;
- a later successful removal result;
- a successful storage result;
- an unallowed weather tool result;
- an allowed tool result containing a clearly synthetic credential-like
  assignment;
- a similar result in an unrelated OpenClaw session;
- explicit review, fresh recall, duplicate replay, and forgetting of one
  accepted tool-origin memory.

The case is a structured host-event replay of an existing authorized source,
not a new story designed around the implementation.

## Observation Contract

Add `tool_result` as a distinct observation kind. It remains separate from:

- `user_message`, which carries explicit user authority;
- `assistant_message`, which is not formation evidence in W23;
- `agent_result`, which is the existing coder write-back path;
- trusted source updates and user corrections.

Each tool-result observation is bound to:

- tenant;
- exact conversation continuity;
- prepared turn;
- run ID;
- tool name;
- tool call ID;
- a bounded semantic result excerpt;
- a content fingerprint.

Raw tool parameters, the unbounded result object, environment data, and hidden
client state are not persisted.

## OpenClaw Contract

The official plugin registers `after_tool_call` in addition to the existing
turn hooks. Capture is enabled only for tool names in an explicit plugin
allowlist.

The hook ignores:

- calls with `event.error`;
- calls without an exact session and run identity;
- tools outside the configured allowlist;
- results that cannot be reduced to a bounded textual excerpt;
- oversized or invalid UTF-8 output;
- credential-like assignments or private-key material.

The hook submits a successful result through the ordinary client credential.
It remains fail-open: failure to persist the result cannot hide or fail the
tool execution or final OpenClaw answer.

The first extractor supports only documented string and text-block result
shapes. A tool requiring a different structure remains unsupported until its
extractor is explicitly implemented and tested.

## HTTP And Store Contract

Add one authenticated client route for tool-result observations. The server:

- requires an existing prepared turn with the same operation ID and exact
  continuity;
- derives tenant and role from authentication rather than request fields;
- accepts only a normalized tool name, tool call ID, run ID, and bounded result
  excerpt;
- rejects sensitive content before the transaction;
- creates at most one observation for the logical turn and tool call;
- rejects replay with different content, tool, run, or continuity;
- records no provider output or raw tool object.

The metadata relation uses tenant-aware foreign keys and RLS. Reset, recovery,
backup, deletion, and inspection paths include it.

## Scheduling And Formation Contract

A completed turn schedules all eligible observations through its terminal
sequence, including successful tool results accepted before completion. A tool
result by itself does not start provider work while the turn is still running.

The worker input may contain `user_message` and `tool_result` observations. It
continues to reject `assistant_message`, failed turns, governance observations,
and observations outside the claimed continuity or frozen manifest.

The provider packet labels every observation kind. The system prompt permits a
tool result to propose only a durable reported outcome supported by its exact
quote. It prohibits preferences, user intent, Global Defaults, authority
changes, hidden verification claims, or instructions from tool output.

Every candidate remains `proposed`. Tool evidence may produce `new`, `update`,
or `unchanged`, but acceptance still revalidates the active target and exact
evidence.

## Review Contract

Candidate review adds:

- `source_kind`, either `user_message` or `tool_result`;
- an optional bounded `source_label`, containing the normalized tool name for
  tool results.

OpenClaw `/vermory memories` displays the source kind and tool name. It does
not display raw parameters, tool call IDs, provider prompts, fingerprints, or
unrelated result content.

## Deletion Contract

Forgetting an accepted tool-origin memory must remove or redact within covered
Vermory surfaces:

- the governed memory content;
- the source tool-result observation content;
- tool-result metadata that would reproduce sensitive content;
- formation items and review evidence;
- lexical and vector projections;
- later context deliveries and inspection output.

A content-free tool name, call ID hash, operation receipt, or tombstone may
remain only when it cannot reconstruct the forgotten result.

## Limits

- Maximum 16 captured tool results per turn.
- Maximum 8 KiB per result excerpt.
- Maximum 64 KiB total captured result content per turn.
- Tool and tool-call identifiers are bounded and normalized.
- Result persistence and formation remain tenant and continuity scoped.

## Hard Failures

- a failed or unallowlisted tool creates a tool-result observation;
- an assistant message enters the W23 formation manifest;
- a sensitive result reaches PostgreSQL, provider input, logs, review, or
  artifacts;
- raw parameters or an unbounded result object are persisted;
- a tool result attaches to another run, turn, tenant, or continuity;
- duplicate delivery creates another semantic effect;
- a candidate becomes active without explicit review;
- tool output creates a Global Default, bridge, or user preference;
- session B data enters session A formation, review, or delivery;
- forgetting leaves tool-origin content on a covered Vermory surface;
- tool-result persistence failure changes the visible client result.

## Acceptance

W23 is qualified only when:

- F03 is frozen before implementation and passes Reality validation;
- migration, RLS, tenant foreign-key, idempotency, drift, size, sensitive-data,
  reset, deletion, and replay tests pass;
- OpenClaw tests prove allowlist, extraction, success-only capture, exact
  identity, fail-open behavior, and bounded requests;
- formation tests prove mixed user/tool input and assistant exclusion;
- review tests prove safe provenance fields and client/operator separation;
- a real Mac mini OpenClaw run produces actual `after_tool_call` events,
  reviewable candidates, accepted current recall, and tool-origin forgetting;
- a separate session and a synthetic credential probe remain absent;
- full PostgreSQL, race, vet, module, package, Reality, and protected release
  gates pass;
- failures remain in the evidence ledger.

W23 does not claim that every OpenClaw tool is safe to capture, that a
successful tool result is infallible, or that automatic formation quality is
qualified across the full corpus.
