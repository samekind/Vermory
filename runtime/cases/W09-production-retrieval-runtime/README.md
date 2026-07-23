# W09 Production Retrieval Runtime

This frozen public case qualifies the first opt-in production semantic
retrieval profile. It is a software release and database deployment workflow,
not a legacy competition scenario.

## Workspace Trajectory

1. Confirm the release-control workspace.
2. Add the old one-maintainer rollback rule, then revise it to the current
   two-maintainer rule.
3. Add the manifest path, canary flag, signature error code, model identifier,
   and one same-workspace release-approval distractor.
4. Add a legacy emergency token and delete it.
5. Commit one agent result that remains proposed.
6. Add a semantically similar rollback rule in another workspace and another
   tenant.
7. Process durable projection events through the fixed-tenant worker.
8. Compare lexical, shadow, and vector retrieval through the workspace MCP
   service boundary.

The Chinese paraphrase must miss under the unchanged lexical default and hit
the current two-maintainer fact under vector retrieval. Shadow must return the
exact lexical context bytes. Exact technical retrieval must return the path,
flag, error code, and model identifier. Proposed, superseded, deleted,
cross-workspace, and cross-tenant content is forbidden.

## Conversation Trajectory

1. Form confirmed governed facts in an OpenClaw thread, a Web Chat handoff
   thread, and an unrelated Web Chat thread.
2. Link only the OpenClaw and handoff threads through the durable bridge API.
3. Process the resulting events through the fixed-tenant worker.
4. Prepare a real Web Chat/OpenClaw HTTP turn from the linked thread.

The prepared context must include the accepted Friday 22:30 migration decision
and its read-only precheck. It must not include the unlinked next-month thread.

## Failure And Recovery Gates

- Provider failure, cursor lag, and an operationally empty vector projection
  return the exact lexical IDs and order.
- A deletion or supersession committed during embedding wins over late worker
  completion.
- Rebuild deletes only the selected tenant/profile projection and resets its
  cursor; replay restores result IDs without changing authoritative rows.
- Restricted-role RLS returns only the selected tenant's events, cursor,
  vectors, and audits, and rejects cross-tenant references.
- Schema 14 dump/restore preserves events, cursors, audits, and disposable
  vector rows; deleting and replaying vector rows after restore preserves the
  authority fingerprint.

Deterministic embeddings in automated tests prove runtime mechanics only. Real
SiliconFlow `BAAI/bge-m3` and real Grok/Web Chat consumption are recorded in the
separate W09 evidence run.
