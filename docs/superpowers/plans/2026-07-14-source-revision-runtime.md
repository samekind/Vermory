# Source Revision Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an explicit source-authoritative revision path that atomically replaces one named active workspace fact, preserves unrelated facts and history, excludes the stale fact from future retrieval, and is exercised by a real coding-client MCP task.

**Architecture:** Reuse the existing PostgreSQL `source_update` observation, `supersedes_memory_id` relation, lifecycle transaction, and disposable search projection. Add one governance-service operation and one trusted local CLI command; do not add semantic auto-matching, a fact-key schema, document-wide overwrite behavior, or model-owned authority. Freeze a software-release case in which a canonical source revises one command while an independent timeout fact remains current.

**Tech Stack:** Go 1.24, PostgreSQL/pgx, Cobra CLI, existing Vermory MCP stdio server, official Codex CLI, JSON/Markdown evidence.

## Existing Authority

- Product Constitution sections 2, 4, 5, 6, 7, and 8 require source/change history, current-state correctness, source-aware delivery, and no stale fact represented as current.
- Hypothesis `H-005 Revision-oriented updates` is already in `testing` and explicitly requires progressive correction and conflicting-source evidence.
- `CommitObservationRequest` already permits `source_update` with `supersedes_memory_id`; the PostgreSQL transaction already validates tenant/continuity/active state, marks the target superseded, removes its projection, inserts the replacement, and records the revision relation.

## Global Constraints

- A source revision must name exactly one active memory; the platform does not infer semantic identity in this slice.
- The replacement observation kind remains `source_update`, not `user_correction`.
- A non-empty new `source_ref` is mandatory.
- The target must belong to the same tenant and confirmed workspace continuity.
- Replaying the same operation ID and identical payload returns the original receipt without a second transition.
- Reusing an operation ID with a different target, content, or source reference is rejected.
- Unrelated active facts remain active and searchable.
- Superseded content remains inspectable as history but never appears in active search, rebuilt projection, or model-facing context.
- No benchmark answer, `has_answer` field, LLM comparison, or semantic auto-match decides supersession.
- Model-facing context contains semantic content only.
- Failed real-client attempts remain retained and reported.
- PostgreSQL remains authoritative and the overall Vermory goal remains active.

---

### Task 1: Freeze The Source Revision Case

**Files:**
- Create: `casebook/cases/106-workspace-source-revision/source.md`
- Create: `casebook/cases/106-workspace-source-revision/claims.json`
- Create: `casebook/cases/106-workspace-source-revision/tasks.json`
- Modify: `internal/casebook/load_test.go`

**Interfaces:**
- Produces: case `106-workspace-source-revision` with one stale command, one current replacement command, and one unaffected timeout fact.
- Produces: deterministic `must_include` and `must_not_include` assertions for the downstream task.

- [x] **Step 1: Add a failing casebook loader assertion**

Extend the casebook loader/suite test to require the new case and its task. The task must require `pnpm exec release:verify --mode locked` and `800 ms`, and forbid `npm run release:verify -- --legacy`.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./internal/casebook -run 'Test.*Case' -count=1
```

Expected: FAIL because case `106-workspace-source-revision` does not exist.

- [x] **Step 3: Add the frozen case**

The source trajectory must state:

```text
The first release note used: npm run release:verify -- --legacy
The canonical release manifest was later revised to:
pnpm exec release:verify --mode locked
The new command replaces only the old release verification command.
The independent API timeout remains 800 ms.
```

Current claims contain only the replacement command and the unchanged timeout. The task asks Codex to create a release check artifact from current governed context.

- [x] **Step 4: Verify GREEN and commit**

Run the focused casebook test, then:

```bash
git add casebook/cases/106-workspace-source-revision internal/casebook/load_test.go
git commit -m "test: freeze workspace source revision case"
```

### Task 2: Explicit Source Revision Service

**Files:**
- Modify: `internal/runtime/governance.go`
- Modify: `internal/runtime/governance_test.go`
- Modify: `internal/runtime/postgres_store.go`

**Interfaces:**
- Produces: `GovernanceService.ReviseSource(ctx context.Context, repoRoot, memoryID string, write GovernanceWriteRequest) (GovernedObservationReceipt, error)`.
- Consumes: existing `Store.CommitGovernedObservation` with `ObservationKindSourceUpdate` and `SupersedesMemoryID`.

- [x] **Step 1: Write failing lifecycle tests**

Add tests proving:

```go
revised, err := service.ReviseSource(ctx, repoRoot, original.Memory.MemoryID, GovernanceWriteRequest{
    OperationID: "source-revision-v2",
    SourceRef:   "repo:release-manifest@v2",
    Content:     "Use pnpm exec release:verify --mode locked.",
})
```

Assertions:

- replacement is active and names the original memory in `SupersedesMemoryID`;
- original is superseded and no longer projected;
- the unrelated `800 ms` fact remains active and searchable;
- rebuilt projection does not revive the old command;
- cross-workspace target revision fails atomically;
- missing source reference fails;
- exact replay returns the original receipt;
- conflicting replay is rejected.

- [x] **Step 2: Verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 ./internal/runtime -run 'TestGovernanceSourceRevision' -count=1
```

Expected: FAIL because `ReviseSource` does not exist.

- [x] **Step 3: Implement the minimal service operation**

Validate `memoryID` and `SourceRef`, then call the existing scoped commit path:

```go
return s.commit(ctx, repoRoot, CommitObservationRequest{
    OperationID:        write.OperationID,
    Kind:               ObservationKindSourceUpdate,
    Content:            write.Content,
    SourceRef:          write.SourceRef,
    SupersedesMemoryID: memoryID,
})
```

Do not change the PostgreSQL schema. The existing governed-memory replay path
must compare the persisted `supersedes_memory_id` with the replay request so a
different target cannot be accepted under the same operation ID.

- [x] **Step 4: Verify GREEN and commit**

Run focused runtime tests and:

```bash
git add internal/runtime/governance.go internal/runtime/governance_test.go internal/runtime/postgres_store.go
git commit -m "feat: add explicit source revision governance"
```

### Task 3: Trusted Local CLI Path

**Files:**
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main_test.go`
- Modify: `docs/integrations/local-operator-workspace-slice.md`
- Modify: `README.md`

**Interfaces:**
- Produces: `vermory memory revise-source`.
- Required flags: `--database-url`, `--tenant-id`, `--repo-root`, `--operation-id`, `--memory-id`, `--source-ref`, and `--content`.
- Produces: the existing JSON mutation receipt shape.

- [x] **Step 1: Write failing command tests**

Require the command to exist and execute:

```bash
vermory memory revise-source \
  --repo-root /repo/release-console \
  --operation-id source-revision-v2 \
  --memory-id <v1-memory-id> \
  --source-ref repo:release-manifest@v2 \
  --content 'Use pnpm exec release:verify --mode locked.'
```

The integration test must inspect lifecycle history and active search after command execution.

- [x] **Step 2: Verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 ./internal/operatorcli ./cmd/vermory -run 'Test.*Memory.*Command|TestRootCommand' -count=1
```

Expected: FAIL because `revise-source` is absent.

- [x] **Step 3: Implement the command and documentation**

Wire the required flags to `GovernanceService.ReviseSource`. Keep `memory correct` as the user-authoritative path and document the distinction:

- `revise-source`: a trusted source revision replaces a named source-backed active fact;
- `correct`: an explicit user correction replaces a named active fact.

- [x] **Step 4: Verify GREEN and commit**

Run focused CLI tests, `go run ./cmd/vermory memory revise-source --help`, and:

```bash
git add internal/operatorcli/command.go internal/operatorcli/command_test.go cmd/vermory/main_test.go docs/integrations/local-operator-workspace-slice.md README.md
git commit -m "feat: expose trusted source revision CLI"
```

### Task 4: Real Coding-Client MCP Replay

**Files:**
- Create: `docs/evidence/2026-07-14-source-revision-runtime.md`
- Create: `docs/evidence/snapshots/2026-07-14-source-revision-grok-release-check.md`
- Modify: `docs/evaluation-matrix.md`
- Modify: Draft PR 1 body

**Interfaces:**
- Consumes: release `vermory` binary, dedicated PostgreSQL database, `memory add-source`, `memory revise-source`, and `mcp-stdio`.
- Produces: preserved client/tool artifacts, downstream repository artifact, lifecycle/database assertions, and a scoped evidence report. Official Codex failures remain explicit when the account path stops before MCP; a successful Grok replay is not relabeled as Codex.

- [x] **Step 1: Prepare an isolated runtime**

Build a `-trimpath` release binary, create a dedicated database, apply embedded migrations, confirm a disposable workspace, and ingest:

1. old release command as active source fact;
2. independent `800 ms` timeout as active source fact;
3. unrelated distractor fact in a different workspace;
4. source revision replacing only the old release command.

- [x] **Step 2: Execute a real coding client through MCP**

Configure only the disposable Vermory MCP server and ask the coding client to:

1. call `prepare_context`;
2. create `release-source-check.md`;
3. include the current release command and timeout;
4. verify the file deterministically;
5. call `commit_observation` with the task result.

Official Codex was attempted first and stopped before MCP because of unsupported
account models and then the account usage limit. The successful replay uses the
logged-in Grok CLI with an isolated `HOME`, user-scoped MCP configuration, and
`grok-4.5`. Preserve failed attempts. Do not use Gemini CLI, Mac mini NewAPI,
or provider API credentials.

- [x] **Step 3: Verify hard gates**

Require:

- artifact includes `pnpm exec release:verify --mode locked`;
- artifact includes `800 ms`;
- artifact excludes `npm run release:verify -- --legacy`;
- delivery includes only active current facts from the target workspace;
- old memory is `superseded`;
- replacement and timeout are `active`;
- replacement has `supersedes_memory_id=<old id>`;
- old memory has no search projection;
- distractor workspace content is absent;
- successful client write-back is `proposed`;
- projection rebuild preserves these assertions;
- exact and paraphrased stale probes do not return the old command.

- [x] **Step 4: Publish evidence**

Record exact binary/client revisions, commands, artifact hashes, PostgreSQL counts, lifecycle rows, failed attempts, and non-claims. This slice proves explicit source revision through one real Grok coder; it does not prove Codex success, automatic conflict detection, or general formation quality.

### Task 5: Release Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-source-revision-runtime.md`
- Modify: Draft PR 1 body

- [x] **Step 1: Run full release gates**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/runtime ./internal/operatorcli ./internal/mcpserver ./cmd/vermory
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -trimpath -o /tmp/vermory-source-revision-release ./cmd/vermory
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
git diff --check
```

- [x] **Step 2: Commit, push, and update Draft PR**

Commit evidence and documentation, push the feature branch, update Draft PR 1 with exact evidence and non-claims, and require remote CI success.

- [x] **Step 3: Complete this plan without closing the platform goal**

Mark every checkbox complete only after the evidence exists on the remote branch. Keep the overall Vermory goal active for automatic formation, broader original benchmarks, source-conflict inference, multi-session aggregation, withheld/sealed evaluation, scale, and final release acceptance.
