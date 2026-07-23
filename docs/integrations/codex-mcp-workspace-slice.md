# Codex MCP Workspace Slice

This guide replays `W02-codex-workspace-slice` through a real local Codex
client. It is an MCP integration check, not an evaluation of a model or a
claim that conversation continuity, Global Defaults, bridges, or HTTP are
implemented.

## Runtime Boundary

The local server exposes exactly two normal-flow tools:

| Tool | Effect |
|---|---|
| `prepare_context` | Resolves the supplied repository root. A confirmed workspace receives only active scoped facts and an opaque delivery receipt. An unknown root returns `needs_confirmation` and no context. |
| `commit_observation` | Records a coding-agent result against a delivery. It is always stored as `proposed`; the agent cannot select a tenant, correction authority, supersession target, deletion target, or workspace-binding override. |

`user_correction`, `source_update`, workspace binding confirmation, rebind, and
forget actions are trusted governance operations. They are intentionally not
normal agent MCP arguments. PostgreSQL is authoritative; the lexical search
projection can be rebuilt from active authority rows.

## Build And Register

Run these commands from the repository root. Use a dedicated local database
for the replay, not a shared application database.

```bash
go build -o ./bin/vermory ./cmd/vermory
codex mcp add vermory-w02 -- "$(pwd)/bin/vermory" mcp-stdio \
  --database-url "postgresql:///vermory_w02_codex?host=/tmp" \
  --tenant-id "codex-w02"
codex mcp get vermory-w02
```

The server writes JSON-RPC only to stdout. Startup diagnostics, migration
messages, and command errors are on stderr. Do not add ordinary logging to the
stdio transport.

## Binding And Seed

Before a coding client can receive context, a trusted operator must create the
database, migrate it, confirm the exact repository-root binding, and seed only
the approved W02 synthetic current fact. The runtime deliberately has no MCP
tool for this action: unknown roots must return `needs_confirmation` rather
than silently creating or selecting a continuity.

For the public W02 contract, use
[`case.json`](../../runtime/cases/W02-codex-workspace-slice/case.json) and
[`seed.sql`](../../runtime/cases/W02-codex-workspace-slice/seed.sql) as the
fixture definition. Replace `/fixtures/web-checkout` only through an explicit
trusted binding for the authorized replay repository. Never seed real source
text, credentials, personal paths, or private chat history into this fixture.

## Required Codex Replay

The Codex task must do all of the following in order:

1. Call `prepare_context` with a fresh operation ID, the exact confirmed repo
   root, and a concrete task.
2. Perform a real, narrow repository task using the returned context.
3. Produce a repository artifact and run its stated verification command.
4. Call `commit_observation` with the delivery receipt and a factual result.
5. Preserve the Codex final message, repository diff, verification output, and
   a redacted delivery/observation ledger under `artifacts/runtime/W02/`.

After a trusted correction changes `checkout_eta_v2` to `checkout_eta_v3`, a
new `prepare_context` call must expose only `checkout_eta_v3`. After trusted
forgetting and projection rebuild, exact `checkout_eta_v3` and the W02
paraphrase probe must return no active memory.

## Evidence Rules

The scripted Go acceptance test proves the W02 service contract. It does not
prove Codex invoked the MCP tools. A replay counts as real-client evidence only
when the ledger contains both a `memory_deliveries` entry and the corresponding
`agent_result` observation, alongside the repository artifact produced by
Codex. If either tool is not called, retain the run as a client-integration
failure instead of replacing it with a scripted pass.

`source_ref` is audit-only. Keep it to an approved opaque fixture reference or
repository-relative identifier; do not send absolute personal paths, raw source
content, tokens, or credentials into the tool.

## Executed Evidence

The official Codex CLI completed this contract against the W04 disposable workspace after the MCP server's non-interactive tool approval was explicitly scoped to `approve`. The run preserved two earlier approval-cancelled failures and the final successful `prepare_context -> file -> grep -> commit_observation` sequence. See [Codex MCP Real-Client Evidence](../evidence/2026-07-14-codex-mcp-real-client.md).
