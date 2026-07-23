# Real Git Workspace Topology Qualification

Date: 2026-07-22

Reality case: `W05-trusted-workspace-attachment`

Runtime case: `W31-real-git-workspace-topology`

Status: `18 / 18 PASS`

## Boundary Exercised

W31 closes the gap between path-only unit fixtures and the actual Git
filesystem behavior used by trusted workspace attachment. The acceptance test
creates, through the installed `git` executable:

- one primary repository and a nested cwd;
- one linked Git worktree with a different checkout root;
- one independent repository with the same basename;
- one ordinary clone of the primary repository;
- one checkout that is physically moved to a new directory.

The test then connects those real attachments to a fresh PostgreSQL authority,
not an in-memory resolver or model response.

## Accepted Trajectory

The accepted run proved:

```text
nested cwd
-> trusted Git probe resolves canonical primary root
-> exact namespaced primary binding is confirmed
-> current governed fact is stored

unadopted worktree / same-name repo / clone / alternate namespace
-> needs_confirmation
-> no delivery and no context

explicit worktree adopt
-> same continuity ID
-> current governed fact delivered
-> idempotent replay
-> reverse
-> worktree returns to needs_confirmation
-> primary remains resolved

physical checkout move
-> new root needs_confirmation
-> explicit rebind preserves continuity and current fact
-> replay is idempotent
-> reverse restores exact database binding state
-> a fresh rebind leaves the physically current root resolved
```

The primary and linked worktree produced different attachment fingerprints and
different canonical roots while retaining the same Git common-directory
fingerprint. The common fingerprint was used only as diagnostic evidence; it
did not auto-merge the worktree.

An identical primary anchor was separately confirmed for another tenant and
given a different marker. Each tenant received only its own current fact.

## Fresh Database Execution

The accepted local run used a dedicated PostgreSQL database and replayed every
migration from `00001_initial.sql` through
`00023_chunked_mean_retrieval_profile.sql` before executing W31. The temporary
database and Git topology were removed by the test harness.

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///fresh-database?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestW31' -v
```

## Preserved Failure

The first test run compared the resolver's canonical `/private/var/...` path
with macOS's symlinked `/var/...` temporary path and failed before reaching the
database gates. The resolver behavior was correct. The topology setup now
canonicalizes its temporary root before deriving expected paths, matching the
product's existing symlink policy rather than weakening the assertion.

## Claim Boundary

This evidence qualifies real local Git topology, conservative attachment,
explicit adopt/rebind governance, replay, reversal, current-memory delivery,
and tenant isolation for W05/W31. It does not claim:

- that a second real coding client executed this entire topology;
- automatic worktree, clone, mirror, fork, or device-migration adoption;
- remote filesystem inspection by the Mac mini;
- cryptographic proof that two checkouts represent the same repository intent;
- a real model call, model-quality result, or model ranking.

The existing Codex W05 evidence remains the real-client qualification. W31 is
the complementary production runtime qualification for actual Git topology;
neither evidence document silently substitutes for the other.

