# Governed Multi-Fact Document Formation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one PostgreSQL-authoritative, provider-assisted path that turns a bounded trusted workspace document into exact-span `new`, `update`, and `unchanged` formation items while keeping every new or changed fact proposed until explicit operator review.

**Architecture:** Persist one document-level run and current active-key snapshot before the provider call, validate strict JSON and exact source spans, then finalize all items atomically against the unchanged snapshot. Reuse the existing source-candidate observation and governed-memory lifecycle for review; do not add formation or review tools to MCP.

**Tech Stack:** Go 1.26, PostgreSQL 18 with PostgreSQL 16+ SQL compatibility, pgx v5, goose, Cobra, existing provider interface, locally authenticated Grok CLI, MCP.

## Global Constraints

- PostgreSQL is authoritative; formation runs and items are not search projections.
- Source files are regular UTF-8 text, at most 65,536 bytes, with no NUL byte.
- Provider output contains at most 16 exact-span items and never activates memory.
- `new` and `update` create existing `source_candidate/proposed` memories; `unchanged` creates audit only.
- One invalid item fails the whole batch with zero observations or memory candidates.
- Database tests using `VERMORY_TEST_DATABASE_URL` run serially.
- The real acceptance path uses locally authenticated Grok CLI and a dedicated database.
- Gemini CLI and Mac mini NewAPI are not used.

---

### Task 1: Freeze W07 Case And Public Contract

**Files:**
- Create: `casebook/cases/109-workspace-multifact-document-formation/source.md`
- Create: `casebook/cases/109-workspace-multifact-document-formation/claims.json`
- Create: `casebook/cases/109-workspace-multifact-document-formation/tasks.json`
- Create: `runtime/cases/W07-governed-document-formation/case.json`

**Interfaces:**
- Consumes: W05 candidate review, W06 unkeyed matching, and existing casebook/runtime fixture formats.
- Produces: frozen `new`, `update`, `unchanged`, injection exclusion, abstention, isolation, pre-review, post-review, stale-probe, and real-client assertions.

- [x] **Step 1: Write the W07 source fixture with one unchanged region, one retry update, one new rollback rule, one injection sentence, and one uncertain fallback sentence.**
- [x] **Step 2: Write claims and the final coder task requiring `us-east-1`, retry limit `5`, two-maintainer approval, and signed SLSA while forbidding retry `3`, static credentials, governance bypass, and invented fallback policy.**
- [x] **Step 3: Write `runtime/cases/W07-governed-document-formation/case.json` with exact source text, expected provider items, current facts, distractor, pre-review assertions, and post-review artifact assertions.**
- [x] **Step 4: Run `jq empty` on all JSON fixtures and `go test ./internal/casebook ./internal/reality -count=1`, then commit with `test: freeze governed document formation case`.**

### Task 2: Migration And Authoritative Store

**Files:**
- Create: `internal/store/postgres/migrations/00013_source_document_formation.sql`
- Create: `internal/runtime/source_formation_types.go`
- Create: `internal/runtime/source_formation_store.go`
- Create: `internal/runtime/source_formation_store_test.go`
- Create: `internal/runtime/source_formation_migration_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/rls_migration_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/runtime/tenant_pool_test.go`
- Modify: `internal/authn/provision.go`
- Modify: `internal/authn/provision_test.go`

**Interfaces:**
- Consumes: `listSourceMatchCandidatesTx`, `canonicalSourceMatchCandidates`, `commitObservationTx`, `governObservationTx`, and the existing tenant context.
- Produces: `BeginSourceFormation`, `CompleteSourceFormation`, `FailSourceFormation`, `InspectSourceFormation`, `SourceFormationReceipt`, and `SourceFormationItemReceipt`.

- [x] **Step 1: Define `SourceFormationStatus` (`pending`, `completed`, `abstained`, `failed`), `SourceFormationDecision` (`new`, `update`, `unchanged`), begin/completion requests, provider items, item receipts, and run receipts in `source_formation_types.go`.**
- [x] **Step 2: Write failing migration tests requiring schema version 13, both formation tables, RLS, one policy per table, tenant-and-continuity foreign keys, runtime grants, reset coverage, and backup-authority inclusion.**
- [x] **Step 3: Run `VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime ./internal/authn -run 'SourceFormation|RLS|RuntimeRole|Operations'` and confirm failure because migration 13 and store APIs are absent.**
- [x] **Step 4: Add migration 13 with `source_formation_runs` and `source_formation_items`, checks for hashes/status/decision/span fields, RLS policies, scope indexes, and tenant-continuity foreign keys to runs, items, targets, observations, and candidate memories.**
- [x] **Step 5: Add failing store tests for exact replay, conflicting replay, changed active snapshot, pending expiry, empty abstention, update/new/unchanged classification, duplicate key, overlapping span, invalid occurrence, batch atomicity, and cross-tenant exclusion.**
- [x] **Step 6: Implement `BeginSourceFormation` so it validates the confirmed workspace, snapshots sorted active keyed facts, stores only source metadata/hash/size, and rejects operation replay when the logical request or active snapshot changed.**
- [x] **Step 7: Implement `CompleteSourceFormation` as one serializable transaction that rechecks source hash/size, locks and compares the active snapshot, validates every decision, verifies exact quote occurrence and non-overlap against ephemeral source bytes, creates deterministic per-item observations/candidates, inserts item rows, and finalizes the run.**
- [x] **Step 8: Implement `FailSourceFormation`, pending expiry, inspection, canonical hashing, scanning helpers, and forget redaction/late-completion protection without storing the whole source document.**
- [x] **Step 9: Update reset, runtime-role validation/grants, RLS inventory, tenant-FK probes, and operations authority fingerprint for both tables.**
- [x] **Step 10: Run the focused database tests serially until green and commit with `feat: add governed document formation store`.**

### Task 3: Strict Provider Formation Service

**Files:**
- Create: `internal/runtime/source_formation_service.go`
- Create: `internal/runtime/source_formation_service_test.go`

**Interfaces:**
- Consumes: `provider.Provider`, `provider.GenerateRequest`, and Task 2 store methods.
- Produces: `NewSourceFormationService`, `NewSourceFormationServiceWithConfig`, `FormDocument`, and `InspectSourceFormation`.

- [x] **Step 1: Write failing parser tests for exact JSON, zero-item abstention, 17 items, unknown fields, trailing JSON, invalid decisions, invalid key syntax, empty quote/content/reason, out-of-range occurrence, duplicate keys, and prompt injection.**
- [x] **Step 2: Write failing service tests for provider success, timeout, cancellation, malformed output, source too large, invalid UTF-8, NUL input, replay without provider recall, active-snapshot drift, and detached terminal failure persistence.**
- [x] **Step 3: Implement a strict system prompt that treats document and active facts as untrusted data, permits only `new/update/unchanged`, and requests no source text beyond selected exact quotes.**
- [x] **Step 4: Implement bounded source validation, SHA-256 request identity, strict decoder with unknown-field rejection and EOF enforcement, item normalization, and provider artifact hashing.**
- [x] **Step 5: Reuse the two-minute provider deadline and detached five-second completion context; map timeout, cancellation, malformed output, invalid spans, and drift to durable failure codes.**
- [x] **Step 6: Run `VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'SourceFormation'` until green and commit with `feat: form governed memory from trusted documents`.**

### Task 4: Trusted Operator CLI

**Files:**
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Consumes: Task 3 service, existing provider builder, governance connection flags, and existing candidate lifecycle commands.
- Produces: `memory form-document` and `memory inspect-source-formation` stable JSON commands.

- [x] **Step 1: Write failing CLI tests for required file/source flags, UTF-8 and size failures, provider construction, completed/abstained/failed JSON, replay, inspection, candidate lifecycle projection, and absence from MCP tools.**
- [x] **Step 2: Run focused CLI tests and confirm both commands are absent.**
- [x] **Step 3: Factor the existing direct provider builder so `match-source` and `form-document` share `grok-cli`, `openai-compatible`, `siliconflow`, and `duojie` construction without changing current defaults.**
- [x] **Step 4: Implement `form-document --source-file` with regular-file validation, one bounded read, explicit `source_ref`, and stable output that excludes full source and raw provider output.**
- [x] **Step 5: Implement provider-free `inspect-source-formation` and include linked candidate lifecycle by reading existing governed memories.**
- [x] **Step 6: Run `VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/operatorcli ./internal/mcpserver ./cmd/vermory` until green and commit with `feat: expose governed document formation commands`.**

### Task 5: W07 Real Grok And MCP Acceptance

**Files:**
- Create: `docs/evidence/2026-07-14-governed-document-formation-runtime.md`
- Create: `docs/evidence/snapshots/2026-07-14-governed-document-formation-grok-deployment-control-policy.md`
- Modify: `docs/evaluation-matrix.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: dedicated schema-13 PostgreSQL database, isolated release binary, locally authenticated Grok CLI, existing two-tool MCP server, and W07 fixture.
- Produces: exact real-provider extraction evidence, pre-review isolation, accepted current context, final model artifact/write-back, stale probes, hashes, and non-claims.

- [x] **Step 1: Build an isolated release binary from the current implementation and migrate a dedicated W07 database to schema 13.**
- [x] **Step 2: Seed the three local active facts and one cross-tenant static-credential distractor, then verify pre-formation context and projection fingerprint.**
- [x] **Step 3: Run real Grok formation over the W07 document and preserve the run ID, provider artifact hash, active-snapshot fingerprint, exact item spans, and linked candidate IDs.**
- [x] **Step 4: Run a real injection-only or irrelevant document and require durable abstention with zero observations and candidates.**
- [x] **Step 5: Verify pre-review MCP context still contains retry 3 and excludes retry 5 plus rollback approval; accept both proposed W07 candidates explicitly and rebuild projections.**
- [x] **Step 6: Run isolated Grok through Vermory MCP to create and deterministically verify `deployment-control-policy.md`, then call `commit_observation` once and require `agent_result/proposed`.**
- [x] **Step 7: Run exact and paraphrased stale probes, cross-tenant checks, lifecycle counts, item/run RLS probes, projection rebuild equivalence, and a forget-redaction probe.**
- [x] **Step 8: Commit normalized artifact snapshot, exact commands, versions, IDs, hashes, row counts, corrections, failures, and explicit non-claims with `docs: record governed document formation evidence`.**

### Task 6: Full Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-governed-document-formation.md`
- Modify: Draft PR 1 body.

**Interfaces:**
- Consumes: all previous tasks and existing release gates.
- Produces: green local and protected-CI verification, downloaded snapshot checksums, clean commits, and an updated Draft PR while the overall Vermory goal remains active.

- [x] **Step 1: Run all Go database tests serially, the established race package set, reality race, `go vet`, `go mod tidy`, module diff, and `git diff --check`.**
- [x] **Step 2: Run actionlint, GoReleaser check/snapshot/checksums, OpenClaw install/check/package dry-run, migration replay, and native backup/restore with formation authority/RLS/runtime-role assertions.**
- [x] **Step 3: Commit and push `agent/grok-cli-runtime`, update Draft PR 1 with the W07 result and non-claims, wait for required CI, download its artifact, verify the GitHub digest and all four Go archives, then close every checklist item only from fresh evidence.**
- [x] **Step 4: Keep the overall Vermory goal active for broader formation quality, source authority ranking, hybrid retrieval, scale/fault qualification, withheld/sealed evaluation, signing, and final release acceptance.**
