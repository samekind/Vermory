# Provider-Assisted Unkeyed Source Target Matching Runtime Evidence

Date: 2026-07-14

Tested implementation revision: `270a0cc510de5be9a3ca9d6e221e6270383297b2`

## Scope

This evidence exercises one trusted source fact that has an exact source
revision and exact semantic content but no Vermory `memory_key`. A real Grok
provider receives only the current tenant and confirmed workspace's closed set
of active keyed facts, selects one listed key or abstains, and cannot activate
memory. Vermory persists the provider evidence, creates the existing governed
source candidate only for a valid unique match, and requires an explicit
operator acceptance before normal AI context changes.

This is provider-assisted closed-set target matching. It is not arbitrary
document extraction, open-vocabulary conflict discovery, source authority
ranking, or automatic activation of model output.

## Frozen Scenario

Case `108-workspace-unkeyed-source-target-match` and runtime case `W06` freeze a
software release-control workflow:

| Key or role | Fact |
|---|---|
| `release.signing.mode` | Production releases use a macOS keychain certificate. |
| `deploy.api.timeout` | The deployment API timeout is 800 ms. |
| `release.attestation.format` | Production releases publish a signed SLSA provenance statement. |
| New unkeyed source fact | Production releases now use GitHub Actions OIDC keyless signing. |
| Other-tenant distractor | Production releases use static cloud credentials. |

The committed fixtures have these SHA-256 values:

```text
source.md   88762385b372deb16fd2c8695f6eb3452be6b3bc5093ff7a521133f6905a89e7
claims.json 9163e754a1e0df14ca29e971455c80414abf3f97f67900449fe5b85f487378c5
tasks.json  a6a7dc4e91ae6ec2cdea67f676f1bb3bf2bf4e22e5ece99944cbc7237afcb594
case.json   f4f832690bcf54859083720c825f82cbc9a0219d1562593a58ef9e8baf0f87a8
```

## Runtime

```text
Vermory version: 0.1.0-dev
Vermory revision: 270a0cc510de5be9a3ca9d6e221e6270383297b2
Vermory build date: 2026-07-14T16:06:32+08:00
Vermory binary SHA-256: 88209d0f82f09e76679f822b122a259ca860e745dd6c1d9a214689c15d1293ad
Go runtime: go1.26.5
Grok CLI: 0.2.101 (5bc4b5dfadcf)
Grok model: grok-4.5
PostgreSQL: 18.4
Schema version: 12
Dedicated database: vermory_w06_final_20260714080714
Target tenant: w06-local
Distractor tenant: w06-other
```

The Grok runs used a fresh `HOME` with copied login material and one configured
MCP server. Cross-session memory, web search, plan mode, and subagents were
disabled. `grok mcp doctor` reported one healthy server, protocol `2025-06-18`,
and exactly two tools: `prepare_context` and `commit_observation`.

## Real Provider Decisions

Three real `grok-4.5` matching operations ran concurrently against the same
candidate snapshot:

| Operation | Result | Selected key | Provider artifact SHA-256 |
|---|---|---|---|
| `w06-real-match` | matched | `release.signing.mode` | `740d2d4730b2d1b2734099959b2f3d7f82728b17e3591b03f09e1e4e5462cdd4` |
| `w06-real-ambiguous` | abstained | none | `4f909377a0c9024d765a4b86ea195eeed80004c3e2edbe78843e9ead4dcccb71` |
| `w06-real-unrelated` | abstained | none | `6d8cc77dc447e24ee01aaa57334203a119773f48181f0189e88812be8760c0cd` |

All three audit rows have candidate-set fingerprint
`06d78bb7d1cc3f00fe3141eac7922cf46455baa2e68fe2dd0be44258385c50cb`.
The stored candidate JSON contains zero occurrences of the other tenant's
static credential fact.

The matched decision returned:

```text
source match: 28f29bfa-6734-432a-a8c1-a3dcdb59a579
selected key: release.signing.mode
target memory: 4666236f-ce22-4e03-997b-480ff901dbe9
observation: b6d67188-7faa-4ef3-a4a0-3b637ee8c683
candidate memory: 41d27191-2ff4-419b-9c2d-3887df21e1be
candidate status before review: proposed
```

The ambiguous source combined an identity-bound release flow with signed
release evidence, so Grok abstained rather than choosing either signing or
attestation. The unrelated maintenance-window source also abstained. Neither
operation created an observation or governed-memory candidate.

## Proposal Isolation And Acceptance

Before operator acceptance, real Grok session
`019f5fac-03c2-7833-b4bf-a807d6439167` called `prepare_context`. PostgreSQL
measured these positions in the persisted delivery:

```text
delivery ID: 23a1c459-9b8f-4099-a908-990cd6049af7
macOS keychain certificate: 146
800 ms: 48
signed SLSA provenance statement: 86
GitHub Actions OIDC keyless signing: 0
static cloud credentials: 0
```

The operator then accepted candidate
`41d27191-2ff4-419b-9c2d-3887df21e1be` with acceptance observation
`09667e00-a62f-46e3-9e5b-02b7b8d60296`. PostgreSQL atomically changed the
target to `superseded`, changed the candidate to `active`, preserved the timeout
and attestation facts as active, and left both abstained match decisions as
audit-only rows.

Projection rebuild retained four active documents across both tenants with
fingerprint `bdfe130b1df34408a324841e5e7c7555` before and after rebuild. Proposed
client write-backs and the superseded keychain fact were excluded.

## Real MCP Coder Task

Real Grok session `019f5fac-feb7-7cf0-a762-15aab01c705a` executed:

```text
prepare_context (w06-grok-final-prepare)
-> current OIDC signing + 800 ms timeout + SLSA attestation
-> create and deterministically verify release-control-policy.md
-> commit_observation (w06-grok-final-observation)
-> agent_result stored as proposed
```

The generated artifact is committed as a
[normalized snapshot](snapshots/2026-07-14-unkeyed-source-target-matching-grok-release-control-policy.md).

```text
delivery ID: 906273a4-3dae-4292-ba6e-91ed9855c9b3
observation ID: 4bb0f3b2-a928-45c8-be71-9572a50e3d7e
write-back memory ID: fcc46503-d727-4d5d-95d1-c12776339914
write-back lifecycle: proposed
artifact SHA-256: 37175009a79c26ce1519d4fd1ddc0b60646582782a26105450233e199b6757d9
transcript SHA-256: 6f9df82341b044ca2399ed144f939ec91194dbe170f3afe8aeffef0ee337afd9
```

Persisted delivery positions independently establish the consumed context:

```text
GitHub Actions OIDC keyless signing: 84
800 ms: 48
signed SLSA provenance statement: 151
macOS keychain certificate: 0
static cloud credentials: 0
```

## Stale Probes

Real Grok session `019f5fae-1a0e-7fb0-b72d-6a726f5e9815` called
`prepare_context` twice without write-back or file changes.

| Probe | Delivery | OIDC position | Old keychain position | Other-tenant position |
|---|---|---:|---:|---:|
| Exact stale statement | `a7ccfebf-714f-45c9-89be-978ff5c4a934` | 46 | 0 | 0 |
| Certificate-backed paraphrase | `be6875f4-9a40-4b67-a275-cfbc0d3525fe` | 46 | 0 | 0 |

The preserved stale transcript SHA-256 is
`25a087bdf2a7509586978468b155fb190c54d523cb068044cb4d89e789b88ea6`.

## Database And Isolation Evidence

The final `source_match_decisions` inventory contains one `matched` and two
`abstained` rows for `w06-local`. The table has RLS enabled and exactly one
tenant-isolation policy. Application tests also require the restricted runtime
role grant, tenant-aware foreign keys for continuity, target, observation, and
candidate links, and inclusion in the authoritative backup fingerprint.

Final target-tenant governed-memory counts were:

```text
active: 3
superseded: 1
proposed: 1
```

The proposed row is the real Grok task write-back recorded above. There are no
extra proposed rows from abandoned or duplicate coder runs in this final W06
database.

## Deterministic Hard Gates

| Gate | Result |
|---|---|
| Provider sees only same-tenant, same-workspace active keyed facts | PASS |
| Clear unkeyed source selects exactly one existing key | PASS |
| Ambiguous source abstains | PASS |
| Unrelated source abstains | PASS |
| Match alone leaves current AI context unchanged | PASS |
| Proposed candidate has no search projection | PASS |
| Explicit acceptance changes current fact | PASS |
| Projection rebuild excludes non-active states | PASS |
| Real artifact contains all three current facts | PASS |
| Real artifact excludes stale and cross-tenant facts | PASS |
| Real MCP write-back remains proposed | PASS |
| Exact and paraphrased stale probes return OIDC only | PASS |
| Match audit table is RLS protected and authoritative | PASS |

## Runtime Safety And Audit Hardening

The final W06 run used revision `270a0cc510de5be9a3ca9d6e221e6270383297b2`,
after the provider and audit lifecycle review fixes were applied. The same
revision includes deterministic regression coverage for:

- writing Grok source and candidate content to a mode-`0600` temporary prompt
  file, keeping that content out of process arguments, and deleting the file
  after the call;
- enforcing a two-minute provider deadline, killing the entire spawned process
  group on cancellation, and bounding process wait cleanup;
- persisting terminal provider timeout or cancellation failures after the
  caller context expires;
- expiring orphaned pending operations and rejecting replay when the active
  candidate snapshot changed;
- making `forget` win over an in-flight provider completion so a late result
  cannot restore deleted text;
- redacting forgotten source, candidate, provider-output, and reason text while
  preserving structural audit IDs and recomputing the canonical fingerprint;
- validating `source_match_decisions` privileges during restricted runtime-role
  startup;
- enforcing tenant-and-continuity foreign keys for target memory, observation,
  and candidate memory references, including migration handling for historical
  invalid rows.

These controls preserve the successful matched/abstained contract while keeping
provider output governance-only and deletion authoritative.

## Native Backup And Restore

The W06 source database, including all three real source-match decisions, was
dumped with PostgreSQL 18.4 `pg_dump --format=custom --no-owner --no-acl` and
restored into a fresh database with `pg_restore --exit-on-error`. The delivery
snapshot binary was built from commit `5f052d4d15a28d858ca67d8e793c94d72d828845`.
Migrate was replayed twice against both source and target; all four commands
returned schema 12 with no migrations to run.

```text
source database: vermory_w06_final_20260714080714
target database: vermory_w06_delivery_restore_20260714083232
source authority fingerprint: 3353564dcddeb2f526f6362ee5710671
target authority fingerprint: 3353564dcddeb2f526f6362ee5710671
dump SHA-256: ef00dca5c8c156fb228debd306f71cc293c8a8d4f5ac52225e888a6c6061c95d
delivery binary SHA-256: c2cfd1da34d95549b56a321a1c6199a87544ab0cfdd5ee5e6aeed05aa29c4d61
restored schema version: 12
restored source-match rows: 3
restored matched rows: 1
restored abstained rows: 2
restored RLS: enabled, one policy
restored projection documents: 4
restricted runtime role: `vermory_w06_delivery_runtime_20260714083232`
```

The restored projection was deleted and rebuilt through the current snapshot
binary. It returned four active documents and the same projection fingerprint
`bdfe130b1df34408a324841e5e7c7555`; the authority fingerprint remained
unchanged. A newly created restricted runtime role was provisioned with
`database grant-runtime`. Direct filter-omission probes against the restored
audit table returned `0` rows with no tenant setting, `3` for `w06-local`, and
`0` for `w06-other`.

## Local Release Gates

All local gates below were run after the evidence refresh on commit
`5f052d4d15a28d858ca67d8e793c94d72d828845`:

```text
go test -p 1 -count=1 ./...                                      PASS
go test -race -p 1 (8 runtime/client/provider packages)           PASS
go test -race -count=1 ./internal/reality                        PASS
go vet ./...                                                     PASS
go mod tidy with zero go.mod/go.sum diff                         PASS
actionlint v1.7.7, CI and Release workflows                      PASS
GoReleaser v2.17.0 configuration check                           PASS
GoReleaser snapshot, four target archives                        PASS
snapshot SHA-256 verification, four archives                     PASS
OpenClaw: 5 files, 43 tests, typecheck, build                     PASS
OpenClaw package dry-run                                          PASS
schema 12 migration replay on source and target                  PASS
native dump/restore authority, projection, role, and RLS checks    PASS
git diff --check                                                 PASS
W06 evidence credential-shaped scan                              0 matches
```

## Claim Boundary

This slice proves one closed-set provider-assisted target match, two real
abstentions, durable audit, explicit candidate acceptance, projection rebuild,
same-scope isolation, stale suppression, and one real Grok MCP consumption and
write-back path. It does not prove arbitrary-document claim extraction,
open-ended semantic conflict detection, provider-generated facts, multi-source
authority ranking, conversation formation, hybrid retrieval, formation quality
at scale, sealed benchmark performance, or final release readiness.
