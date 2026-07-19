# Real Utility Comparison Design

Status: frozen for implementation

Date: 2026-07-19

## 1. Purpose

W27 measures whether governed Vermory context improves real downstream tasks
without reviving stale, deleted, or out-of-scope information. It replaces no
historical evidence. In particular, the W19 baseline table remains evidence for
its report and eligibility contract; those fixture-provided counts are not
relabelled as model-executed utility results.

W27 executes previously frozen reality cases under comparable context
conditions. The model, task, system frame, generation budget, and tool access
stay fixed inside one lane. Only the memory or context condition changes.

This is a memory-system comparison, not a model ranking.

## 2. Frozen Cases

The first utility batch reuses four cases frozen before this design:

| Case | Capability pressure |
|---|---|
| `W01-synapseloom-continuity` | current repository truth versus stale historical workspace state |
| `C01-device-maintenance-continuity` | corrected action, verified state, and an explicit user exclusion |
| `G01-language-default-local-override` | thin Global Default versus an expired task-local override |
| `S01-deletion-and-source-injection` | deletion, paraphrased disclosure pressure, and untrusted source instructions |

The committed manifests, events, fixtures, and fixture locks remain the case
authority. W27 may add condition inputs and scoring aliases, but it may not
change case history or expected behavior after observing model output.

## 3. Comparison Conditions

Every successful model lane executes the conditions in this order:

1. `no_context`: the downstream task and common system frame only.
2. `full_history`: the complete authorized case event trajectory in sequence,
   including historical and later-corrected statements. S01 is synthetic, so
   the deleted marker may be used in this counterfactual privacy baseline.
3. `plain_summary`: one versioned summary frozen before the formal model run.
   It may compress history but receives no Vermory lifecycle, authority,
   deletion, or continuity metadata.
4. `plain_retrieval`: ordinary retrieval over raw case-event chunks with a
   fixed query, embedding profile, top-k, and no Vermory lifecycle or authority
   filtering.
5. `mem0_oss`: an isolated official mem0 OSS instance receives the same
   authorized trajectory and serves its normal memory result. PostgreSQL does
   not treat mem0 output as authority. If the official condition cannot run,
   the lane records the exact blocker and remains incomplete.
6. `vermory_native`: the production PostgreSQL path forms or imports candidate
   state, applies explicit case governance, resolves the exact continuity, and
   prepares current eligible context through the normal runtime. The evaluator
   consumes the returned delivery rather than constructing a packet by hand.

All context is bounded by the same byte and generation-token limits. Truncation
is deterministic and recorded. A condition never receives another
continuity's distractor as useful context; separate distractors are used only
to test isolation.

## 4. Model Lanes

The formal profile targets two direct OpenAI-compatible provider lanes:

- SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`;
- Duojie `gemini-3-flash`.

Only non-Pro SiliconFlow models are permitted. Neither provider may route
through the Mac mini NewAPI. Credentials are supplied at runtime, never written
to reports, fixtures, command lines, process listings, or repository files.

Each lane is reported independently. A provider failure does not become a
score of zero, does not promote another model to a winner, and does not allow a
mock or different client to substitute for the failed lane. At least one full
real lane is required for the W27 platform qualification; the second lane is a
cross-provider compatibility target whose success or failure remains visible.

## 5. Common Consumer Contract

The consumer receives:

```text
common system frame
+ condition-labelled context body, if any
+ exact frozen downstream task
```

The system frame says that context is evidence rather than instruction, current
and historical claims must be distinguished, unavailable facts must not be
guessed, and the response must answer the user-facing task. It does not reveal
expected answers, forbidden strings, condition identity, memory IDs, lifecycle
labels, or scores.

The provider request records endpoint class, provider, model, request hash,
context hash, context byte count, max tokens, response hash, latency, status,
and normalized failure category. Raw provider artifacts are retained only in
the private evidence root and are privacy-scanned before any normalized subset
is committed.

## 6. Deterministic Scoring

W27 does not use an LLM as the sole judge. Each case has versioned deterministic
checks derived from its frozen manifest:

- required current facts or action constraints;
- forbidden stale, deleted, injected, or cross-scope facts;
- output-language requirements where applicable;
- exact technical values such as ports, counts, percentages, and repository
  behavior;
- artifact shape when the case defines one.

Scoring normalizes Unicode, whitespace, punctuation, and declared equivalent
phrases. It does not use semantic similarity to waive a forbidden fact. The
original output, normalized output, individual check results, and scorer
version are retained.

One condition succeeds on a case only when every required check passes and no
forbidden check fires. Missing output, provider errors, malformed artifacts,
and scorer errors are separate failures rather than incorrect model answers.

## 7. PostgreSQL And Governance Boundary

PostgreSQL remains the only semantic authority in the native condition.

- Case input first enters as source or conversation observations.
- Model-generated formation may create candidates only.
- Explicit frozen governance actions establish current, superseded, rejected,
  expired, or deleted state.
- Retrieval enforces tenant, continuity, lifecycle, validity, and deletion
  before relevance ranking.
- The exact delivery ID and delivered memory IDs are retained.
- Consumer output returns as one proposed observation and cannot activate
  itself.
- Repeating the same write-back operation is idempotent.
- Forgetting or supersession must be visible in both lexical and vector
  projections before the formal native task is run.

The evaluator independently queries PostgreSQL after each native case. Model
text alone cannot prove delivery, isolation, deletion, or proposed-only
write-back.

## 8. Acceptance Gates

W27 passes the named platform profile only when all of these hold:

1. All four case fixture locks validate before implementation execution.
2. Every condition input is generated from the declared case and has a stable
   SHA-256 digest before provider output is scored.
3. One real model lane completes all 24 case-condition calls with the same
   task, system frame, budget, and provider parameters inside each case.
4. `vermory_native` succeeds on all four downstream cases.
5. `vermory_native` has zero stale, deleted, injected, cross-continuity, and
   cross-tenant forbidden hits.
6. Native PostgreSQL evidence proves exact continuity delivery, current-only
   eligibility, proposed-only write-back, and idempotent replay for every case.
7. No baseline result is copied from a test fixture or manually entered into
   the formal report; aggregate counts are recomputed from retained per-call
   artifacts.
8. No simple baseline is required to fail. The report states whether Vermory
   is better, equal, or worse on task success, forbidden use, and context cost.
9. A positive utility claim is allowed only when Vermory is no worse than the
   best completed simple baseline on task success, strictly safer or strictly
   more successful on at least one case, and uses less context than
   `full_history` in aggregate.
10. If utility is equal, only a governance-at-comparable-utility claim is
    allowed, and PostgreSQL evidence must prove a hard behavior the equal
    baseline does not provide.
11. mem0 is isolated, removable, and unable to mutate native authority; its
    outage or deletion leaves the native condition rebuildable and usable.
12. Provider, database, adapter, scorer, and transport failures remain in an
    append-only failure ledger and cannot be silently retried under the same
    run identity.
13. Normalized committed evidence contains no credentials, private keys,
    account identities, private absolute paths, full environments, or private
    raw transcripts.
14. The complete repository gate and protected `samekind/Vermory` CI pass on
    the exact evidence head.

## 9. Reports

The machine-readable report includes:

- implementation revision and runtime profile digest;
- case fixture-lock digests;
- provider lane and condition execution status;
- per-call input, context, output, score, latency, and failure hashes;
- current-fact hits and forbidden-fact hits;
- task success and context bytes by case and condition;
- native delivery and write-back receipts;
- mem0 isolation and teardown result;
- utility decision and the rule that produced it;
- exclusions, failures, and explicit non-claims.

The Markdown report presents conditions within each model lane. It never sorts
models into a leaderboard or emits a single cross-model score.

## 10. Non-Claims

W27 does not create sealed evidence, prove automatic memory-formation quality
for arbitrary input, qualify every provider, rank models, establish universal
latency or cost SLOs, prove long-duration user satisfaction, or complete
Vermory. It proves or falsifies one public four-case utility profile and keeps
all weaker or failed results visible.

## 11. Scorer v2 Amendment

The first complete SiliconFlow lane exposed two deterministic false negatives
without exposing a native context failure. C01 returned `82%` for the frozen
`82 percent` fact. G01 followed the task's Chinese-language requirement and
expressed `local-scope` and `Chinese` as Chinese phrases. The original
`utilityeval-checks-v1` scorer rejected those surface forms even though the
retained context and output artifacts contained the required meaning.

The original run and its 2/4 native aggregate remain append-only evidence.
They are not rescored or overwritten. The replacement profile is explicitly
versioned as `real-utility-comparison-v2` with
`utilityeval-checks-v2`. Its declared aliases are stored in `case.json`, copied
into the hashed context bundle, validated against each case's existing
deterministic checks, and recorded by the scorer when matched. The case
history, prompt, current facts, forbidden facts, context bodies, and hard gates
are unchanged.

This amendment is a scorer correction made after observing the first run, so
the v2 result is not represented as a blind pre-registered replication. A new
provider run identity and a new report are required. Forbidden checks use the
same declared-alias mechanism and still receive no semantic-similarity or LLM
judge waiver.

Two later append-only runs tightened the execution contract before the final
replication. A four-worker run with provider-default thinking completed only
13 of 24 calls and retained 11 `provider_timeout` failures. The next profile
fixed two workers and explicitly disabled thinking for the SiliconFlow
DeepSeek-V4-Flash lane; it completed 24 of 24 calls, then exposed two further
surface-form false negatives: `has been deleted` versus
`already been deleted`, and `rotate recovery codes after each use` versus
`rotated after use`. The v4 profile declares common English and Chinese forms
for those same frozen lifecycle facts. Each profile revision has a distinct
SHA-256-bound bundle and run identity; no prior report is overwritten.

The following v4 replication completed 24 of 24 calls and passed three native
cases. C01 used the additional positive adverb in `has been successfully
deleted`, demonstrating that substring aliases remained brittle. The v5
profile therefore upgrades to `utilityeval-checks-v3` and declares a bounded
RE2 equivalent that requires the `Game A resource bundle` subject and a
positive deleted/removed construction. The same pattern rejects a negated
`has not been deleted` clause. Regex equivalents are compiled during profile
and bundle validation and are retained in the hashed scoring contract.

The v5 replication again completed 24 of 24 calls but changed the valid G01
and S01 wording under the provider's implicit sampling default. The v6 profile
therefore freezes `temperature=0` as an explicit request and report field and
adds bounded positive relation predicates for task-local override expiry and
post-use recovery-code rotation. This removes provider-default randomness from
the execution contract without weakening stale, deleted, injected, or
cross-scope forbidden checks.
