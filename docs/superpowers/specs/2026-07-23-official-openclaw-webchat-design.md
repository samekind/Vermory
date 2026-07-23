# Official OpenClaw WebChat Qualification Design

Status: frozen for W38 execution

Date: 2026-07-23

## Purpose

Vermory already has separate evidence for its OpenClaw lifecycle plugin, the
Gateway HTTP agent endpoint, direct Web Chat, conversation formation, and
browser governance. W38 closes a different user-facing gap: a person uses the
official OpenClaw Control UI as an ordinary chat client, while Vermory provides
governed continuity after OpenClaw's own transcript is deliberately reset.

The accepted path is:

```text
real Chrome
-> official OpenClaw Control UI
-> Gateway WebSocket chat.send / chat.history
-> official agent and plugin runtime
-> Vermory before_prompt_build / agent_end lifecycle
-> stateless real Grok CLI backend
-> PostgreSQL-authoritative observations and governed memory
```

This is a client qualification, not a new memory model and not a model ranking.

## Why Transcript Reset Is Required

Asking a model to repeat a fact in one uninterrupted OpenClaw conversation
would prove only that OpenClaw retained its transcript. W38 therefore resets the
OpenClaw transcript before each recall stage. The canonical OpenClaw
`sessionKey` remains stable, but Gateway returns a new backing `sessionId` and
the new transcript contains no earlier fact-bearing message.

An answer after reset can pass only when the active fact was delivered by
Vermory. An unrelated session is maintained as a negative control throughout
the run.

## Scenario

The browser conversation follows an ordinary household service appointment.
The initial durable state is Saturday at 10:00 with a requirement that the
technician contact the front desk before arrival. Lunch chatter is explicitly
temporary. The durable state is later corrected to Sunday at 14:00, then
forgotten after the appointment is no longer relevant.

This scenario is synthetic to protect personal data, but the runtime path,
browser, Gateway, model, plugin, PostgreSQL lifecycle, failures, and restarts
are real. The model may phrase answers naturally; deterministic assertions are
made against semantic facts and forbidden stale values rather than an exact
generated sentence.

## Frozen Runtime Boundary

- OpenClaw is pinned to `2026.6.11` for this run.
- The Control UI must be served by the real Gateway and communicate over its
  WebSocket API.
- Chrome interaction must use the rendered official UI for user messages and
  `/vermory` governance commands. A protocol client may inspect or reset a
  session, but it cannot substitute for the accepted browser interactions.
- Grok runs through a one-turn CLI backend with `sessionMode: none`; ambient
  Grok memory, web search, subagents, and tool access are disabled.
- PostgreSQL is the only Vermory authority. OpenClaw JSONL, browser storage,
  provider output, and search rows are evidence or projections.
- Formation may use a separate direct real provider. It can only create
  reviewable candidates. The user/operator must accept, correct, and forget
  through the official WebChat command path.
- Runtime state, package caches, build products, PostgreSQL, and raw evidence
  are rooted below `/Volumes/JSData`. A short `/tmp` PostgreSQL socket symlink
  is permitted only because macOS limits Unix socket path length; it is removed
  after the run.

## Accepted Trajectory

```text
fresh external PostgreSQL schema 0 -> 25
-> authenticated Vermory service
-> isolated OpenClaw Gateway with official Vermory plugin
-> official Control UI opened in real Chrome
-> browser sends initial fact and temporary chatter
-> real Grok answers and plugin completes one durable turn
-> real formation worker produces reviewable candidates
-> browser runs /vermory memories and accepts the durable candidate
-> reset OpenClaw transcript, retain canonical sessionKey
-> browser recalls the fact from Vermory context
-> prove unrelated session has no fact
-> browser corrects the fact
-> reset transcript and recall current-only state
-> browser forgets the current memory
-> reset transcript and run exact plus paraphrased deletion probes
-> refresh browser
-> restart Gateway and Vermory
-> verify continuity, isolation, deletion, replay, and PostgreSQL residue
```

## Evidence Separation

The run retains three evidence levels:

1. Raw local evidence contains minimized Gateway logs, browser screenshots,
   network metadata, provider receipts, database assertions, and failed probes.
   It remains on the external volume and is not committed.
2. The public JSON snapshot contains normalized versions, counts, booleans,
   hashes, and bounded answer assertions. It contains no token, provider auth,
   database URL, personal path, proxy endpoint, or private raw transcript.
3. The public Markdown report explains exactly what ran, every accepted gate,
   preserved failures, and non-claims.

The implementation revision and case-file SHA-256 bind the accepted report.
Browser screenshots are evidence of the visible surface, while PostgreSQL and
Gateway assertions establish authority and transcript boundaries.

## Failure Semantics

- Failure to load the official Control UI is a browser-surface failure.
- Failure before a Grok request is a harness or CLI-backend failure, not model
  evidence.
- A completed Grok response with missing Vermory write-back is a plugin
  persistence failure even when the user sees an answer.
- Recall from an unreset transcript is rejected as continuity evidence.
- A formation parse failure remains a provider-formation failure; manually
  injecting the expected memory cannot replace that gate.
- Cross-session delivery, stale-current delivery, deletion residue, or secret
  exposure fails W38 regardless of the aggregate pass count.
- Every retry receives a new retained attempt directory; accepted evidence does
  not overwrite failed logs.

## Claim Boundary

Passing W38 means that the exact official OpenClaw WebChat stack used in the run
can consume Vermory continuity through a real model and complete explicit
formation review, recall after host-history reset, correction, forgetting,
isolation, refresh, restart, and idempotent replay.

It does not qualify all OpenClaw channels or versions, all browsers, all model
outputs, automatic governance, public Internet deployment, or model quality.
The host transcript is not erased by Vermory deletion; the qualification asks
whether deleted content can enter fresh Vermory delivery after the host
transcript has been reset.
