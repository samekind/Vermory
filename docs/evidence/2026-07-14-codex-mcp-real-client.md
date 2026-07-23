# Codex MCP Real-Client Evidence

Date: 2026-07-14

## Scope

W04 proves a real official Codex CLI can consume a governed workspace context through Vermory MCP, perform a narrow repository task, verify its artifact, and write the result back as a proposed observation. It is client-integration evidence, not a model ranking and not proof of the entire coding-agent workflow.

The replay used a disposable Git repository, a dedicated PostgreSQL database, and synthetic facts only.

## Runtime

```text
Codex CLI: 0.144.3
MCP transport: stdio
Vermory command: mcp-stdio
Database: PostgreSQL 18.4, schema 9
Workspace continuity: one confirmed disposable repository root
```

The Codex MCP server was configured as `vermory-w04` and pointed at the dedicated database with tenant `codex-w04`. The run used `workspace-write` and no shell/full-access bypass. MCP tool approval was scoped to this server with `default_tools_approval_mode = "approve"` so non-interactive execution could invoke the local server without an interactive prompt.

## Preserved Failed Attempts

The first two attempts are retained in the ignored runtime artifact directory rather than replaced:

1. Default non-interactive approval: `prepare_context` was attempted twice and both calls returned `user cancelled MCP tool call`.
2. `approval_policy = "never"` for shell approvals: the same MCP calls were still cancelled because MCP tool approval is a separate control.

Both failed attempts made zero file changes, created zero deliveries, created zero observations, and did not guess the checkout flag. This is a client integration failure, not a scored success.

The third attempt used the documented per-MCP approval mode and completed.

## Successful Replay

Codex events show this exact order:

1. `vermory-w04.prepare_context` with operation `w04-codex-prepare-1` and the exact confirmed workspace root.
2. The server returned `status=resolved` and semantic context containing only `Use checkout_eta_v2 for the staged checkout release.`
3. Codex created `release-check.md` from that returned context.
4. Codex ran `grep -q 'checkout_eta_v2' release-check.md` and received exit code `0`.
5. Codex called `vermory-w04.commit_observation` with the delivery receipt, operation `w04-codex-observation-1`, and source reference `artifact:release-check.md`.
6. The write-back returned `memory_status=proposed` and `replayed=false`.

The preserved artifact is [W04 release-check.md](snapshots/2026-07-14-w04-codex-release-check.md). Its SHA-256 is:

```text
f9fb25c4436e8e4cfdebc1e7e97361ac19a33c36c08466bf741859b762e4e544
```

## PostgreSQL Ledger Checks

The database ledger independently returned:

```json
{
  "deliveries": 1,
  "agent_results": 1,
  "proposed_memories": 1,
  "active_v2": 1,
  "stale_active": 0,
  "distractor_delivered": 0,
  "observation_kind": "agent_result",
  "memory_status": "proposed",
  "source_ref": "artifact:release-check.md"
}
```

The other workspace held an unrelated operations fact. It was not present in the W04 delivery. The superseded checkout flag was not active and was not delivered.

## Evidence Boundary

- Codex genuinely invoked both MCP tools; this is not an internal Go-function replay.
- PostgreSQL proves the delivery, observation, lifecycle status, source reference, and isolation assertions.
- The produced artifact and `grep` exit code prove the narrow downstream task.
- The run does not claim Codex performed a broad software-engineering change, used tools beyond the requested shell verification, or validated conversation, Global Defaults, bridges, or HTTP behavior.
