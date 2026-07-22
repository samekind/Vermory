# Vermory Capability And Evidence Matrix

Date: 2026-07-22

This is the repository's authoritative map of what Vermory has actually
qualified. It separates a frozen case contract from runtime execution, real
client use, real model consumption, and protected delivery. The historical
provider-oriented matrix remains in
[`evaluation-matrix.md`](evaluation-matrix.md), but it is not a source for
current product claims.

## Reading The Matrix

Status meanings are deliberately narrow:

- `client-qualified`: the exact case reached an accepted external-client
  trajectory, with database or artifact evidence in addition to client output.
- `model-qualified`: the exact case reached an accepted real-model comparison,
  but that run was a provider evaluation rather than a named client workflow.
- `runtime-qualified`: the exact case passed its runtime, database, recovery,
  security, or delivery contract; no real-model claim is needed or made.
- `contract-only`: the case is frozen and its deterministic contract passes,
  but the complete external trajectory has not been qualified.
- `external-blocked`: the required external client or provider failed before
  the case's acceptance boundary. The failure is retained and no pass is
  inferred.

Frozen public reality cases: `21`.

| Continuity line | Cases containing the line |
|---|---:|
| Workspace | 6 |
| Conversation | 12 |
| Global Defaults | 6 |
| Bridge | 7 |
| Security | 14 |

A case may cover more than one line, so these counts intentionally overlap.

## Workspace And Cross-Mode Continuity

| Case | Status | Exact accepted boundary | External surface / model | Primary evidence | Explicit boundary |
|---|---|---|---|---|---|
| `W01-synapseloom-continuity` | `model-qualified` | The six-condition run completed; the native condition used current repository facts, rejected stale workspace observations, and passed the downstream task | Direct SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`; not a named coding-client run | [W27 real utility comparison](evidence/2026-07-19-w27-real-utility-comparison.md), [native retrieval follow-up](evidence/2026-07-19-w27-native-retrieval-followup.md) | Does not qualify arbitrary repository migration or a full coding task. |
| `W03-workspace-workaround-validity` | `runtime-qualified` | Expired workaround is excluded while durable engineering constraints remain current; rebuild, restore, outage, and concurrent forget gates pass | Profile-level Grok and Codex client receipts exist, but no exact W03 client artifact is claimed | [Memory eligibility and retention](evidence/2026-07-16-memory-eligibility-retention.md) | The measured W19 profile is not a universal capacity or latency claim. |
| `W04-canonical-repository-cross-client` | `external-blocked` | Current case is frozen; Cursor MCP discovery succeeded, but generation stopped before `prepare_context` | Cursor Agent account returned `ActionRequiredError: You have an unpaid invoice` | [Cursor Agent attempt](evidence/2026-07-19-cursor-agent-real-client-attempt.md) | The earlier Codex W04 run predates the current Cursor-specific contract and cannot substitute for it. |
| `W05-trusted-workspace-attachment` | `client-qualified` | Trusted launcher resolves the namespaced Git root; real worktree, same-name repository, clone, bare mirror, mirror/fork checkout, multiple-remote, physical-move, and cross-namespace gates abstain or reconnect only through explicit adopt/rebind; Codex receives current facts and writes one proposed idempotent observation | Official Codex CLI `0.144.3` for the client trajectory; deterministic Git/PostgreSQL W31 and W33 for the topology trajectories | [Trusted workspace attachment](evidence/2026-07-19-trusted-workspace-attachment.md), [real local Git topology](evidence/2026-07-22-real-git-workspace-topology.md), [remote Git topology](evidence/2026-07-22-remote-git-workspace-topology.md) | Does not qualify automatic repository-intent inference, a second coding client running the full topology, or physical cross-host migration. |
| `B01-conversation-workspace-promotion` | `client-qualified` | Selected governed conversation memory is promoted into a workspace, consumed through MCP, then reversed without deleting the source | Real Grok client consumption | [Durable bridges runtime](evidence/2026-07-14-durable-bridges-runtime.md) | Promotion is explicit and reversible; it is not automatic topic-to-project merging. |
| `B02-linked-conversations-workspace-rebind` | `client-qualified` | Explicit conversation link and workspace rebind deliver only governed memory; reversal stops future sharing while preserving local history | Real Grok replay through linked conversation and workspace paths | [Durable bridges runtime](evidence/2026-07-14-durable-bridges-runtime.md) | Reversal is not deletion of external-client history. |

## Conversation, Formation, And Defaults

| Case | Status | Exact accepted boundary | External surface / model | Primary evidence | Explicit boundary |
|---|---|---|---|---|---|
| `C01-device-maintenance-continuity` | `client-qualified` | Corrected action and verified state survive restart; deleted or excluded details do not return | Grok Web Chat `grok-4.5`; direct DeepSeek-V4-Flash comparison | [Grok Web Chat runtime](evidence/2026-07-13-grok-webchat-runtime.md), [W27 comparison](evidence/2026-07-19-w27-real-utility-comparison.md) | The original C01 run is the HTTP runtime plus external model client; W32 separately qualifies browser lifecycle without changing C01's model evidence. |
| `C02-housing-viewing-validity` | `runtime-qualified` | Expired appointment is ineligible while durable housing constraints remain current | Deterministic W19 runtime; no exact external-client artifact claimed | [Memory eligibility and retention](evidence/2026-07-16-memory-eligibility-retention.md) | Expiry is not deletion and is not advertised as forgetting. |
| `G01-language-default-local-override` | `client-qualified` | A local English instruction does not mutate the Chinese Global Default; correction and deletion propagate to Web Chat and MCP | Grok Web Chat and external MCP, `grok-4.5`; direct DeepSeek-V4-Flash comparison | [Grok Global Defaults runtime](evidence/2026-07-13-grok-global-defaults-runtime.md), [W27 comparison](evidence/2026-07-19-w27-real-utility-comparison.md) | Global Defaults remain explicit and thin; ordinary chat does not auto-grow personality settings. |
| `S01-deletion-and-source-injection` | `client-qualified` | Deleted target fails exact and paraphrased probes while valid related guidance remains; source text cannot promote itself | Grok Web Chat `grok-4.5`; direct DeepSeek-V4-Flash comparison | [Grok Web Chat runtime](evidence/2026-07-13-grok-webchat-runtime.md), [W27 comparison](evidence/2026-07-19-w27-real-utility-comparison.md) | This is one governed deletion/injection case, not a universal proof against every prompt-injection technique. |
| `O01-openclaw-home-maintenance` | `client-qualified` | OpenClaw restart, explicit link, correction, deletion, thin defaults, reversal, and fail-open behavior pass | Real OpenClaw with Grok provider | [OpenClaw runtime](evidence/2026-07-14-openclaw-runtime.md) | Unrelated upstream generated-module warnings are outside the qualification. |
| `H01-hermes-linked-sessions` | `client-qualified` | Two isolated Hermes sessions remain separate until explicitly linked; current governed memory is consumed, then reversal and fail-open pass | Official Hermes `v0.18.2`, direct SiliconFlow `deepseek-ai/DeepSeek-V4-Flash` | [Hermes real-client qualification](evidence/2026-07-18-hermes-real-client.md) | Reversal does not rewrite Hermes-owned transcript history. |
| `F01-conversation-formation-loop` | `client-qualified` | User observations form reviewable candidates; accept, reject, correction, temporary-instruction abstention, deletion, replay, and isolation pass | OpenClaw with Grok `grok-4.5`; direct SiliconFlow DeepSeek-V4-Flash formation | [Conversation formation loop](evidence/2026-07-18-conversation-formation-loop.md) | Model output remains proposed until explicit governance; one rejected model-quality result is retained. |
| `F02-automatic-conversation-review` | `client-qualified` | Completed turns schedule bounded review; operator actions govern activation, correction, rejection, and forgetting; cross-client isolation holds | OpenClaw/Grok accepted path and Hermes isolation control | [Automatic conversation review](evidence/2026-07-18-automatic-conversation-review.md) | The worker does not grant clients authority to activate their own memories. |
| `F03-verified-tool-outcome-formation` | `client-qualified` | Only successful allowed tool outcomes become reviewable; denied, failed, and forged outcomes do not; forgetting removes shared-evidence recall | Real OpenClaw tool trajectory with real-model post-delete proof | [Verified tool outcome formation](evidence/2026-07-18-verified-tool-outcome-formation.md) | An assistant claim is not treated as verified tool evidence. |
| `B03-three-client-conversation-bridge` | `client-qualified` | One real Chrome Web Chat source is linked to exact official Hermes and OpenClaw Gateway continuities; only the governed bundle is delivered, replay is idempotent, unrelated sessions stay empty, and reversal stops fresh delivery while retaining source and client history | Chrome 150, Hermes `v0.18.2`, OpenClaw `2026.6.11`, Codex CLI `0.144.3` / `gpt-5.6-terra` through a temporary loopback relay | [Three-client bridge contract](evidence/2026-07-21-three-client-conversation-bridge-contract.md), [Three-client real-client run](evidence/2026-07-22-three-client-conversation-bridge-real-clients.md) | Qualifies the frozen same-run trajectory, not automatic cross-client merging, transcript deletion, a production relay, every client channel, or model ranking. |

## Security, Operations, And Delivery

| Case | Status | Exact accepted boundary | External surface / model | Primary evidence | Explicit boundary |
|---|---|---|---|---|---|
| `I01-authenticated-multitenant-rls` | `client-qualified` | Digest-only tokens, role-gated routes, PostgreSQL RLS, revocation, same-anchor tenant isolation, and fail-open behavior pass | Authenticated OpenClaw/Grok replay plus non-owner database role | [Identity, authorization, and RLS](evidence/2026-07-14-identity-authorization-rls.md) | Application checks and RLS are both required; neither is claimed sufficient alone. |
| `I02-postgresql-operations-recovery` | `runtime-qualified` | Migration replay, dump/restore, projection rebuild, runtime-role restoration, and bounded outage recovery pass | PostgreSQL 18 and Linux runtime; model use is not required | [PostgreSQL operations and recovery](evidence/2026-07-14-postgresql-operations-recovery.md), [Linux portability](evidence/2026-07-14-linux-runtime-portability.md) | Does not define a universal HA topology or service SLO. |
| `I03-postgresql-ha-pitr` | `runtime-qualified` | Streaming standby promotion and exact-LSN PITR restore the expected authoritative state without reviving later deletion/revocation changes | PostgreSQL 18; authenticated Web Chat recovery probe; no model-quality claim | [PostgreSQL HA and PITR](evidence/2026-07-16-postgresql-ha-pitr.md) | One measured local topology is qualified, not every distributed deployment. |
| `I04-protected-artifact-signing` | `runtime-qualified` | Untrusted test job builds complete payload manifest; separate OIDC job signs it; identity, issuer, tamper, and cross-host verification pass | GitHub Actions, Cosign, ARM64 Mac mini; model use is not applicable | [Protected artifact signing](evidence/2026-07-18-protected-artifact-signing.md) | The signed subject is the release manifest, not a claim that every runtime trajectory was replayed on that exact head. |
| `I05-durable-linux-service-lifecycle` | `runtime-qualified` | Exact-head install, authenticated startup, upgrade, automatic and explicit rollback, private backup, digest/non-empty-target rejection, empty-target restore, runtime-role recovery, projection rebuild, restored authentication/default access, and credential scans passed on both native architectures | GitHub-hosted Ubuntu 24.04 AMD64 and ARM64, systemd, PostgreSQL 18; model use is not applicable | [Durable Linux lifecycle qualification](evidence/2026-07-22-durable-linux-service-lifecycle.md) | The ephemeral runners do not qualify long-duration uptime/SLA, DEB/RPM repositories, backup encryption, or PostgreSQL migration rollback. |

## Public Benchmark And Comparison Evidence

| Evidence lane | Qualified result | What it supports | What it does not support |
|---|---|---|---|
| LongMemEval original sample | `dataset_sample` over six official records with real Grok reader | Original-dataset ingestion, four-condition comparison, deterministic scoring, and evidence retention | A full benchmark score |
| LongMemEval-S lexical retrieval | `qualified_dataset_full` | Full public retrieval execution and reproducible ranking metrics | Reader QA or universal retrieval quality |
| LongMemEval-S vector retrieval | `qualified_dataset_full` | Full public direct-vector retrieval, long-input accounting, and retained rejected runs | Candidate profile promotion or reader QA |
| LongMemEval-S reader QA | `qualified_dataset_full` with custom Grok judge | Full public reader execution for the frozen lexical lane | Model ranking, sealed evaluation, or transfer to every client |
| W27 real utility | 4 frozen cases x 6 conditions; Vermory `4/4`, best completed simple baseline `1/4`, zero native forbidden hits | Positive utility for the frozen cases with less context than full history | Universal superiority, latency leadership, or model ranking |
| Domestic vector-reader attempt | rejected before an accepted result because the direct provider account balance was exhausted | Honest failure attribution and stop conditions | Any domestic reader score or pass |

See [LongMemEval original sample](evidence/2026-07-14-longmemeval-original-sample.md),
[full lexical retrieval](evidence/2026-07-15-longmemeval-s-full-retrieval.md),
[full reader QA](evidence/2026-07-15-longmemeval-s-full-reader-qa.md),
[full vector retrieval](evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md),
and the [rejected domestic reader run](evidence/2026-07-20-longmemeval-s-domestic-vector-reader-qa-rejected-v1.md).

## Additional Surface Qualification

| Runtime case | Status | Accepted boundary | Primary evidence | Explicit boundary |
|---|---|---|---|---|
| `W32-browser-webchat-lifecycle` | `runtime-qualified` | Real Chrome same-origin application passes thread isolation, refresh, unavailable-service retry, lost-response replay, candidate accept/reject, correction, forgetting, desktop, and mobile gates | [Browser Web Chat lifecycle](evidence/2026-07-22-browser-webchat-lifecycle.md) | Uses a deterministic provider to isolate the browser contract; it is not a real-model or public multi-user deployment claim. |

## Client Coverage

| Client or surface | Current accepted scope | Not yet qualified |
|---|---|---|
| Codex | Workspace MCP consumption, trusted workspace attachment, artifact creation, proposed write-back, replay | Arbitrary clone/worktree adoption and every coding workflow |
| Grok CLI | Web Chat provider, workspace MCP consumer, Global Defaults, bridge and retrieval trajectories | Current account refresh is unavailable for new runs; old evidence remains valid but is not silently refreshed |
| OpenClaw | Conversation continuity, formation/review, verified tool outcomes, correction, deletion, links, fail-open, and the B03 same-run Web Chat/Hermes bridge through the official Gateway HTTP agent path | Messaging channels and arbitrary session migration beyond the qualified cases |
| Hermes | Explicit linked-session continuity, current-only recall, reversal, fail-open, and the B03 same-run Web Chat/OpenClaw bridge | Gateway and messaging modes beyond the qualified CLI sessions |
| Cursor Agent | MCP discovery and zero-side-effect failure handling | The W04 generation, prepare, artifact, commit, and replay gates remain externally blocked |
| Web Chat | Real HTTP and Chrome browser lifecycle with thread isolation, refresh, persisted retry, replay deduplication, candidate review, correction, deletion, responsive layout, and explicit three-client bridge delivery to Hermes/OpenClaw | Authenticated public multi-user browser deployment and browsers other than the qualified Chrome version |

## Protected Delivery Rule

Every publishable pull-request head must pass the protected `test`,
`linux-service-lifecycle`, `linux-service-lifecycle-arm64`, and `sign-snapshot`
jobs. The exact live head, run,
jobs, and signed artifact belong
in GitHub's protected check record and PR evidence comment rather than a
manually copied static status. A green CI badge alone does not upgrade a
`contract-only` or `external-blocked` case to `client-qualified`.

## Current Open Qualification Boundaries

The following remain explicit work, not hidden implementation details:

1. Re-run W04 with Cursor only after the external account can generate; Codex,
   Hermes, or OpenClaw cannot substitute for that result.
2. Run the real worktree/adopt/rebind topology through an additional coding
   client and a physical cross-host migration. W31 qualifies actual local Git
   worktree, same-name repository, clone, and physical move behavior; W33 adds
   bare mirror, mirror/fork checkout, multiple-remote, and cross-namespace
   migration behavior. Both runs qualify tenant isolation, explicit governance,
   replay, and reversal without substituting for another client or host.
3. Extend I05 beyond its qualified ephemeral Ubuntu AMD64 and ARM64 runners to
   long-duration uptime/SLA evidence, DEB/RPM repositories, and an explicit
   database-migration rollback policy.
4. Add a genuine withheld external evaluation; public cases and internal blind
   splits are not called sealed evidence.
5. Complete blocked real-provider reader runs only when their original frozen
   provider and account requirements are available; do not replace them with a
   different client or model and keep the same claim.
