# Vermory Hypothesis Register

Status: review candidate

Date: 2026-07-11

## 1. Purpose

This register contains implementation choices that appear reasonable but have not earned permanent design authority.

Each hypothesis must remain falsifiable. Passing unit tests proves that an implementation matches the hypothesis; it does not prove that the hypothesis matches reality.

Statuses are:

```text
proposed       reasoned candidate without sufficient real evidence
testing        currently exercised by a frozen experiment
supported      evidence supports continued use, but revision remains possible
accepted       frozen for a named release contract
rejected       evidence showed the hypothesis should not continue
```

## 2. Register

### H-001: Go modular monolith

- Status: `proposed`
- Candidate: one Go codebase with HTTP, MCP, worker, and CLI roles.
- Reason: preserves clear domain boundaries without early distributed-system cost.
- Evidence needed: the first three continuity paths can share domain services without client-specific semantics leaking into the core.
- Falsifier: one role requires materially different availability, security, scaling, or deployment semantics that cannot be isolated inside the modular monolith.
- Decision gate: after the first real-client batch and failure-recovery experiment.

### H-002: PostgreSQL-native operational stack

- Status: `supported`
- Candidate: PostgreSQL plus pgvector is sufficient for the native deployment; Redis, Neo4j, Qdrant, and Elasticsearch are not default dependencies.
- Existing evidence: backend lifecycle, B01-B10, 200-record, 1,000-record, deletion, ARM64, and AMD64 tests support the retrieval substrate.
- Existing operational evidence: the opt-in schema-15 scale profile seeded 10,000 governed memories, ran 8 concurrent readers with concurrent deletion, measured search p50/p95 at 17/210 ms on this machine, and recovered after terminating one PostgreSQL backend connection. W12 then qualified the named `server-qualification-v1` profile with 550,000 governed memories, 100,000 current lexical and vector rows, 1,000,000 retained projection events, 1,000 concurrent deletions, competing tenant workers, zero final lag, zero scope leakage, and a 2.71 GB database on the reference M4 Pro machine. Current-authority snapshot bootstrap embedded 100,000 current facts rather than replaying obsolete history.
- Evidence artifact: `docs/evidence/2026-07-15-server-qualification-scale-profile.md`.
- Evidence needed: formation and sealed quality cases, long-duration growth and retention, query-planner monitoring under new data distributions, dimensional migration under backlog, and HA/PITR deployment profiles.
- Falsifier: a required constitutional behavior cannot be implemented reliably or within calibrated profiles without another default service.
- Decision gate: after the second evidence batch and first operational profile.

### H-003: Logical source-observation-memory separation

- Status: `testing`
- Candidate: preserve distinct logical concepts for source, observation, governed memory, history, projection, and delivery.
- Reason: prevents raw input, durable memory, vector state, and generated context from becoming one undifferentiated text pool.
- Experiment 0 signal: `W01-synapseloom-continuity` requires historical conversation observations to remain distinct from current repository authority; `S01-deletion-and-source-injection` requires an untrusted source instruction, governed scope, deleted target, and valid related guidance to remain behaviorally distinct.
- Evidence artifact: `artifacts/experiment-0/experiment-0-v1/report.json` and `report.md` under the same run directory.
- Evidence needed: real cases require independent provenance, formation, deletion, or delivery behavior for these concepts.
- Falsifier: two concepts remain observably identical across all first- and second-batch workflows and their separation creates only maintenance cost.
- Decision gate: after mapping both evidence batches to required operations.

### H-004: Candidate physical schema

- Status: `proposed`
- Candidate entities include tenants, continuities, bindings, sources, versions, observations, memories, revisions, evidence links, relations, events, projections, deliveries, jobs, and audits.
- Reason: this shape can express the current known lifecycle without making embeddings authoritative.
- Evidence needed: every entity must support at least one frozen query, invariant, state transition, or audit requirement.
- Falsifier: an entity has no independent behavior; required operations demand a missing boundary; or transaction consistency becomes unnecessarily complex.
- Decision gate: schema version 0.1 after discovery mapping; schema version 1 only after two materially different batches.

No table name or one-to-one mapping between logical and physical entities is accepted by this hypothesis.

### H-005: Revision-oriented updates

- Status: `testing`
- Candidate: ordinary updates append revisions and explicit change events; privacy deletion can erase protected content while retaining permitted content-free tombstones.
- Reason: supports current-state use, historical explanation, and deletion without silent overwrite.
- Experiment 0 signal: `W01-synapseloom-continuity` freezes a user correction followed by newer repository truth; `C01-device-maintenance-continuity` freezes a failed action followed by a verified correction; `S01-deletion-and-source-injection` freezes explicit deletion without requiring unrelated valid guidance to disappear.
- Evidence artifact: `artifacts/experiment-0/experiment-0-v1/report.json` and `report.md` under the same run directory.
- Evidence needed: progressive correction, conflicting sources, user reversal, archive/reactivation, and hard deletion cases.
- Falsifier: revision identity cannot express common real corrections without duplicating or fragmenting memory unnaturally.
- Decision gate: after update/conflict/deletion experiments.

### H-006: Memory-kind taxonomy

- Status: `proposed`
- Candidate seed kinds: fact, decision, constraint, preference, progress, next action, procedure, experience, risk, identity.
- Reason: these labels may help formation policy and context composition.
- Evidence needed: each retained kind changes a real policy, ranking, presentation, retention, or evaluation decision.
- Falsifier: labels are ambiguous, frequently multi-valued, require constant relabeling, or do not change behavior.
- Decision gate: after labeling the first two evidence batches independently and measuring disagreement.

The system may adopt fewer kinds, multi-label facets, structured attributes, or no fixed taxonomy.

### H-007: Retention classes

- Status: `supported` with an orthogonal representation; no separate `retention_class` column adopted.
- Decision: distinguish request-local working input, workspace continuity, conversation continuity, and thin Global Defaults by location and promotion policy; represent current reuse separately through lifecycle state and half-open temporal validity.
- Reason: same-session usefulness, continuity reuse, and cross-context defaults have different promotion and expiry risks, but W19 represented those differences without forcing one multi-purpose class label.
- Accepted evidence: W19 ran 10,000 governed memories and 320 scoped queries with 16/16 hard gates, zero scheduled/expired/archived/deleted misuse, zero Global Default pollution, and real Grok Web Chat, Grok MCP, and Codex MCP trajectories.
- Evidence artifact: `docs/evidence/2026-07-16-memory-eligibility-retention.md` and `docs/evidence/snapshots/2026-07-16-memory-eligibility-retention.json`.
- Falsifier: a real cross-deployment, privacy, legal-retention, or erasure requirement cannot be represented by continuity location, lifecycle, validity, source authority, and policy without introducing contradictory exceptions.
- Remaining boundary: this does not accept a universal compliance or deletion policy; expiry and archive remain explicitly different from deletion.

### H-008: Lifecycle state machine

- Status: `testing`
- Candidate meanings: pending, active, superseded, archived, rejected, and deleted, with conflict represented separately.
- Reason: separates proposal, current use, historical retention, rejection, and forgetting.
- Experiment 0 signal: `C01-device-maintenance-continuity` distinguishes failed, corrected, and verified action state; `S01-deletion-and-source-injection` requires a deleted target to remain unavailable while independent related guidance remains active.
- Evidence artifact: `artifacts/experiment-0/experiment-0-v1/report.json` and `report.md` under the same run directory.
- Evidence needed: every transition must correspond to a real user or source workflow; concurrent transitions must be deterministic.
- Falsifier: common correction, merge, split, temporary validity, contested state, or deletion behavior cannot be represented without exceptions.
- Decision gate: after lifecycle mutation tests and two real evidence batches.

Exact state names and transition edges are not frozen.

### H-009: Hybrid native retrieval

- Status: `testing` (`measured` on W08/W10; `production_path_integrated` on W09; full public candidate qualification on W28)
- Candidate: continuity and lifecycle filtering followed by lexical, exact structured, trigram, and pgvector candidate generation with versioned fusion and optional reranking.
- Reason: pure vector Top-K is weak for technical identifiers and cannot itself encode source authority or lifecycle.
- Existing evidence: W08 ran 24 frozen mixed-language and technical queries over 48 active memories plus proposed, superseded, deleted, cross-continuity, and cross-tenant controls using direct SiliconFlow `BAAI/bge-m3`. Active-only pgvector and exact-guarded RRF both reached Recall@K `1.0000` and MRR `0.9792`, compared with lexical Recall@K `0.6875` and MRR `0.6806`; exact identifiers remained `1.0000`. W09 then connected the active-only vector path to real MCP and Web Chat runtimes with a durable event worker, restricted-role RLS, exact lexical degradation for cursor lag and provider outage, vector reset/rebuild, native dump/restore, and real Grok consumption/writeback. All W09 scope, lifecycle, recovery, and credential hard gates passed.
- Current interpretation: the pgvector candidate path is production-path integrated but remains opt-in. W08 and the independent W10 batch show a large semantic-retrieval improvement over lexical on their frozen corpora, while the current RRF formula matches vector quality and adds latency rather than demonstrating an independent gain. W28 then qualified direct vector retrieval over all 500 public LongMemEval-S records: K10 RecallAll `0.9404`, nDCG `0.9069`, and MRR `0.9006`, versus lexical `0.7340`, `0.6918`, and `0.6974`, with 500 effective vector queries and zero degradation or scope/lifecycle violation. The candidate remains inactive and lexical remains the default.
- Evidence artifact: `docs/evidence/2026-07-15-independent-retrieval-batch.md` and its report snapshot record a fresh PostgreSQL 18 run over 39 governed records, 18 queries, 102 direct SiliconFlow `BAAI/bge-m3` requests, zero forbidden/ineligible results, and rebuild equivalence.
- Existing source-authority evidence: lexical workspace/conversation and vector retrieval now apply the same explicit origin tie-break; PostgreSQL tests prove an explicit user correction wins an equal-relevance source update without bypassing lifecycle or scope controls.
- Full public benchmark evidence: W14 executed all 500 cleaned LongMemEval-S records through 500 isolated conversation continuities and 23,867 governed session memories. Production lexical K10 measured RecallAny `0.9021`, RecallAll `0.7340`, nDCG `0.6918`, and MRR `0.6974`, below the same-text token-overlap baseline at `0.9489`, `0.8383`, `0.7983`, and `0.8119`. Multi-session RecallAll was `0.5620`. The run had zero runtime or scope failures and idempotent resume preserved score and failure hashes.
- Evidence artifact: `docs/evidence/2026-07-15-longmemeval-s-full-retrieval.md`.
- Full vector evidence: `docs/evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md` records 23,867 current vectors, exact `24,367` logical and `46,657` physical provider items, zero terminal failure, and the retained 21 partial/no-evidence K12 records.
- Evidence needed: reader-task utility from the frozen vector ranking, authority behavior on a broader conflict corpus, calibrated cost/latency thresholds, and optional hybrid or rerank comparison on a sealed or externally held corpus. The same public labels cannot be reused for undisclosed tuning and qualification.
- Falsifier: a simpler measured strategy matches quality, task success, cost, and failure behavior; or the candidate strategy cannot meet calibrated latency.
- Decision gate: after a second independent retrieval batch, calibrated latency/quality thresholds, and a production outage/fallback slice.

No ranking algorithm or weight is accepted before ablation.

### H-010: Language-aware application analyzer

- Status: `proposed`
- Candidate: Go-side normalization and token extraction preserve Chinese terms and exact technical identifiers before PostgreSQL lexical indexing.
- Reason: default PostgreSQL tokenization alone may not serve mixed Chinese technical text reliably.
- Evidence needed: Chinese and mixed-language retrieval ablation with paths, flags, errors, model names, and code symbols.
- Falsifier: database-native or another established analyzer provides better portable quality with lower maintenance.
- Decision gate: after mixed-language retrieval experiments on ARM64 and AMD64.

### H-011: Versioned semantic projection generations

- Status: `supported` for same-class generations and the named `vector_1024` to `halfvec_2560` migration; both measured alternatives remain `candidate`
- Candidate: embeddings are stored by model and projection generation so old and candidate models can coexist during migration.
- Reason: avoids coupling authoritative memory to one embedding model and supports measured cutover.
- Existing evidence: migration 15 registers active v1 and candidate v2 profiles with independent cursors and vector rows. The first rehearsal rebuilt `BAAI/bge-m3` and `BAAI/bge-large-zh-v1.5` side by side with 31 requests each, 30 rows each, zero cursor lag, unchanged v1 row count, and required-fact retrieval through both profiles. The W10 profile comparison then ran both registered profiles through the production worker, coordinator, audit, reset, and rebuild paths. Three corrected-corpus runs produced identical quality values and zero safety/lifecycle/degradation failures. v2 preserved Recall@K `1.0000` but regressed Hit@1 by `0.1111`, MRR by `0.0648`, and nDCG@K by `0.0503`, so the frozen promotion policy retained it as a candidate. W17 added a physically separate `halfvec_2560` class for `Qwen/Qwen3-Embedding-4B`, kept the active 1024 class serving through a 5,000-event backlog and immediate restart, converged both classes to the same 20,000 current IDs with zero lag, isolated candidate reset/rebuild, and completed a two-request direct-provider projection/query probe. W28 registered `siliconflow-bge-m3-1024-chunked-mean-v2` as a third explicit candidate, projected all 23,867 LongMemEval-S memories without truncation, and kept both the candidate lifecycle and incumbent active lifecycle unchanged.
- Evidence artifact: `docs/evidence/2026-07-15-retrieval-profile-migration.md`, `docs/evidence/2026-07-15-retrieval-profile-promotion-decision.md`, `docs/evidence/2026-07-16-active-backlog-dimensional-migration.md`, and `docs/evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md`.
- Current decision: keep `siliconflow-bge-m3-1024-v1` active/default; keep `siliconflow-bge-large-zh-1024-v2`, `siliconflow-qwen3-embedding-4b-2560-v3`, and `siliconflow-bge-m3-1024-chunked-mean-v2` as explicit candidates. W28 establishes full public retrieval quality for the long-input candidate, but does not supply a promotion decision by itself.
- Evidence needed: a separately frozen quality and promotion study for the 2560-dimensional candidate, plus repeated long-duration migration on another deployment profile.
- Falsifier: a simpler rebuild-and-swap mechanism is operationally sufficient for calibrated deployment profiles.
- Decision gate: passed for the current 1024-dimensional generation mechanism and the named 1024-to-2560 physical-class migration; reopen for arbitrary dimensions, another storage class, or promotion.

### H-012: PostgreSQL transactional outbox

- Status: `supported` for the current self-hosted and named server-qualification profiles
- Candidate: authoritative transactions enqueue projection and provider work through PostgreSQL, with idempotent workers and no default Redis dependency.
- Reason: aligns memory state and projection jobs without introducing a second required service.
- Existing evidence: W11 created 1,000 governed facts and projection events in a disposable PostgreSQL 18 cluster, processed a bounded 128-event batch, retained cursor position across provider failure, replayed the full event stream from cursor zero without duplicate vectors, stopped PostgreSQL with `immediate` while embedding was in flight, recovered through the same runtime pool, and proved concurrent deletion wins over late embedding. W12 created 1,000,000 real trigger events across ten tenants, collapsed them into 100,000 current vector projections, then used two competing workers per tenant to consume 1,000 deletion events with exactly ten lock winners, ten `already_running` results, zero duplicate processing, and zero final lag. W13 attribution proved every degraded vector request in the concurrent replay used `projection_lag`, while a 100,000-vector projection-current control completed 550 of 550 vector requests without degradation. W17 kept authority writes independent while a different physical projection class consumed 5,000 active-tail events, then recovered both profile cursors through an immediate restart with zero partial candidate row or cursor advance. W28 used one-memory durable passes to ensure a successful provider prefix was committed before the next operation; all 23,867 events converged with zero lag, but projection took `7,210.157s` and only about `3.31` logical memories per second. Separate W11, W12, W17, and W28 tenants completed direct SiliconFlow projection and retrieval probes.
- Evidence artifact: `docs/evidence/2026-07-15-projection-outbox-fault-profile.md`, `docs/evidence/2026-07-15-server-qualification-scale-profile.md`, `docs/evidence/2026-07-15-vector-degradation-attribution.md`, `docs/evidence/2026-07-16-active-backlog-dimensional-migration.md`, and `docs/evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md`.
- Current decision: PostgreSQL remains the default authority and transactional outbox; Redis is not a required deployment dependency for the measured developer-local, self-hosted, `server-qualification-v1`, and named dimensional-migration profiles.
- Evidence needed: long-duration arrival pressure, event-retention pruning, cross-host HA, and cross-region profiles.
- Falsifier: queue contention or operational requirements exceed calibrated profiles and an external queue produces a clearly safer design.
- Decision gate: passed for the current self-hosted, `server-qualification-v1`, and named dimensional-migration profiles; reopen for long-duration retention, cross-host HA, cross-region, or materially higher arrival pressure.

### H-013: Row-level security defense in depth

- Status: `supported` for the authenticated single-host PostgreSQL 18 multi-tenant profile
- Candidate: application authorization, tenant-bearing foreign keys, and PostgreSQL row-level security jointly protect tenant boundaries.
- Reason: query filters alone are an insufficient final barrier for a multi-tenant memory platform.
- Existing evidence: I01-I05 used real random API tokens, a real non-owner `LOGIN NOSUPERUSER NOBYPASSRLS` PostgreSQL role, RLS-aware connection pools, authenticated HTTP handlers, and two tenants resolving the same OpenClaw session key to different continuities. Deliberate queries with omitted tenant predicates saw only the transaction tenant; missing tenant context saw zero rows; cross-tenant continuity, memory, delivery, and bridge foreign-key attacks failed; revoked and expired tokens returned `401`; concurrent requests through a two-connection pool did not inherit stale tenant settings. Native dump/restore and later formation, retrieval, retention, and HA/PITR profiles preserved restricted-role validation, RLS policy inventory, filter-omission behavior, and tenant-aware foreign keys.
- Evidence artifacts: `docs/evidence/2026-07-14-identity-authorization-rls.md`, `docs/evidence/2026-07-14-postgresql-operations-recovery.md`, `docs/evidence/2026-07-14-production-retrieval-runtime.md`, `docs/evidence/2026-07-16-postgresql-ha-pitr.md`, and `docs/evidence/2026-07-18-automatic-conversation-review.md`.
- Current decision: application authorization, tenant-bearing foreign keys, transaction-local tenant context, and PostgreSQL RLS remain jointly required for the named profile. Query predicates alone do not satisfy the boundary.
- Evidence needed: independent security review, hostile connection-pool and role-misconfiguration testing beyond the named profiles, and cross-host or cross-region deployment qualification.
- Falsifier: the initial supported deployment is explicitly single-tenant and RLS creates correctness or operations problems; the multi-tenant profile would still require a separate acceptance decision.
- Decision gate: passed for the authenticated single-host PostgreSQL 18 profile; reopen for a different database role topology, cross-host tenancy, or a broader multi-tenant release claim.

### H-014: Prepare and commit client operations

- Status: `supported` for the current bounded-turn Web Chat, MCP, OpenClaw, and Hermes contracts
- Candidate: ordinary clients need one pre-task context operation and one post-task observation/write-back operation, with administrative APIs separate.
- Reason: keeps normal client integration low-friction while preserving governance.
- Existing evidence: W19 bound real Web Chat/Grok, workspace MCP/Grok, and official Codex CLI trajectories into one formal report. Each consumed currently eligible context, completed a bounded task, and wrote the result back as a proposed observation; no client output became active memory or a Global Default. W22 then ran real OpenClaw and official Hermes continuities through completed-turn scheduling, asynchronous formation, explicit review, later recall, correction, forgetting, restart, and isolation. W23 added verified tool-result completion without granting models governance tools. W27 wrote four real direct-model outputs through the production service as `agent_result`, retained all four as proposed with zero search projection rows, and replayed the writes idempotently after the optional mem0 process was removed.
- Evidence artifacts: `docs/evidence/2026-07-16-memory-eligibility-retention.md`, `docs/evidence/2026-07-18-automatic-conversation-review.md`, `docs/evidence/2026-07-18-verified-tool-outcome-formation.md`, and `docs/evidence/2026-07-19-w27-real-utility-comparison.md`.
- Current decision: ordinary bounded client turns use one prepared delivery before model work and one observed-result commit after work. Formation and governance remain asynchronous or operator-controlled and are not model tools.
- Evidence needed: streaming partial-output cancellation, multi-hour tool loops, offline/mobile replay, and clients whose lifecycle cannot be represented as a bounded prepare/complete pair.
- Falsifier: streaming, tool-loop, long-running session, or client lifecycle requires a different interaction contract.
- Decision gate: passed for the current bounded-turn Web Chat, MCP, OpenClaw, and Hermes integrations; reopen before freezing a stable public client API or claiming arbitrary streaming/agent compatibility.

Names, payloads, streaming behavior, and transport remain versioned rather than universally frozen.

### H-015: Optional backend projection adapters

- Status: `supported`
- Candidate: mem0, MemOS, and Supermemory receive only eligible search projections and can be rebuilt from PostgreSQL.
- Existing evidence: all four tested backends implement the lifecycle adapter contract and pass the current scenario set.
- Evidence needed: formation-to-projection synchronization, deletion propagation, adapter outage, and rebuild under real trajectories.
- Falsifier: an adapter cannot preserve constitutional deletion or isolation even when treated as disposable; that adapter is removed rather than weakening the constitution.
- Decision gate: independently for each adapter before supported-release status.

### H-016: Deterministic release/database compatibility preflight

- Status: `candidate`
- Candidate: each binary declares an inclusive PostgreSQL schema support interval and checks it through a restricted, read-only database function before opening a production listener.
- Reason: discovering schema drift through a later table query produces ambiguous service failures and permits unsafe assumptions about package or binary rollback.
- Evidence needed: real old/current/future schema decisions, a restricted login role that can inspect only the bounded version function, no-listener/no-write negative controls, PostgreSQL 17 local execution, PostgreSQL 18 exact-head CI, and package scripts that remain migration-free.
- Falsifier: the preflight leaks migration metadata or credentials, expands runtime authority, performs a hidden migration, permits an unsupported schema to listen, or blocks a declared-compatible schema.
- Decision gate: after `I07-release-database-compatibility` passes all hard gates on an exact protected head. Binary-only rollback remains conditional on the older binary's support interval; incompatible schema rollback uses PostgreSQL backup or PITR rather than an assumed automatic down migration.

## 3. Decision Records

When a hypothesis changes status, record:

```text
hypothesis id
old and new status
evidence batch and run ids
public and sealed result summary
baseline comparison
failure and limitation summary
decision and rationale
schema or API compatibility effect
rollback path
```

Accepted hypotheses remain revisable through a new versioned product or architecture decision. Existing evidence is never rewritten to match a later design.

## 4. Review Rule

Before implementation adds a new permanent entity, service, state, relation, provider dependency, or release metric, it must either:

- map to an existing hypothesis and its evidence gate; or
- enter this register as a new falsifiable hypothesis.

This rule prevents both speculative complexity and unrecorded simplification.
