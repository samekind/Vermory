# Durable Bridges Runtime

Vermory isolates workspace and conversation continuities by default. A bridge is the explicit operator action that moves or shares a bounded effect across those boundaries.

## Promote

Promote copies selected active conversation memory into a confirmed workspace. It does not merge the conversation transcript or move the source memory.

```bash
vermory bridge promote \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id release-flag-promote \
  --source-channel web_chat \
  --source-thread-id release-planning \
  --target-repo-root /workspaces/checkout \
  --memory-id '<confirmed-conversation-memory-id>'
```

The generated target memory is active in the workspace and appears through normal MCP/workspace context preparation. Reversal deletes the generated target copy and leaves the source active.

## Link

Link lets two exact conversation anchors share active governed memory while preserving separate continuity IDs and local raw history.

```bash
vermory bridge link \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id thesis-channel-link \
  --primary-channel openclaw_dm \
  --primary-thread-id thesis-submission \
  --linked-channel web_chat \
  --linked-thread-id thesis-submission
```

Linked anchors can retrieve each other's confirmed active memory. `Recent conversation` remains local to the current exact anchor. Similar unlinked threads are unaffected. Reversal stops future shared retrieval; it does not erase deliveries that were valid while the link was active.

V1 supports a primary with multiple direct children and rejects nested or ambiguous link graphs.

## Export

Export creates a bounded semantic view from selected active workspace memory. It does not create or merge a target continuity.

```bash
vermory bridge export \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id release-handoff-export \
  --repo-root /workspaces/checkout \
  --memory-id '<release-flag-memory-id>' \
  --memory-id '<smoke-gate-memory-id>' \
  --title 'Release handoff' \
  --target-profile team_handoff
```

The returned `export_body` contains the title and selected semantic facts only. Revocation redacts Vermory's internal export body. It cannot retract copies already delivered outside Vermory.

## Adopt

Adopt adds a new confirmed alias to an existing workspace continuity. The existing root remains confirmed.

```bash
vermory bridge adopt \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id checkout-worktree-adopt \
  --existing-repo-root /workspaces/checkout \
  --new-repo-root /worktrees/checkout-release
```

This is appropriate for an explicitly approved worktree, mirror, or second-device path. Reversal retires only the added alias.

## Rebind

Rebind moves a workspace continuity from one exact root to another.

```bash
vermory bridge rebind \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id thesis-workspace-move \
  --old-repo-root /workspaces/thesis-old \
  --new-repo-root /workspaces/thesis-new
```

The old binding becomes retired, the new binding becomes confirmed, and the continuity ID and governed memory remain unchanged. Reversal restores the old root and retires the new root.

## Inspect And Reverse

```bash
vermory bridge inspect \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --bridge-id '<bridge-id>'

vermory bridge reverse \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --operation-id '<new-reversal-operation-id>' \
  --bridge-id '<bridge-id>'
```

Inspection exposes operator diagnostics: action, status, source and target receipts, append-only events, and memory effects. Normal model-facing context and export prose contain semantic content only.

## Loopback HTTP

The Web Chat runtime exposes equivalent server-owned endpoints:

- `POST /v1/bridges/promote`
- `POST /v1/bridges/link`
- `POST /v1/bridges/export`
- `POST /v1/bridges/adopt`
- `POST /v1/bridges/rebind`
- `POST /v1/bridges/reverse`
- `GET /v1/bridges/{bridge_id}`

Request JSON cannot select `tenant_id`, continuity IDs, lifecycle status, source authority, provider, or model. Unknown fields are rejected.

## Hard Boundaries

- only named active governed memories can be promoted or exported;
- proposed, superseded, deleted, redacted, cross-tenant, or wrong-continuity memory is rejected;
- operation IDs are idempotent only for the same normalized request;
- retries check the durable operation receipt before resolving anchors whose state may already have changed;
- bridge failure rolls back the operation, events, generated memory, projections, links, and bindings together;
- normal workspace MCP retains only `prepare_context` and `commit_observation`; bridge authority is not exposed to coding agents.
