# Explicit Source Revision Runtime Evidence

Date: 2026-07-14

Tested implementation revision: `a2f4da345bac9215b7967ac9041aaa3e3d3ff817`

## Scope

This evidence exercises one explicit, source-authoritative revision through the
production PostgreSQL workspace runtime. A trusted operator names one active
memory, supplies a newer non-empty source reference, and replaces only that
fact. An unrelated fact remains current, the stale revision remains available
as history but leaves active retrieval, and a real coding client consumes the
result through MCP and writes its task result back as `proposed`.

This is a source-governance slice, not automatic conflict detection. Vermory
does not infer the target from semantic similarity in this path.

## Frozen Scenario

Case `106-workspace-source-revision` freezes a software release workflow:

| Role | Fact |
|---|---|
| Superseded source fact | `npm run release:verify -- --legacy` |
| Current source fact | `pnpm exec release:verify --mode locked` |
| Unchanged independent fact | API timeout `800 ms` |
| Other-workspace distractor | `ops_exception_queue_refresh` |

The committed case files have these SHA-256 values:

```text
source.md  3db2397e2067df1ea10e067fefbd9c0d1f245eda83a16dd6f65fa57ac8e97f18
claims.json  2b7653dda22ea329d9d3d745c75ff158ab21622878d21a4d991cbb8ce721fa23
tasks.json  56cacbe55103c8c31f8d7d64856b5c0c0fb181d3c0a66b5df20bd5439927b013
```

## Runtime

```text
Vermory binary source revision: a2f4da345bac9215b7967ac9041aaa3e3d3ff817
Vermory binary SHA-256: 9ac06863d71a1fc4b30c1984bf9a4d31688e8a6871117adec2d1045ee73517ff
Grok CLI: 0.2.101 (5bc4b5dfadcf)
Successful client model: grok-4.5
Official Codex CLI attempted: 0.144.3
PostgreSQL: 18.4
Schema version: 9
Dedicated database: vermory_source_revision_20260714010707
Tenant: codex-w06
```

The successful client used an isolated Grok `HOME`, no web search, no
cross-session memory, no subagents, no plan mode, and a user-scoped stdio MCP
configuration. `grok mcp doctor` reported one healthy server, a successful MCP
2025-06-18 handshake, and two discovered tools.

## Real MCP Replay

Grok session `955B4CA6-68EB-4D0E-9CB4-96BE91AC1776` executed this chain:

```text
prepare_context (operation w06-grok-prepare-1)
-> current command plus 800 ms, with no stale or distractor content
-> create release-source-check.md
-> deterministic positive and negative file checks
-> commit_observation (operation w06-grok-observation-1)
-> agent_result retained as proposed
```

The exported transcript records both MCP tool calls, the deterministic file
verification, delivery ID `d4b8c01a-4e5b-41e0-b372-ff9d41521868`, and
observation ID `efd141f6-e1f6-4988-9ee8-a67565f72332`.

The generated artifact is committed as [a normalized snapshot](snapshots/2026-07-14-source-revision-grok-release-check.md).

```text
runtime artifact SHA-256:
dc200b342162e5e6188447e040f6dc3f9593a0e086c7215618187a4877b3606a

successful transcript SHA-256:
abcd3998192b98c7ab8b49d9472dbffa8849d7f8403e4e5e74d124419741c701
```

## Deterministic Hard Gates

| Gate | Result |
|---|---|
| Old command lifecycle | Exactly one row, `superseded` |
| Replacement lifecycle | Exactly one row, `active`, with `supersedes_memory_id` pointing to the old command |
| Independent timeout | Remained `active` |
| Grok write-back | Exactly one `agent_result`, memory lifecycle `proposed` |
| Old search projection | `0` rows |
| Replacement projection | `1` row |
| Target task delivery | Included the current command and `800 ms`; stale and distractor positions were both `0` |
| Exact stale probe | Returned current command and timeout; stale and distractor positions were both `0` |
| Paraphrased stale probe | Returned the current command; stale and distractor positions were both `0` |
| Projection rebuild | `3` documents before and after, fingerprint `a86fa2d9945e49225efb6f23c26b0a3e` before and after |

The exact and paraphrased probes were executed by a second real Grok MCP
session after projection rebuild. The persisted delivery contexts, rather than
the model's self-report, establish stale suppression.

## Preserved Attempts

The ignored runtime artifact root retains non-secret evidence for failed
attempts:

1. Initial setup stopped while parsing migration logs mixed with a JSON receipt.
   Idempotent operation IDs allowed setup to continue without duplicate facts.
2. Official Codex attempt 1 requested `gpt-5.6-sol`; the ChatGPT account path
   rejected the model before any MCP call.
3. Official Codex attempt 2 requested `gpt-5.3-codex`; the ChatGPT account path
   rejected the model before any MCP call.
4. Official Codex attempt 3 used a catalog-supported model but hit the account
   usage limit before any MCP call. No Codex success is claimed for this slice.
5. Grok attempt 1 entered a non-TTY UI path and returned `Device not configured`.
6. Grok attempt 2 encountered the project-scope trust gate and reached the turn
   limit after leaving a valid artifact and delivery but no write-back.
7. Grok attempt 3 moved the same server to the isolated user's MCP scope and
   completed both MCP calls.

Debug logs that could contain temporary client authentication fields were
deleted and are not evidence artifacts.

## Automated Verification

Focused tests cover case loading, same-workspace replacement, unrelated-fact
preservation, cross-workspace atomic rejection, mandatory source reference,
exact replay, conflicting replay, projection removal, and rebuild behavior.

```bash
go test ./internal/casebook -run 'Test.*Case' -count=1

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 ./internal/runtime -count=1

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 ./internal/operatorcli ./cmd/vermory -count=1
```

The repository-wide release gates are recorded in the source revision plan and
Draft PR validation section. The final local gate run passed:

```text
go test -p 1 -count=1 ./...                         PASS
go test -race (runtime/operatorcli/mcpserver/cmd)  PASS
go vet ./...                                       PASS
go mod tidy with zero go.mod/go.sum diff           PASS
go build -trimpath                                 PASS
OpenClaw check: 5 files, 43 tests, typecheck/build PASS
OpenClaw package dry-run                           PASS
git diff --check                                   PASS
runtime credential scan                            0 matches
```

The final release build SHA-256 remained
`9ac06863d71a1fc4b30c1984bf9a4d31688e8a6871117adec2d1045ee73517ff`,
matching the binary used for the successful real-client replay.

## Claim Boundary

This slice proves explicit revision of one named source-backed workspace fact,
active-state retrieval after revision and rebuild, isolation from another
workspace, and one successful Grok coder consumption/write-back loop. It does
not prove automatic extraction, semantic conflict matching, document-wide
reconciliation, general source-authority inference, Codex success in this run,
formation quality, benchmark superiority, scale, or final release readiness.
