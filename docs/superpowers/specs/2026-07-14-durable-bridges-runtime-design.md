# Durable Bridges Runtime Design

**Status:** Frozen for implementation

**Date:** 2026-07-14

## 1. Purpose

Vermory continuities are isolated by default. A bridge is the explicit, durable governance action that allows selected continuity effects to cross that isolation without silently pooling all history.

This slice replaces the legacy in-memory `internal/bridge` result builders with PostgreSQL-authoritative operations whose effects are visible, idempotent, auditable, and reversible where the effect remains under Vermory's control.

## 2. Frozen Cases

### B01: conversation decision promoted into a software workspace

A release-planning conversation contains two confirmed facts and one unconfirmed tangent. The operator selects one confirmed decision and promotes it into an existing repository continuity.

Required behavior:

- only named active conversation memory is copied;
- recent chat history, unconfirmed observations, and unrelated memory are not copied;
- the target workspace receives the semantic fact through its normal MCP/workspace context path;
- the source conversation remains unchanged;
- reversing the bridge deletes the promoted target copy and leaves the source memory active;
- an export of selected current workspace facts produces a bounded semantic handoff view without merging any continuity.

### B02: two conversation channels linked, then separated; workspace path moved

Two exact conversation anchors belong to the same ongoing thesis-submission matter. A third similarly worded thread is unrelated. The operator links the first two, later reverses that link, and separately rebinds a moved workspace path.

Required behavior:

- before link, each thread sees only its own governed memory;
- after link, both linked threads can retrieve active governed memory from the linked matter group;
- recent raw turns remain anchor-local and are never pooled by link;
- the unrelated third thread remains isolated;
- reversing the link stops future cross-thread governed-memory retrieval;
- rebind preserves the workspace continuity ID and memory while moving the confirmed path;
- old path no longer resolves after rebind, new path resolves to the original continuity;
- reversing rebind restores the old path and retires the new path.

## 3. Shared Operation Model

Every bridge is represented by one `bridge_operations` row with:

- server-owned tenant;
- caller-owned idempotency `operation_id`;
- action: `promote`, `link`, `export`, `adopt`, or `rebind`;
- status: `active`, `reversed`, or `revoked`;
- source and target continuity IDs when applicable;
- source and target anchor descriptions;
- request fingerprint for conflicting replay detection;
- export title, target profile, and body when applicable;
- creation and reversal timestamps;
- optional reversal operation ID.

An append-only `bridge_events` table records `created` and `reversed`/`revoked` transitions. `bridge_memory_effects` records source and generated target memory IDs for promotion. `conversation_links` records active or reversed link edges.

PostgreSQL remains authoritative. Runtime receipts and exported bodies are views of these rows.

## 4. Promote Contract

`PromoteConversationToWorkspace` accepts:

- one existing conversation anchor;
- one confirmed workspace root;
- one or more explicitly selected source memory IDs;
- one operation ID.

The transaction:

1. resolves source and target under the same tenant;
2. locks and validates every selected source memory as active and conversation-scoped;
3. creates one target observation and one active `bridge_promoted` governed memory per selected source memory;
4. creates target search projections;
5. records source-to-target memory effects and an immutable bridge event.

Promotion is a governed snapshot, not a live alias. Later source corrections do not silently rewrite the target. A new governed action is required.

Reversal redacts only generated target memories and matching deliveries. Source observations and source memories remain unchanged.

## 5. Link Contract

`LinkConversations` accepts one primary conversation anchor and one linked conversation anchor. Repeating the action can attach additional anchors to the same primary group.

The link does not rewrite continuity IDs or move historical observations. Instead, conversation governed-memory retrieval resolves an active link group:

- a primary sees its own active memory plus all active linked children;
- a linked child sees the primary plus active siblings;
- recent raw observations remain local to the current exact anchor continuity;
- memory formation and correction remain owned by the continuity where they occurred.

V1 rejects nested or ambiguous link graphs:

- a linked child cannot simultaneously be a primary;
- a primary cannot already be linked under another primary;
- the same linked continuity cannot have two active primaries;
- source and target must be distinct active conversation continuities in the same tenant.

Reversal deactivates the edge. It stops future cross-continuity retrieval but does not falsify or erase historical deliveries that were valid when the link was active. Sensitive historical content requires an explicit memory deletion.

## 6. Export Contract

`ExportWorkspace` accepts:

- one confirmed workspace root;
- explicitly selected active memory IDs;
- a title;
- a target profile such as `general_chat` or `team_handoff`;
- one operation ID.

The export body contains only semantic content:

```text
Release handoff

- Use checkout_eta_v2 for the staged release.
- Run the smoke suite before rollout.
```

The export does not create or merge a target continuity. It is a bounded durable view with provenance in the bridge operation and memory-effect rows, while normal output omits UUIDs and database metadata.

Reversal marks the internal export `revoked` and redacts its stored body. Vermory cannot revoke copies already delivered outside the system, and the API/documentation must not claim otherwise.

## 7. Adopt And Rebind Contracts

`AdoptWorkspaceAnchor` adds a new confirmed alias to an existing workspace continuity while retaining existing confirmed aliases. It is suitable for an explicitly approved worktree, mirror, or second-device path.

`RebindWorkspace` moves one confirmed root to a new root:

- old binding becomes `retired`;
- new binding becomes `confirmed` for the same continuity;
- memory, deliveries, and continuity ID remain unchanged.

Both actions reject a target root already confirmed to any continuity. Adopt reversal retires only the added alias. Rebind reversal retires the new root and restores the old root.

## 8. Runtime Service

`BridgeService` owns the store and tenant. Public methods accept anchors and selected memory IDs, never tenant IDs or raw continuity IDs:

```go
PromoteConversationToWorkspace(ctx, request)
LinkConversations(ctx, request)
ExportWorkspace(ctx, request)
AdoptWorkspaceAnchor(ctx, request)
RebindWorkspace(ctx, request)
Reverse(ctx, request)
Inspect(ctx, bridgeID)
```

Every method validates input length, duplicate memory IDs, exact anchors, active lifecycle, same-tenant scope, and operation replay.

## 9. HTTP And CLI Surfaces

Loopback HTTP adds:

- `POST /v1/bridges/promote`
- `POST /v1/bridges/link`
- `POST /v1/bridges/export`
- `POST /v1/bridges/adopt`
- `POST /v1/bridges/rebind`
- `POST /v1/bridges/reverse`
- `GET /v1/bridges/{bridge_id}`

The local CLI adds equivalent `vermory bridge` subcommands.

Tenant identity remains server-owned. HTTP request bodies cannot supply continuity IDs, bridge status, source authority, or lifecycle overrides.

## 10. Idempotency And Reversal

- replaying the same operation ID with the same normalized request returns the original bridge receipt with `replayed=true`;
- reusing an operation ID for a different action, anchor, memory set, title, or profile fails;
- reversal has its own operation ID;
- replaying the same reversal returns the same reversed receipt;
- a different reversal operation against an already reversed bridge fails;
- failure rolls back the bridge row, effects, observations, memories, projections, bindings, links, and events atomically.

## 11. Security And Model-Facing Rules

- ordinary model clients cannot call bridge governance through workspace MCP tools;
- only active selected memories cross a bridge;
- proposed, superseded, deleted, redacted, and cross-tenant memories are rejected;
- untrusted source text cannot initiate or modify bridge operations;
- linked raw history is never pooled;
- normal model packets and export bodies contain semantic content only;
- inspection endpoints may expose technical receipts because they are operator/diagnostic surfaces.

## 12. Acceptance Gates

The bridge slice is accepted only when:

- B01 and B02 frozen case manifests and fixture hashes exist;
- migration constraints reject duplicate active link ownership and invalid action/status values;
- all five action types are durable, idempotent, inspectable, and correctly scoped;
- promote and reverse alter actual workspace context through PostgreSQL and MCP;
- link and reverse alter actual Web Chat governed-memory retrieval while raw history stays local;
- unrelated conversation and workspace continuities never receive bridged content;
- export contains only selected active semantic content and revocation redacts the internal body;
- adopt/rebind and reversal preserve continuity identity and resolve only intended paths;
- cross-tenant targets and unconfirmed anchors are rejected;
- full serial tests, race tests, `go vet`, tidy diff, build, and diff checks pass;
- a real external MCP replay demonstrates promote/reverse;
- a real Grok Web Chat replay demonstrates link/reverse without relying on model self-report for isolation.

Legacy pure functions in `internal/bridge` are not evidence of these gates and may remain only as compatibility helpers until their callers are removed.

## 13. Non-Goals

This slice does not implement automatic semantic linking, automatic promotion, full continuity merge, raw-history migration, split of previously merged history, external export recall, or public-network authorization. OpenClaw integration and hosted identity/RLS remain subsequent slices.
