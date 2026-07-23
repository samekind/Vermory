# Provider-Assisted Unkeyed Source Target Matching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable closed-set provider matching for trusted source facts that lack a `memory_key`, without allowing providers to change active memory.

**Architecture:** Snapshot current same-scope active keyed facts into an RLS-protected audit row, ask a provider to select exactly one listed key or abstain, then validate the unchanged snapshot and create the existing source candidate inside one PostgreSQL transaction. Provider output remains governance-only and operator accept/reject remains mandatory.

**Tech Stack:** Go 1.26, PostgreSQL 18 with PostgreSQL 16+ SQL compatibility, pgx v5, goose, Cobra, existing provider interface, Grok CLI, MCP.

## Global Constraints

- PostgreSQL is authoritative; `source_match_decisions` is not a disposable projection.
- No provider may create authority, cross scope, activate memory, or write global defaults.
- Candidate proposal and decision finalization are atomic against candidate-set drift.
- Database tests using `VERMORY_TEST_DATABASE_URL` run serially.
- The real acceptance path uses locally authenticated Grok CLI and a dedicated database.
- Gemini CLI and Mac mini NewAPI are not used.

---

### Task 1: Freeze W06 Case And Contract

**Files:**
- Create: `casebook/cases/108-workspace-unkeyed-source-target-match/source.md`
- Create: `casebook/cases/108-workspace-unkeyed-source-target-match/claims.json`
- Create: `casebook/cases/108-workspace-unkeyed-source-target-match/tasks.json`
- Create: `runtime/cases/W06-unkeyed-source-target-match/case.json`
- Create: `docs/superpowers/specs/2026-07-14-unkeyed-source-target-matching-design.md`

**Interfaces:**
- Consumes: W05 source candidate lifecycle and the existing reality case formats.
- Produces: frozen matched, abstained, invalid, drift, isolation, and real-client gates.

- [x] **Step 1: Write the W06 runtime and public casebook fixtures.**
- [x] **Step 2: Freeze provider permissions, durable audit fields, transaction rules, and non-claims.**
- [x] **Step 3: Validate JSON syntax and reality case inventory, then commit the frozen assets.**

### Task 2: Migration And Store Contract

**Files:**
- Create: `internal/store/postgres/migrations/00011_source_match_decisions.sql`
- Create: `internal/runtime/source_match_store.go`
- Create: `internal/runtime/source_match_store_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/rls_migration_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/authn/provision.go`
- Modify: `internal/authn/provision_test.go`

**Interfaces:**
- Consumes: `commitObservationTx`, `governObservationTx`, tenant context, active keyed memory, and source candidate lifecycle.
- Produces: `BeginSourceMatch`, `FinalizeSourceMatch`, `FailSourceMatch`, `InspectSourceMatch`, canonical candidate snapshots, and replay receipts.

- [x] **Step 1: Write failing migration tests for schema version 11, RLS, tenant-aware foreign keys, runtime grants, reset, and backup authority inventory.**
- [x] **Step 2: Run focused database tests and confirm they fail because migration 11 and store methods are absent.**
- [x] **Step 3: Add the minimum migration and store types for pending, matched, abstained, and failed decisions.**
- [x] **Step 4: Write failing store tests for exact replay, conflicting replay, closed-set validation, duplicate-key ambiguity, drift, unchanged content, and provider evidence isolation.**
- [x] **Step 5: Implement atomic finalization by reusing the existing source-candidate transaction helpers.**
- [x] **Step 6: Run focused store, RLS, authn, and operations tests serially until green.**
- [x] **Step 7: Commit the migration and store contract.**

### Task 3: Matching Service And Strict Provider Output

**Files:**
- Create: `internal/runtime/source_match_service.go`
- Create: `internal/runtime/source_match_service_test.go`
- Create: `internal/runtime/source_match_types.go`

**Interfaces:**
- Consumes: `provider.Provider`, `provider.GenerateRequest`, and Task 2 store methods.
- Produces: `NewSourceMatchingService` and `MatchSource` with terminal audit receipts.

- [x] **Step 1: Write failing service tests for correct match, abstain, empty set, invalid key, malformed JSON, trailing output, timeout, replay without provider recall, candidate drift, cross-tenant exclusion, and prompt injection.**
- [x] **Step 2: Run the focused service tests and confirm behavior failures, not fixture errors.**
- [x] **Step 3: Implement the strict JSON parser, bounded prompt, source/candidate data separation, artifact hashing, and durable failure mapping.**
- [x] **Step 4: Implement match orchestration without adding retry, extraction, ranking, or automatic activation.**
- [x] **Step 5: Run focused runtime tests serially until green and commit.**

### Task 4: Operator CLI

**Files:**
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Consumes: `NewSourceMatchingService`, existing Grok/OpenAI-compatible providers, and governance connection flags.
- Produces: `memory match-source` and `memory inspect-source-match` JSON commands.

- [x] **Step 1: Write failing CLI tests for required flags, provider construction, matched/abstained/failed output, replay, inspection, and absence from MCP tools.**
- [x] **Step 2: Run focused CLI tests and confirm the commands are missing.**
- [x] **Step 3: Add `--provider`, `--model`, `--base-url`, `--api-key-env`, and `--grok-command` only to `match-source`; default to `grok-cli` and `grok-4.5`.**
- [x] **Step 4: Implement stable JSON output without exposing raw provider artifacts or adding governance commands to MCP.**
- [x] **Step 5: Run focused CLI and command-surface tests serially until green and commit.**

### Task 5: W06 Real Grok And MCP Acceptance

**Files:**
- Create: `docs/evidence/2026-07-14-unkeyed-source-target-matching-runtime.md`
- Create: `docs/evidence/snapshots/2026-07-14-unkeyed-source-target-matching-grok-release-control-policy.md`
- Modify: `docs/evaluation-matrix.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: dedicated PostgreSQL database, release binary, locally authenticated Grok CLI, existing MCP server, and W06 fixture.
- Produces: reproducible matched/abstained evidence, accepted current context, model artifact, write-back, hashes, and non-claims.

- [x] **Step 1: Build an isolated release binary and migrate a dedicated W06 database to schema 11.**
- [x] **Step 2: Seed three local keyed facts plus one cross-tenant distractor and verify the pre-match context.**
- [x] **Step 3: Run real Grok match, ambiguous abstain, and unrelated abstain operations and preserve exact provider evidence hashes.**
- [x] **Step 4: Verify proposal invisibility, inspect the durable match, accept the candidate explicitly, and rebuild projections.**
- [x] **Step 5: Run isolated Grok through Vermory MCP to create `release-control-policy.md` and commit its result as proposed.**
- [x] **Step 6: Verify positive and negative artifact assertions, stale-query probes, cross-tenant isolation, lifecycle rows, RLS, and projection fingerprint.**
- [x] **Step 7: Record exact commands, revisions, versions, hashes, row counts, failures, and non-claims, then commit.**

### Task 6: Full Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-unkeyed-source-target-matching.md`
- Modify: Draft PR 1 body.

**Interfaces:**
- Consumes: all previous tasks and existing release gates.
- Produces: green local/remote verification, clean commits, pushed branch, and CI artifact evidence.

- [x] **Step 1: Run all Go database tests serially, the established race sets, `go vet`, `go mod tidy`, and diff checks.**
- [x] **Step 2: Run actionlint, GoReleaser check/snapshot, OpenClaw tests/typecheck/build/package dry-run, and release checksum checks.**
- [x] **Step 3: Run migration replay and native backup/restore with source-match authority, RLS, runtime-role, and projection assertions.**
- [x] **Step 4: Mark every completed checklist item only after fresh evidence exists.**
- [x] **Step 5: Commit, push `agent/grok-cli-runtime`, update Draft PR 1, wait for required CI, download its snapshot artifact, and verify all four archives.**
- [x] **Step 6: Keep the overall Vermory goal active for broader formation, hybrid retrieval, scale/fault qualification, sealed evaluation, signing, and final release acceptance.**
