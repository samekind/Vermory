# Vermory Reality Program

Status: review candidate

Date: 2026-07-11

## 1. Purpose

The Reality Program prevents Vermory from being validated only inside scenarios designed around its current implementation.

It defines how real workflows become evidence, how expected and forbidden behavior is frozen before implementation, how baselines are compared, how sealed evaluation works, and how quality or scale thresholds become justified.

The program does not dictate the final database schema or retrieval algorithm.

## 2. Evidence Principles

- Real failures and real workflows precede implementation claims.
- One prompt with one expected phrase is not a complete trajectory.
- Public cases support reproducibility; sealed cases test generalization.
- Official benchmark results and translated local cases are reported separately.
- Synthetic data supports security, privacy, mutation, and load testing; it does not prove real memory quality.
- Deterministic assertions decide hard facts whenever possible.
- LLM judges are secondary and cannot override isolation, deletion, source, or task-artifact assertions.
- Failed cases remain in evidence. They are not removed to improve a headline score.
- The same generated output cannot be the sole origin of a task, its expected answer, and its success judgment.

## 3. Real Trajectory Definition

A qualifying trajectory contains:

- an authorized source or reproducible public source;
- a continuity anchor or an intentionally ambiguous anchor;
- multiple events or source versions;
- at least one realistic distractor, correction, branch, or state change where relevant;
- an explicit downstream task;
- expected useful behavior;
- forbidden behavior;
- evidence needed to determine task success;
- anonymization and fixture hashes where local data is used.

Examples include a repository changing decisions over several sessions, a long-running conversation changing its shortlist, a temporary preference that must not become global, or a deleted fact tested through several reformulations.

## 4. Evidence Sources

Priority sources are:

1. Authorized real local workflows across varied projects and domains.
2. Real Codex, Grok, cursor-agent, domestic coding-tool, Web Chat, OpenClaw, Hermes, and everyday-assistant trajectories. `cursor-agent` is a real external-client target and is not interchangeable with editor implementation delegation. Gemini CLI is retired and is not an active client target.
3. Authorized long-running conversation matters with privacy-safe anonymization.
4. Public repositories, issues, pull requests, releases, documentation, and migrations.
5. Official or verified public benchmark datasets where licensing permits.
6. Synthetic mutations, secrets, attacks, and load records where real data would be unsafe or insufficient.

No single repository, client, model family, or domain may dominate the quality corpus.

When one client is unavailable, another client may execute a separate trajectory
against the same platform contract. The unavailable-client failure remains in
the evidence, and the substitute result must name its own client, version,
permissions, and interaction boundary. A cursor-agent pass therefore cannot be
reported as a Grok CLI pass, and an implementation delegation report is not a
cursor-agent runtime qualification.

## 5. Initial Discovery Batch

The first batch is deliberately small enough to begin implementation and broad enough to challenge one shared memory engine.

The target composition is:

| Area | Target seed trajectories | Required pressure |
|---|---:|---|
| Workspace-backed continuity | 3 | cross-client continuation, source conflict, path or identity change |
| Conversation-backed continuity | 3 | topic ambiguity, correction over time, cross-channel or thread behavior |
| Global defaults | 3 paired positive/negative sets | stable default versus temporary instruction or personal event |
| Bridges | 2 | at least one promotion or link and one rebind, split, or export |
| Security and forgetting | 2 | poisoning or scope attack plus exact and paraphrased deletion checks |

These counts are discovery targets, not immutable release requirements. A trajectory may cover multiple pressures. A batch is sufficient when each constitutional invariant needed by the first experiment has at least one meaningful positive and negative case.

After the first experiment, the corpus expands in evidence batches. Schema and policy are not frozen as version 1 until at least two materially different batches have exercised all three continuity lines.

## 6. Case Freezing

Before implementation for a case begins, freeze:

```text
input events and source snapshots
continuity anchors available to the client
authorized context boundary
expected current facts
forbidden facts and behaviors
downstream task
artifact-based success checks
baseline configuration
fixture hash
```

Changing an expected result after seeing implementation output requires a recorded case revision and rationale. The original result remains in history.

Implementation-specific strings, IDs, paths, or ranking positions are not valid expectations unless the real task intrinsically requires them.

## 7. Sealed Evaluation

A directory named `hidden` inside the coding workspace is not a blind test.

Sealed cases are maintained outside the implementation-readable workspace or through a service that exposes only an evaluation contract.

The sealed runner accepts:

- a versioned Vermory endpoint or binary;
- a provider configuration class without exposing credentials;
- a case-suite version;
- an artifact destination controlled by the evaluator.

It returns:

- aggregate and per-capability counts;
- hard-gate pass or fail;
- failure category and minimal non-answer-leaking diagnostics;
- signed or hashed run identity;
- full details only through an explicit review release process.

The coding agent and implementation process must not have read access to sealed expected outputs. This requires a separate service, account, or execution boundary that is not available to the implementation session. When a sealed failure is promoted into the public regression suite, a replacement sealed case is added first.

Before that boundary exists, an owner-controlled directory outside the repository may be used only as a `withheld_local` holdout. It is not reported as blind or sealed if the implementation process could technically read it.

Historical external results use `reality/schema/attestation.schema.json`.
New external runs use
`reality/schema/external-evaluation-submission-v1.schema.json` and
`reality/schema/attestation-v2.schema.json`. The public submission binds an
exact immutable artifact, protocol, suite profile, validity interval, declared
interfaces, runtime platforms, and evaluator-controlled execution boundary.
The version-2 Ed25519 attestation additionally binds the evaluator key,
submission digest, implementation digest, run identity, aggregate counts,
named hard-gate results, and evaluator-owned detailed-result digest. The
repository provides submission creation and verification plus attestation
verification only; it does not provide a command that signs a local result or
upgrades readable evidence to sealed evidence. See the
[external evaluator handoff](../../integrations/external-evaluator.md).

Experiment 0 may complete while a genuine external sealed evaluator is unavailable. In that state, the readout must report sealed infrastructure as unavailable and retain the limitation explicitly; it must not substitute `withheld_local` or a repository directory for sealed evidence.

## 8. Baselines

Each qualifying downstream task uses relevant comparable baselines:

```text
no historical context
full allowed history
plain summary
plain retrieval or vector RAG
mem0 OSS where the use case fits
Vermory native
```

Optional adapters and additional memory systems are included only when their claimed capability is relevant. The matrix is not required to include every backend in every task.

Provider, prompt, context budget, source access, tool permissions, and downstream task are held comparable. Any unavoidable difference is reported.

The comparison asks:

- Did the task succeed?
- Which current facts were used or missed?
- Was stale or forbidden information used?
- How much context was consumed?
- How many corrections did the user need?
- Did the memory system create new errors or unnecessary friction?

## 9. Public Benchmark Registry

Public benchmarks are registered by capability and evidence level. Inclusion in the registry does not require immediate execution and does not make a benchmark a release gate.

Candidate mappings include:

| Benchmark | Candidate capability |
|---|---|
| LoCoMo | long multi-session conversation memory and temporal continuation |
| LongMemEval | long-term update, stale override, abstention, and integration |
| MemBench | memory-type and retention behavior |
| EverMemBench | evolving or collaborative memory pressure |
| BEAM | compression and long-context efficiency |
| ARES | retrieval relevance, faithfulness, and groundedness |
| RAGBench | evidence use and retrieval-grounded generation |
| CORAL | multi-turn conversational retrieval |
| BRIGHT | reasoning-intensive retrieval |
| HaluEval | unsupported generation control |
| Mu-SHROOM | multilingual unsupported-generation detection |

Every result uses one evidence label:

```text
official_dataset   official or independently verified dataset and metric
translated_proxy  frozen Vermory task translating a benchmark capability
inspired_case      local case influenced by benchmark methodology
```

Evidence labels are never merged into one score. Before a benchmark becomes a release qualification input, verify its current availability, license, metric implementation, relevance, and contamination risk.

## 10. Metrics

### 10.1 Hard factual metrics

- forbidden target-fact leakage;
- stale fact represented as current;
- wrong continuity attachment;
- abstention on labeled ambiguity;
- deleted target-fact leakage under reformulation;
- active-state rebuild equivalence;
- duplicate-event effects;
- source-authority resolution on labeled conflicts;
- downstream artifact correctness.

### 10.2 Quality metrics

- current-fact recall and precision;
- memory-candidate precision and recall;
- automatic-activation precision;
- downstream task success;
- user correction burden;
- unnecessary-memory and unnecessary-context rate;
- context cost;
- explanation usefulness;
- retrieval and end-to-end latency.

### 10.3 Operational metrics

- write, update, delete, and rebuild duration;
- queue age and retry count;
- provider failure and fallback rate;
- index size and memory footprint;
- concurrency behavior;
- long-duration growth and stale working-state accumulation.

## 11. Hard Gates And Calibrated Targets

The zero-tolerance invariants in the Product Constitution are fixed hard gates.

Recall, precision, latency, token cost, correction burden, concurrency, and scale are calibrated targets. They are not assigned final numbers before the first real corpus and baselines run.

Calibration procedure:

1. Run all baselines on the frozen public discovery batch.
2. Record raw case counts and latency decomposition.
3. Identify the minimum quality required for useful operation and the measured capability of safe baselines.
4. Propose target values with hardware, provider, dataset, and confidence information.
5. Freeze the target profile for the next evidence batch.
6. Revise a target only through a versioned decision that preserves prior results.

An empty or nearly disabled feature cannot satisfy a precision target by producing no candidates. Reports include coverage and denominators.

## 12. Scale And Failure Profiles

Scale is validated through named profiles rather than one universal number.

Initial profiles are hypotheses:

- `developer-local`: one active user and several real continuities;
- `self-hosted-team`: multiple users and concurrent clients;
- `server-qualification`: large projection and sustained-concurrency stress.

Record counts, concurrency, hardware, and latency objectives are set after measuring the first two profiles. The previous 100,000-memory, 1,000,000-projection, 10,000-similar-record, and 50-client values remain candidate server-qualification stress points, not implementation prerequisites or product truths.

Failure testing begins in the first vertical experiment and expands over time. It includes provider timeout, duplicate delivery, index loss, PostgreSQL restart, migration, backup and restore, optional adapter loss, deletion propagation, and long-running memory growth.

## 13. Evidence Separation

To reduce self-confirmation:

- public fixtures and expected assertions are versioned before implementation;
- sealed expectations are owned outside the implementation workspace;
- deterministic checks are executable independently of the memory implementation;
- open-ended review records judge disagreement;
- model providers used to generate candidates are not automatically used as the sole judge;
- baseline and Vermory runs preserve raw inputs, outputs, tool actions, and final artifacts;
- reports include failures, exclusions, unavailable providers, and manual intervention.

## 14. Evidence Batch Exit

An evidence batch is complete when:

- its cases and fixture hashes are frozen;
- relevant baselines ran or their blockers are recorded;
- hard-gate assertions have independent evaluators;
- downstream artifacts are available where the task produces artifacts;
- every failure is classified as product, implementation, provider, fixture, evaluator, or infrastructure;
- resulting architecture decisions update the Hypothesis Register;
- the next batch changes domains, wording, identities, or interaction pressure enough to test generalization.
