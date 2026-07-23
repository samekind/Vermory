# Production Retrieval Ablation Evidence

Date: 2026-07-14

Evidence level: public measured corpus

## Scope

W08 compares the unchanged production lexical runtime, direct
PostgreSQL/pgvector retrieval, and deterministic exact-guarded RRF over the same
governed corpus. It measures retrieval only. It does not switch Vermory's
runtime default or treat embeddings as authority.

The public corpus contains:

```text
tenants:       4
continuities:  8
active:       48
proposed:      4
superseded:    4
deleted:       4
queries:      24
```

Queries cover exact identifiers, paths, flags, error codes, model names,
Chinese paraphrases, English paraphrases, mixed-language requests, dates,
durations, numeric constraints, multi-fact requests, and continuity isolation.
Every query freezes relevant and forbidden record IDs before execution.

## Environment

```text
macOS:       26.5.1 (25F80), darwin/arm64
Go:          1.26.5
PostgreSQL:  18.4
pgvector:    0.8.5
schema:      13
provider:    SiliconFlow direct API
base URL:    https://api.siliconflow.cn/v1
model:       BAAI/bge-m3
dimensions:  1024
```

The final binary was built with `-trimpath` from:

```text
eda70a4bcccbe760572eca72e455058af2947782
```

The API key was supplied through one transient environment variable read with
terminal echo disabled. It was not placed in arguments, files, artifacts, or
Git. Mac mini NewAPI was not used.

## Preserved Provider Compatibility Failure

The first direct embedding probe sent:

```json
{"model":"BAAI/bge-m3","input":"Vermory retrieval qualification probe","dimensions":1024}
```

SiliconFlow returned:

```text
HTTP 400
code: 20015
message: The parameter is invalid. Please check again.
response SHA-256:
1c3d63d566c947eb7b9ad5beb2d5c52af42bbd9b6a07cd39bf13b1ac178ee0d6
```

The same direct request without the optional `dimensions` field returned in
`0.188891 s`:

```text
HTTP 200
model: BAAI/bge-m3
vectors: 1
dimensions: 1024
response SHA-256:
d41f39b44b1a52767d0cae2a957fdeb1aadb78bf735869352c66a0ac32638a4e
```

This exposed a real adapter defect: Vermory treated the expected response
dimension as a universally supported request parameter. Revision `e0c74ac`
stopped sending that optional field while retaining strict response-length
validation. The unit test now rejects any fixed-dimension request that contains
`dimensions`.

## Preserved Projection Failure

The first complete run used implementation `e0c74ac` and completed all three
conditions, but failed the rebuild hard gate:

```text
run: w08-siliconflow-bge-m3-20260714-v1
hard gates: fail
ineligible results: 0
projection rebuild equivalent: false
report SHA-256:
a21cb5c17ab39a56e16f0895ae2ce7580901c7fd8f01638918cbc3f99256be7f
```

The initial ANN table contained active, proposed, and superseded rows with a
status filter at query time. Rebuild retained only active rows. Because HNSW
candidate generation precedes SQL filtering, changing the graph population
changed result IDs even though non-active rows were not returned.

The corrected projection contract is active-only:

- active governed memories are inserted;
- proposed memories are never inserted;
- superseded and deleted memories exercise projection deletion;
- every returned vector candidate is still rechecked against current
  PostgreSQL authority.

Revision `16aad38` implements this boundary. The next complete run passed
rebuild equivalence. This failure is retained because it demonstrates why
status-filtered non-active vectors are not equivalent to an active-only ANN
projection.

## Final Run

```text
run ID:
w08-siliconflow-bge-m3-20260714-v3

request fingerprint:
644c1fea9df1ce822e271d3f40851bda480d80769629a5232cec00442024d639

corpus SHA-256:
d8dc705d6a073d628dae7999d1123182e01982217aac1c65bdf98f1162e4f0a1

authority fingerprint:
5dc376455f701c2cf1083825c1640b0d1a198eaa8b578fcf96225dfbc1ba2e55

embedding requests:
152

duration:
20.682242 s

hard gates:
pass

projection rebuild equivalent:
true
```

Authoritative and projection row counts after the final rebuild were:

```text
continuities:       8
tenants:            4
observations:      60
governed memories: 60
active:            48
proposed:           4
superseded:         4
deleted:            4
lexical documents: 48
vector documents:  48
vector status:      active only
lexical - vector:   0
vector - lexical:   0
```

## Aggregate Metrics

| Condition | Hit@1 | Recall@K | MRR | nDCG@K | P50 | P95 | Forbidden | Ineligible |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `lexical_runtime` | 0.6667 | 0.6875 | 0.6806 | 0.6526 | 0.862 ms | 2.871 ms | 0 | 0 |
| `vector_pg` | 0.9583 | 1.0000 | 0.9792 | 0.9623 | 100.157 ms | 126.526 ms | 0 | 0 |
| `hybrid_rrf` | 0.9583 | 1.0000 | 0.9792 | 0.9623 | 101.199 ms | 130.134 ms | 0 | 0 |

Important cohort deltas:

| Cohort | Lexical Recall / MRR | Vector Recall / MRR | Hybrid Recall / MRR |
|---|---:|---:|---:|
| Exact identifiers | 1.0000 / 1.0000 | 1.0000 / 1.0000 | 1.0000 / 1.0000 |
| Chinese semantic | 0.0714 / 0.1429 | 1.0000 / 0.9286 | 1.0000 / 0.9286 |
| Mixed language | 0.5000 / 0.5000 | 1.0000 / 1.0000 | 1.0000 / 1.0000 |
| Semantic paraphrase | 0.8000 / 0.7333 | 1.0000 / 0.9500 | 1.0000 / 0.9500 |
| Numeric constraints | 0.6000 / 0.4667 | 1.0000 / 1.0000 | 1.0000 / 1.0000 |
| Multi-fact | 0.6875 / 0.7500 | 1.0000 / 1.0000 | 1.0000 / 1.0000 |

The only vector Hit@1 miss was `design-copy-action`: `design-mobile-rule`
ranked first and the relevant `design-copy-rule` ranked second. Recall remained
1.0 and MRR was 0.5 for that query.

## Interpretation

This corpus strongly supports a production integration experiment for an
active-only `BAAI/bge-m3` pgvector candidate path. It does not yet support the
specific RRF strategy: `hybrid_rrf` and `vector_pg` produced identical aggregate
and cohort quality, while hybrid added small local fusion overhead. The current
exact guard preserved 1.0 exact-identifier metrics, but the public corpus is too
small to freeze final weights or a default strategy.

H-009 therefore moves from `proposed` to `testing`, with status `measured`.
Vermory's current lexical runtime remains the product default. A later slice
must run another independent batch, calibrate latency and quality thresholds,
and qualify provider outage behavior before any default switch.

## Outage And Replay

`TestRunNativeVectorOutageDegradesToLexical` uses real PostgreSQL/pgvector and a
real local HTTP embedding endpoint that returns HTTP 503 for both initial and
post-rebuild query embeddings. It passed in `0.63 s` and proved:

- vector query failures remain in the failure ledger;
- completed lexical evidence is retained;
- hybrid returns the exact lexical IDs in the exact lexical order;
- `degraded_to_lexical=true` is recorded;
- authority and rebuild hard gates remain intact.

The final artifact was then replayed with the intentionally invalid database
URL `postgresql://invalid.invalid/should-not-connect`. The command returned
immediately with `replayed=true`, proving exact replay occurs above database,
deletion, and provider work.

## Artifact Hashes

```text
report.json:
088b6329ce6832356be3fb75ae625468ec413690cb17d0a48217e5ecb3abebc8

report.md:
eb605b4965066ea36efe54b399521dbdc39e86bef566cc73f3d0aaae33166371
```

The committed machine-readable snapshot is:

```text
docs/evidence/snapshots/2026-07-14-production-retrieval-ablation-report.json
```

## Non-Claims

This result is not:

- a sealed or withheld evaluation;
- a BRIGHT, LongMemEval, LoCoMo, or MemBench score;
- proof that RRF is better than vector retrieval;
- source-authority ranking across conflicting active sources;
- an embedding migration rehearsal;
- million-record latency or concurrency qualification;
- a production default switch;
- final release acceptance.
