# Governed Source Conflict Candidate Runtime Evidence

Date: 2026-07-14

Tested implementation revision: `2f946a99f4e329ce69556551825e4739c269d440`

## Scope

This evidence exercises one deterministic source-conflict candidate through the
release binary, operator CLI, PostgreSQL authority store, rebuildable search
projection, and a real logged-in Grok coding client over MCP. A trusted source
ingestor supplies a stable fact key and an exact source revision. Vermory
proposes the changed fact without changing current AI context, preserves an
operator rejection, accepts a second proposal atomically, and exposes only the
accepted fact to the client.

This is keyed candidate formation. It is not arbitrary-document extraction or
general semantic conflict detection.

## Frozen Scenario

Case `107-workspace-source-conflict-candidate` and runtime case `W05` freeze a
software release-control workflow:

| Role | Fact |
|---|---|
| Initial current fact | Production releases use a macOS keychain certificate. |
| Proposed replacement | Production releases use GitHub Actions OIDC keyless signing. |
| Independent fact | The deployment API timeout is 800 ms. |
| Other-tenant distractor | Production releases use static cloud credentials. |

The committed fixtures have these SHA-256 values:

```text
source.md   78ba05598d785cf2573ba7dcc3ee701b57c2c1804c282275a5247408eaff3b3c
claims.json d3bef8dedeaf5ca3930d4d63e017940dc33ed83f600050a40e824975c7f3b5a2
tasks.json  64d10ace126200ab2d172696c3e91e83edb2b863decb98b4f55ca4186e186c02
case.json   6241e07f07336624147ee18b831f69acd1dbb36aca35670a0731c1c8e8a17e3e
```

## Runtime

```text
Vermory version: 0.1.0-dev
Vermory revision: 2f946a99f4e329ce69556551825e4739c269d440
Vermory build date: 2026-07-14T13:59:55+08:00
Vermory binary SHA-256: 27bdb45e25f120e8daa8861296d9273a82dfd9a3254fa39267c7b064a670aa04
Go runtime: go1.26.5
Grok CLI: 0.2.99 (b1b49ccb71a7)
Grok model: grok-4.5
PostgreSQL: 18.4
Schema version: 10
Dedicated database: vermory_source_candidate_20260714140157
Target tenant: w05-local
Distractor tenant: w05-other
```

The Grok replay used a fresh `HOME` containing only the current login material
and one user-scoped MCP server. Cross-session memory, web search, plan mode, and
subagents were disabled. `grok mcp doctor` reported one healthy server, protocol
`2025-06-18`, and exactly two tools.

## Candidate Lifecycle

The release binary executed this sequence against a confirmed disposable
workspace:

```text
add initial keyed signing fact + independent timeout
-> propose replacement candidate
-> reject candidate
-> propose the same source revision under a new operation
-> accept candidate
-> rebuild all search projections
```

The first proposal returned:

```text
disposition: replacement
target memory: 2a4c60eb-d523-4728-b33e-5377e61c10ac
candidate memory: 32665318-262f-43c3-af7c-5673f5de3130
candidate status: proposed
```

Before the proposal, both initial facts were active and projected. After the
proposal, the initial signing fact remained active and projected while the new
candidate had zero projection rows. Cross-tenant acceptance failed with
`candidate does not belong to this workspace continuity`. Rejection changed
only the candidate to `rejected`; the current signing fact and timeout remained
active.

The second proposal created candidate
`4a102d85-b413-4562-9b80-6020fe404291`. Acceptance changed the initial signing
fact to `superseded`, activated the second candidate, preserved the rejected
candidate as audit history, and left the timeout active. Projection rebuild
kept three documents across both tenants with fingerprint
`9af1195c74cf0eda11ad12befd99706a` before and after rebuild.

## Real MCP Replay

Grok session `019f5f3c-d836-7d80-a8de-995dcde29ef8` executed:

```text
prepare_context (w05-grok-prepare-current-1)
-> accepted OIDC signing fact + 800 ms timeout
-> create and deterministically verify release-signing-check.md
-> commit_observation (w05-grok-observation-current-1)
-> agent_result stored as proposed
```

The generated artifact is committed as a
[normalized snapshot](snapshots/2026-07-14-source-conflict-candidate-grok-release-signing-check.md).

```text
delivery ID: 337963a0-8c60-4ba9-853a-18c601c016e2
observation ID: d021463b-eca8-4b54-96cb-eebeffc129d2
artifact SHA-256: bdc0c61328a06dde87390e565b2c0ecd848bb78fef169026971b506bee463eab
transcript SHA-256: d8208eb51bc5dd275acb7a0d98105696c89b9401e3636b3f81e3a3f885b0ec8a
```

PostgreSQL measured positions in the persisted delivery body:

```text
GitHub Actions OIDC keyless signing: 80
800 ms: 48
macOS keychain certificate: 0
static cloud credentials: 0
```

The write-back row independently reports observation kind `agent_result`,
lifecycle `proposed`, and source reference
`agent:grok-4.5:source-conflict-candidate`.

## Stale Probes

After projection rebuild, Grok session
`019f5f3e-4b7f-7510-8cda-a26e0ba89725` called `prepare_context` for both the
exact stale statement and a paraphrased certificate-based signing question.
The two persisted deliveries returned the accepted OIDC fact.

| Probe | OIDC position | Timeout position | Old keychain position | Other-tenant position |
|---|---:|---:|---:|---:|
| Exact stale statement | 42 | 0 | 0 | 0 |
| Paraphrased stale question | 42 | 0 | 0 | 0 |

No observation was committed by either stale probe. The preserved stale-probe
transcript SHA-256 is
`ce5fc8c18dc9db9dcb4342d438b4e8fa1effe8d2df61ba2ec0aa98a524ced133`.

## Deterministic Hard Gates

| Gate | Result |
|---|---|
| Proposal leaves current context unchanged | PASS |
| Proposed candidate has no search projection | PASS |
| Reject preserves the active target and independent fact | PASS |
| Cross-tenant acceptance | Rejected |
| Accept atomically supersedes the keyed target | PASS |
| Rejected candidate remains audit history | PASS |
| Projection rebuild excludes proposed, rejected, and superseded rows | PASS |
| Real artifact includes OIDC and 800 ms | PASS |
| Real artifact excludes old and other-tenant facts | PASS |
| Real MCP write-back remains proposed | PASS |
| Exact and paraphrased stale deliveries exclude old and other-tenant facts | PASS |

Final target-tenant lifecycle counts were two `active`, one `superseded`, one
`rejected`, and one `proposed` memory. The `proposed` row is the real client's
post-task observation, not a source candidate.

## Preserved Preflight Corrections

1. A first local binary was discarded before database use because its injected
   build timestamp was rounded rather than copied exactly from the commit.
2. `grok models` printed an unauthenticated status while listing `grok-4.5`;
   an actual isolated single-turn request returned `AUTH_OK`, and both real MCP
   sessions completed successfully.
3. The first ledger verification query referenced `context_text`, which is not
   a schema column. The corrected read-only query used the authoritative
   `memory_deliveries.context_body` column.
4. The first OpenClaw check command single-quoted the entire `PATH` assignment,
   so `$PATH` did not expand and `pnpm` was not found. The corrected command
   used the repository's documented double-quoted assignment and passed.
5. The first snapshot checksum command ran from the repository root even though
   `checksums.txt` contains archive basenames. Running the same verification
   from `dist/` returned `OK` for all four archives.

## Local Release Gates

The final local verification completed on revision `10f4df8` after the runtime
evidence and documentation commit:

```text
go test -p 1 -count=1 ./...                                      PASS
go test -race -p 1 (8 runtime/client packages)                   PASS
go test -race -count=1 ./internal/reality                        PASS
go vet ./...                                                     PASS
go mod tidy with zero go.mod/go.sum diff                         PASS
actionlint v1.7.7, CI and Release workflows                      PASS
GoReleaser v2.17.0 configuration check                           PASS
GoReleaser snapshot, four target archives                        PASS
snapshot SHA-256 verification, four archives                     PASS
OpenClaw: 5 files, 43 tests, typecheck, build                     PASS
OpenClaw package dry-run                                         PASS
git diff --check                                                 PASS
W05 artifact credential-shaped scan                              0 matches
```

## Remote Pull Request Gate

GitHub Actions run
[`29311201470`](https://github.com/jstar0/Vermory/actions/runs/29311201470)
tested branch head `c6b50cd849527a297b046c5e3317d6cdc966711b`. Job
`87015135131` passed all 19 main steps in `3m28s`, including PostgreSQL tests,
runtime and reality race tests, module drift, OpenClaw, release snapshot,
artifact upload, and clean diff.

The run uploaded artifact `8302182898`:

```text
name: vermory-pr-snapshot-74d961d9473656187fe115abf5721cbdd9a7b572
bytes: 20297161
digest: sha256:a220aa810411b47cb48327aa43cb2d787f34d588be25df08c7b4802c588e462d
expires: 2026-07-21T06:26:44Z
```

The artifact was downloaded and independently checked. All four archive
entries in `checksums.txt` returned `OK`; every Go archive contains `vermory`,
`LICENSE`, `README.md`, and `README.zh-CN.md`; the OpenClaw tarball contains the
compiled JavaScript, declarations, plugin manifest, and package metadata. The
darwin/arm64 binary executed on the host and reported snapshot revision
`74d961d9473656187fe115abf5721cbdd9a7b572`.

Draft PR 1 remained `CLEAN` and `MERGEABLE`, with required check `test` at
`SUCCESS`. The repository still had zero tags and zero GitHub Releases. The
only workflow annotation was GitHub's Node.js 20 action-runtime deprecation
notice; the workflow forced those actions to run on Node.js 24 and passed.

## Claim Boundary

This slice proves deterministic keyed proposal, rejection, acceptance,
projection rebuild, cross-tenant rejection, stale suppression, and one real
Grok MCP consumption/write-back loop. It does not prove key extraction from
arbitrary documents, provider-assisted unkeyed matching, conversation
formation, multi-source authority ranking, formation quality at scale, every
supported model/client, sealed benchmark performance, or final release
readiness.
