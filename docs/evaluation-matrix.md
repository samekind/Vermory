# Legacy ContextMesh Evaluation Matrix

> Historical self-case and provider evidence retained for reproducibility. It
> is not the authority for current Vermory capability claims. Use the
> [Vermory Capability And Evidence Matrix](capability-evidence-matrix.md) for
> frozen-case execution state, real-client coverage, blocked qualifications,
> protected delivery, and explicit non-claims.

## Purpose

This document records the model and client consumption matrix for the legacy direct-provider harness. Unlike one-off smoke runs, it exercises the same packet contract across multiple compatible model targets and client harnesses.

## Self-Case Task Set

- `self-case-stale-context`
- `self-case-domestic-scope`
- `self-case-architecture-core`
- `self-case-real-case-policy`
- `self-case-artifact-evidence`

## Matrix Command

### Mock matrix

```bash
go run ./cmd/vermory eval-matrix \
  --provider mock \
  --models mock-a,mock-b \
  --run-id matrix-mock-full \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 64
```

### Duojie core matrix

```bash
DUOJIE_API_KEY='***' \
go run ./cmd/vermory eval-matrix \
  --provider duojie \
  --models gemini-3-flash,gemini-3.1-pro,glm-5 \
  --run-id duojie-matrix-core \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 256
```

## Artifact Layout

- `matrix-runs/<run-id>/report.md`
- `matrix-runs/<run-id>/report.json`
- `matrix-runs/<run-id>__<model>__<task-id>/...`

Each task directory contains a full four-baseline evaluation report with:

- `input.md`
- `packet.md` for packet baseline
- `output.md`
- `raw.json`
- `score.json`
- `report.md`

## Internal Ready Casebook Artifacts

The Internal Ready slice adds executable casebook runs for the three core continuity tracks.

Workspace casebook runs write:

- `casebook-runs/<run-id>/workspace/report.json`
- `casebook-runs/<run-id>/workspace/report.md`
- `casebook-runs/<run-id>/workspace/platform-report.md`
- `casebook-runs/<run-id>/workspace/<baseline>/input.md`
- `casebook-runs/<run-id>/workspace/<baseline>/output.md`
- `casebook-runs/<run-id>/workspace/<baseline>/score.json`
- `casebook-runs/<run-id>/workspace/contextmesh_packet/packet.md`

Conversation casebook runs write:

- `casebook-runs/<run-id>/conversation/report.json`
- `casebook-runs/<run-id>/conversation/report.md`
- `casebook-runs/<run-id>/conversation/conversation/<thread-id>/input.md`
- `casebook-runs/<run-id>/conversation/conversation/<thread-id>/output.md`
- `casebook-runs/<run-id>/conversation/conversation/<thread-id>/score.json`
- `casebook-runs/<run-id>/conversation/conversation/<thread-id>/raw.json` when the provider returns raw response evidence

Bridge casebook runs use the same four-baseline artifact shape as workspace runs and additionally record the governed bridge action metadata in `report.json`:

- `casebook-runs/<run-id>/bridge/report.json`
- `casebook-runs/<run-id>/bridge/report.md`
- `casebook-runs/<run-id>/bridge/platform-report.md`
- `casebook-runs/<run-id>/bridge/<baseline>/input.md`
- `casebook-runs/<run-id>/bridge/<baseline>/output.md`
- `casebook-runs/<run-id>/bridge/<baseline>/score.json`
- `casebook-runs/<run-id>/bridge/contextmesh_packet/packet.md`

Internal-ready acceptance summaries write:

- `acceptance-reports/<run-id>/report.json`
- `acceptance-reports/<run-id>/report.md`

The current minimum acceptance slice has real gates for all three lines:

- workspace: continuation, isolation, and groundedness from the `contextmesh_packet` baseline
- conversation: continuation, isolation, and groundedness from the chat contract runner score
- bridge: signal retention, groundedness, and target fitness from the `contextmesh_packet` baseline

Casebook suite runs write:

- `casebook-suite/<run-id>/report.json`
- `casebook-suite/<run-id>/report.md`

The suite runner executes every case directory under the selected `case-root`, infers the continuity line from the case id prefix, and records per-case status, line, and report URI. The current smoke run against `casebook/cases` executes 16 cases with 0 failures under the mock provider.

Benchmark coverage reports write:

- `benchmark-coverage/<run-id>/report.json`
- `benchmark-coverage/<run-id>/report.md`

The benchmark coverage runner validates that all named public benchmarks are at least `translated_task`, at least 4 reach `executable_evaluation`, and executable benchmarks name concrete case ids. It reports translated proxies, design mappings, and registered original executions as separate counters. Every original evidence path must load a valid execution manifest and qualification; a path string alone is rejected. The current map covers 11 public benchmarks, 8 executable translated evaluations, 3 design mappings, and 4 qualified original-data executions: one oracle QA sample, full lexical and vector LongMemEval-S retrieval runs, and one full LongMemEval-S reader QA run with a custom judge.

Internal Ready reports write:

- `internal-ready/<run-id>/report.json`
- `internal-ready/<run-id>/report.md`

The `internal-ready` command runs the casebook suite and benchmark coverage chain, then checks these gates:

- at least 15 main cases are defined
- at least 10 casebook cases execute
- all named benchmarks are translated and at least 4 are executable
- workspace, conversation, and bridge all have executed coverage
- JSON/Markdown artifacts exist for the suite and benchmark reports

The current mock smoke result is `pass=true`, `cases=16`, `executed=16`, `benchmarks=11`, and `executable_benchmarks=8`.

## Internal Ready Commands

Run the full Internal Ready chain:

```bash
go run ./cmd/vermory internal-ready \
  --artifact-root ./artifacts \
  --run-id internal-ready-smoke \
  --provider mock \
  --model mock-model \
  --case-root casebook/cases \
  --benchmark-map casebook/benchmarks/public-benchmark-map.json
```

Run only the executable casebook suite:

```bash
go run ./cmd/vermory eval-casebook-suite \
  --artifact-root ./artifacts \
  --run-id casebook-suite-smoke \
  --provider mock \
  --model mock-model \
  --case-root casebook/cases
```

Run only benchmark coverage:

```bash
go run ./cmd/vermory benchmark-coverage \
  --artifact-root ./artifacts \
  --run-id benchmark-coverage-smoke \
  --map-path casebook/benchmarks/public-benchmark-map.json
```

## Current Status

- Mock matrix: completed
- Duojie core matrix: completed
- Internal Ready mock chain: completed
- LongMemEval original oracle sample with Grok: completed as `dataset_sample`
- LongMemEval-S full retrieval: completed as `qualified_dataset_full`
- LongMemEval-S full vector retrieval: completed as `qualified_dataset_full`
- LongMemEval-S full reader QA with Grok: completed as `qualified_dataset_full`
- Explicit source revision runtime with Grok MCP: completed
- Governed source conflict candidate runtime with Grok MCP: completed
- Provider-assisted unkeyed source target matching with Grok MCP: completed
- Governed multi-fact trusted-document formation with Grok MCP: completed
- Automatic conversation formation and direct OpenClaw review with Grok: completed
- Official Hermes automatic-formation isolation control with direct DeepSeek-V4-Flash: completed
- Verified OpenClaw tool-outcome formation, review, recall, isolation, replay, and shared-evidence forgetting: completed

## Completed Runs

- Mock matrix run ID: `matrix-mock-full`
- Mock regression run ID: `matrix-mock-regression`
- Duojie parallel smoke run ID: `duojie-matrix-parallel-smoke`
- Duojie core matrix run ID: `duojie-matrix-core-v3`
- SiliconFlow Qwen core matrix run ID: `siliconflow-matrix-qwen-core`
- Casebook suite smoke run ID: `casebook-suite-smoke`
- Benchmark coverage smoke run ID: `benchmark-coverage-smoke`
- Internal Ready smoke run ID: `internal-ready-smoke`
- LongMemEval original sample run ID: `longmemeval-original-sample-grok-20260714-attempt-6`
- LongMemEval-S full retrieval run ID: `longmemeval-s-full-retrieval-20260715-v1`
- LongMemEval-S full vector retrieval run ID: `longmemeval-s-full-vector-retrieval-20260719-v7`
- LongMemEval-S full reader QA run ID: `longmemeval-s-full-reader-qa-grok-20260715-v3`
- Source revision Grok session: `955B4CA6-68EB-4D0E-9CB4-96BE91AC1776`
- Source candidate Grok session: `019f5f3c-d836-7d80-a8de-995dcde29ef8`
- Source candidate stale-probe session: `019f5f3e-4b7f-7510-8cda-a26e0ba89725`
- Unkeyed source matching coder session: `019f5fac-feb7-7cf0-a762-15aab01c705a`
- Unkeyed source matching stale-probe session: `019f5fae-1a0e-7fb0-b72d-6a726f5e9815`
- Governed document formation coder session: `019f6019-63fc-78e2-8eb6-41ddfabe62d8`
- Governed document formation stale-probe session: `019f601a-fdfe-7bb0-96ac-376ba8b99515`
- Automatic conversation review run ID: `w22-f02-20260718-b6c6154`
- Verified tool-outcome formation run ID: `7e76df6f05`

## Automatic Conversation Review W22

The frozen `F02-automatic-conversation-review` case executes a real
OpenClaw/Grok conversation, durable asynchronous formation, exact-evidence
review, explicit acceptance and rejection, a corrected deadline update,
forgetting, worker restart, and completion replay. The same fixed-tenant worker
also processes an official Hermes session while preserving exact OpenClaw and
Hermes continuity isolation.

The run is a client and governance qualification, not a model ranking. Grok
and DeepSeek are compatibility targets on different parts of the trajectory.
The accepted result is `23 / 23` hard gates with zero cross-client candidate
leakage, zero model governance tools, HTTP `403` for client-token candidate
access, and zero credential-pattern hits in normalized evidence. See
[Automatic Conversation Formation And Review Qualification](evidence/2026-07-18-automatic-conversation-review.md).

## Verified Tool Outcome Formation W23

The frozen `F03-verified-tool-outcome-formation` case executes a real
OpenClaw/DeepSeek tool call, stores one successful allowlisted `read` result,
forms two reviewable candidates through Grok, accepts both through direct
OpenClaw governance, recalls both through current vector retrieval, and then
forgets one of two memories sharing the same tool-result observation.

The accepted result proves that forgetting the capacity memory removes its
governed content, formation item, lexical projection, vector projection, and
delivery residue without destroying the live cleanup-safety sibling or
shifting the shared evidence byte offsets. A reset OpenClaw transcript then
received only the safety memory; DeepSeek-V4-Flash reported safety and stated
that capacity was unavailable. PostgreSQL independently recorded one current
vector delivery, no degradation, and only the safety memory ID.

Failed `read`, unallowlisted `exec`, and sensitive `read` controls stored zero
tool-result rows. An unrelated `read` formed one C-204 candidate only in its
own review inbox. Exact replay returned `replayed=true` without a third tool
row. The run is a platform and client compatibility qualification, not a model
ranking.

The post-delete user message also produced a retained provider compatibility
matrix. Expired Grok authentication, strict-output failures from
DeepSeek-V4-Flash and MiniMax-M2.5, an out-of-manifest Qwen 30B reference, and
disabled Qwen 235B and GLM-4.6 targets all failed closed and produced no new
candidate. See [Verified Tool Outcome Formation Qualification](evidence/2026-07-18-verified-tool-outcome-formation.md).

## Explicit Source Revision Runtime

The frozen `106-workspace-source-revision` case replaces one named release
command from a newer trusted source while preserving an independent `800 ms`
timeout. A real Grok `grok-4.5` coding task consumed the current workspace
context through MCP, generated and deterministically checked
`release-source-check.md`, and wrote the task result back as `proposed`.

PostgreSQL and real MCP probes established:

- the old command is `superseded` and has no search projection;
- the replacement is `active` and points to the old memory;
- the independent timeout remains `active`;
- the other-workspace distractor is absent from all three deliveries;
- target-task, exact-stale, and paraphrased-stale deliveries contain no stale command;
- projection rebuild preserves three active documents and the same fingerprint.

Official Codex CLI attempts were retained but do not count as a success for
this slice because unsupported model selections and then the account usage
limit stopped each run before MCP. See [the scoped runtime evidence](evidence/2026-07-14-source-revision-runtime.md).

## Governed Source Conflict Candidate Runtime

The frozen `107-workspace-source-conflict-candidate` case uses stable key
`release.signing.mode`. A first source change was proposed and rejected without
changing current retrieval. A second proposal was accepted, atomically
superseding the old keychain fact while preserving the independent `800 ms`
timeout and the rejected candidate as audit history.

A real isolated Grok `grok-4.5` task consumed the accepted workspace through
MCP, created and deterministically checked `release-signing-check.md`, and
wrote its result back as `proposed`. Persisted delivery bodies, not model
self-report, established that:

- proposal alone kept the old current fact and excluded the candidate;
- cross-tenant acceptance was rejected;
- rebuild excluded proposed, rejected, and superseded source states;
- the target task included OIDC and `800 ms` but no keychain or other-tenant fact;
- exact and paraphrased stale probes returned OIDC and no stale fact;
- the client result remained a non-authoritative proposed observation.

This is deterministic keyed formation, not arbitrary-document extraction or
general semantic conflict matching. See
[the scoped runtime evidence](evidence/2026-07-14-source-conflict-candidate-runtime.md).

## Provider-Assisted Unkeyed Source Target Matching

The frozen `108-workspace-unkeyed-source-target-match` case removes the trusted
ingestor's knowledge of `memory_key` while retaining exact source content and
revision identity. A real Grok `grok-4.5` provider received three current
same-workspace keyed facts and selected `release.signing.mode` for the OIDC
signing revision. The same candidate snapshot produced abstentions for one
ambiguous release-flow source and one unrelated maintenance source.

PostgreSQL and real MCP probes established:

- the provider candidate packet excluded the other tenant;
- all three provider decisions share one durable candidate-set fingerprint;
- matching created only a proposed candidate and left pre-accept context on the
  old keychain fact;
- explicit acceptance activated OIDC and projection rebuild excluded the stale
  target and proposed write-backs;
- a real Grok MCP coder created `release-control-policy.md` with OIDC, `800 ms`,
  and SLSA attestation while excluding keychain and static credentials;
- exact and paraphrased stale probes returned OIDC only;
- source-match audit is RLS protected and excluded from model-facing context.

This is closed-set provider matching, not arbitrary-document extraction,
open-vocabulary conflict discovery, or automatic memory activation. See
[the scoped runtime evidence](evidence/2026-07-14-unkeyed-source-target-matching-runtime.md).

## Governed Multi-Fact Document Formation

The frozen `109-workspace-multifact-document-formation` case supplies one
bounded trusted revision with an unchanged region, an updated retry limit, a
new rollback-approval rule, one embedded instruction, and one uncertain policy
sentence. A real Grok `grok-4.5` provider returned structured formation output;
Vermory verified exact byte spans and the unchanged active-fact snapshot before
creating two proposed candidates and one audit-only unchanged item.

PostgreSQL and real MCP probes established:

- malformed fenced output and incorrect unchanged normalization fail durably
  with zero formation items;
- the injection-only and undecided-policy document abstains with zero items;
- pre-review context retains retry 3 and excludes retry 5 plus rollback;
- explicit acceptance activates only the reviewed retry and rollback facts;
- target-tenant projection rebuild preserves four active documents and an
  identical fingerprint;
- a real Grok MCP coder creates `deployment-control-policy.md` with region,
  retry 5, two-maintainer approval, and signed SLSA while excluding stale,
  injected, cross-tenant, and uncertain text;
- exact and paraphrased stale probes return only current facts;
- formation audit tables are RLS protected, and a real provider forget probe
  leaves zero old/new marker residue in linked run and item text.

The real provider suggested `deploy.rollback.maintainers` while the deterministic
fixture names the semantic slot `deploy.rollback.approvals`. The candidate was
operator-reviewed and usable, but this slice does not claim deterministic
provider-generated ontology naming. See
[the scoped runtime evidence](evidence/2026-07-14-governed-document-formation-runtime.md).

## Production Retrieval Ablation

W08 materializes 48 active, four proposed, four superseded, and four deleted
governed memories across four tenants and eight continuities, then executes 24
frozen retrieval queries through the unchanged lexical runtime, direct
SiliconFlow `BAAI/bge-m3` PostgreSQL/pgvector, and exact-guarded RRF.

| Condition | Hit@1 | Recall@K | MRR | nDCG@K | P95 | Forbidden | Ineligible |
|---|---:|---:|---:|---:|---:|---:|---:|
| `lexical_runtime` | 0.6667 | 0.6875 | 0.6806 | 0.6526 | 2.871 ms | 0 | 0 |
| `vector_pg` | 0.9583 | 1.0000 | 0.9792 | 0.9623 | 126.526 ms | 0 | 0 |
| `hybrid_rrf` | 0.9583 | 1.0000 | 0.9792 | 0.9623 | 130.134 ms | 0 | 0 |

The final run used 152 direct embedding requests and passed active-only
projection set equality plus delete/rebuild result-ID equivalence. The first
provider probe and first complete run remain documented: SiliconFlow rejected
the optional `dimensions` request field, and an ANN index containing filtered
non-active rows changed results after active-only rebuild. Both defects were
fixed before the final run.

The result supports an active-only pgvector production integration slice. It
does not establish an RRF benefit: vector and hybrid quality were identical,
and hybrid added latency. H-009 is therefore `testing/measured`, not accepted as
the product default. See
[the scoped evidence](evidence/2026-07-14-production-retrieval-ablation.md).

## Production Retrieval Runtime

W09 connects the frozen direct SiliconFlow `BAAI/bge-m3` profile to the real
workspace MCP, conversation Web Chat, and authenticated runtime construction
paths while retaining lexical as the default. PostgreSQL projection events,
cursors, active-only vectors, and non-sensitive retrieval audits are durable;
the worker runs under a fixed tenant and restricted PostgreSQL role.

| Runtime gate | Result |
|---|---|
| Real Grok MCP vector consumption and proposed writeback | PASS |
| Real Grok Web Chat linked-conversation answer | PASS after recorded cursor catch-up |
| Shadow delivery byte equality with lexical | PASS |
| Projection-lag degradation | `vector -> lexical / projection_lag` |
| Provider-outage degradation | `vector -> lexical / embedding_unavailable` |
| Vector reset/rebuild result IDs and authority | unchanged |
| Restricted-role no-context and cross-tenant probes | PASS |
| Native dump/restore and restore-side rebuild | PASS |
| New credential-shaped artifacts | 0 |

This is `production_path_integrated`, not `accepted_default`. H-009 remains
`testing` pending a second independent retrieval batch, threshold review,
source-authority ranking, embedding migration, and scale/fault qualification.
See [the scoped evidence](evidence/2026-07-14-production-retrieval-runtime.md).

## Independent Retrieval Batch W10

W10 is the second independent retrieval-quality batch. The qualified v5 replay
used corpus version 1 with a fresh PostgreSQL 18 database, schema 15, 39 governed records across six
scopes and four tenants, 18 queries, and 102 direct SiliconFlow `BAAI/bge-m3`
embedding requests. All
retrieval records were seeded through the authoritative runtime and all vector
projection state was rebuilt from active authority.

| Condition | Hit@1 | Recall@K | MRR | nDCG@K | P95 | Forbidden | Ineligible |
|---|---:|---:|---:|---:|---:|---:|---:|
| `lexical_runtime` | 0.7222 | 0.7593 | 0.7500 | 0.7353 | 3.894 ms | 0 | 0 |
| `vector_pg` | 1.0000 | 1.0000 | 1.0000 | 0.9919 | 154.697 ms | 0 | 0 |
| `hybrid_rrf` | 1.0000 | 1.0000 | 1.0000 | 0.9908 | 154.953 ms | 0 | 0 |

All hard gates passed, including active-only projection equality, zero
forbidden/ineligible results, and rebuild equivalence. A same-identity replay
returned `replayed=true`. The batch strengthens the case for an opt-in semantic
projection but again provides no independent RRF gain; H-009 remains
`testing/measured` and lexical remains the default. See [the W10 evidence](evidence/2026-07-15-independent-retrieval-batch.md).

W10 corpus version 2 subsequently corrected one scoring label: an active
same-scope shopping-budget distractor was removed from the order-date query's
zero-tolerance forbidden set. No record, query text, relevant set, or returned
order changed. The validator now requires any same-scope active forbidden fact
to be explicitly listed in `task_excluded_record_ids`.

## Retrieval Profile Migration Decision H-011

The production profile comparison seeded W10 once into PostgreSQL authority,
built both registered SiliconFlow profiles from the same durable projection
events, ran all 18 queries through the production coordinator, reset and
rebuilt each profile, and repeated the queries. The formal full-revision run
and two supporting runs all produced the same quality values and decision.

| Profile | Hit@1 | Recall@K | MRR | nDCG@K | Formal P95 | Hard gates | Rebuild |
|---|---:|---:|---:|---:|---:|---|---|
| `siliconflow-bge-m3-1024-v1` | 1.0000 | 1.0000 | 1.0000 | 0.9919 | 118.283 ms | PASS | equivalent |
| `siliconflow-bge-large-zh-1024-v2` | 0.8889 | 1.0000 | 0.9352 | 0.9416 | 125.794 ms | PASS | equivalent |

Both profiles kept 30 active vectors, zero cursor lag, zero forbidden or
ineligible results, zero degradation, and 36 successful profile-specific audit
rows. The candidate exceeded the frozen MRR and nDCG regression limits, so the
decision is `keep_candidate`; v1 remains active/default. This supports the
versioned-generation mechanism, not a general embedding-model ranking. See
[the decision evidence](evidence/2026-07-15-retrieval-profile-promotion-decision.md).

## Active-Backlog Dimensional Migration W17

W17 qualifies a separate physical projection class while the existing active
profile continues serving. PostgreSQL 18 held 20,000 current facts across four
tenants. The active `vector_1024` profile and candidate `halfvec_2560` profile
each converged to the same 20,000 eligible IDs after 2,000 revisions, 500
deletions, 500 new facts, and exactly 5,000 tail events.

| Gate | Result |
|---|---:|
| authority / lexical / incumbent / candidate | `20,000 / 20,000 / 20,000 / 20,000` |
| final incumbent / candidate lag | `0 / 0` |
| scoped incumbent queries | `320 / 320` successful |
| cross-scope results | `0` |
| query p50 / p95 / p99 | `12 / 20 / 30 ms` |
| partial candidate rows after immediate restart | `0` |
| interrupted cursor advance | `0` |
| same pools recovered | `PASS` |
| candidate reset isolated and rebuilt | `PASS` |
| direct provider model / dimensions / requests | `Qwen3-Embedding-4B / 2560 / 2` |
| hard gates | `12 / 12 PASS` |

The candidate uses `halfvec(2560)` because pgvector 0.8.5 limits HNSW indexes
on `vector` to 2,000 dimensions and supports `halfvec` through 4,000. This run
qualifies mechanics and recovery for the named class; it does not rank models,
measure half-precision quality, or promote the candidate. See
[the W17 evidence](evidence/2026-07-16-active-backlog-dimensional-migration.md).

## Memory Eligibility And Retention W19

W19 evaluates current-use eligibility separately from durable history. The
formal profile created 10,000 governed memories across four tenants and twenty
continuities, then ran 320 production-shaped scoped queries through the shared
runtime.

| Gate | Result |
|---|---:|
| current facts returned | `4,000 / 4,000` |
| scoped queries | `320 / 320` successful |
| query p50 / p95 / p99 | `23 / 40 / 53 ms` |
| scheduled premature use | `0` |
| expired / archived misuse | `0 / 0` |
| deletion residue | `0` |
| Global Default pollution | `0` |
| cross-scope leakage | `0` |
| lexical / vector eligible rows | `7,000 / 7,000` |
| provider model / dimensions / requests | `BAAI/bge-m3 / 1024 / 2` |
| hard gates | `16 / 16 PASS` |

The frozen four-task baseline produced `0/4` success without context, `1/4`
with full history, `2/4` with lifecycle-only filtering, and `4/4` with Vermory
eligibility. Full history made six invalid reuses and lifecycle-only filtering
made three; Vermory eligibility made zero while delivering four memories in 48
context tokens. These are deterministic case outcomes, not a model-judge score.

The accepted report binds real Grok Web Chat, Grok MCP, and official Codex MCP
artifacts. Rebuild, restart, restore, idempotency, conflict rejection, concurrent
forget, and provider-outage degradation all passed without reviving ineligible
content. This supports an orthogonal retention model based on continuity
location, lifecycle, validity, and policy rather than one mandatory
`retention_class`. See [the W19 evidence](evidence/2026-07-16-memory-eligibility-retention.md).

## Hermes Real-Client Continuity W20

W20 executes the frozen `H01-hermes-linked-sessions` case through the official
Hermes `v0.18.2` CLI and a direct SiliconFlow
`deepseek-ai/DeepSeek-V4-Flash` model route. This is client and continuity
qualification, not a model ranking.

| Gate | Result |
|---|---:|
| official Hermes CLI and real model turn | pass |
| session B filename occurrences before link | `0` |
| automatic cross-session merge | `0` |
| explicit link required | pass |
| linked current answer | `thesis-defense-v7.zip` |
| obsolete filename presented as current | `0` |
| unrelated Hermes/OpenClaw context in delivery | `0` |
| client-reported model audited | `deepseek-ai/DeepSeek-V4-Flash` |
| post-reverse context bytes | `0` |
| fail-open visible answer | `FAIL-OPEN-OK` |
| fail-open binding / turn delta | `0 / 0` |
| credential leak count | `0` |
| LaunchAgent restart and loopback listener | pass |
| hard gates | `16 / 16 PASS` |

The accepted delivery contained exactly one confirmed governed memory and no
raw session-A transcript. The failure ledger retains an invalid
`--oneshot --resume` false positive whose Hermes answer was nonempty while the
Vermory delivery was empty; that attempt is explicitly rejected. See
[the W20 evidence](evidence/2026-07-18-hermes-real-client.md).

## Conversation Formation Loop W21

W21 executes the frozen `F01-conversation-formation-loop` case through real
OpenClaw user turns, `grok-cli/grok-4.5`, and a direct SiliconFlow
`deepseek-ai/DeepSeek-V4-Flash` formation route. The model is a compatibility
target, not a model-ranking result.

| Gate | Result |
|---|---:|
| real Session A user turns persisted | pass |
| real Session B/C continuities isolated | `2 / 2` rejected before provider execution |
| accepted initial candidates | `3` |
| rejected lifecycle candidate | `1` |
| rain/lunch candidates | `0` |
| accepted correction updates | `1` |
| turn-local language candidates | `0` |
| active Global Defaults | `0` |
| fresh OpenClaw answer | Saturday at 10:00 + concierge |
| deleted-value residual surfaces | `0 / 7` nonzero |
| external OpenClaw state exact-value matches | `0` |
| offline provider replay | `0 s`, one run, no duplicate candidates |
| outside-manifest provider output | `evidence_observation_outside_manifest`, 0 items |
| input / active-snapshot drift | `input_manifest_changed / active_snapshot_changed`, 0 items |
| fail-open answer / false binding | `FAIL_OPEN_OK / 0` |
| provider-key, `.env`, direct assignment, env-dump leaks | `0 / 0 / 0 / 0` |
| qualification gates | `18 / 18 PASS` |

The accepted direct formation took 14 seconds in explicit non-thinking mode
after the OpenAI-compatible adapter was corrected to include the required JSON
schema. A default-thinking timeout, invalid pre-schema output, an extra model
candidate, an ambiguous client answer, two incomplete deletion-probe answers,
a 179-second Grok timeout, and a failed-audit redaction defect remain in the
failure ledger. See
[the W21 evidence](evidence/2026-07-18-conversation-formation-loop.md).

## Projection Outbox Fault Profile W11

W11 starts a disposable PostgreSQL 18 cluster and exercises the production
projection event worker under backlog, retry, duplicate replay, immediate
database restart, pool recovery, and concurrent deletion. A separate tenant
then performs direct SiliconFlow projection and vector retrieval after restart.

| Gate | Result |
|---|---:|
| Initial authority/event backlog | 1,000 |
| First bounded pass | 128 processed / 872 lag |
| Provider failure cursor movement | 0 |
| Vectors after full cursor rewind/replay | 1,000 |
| Partial vector during PostgreSQL stop | 0 |
| Same pgx pool recovery | PASS |
| Deleted in-flight vector after retry | 0 |
| Final lag | 0 |
| Direct provider requests | 2 |

The case supports H-012 for the current self-hosted profile and keeps Redis
optional. It does not qualify 100k/1M scale, sustained multi-worker throughput,
HA, or PITR. See [the W11 evidence](evidence/2026-07-15-projection-outbox-fault-profile.md).

## Server Qualification Scale Profile W12

W12 starts a disposable PostgreSQL 18 cluster and creates 100,000 initial
active facts plus 450,000 append-only governed revisions. The resulting
550,000 governed memories preserve 100,000 current facts and 450,000
superseded facts while real triggers create exactly 1,000,000 projection
events. A current-authority snapshot embeds 100,000 current facts rather than
replaying the full history, then competing workers consume 1,000 concurrent
deletion events.

| Gate | Result |
|---|---:|
| Governed / active / superseded | `550,000 / 100,000 / 450,000` |
| Projection events before / after delete tail | `1,000,000 / 1,001,000` |
| Lexical / vector rows before deletion | `100,000 / 100,000` |
| Snapshot embedding requests | `100,000` |
| Active / lexical / vector rows after deletion | `99,000 / 99,000 / 99,000` |
| Competing workers | `10` winners / `10` already running |
| Final lag / scope leaks / deleted residue | `0 / 0 / 0` |
| Query samples and P50/P95/P99 | `1,000`, `8/234/266 ms` |
| Database size | `2,710,910,655 bytes` |
| Direct provider requests | `2` |

The 1,000 queries all returned the expected current memory during concurrent
deletion. The original run recorded 138 effective vector results and 412 exact
lexical fallbacks among 550 requested vector queries. An instrumented replay
recorded 145 effective vector results, 405 `projection_lag` fallbacks, and zero
other degraded results; a separate projection-current 100,000-vector control
served all 550 vector requests without degradation. The split is
scheduler-dependent, but every degraded request was caused by intentional lag
gating while delete events were pending, not by a scoped HNSW recall failure.
W12 therefore supports the named operational and degradation contract. Lexical
remains the default. See
[the W12 evidence](evidence/2026-07-15-server-qualification-scale-profile.md).
See also [the W13 attribution](evidence/2026-07-15-vector-degradation-attribution.md).

## LongMemEval Original Sample

The committed original-data evidence uses six frozen records from the official
cleaned oracle artifact. It compares `no_context`, `full_oracle_history`,
`plain_lexical_retrieval`, and `vermory_packet` with the same isolated Grok
reader.

| Condition | Completed | Exact | Mean token F1 | Mean answer recall | Abstention |
|---|---:|---:|---:|---:|---:|
| `no_context` | 6/6 | 0/6 | 0.0263 | 0.0385 | 1/1 |
| `full_oracle_history` | 6/6 | 2/6 | 0.5497 | 0.5727 | 1/1 |
| `plain_lexical_retrieval` | 6/6 | 2/6 | 0.4954 | 0.5154 | 1/1 |
| `vermory_packet` | 6/6 | 2/6 | 0.5201 | 0.5214 | 1/1 |

These are deterministic local sample metrics, not official LongMemEval GPT-4o
judge accuracy. The run retained a multi-session counting failure and an
unresolved source-conflict observation rather than upgrading the result to a
full benchmark claim. See [the evidence document](evidence/2026-07-14-longmemeval-original-sample.md).

## LongMemEval-S Full Retrieval

W14 streamed all 500 records from the pinned 277,383,467-byte cleaned
LongMemEval-S artifact into 500 isolated conversation continuities and 23,867
active governed session memories. It scored 470 non-abstention records through
the production lexical coordinator and the same-text token-overlap baseline.

| Condition | K | Recall any | Recall all | nDCG | MRR |
|---|---:|---:|---:|---:|---:|
| token overlap | 10 | `0.9489` | `0.8383` | `0.7983` | `0.8119` |
| Vermory lexical | 10 | `0.9021` | `0.7340` | `0.6918` | `0.6974` |

The run retained the quality regression instead of tuning on the evaluated
labels. Multi-session RecallAll at K10 was `0.5620`. The old `0a995998`
counting failure had all three answer sessions in Vermory's first three ranks,
so it is downstream of retrieval. For `6a1eabeb`, both update sessions were
available by K10, but the older session ranked sixth and remained a separate
source memory. The run had zero runtime failures, 23,867 distinct operation
IDs, and a byte-stable score/failure result after idempotent resume. See
[the W14 evidence](evidence/2026-07-15-longmemeval-s-full-retrieval.md).

## LongMemEval-S Full Reader QA

W15 replayed the frozen W14 K=10 rankings through 1,000 isolated real-reader
tasks: 500 `plain_token_overlap_k10` and 500 `vermory_lexical_k10`. The reader
was `grok-composer-2.5-fast`; the custom judge was `grok-4.5` using the pinned
upstream prompt branches. Both ran with no memory, web search, plans,
subagents, or advertised tools under one-shot `KeepAlive=false` LaunchAgents.

| Condition | Completed | Judge correct | Accuracy | Exact | Mean token F1 | Mean answer recall |
|---|---:|---:|---:|---:|---:|---:|
| token overlap K10 | `500` | `379` | `0.7580` | `252` | `0.5906` | `0.7060` |
| Vermory lexical K10 | `500` | `341` | `0.6820` | `226` | `0.5328` | `0.6557` |

Paired outcomes were `310` both correct, `69` plain only, `31` Vermory only,
`90` neither, and `0` incomplete. The result retains the current lexical
regression rather than tuning on the evaluated labels. Reader runtime failures,
terminal judge failures, cross-condition incompleteness, and tool-call fields
were all zero. Two judge attempts were retried: one fixed 120-second timeout
and one strict rejection of Markdown-wrapped `**yes**`.

This is a qualified public full-dataset reader QA execution with a custom Grok
judge. It is not official GPT-4o LongMemEval accuracy, not a model ranking, and
not withheld or externally sealed evidence. See
[the W15 evidence](evidence/2026-07-15-longmemeval-s-full-reader-qa.md).

## LongMemEval-S Full Vector Retrieval W28

W28 projected the same 23,867 governed session memories through the registered
candidate `siliconflow-bge-m3-1024-chunked-mean-v2` profile and executed the
production vector coordinator for all 500 records. The candidate uses direct
SiliconFlow `BAAI/bge-m3`, bounded UTF-8 chunks, normalized-mean pooling, and
one logical vector per governed memory. It remained inactive throughout the
run.

| Condition | K | Recall any | Recall all | nDCG | MRR |
|---|---:|---:|---:|---:|---:|
| token overlap | 10 | `0.9489` | `0.8383` | `0.7983` | `0.8119` |
| Vermory lexical | 10 | `0.9021` | `0.7340` | `0.6918` | `0.6974` |
| Vermory vector | 10 | `0.9830` | `0.9404` | `0.9069` | `0.9006` |

All `500/500` vector queries remained effective vector results with zero
degradation. The projection reached `idle`, lag `0`, and exactly `23,867`
vectors. Logical embedding items were exactly `24,367/24,367`; physical
provider items were exactly `46,657/46,657`. Independent PostgreSQL checks
found zero scope/lifecycle or content-hash violations across 6,000 delivered
memory IDs. Runtime and terminal provider failures were zero.

The positive quality result has an explicit operational cost. Projection took
`7,210.157s` with one-memory durable commit granularity, and 1,088 of 25,455
provider attempts required retry. Vector p95 query latency was `274ms`, versus
lexical `72ms`. W28 therefore justifies a separately identified frozen-vector
reader replay, but it does not activate the candidate or switch the default.
See [the W28 evidence](evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md).

## LongMemEval-S Domestic Reader QA W30

W30 freezes the W28 `vermory_lexical_k10` and `vermory_vector_k10`
rankings and assigns one direct domestic reader plus a distinct direct custom
judge. It is a provider compatibility and downstream-context utility run, not
a model ranking.

| Gate | Result |
|---|---|
| Frozen reader | SiliconFlow `deepseek-ai/DeepSeek-V4-Flash` |
| Frozen judge | SiliconFlow `Qwen/Qwen3-30B-A3B-Instruct-2507` |
| v1 reader completed | `32 / 1,000` |
| v1 terminal reader failures | `45` |
| v1 judge tasks | not started |
| v1 paired eligible records | `0` |
| exact-head `34d0402` reader probe | `403`, code `30001` |
| exact-head `34d0402` judge probe | `403`, code `30001` |
| v2 | not created |
| W30 qualification | **blocked / not qualified** |

The provider classified every retained failure as insufficient account
balance. The partial reader prefix is not randomized and no judge ran, so it
does not produce lexical-versus-vector evidence. The exact-head availability
check also exposed that the old probe CLI returned exit `0` even when every
per-model result was `error`; the CLI now preserves the report and returns
nonzero for any failed model. See the
[rejected v1 evidence](evidence/2026-07-20-longmemeval-s-domestic-vector-reader-qa-rejected-v1.md)
and the
[exact-head availability check](evidence/2026-07-21-w30-provider-availability-34d0402.md).

## PostgreSQL HA And PITR Qualification

W16 executes the frozen `I03-postgresql-ha-pitr` operations trajectory against
dedicated PostgreSQL 18.4 clusters. This is a deterministic platform and
database qualification, not a model evaluation; the Web Chat path uses the
mock provider because LSN, RLS, row, deletion, and credential gates are checked
directly.

| Gate | Result |
|---|---:|
| primary/standby system identifiers equal | pass |
| standby replay reached primary flush LSN | `0/5000000` = `0/5000000` |
| transition operation rows | `0` |
| pre-failover / post-promotion rows | `1 / 1` |
| unchanged handler/runtime/auth pools | pass |
| PITR target LSN | `0/402ACE0` |
| T2/restored authority fingerprint equal | pass |
| T3 deletion/revocation and T4 fact excluded | pass |
| rebuilt active lexical projections | `3` |
| historical token rejected after re-governance | HTTP `401` |
| new operator token accepted | pass |
| restored RLS policies / tenant FKs | `19 / 36` |

The final report preserves three implementation failures plus the intentionally
injected transition outage. Same-host timings are not SLOs, and the run does
not claim cross-host HA, leader election, split-brain prevention, or a database
proxy. See [the W16 evidence](evidence/2026-07-16-postgresql-ha-pitr.md).

## Protected Artifact Signing W24

W24 is a security and delivery qualification rather than a memory-quality or
model-ranking experiment. The protected pull-request workflow builds a complete
eight-file release payload manifest in an ordinary no-OIDC test job, then a
separate same-repository post-test job signs the exact manifest with GitHub OIDC
and pinned Cosign `v3.0.6`.

| Gate | Result |
|---|---:|
| protected `test` job | pass |
| protected `sign-snapshot` job | pass |
| release-manifest entries | `8` |
| payload hash failures | `0` |
| exact workflow identity and issuer | pass |
| modified manifest accepted | `0` |
| wrong workflow identity accepted | `0` |
| cross-host artifact digest mismatch | `0` |
| private signing keys or retained OIDC tokens | `0` |
| tags or GitHub Releases created | `0` |

The signed subject is the complete payload manifest, not the GitHub transport
ZIP. Independent ARM64 Mac mini verification used the Qingdao reverse-management
tunnel, an isolated official Cosign binary, and no `sudo` or system-wide
installation. `test` and `sign-snapshot` are strict required checks on `main`.

This does not qualify manual snapshots, tagged release publication,
notarization, package-manager distribution, container signing, SLSA provenance,
or external sealed evaluation. See
[the W24 evidence](evidence/2026-07-18-protected-artifact-signing.md).
+
## Cursor Agent Real-Client Attempt W25

W25 freezes `W04-canonical-repository-cross-client` and attempts the complete
workspace MCP path through the real `cursor-agent` CLI. The Mac mini
PostgreSQL seed contains a current `samekind/Vermory` revision, one superseded
personal-repository fact, and an active same-name distractor in another
continuity.

The real Cursor CLI successfully discovers the remote stdio server and exposes
only `prepare_context` and `commit_observation`. The generation request then
exits `1` with an unpaid-invoice account error before either tool is called.
The final ledger therefore contains zero deliveries, zero agent-result
observations, zero proposed memories, and no artifact.

| Gate | Result |
|---|---|
| Real Cursor binary and model discovery | pass |
| Project-local MCP approval and readiness | pass |
| Restricted two-tool MCP schema | pass |
| Real `prepare_context` call | blocked |
| Exact artifact | not run |
| Proposed-only write-back and replay | not run |
| PostgreSQL zero-side-effect failure check | pass |
| Privacy and 85-entry checksum verification | pass |
| W25 qualification | **blocked / not qualified** |

The failed attempts are retained rather than replaced by another client. See
[the W25 evidence](evidence/2026-07-19-cursor-agent-real-client-attempt.md).

## Physical Cross-Host Workspace Continuity W34

W34 moved one existing workspace continuity from a real external-volume clone
on the operator workstation to an independent clone on the Mac mini. Both
clones used the same `samekind/Vermory` remote and exact commit, but that Git
identity did not authorize continuity until an explicit rebind.

| Gate | Result |
|---|---|
| exact protected release head | `ceafea623791beeeb1f2a10e70f3f45a0bae494c` |
| protected CI and OIDC snapshot | pass |
| physical hosts / real clones | `2 / 2` |
| unbound target | `needs_confirmation`, zero side effects |
| explicit rebind and replay | pass |
| continuity ID preserved | pass |
| target/other-tenant isolation | pass |
| MCP protocol / tools | `2025-06-18` / exactly `2` |
| SDK artifact / proposed writeback / replay | pass |
| exact reversal | pass |
| Grok MCP doctor | pass |
| Grok generation | external blocked by expired authentication |
| W34 hard gates | `28 / 32` pass, `4` external blocked, `0` platform failures |

The SDK trajectory proves the physical runtime and PostgreSQL contract but is
not attributed to Grok. The Grok attempt produced no artifact, delivery,
observation, or memory and cannot be replaced by another client. W34 therefore
does not claim a complete Grok client pass. See
[the W34 evidence](evidence/2026-07-22-physical-cross-host-workspace-continuity.md).

## Authenticated Remote Web Chat W35

W35 moved the Web Chat browser contract from loopback-only qualification to an
authenticated public HTTPS deployment. Chrome reached an exact protected and
OIDC-signed Darwin ARM64 release through a reverse entrypoint and private relay;
the Vermory backend remained loopback-only and PostgreSQL 18.3 schema 24
remained authoritative.

| Gate | Result |
|---|---|
| protected source revision | `7d0839d7f2935a933cab9477a82bdcae2d7252e8` |
| protected CI / signed snapshot | pass / pass |
| remote Chrome and HTTPS | Chrome 150 / secure same-origin |
| W35 hard gates | `26 / 26` pass |
| same-anchor tenant isolation | `2` bindings / `2` tenants / `2` continuities |
| client/operator authority | chat allowed / governance role-gated |
| correction | fresh linked turn contains current value, not superseded value |
| forgetting | `deleted|true|true|0|0|0` with source transcript retained |
| revoked browser retry | `0` committed rows before replacement auth, then `1 / 1` idempotent row |
| relay failure retry | `502 / 0` while down, then `200 / 1` after restore |
| restart | transcript and deleted state preserved |
| raw-token residue | zero across browser, logs, PostgreSQL, and public files |

The deterministic provider isolates deployment, authentication, isolation, and
governance behavior. W35 is not a model-quality, multi-browser, Internet-scale
SLA, or general identity-product claim. See
[the W35 evidence](evidence/2026-07-23-authenticated-remote-webchat.md).

## Native Linux Package Repository Qualification I08

I08 extends the accepted I06 package bytes into native signed APT and DNF
repository bundles without substituting a rebuilt package. Four protected CI
legs cover APT/DNF on native AMD64/ARM64 machines, and a dependent OIDC job
incorporates the exact accepted bundles into the complete release manifest.

| Gate | Result |
|---|---:|
| exact pull-request head | `858e5afc05b8136fd81d46a88164f799f2925197` |
| APT native legs | `2 / 2` pass |
| DNF native legs | `2 / 2` pass |
| hard gates per leg | `18 / 18` |
| exact I06 package digest mismatches | `0` |
| package-manager download digest mismatches | `0` |
| installed source revision mismatches | `0` |
| repository metadata signature enforcement | `4 / 4` pass |
| tampered metadata accepted | `0` |
| service auto-activation | `0` |
| private signing keys in bundles | `0` |
| signed snapshot repository entries | `4` |
| complete release-manifest entries | `16` |
| payload hash failures | `0` |
| exact workflow identity and issuer | pass |
| modified manifest accepted | `0` |
| wrong workflow identity accepted | `0` |

APT used `signed-by`; DNF used `repo_gpgcheck=1`. The DNF negative control
used an independent repository identifier, isolated cache and persistence
directories, forced refresh, and disabled unavailable-repository skipping, so
the package manager itself had to reject the modified metadata. Six failed CI
runs are retained as evidence of incorrect cache, metadata-format, DNF5 CLI,
download-location, and tamper-probe assumptions.

This qualification uses per-leg ephemeral CI signing keys and `file://`
transport. It does not claim a stable production signing key, public hosted
repository, mirrors, retention, cross-version upgrades or rollback, tagged
publication, or RPM payload signatures. See
[the I08 evidence](evidence/2026-07-23-linux-package-repository.md).

## Cross-Version Linux Repository Lifecycle Qualification I09

I09 keeps I08's signed repository boundary and adds one frozen cross-version
lifecycle. Exact base packages from
`858e5afc05b8136fd81d46a88164f799f2925197` and exact candidate packages from
`b2076981351f3e4f2f31c86a1abcd22c04cd71ec` are assigned qualification-only
versions, retained together in the full repository snapshot, and exercised by
native APT and DNF on both architectures.

| Gate | Result |
|---|---:|
| accepted complete CI run | `29959050993` |
| APT native lifecycle legs | `2 / 2` pass |
| DNF native lifecycle legs | `2 / 2` pass |
| hard gates per leg | `26 / 26` |
| base source-revision mismatches | `0` |
| candidate source-revision mismatches | `0` |
| normal candidate upgrades | `4 / 4` pass |
| implicit downgrades observed | `0` |
| explicit retained-base rollbacks | `4 / 4` pass |
| normal candidate re-upgrades | `4 / 4` pass |
| operator configuration losses | `0` |
| service identity changes | `0` |
| service activations or enables | `0` |
| package-triggered database migrations | `0` |
| private signing keys in bundles | `0` |
| downloaded artifact digest mismatches | `0` |
| complete release-manifest entries | `16` |
| I09 qualification bundles presented as release payloads | `0` |
| exact workflow identity and issuer | pass |
| modified manifest accepted | `0` |
| wrong workflow identity accepted | `0` |

The accepted lifecycle is `base install -> normal candidate upgrade -> normal
update without downgrade -> explicit base rollback -> normal candidate
re-upgrade -> removal`. Operator configuration and the non-login service
UID/GID survive every transition, while the service remains inactive and
disabled. Both repository snapshots in one leg use the same ephemeral
qualification key; unrelated repositories are disabled.

The first workflow attempt is retained as a GitHub hosted-runner delay during a
published Actions incident. Its rerun is also retained: all four lifecycle legs
exited after the correct dormant-service result because a `set -e` shell check
treated the expected non-zero `systemctl` status as failure. The accepted head
uses explicit `if` checks and includes a regression contract test.

This qualification does not claim a stable production key, public hosted
repository, mirror or retention policy, release tag, public semantic-version
promise, unattended update policy, database migration or rollback, or RPM
payload signature. See
[the I09 evidence](evidence/2026-07-23-linux-repository-lifecycle.md).


## Duojie Core Matrix Findings

Tested models:

- `gemini-3-flash`
- `gemini-3.1-pro`
- `glm-5`

Task coverage:

- 5 real self-case tasks per model
- 4 baselines per task
- 15 task reports total

Observed pattern:

- The three models respond differently to the same packet and task, so they remain useful compatibility-test objects.
- `gemini-3.1-pro` produced forbidden phrases like `invented teams` / `fake timelines` on the real-case-policy task; the evidence is retained rather than replaced by a cleaner run.
- The task-level scores identify packet, runner, assertion, or consumer behavior that needs investigation. They do not choose a product-wide default model.

Notable task-level results from `contextmesh_packet` baseline:

- `self-case-artifact-evidence`
  - `gemini-3-flash`: 1.00
  - `gemini-3.1-pro`: 1.00
  - `glm-5`: 0.33
- `self-case-real-case-policy`
  - `gemini-3-flash`: 0.67
  - `gemini-3.1-pro`: 0.67 with forbidden violations
  - `glm-5`: 0.33

Interpretation:

- The matrix proved the earlier low score on `real-case-policy` and `artifact-evidence` was partly a packet-design issue, not only a model issue.
- After adding explicit self-case claims for `real cases`, `source-bound claims`, `audit logs`, `artifacts`, and `WCEF reports`, the packet baseline improved materially on those tasks.

## SiliconFlow Qwen Matrix Findings

Tested models:

- `Qwen/Qwen3-Coder-30B-A3B-Instruct`
- `Qwen/Qwen3-30B-A3B-Instruct-2507`

Task coverage:

- 5 real self-case tasks per model
- 4 baselines per task
- 10 task reports total

Observed pattern:

- Both models are fully integrated into the current matrix workflow and can be treated as covered SiliconFlow domestic-model test objects.
- Their differing outputs expose where a packet, task wording, or assertion contract needs further scrutiny.
- Both remain valid test targets regardless of isolated-task scores.

Important scope note:

- `deepseek-ai/DeepSeek-V4-Flash` is still part of SiliconFlow test coverage, but at this stage as a self-case/probe-covered target rather than a completed matrix-covered target, because repeated direct runs currently hit upstream timeout/busy behavior.

## Interpretation Rule

The matrix is intended to answer:

- whether a model or client can consume the same governed packet and preserve required current facts
- whether the same integration degrades on stale-context correction, scope retention, architecture wording, evidence wording, or real-case policy wording
- which failure belongs to the packet, runner, assertion contract, or consuming client

The matrix does not by itself prove browser, CLI, or MCP tool integration quality.

## Grok CLI Casebook Evidence

- Provider mode: `grok-cli`, using the locally authenticated `grok` executable with model `grok-4.5`.
- Every request starts as an isolated single turn with cross-session memory, web search, plan mode, and subagents disabled. The harness stores the returned CLI JSON as `raw.json`.
- Workspace run `vermory-grok-workspace-v2` executed the first task of `101-workspace-parallel-repos`. Its no-context baseline scored `0.33`; both the ordinary summary and legacy packet baseline scored `1.00` for declared facts and isolation assertions. The corresponding acceptance artifact `vermory-grok-workspace-acceptance-v2` passed.
- Conversation run `vermory-grok-conversation-v4` executed the first chat-contract task of `201-conversation-housing-search` at `1.00`; it retained the active Seattle, budget, pet, one-bedroom, Fremont, and commute facts without workspace framing. The corresponding acceptance artifact `vermory-grok-conversation-acceptance-v2` passed.
- These runs are consumer-compatibility evidence, not a model ranking or proof that Grok completed a real coding-agent task. The harness deliberately disables tools and browser/search behavior.
