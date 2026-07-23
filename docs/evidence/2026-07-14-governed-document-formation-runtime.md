# Governed Multi-Fact Document Formation Runtime Evidence

Date: 2026-07-14

Tested implementation revision: `06f54fd6b52e3fa902bb1ddbb980cc16607ff73a`

## Scope

This evidence exercises one bounded trusted workspace document containing an
unchanged fact, an update, a new fact, an instruction-injection sentence, and
an uncertain sentence. A real Grok provider receives the document and only the
current same-tenant, same-workspace active keyed facts. Vermory verifies exact
source spans, classifies every item against the frozen active snapshot, stores
one PostgreSQL-authoritative batch, and creates only proposed candidates until
an operator explicitly accepts them.

The proof includes pre-review context isolation, real provider abstention,
explicit review, projection rebuild, a real Grok MCP coder task, stale probes,
RLS, cross-tenant isolation, and real-provider forget redaction. It does not
claim arbitrary repository crawling, automatic source trust, stable ontology
discovery, automatic candidate acceptance, or formation quality at scale.

## Frozen Scenario

Case `109-workspace-multifact-document-formation` and runtime case `W07` freeze
these current facts:

| Key | Current fact |
|---|---|
| `deploy.region.primary` | Production deploys to us-east-1. |
| `deploy.retry.max` | Production deployments retry at most 3 times. |
| `release.attestation.format` | Production releases publish a signed SLSA provenance statement. |

The trusted 309-byte revision states that the region remains `us-east-1`, the
retry ceiling is now 5, and rollback approval requires two maintainers. It also
contains one governance-bypass/static-credential instruction and one undecided
fallback-policy sentence. Another tenant contains a static-credential fact.

Committed fixture SHA-256 values:

```text
source.md   623a16d78cfe5afcd6737ddb452cd96a31bc50e560643f24a204c2522e7ec1ac
claims.json 0ee73289580cb2185fa62581d075e81f8d37eba255ec8e132707159d16905862
tasks.json  d11b52cc0a2031d47def2caac4789b1f5f52bd4c5a08cb960f425e40bad6bacd
case.json   06c68319c958243399f2fdbc99f53149a29b88b3cbdd02c0e2cfb4b01deb62a9
```

## Runtime

```text
Vermory version: 0.1.0-dev
Vermory revision: 06f54fd6b52e3fa902bb1ddbb980cc16607ff73a
Vermory build date: 2026-07-14T18:03:53+08:00
Vermory binary SHA-256: 1ccd0929450ce77b37fc8e1646c151beee06e4b0887709cb1d009276f3cc802d
Go runtime: go1.26.5
Grok CLI: 0.2.101 (5bc4b5dfadcf)
Grok model: grok-4.5
PostgreSQL: 18.4
Schema version: 13
Dedicated database: vermory_w07_20260714175236
Target tenant: w07-local
Distractor tenant: w07-other
Forget-probe tenant: w07-forget
```

The real client used an isolated synthetic Git workspace and an isolated Grok
home containing copied login material and exactly one MCP server. Web search,
cross-session memory, plan mode, and subagents were disabled. `grok mcp doctor`
reported one healthy stdio server, protocol `2025-06-18`, and exactly two tools:
`prepare_context` and `commit_observation`.

## Preserved Provider Iterations

The evidence retains provider and contract failures rather than replacing them
with a scripted pass.

1. Initial primary and abstention runs returned Markdown-fenced JSON. The
   strict parser persisted both as `failed/invalid_provider_output`. The initial
   source path also pointed at the casebook explanation page rather than the
   309-byte trusted revision; that setup error is preserved.
2. Vermory added the Grok CLI's official `--json-schema` structured-output
   option and reran against the correct source. The abstention passed, while the
   primary run was persisted as `failed/unchanged_content_changed` because the
   provider restated the source quote instead of copying the current governed
   content for the unchanged item.
3. The provider contract was clarified without relaxing the store. The final
   run completed with three exact-span items.

Final target-tenant run inventory:

```text
abstained: 1
completed: 1
failed:    3
items:     3
```

## Real Formation Result

Final run:

```text
operation: w07-real-formation-v3
run ID: 385eb946-c9ef-498b-b87d-522bf1b1c4b2
source SHA-256: efcb2033c5956a3475e22c270b83fde26f761c2057f920964fd6ea1795a87a3f
active snapshot SHA-256: 5acd38ea6667c326fe3d048639fc8e0e2662ba56d4e1b3fba9dbf73f598fcdc6
provider artifact SHA-256: 1d0158c27f29b40960f5f72c167438e6dc5c387558f224624233106b53c95108
```

| Ordinal | Decision | Key | Byte span | Target | Candidate |
|---:|---|---|---:|---|---|
| 1 | `unchanged` | `deploy.region.primary` | `34..78` | `5d71a06e-21f7-4066-803e-6a8110cc06bb` | none |
| 2 | `update` | `deploy.retry.max` | `79..128` | `ca129068-ded5-4f13-a34b-3dff7b90db67` | `44fbd051-2f26-45d1-8031-84c81b65d371` |
| 3 | `new` | `deploy.rollback.maintainers` | `129..172` | none | `ffae3ca1-fc9c-442b-a681-5c5e722e14e1` |

Every persisted quote matches the selected source bytes. The provider suggested
`deploy.rollback.maintainers` for the new fact. The frozen deterministic fixture
uses `deploy.rollback.approvals`; both name the same governed statement, but this
run does not claim stable provider-generated ontology naming. The operator
reviewed the actual suggested key and accepted the semantic candidate.

The final run's active snapshot, provider output, and item fields contain zero
occurrences of the other tenant's static-credential fact. Persisted items also
contain zero occurrences of the governance-bypass sentence and fallback-policy
sentence.

The separate real provider operation `w07-real-abstention-v2` returned
`abstained` with zero items for an injection-only and undecided-policy document.
Its provider artifact SHA-256 is
`96a6b065164396141e36077a737827adb8afa5e061ea4143d6ddfbb64fa67d8d`.

## Pre-Review Isolation

Before candidate acceptance, real Grok session
`019f6017-840c-79f2-b44c-0652c9b24fdc` called `prepare_context` with operation
`w07-pre-review-prepare`. Persisted delivery
`b47221b4-9005-4e7e-bb8e-260f55ee8456` measured:

```text
retry at most 3 times: 41
retry at most 5 times: 0
Rollback approval requires two maintainers: 0
static cloud credentials: 0
fallback policy: 0
```

The delivery also contained the current region and signed SLSA attestation.
Formation alone therefore did not change current AI context.

## Explicit Acceptance And Projection

The operator accepted both proposed candidates:

```text
retry candidate: 44fbd051-2f26-45d1-8031-84c81b65d371
retry acceptance observation: c78fb045-9db2-4c2d-bbd4-078c276b4689
rollback candidate: ffae3ca1-fc9c-442b-a681-5c5e722e14e1
rollback acceptance observation: 210b05e1-3325-4222-88f5-bbb7069d2062
```

Post-review formation inspection reported both candidate lifecycles as
`active`; the old retry memory became `superseded`. The target tenant's search
projection contained four documents before and after rebuild with identical
fingerprint `accc818c9dbb062d234fd380048cdd77`. The rebuild reported five total
documents because the separate distractor tenant retained its own active fact.

## Real MCP Coder Task

Real Grok session `019f6019-63fc-78e2-8eb6-41ddfabe62d8` executed:

```text
prepare_context (w07-grok-final-prepare)
-> current region + retry 5 + rollback two maintainers + signed SLSA
-> create and deterministically verify deployment-control-policy.md
-> commit_observation (w07-grok-final-observation)
-> agent_result retained as proposed
```

The generated artifact is committed as a
[normalized snapshot](snapshots/2026-07-14-governed-document-formation-grok-deployment-control-policy.md).

```text
delivery ID: 69f7178b-d8ca-4f62-92b3-505fb6ec7031
observation ID: ab05c377-4edc-478e-8acb-c9b16f6fa3de
write-back memory ID: f176ac8a-1321-4017-80ec-726cba1173a7
write-back lifecycle: proposed
artifact SHA-256: 7ce3f035e0984ec1915ee8bd2881cf5dea61aa3b9f56c904e6180080eb5aeaa2
client JSON SHA-256: 35fd7bb1cec7c0bc02f9daaeaa6ac6af548b244ea2d0e29ec2bb3d978dee8ab0
transcript SHA-256: 5d1c08b2e0a624531ef69698c857a684bc8f453a35d42fc1790f1e8ad143781f
```

Deterministic file checks found all four required phrases and none of the four
forbidden phrases. The persisted delivery independently measured retry 5 at
position 85, rollback approval at position 18, and retry 3, static credentials,
and fallback policy at position 0.

## Stale Probes

Real Grok session `019f601a-fdfe-7bb0-96ac-376ba8b99515` called
`prepare_context` twice without file changes or write-back.

| Probe | Delivery | Retry 5 | Retry 3 | Rollback | Other tenant | Fallback |
|---|---|---:|---:|---:|---:|---:|
| Exact stale statement | `977537f5-9e40-402a-9a1b-20e3a6fef056` | 41 | 0 | 64 | 0 | 0 |
| Paraphrased stale setup | `11d68a3f-c5dc-48b5-ae54-e793e65a3615` | 41 | 0 | 64 | 0 | 0 |

The transcript SHA-256 is
`dceffcaa3fbdbaae405f87c608b531b8aee0ab5a6691a612d727a022d10f0e81`.

## RLS And Isolation

Both `source_formation_runs` and `source_formation_items` have RLS enabled and
exactly one tenant-isolation policy. Restricted runtime role
`vermory_w07_runtime_20260714175236` was granted the served-table boundary.
Direct filter-omission probes through that role returned:

```text
tenant setting absent: runs 0, items 0
w07-local:            runs 5, items 3
w07-other:            runs 0, items 0
```

The role cannot bypass RLS, own served tables, or read legacy authority tables;
those boundaries are also enforced by startup validation and automated tests.

## Real Provider Forget Redaction

An independent `w07-forget` tenant used a real Grok formation update from
`FORGET-OLD-KEYCHAIN` to `FORGET-NEW-OIDC`. The operator accepted the proposed
candidate and then forgot both the candidate and its target.

Inspection preserved structural IDs, hashes, decision, key, and links while
returning:

```text
source_ref: [redacted]
provider_output: [redacted]
run reason: [redacted]
item quote/content/reason: [redacted]
candidate lifecycle: deleted
target lifecycle: deleted
```

Database residue checks across run source ref, active snapshot, provider output,
run reason, and item quote/content/reason returned position `0` for both marker
strings.

## Deterministic Hard Gates

| Gate | Result |
|---|---|
| Provider input is same-tenant and same-workspace only | PASS |
| Source document is bounded and stored only by ref/hash/size plus selected spans | PASS |
| Exact unchanged, update, and new spans persist atomically | PASS |
| Invalid provider output and invalid unchanged normalization fail durably | PASS |
| Injection-only and undecided-policy source abstains with zero items | PASS |
| Formation alone leaves current context unchanged | PASS |
| Proposed candidates have no search projection | PASS |
| Explicit acceptance changes only reviewed facts | PASS |
| Projection rebuild preserves active target-tenant documents | PASS |
| Real artifact contains region, retry 5, rollback requirement, and SLSA | PASS |
| Real artifact excludes retry 3, injection, other tenant, and fallback policy | PASS |
| Real MCP write-back remains proposed | PASS |
| Exact and paraphrased stale probes return current facts only | PASS |
| Formation audit tables are RLS protected | PASS |
| Forget redacts linked formation evidence | PASS |
| Ordinary MCP still exposes exactly two normal-flow tools | PASS |

## Native Backup And Restore

The dedicated W07 source database was dumped with PostgreSQL 18.4 using custom
format, `--no-owner`, and `--no-acl`, then restored into a fresh database with
`pg_restore --exit-on-error`. Migration replay ran twice against both databases;
all four commands reported schema 13 with no migrations left to apply.

```text
source database: vermory_w07_20260714175236
restore database: vermory_w07_restore_20260714175236
source authority fingerprint: 57a7cb60c58a48734de49b2b685e92a7
restore authority fingerprint: 57a7cb60c58a48734de49b2b685e92a7
dump SHA-256: 91571cfc7218b234fe94fdee39fa98b8ab6f97d2db074b9a9803f2f344fb79bc
restored schema version: 13
restored formation runs: 6
restored formation items: 4
restored target projection: 4 documents, accc818c9dbb062d234fd380048cdd77
restricted restore role: vermory_w07_restore_runtime_20260714175236
```

The authority fingerprint includes formation runs and items in addition to the
served continuity graph and auth metadata. Deleting and rebuilding the restored
search projection preserved both its four-document target-tenant fingerprint
and the authority fingerprint. Restored formation RLS had one policy per table;
filter-omission probes returned `0/0`, target-tenant `5/3`, then other-tenant
`0/0` for runs/items.

## Local Release Gates

Fresh local gates ran from documentation revision
`988ef6069cc41a51d97d5bbb56303e6d77aab877` after the runtime evidence was
recorded:

```text
go test -p 1 -count=1 ./...                                      PASS
go test -race -p 1 across 8 runtime/client/provider packages      PASS
go test -race -count=1 ./internal/reality                         PASS
go vet ./...                                                      PASS
go mod tidy with zero go.mod/go.sum diff                          PASS
actionlint v1.7.7, CI and Release workflows                       PASS
GoReleaser v2.17.0 configuration check                            PASS
GoReleaser snapshot, four target archives                         PASS
snapshot SHA-256 verification, four archives                      PASS
host darwin/arm64 archive execution                               PASS
OpenClaw: 5 files, 43 tests, typecheck, build                      PASS
OpenClaw package dry-run                                           PASS
schema 13 migration replay on source and restore                  PASS
native dump/restore authority, projection, role, and RLS checks    PASS
git diff --check                                                   PASS
W07 evidence credential-shaped scan                               0 matches
```

The snapshot archives cover `darwin/amd64`, `darwin/arm64`,
`linux/amd64`, and `linux/arm64`. Every archive independently contained exactly
`vermory`, `LICENSE`, `README.md`, and `README.zh-CN.md`; all checksums matched.
The host archive reported version `0.0.0-SNAPSHOT-988ef60` and revision
`988ef6069cc41a51d97d5bbb56303e6d77aab877`.

## Remote CI Delivery

Implementation and evidence head `5b962a95ecb65f79b0a34c1396b037d4dec23d4a`
passed protected pull-request CI run
[`29325444582`](https://github.com/jstar0/Vermory/actions/runs/29325444582).
Job `87060532691` completed all 19 main steps in `3m51s`, including the
PostgreSQL-backed suite, runtime and reality race gates, vet, module verification,
release build, OpenClaw check/package, snapshot build/upload, and clean-diff
verification.

```text
artifact ID: 8307774917
artifact name: vermory-pr-snapshot-6fe81bac9f42313c9e07cf47ed8deea5ec93b133
artifact size: 20,540,870 bytes
GitHub digest: sha256:d1c8380fdf888b6372877ba96ada10110712bd227d0f9b4d67977fd4473ed8d4
downloaded ZIP SHA-256: d1c8380fdf888b6372877ba96ada10110712bd227d0f9b4d67977fd4473ed8d4
```

The independently downloaded artifact contained the OpenClaw `0.1.0` package
and four Go archives. Its `checksums.txt` returned `OK` for darwin/amd64,
darwin/arm64, linux/amd64, and linux/arm64. Every archive again contained the
same exact four-file layout. The downloaded darwin/arm64 binary executed on the
host and reported version `0.0.0-SNAPSHOT-6fe81ba`, merge revision
`6fe81bac9f42313c9e07cf47ed8deea5ec93b133`, build date
`2026-07-14T10:28:00Z`, and Go `1.25.7`.

After that run, Draft PR 1 reported `CLEAN`, `MERGEABLE`, and required
`test=SUCCESS`. The workflow emitted a Node 20 deprecation annotation for pinned
GitHub actions being forced onto Node 24; it did not fail or skip a gate.

## Cleanup

After evidence capture, both dedicated databases, both temporary runtime roles,
the custom dump, the isolated Grok home, and its copied login material were
removed and verified absent. The ignored non-secret runtime JSON and transcripts
remain local. The shared `vermory_test` database and unrelated user MCP
configuration were not removed or modified.

## Claim Boundary

This slice proves bounded trusted-document formation, exact-span validation,
atomic proposal creation, explicit review, real Grok compatibility, real MCP
consumption and write-back, stale suppression, RLS isolation, and deletion
redaction for the exercised scenario. It does not prove arbitrary-document
formation quality, deterministic provider-generated key naming, multi-source
authority ranking, PDF/OCR ingestion, conversation or Global Defaults
formation, hybrid retrieval, scale/fault qualification, sealed benchmark
performance, signing, or final release readiness.
