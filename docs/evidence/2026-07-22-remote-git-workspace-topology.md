# Remote Git Workspace Topology Qualification

Date: 2026-07-22

Reality case: `W05-trusted-workspace-attachment`

Runtime case: `W33-remote-git-workspace-topology`

Status: `24 / 24 PASS`

## Boundary Exercised

W33 extends the real Git qualification beyond the local checkout shapes covered
by W31. The acceptance test uses the installed `git` executable to create:

- one bare upstream repository and a normal primary checkout;
- one real `--mirror` bare repository and a checkout cloned from it;
- one independent bare fork endpoint and a checkout cloned from it;
- one migrated checkout probed under a different filesystem namespace;
- a primary checkout whose `origin` is changed after two additional remotes are
  added.

Every checkout starts at the same commit. This deliberately removes commit
history, basename, and remote similarity as safe automatic binding signals.
The resulting attachments are then exercised against a fresh PostgreSQL
authority, not an in-memory substitute or a model response.

## Accepted Trajectory

The accepted run proved:

```text
bare mirror
-> rejected as a workspace attachment

primary / mirror checkout / fork checkout / migrated checkout
-> same HEAD commit
-> distinct canonical roots and Git common-directory fingerprints
-> no automatic continuity merge

primary remote additions and origin replacement
-> attachment identity remains unchanged
-> confirmed primary still receives its current governed fact

unadopted mirror / fork / migrated namespace
-> needs_confirmation
-> no delivery and no context

explicit mirror and fork adoption
-> primary continuity preserved
-> current governed fact delivered
-> replay is idempotent
-> reversal removes the adopted binding without changing the primary

explicit device-namespace rebind
-> primary continuity and current governed fact preserved
-> old binding removed
-> another tenant at the same target anchor remains isolated
-> replay is idempotent
-> reversal restores the exact binding state
-> a fresh final rebind serves only the migrated target for the target tenant
```

The target tenant and a second tenant were both bound to the migrated anchor.
Each received only its own current marker before and after the target tenant's
rebind. No cross-tenant context appeared.

## Fresh Database Execution

The accepted local run used the dedicated external-volume PostgreSQL 17 test
cluster, created a unique disposable database, and replayed every migration from
`00001_initial.sql` through `00023_chunked_mean_retrieval_profile.sql`. The test
database was dropped and PostgreSQL was stopped after the run. Go build, module,
temporary, Git-fixture, and database data all remained on the external volume.

Reproduction shape:

```bash
TMPDIR=/path/on/external-volume \
GOTMPDIR=/path/on/external-volume \
GOCACHE=/path/on/external-volume \
GOMODCACHE=/path/on/external-volume \
VERMORY_TEST_DATABASE_URL='postgresql:///disposable?host=/path/to/socket' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestW33' -v
```

## Preserved Test-First Failure

The first contract run failed because
`runtime/cases/W33-remote-git-workspace-topology/case.json` did not yet exist.
Only after that expected failure was retained was the 24-gate manifest frozen
and the complete database-backed trajectory executed. No product assertion was
weakened to obtain the accepted result.

## Claim Boundary

This evidence qualifies conservative handling of a real bare mirror, mirror and
fork checkouts, shared commit history, multiple remotes, explicit adoption,
cross-namespace rebind, replay, reversal, current-memory delivery, and tenant
isolation for W05/W33. It does not claim:

- that Git remote URLs or shared object graphs prove repository intent;
- automatic mirror, fork, clone, or device-migration adoption;
- a network-hosted Git provider or remote filesystem inspection;
- a second physical device or cross-host data transfer;
- that another coding client executed this topology;
- a real model call, model-quality result, or model ranking.

The Codex W05 evidence remains the real-client qualification. W31 qualifies
local Git topology, and W33 qualifies remote-shaped Git topology plus an
explicit device-namespace transition. These are complementary boundaries and
do not substitute for a second-client or physical cross-device run.
