# OpenClaw Runtime Integration Design

**Status:** Approved for autonomous implementation under the active Vermory goal  
**Date:** 2026-07-14  
**Official OpenClaw source reviewed:** `openclaw/openclaw` at `3c417f791cc5c2ff96282cf145ad04152b831722`  
**Installable compatibility target:** `openclaw@2026.6.11` (latest stable verified on 2026-07-14)

## 1. Purpose

This slice makes OpenClaw a real everyday-use consumer of Vermory. OpenClaw remains the chat, channel, session, and model harness. Vermory remains the authority for conversation continuity, governed memory, Global Defaults, explicit bridges, lifecycle, deletion, and audit.

The integration must prove this user-facing loop:

```text
OpenClaw message
-> OpenClaw resolves its canonical session
-> Vermory prepares governed semantic context
-> OpenClaw calls its configured model
-> OpenClaw returns the answer
-> Vermory records the user and assistant observations
-> later confirmed memory can continue across restarts and explicit links
```

OpenClaw is one client of the platform. It does not redefine Vermory as an OpenClaw plugin, a Web Chat wrapper, or an OpenClaw-specific memory backend.

## 2. Official Contract Findings

The official plugin SDK provides the required boundaries:

- `before_prompt_build` runs after the OpenClaw session messages are loaded and can return `prependContext` for the current model call;
- `agent_end` observes final messages, success, duration, and the same canonical `sessionKey` used by the run;
- `PluginHookAgentContext` supplies `runId`, `sessionKey`, `sessionId`, provider/model, channel, chat, sender, and workspace fields when known;
- the CLI runner executes both prompt-build and agent-end hooks, so a real local Grok-backed OpenClaw run exercises the same plugin boundary;
- non-bundled plugins need explicit `hooks.allowPromptInjection=true` to mutate prompts and `hooks.allowConversationAccess=true` to inspect `agent_end` conversation content;
- local plugins can be installed with `openclaw plugins install --link <path>` and verified with `openclaw plugins inspect <id> --runtime --json`.

The official `plugins.slots.memory` contract is exclusive and defaults to OpenClaw's own `memory-core`. Occupying it would force Vermory into OpenClaw's memory-plugin semantics and displace an existing OpenClaw subsystem. This integration therefore does not declare `kind: "memory"` and does not register `MemoryPluginCapability`.

## 3. Approaches Considered

### A. Exclusive OpenClaw Memory Slot

Vermory could replace `plugins.slots.memory` and implement OpenClaw's memory capability. This offers deep host integration but makes OpenClaw's memory model the controlling abstraction, creates slot conflicts, and weakens Vermory's independent workspace/conversation/default/bridge contracts.

**Decision:** rejected for V1.

### B. Typed Lifecycle Plugin

A normal official plugin calls Vermory before and after each agent turn. OpenClaw owns routing and inference; Vermory owns context and governance. The integration uses stable official hook types without taking over a host-exclusive slot.

**Decision:** selected.

### C. Custom OpenClaw Channel Or Gateway Fork

Vermory could proxy inbound messages or patch Gateway routing. This duplicates OpenClaw's channel and session responsibilities, adds coupling, and would not represent ordinary third-party OpenClaw deployment.

**Decision:** rejected.

## 4. Runtime Components

### 4.1 Vermory External-Turn API

The existing loopback HTTP service adds three server-owned integration routes:

- `POST /v1/integrations/openclaw/turns/prepare`
- `POST /v1/integrations/openclaw/turns/complete`
- `POST /v1/integrations/openclaw/turns/fail`

The request may supply only a stable operation ID, canonical OpenClaw session key, current message, answer, model label, and bounded failure information. It cannot supply tenant ID, continuity ID, lifecycle state, source authority, bridge state, or arbitrary Vermory channel names.

Vermory maps every request to:

```go
ConversationAnchor{
    Channel:  "openclaw",
    ThreadID: canonicalSessionKey,
}
```

The server process owns the tenant. Different Vermory server tenants cannot be selected by plugin input.

### 4.2 OpenClaw Plugin Package

The repository contains a publishable package under `integrations/openclaw` with:

- `openclaw.plugin.json` declaring id, config schema, and no exclusive kind;
- `package.json` with OpenClaw as a peer/dev dependency and `pnpm` scripts;
- a typed plugin entry using `definePluginEntry`;
- a bounded HTTP client;
- strict identity resolution;
- assistant-message extraction that ignores reasoning-only content;
- Vitest unit and registration-contract tests.

Plugin configuration contains only:

- `baseUrl`, default `http://127.0.0.1:8787`;
- `timeoutMs`, bounded to a safe positive range;
- `enabled`, default `true`.

It contains no tenant, continuity, provider, model-routing, or governance override.

## 5. Identity And Isolation Contract

The plugin requires both `ctx.sessionKey` and `ctx.runId` for a turn.

- `sessionKey` is the conversation anchor;
- `openclaw:<runId>` is the idempotent operation ID;
- `sessionId`, channel, sender, and chat metadata are diagnostic only and never replace a missing canonical session key;
- if either required value is absent, the plugin abstains and logs a bounded diagnostic without calling Vermory;
- no directory name, sender display name, prompt text, model guess, or previous active session may be used to infer identity.

This preserves the anchor-first platform rule: false binding is worse than a missed attachment.

## 6. Turn Lifecycle

### 6.1 Prepare

`before_prompt_build` sends:

```json
{
  "operation_id": "openclaw:<run-id>",
  "session_key": "<canonical-session-key>",
  "message": "<current-user-message>"
}
```

Vermory atomically or idempotently:

1. resolves or creates the exact conversation continuity;
2. begins the turn and stores the user message as an observation;
3. retrieves active Global Defaults;
4. retrieves active governed memory from the anchor and explicit active link group;
5. records the exact semantic delivery;
6. binds the delivery to the turn;
7. returns the persisted context and receipts.

The OpenClaw packet intentionally excludes Vermory's raw recent-conversation projection because OpenClaw already owns and supplies the canonical session transcript. This avoids duplicate history and preserves the bridge rule that linked raw history is never pooled.

The plugin injects only non-empty semantic context under a short reference-data wrapper. UUIDs, source paths, lifecycle fields, scores, tenant names, operation IDs, and audit metadata never enter the model-facing text.

### 6.2 Complete

`agent_end` on a successful run extracts the final visible assistant text and sends:

```json
{
  "operation_id": "openclaw:<run-id>",
  "session_key": "<canonical-session-key>",
  "answer": "<final-visible-assistant-text>",
  "model": "<provider/model-or-unknown>"
}
```

Vermory resolves the same anchor and operation, verifies the persisted turn and delivery belong to that continuity, stores the assistant observation, and marks the turn completed. Replays return the existing receipt and cannot create duplicate observations.

Assistant output remains a draft observation. It does not become active memory merely because a model produced it.

### 6.3 Fail

If OpenClaw reports an unsuccessful run or produces no visible final answer, the plugin sends a bounded failure code and message. Vermory marks the matching in-progress turn failed. A failed turn cannot create assistant memory.

## 7. Governance Contract

- ordinary OpenClaw user and assistant messages create observations only;
- no OpenClaw hook automatically confirms, promotes, corrects, deletes, links, or changes Global Defaults;
- existing Vermory operator/HTTP governance surfaces perform confirmation, correction, deletion, link, and default changes;
- explicit links share only active governed memory, never raw OpenClaw transcripts;
- current task-local instructions can override a Global Default for one model turn without mutating that default;
- deleted or superseded memory must be absent from every fresh OpenClaw delivery.

An OpenClaw-specific user-facing governance command can be added later, but it must call the same governed service and must not introduce fuzzy deletion or automatic promotion. It is not required for this slice.

## 8. Failure And Security Behavior

The plugin is fail-open for the chat harness and fail-closed for memory claims:

- if Vermory prepare fails or times out, OpenClaw continues without Vermory context;
- if completion persistence fails, OpenClaw's answer remains delivered, but the plugin logs that the observation was not persisted;
- logs contain endpoint, phase, status class, and bounded error text, never prompts, answers, context packets, credentials, or database identifiers;
- HTTP response bodies are size-bounded and parsed strictly;
- only loopback HTTP is accepted by the initial operator command;
- plugin config has no API key and no tenant selector;
- untrusted conversation text is wrapped as reference data, not system authority;
- OpenClaw must explicitly trust the plugin through `allowPromptInjection` and `allowConversationAccess`.

Durable offline spooling and public-network authentication belong to the later operations and identity/RLS slices. This slice must not claim successful persistence when the HTTP call failed.

## 9. Frozen Case O01

`O01-openclaw-home-maintenance` represents a normal everyday-use matter rather than a software project.

Trajectory:

1. OpenClaw session A discusses a home maintenance appointment, a current visit time, access instructions, and one temporary sensitive access code.
2. Selected durable facts are explicitly confirmed in Vermory; transient chatter remains observation-only.
3. Vermory and OpenClaw restart.
4. Session A asks for the current arrangement and receives active governed facts plus the thin Global Default.
5. OpenClaw session B is explicitly linked to session A and can receive the same governed matter without receiving session A's raw transcript.
6. Unrelated session C receives none of the maintenance facts.
7. The appointment is corrected; the old time becomes superseded and is absent from fresh delivery.
8. The temporary access code is deleted; exact, paraphrased, and related probes cannot retrieve it from fresh Vermory deliveries.
9. A one-turn request for English overrides the Chinese reply default for that turn only; the next turn returns to Chinese.
10. Reversing the A-B link stops future cross-session governed-memory retrieval, while each OpenClaw session retains only its own already-produced transcript history.

Deterministic evidence, not model self-report, decides isolation, lifecycle, and deletion.

## 10. Acceptance Gates

The OpenClaw slice is accepted only when all of the following are true:

- the O01 manifest, event trajectory, fixtures, and fixture hashes are frozen;
- repeated prepare/complete/fail requests are idempotent and conflicting operation reuse is rejected;
- missing `sessionKey` or `runId` causes plugin abstention;
- the plugin is accepted by the installable official OpenClaw runtime and registers both hooks;
- OpenClaw's CLI runner with the authenticated local Grok CLI consumes a real Vermory packet;
- plugin restart and Vermory restart preserve governed recall for the same canonical session key;
- explicitly linked session B receives active governed memory, while unrelated session C does not;
- raw history from A is absent from B's Vermory delivery;
- correction removes stale facts from fresh delivery;
- deletion removes the sensitive target from authority, projection, and fresh delivery after rebuild;
- Global Defaults apply and task-local override does not mutate them;
- Vermory outage leaves OpenClaw usable and does not create false persistence evidence;
- Go tests, Go race tests, `go vet`, tidy/build/diff checks, plugin unit tests, plugin typecheck/build, real plugin inspection, and real replay all pass;
- evidence records the official OpenClaw version, plugin runtime registrations, session keys in operator-only artifacts, model route, receipts, deterministic database assertions, and any failed probes.

## 11. Non-Goals

This slice does not replace OpenClaw memory-core, ingest arbitrary OpenClaw history, auto-confirm memories, implement fuzzy forget, expose a public hosted API, add multi-user auth, add PostgreSQL RLS, provide durable offline plugin queues, or prove Linux recovery. Those remain separate platform slices under the active overall goal.
