# Vermory

**面向 AI 的可治理记忆与上下文连续性平台**

[![CI](https://github.com/samekind/Vermory/actions/workflows/ci.yml/badge.svg)](https://github.com/samekind/Vermory/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[English](README.md)

Vermory 是一个面向 AI 客户端的、以真实场景和可验证证据驱动的记忆与上下文连续性平台。它服务于 AI Coding 工具、Web Chat、个人助手及其他需要持续处理真实事务的 AI 系统。

项目的 canonical repository 现为
[`samekind/Vermory`](https://github.com/samekind/Vermory)。历史证据中原有的
`jstar0/Vermory` 链接与签名 identity 作为已发生事实保留，不做伪改写。详见
[ADR 0002](docs/adr/0002-canonical-repository-move.md)。

Vermory 不只是保存几段 memo，也不只是给 PostgreSQL 套一层向量检索。它要解决的是：

- 当前交互到底属于哪个持续空间；
- 哪些观察值得形成长期记忆；
- 多个来源冲突时应该相信谁；
- 哪些事实仍然有效、已经过期、仅在局部有效或已经删除；
- 当前模型和任务真正需要看到哪些上下文；
- 记忆的形成、修正、召回、桥接和删除怎样保持可解释、可审计。

## 三种连续性模式

| 模式 | 主要锚点 | 用户侧行为 |
|---|---|---|
| Workspace-backed continuity | 仓库根目录、工作区路径、manifest、显式绑定 | 同一工作区可跨 Codex、Claude Code、Grok 等客户端接续；不同工作区默认隔离。 |
| Conversation-backed continuity | thread、渠道、联系人、命名事务或话题 | 日常事务可以跨会话继续，但无关话题不能被擅自合并。 |
| Global Defaults | 用户显式确认的稳定偏好和长期设置 | 只保留薄而稳定的默认层，临时任务要求不能污染全局偏好。 |

`promote`、`link`、`export`、`adopt`、`rebind` 等跨模式操作属于显式治理动作，而不是底层自动混合。

## 核心约束

Vermory 当前的产品宪法要求：

- 标注为硬门的测试中，跨租户、跨 continuity 禁止事实泄漏必须为零；
- 强锚点识别不确定时必须拒绝猜测或请求确认；
- 过期和被替代的事实不能继续作为当前事实使用；
- 已删除目标不能通过精确、改写、语义、缓存、历史或可选后端再次泄漏；
- 当前仓库和事实源可以纠正旧记忆；
- 模型推断不能悄悄覆盖用户明确意图；
- PostgreSQL 保存权威状态，检索投影必须可销毁、可重建；
- mem0、MemOS、Supermemory 等只能作为可选投影适配器，不能成为第二套事实源。

详见 [产品宪法](docs/superpowers/specs/2026-07-11-vermory-product-constitution.md)。

## 当前进度

Experiment 0 已完成，当前仓库已经具备：

- 严格的 reality case manifest 与 JSONL 事件合同；
- 来源授权、匿名化、fixture 哈希和路径越界校验；
- 确定性的 `fixture-lock.json` 与冻结后变更检测；
- `public` 和 `withheld_local` 证据等级，并拒绝把本地可读目录伪装成 sealed；
- 外部 sealed evaluator 的 Ed25519 attestation 验签能力；
- 17 个覆盖 workspace、conversation、Global Defaults、删除、source injection、durable bridge、OpenClaw 与 Hermes 真实客户端连续性、authenticated multi-tenant RLS、PostgreSQL 恢复、自动 conversation review、F03 工具结果形成合同和 I04 受保护制品签名合同的公开冻结案例；
- JSON 和 Markdown 实验报告。

仓库同时已经包含 workspace、conversation、Global Defaults、durable bridge、显式可信来源修订、按稳定事实 key 治理的 source candidate、可信来源无 key 时的 provider 闭集目标匹配、OpenClaw external-turn lifecycle、authenticated multi-tenant HTTP profile、原生 PostgreSQL 恢复、可选 active-only pgvector runtime、可并行构建和测量切换的版本化语义投影，以及 PostgreSQL transactional outbox 故障资格。来源变化可以先形成候选而不改变 AI 当前上下文；拒绝候选不会改动当前事实，接受候选则原子替代仍然有效的同 key 目标。可信来源只有精确内容和 revision、没有内部 key 时，provider 只能从当前 scope 的闭合集合中选一个现有 key 或 abstain；Vermory 会验证并审计结果，仍然要求操作者明确接受。真实 Grok MCP 任务已经在投影重建后只消费被接受的新事实，并把结果回写为 proposed。语义检索 profile 共享 PostgreSQL 权威事实，但拥有独立 cursor、vector、audit、reset/rebuild 与 promotion decision；当前实测 v2 仍保留为 candidate，lexical 和 v1 默认均未被擅自切换。W11 disposable-cluster 运行进一步证明 1000 条 backlog 的有界消费、provider 重试、at-least-once replay、embedding 进行中的 PostgreSQL immediate restart、同一 pool 恢复、删除压过晚到结果，以及重启后的直接 provider 恢复，因此当前 self-hosted profile 不要求 Redis。W12 又在 `server-qualification-v1` 下完成 55 万条 governed memory、10 万条当前 lexical/vector、100 万条历史 projection event、1000 条并发删除、租户内竞争 worker、最终 lag 与 scope leakage 均为 0，以及真实 provider 的 post-scale projection/query probe；current-authority bootstrap 只嵌入当前事实，不重放过时历史。1000 次 scoped query 全部返回正确当前事实，但 550 次 vector 请求中有 412 次受控回退 lexical，因此这是规模运行与降级合同资格，不是 10 万向量下的语义召回质量宣称。认证 profile 使用服务端发行且只保存 digest 的 token、角色路由、非 owner PostgreSQL runtime identity、tenant-aware foreign keys，以及覆盖当前 continuity graph 的 RLS。恢复证据覆盖迁移重放、原生 dump/restore、投影重建、runtime role 重建、有界数据库中断恢复、PostgreSQL 18 streaming standby 提升，以及带恢复后凭据治理的精确 LSN PITR。Pull Request CI 会在干净 Ubuntu runner 上启动 PostgreSQL 18，并自动执行数据库 Go 测试、关键 runtime race、release build 和 OpenClaw 安装/检查/打包链路。每份证据只对实际执行过的客户端、模型、故障条件和确定性硬门负责，任何单一切片都不被当成“整个平台已经完成”的证明。

后续 audit 归因确认：W12 的所有 vector 降级都发生在并发删除事件使
projection 暂时存在 lag 的窗口，failure code 均为 `projection_lag`；另一个
projection 始终 current 的 10 万向量控制组完成了 `550/550` 次有效 vector
请求且零降级，因此没有引入 scoped HNSW 生产改动。详见
[Vector 降级归因实证](docs/evidence/2026-07-15-vector-degradation-attribution.md)。

W14 随后对固定 revision 的 LongMemEval-S cleaned artifact 完成全量检索资格：
500 条隔离 conversation continuity、23,867 条 governed session memory、470 条
计分查询，runtime failure 和 scope leakage 均为 0。生产 lexical 在 K10 的
RecallAll 为 `0.7340`，低于同文本 token-overlap baseline 的 `0.8383`；
multi-session RecallAll 为 `0.5620`。这是一份全量数据集检索诊断，不是 QA
分数，也不宣称 lexical 已经最优。详见
[LongMemEval-S 全量检索实证](docs/evidence/2026-07-15-longmemeval-s-full-retrieval.md)。

W15 随后把固定的 K10 ranking 交给真实 reader，完成 1000 条隔离任务：
`grok-composer-2.5-fast` 负责回答，`grok-4.5` 使用固定 upstream prompt
作为 custom judge。两组条件都完成 500/500，reader 和 judge 终态失败均为
0。token-overlap 的 custom-judge accuracy 为 `0.7580`，Vermory lexical 为
`0.6820`；paired outcomes 为 `310` 条都正确、`69` 条仅 plain 正确、`31`
条仅 Vermory 正确、`90` 条都错误。该负结果被原样保留，不切换 lexical
默认值，也不用于模型排名。详见
[LongMemEval-S 全量 Reader QA 实证](docs/evidence/2026-07-15-longmemeval-s-full-reader-qa.md)。

W16 随后完成了一条专用 PostgreSQL 18 物理恢复轨迹：streaming standby
追到 primary flush LSN 后，专用 primary 被 immediate stop；过渡期 Web Chat
请求返回零 receipt、零行，原 handler/runtime/auth pools 在 standby promotion
后恢复。另一条 PITR 恢复精确停在 `0/402ACE0`，恢复 authority fingerprint
与 T2 完全相同，排除了更晚的删除/撤销与 fact C；随后从恢复 authority
重建 3 条 active lexical projection，证明历史 token 已复活，再次撤销并
验证旧 token 返回 `401`，最后新 operator token 才获准访问。运行保留了
setup 与 archive command 失败；这是 same-host 资格，不是跨机 HA、自动故障
转移或 SLO。详见
[PostgreSQL HA/PITR 实证](docs/evidence/2026-07-16-postgresql-ha-pitr.md)。

W17 随后完成 active-backlog 维度迁移资格，但没有改变默认 profile。四个
tenant 保持 20,000 条当前事实，active `vector_1024` 与 candidate
`halfvec_2560` 同时消费 5,000 条修订、删除和新增 tail event。320 次 scoped
incumbent query 全部成功且 cross-scope 结果为 0；PostgreSQL immediate restart
没有提交 partial candidate row，也没有推进被中断 cursor；原有 pools 恢复后，
两个物理类都收敛到 20,000 行且 lag 为 0；candidate reset/rebuild 不改变
incumbent 与 authority。直连硅基流动 `Qwen/Qwen3-Embedding-4B` 用两次请求
返回并实际使用 2,560 维。candidate 仍未 promotion，lexical 仍是默认。
详见 [Active-Backlog 维度迁移实证](docs/evidence/2026-07-16-active-backlog-dimensional-migration.md)。

W19 随后把“当前是否可用”与“历史是否保留”分开完成资格验证。正式
PostgreSQL 18 profile 在 4 个 tenant、20 条 continuity 中创建 10,000 条
governed memory，完成 `320 / 320` 次 scoped query，返回全部 4,000 条当前
事实；scheduled 提前使用、expired/archived/deleted 误用、Global Default
污染和 cross-scope 泄漏均为 0。projection rebuild、immediate restart 与
restore 保持有效状态 fingerprint 一致，forgotten 内容没有复活。报告同时
绑定真实 Grok Web Chat、Grok MCP 和官方 Codex MCP 轨迹；直连硅基流动
`BAAI/bge-m3` 用两次请求返回并使用 1,024 维。16 个 hard gate 全部通过。
expiry 与 archive 保留可审查历史，不等于 deletion；普通 working input 仍需
单独治理后才能成为 durable memory。详见
[记忆有效性与保留实证](docs/evidence/2026-07-16-memory-eligibility-retention.md)。

W20 随后把官方 Hermes `v0.18.2` CLI 作为真实 conversation client 完成资格
验证。两个隔离 Hermes session 在显式 Vermory link 前互不可见；link 后，直连
硅基流动 `deepseek-ai/DeepSeek-V4-Flash` 的真实 turn 只消费一条已确认当前
memory，并返回当前合成论文包名。reverse 后对 session B 的全新 delivery 为 0
字节。只停止 Hermes 专用 Vermory canary 时，Hermes 仍返回可见模型答案，而
Vermory 没有生成虚假持久化 receipt。模型审计、Mac mini 用户级 LaunchAgent
重启、确定性发布包和隐私门均通过。详见
[Hermes 真实客户端连续性实证](docs/evidence/2026-07-18-hermes-real-client.md)。

W21 完成了一条真实 conversation write-back 闭环。OpenClaw 持久化用户 turn，
直连硅基流动 `deepseek-ai/DeepSeek-V4-Flash` 形成可审查 candidate；operator
接受三条事实并拒绝一条仅用于生命周期控制的额外 candidate。后续纠正把 Friday
替换为 Saturday 10:00；仅本轮使用英文的要求没有形成 candidate，也没有污染
Global Defaults。删除合成 access code 后，governed memory、observation、answer、
delivery history、lexical projection、formation run、formation item 与检查过的
OpenClaw 隔离 state 中精确残留均为 0。新的真实 OpenClaw/Grok turn 正确回答
Saturday 10:00 与 concierge 要求。Mac mini 上还验证了跨 continuity 拒绝、
manifest 外证据拒绝、离线 replay、input drift、active snapshot drift 与 fail-open。
报告保留 provider timeout、无效输出、客户端回答失败，以及本轮发现并修复的
failed-audit 删除缺口。详见
[Conversation Formation Loop 实证](docs/evidence/2026-07-18-conversation-formation-loop.md)。

W22 把 conversation formation 改成真实客户端内可审查的异步闭环。OpenClaw
和 Hermes 完成 turn 后只入队同 continuity 的 durable work，不等待模型；受限
worker 形成 candidate，独立 operator 完成 accept、reject、correct 和 forget。
真实 OpenClaw/Grok 只召回当前已接受事实，Hermes session 保持独立，worker
停启、completion replay、RLS、package、checksum 与隐私门均通过。详见
[自动 Conversation Formation 与审查实证](docs/evidence/2026-07-18-automatic-conversation-review.md)。

W24 完成受保护制品签名资格。普通 PR test job 在无 OIDC 权限的情况下构建并
验证覆盖 4 个 Go 归档、GoReleaser checksum、OpenClaw、Hermes 与 Hermes
sidecar 的完整 manifest；独立的 same-repository post-test job 使用 GitHub OIDC
和固定 Cosign `v3.0.6` 对 manifest 做 keyless 签名，并验证精确 workflow identity
与 issuer，同时拒绝篡改 manifest 和错误 workflow identity。签名产物通过青岛
反向管理隧道直接流到 ARM64 Mac mini 独立验签，全程没有 `sudo`、系统级 Cosign、
长期私钥、tag 或 GitHub Release。`test` 和 `sign-snapshot` 都是 strict required
checks。详见[受保护制品签名实证](docs/evidence/2026-07-18-protected-artifact-signing.md)。

完整状态见 [Experiment 0 读数](docs/experiment-0-readout.md)。

## 快速开始

要求：

- Go `1.25.7` 或兼容的新版本；
- 使用 `jq` 查看生成的 JSON 证据；
- 只有运行原生后端和旧垂直切片时才需要 PostgreSQL。

```bash
go test ./...
go test -race ./internal/reality
go vet ./...
```

```bash
go run ./cmd/vermory --help
```

对官方 LongMemEval-S cleaned artifact 运行不依赖 LLM provider 的全量检索
资格：

```bash
go run ./cmd/vermory benchmark-longmemeval-retrieval \
  --database-url "$VERMORY_BENCHMARK_DATABASE_URL" \
  --source-dataset /path/to/longmemeval_s_cleaned.json \
  --implementation-revision "$(git rev-parse HEAD)" \
  --run-id longmemeval-s-full-retrieval
```

只有 checkpoint 的 run、revision、dataset 和 record-set 全部一致时才使用
`--resume`。

取得固定 source、已资格化的 W14 retrieval JSONL 和隔离 provider command
后，运行全量 reader QA：

```bash
go run ./cmd/vermory benchmark-longmemeval-qa \
  --source-dataset /path/to/longmemeval_s_cleaned.json \
  --retrieval-results /path/to/w14/retrieval-results.jsonl \
  --implementation-revision "$(git rev-parse HEAD)" \
  --run-id longmemeval-s-full-reader-qa \
  --reader-command /path/to/isolated-grok-wrapper \
  --judge-command /path/to/isolated-grok-wrapper
```

已提交结果使用 custom Grok judge，不是官方 GPT-4o judge。详见
[LongMemEval-S 全量 Reader QA 实证](docs/evidence/2026-07-15-longmemeval-s-full-reader-qa.md)。

验证冻结案例：

```bash
go run ./cmd/vermory reality-validate \
  --case-root reality/cases \
  --artifact-root ./artifacts \
  --run-id experiment-0-public-v1
```

生成 Experiment 0 报告：

```bash
go run ./cmd/vermory experiment-0 \
  --case-root reality/cases \
  --artifact-root ./artifacts \
  --run-id experiment-0-v1
```

## 本地工作区治理

普通 AI 客户端只通过 MCP 获取已确认工作区的有效上下文，并把任务结果写回为待确认观察。工作区确认、来源事实记录、指定事实纠正和指定事实遗忘由本机操作者显式执行，不作为模型工具开放。完整命令与边界见[本地工作区治理指南](docs/integrations/local-operator-workspace-slice.md)；[Codex MCP 真实客户端实证](docs/evidence/2026-07-14-codex-mcp-real-client.md)记录了 Codex 自行调用 `prepare_context`、生成并验证文件、调用 `commit_observation`，以及 PostgreSQL 将结果保持为 `proposed` 的完整链路。

可信来源发生变化时，`memory revise-source` 会替代一个被明确指定的当前事实；它不会用语义相似度猜测目标，也不会覆盖同工作区中的无关事实。[显式来源修订运行实证](docs/evidence/2026-07-14-source-revision-runtime.md)记录了软件发布命令更新、投影重建、旧事实精确与改写探针、真实 Grok MCP 消费回写，以及本轮 Codex 因模型或账户配额在 MCP 前失败的边界。

当可信 ingestor 能提供稳定事实 key 时，`memory propose-source` 会先生成可审查候选，不改变当前检索；操作者再使用 `memory accept-candidate` 或 `memory reject-candidate` 明确裁决，普通 MCP 客户端看不到 proposed/rejected 内容。[来源冲突候选运行实证](docs/evidence/2026-07-14-source-conflict-candidate-runtime.md)记录了拒绝、接受、跨租户阻断、投影重建、旧事实原文与改写探针，以及真实 Grok MCP 消费回写的完整链路。

当可信来源只有精确事实与 revision、没有 Vermory 内部 key 时，`memory match-source` 会让已配置 provider 只从当前 workspace 的闭集 key 中选择一个目标或 abstain。provider 不能创造 authority、跨 scope 或直接激活 memory；合法匹配只会形成原有的可审查 source candidate。[无 key 来源目标匹配运行实证](docs/evidence/2026-07-14-unkeyed-source-target-matching-runtime.md)记录了真实 Grok matched/abstained、proposal 隔离、显式接受、RLS 审计、投影重建、stale probes 与真实 MCP coder 回写。该能力是闭集匹配，不是任意文档抽取。

## 生产检索运行线

W09 把 active-only PostgreSQL/pgvector 检索接到了真实 workspace MCP、Web
Chat 和 authenticated API。固定租户的受限 worker 消费 durable projection
events；`shadow` 和 `vector` 必须显式启用，默认仍是 lexical。projection
lag 或 embedding provider 故障时，运行时按原 ID 与顺序退回 lexical，不能
影响 PostgreSQL authority。

真实回放覆盖了 Grok MCP 语义事实与技术标识消费、proposed 回写、链接会话
Web Chat、shadow 字节等价、cursor lag、HTTP 503、vector 清空重建、RLS、
原生 dump/restore 和恢复库重建。该结果只表示 `production_path_integrated`，
不表示已经切换默认检索，也不表示完成规模、embedding migration 或最终发布
验收。详见[生产检索运行实证](docs/evidence/2026-07-14-production-retrieval-runtime.md)。

## 发布产物

每个 Pull Request 都会生成保留 7 天的可下载签名 snapshot，包括带 SHA-256 校验的 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 归档，以及独立的 `@vermory/openclaw` 包和确定性的 `vermory-hermes-0.1.0.tar.gz` provider 包。每个 Go 归档固定包含 `vermory`、`LICENSE`、`README.md` 和 `README.zh-CN.md`。

snapshot 还包含恰好覆盖 8 个 payload 的 `release-manifest.sha256`，以及绑定精确 GitHub workflow identity 的 keyless Sigstore bundle `release-manifest.sigstore.json`。普通 test job 没有 OIDC 权限，只有在受保护测试成功后，独立 `sign-snapshot` job 才能签名。

```bash
vermory version
```

发布二进制会输出注入的版本、完整 revision、构建时间和 Go runtime 版本。手动 Release workflow 只生成不发布的 snapshot；只有 `v*` tag 可以创建 draft GitHub Release。当前 Draft PR 不创建 tag，也不创建 GitHub Release。精确 checksum、两次构建可复现性、Actions 下载产物、本机执行和明确不承诺项见[发布打包实证](docs/evidence/2026-07-14-release-packaging.md)；完整 manifest 签名、identity、负向控制与 Mac mini 独立验签见[受保护制品签名实证](docs/evidence/2026-07-18-protected-artifact-signing.md)。

## OpenClaw 接入

`@vermory/openclaw` 使用 OpenClaw 的 canonical `sessionKey` 和 `runId`：在 `before_prompt_build` 注入当前有效的语义上下文，在 `agent_end` 记录最终 turn lifecycle。它不替代 OpenClaw 的 transcript、memory slot、渠道或模型路由。

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw install --frozen-lockfile
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw check
```

loopback 部署、OpenClaw trust 配置、runtime inspection、确认/纠正/删除、显式 link、故障语义、隔离状态重放和卸载步骤见 [OpenClaw 运行接入指南](docs/integrations/openclaw-runtime.md)。

## Hermes 接入

Hermes 的 `vermory` `MemoryProvider` 使用 Hermes 持久化的 CLI 或 gateway
session 标识：在模型 turn 前准备当前有效的治理上下文，在 turn 完成后记录用户与
助手 observation。确认、纠正、删除和桥接仍由 Vermory 显式治理接口负责，不暴露
为模型工具。不同 Hermes session 默认隔离，只有操作者显式 link 后才共享 governed
memory。

以下命令把 uv 环境放在 `/tmp`，并禁止 Python bytecode 污染工作区：

```bash
PYTHONDONTWRITEBYTECODE=1 \
UV_PROJECT_ENVIRONMENT=/tmp/vermory-hermes \
  uv run --project integrations/hermes --locked \
  python -m unittest discover -s integrations/hermes/tests -v
```

`integrations/hermes/package.sh` 生成确定性的
`vermory-hermes-0.1.0.tar.gz` 发布包。冻结案例
`H01-hermes-linked-sessions` 要求真实模型调用、显式跨 session link、旧事实排除、
reverse 后直接检查新 delivery、无关 continuity 隔离、Vermory 不可用时 Hermes
仍返回可见答案，以及凭据泄漏数严格为 0。具体见
[Hermes 接入指南](integrations/hermes/README.md)与
[Hermes 真实客户端连续性实证](docs/evidence/2026-07-18-hermes-real-client.md)。

authenticated 部署、token 生命周期、runtime role 授权、TLS 规则、RLS 验证、备份、恢复、投影重建与撤销边界见[身份授权与 PostgreSQL RLS 指南](docs/integrations/identity-authorization-rls.md)。[身份授权实证](docs/evidence/2026-07-14-identity-authorization-rls.md)包含确定性租户隔离硬门和真实 OpenClaw/Grok 认证回放；[PostgreSQL 运维恢复实证](docs/evidence/2026-07-14-postgresql-operations-recovery.md)记录原生 dump/restore、投影丢失与重建、数据库中断恢复；[PostgreSQL HA/PITR 实证](docs/evidence/2026-07-16-postgresql-ha-pitr.md)记录 streaming standby 提升、精确 LSN 恢复、历史状态隔离、投影重建和凭据再治理。

## 开发原则

后续能力按照以下顺序推进：

```text
真实失败或长期轨迹
-> 冻结当前事实、禁止行为和验收标准
-> 建立 public、withheld 和 sealed 证据
-> 跑简单基线
-> 提出可证伪实现假设
-> 接入真实客户端执行
-> 验证删除、故障、迁移和规模
```

不要因为某个表、状态机、服务或中间件看起来“架构完整”就直接把它写死。永久设计必须先对应真实案例和可证伪假设。

## 协作与治理

新贡献者先阅读[架构说明](ARCHITECTURE.md)、[开发指南](DEVELOPMENT.md)和[贡献指南](CONTRIBUTING.md)。[仓库协作规程](docs/collaboration/repository-workflow.md)定义 issue、Reality case、评审、合并、协作者权限和发布流程；[治理规则](GOVERNANCE.md)定义维护者与重大决策权限。

所有改动通过 Pull Request 进入受保护的 `main`，最新提交必须通过 `test` 与 `sign-snapshot`。PR snapshot 只是带身份签名的临时测试制品，不是正式发布。真实失败必须保留，替代客户端或替代 provider 只能形成另一份证据，不能把原失败改写成成功。

安全问题按[安全策略](SECURITY.md)私下报告。Reality case 和公开证据不得包含凭据、私有原始对话、未脱敏个人路径或虚假的 `sealed` 标签。

## 许可证

项目采用 [Apache License 2.0](LICENSE)。
