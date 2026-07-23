# Source Conflict Candidate Implementation Plan

**Goal:** Add deterministic, durable, reviewable source conflict candidates
without allowing source imports or models to silently replace active memory.

## Task 1: Freeze Case And Contract

- [x] Add the W05 runtime fixture and 107 public casebook source, claims, and task.
- [x] Freeze keyed resolution, proposal, acceptance, rejection, replay, isolation,
  rebuild, and real-client gates in the design.

## Task 2: Schema And Store Lifecycle

- [x] Write failing migration, store, replay, lifecycle, projection, ambiguity,
  and cross-scope tests.
- [x] Add migration 10 observation kinds and `rejected` lifecycle state.
- [x] Add source candidate lookup, proposal replay, accept, and reject store
  operations while reusing governed memory authority.
- [x] Verify focused database tests and commit.

## Task 3: Governance Service And Operator CLI

- [x] Write failing service and CLI tests for `--key`, `propose-source`,
  `accept-candidate`, `reject-candidate`, inspect, and conflicting replay.
- [x] Implement the minimum service and CLI surface.
- [x] Verify focused tests and commit.

## Task 4: W05 Acceptance And Real Client

- [x] Run deterministic W05 acceptance before proposal, after proposal, after
  rejection, after acceptance, after rebuild, and against cross-tenant data.
- [x] Build an isolated release binary and execute a real Grok MCP task against
  the accepted current context.
- [x] Verify the artifact, PostgreSQL lifecycle/audit rows, write-back status,
  stale probes, and distractor exclusion.

## Task 5: Evidence And Delivery

- [x] Run the full database suite, runtime race set, reality race, vet, tidy,
  actionlint, GoReleaser snapshot, OpenClaw check/package, and diff check.
- [x] Record exact revisions, commands, hashes, row counts, failures, model
  route, and non-claims in evidence and integration docs.
- [x] Commit, push, update Draft PR 1, and require protected CI plus snapshot
  artifact success.
- [x] Keep the overall Vermory goal active for unkeyed formation, broader
  benchmarks, scale, sealed evaluation, signing, and final release acceptance.
