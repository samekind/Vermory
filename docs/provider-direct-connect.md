# Vermory Direct Provider Connectivity

> Direct provider compatibility evidence. It includes the historical ContextMesh self-case, the Vermory W27 reality-case utility run, and the W28 full-dataset embedding qualification; none is a ranking of models for Vermory.

## Purpose

This document records the direct provider path retained by the Vermory legacy evaluation harness. It is intentionally independent from aggregation gateways and private control-plane services.

## Runtime Rule

- Secrets are provided only through environment variables.
- Provider URLs and model names are runtime flags, not source-controlled configuration.
- OpenAI-compatible providers use direct `/v1/chat/completions` calls.
- The locally authenticated Grok CLI is supported as a separate client harness; it does not route through NewAPI, a Mac mini gateway, or a private control plane.

## Supported Direct Provider Modes

### 1. Grok CLI

- Provider flag: `--provider grok-cli`
- Authentication: current local `grok` CLI login; the adapter does not read an API key or base URL.
- Default model: `grok-4.5`
- Each request is a fresh single turn with `--no-memory`, `--disable-web-search`, `--no-plan`, `--no-subagents`, `--max-turns 1`, and JSON output.
- Verified real casebook runs:
  - Workspace consumer: `vermory-grok-workspace-v2`
  - Workspace acceptance: `vermory-grok-workspace-acceptance-v2`
  - Conversation consumer: `vermory-grok-conversation-v4`
  - Conversation acceptance: `vermory-grok-conversation-acceptance-v2`

### 2. SiliconFlow

- Provider flag: `--provider siliconflow`
- Default base URL: `https://api.siliconflow.cn/v1`
- Default API key env: `SILICONFLOW_API_KEY`
- Verified real run:
  - Model: `deepseek-ai/DeepSeek-V4-Flash`
  - Command path: `vermory eval-self-case`
  - Artifact run ID: `siliconflow-deepseek-v4-flash-smoke`
- Verified probe run:
  - Artifact run ID: `siliconflow-probe-selected`
  - `Qwen/Qwen3-Coder-30B-A3B-Instruct`: clean `OK`
  - `Qwen/Qwen3-30B-A3B-Instruct-2507`: clean `OK`
  - `deepseek-ai/DeepSeek-V4-Flash`: timed out in direct probe mode under current client timeout, even though the full self-case run had succeeded earlier
- Verified W27 real-utility run:
  - Model: `deepseek-ai/DeepSeek-V4-Flash`
  - Evidence: `docs/evidence/2026-07-19-w27-real-utility-comparison.md`
  - `24/24` direct provider calls completed with `0` provider failures
  - Four reality cases executed across six context conditions
  - Vermory native completed `4/4` cases with `0` forbidden hits and `816` total delivered context bytes
- Verified W28 full-dataset retrieval run:
  - Embedding model: `BAAI/bge-m3`
  - Retrieval profile: `siliconflow-bge-m3-1024-chunked-mean-v2` (candidate)
  - Evidence: `docs/evidence/2026-07-20-longmemeval-s-full-vector-retrieval.md`
  - `23,867` governed session memories projected and `500/500` vector queries completed
  - Logical items: `24,367/24,367`; physical provider items: `46,657/46,657`
  - Provider attempts: `25,455`, including `1,088` retried failed attempts and `0` terminal failures
  - No NewAPI route, degraded vector query, scope/lifecycle violation, or profile activation

### 3. Duojie

- Provider flag: `--provider duojie`
- Default base URL: `https://api.duojie.games/v1`
- Default API key env: `DUOJIE_API_KEY`
- Verified real runs:
  - Model: `gemini-3-flash`
  - Artifact run ID: `duojie-gemini-3-flash-smoke`
  - Model: `gemini-3.1-pro`
  - Artifact run ID: `duojie-gemini-3.1-pro-smoke`
  - Model: `glm-5`
  - Artifact run ID: `duojie-glm-5-smoke`
  - Probe matrix artifact run ID: `duojie-probe-full`

## Duojie Model Probe Notes

The following quick probes were executed through direct `/v1/chat/completions` calls:

- `gemini-3-flash`: usable, returns standard assistant `content`, upstream reported model alias `gemini-3-flash-preview`
- `gemini-3.1-pro`: usable, returns standard assistant `content`
- `glm-5`: usable, returns `content` plus extra `reasoning_content`, and now passes through the current provider adapter
- `glm-5-turbo`: probe returned large reasoning text instead of a stable final answer
- `glm-5.1`: probe returned polluted output such as `OK</arg_value>`

Current engineering decision:

- These models are retained as test targets, not merely recommended defaults.
- A model may still be a valid test target even if it is slow, noisy, or currently unstable.
- `glm-5-turbo` and `glm-5.1` remain covered as probe targets, with their observed output quality retained in artifacts.
- `deepseek-ai/DeepSeek-V4-Flash` remains an explicit SiliconFlow compatibility target. W27 completed all 24 direct calls, while the earlier probe timeout remains retained operational evidence that long-running qualifications need bounded retry and recovery rather than assuming continuous provider availability.

## Verified Commands

### Mock baseline

```bash
go run ./cmd/vermory eval-self-case \
  --provider mock \
  --model mock-model \
  --run-id mock-direct-smoke \
  --artifact-root ./artifacts-provider-smoke
```

### Grok CLI casebook runs

```bash
go run ./cmd/vermory eval-casebook \
  --provider grok-cli \
  --model grok-4.5 \
  --case-dir ./casebook/cases/101-workspace-parallel-repos \
  --line workspace \
  --run-id vermory-grok-workspace-v2 \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 512
```

```bash
go run ./cmd/vermory eval-casebook \
  --provider grok-cli \
  --model grok-4.5 \
  --case-dir ./casebook/cases/201-conversation-housing-search \
  --line conversation \
  --run-id vermory-grok-conversation-v4 \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 512
```

### SiliconFlow

```bash
SILICONFLOW_API_KEY='***' \
go run ./cmd/vermory eval-self-case \
  --provider siliconflow \
  --model deepseek-ai/DeepSeek-V4-Flash \
  --run-id siliconflow-deepseek-v4-flash-smoke \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 256
```

### Duojie

```bash
DUOJIE_API_KEY='***' \
go run ./cmd/vermory eval-self-case \
  --provider duojie \
  --model gemini-3-flash \
  --run-id duojie-gemini-3-flash-smoke \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 256
```

```bash
DUOJIE_API_KEY='***' \
go run ./cmd/vermory eval-self-case \
  --provider duojie \
  --model gemini-3.1-pro \
  --run-id duojie-gemini-3.1-pro-smoke \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 256
```

```bash
DUOJIE_API_KEY='***' \
go run ./cmd/vermory eval-self-case \
  --provider duojie \
  --model glm-5 \
  --run-id duojie-glm-5-smoke \
  --artifact-root ./artifacts-provider-smoke \
  --max-tokens 256
```

```bash
DUOJIE_API_KEY='***' \
go run ./cmd/vermory probe-provider \
  --provider duojie \
  --models gemini-3-flash,gemini-3.1-pro,glm-5,glm-5-turbo,glm-5.1 \
  --run-id duojie-probe-full \
  --artifact-root ./artifacts-provider-smoke \
  --prompt '不要输出推理过程、不要任何标签或解释，只回复 OK' \
  --max-tokens 128
```

### SiliconFlow probe

```bash
SILICONFLOW_API_KEY='***' \
go run ./cmd/vermory probe-provider \
  --provider siliconflow \
  --models deepseek-ai/DeepSeek-V4-Flash,Qwen/Qwen3-Coder-30B-A3B-Instruct,Qwen/Qwen3-30B-A3B-Instruct-2507 \
  --run-id siliconflow-probe-selected \
  --artifact-root ./artifacts-provider-smoke \
  --prompt '不要输出推理过程、不要任何标签或解释，只回复 OK' \
  --max-tokens 128
```

## Artifact Paths

- `artifacts-provider-smoke/platform-runs/mock-direct-smoke`
- `artifacts-provider-smoke/platform-runs/siliconflow-deepseek-v4-flash-smoke`
- `artifacts-provider-smoke/platform-runs/duojie-gemini-3-flash-smoke`
- `artifacts-provider-smoke/platform-runs/duojie-gemini-3.1-pro-smoke`
- `artifacts-provider-smoke/platform-runs/duojie-glm-5-smoke`
- `artifacts-provider-smoke/provider-probes/duojie-probe-full`
- `artifacts-provider-smoke/provider-probes/siliconflow-probe-selected`
- `artifacts-provider-smoke/casebook-runs/vermory-grok-workspace-v2`
- `artifacts-provider-smoke/casebook-runs/vermory-grok-conversation-v4`

Each run stores:

- `input.md`
- `packet.md` for packet baseline
- `output.md`
- `raw.json`
- `score.json`
- `report.md`

## Scope Boundary

These runs prove:

- the retained harness can call direct providers without an aggregation gateway
- real provider outputs can be captured into repeatable artifacts
- the four-baseline evaluation loop is operational
- the locally authenticated Grok CLI can consume a packet in an isolated single-turn harness
- direct SiliconFlow embeddings can drive the registered PostgreSQL projection and vector coordinator over the complete public LongMemEval-S corpus with exact logical/physical request accounting

These runs do not yet prove:

- final WCEF quality
- AI coding tool integration quality
- browser or MCP consumption quality
- broad real-world advantage beyond the four bounded W27 reality cases
- production-default suitability of the W28 candidate profile; the measured public-corpus gain does not by itself satisfy promotion

The Grok harness deliberately disables tools and browser/search behavior. Its evidence is therefore packet-consumption evidence, not proof that a coding agent completed an implementation task.

## Current Test Coverage Status

### Duojie

- Matrix-covered:
  - `gemini-3-flash`
  - `gemini-3.1-pro`
  - `glm-5`
- Probe-covered:
  - `glm-5-turbo`
  - `glm-5.1`

Coverage note:

- `gemini-3-flash`, `gemini-3.1-pro`, and `glm-5` are fully integrated into the current self-case matrix workflow.
- `glm-5-turbo` and `glm-5.1` are still valid compatibility-test objects, but currently only at the probe layer because their outputs are not clean enough for routine matrix use.

### SiliconFlow

- Matrix-covered:
  - `Qwen/Qwen3-Coder-30B-A3B-Instruct`
  - `Qwen/Qwen3-30B-A3B-Instruct-2507`
- W27 real-utility covered:
  - `deepseek-ai/DeepSeek-V4-Flash`
- W28 full-dataset retrieval covered:
  - `BAAI/bge-m3` embedding through `siliconflow-bge-m3-1024-chunked-mean-v2`
- Probe-covered:
  - `deepseek-ai/DeepSeek-V4-Flash`
- Self-case covered:
  - `deepseek-ai/DeepSeek-V4-Flash`

Coverage note:

- The two Qwen models have full SiliconFlow matrix coverage.
- `deepseek-ai/DeepSeek-V4-Flash` completed the W27 four-case, six-condition direct utility run with 24/24 calls and zero provider failures.
- Its earlier direct-probe timeout remains part of the evidence record; the later successful run establishes compatibility, not guaranteed provider availability or model superiority.
- `BAAI/bge-m3` completed W28 with exact `46,657` physical provider items and zero terminal failure, but 1,088 retry events and a 7,210-second projection remain material operational evidence rather than being hidden behind the retrieval score.
