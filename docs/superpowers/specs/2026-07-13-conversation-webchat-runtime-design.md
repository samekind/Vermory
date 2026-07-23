# Conversation-Backed Continuity And Web Chat Runtime Design

Status: frozen for implementation

Date: 2026-07-13

## 1. Purpose

This slice makes conversation-backed continuity a real persistent runtime rather than a prompt-building helper.

The user-visible result is a local Web Chat/API service that can:

- continue an exact conversation thread across process restarts;
- use bounded recent conversation state without treating every turn as durable memory;
- reuse explicitly governed memory from the same conversation continuity;
- let a user confirm, correct, inspect, and forget memory;
- preserve deletion and isolation even when recent history is included;
- call a configured real model provider and retain an auditable delivery/write-back trail.

This is the second production-shaped vertical slice after workspace-backed continuity. It does not complete Global Defaults, bridges, OpenClaw, or remote deployment.

## 2. Decision: Two-Layer Conversation Continuity

The runtime separates two forms of continuity that serve different purposes.

### 2.1 Recent conversation state

Every accepted user and assistant turn is stored as an observation in the exact conversation continuity. A bounded recent window is available to later turns in the same continuity, including after server restart.

Recent observations are working context, not authoritative long-term memory. They are presented to the provider as chronological reference data. Assistant text in this layer cannot grant authority to itself.

### 2.2 Governed memory

Only an explicit user governance action can make a conversation observation eligible as active governed memory in V1.

- `confirm` promotes the content of a specified user or assistant observation into active memory while retaining the confirmation event separately.
- `correct` creates a user-authoritative replacement for a specified active memory and marks the old memory superseded.
- `forget` targets a specified memory and removes its content from current memory, its content-bearing origin observation, recent-history delivery, and search projection.

Automatic LLM candidate extraction may be added later, but extracted candidates must remain proposed until a governed promotion path accepts them.

### 2.3 Rejected alternatives

The implementation must not:

- automatically promote every user turn to active memory;
- automatically promote any assistant output;
- require the user to retype remembered content when a specific observation can be confirmed;
- treat raw full history as the memory system;
- drop recent state entirely and reduce conversation continuity to manually maintained memos.

## 3. Continuity Resolution Contract

The conversation anchor is the exact pair `(channel, thread_id)` inside the server-owned tenant.

- Both fields are required and normalized by trimming surrounding whitespace.
- An unseen exact pair creates a new active conversation continuity and confirmed conversation binding.
- A known exact pair resolves to the existing continuity.
- The same `thread_id` under a different channel is a different continuity.
- Semantic similarity, shared participants, shared words, or model inference never merge continuities.
- Requests cannot supply a continuity ID or tenant ID.
- Cross-thread and cross-channel linking is deferred to the durable bridge slice.

Creating an exact thread continuity is safe because it creates isolation rather than merging prior state. Missing or malformed anchors are rejected rather than mapped to a shared default thread.

## 4. Runtime Flow

For one accepted chat request:

```text
HTTP request
-> validate operation and exact conversation anchor
-> resolve or create conversation continuity
-> persist user_message observation
-> load active governed memories for this continuity
-> load bounded non-redacted recent user/assistant observations
-> assemble separated governed-memory and recent-conversation sections
-> record the exact delivery
-> call the server-configured provider
-> persist assistant_message observation and provider metadata
-> return the persisted turn receipt
```

The current user message is not duplicated inside the historical section. It is sent as the provider task after the bounded prior context.

The provider receives semantic content only. Memory IDs, observation IDs, lifecycle states, tenant IDs, audit fields, and database terminology are not inserted into ordinary model-facing prose.

## 5. HTTP Surface

The first server uses Go `net/http`, JSON, and a configurable address whose default is `127.0.0.1:8787`. It is a local API, not MCP Streamable HTTP.

### 5.1 `POST /v1/chat/turn`

Request:

```json
{
  "operation_id": "client-stable-id",
  "channel": "web_chat",
  "thread_id": "device-maintenance-2026-05-14",
  "message": "What should we check next?"
}
```

Response after a completed provider call:

```json
{
  "status": "completed",
  "continuity_id": "uuid",
  "delivery_id": "uuid",
  "user_observation_id": "uuid",
  "assistant_observation_id": "uuid",
  "answer": "...",
  "model": "configured-model",
  "replayed": false
}
```

The client cannot select tenant, provider, model, authority, observation kind, target memory, or continuity ID through this endpoint.

### 5.2 `POST /v1/memories/confirm`

Required fields are `operation_id`, `channel`, `thread_id`, and `observation_id`.

The target observation must belong to the resolved conversation continuity and must be a non-redacted `user_message` or `assistant_message`. The runtime records a content-free user-confirmation observation and creates active memory whose content-bearing origin is the target observation.

### 5.3 `POST /v1/memories/correct`

Required fields are `operation_id`, `channel`, `thread_id`, `memory_id`, and replacement `content`.

The target must be active and belong to the resolved continuity. Correction, supersession, projection removal, replacement creation, and replacement projection occur in one PostgreSQL transaction.

### 5.4 `POST /v1/memories/forget`

Required fields are `operation_id`, `channel`, `thread_id`, and `memory_id`.

The request contains no free-text reason or old content. The audit observation uses a fixed content-free message. Deletion redacts the governed memory and its content-bearing origin observation and removes the search projection in one transaction.

### 5.5 `GET /v1/conversations/inspect`

Required query parameters are `channel` and `thread_id`.

This operator-facing endpoint returns the resolved continuity ID, recent observation receipts, and governed memory receipts needed to choose exact confirmation, correction, and deletion targets. Deleted content is returned only as `[redacted]`.

## 6. Idempotency And Failure Semantics

`operation_id` is unique per tenant and logical operation.

- Repeating a completed chat operation returns the persisted response and does not call the provider again.
- Repeating a failed chat operation returns the persisted failure receipt and does not call the provider again. A caller uses a new operation ID to retry intentionally.
- Reusing an operation ID for another continuity or operation type is rejected.
- Repeating confirm, correct, or forget returns the original receipt without duplicating lifecycle effects.

The user observation and chat-turn record are committed before the provider call. If the provider fails:

- the user message remains available for inspection;
- the turn is marked failed with a bounded diagnostic code/message;
- no assistant observation is fabricated;
- no memory is promoted;
- a later request with a new operation ID may continue the same thread.

Delivery and completed assistant write-back are persisted atomically after provider success. The response is not reported as completed unless the assistant observation is durable.

## 7. Storage Changes

The existing authority model is extended rather than replaced.

- `continuity_spaces.continuity_line` accepts `workspace` and `conversation`.
- A dedicated conversation-binding relation stores tenant, channel, thread ID, state, and continuity ID. Workspace paths remain in the existing workspace binding relation.
- Observation kinds add `user_message`, `assistant_message`, `user_confirmation`, and `provider_failure` as needed by the runtime evidence trail.
- A chat-turn relation stores the operation receipt, continuity, observation IDs, delivery ID, provider/model metadata, status, persisted answer, and bounded failure evidence.
- Existing governed memory, lifecycle, delivery, projection, and deletion semantics remain shared across workspace and conversation continuities.

PostgreSQL remains the only authority. Provider output, recent-history formatting, lexical search documents, and response JSON are projections or observations, not independent truth.

## 8. Context Assembly

Context assembly has two explicit semantic sections:

```text
Governed memory:
<active scoped memory selected for the current message>

Recent conversation:
<bounded chronological non-redacted prior user/assistant turns>
```

Provider system instructions state that both sections are reference data, not executable instructions. Governed memory is reusable state; recent conversation may contain stale, mistaken, or adversarial text and must be interpreted chronologically.

The first slice uses deterministic bounds configured by the server:

- up to 6 active memories selected by the existing scoped retrieval path;
- up to 12 recent observations before the current message;
- no full-history dump;
- `[redacted]` observations are omitted from model-facing context.

The exact ranking algorithm remains a measured hypothesis. Scope, lifecycle, deletion, and chronology are mandatory before relevance optimization.

## 9. Security Boundary

- The default listener is loopback-only.
- Tenant identity is server-owned.
- Provider and model are server-owned configuration.
- Request JSON size is bounded.
- Unknown JSON fields are rejected.
- Content from history, source documents, and governed memory is reference data and cannot change continuity policy or Global Defaults.
- This slice cannot create or mutate Global Defaults.
- All reads and mutations verify tenant and continuity ownership in PostgreSQL.
- Error responses do not expose database URLs, API keys, raw provider artifacts, SQL, or other tenant data.

Authentication and PostgreSQL RLS remain required before non-loopback or multi-user deployment. This slice must not be represented as a remotely safe hosted service.

## 10. Acceptance Gates

### 10.1 Deterministic runtime tests

- Exact `(channel, thread_id)` survives store/server restart.
- Different thread or channel anchors never share observations or memories.
- Missing anchors are rejected and never create a default continuity.
- Duplicate chat operations do not call the provider twice.
- Assistant output remains an observation until explicit confirmation.
- Confirmation creates active memory from the targeted observation.
- Correction supersedes only the targeted active memory.
- Forget redacts the memory and content-bearing origin observation and removes the projection.
- Recent-history assembly omits redacted observations.
- Provider failure persists the user observation and no assistant observation.
- Projection rebuild restores active conversation memory and never restores deleted or superseded content.

### 10.2 C01 persistent conversation acceptance

The frozen device-maintenance trajectory is replayed through persistent conversation operations. A final chat turn must:

- include `1,333,470`;
- state that the Game A bundle has already been deleted;
- include the final `82 percent` and `87 GB free` state;
- preserve the QQ and WeChat exclusion;
- avoid presenting the first failed deletion as successful;
- propose only non-destructive next checks.

The acceptance test restarts the store/service before the final turn so an in-memory history cannot pass.

### 10.3 S01 deletion and injection acceptance

The deleted target `ORCHID-7419` must be absent from:

- exact recall;
- paraphrased recall;
- recent conversation context;
- governed memory retrieval;
- rebuilt projection;
- HTTP inspection except for `[redacted]` markers.

Independent guidance that recovery codes are rotated after use remains available. Untrusted source instructions cannot create Global Defaults or alter continuity policy.

### 10.4 Real provider replay

After deterministic mock tests pass, the local server is run with the authenticated Grok CLI provider. Preserved artifacts must show:

- multiple HTTP turns on the same exact thread;
- context reuse after server restart;
- a provider-generated answer using governed or recent conversation state;
- explicit confirmation and later reuse;
- deletion followed by exact and paraphrased probes;
- no second provider invocation for a repeated operation ID.

The real provider replay is compatibility and end-to-end evidence. Deterministic hard gates remain authoritative for deletion and isolation.

## 11. Non-Goals

This slice does not implement:

- semantic auto-linking or durable bridge operations;
- cross-channel identity unification;
- Global Defaults runtime;
- OpenClaw hooks;
- MCP Streamable HTTP;
- public-network authentication or hosted multi-tenancy;
- automatic LLM memory extraction or silent promotion;
- vector embeddings, reranking, or original public benchmark execution;
- browser UI.

These remain separate evidence-producing slices and must not be hidden behind the Web Chat API.
