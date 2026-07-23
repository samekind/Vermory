# Vermory Runtime Readiness Research

Date: 2026-07-13
Repository snapshot: `agent/grok-cli-runtime` at `7f4fb5e`
Scope: product contract, repository evidence, client transport, benchmark boundaries, and the next evidence-producing runtime slice.

## 1. Decision Summary

Vermory is a governed continuity and memory platform for AI clients. It is not a generic RAG wrapper, model leaderboard, project-import utility, or a frontend for mem0, MemOS, or another memory framework.

```text
AI client or harness
-> resolve the correct continuity boundary
-> retrieve governed, current, bounded context
-> AI and human perform the task
-> capture outcome, correction, source change, or deletion as an observation
-> govern the observation into current history, a proposal, or forgetting
-> reuse only inside the correct future boundary
```

The first runtime transport should be a local MCP `stdio` server for a coding harness. It must prove a real workspace task before the project adds a broad HTTP API, dashboard, browser extension, or provider matrix. MCP gives Vermory callable tools; it does not itself guarantee a harness will invoke them. The first acceptance therefore requires preserved pre-task and post-task tool calls, not a claim of automatic integration.

OpenClaw is the first-class everyday-assistant adapter after that local slice. Its official plugin hooks can inject bounded context before prompt construction and observe a completed turn after execution. Vermory integrates with OpenClaw's session/channel lifecycle; it does not replace OpenClaw's Gateway, agent runtime, channel routing, or memory implementation.

## 2. Product Contract That Is Fixed

The [Product Constitution](../superpowers/specs/2026-07-11-vermory-product-constitution.md) is the authority for product behavior. It fixes the following semantics while deliberately leaving physical tables and ranking algorithms open to evidence.

| Contract | Required behavior |
|---|---|
| Workspace-backed continuity | Stable workspace anchors are shared across supported coding clients, isolated from other workspaces, and never auto-merged when ambiguous. Rename, migration, worktree, and mirror handling use visible adopt/rebind operations. |
| Conversation-backed continuity | Threads, channels, contacts, topics, and named matters can continue work, but similarity never silently merges matters. Cross-channel connection is conservative and user-visible. |
| Global Defaults | A thin, explicit preference/default layer. Temporary instructions, emotions, errands, and workspace facts cannot silently become global. |
| Bridges | Promote, link, export, adopt, rebind, split, and merge are governed actions with provenance. They are not implicit pooling. |
| Authority | PostgreSQL is the native authority boundary. Embeddings, lexical indexes, caches, generated context, and optional backends are disposable projections. LLM output can propose but cannot grant itself authority. |
| Forgetting | A deleted target must not reappear through exact, paraphrased, semantic, cached, historical-default, or optional-adapter retrieval. |

The zero-tolerance invariants are also fixed: no forbidden tenant or continuity leakage, no wrong strong-anchor merge, no stale fact presented as current, no deletion residue, no inference overriding explicit intent or live authoritative source, deterministic rebuild, idempotent event handling, and no provider failure bypassing governance.

## 3. Verified Repository Reality

The repository has useful, tested foundations. It does not yet contain the governed runtime described above. A passing mock, fixture, or packet run proves only its narrow behavior.

| Area | Actual state | Evidence level |
|---|---|---|
| Product boundary and validation method | Constitution, Reality Program, hypothesis register, vertical experiment plan, and four frozen reality cases exist. | Documented and fixture-validated. |
| Reality-case integrity | Authorization metadata, hashes, fixture locks, path containment, mutation detection, `withheld_local` labels, and Ed25519 attestation verification exist. | Implemented and tested. Experiment 0 validates evidence assets only. |
| PostgreSQL migration | `projects`, `sources`, `source_versions`, `claims`, `capsules`, `packets`, `audit_logs`, and `wcef_runs` exist. | Implemented legacy vertical slice. It is project-centric. |
| Native retrieval substrate | PostgreSQL/pgvector plus mem0, MemOS, and Supermemory adapters implement the current lifecycle/rebuild contract. | Historical bake-off evidence and adapter tests. |
| Workspace resolver | Pure resolver handles explicit IDs and supplied candidates. With only a repo root it derives `workspace:<basename>`. | Prototype, unit-tested only; no durable registry, Git identity, worktree, alias, migration transaction, or client integration. |
| Conversation resolver | Pure resolver derives `thread:<channel>:<thread>` or `topic:<channel>:<topic>`. Suggested links are returned. | Prototype, unit-tested only; no persistent/confirmed link relation. |
| Governance and bridge helpers | Draft/confirm/archive/delete functions plus in-memory promote/link/export/adopt/rebind builders exist. | Prototype, unit-tested only; no durable observation, authorization, audit transaction, rollback, or client propagation. |
| Casebook evaluator | Sixteen local scenarios, deterministic phrase checks, artifact persistence, mock Internal Ready chain, and a one-prompt chat-contract runner exist. | Executable local proxy. The conversation runner concatenates history and continuity into one provider prompt; it is not a persistent Web Chat service or write-back loop. |
| Direct provider harness | Direct SiliconFlow, Duojie, OpenAI-compatible, mock, and local Grok CLI paths capture artifacts. Gemini CLI is not an active client target. | Provider-connectivity and packet-consumption evidence only. |
| Real consumer evidence | Grok `grok-4.5` consumed workspace packet `vermory-grok-workspace-v2` and conversation packet `vermory-grok-conversation-v4`; their declared deterministic acceptance reports passed. | Real but narrow. Each call was isolated with memory, web search, planning, tools, and subagents disabled. It is not a coding-agent task, Codex integration, MCP integration, or write-back proof. |
| MCP, HTTP server, Codex adapter, OpenClaw adapter, Web Chat service, review UI | No server or adapter is present. | Absent. |

The previous backend document claimed that PostgreSQL already owned continuity identity, lifecycle state, bridge records, and the full audit history. The migration does not contain those concepts. This research corrects the statement: PostgreSQL is the selected future authority boundary, while the current migration remains a legacy project-centric slice.

## 4. What Current Tests Establish

### Reality Program

The four public cases are a legitimate evidence bootstrap, not a runtime pass:

| Case | Frozen pressure | Current validation |
|---|---|---|
| `W01-synapseloom-continuity` | Current repository truth must beat historical session assertions. | Fixture integrity and stated expectation. |
| `C01-device-maintenance-continuity` | Long conversation correction, safe follow-up, and local exclusions. | Fixture integrity and stated expectation. |
| `G01-language-default-local-override` | Stable Chinese default survives a task-local English override. | Fixture integrity and stated expectation. |
| `S01-deletion-and-source-injection` | Deleted secret never returns; untrusted source text cannot alter policy. | Fixture integrity and stated expectation. |

`Experiment 0` explicitly records that it does not execute a production memory implementation, a real client path, full-history, ordinary vector RAG, mem0, or Vermory against those cases. Its `pass=true` means source and case contracts are frozen and valid.

### Backend Bake-Off

The [backend bake-off](../backend-bakeoff-results.md) is solid substrate evidence. On its recorded Linux ARM64 environment, native PostgreSQL/pgvector, mem0, MemOS, and Supermemory passed the lifecycle gates and 56 B01-B10 assertions. Native pgvector was selected because it had the smallest operational state surface and fastest tested scope erasure.

This supports these decisions:

1. PostgreSQL plus pgvector is the default retrieval substrate.
2. Optional backends are projections, never semantic authorities.
3. Lifecycle filtering belongs to Vermory before relevance ranking.

It does not establish formation quality, durable continuity resolution, actual context utility, cross-client reuse, OpenClaw/Codex integration, sealed quality, or release scale. [pgvector](https://github.com/pgvector/pgvector/tree/a6420355c5d1c08f4c5fbd5112fc17e4cf3b5eb5) documents that approximate filtering occurs after the ANN scan and can reduce recall. Its iterative scan and tenant partitioning guidance is evidence for measuring scope-aware retrieval rather than treating vector Top-K as truth.

### Casebook and Provider Evidence

The casebook is valuable regression infrastructure, but it is a translated local proxy:

- `EvalCasebook` uses the first task only.
- Workspace and bridge runs inject a static packet into one provider request.
- The chat-contract runner is one concatenated prompt of supplied history, continuity view, and current turn.
- Bridge functions return in-memory results; the evaluator does not persist a bridge operation.
- `internal-ready` proves local proxy artifacts can be generated and scored. It is not a product-readiness gate.

SiliconFlow and Duojie models remain compatibility-test objects, not a product-wide ranking. Their failures and DeepSeek-V4-Flash timeout/busy behavior must remain retained in artifacts. The real Grok packet tests likewise show only bounded packet consumption.

## 5. External Runtime Research

### MCP

The official [MCP transport specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) supports two relevant transports.

| Transport | Verified behavior | Vermory decision |
|---|---|---|
| `stdio` | The client launches the server. JSON-RPC messages are newline-delimited over stdin/stdout. The server must emit only valid MCP messages on stdout; diagnostics go to stderr. | First runtime transport for local coding harnesses. |
| Streamable HTTP | Remote transport. The server must validate `Origin`; local deployments should bind to `127.0.0.1`, and exposed services need authentication. | Second transport for Web Chat, OpenClaw, multi-device, or remote deployment. Do not start here. |

The first MCP service must not print ordinary logs to stdout. It must also separate normal task operations from administrative mutations.

### OpenClaw

OpenClaw is a multi-platform personal-assistant gateway with sessions, channels, plugins, agent runs, tools, and its own memory facilities. Upstream source was inspected at [`3c30634`](https://github.com/openclaw/openclaw/tree/3c30634de45e95f22af70a0c8137b9a04256497b).

Its official [plugin hooks](https://github.com/openclaw/openclaw/blob/3c30634de45e95f22af70a0c8137b9a04256497b/docs/plugins/hooks.md) provide the needed integration boundary:

- `before_prompt_build` appends bounded task context before the model call.
- `agent_end` observes final messages, outcome, and duration after a turn.
- `message_received`, `session_start`, `session_end`, `before_compaction`, and `after_compaction` expose conversation lifecycle boundaries.
- Hook context can include `sessionKey`, `agentId`, `runId`, `channel`, `channelId`, `senderId`, `chatId`, and channel-owned identity fields when provided.

Vermory should use session/channel identity as a candidate conversation anchor, request context before prompt construction, and submit an observation after the run. It must preserve the host trust boundary: channel metadata is not automatically proof of a global user identity, and semantic similarity does not justify cross-channel linking.

The adapter must not replace OpenClaw's Gateway, expose raw Vermory audit metadata in model prose, or make OpenClaw memory authoritative. It should be pinned to an upstream commit and tested against the actual hook lifecycle.

### Backend Boundary

Current upstream references confirm the product decision:

- [mem0](https://github.com/mem0ai/mem0/tree/17836748d7afe0521516c6a73c6a256680f05527) includes formation, retrieval, user/agent scopes, graph, and hybrid options. It can be an adapter or deployment choice but cannot own Vermory anchors, authority, revision, deletion policy, bridges, or delivery ledger.
- [MemOS](https://github.com/MemTensor/MemOS/tree/13fbd43743b3b04b8c105a485f6c97729d26f776) was verified as the exact adapter revision. Its self-hosted service uses Neo4j and Qdrant; its richer graph/memory functionality is optional, not a reason to create a second authority database.

The native runtime needs no default Redis. If background projection work becomes necessary, the candidate is a PostgreSQL transactional outbox with idempotent workers; this remains hypothesis `H-012` until failure tests prove it.

## 6. Public Benchmark Matrix

The repository currently has 11 names in `casebook/benchmarks/public-benchmark-map.json`. All current entries are local translated mappings or design mappings. None is evidence that Vermory has run the original upstream dataset.

| Benchmark | Official source and actual target | Valid Vermory use | Current state |
|---|---|---|---|
| LoCoMo | [`snap-research/locomo`](https://github.com/snap-research/locomo/tree/3eb6f2c585f5e1699204e3c3bdf7adc5c28cb376): long two-speaker conversations, QA and event summarization. | Conversation recall, multi-session reasoning, temporal evidence, session retrieval. | Local conversation cases are translated proxies; original not run. |
| LongMemEval | [`xiaowu0162/LongMemEval`](https://github.com/xiaowu0162/LongMemEval/tree/9e0b455f4ef0e2ab8f2e582289761153549043fc): 500 questions for extraction, multi-session reasoning, knowledge updates, temporal reasoning, abstention. | Conversation memory, update-over-stale, time reasoning, abstention. It does not test workspace identity. | Local workspace/conversation mapping is a proxy; original not run. |
| MemBench | [ACL 2025 paper](https://aclanthology.org/2025.findings-acl.989.pdf): multi-scenario, multi-level, multi-metric LLM-agent memory evaluation. | Formation/retrieval pressure, knowledge update, preference, temporal memory. | Design mapping only. This audit confirmed the paper but no canonical upstream executable repository. |
| EverMemBench | [`EverMind-AI/EverMemBench`](https://github.com/EverMind-AI/EverMemBench/tree/e10b3d52f0e4cfc5c124ad406b5d95c59c73738b): multi-person/group conversations with Add -> Search -> Answer -> Evaluate. | Speaker/group attribution, cross-topic interference, user-profile update. | Original pipeline available but not run; local bridge mapping is only analogy. |
| BEAM | [`mohammadtavakoli78/BEAM`](https://github.com/mohammadtavakoli78/BEAM/tree/3e12035532eb85768f1a7cd779832b650c4b2ef9): 100 conversations, 2,000 questions, 128K to 10M tokens, ten memory abilities. | Long-context pressure, contradiction resolution, event order, update, preference, bounded summary. | Original pipeline available but not run; export/packet case is a proxy. |
| ARES | [`stanford-futuredata/ARES`](https://github.com/stanford-futuredata/ARES/tree/c7c9018a755faf8347c4da415632bae1593ef104): RAG evaluator for context relevance, answer faithfulness, answer relevance. | Score a delivery view after retrieval. | Not a continuity benchmark; not run. |
| RAGBench | [`rungalileo/ragbench`](https://github.com/rungalileo/ragbench/tree/c28e6c22fc858086468eabb274250e27b5a8e9d8): RAG evaluation metrics across component datasets. | Delivery evidence use, faithfulness, completeness, context utilization. | Not a continuity benchmark; not run. |
| CORAL | [`RUC-NLPIR/CORAL`](https://github.com/RUC-NLPIR/CORAL/tree/9b2b616984b3f3e82658085cce814c8f838cbfda): conversational RAG retrieval, generation, citation labeling. | Multi-turn delivery quality and cited source support after correct conversation resolution. | Original pipeline available but not run. It cannot validate conservative linking by itself. |
| BRIGHT | [`xlang-ai/BRIGHT`](https://github.com/xlang-ai/BRIGHT/tree/d99e8391d967d4c2b3a74732530d2309e2fc92b6): reasoning-intensive retrieval over 12 domains including code. | Lexical/vector/hybrid ablation for difficult technical queries. | Original pipeline available but not run. It does not model continuity or formation. |
| HaluEval | [`RUCAIBox/HaluEval`](https://github.com/RUCAIBox/HaluEval/tree/b7253db3cdaa0ab2c382f92b26b390109174f77e): 35K hallucination examples for QA, dialogue, summarization, general queries. | Output-groundedness guard after context delivery. | Original pipeline available but not run. It cannot prove storage or deletion. |
| Mu-SHROOM | [official SemEval task](https://helsinki-nlp.github.io/shroom/2025) and [`Helsinki-NLP/shroom`](https://github.com/Helsinki-NLP/shroom/tree/ce3aea0842d8bda8910d9f681b16824036b8edd4): multilingual hallucination-span detection including Chinese. | Chinese/multilingual unsupported-generation analysis after delivery. | Original scorer/data not run. It is not a continuity benchmark. |

Future reports must classify every row as one of `original_executed`, `translated_proxy_executed`, `inspired_case_executed`, `design_mapping`, or `unsupported`. Original-dataset results and local casebook results must be reported separately. No aggregate benchmark-coverage count can blur the categories.

## 7. Required Authority Model Before a Runtime Schema

These are logical concepts, not a premature one-table-per-noun prescription:

1. **Principal and scope**: user/tenant and authorization boundary.
2. **Continuity space**: workspace, conversation, or global-default boundary with current state.
3. **Anchor binding**: candidate/confirmed/ambiguous binding plus alias, rebind, and adoption history.
4. **Source and version**: document, repository, conversation event, tool result, or user assertion with revision and authority metadata.
5. **Observation**: immutable captured event/spans from a client or source. Observation is not automatically memory.
6. **Governed memory and revision history**: reusable item, eligibility, evidence links, authority, validity, correction, conflict state.
7. **Projection generation**: lexical/vector/adapter materialization that can be deleted and rebuilt without semantic loss.
8. **Delivery ledger**: bounded context sent to a consumer/task, with rationale and budget outside normal model prose.
9. **Idempotent operation and deletion work**: duplicate control, propagation, deletion verification, and permitted content-free audit evidence.

The old `projects + claims` model cannot be stretched into these behaviors by adding labels. It remains useful source/version and artifact work, but runtime needs continuity-centric authority before it can offer automatic reuse.

## 8. Initial MCP Contract

The following are candidate names, not a frozen API. Normal work needs two calls; ambiguity and governance remain explicit.

| Candidate operation | Minimum input | Result and invariant |
|---|---|---|
| `prepare_context` | client identity, operation ID, workspace or conversation anchors, task intent, budget | Resolve or abstain; return bounded semantic context plus delivery ID. Never invent a binding. |
| `commit_observation` | delivery/operation ID, task outcome, source/artifact refs, correction/update, optional candidate content | Idempotently record an observation and return `accepted`, `proposed`, `requires_confirmation`, or `rejected`. Task success never silently promotes model text. |
| `resolve_continuity` | candidate anchors and optional explicit binding | Confirm, abstain, or expose visible candidates. |
| `govern_memory` | proposal ID plus confirmation/rejection/correction/forget action | Apply lifecycle policy, emit audit evidence, schedule projection work. |

For Codex, a minimal harness rule or task protocol may request `prepare_context` before repository work and `commit_observation` after it. The acceptance artifact must show both calls. The system cannot call that automatic attachment until an actual lifecycle integration invokes it without model discretion.

## 9. First True Runtime Slice

The next implementation is a Codex workspace vertical slice, not a final schema and not an all-client platform.

```text
real repository and task
-> Codex invokes local Vermory MCP stdio
-> durable workspace binding resolution or explicit abstention
-> governed current context retrieval
-> Codex completes a real repository action
-> source/result and task outcome become an observation
-> governed proposal or current-memory update
-> repository fact changes or user correction
-> next task retrieves current state, not stale state
-> explicit deletion and re-query
-> retained artifacts and baseline comparison
```

Required scope:

- One real repository with a stable anchor and a conflicting workspace distractor.
- One actual coding task with a verifiable patch, test, command output, or reviewable repository artifact. A prose-only answer is insufficient.
- One current source fact, one stale fact, one exact technical identifier, and one correction or source revision.
- One post-task observation write-back and later retrieval from the same workspace.
- One explicit forget request with exact and paraphrased probes after deletion.
- Native PostgreSQL authority plus a rebuildable lexical/vector projection. mem0 is an optional comparison, never a required dependency.
- Retained raw client/tool artifacts with secrets and private paths redacted before storage.

The same task/model/tool policy must run under no context, full history/source dump where feasible, static summary, scoped vector retrieval, Vermory governed context, and optional mem0 where a fair lifecycle boundary is possible. The report includes raw case counts, correction burden, token cost, task success, stale use, leakage, deletion residue, and abstention reasons.

| Gate | Pass condition |
|---|---|
| Real transport | Codex connects to/launches MCP and preserved artifacts show both pre-task retrieval and post-task observation calls. |
| Strong-anchor safety | Ambiguous candidates abstain; distractor workspace never contaminates retrieval. |
| Current-state correctness | Corrected/new source fact is delivered; stale fact is not current. |
| Technical recall | Paths, flags, identifiers, commands, numbers, Chinese and English are checked deterministically, not only by LLM judge. |
| Governance | Model/tool result is observation or proposal, never automatically authority. |
| Forgetting | Exact, paraphrased, semantic/related, cache, and rebuilt-projection probes do not recover deleted content. |
| Rebuild | Projection rebuild from PostgreSQL preserves active assertions and never revives deleted/superseded material. |
| Downstream utility | Repository artifact is checked. Vermory shows better task success than simple baseline or materially stronger zero-tolerance governance at comparable utility. |
| Evidence integrity | Inputs, anchors, delivery, output, write-back, revision, score, failures, model/provider version, and environment are retained. Failed cases remain. |

Passing this slice does not complete Experiment 1 or qualify Vermory as a platform. Experiment 1 exits only after the same authority model reaches a real Web Chat/API conversation path and a real Global Defaults path. OpenClaw then tests automatic everyday attachment, multichannel identity, and longitudinal write-back. Bridges, operational profiles, RLS, embedding migration, optional adapters, and original benchmark runs follow as their evidence gates require.

## 10. Implementation Order and Non-Goals

| Order | Evidence-producing work | Do not do first |
|---:|---|---|
| 1 | Freeze the real Codex trajectory, baselines, forbidden facts, artifacts, and withheld/sealed boundary. | Add every final table or arbitrary taxonomy. |
| 2 | Implement the smallest PostgreSQL authority core, durable binding registry, observation/write-back, lifecycle filter, rebuildable projection. | Make another framework authoritative or add Redis by default. |
| 3 | Expose MCP `stdio` tools and replay the real Codex task with correction and deletion. | Call packet consumption an agent integration. |
| 4 | Build actual persistent Web Chat/API behavior, then execute frozen conversation and global-default cases. | Treat the current one-prompt chat runner as Web Chat. |
| 5 | Build pinned OpenClaw plugin using prompt/completion hooks and test multichannel linking. | Replace OpenClaw Gateway or merge by similarity. |
| 6 | Add Streamable HTTP, review surfaces, bridges, operational profiles, RLS, migration, optional adapters, and original benchmarks as evidence gates require. | Claim release scale from synthetic records or benchmark-name coverage. |

The governing discipline remains: real failure/workflow -> frozen expected and forbidden behavior -> smallest end-to-end hypothesis -> real client -> baseline comparison -> preserved failure -> keep, revise, or reject the hypothesis.

## 11. Source Record

Primary internal sources:

- [Product Constitution](../superpowers/specs/2026-07-11-vermory-product-constitution.md)
- [Reality Program](../superpowers/specs/2026-07-11-vermory-reality-program.md)
- [Hypothesis Register](../superpowers/specs/2026-07-11-vermory-hypothesis-register.md)
- [Vertical Experiment Plan](../superpowers/specs/2026-07-11-vermory-vertical-experiment-plan.md)
- [Backend Bake-Off Results](../backend-bakeoff-results.md)
- [`internal/reality/experiment0.go`](../../internal/reality/experiment0.go)
- [`internal/resolver/resolver.go`](../../internal/resolver/resolver.go)
- [`internal/bridge/bridge.go`](../../internal/bridge/bridge.go)
- [`internal/governance/service.go`](../../internal/governance/service.go)
- [`internal/store/postgres/migrations/00001_initial.sql`](../../internal/store/postgres/migrations/00001_initial.sql)
- [`internal/runner/chat_contract_runner.go`](../../internal/runner/chat_contract_runner.go)
- [`internal/app/eval_casebook.go`](../../internal/app/eval_casebook.go)

External sources were checked on 2026-07-13. Pinned repository commits and official dataset/paper links appear above. External benchmark implementations are references for future controlled runs, not evidence that Vermory has already achieved their scores.
