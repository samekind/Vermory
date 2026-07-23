# Real Repository Corpus Design

Status: frozen for W37 execution; corpus entries remain append-only

Date: 2026-07-23

## Purpose

W31, W33, and W34 qualify difficult Git topology and cross-host trajectories,
but their repositories are controlled fixtures or Vermory itself. W37 checks
that the trusted workspace probe and exact continuity boundary work on a small,
heterogeneous set of real public repositories before claiming broader client
compatibility.

The case changes validation breadth, not workspace identity semantics. Git is
used to locate a checkout root. Repository names, remotes, shared commits, and
ecosystem similarity still do not authorize automatic continuity merging.

## Reality-First Input

The initial corpus contains six public repositories at exact commits:

- `golang/example`, a Go multi-example repository;
- `rust-lang/mdBook`, a Rust workspace;
- `django/django`, a Python framework repository;
- `pnpm/pnpm`, a Node.js monorepo;
- `mem0ai/mem0`, a memory-platform monorepo;
- `openclaw/openclaw`, an agent-platform monorepo.

Each checkout is shallow and may use sparse checkout. Source objects, build
outputs, databases, and raw evidence remain outside the repository. The public
manifest stores only authorized URLs, exact commits, relative corpus paths,
expected markers, and the acceptance contract.

Future corpus expansion appends exact sources and reruns the same invariants.
No ecosystem in this first batch becomes a hard-coded product category.

## Runtime Contract

For every repository:

```text
exact public checkout
-> probe repository root
-> probe one real nested directory
-> verify one canonical root and Git common-directory identity
-> encode and decode the bounded attachment
-> confirm the exact namespaced anchor
-> add one governed source marker
-> prepare current context
-> replay the exact prepare
```

All repository continuities use the same tenant and filesystem namespace so
cross-repository isolation is tested directly. One anchor is also confirmed
under another tenant with a different marker. An alternate unconfirmed
filesystem namespace must abstain without a delivery.

## Hard Boundary

W37 passes only when all eighteen gates in
`runtime/cases/W37-real-repository-corpus/case.json` pass in one fresh run.
The deterministic case test always runs. The public checkout trajectory is
opt-in through `VERMORY_W37_CORPUS_ROOT`, and the database part additionally
requires an isolated `VERMORY_TEST_DATABASE_URL`.

The accepted evidence records exact source revisions and normalized results.
It does not commit clone data, raw database state, personal absolute paths, or
credentials. A network or checkout preparation failure remains distinct from
a resolver or isolation failure.

## Non-Claims

W37 qualifies only the six exact repositories and Git version that were run.
It does not prove every repository, filesystem, Git implementation, worktree,
submodule, coding client, or network condition. W31 remains the local topology
qualification, W33 remains the mirror/fork and namespace-migration
qualification, and W34 remains the two-physical-host real-client qualification.

Most importantly, success does not make remote URL or shared history an
automatic binding signal. Unknown exact anchors continue to require explicit
confirmation, adopt, or rebind.
