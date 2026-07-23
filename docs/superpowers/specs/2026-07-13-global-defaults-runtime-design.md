# Global Defaults Runtime Design

**Status:** Frozen for implementation

**Date:** 2026-07-13

## 1. Purpose

Global Defaults is Vermory's thin, strongly governed cross-continuity layer. It carries only settings that remain valid across workspace-backed and conversation-backed use, such as the user's stable reply language. It is not a general personal-memory pool and does not receive automatic promotions from chat, imported documents, model output, or source text.

This runtime completes the third continuity line without creating a separate memory product. It reuses PostgreSQL authority, observations, governed-memory lifecycle, delivery receipts, redaction, and projection rebuilding while adding the stricter authority and keyed semantics required by global settings.

## 2. Frozen Cases

### G01: stable default with local override

Source: `reality/cases/G01-language-default-local-override`.

Required behavior:

- an operator explicitly sets `reply_language` to the semantic rule "Default user-facing replies to Chinese unless the active task explicitly requests another language.";
- both a conversation consumer and a workspace consumer receive that rule;
- an English instruction inside one MCM task overrides the default only for that task;
- the local English instruction never creates, replaces, or supersedes the global default;
- a later unrelated Chinese request receives the unchanged Chinese default;
- correcting or deleting the default is reflected in both consumers on replay.

### S01: untrusted source cannot promote

Source: `reality/cases/S01-deletion-and-source-injection`.

Required behavior:

- imported or retrieved source text has no code path that can create or mutate Global Defaults;
- ordinary conversation observations and model responses have no code path that can create or mutate Global Defaults;
- deleting a default redacts its governed-memory content, origin observation, search projection, and historical deliveries that contain its exact semantic content;
- deleted content does not reappear after projection rebuild or through either consumer.

## 3. Authority Contract

Global Defaults may be changed only through explicit management operations owned by the Vermory server:

- `set`: create a new keyed default when that key has no active value;
- `correct`: replace one named active default while retaining the same key;
- `forget`: delete one named default and redact its authority trail;
- `inspect`: list the complete visible lifecycle for operator review.

There is intentionally no `promote from source`, `infer from chat`, or `auto-learn preference` operation in this vertical slice. A future promotion workflow must be a separately authorized, audited bridge and cannot be added by reusing ordinary observation APIs.

## 4. Data Model

Each server-owned tenant has exactly one active `global_defaults` continuity.

Each governed default has:

- `memory_key`: a normalized stable key such as `reply_language`;
- semantic `content` suitable for direct model consumption;
- `lifecycle_status`: `active`, `superseded`, or `deleted`;
- an explicit origin observation;
- optional `supersedes_memory_id` revision linkage.

Database invariants:

- at most one active `global_defaults` continuity exists per tenant;
- at most one active governed memory exists per `(tenant, global-default continuity, memory_key)`;
- all mutations remain tenant-scoped and continuity-scoped;
- correction targets an active default and preserves its key;
- deletion targets a memory in the tenant's global-default continuity;
- duplicate `operation_id` is idempotent only when it represents the same logical operation.

The existing PostgreSQL tables remain authoritative. Search documents are rebuildable projections only.

## 5. Runtime API

The Go runtime exposes a `GlobalDefaultsService` with these operations:

```go
Inspect(ctx context.Context) (GlobalDefaultsInspection, error)
Set(ctx context.Context, request SetGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error)
Correct(ctx context.Context, request CorrectGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error)
Forget(ctx context.Context, request ForgetGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error)
```

`Set` validates and normalizes the key. Keys use lowercase ASCII segments separated by underscores and are capped at 64 bytes. Content is required, trimmed, and capped at the existing governed-memory input boundary.

`Correct` and `Forget` target a memory ID rather than trusting a caller-supplied key. The store resolves and verifies the active memory and its key inside one transaction.

## 6. Consumer Contract

Both workspace and conversation consumers load all active global defaults before continuity-scoped memory.

Model-facing packet shape:

```text
Global defaults:
Default user-facing replies to Chinese unless the active task explicitly requests another language.

Governed memory:
...
```

Normal model-facing prose contains only semantic content. It must not expose `memory_key`, UUIDs, lifecycle status, authority scores, database fields, or audit metadata.

Precedence is explicit:

1. current user or task instruction;
2. current continuity's governed, relevant context;
3. Global Defaults.

A local instruction is consumed for the current task but never written back into Global Defaults. The system prompt tells the model that active task instructions override defaults without mutating them.

Workspace delivery continues to attach to the workspace continuity so observation write-back remains correctly scoped. Conversation delivery continues to attach to the conversation continuity. The global-default continuity is read-only from both consumption paths.

## 7. HTTP And CLI Surfaces

The loopback Web Chat server adds explicit operator endpoints:

- `GET /v1/defaults`
- `POST /v1/defaults/set`
- `POST /v1/defaults/correct`
- `POST /v1/defaults/forget`

Tenant identity remains server-owned and is never accepted from request JSON.

The local CLI adds:

- `vermory defaults inspect`
- `vermory defaults set --operation-id ... --key ... --content ...`
- `vermory defaults correct --operation-id ... --memory-id ... --content ...`
- `vermory defaults forget --operation-id ... --memory-id ...`

Both surfaces call the same runtime service and return durable mutation receipts.

## 8. Failure And Security Rules

- an existing active key makes `set` fail; callers must use `correct` with a visible memory ID;
- cross-tenant memory IDs fail without revealing another tenant's content;
- correction of a superseded or deleted default fails;
- deletion is idempotent only through replay of the same operation; a fresh operation against an already deleted memory returns its deleted state without restoring content;
- provider failure cannot mutate Global Defaults because provider execution has no mutation capability;
- source import and conversation confirmation remain scoped to their own continuity lines;
- projection rebuild includes only active defaults and never restores redacted content;
- historical deliveries are redacted when the exact deleted content was previously injected.

## 9. Acceptance Gates

The runtime is accepted only when all of the following are reproducible:

- migration enforces singleton continuity and unique active key;
- service tests prove explicit set, replay, correction, deletion, cross-tenant isolation, and key uniqueness;
- conversation tests prove defaults are delivered before scoped memory and local instructions do not mutate them;
- workspace tests prove the same default is delivered without changing workspace attachment;
- HTTP tests prove server-owned tenant identity and reject unknown fields;
- CLI tests complete inspect, set, correct, and forget against PostgreSQL;
- G01 acceptance replays chat and workspace consumers before and after correction/deletion;
- S01 acceptance proves source and ordinary chat paths cannot promote into Global Defaults;
- exact deleted content is absent from active memory, search projection, delivery history, and rebuilt projection;
- a real Grok CLI replay demonstrates Chinese default, English task-local override, later Chinese behavior, unchanged inspection, and post-delete absence;
- full serial tests, race tests, `go vet`, `go mod tidy` diff check, and binary build pass.

Static fixtures, mock-only provider tests, or direct store calls alone do not satisfy the runtime acceptance.

## 10. Non-Goals

This slice does not add automatic preference learning, a management UI, fuzzy global-default retrieval, per-default policy scripting, or cross-tenant sharing. Those features are unnecessary for G01/S01 and would weaken the explicit-authority boundary.
