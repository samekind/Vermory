# Production Retrieval Ablation Design

Date: 2026-07-14

Status: frozen for implementation

## Goal

Vermory must measure whether vector or hybrid retrieval improves real context
selection before either becomes part of the default runtime. The comparison
must use the same governed records, tenant and continuity boundaries, lifecycle
states, query limits, and real embedding model. It must preserve exact
technical identifiers and zero-tolerance isolation while exposing every ranked
result needed to explain a score.

This slice qualifies retrieval candidates. It does not change
`SearchActiveMemory`, add embeddings to the authoritative schema, or claim that
one public corpus establishes production-wide quality.

## Decision Being Tested

Hypothesis H-009 proposes continuity and lifecycle filtering followed by exact,
lexical, trigram, and vector candidate generation with deterministic fusion.
The current production runtime already provides exact substring, PostgreSQL
full-text, and trigram retrieval. The repository also has a disposable native
PostgreSQL/pgvector backend, but it is not connected to the governed runtime.

W08 compares three conditions:

| Condition | Implementation | Product meaning |
|---|---|---|
| `lexical_runtime` | Current `runtime.Store.SearchActiveMemory` | Current production baseline. |
| `vector_pg` | Existing native PostgreSQL/pgvector backend | Scoped semantic candidate baseline. |
| `hybrid_rrf` | Exact guard plus reciprocal-rank fusion of the first two conditions | Candidate production strategy, not yet the default. |

No LLM reranker is included. Adding one before lexical/vector ablation would
mix retrieval quality, provider behavior, cost, and prompt sensitivity in one
result.

## Corpus Contract

Create one versioned public corpus under
`runtime/cases/W08-production-retrieval-ablation`. Every record and query has a
stable ID and cites one existing public casebook case as provenance. The first
corpus draws from software development, research, device troubleshooting,
household administration, cross-client relay, and explicit bridge cases. It
must not reuse the legacy Bluebridge self-case.

The corpus contains at least:

- four tenants;
- eight workspace or conversation continuities;
- forty-eight active records;
- four superseded records;
- four deleted records;
- four proposed records;
- twenty-four scored queries.

The scored query cohorts are:

- exact identifiers, paths, flags, model names, and error codes;
- Chinese and English semantic paraphrases;
- mixed Chinese/English technical requests;
- dates, versions, durations, and numeric constraints;
- multi-fact requests that require more than one relevant record;
- highly similar distractors inside and outside the authorized continuity.

Each query declares:

```json
{
  "id": "exact-release-command",
  "tenant_id": "retrieval-local",
  "scope_id": "workspace-release",
  "text": "Which locked release command should I run?",
  "limit": 6,
  "relevant_record_ids": ["release-command-current"],
  "forbidden_record_ids": [
    "release-command-superseded",
    "release-command-other-workspace",
    "release-command-other-tenant"
  ],
  "cohorts": ["semantic_paraphrase", "technical_command"]
}
```

Relevant and forbidden sets are frozen before any condition runs. The public
corpus is not sealed. An external corpus may be supplied later and must be
labelled `withheld_local` unless its expected answers are held by an external
evaluator.

## Authority And Lifecycle Setup

The runner uses a dedicated database and the actual runtime APIs to create
workspace or conversation continuities and governed memories. It must not
insert fake ranked rows directly into `memory_search_documents`.

Corpus lifecycle is materialized through existing operations:

- `source_update` creates active source facts;
- `agent_result` creates proposed memories;
- a later `source_update` supersedes a named active memory;
- `DeleteMemory` deletes and redacts a named memory;
- separate anchors create cross-continuity distractors;
- separate tenants create cross-tenant distractors.

The vector backend uses the same runtime memory IDs, but its final ANN
projection contains only active eligible records. Supersession and deletion are
replayed as projection deletion, while proposed records are never inserted.
This avoids leaving ineligible rows inside the HNSW graph, where post-scan
filtering can change recall after rebuild. The state remains disposable, and a
record cannot become eligible for vector or hybrid retrieval unless the
authoritative governed memory is active for the requested tenant and
continuity.

## Real Embedding Contract

The real W08 run uses the SiliconFlow OpenAI-compatible embeddings endpoint
directly:

```text
base URL: https://api.siliconflow.cn/v1
model: BAAI/bge-m3
dimensions: 1024
```

The API key is supplied only through a named environment variable. It is never
written to the corpus, report, command transcript, process arguments, Git
history, or runtime evidence. Mac mini NewAPI is not used.

Unit tests use a deterministic local embedding server. Those tests verify the
harness but never count as real vector-quality evidence.

## Retrieval Conditions

### Lexical Runtime

The lexical condition calls the unchanged production
`SearchActiveMemory(tenantID, continuityID, query, limit)` path. Its returned
memory IDs and order are recorded exactly.

### Vector PostgreSQL

The vector condition calls the existing native backend with the same tenant,
continuity, query text, and limit. The runner intersects every vector result
with the current active governed-memory eligibility set before scoring. A stale
or wrongly scoped vector row is therefore observable as a projection defect
but cannot become delivered context.

### Hybrid RRF

Hybrid requests the production lexical maximum of 12 candidates and requests
`max(20, limit * 4)` vector candidates capped at 100. It deduplicates by
governed memory ID and computes:

```text
rrf_score = 1 / (60 + lexical_rank) + 1 / (60 + vector_rank)
```

A missing rank contributes zero. Exact case-insensitive query substrings in an
active record form an exact guard and sort before non-exact records. Remaining
ties sort by higher RRF score, then lexical rank, then vector rank, then stable
memory ID. The final list is truncated to the query limit.

The formula and tie-break rules are versioned in every report. W08 does not tune
weights after seeing individual query answers.

## Failure And Degradation Rules

- Lexical failure fails the run because it is the production baseline.
- Embedding or vector failure marks `vector_pg` unavailable for that query.
- Hybrid then returns the lexical IDs in exactly the same order and marks the
  query `degraded_to_lexical`.
- Provider failure cannot change authoritative memory or lexical projection.
- One failed query does not erase completed query evidence.
- A replay with the same run ID, corpus hash, embedding profile, code revision,
  and database authority fingerprint returns the existing report.
- A conflicting replay is rejected.

## Metrics

For every condition and cohort, record:

- `hit_at_1`;
- `recall_at_k`;
- mean reciprocal rank;
- binary `ndcg_at_k`;
- forbidden-result count;
- stale, deleted, proposed, cross-continuity, and cross-tenant violations;
- query p50 and p95 latency;
- embedding request count;
- returned-result count.

The report contains each query's expected IDs, ranked result IDs, ranks,
component scores, eligibility decision, degradation status, and duration. It
may contain public corpus text, but never provider credentials or private local
corpus content.

## Hard Gates And Calibration

These are fixed hard gates for every condition:

- cross-tenant violations equal zero;
- cross-continuity violations equal zero;
- superseded, deleted, and proposed violations equal zero;
- vector and hybrid results are rechecked against PostgreSQL authority;
- embedding outage leaves lexical results unchanged;
- deleting and rebuilding the vector projection preserves the vector and
  hybrid result IDs for the frozen run;
- report and corpus hashes reproduce on replay.

W08 does not preselect a product-quality winning threshold. It measures the
first public corpus, reports exact deltas by cohort, and records whether the
candidate is `measured`, `hard_gate_failed`, or `ready_for_threshold_review`.
Only a subsequent decision record may freeze minimum quality and latency
targets for production integration. A green W08 run therefore cannot silently
switch the runtime default.

## CLI And Artifacts

Add:

```text
vermory retrieval-ablation
```

Required flags:

```text
--database-url
--corpus
--run-id
--output-dir
--embedding-base-url
--embedding-api-key-env
--embedding-model
--embedding-dimensions
```

Stable outputs:

```text
report.json
report.md
```

The JSON report is the machine-readable authority for metrics. Markdown is a
deterministic rendering of the same result. Both include corpus SHA-256, Git
revision, schema version, embedding profile, engine version, authority
fingerprint, query count, per-condition metrics, per-cohort metrics, failures,
and explicit non-claims.

## Verification

The implementation must pass:

- corpus schema and provenance validation;
- deterministic metric and RRF unit tests;
- lifecycle and scope hard-gate tests against PostgreSQL;
- vector-outage exact fallback tests;
- vector projection delete/rebuild equivalence;
- serial database integration tests;
- race tests for the new package and command;
- full existing Go, vet, module, release, OpenClaw, migration, RLS, and clean
  diff gates;
- one real direct SiliconFlow `BAAI/bge-m3` execution on a dedicated database;
- downloaded protected-CI artifact verification for the final head.

## Non-Claims

W08 does not prove:

- a full BRIGHT, LongMemEval, LoCoMo, MemBench, or other benchmark score;
- source-authority ranking across conflicting active sources;
- embedding-model migration or zero-downtime index generations;
- million-record latency or concurrency qualification;
- reranker quality;
- automatic formation quality;
- sealed generalization;
- final release readiness.

Its result decides only whether the measured retrieval candidates deserve a
separate production-integration slice.
