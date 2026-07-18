# Cursor Agent Real-Client Qualification

This is the replay guide for W25. It qualifies the real `cursor-agent` CLI as
an external Vermory client. It is not the Cursor implementation-delegation
workflow.

## Runtime Boundary

The database, Vermory binary, and retained evidence run on the Mac mini. The
workstation reaches only the MCP stdio process through the existing Qingdao
reverse SSH management path. Do not expose PostgreSQL to the network, add a
NewAPI route, or put credentials in `.cursor/mcp.json`.

The project-local MCP configuration is created and removed by
`scripts/cursor-client-qualification.sh`. The two temporary wrappers used by
the current Mac mini run are intentionally not committed because they contain
machine-specific SSH paths.

## Required Inputs

Use a disposable checkout that does not contain the W04 fixture or an existing
`continuity-report.md`. Use a fresh Mac mini PostgreSQL database and a unique
runner `--run-id`. The ledger wrapper must accept the run-derived prepare and
observation operation IDs and emit the W25 ledger JSON contract.

The runner requires all of these before it can report pass:

- Cursor reports the selected model as available and the account as logged in.
- project-local MCP is approved, ready, and exposes exactly the two normal-flow
  tools without governance arguments;
- the preflight ledger contains no rows for the run-derived operation IDs;
- the real client exits zero and creates the exact two-line artifact;
- the client stream shows the replayed second observation call;
- the post-run PostgreSQL ledger contains one delivery, one agent result, one
  proposed memory, zero active agent memories, and zero stale or distractor
  delivery content.

The runner does not use `--force` or `--yolo`. It refuses to overwrite an
existing run directory or artifact and removes the temporary project MCP
configuration after every attempt.

## Current Evidence

The current real-client attempt is recorded in
[`2026-07-19-cursor-agent-real-client-attempt.md`](../evidence/2026-07-19-cursor-agent-real-client-attempt.md).
Cursor MCP discovery passed after approval and SSH URI correction. The account
then returned `ActionRequiredError: You have an unpaid invoice` before either
Vermory tool was called. The failure remains a blocked qualification, not a
client pass.
