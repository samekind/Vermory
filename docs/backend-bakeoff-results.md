# Vermory Memory Backend Bake-Off Results

> Supporting substrate evidence. The recorded results predate the Vermory rename and do not establish end-to-end memory quality.

## Decision

Vermory selects PostgreSQL with pgvector as the native retrieval substrate.
mem0, MemOS, and Supermemory remain optional adapters, not mandatory platform
dependencies.

This decision does not mean that PostgreSQL already implements the whole
Vermory authority model. The current migration is the legacy project-centric
vertical slice: it persists projects, sources, source versions, claims,
capsules, packets, audit logs, and WCEF runs. The Product Constitution assigns
PostgreSQL the future authoritative boundary for continuity bindings,
observations, governed memory, history, deletion, and delivery; that runtime
schema is intentionally not frozen or implemented yet. The native backend is a
disposable vector projection over eligible records and can be erased and
rebuilt once authoritative records exist.

## Test Environment

- Ubuntu Linux ARM64 VM, 6 CPU, 12 GB RAM, containerd
- The same `bge-m3` 1024-dimensional embedding model for every backend
- Local Ollama model runtime, excluded from backend resource comparisons
- Native PostgreSQL/pgvector, mem0 OSS, MemOS OSS, and Supermemory Local
- Source revisions and machine-readable measurements are recorded in
  `backend-casebook/results/linux-arm64-20260711.json`

Linux AMD64 was also exercised rather than accepted from documentation alone:

- Vermory built as a static x86-64 Go executable.
- mem0 built as an AMD64 image and executed Uvicorn under x86-64 emulation.
- MemOS built from its official AMD64 base image and executed Uvicorn under
  x86-64 emulation.
- Supermemory's official Linux x64 binary passed its published SHA-256 check and
  started under x86-64 emulation.

## Correctness

Every candidate passed the lifecycle hard gates:

- explicit tenant and continuity isolation
- current fact recall
- superseded fact suppression
- deletion residue check
- whole-scope reset
- rebuild equivalence from PostgreSQL-owned records

Every candidate also passed all 56 B01-B10 assertions with zero forbidden
cross-scope or stale-content leakage. Those scenarios cover parallel software
repositories, cross-coder relay, isolated housing and job conversations,
superseded decisions, deletion and paraphrased recall, source-first conflict,
chat-to-workspace noise filtering, workspace rebind/rebuild, troubleshooting
noise, and Chinese text mixed with paths, feature flags, and model identifiers.

## Load Results

The 200-record run used deliberately similar software facts. Each record had a
different `module-XXXX`, `ff_XXXX`, owner, and timeout. Twenty queries required
the exact flag and owner rather than a merely related result.

| Backend | Writes | Ingest | Identifier recall | Search P50 | Search P95 |
|---|---:|---:|---:|---:|---:|
| Native pgvector | 200/200 | 23.68 s | 20/20 | 125.9 ms | 141.4 ms |
| mem0 | 200/200 | 24.09 s | 20/20 | 135.4 ms | 139.7 ms |
| Supermemory | 200/200 | 26.17 s | 20/20 | 141.4 ms | 151.7 ms |
| MemOS | 200/200 | 35.31 s | 20/20 | 251.6 ms | 284.5 ms |

The two operational finalists were then tested with 1,000 similar records and
50 exact identifier queries.

| Backend | Writes | Ingest | Identifier recall | Search P95 | Erase 1,000-record scope |
|---|---:|---:|---:|---:|---:|
| Native pgvector | 1000/1000 | 122.10 s | 50/50 | 141.6 ms | 23.3 ms |
| mem0 | 1000/1000 | 134.83 s | 50/50 | 151.7 ms | 229.0 ms |

Native pgvector uses an HNSW cosine index plus a tenant/continuity/status index.
Its 1,000-record run retained full exact-identifier recall and stable search
latency.

## Operations

Resource figures are observed runtime values, not vendor estimates. Ollama is
common to every candidate and is excluded.

| Backend | Required runtime | Steady memory after load | Cold start | Main operational finding |
|---|---|---:|---:|---|
| Native pgvector | Existing Vermory PostgreSQL | No extra service | PostgreSQL lifecycle | Smallest failure and backup surface |
| mem0 | API plus PostgreSQL/pgvector | About 347 MiB | 6.1 s | Correct, but duplicates API/config/history responsibilities |
| Supermemory | Single local server | About 1.1 GiB | 3.2 s | Simple process model, high RAM, local single-key boundary |
| MemOS | API plus Neo4j plus Qdrant | About 1.22 GiB | 10.0 s | Richest model, heaviest operations and largest state surface |

The tests also found integration issues that are hidden by feature lists:

- mem0's server required flat `user_id` and `agent_id` filters rather than the
  nested `AND` shape first attempted from examples. Its efficient scope erase
  also requires an administrative server-side delete endpoint.
- Supermemory Local exposes `/v4/openapi`, not `/openapi.json` or `/health`, and
  requires both LLM and embedding environment variables at startup even when
  direct-memory APIs are used.
- MemOS started before Neo4j was ready under the tested compose frontend. Its
  filter-based delete path also hit a missing `pref_mem` attribute. The adapter
  avoids that path by listing the cube and deleting explicit IDs.
- MemOS default MMR dedup initially retained a superseded `/v1/orders` candidate
  over the active `/v2/orders` candidate. The adapter now applies active-state
  metadata filtering before retrieval and disables backend dedup/rerank so
  Vermory, not the index engine, controls current-state semantics.

## Why Native Wins

The target role is a rebuildable retrieval index, not a second source of truth.
All four candidates reached the same correctness score, so extra framework
features did not improve the tested Vermory behavior. Native pgvector then
won on the boundaries that remain:

- no additional service or vendor control plane
- one authorization, backup, audit, and migration boundary
- fastest whole-continuity erasure
- full ARM64 and AMD64 portability through the Go/PostgreSQL deployment
- no translation between Vermory lifecycle state and a second memory model
- straightforward index destruction and deterministic rebuild

mem0 is the first optional adapter when a deployment explicitly wants its own
memory framework. MemOS is appropriate only when graph and multi-memory
capabilities justify Neo4j and Qdrant. Supermemory is appropriate when a
single-binary local service is more important than memory footprint and the
single-key local boundary is acceptable.
