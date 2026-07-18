# Cursor Agent Real-Client Qualification Design

Status: frozen for execution

Date: 2026-07-19

## 1. Purpose

W25 qualifies `cursor-agent` as a distinct external coding client of Vermory.
It does not use Cursor as an implementation delegate and does not relabel a
Codex or Grok run as Cursor evidence.

The qualifying path is:

```text
trusted workstation attachment for a confirmed canonical workspace
-> Cursor Agent starts Vermory MCP over stdio
-> prepare_context returns current accepted facts only
-> Cursor creates and verifies continuity-report.md
-> commit_observation records the result
-> PostgreSQL retains the result as proposed
```

The frozen reality case is
`reality/cases/W04-canonical-repository-cross-client`. It exercises the real
move from the personal repository to the independent `samekind/Vermory`
repository plus a synthetic non-secret marker and a separate same-name
workspace distractor.

## 2. Existing Failure

A historical Cursor request stopped with `ActionRequiredError: You have an
unpaid invoice`. On 2026-07-19, client login and model discovery succeeded, but
a fresh minimal generation request again exited `1` with the same account-level
error before MCP startup.

That attempt remains a client failure. MCP discovery, another Cursor model, a
Codex pass, or a Grok pass cannot convert it into a successful Cursor
qualification. W25 was frozen before the W26 attachment boundary was added;
any fresh execution uses the startup attachment contract below and does not
ask the model to provide `repo_root`.

## 3. Runtime Boundary

The external client runs in a disposable checkout or worktree. Project-local
`.cursor/mcp.json` contains only a temporary command reference for the W25
server; no repository credential or database password is committed.

The stable authority and retained evidence live on the Mac mini. Because the
workstation and Mac mini are not on one LAN, the project-local MCP command uses
the existing Qingdao SSH management path and runs the Mac mini Vermory binary
over stdio. PostgreSQL remains local to the Mac mini and is not exposed as a
network service.

The MCP surface remains exactly:

| Tool | Client authority |
|---|---|
| `prepare_context` | Supplies task text and consumes the exact workspace attachment fixed at MCP startup; cannot choose or override a binding. |
| `commit_observation` | Supplies a delivery receipt and result; receives `proposed` status. |

The client receives no tool for tenant selection, acceptance, correction,
deletion, promote, link, adopt, rebind, or projection control.

## 4. Frozen Data State

The dedicated W25 database contains two confirmed workspace continuities.

The canonical continuity contains:

- a historical active fact naming `jstar0/Vermory`, followed by a trusted
  source revision naming `https://github.com/samekind/Vermory`;
- the current marker `samekind-w25-current`;
- the independent-repository constraint.

The unrelated same-name continuity contains only `distractor-w25-only`.

The old repository fact must be superseded before the real client starts. The
distractor remains active in its own continuity so isolation is tested against
a real eligible row rather than an absent fixture.

## 5. Client Task

The client instruction requires this order:

1. Call `prepare_context` with an operation ID derived from the unique W25 run
   ID and the frozen task. The trusted launcher fixes the exact confirmed root
   before MCP starts, and the runner first proves that these operation IDs have
   no existing authority effects.
2. Stop without guessing if the result is not `resolved`.
3. Create `continuity-report.md` with exactly these semantic fields derived
   from governed context:

   ```text
   canonical_repository=https://github.com/samekind/Vermory
   continuation_marker=samekind-w25-current
   ```

4. Verify both expected values and absence of the stale personal repository and
   unrelated marker.
5. Call `commit_observation` with the returned delivery ID, the run-derived
   observation operation ID, and `artifact:continuity-report.md` as the source
   reference.
6. Repeat the exact observation call once and verify that the second response
   reports `replayed=true`.
7. Report the artifact and observation receipt without attempting governance.

The task does not reveal the current marker in its prompt. The client workspace
must not contain the W04 fixture, so the marker can only arrive through the MCP
delivery.

## 6. Acceptance Gates

W25 passes only when one fresh execution satisfies every gate:

1. Client binary, version, login identity class, selected model, flags, and
   workspace revision are recorded.
2. `cursor-agent mcp list` reports the project-local server ready, and
   `mcp list-tools` exposes `prepare_context` and `commit_observation` without
   governance arguments.
3. The generation process exits `0`; no account, quota, approval, MCP startup,
   or provider error is present.
4. Raw client events and the PostgreSQL ledger prove `prepare_context` occurred
   before artifact creation and `commit_observation` occurred afterward.
5. The delivery belongs to the exact confirmed canonical continuity and contains
   the current samekind repository and marker.
6. Delivery and artifact contain neither the superseded personal-repository
   value nor `distractor-w25-only`.
7. `continuity-report.md` passes all frozen deterministic checks and its SHA-256
   is recorded.
8. PostgreSQL contains exactly one qualifying delivery, one `agent_result`
   observation, and one corresponding `proposed` memory for the frozen
   operations.
9. The proposed result is absent from active retrieval and cannot become
   current without a separate trusted governance action.
10. The repeated `commit_observation` operation is idempotent, reports replay,
    and does not create a second authoritative effect.
11. Raw artifacts, normalized summary, SQL assertions, privacy scan, checksums,
    and every failed attempt are retained under a unique run directory.
12. Credential patterns, private keys, database passwords, full environments,
    private transcripts, and personal absolute paths appear zero times in
    committed evidence.

MCP discovery without generation, an internally scripted MCP call, or a model
answer without both ledger entries is not a pass.

## 7. Failure Semantics

Each client attempt gets its own immutable directory and exit status. The
runner never overwrites an earlier attempt. Failures are classified at least as:

- `client_account_blocked`;
- `client_quota_or_provider_failed`;
- `mcp_discovery_failed`;
- `mcp_tool_approval_failed`;
- `workspace_resolution_failed`;
- `artifact_failed`;
- `writeback_missing`;
- `governance_boundary_failed`.

The current invoice failure is `client_account_blocked`. W25 remains
unqualified until a later fresh run passes all gates.

## 8. Non-Claims

W25 does not rank models, qualify every Cursor model, prove editor GUI behavior,
test implementation delegation, qualify conversation continuity or Global
Defaults, publish a release, or complete Vermory. It qualifies one exact real
Cursor Agent workspace trajectory only after all hard gates pass.
