# OpenClaw Runtime Evidence

Replay completed: 2026-07-14 Asia/Shanghai

Runtime: official OpenClaw plugin, loopback Vermory HTTP API, PostgreSQL, authenticated local Grok CLI

Provider/model: `grok-cli/grok-4.5`

Replay tenant: `o01-real-v3`

## Claim Boundary

This replay proves the OpenClaw integration boundary, not model optimality. OpenClaw owns the canonical session key, local transcript, model selection, and inference. Vermory owns governed continuity, Global Defaults, explicit bridge operations, lifecycle storage, and delivery audit.

Model wording is behavioral evidence. Isolation, correction, deletion, restart durability, model attribution, and outage persistence claims come from PostgreSQL records and fresh delivery bodies.

## Runtime Inventory

| Component | Version or value |
|---|---|
| Node.js | `v24.18.0` |
| pnpm | `11.12.0` |
| OpenClaw | `2026.6.11 (e085fa1)` |
| Grok CLI | `0.2.99 (b1b49ccb71a7)` |
| PostgreSQL database | `vermory_openclaw_20260714_v3` |
| Schema version | `8` |
| OpenClaw state | `/tmp/vermory-openclaw-state-v3` |
| OpenClaw config | `/tmp/vermory-openclaw-config-v2/openclaw.json` |
| OpenClaw workspace | `/tmp/vermory-openclaw-workspace-v2` |
| Vermory binary | `/tmp/vermory-openclaw-runtime-v2/vermory` |
| Binary SHA-256 | `f8e76098ddce0d1b054ec3f7bb5173d530de14ef55a9dc39e6a89822dee50b39` |

No Gemini CLI or Mac mini NewAPI route was used. The backend cleared common OpenAI, Anthropic, xAI API-key, Grok API-key, and NewAPI environment variables.

Grok ran through an isolated wrapper that set separate `HOME` and `GROK_HOME` values while reusing the operator's authenticated Grok session material outside the repository. Runtime inspection returned:

```json
{
  "grokVersion": "0.2.99",
  "pluginCount": 0,
  "mcpCount": 0
}
```

The final replay used stateless Grok single-turn execution with native tools disabled. This avoids making Vermory continuity depend on Grok's private session store and prevents user-level Grok/Claude plugins from contaminating structured output. OpenClaw still retained the canonical session key and transcript; Vermory continuity survived process and backend-session changes through PostgreSQL.

## Plugin Runtime Boundary

Official runtime inspection returned:

```json
{
  "id": "vermory",
  "status": "loaded",
  "enabled": true,
  "activated": true,
  "shape": "hook-only",
  "capabilityMode": "none",
  "capabilityCount": 0,
  "typedHooks": ["agent_end", "before_prompt_build"],
  "tools": [],
  "providers": [],
  "channels": []
}
```

The plugin did not claim a memory slot, model provider, channel, tool, model router, or inference capability. Both `sessionKey` and `runId` remained mandatory. The server, not the plugin, owned tenant and channel mapping.

## Model Attribution Fix

The first real run exposed that generic OpenClaw CLI backends may omit `modelProviderId` and `modelId` from `agent_end` context even though the final assistant message contains authoritative `provider` and `model` fields.

Commit `00bd525` added a failing test from the real event shape and changed completion persistence to use the model metadata attached to the final visible assistant message, with hook context as a fallback.

Final database result:

```json
{
  "completed_wrong_model": 0,
  "expected_model": "grok-cli/grok-4.5"
}
```

## O01 Real Replay

### Initial State And Governance

Session A initially returned:

```text
NO_GOVERNED_MEMORY_YET
```

Three real Grok turns then produced exact assistant observations:

```text
The plumbing inspection is Friday at 15:30.
The technician must check in with the concierge.
The temporary access code is CEDAR-4826.
```

Only those three observations were explicitly confirmed. The rain/lunch turn remained draft chatter.

### Process Restart

Before restart, loopback listeners were:

```text
Vermory PID 88761 on 127.0.0.1:8787
OpenClaw PID 89531 on 127.0.0.1:18789
```

After stopping and restarting both processes against the same PostgreSQL database and OpenClaw state:

```text
Vermory PID 5728 on 127.0.0.1:8787
OpenClaw PID 6532 on 127.0.0.1:18789
```

Fresh session A delivery:

```text
Governed memory:
The temporary access code is CEDAR-4826.
The technician must check in with the concierge.
The plumbing inspection is Friday at 15:30.
```

Grok returned all three governed facts and excluded rain/lunch chatter.

### Link And Isolation

Before link, session B delivery length was `0` and Grok returned:

```text
NO_LINKED_MEMORY_YET
```

Bridge `8395007e-3bdc-4ae6-b4df-ea7b81276707` explicitly linked A to B. A fresh B delivery contained the three governed facts and no rain/lunch text. Grok returned the appointment, concierge requirement, and access code.

Unrelated session C delivery length was `0`; Grok returned:

```text
NO_GOVERNED_DETAILS_AVAILABLE
```

### Correction And Deletion

The appointment was corrected from Friday 15:30 to Saturday 10:00. The old memory became `superseded`; the new memory `40e7b3a7-3283-440a-a107-69b0c19009d6` became active.

Deleting synthetic access-code memory `c532c183-9f37-42c5-a68e-c5792a8465c8` during the first live attempt exposed that downstream assistant observations could retain a secret they had previously echoed. Commit `3c1ac1b` added failing runtime and linked-session acceptance gates, then changed deletion propagation to redact every assistant observation and persisted answer generated from a delivery that contained the deleted memory.

The clean final replay, including projection rebuild, returned:

```json
{
  "observations": 0,
  "governed_memories": 0,
  "search_projection": 0,
  "delivery_history": 0,
  "turn_answers": 0,
  "exact_search_matches": 0,
  "paraphrase_search_matches": 0,
  "active_old_appointment": 0,
  "active_new_appointment": 1
}
```

A fresh linked B turn received only:

```text
Governed memory:
The technician must check in with the concierge.
The plumbing inspection is Saturday at 10:00.
```

Grok answered with Saturday 10:00, the concierge requirement, and `NO_ACCESS_CODE_AVAILABLE`. A separate paraphrased cedar-style probe reported that no such sequence was present.

### Global Default

The stable default was set to:

```text
默认使用中文回答，除非当前任务明确要求其他语言。
```

An explicit one-turn English override returned:

```text
The governed plumbing inspection appointment is Saturday at 10:00.
```

Immediate inspection still showed exactly one active Global Default with the original Chinese content. A later English prompt asked the model to follow the stable reply-language default; Grok answered in Chinese and repeated the active rule.

### Link Reversal

The A-B link was reversed through bridge operation `o01-v3-reverse-a-b`. A fresh B delivery contained only the Global Default and no appointment or concierge facts. Grok returned:

```text
NO_LINKED_MEMORY_AFTER_REVERSAL
```

## Outage Proof

Vermory was stopped while the OpenClaw Gateway remained online. A fresh OpenClaw/Grok turn returned:

```text
OPENCLAW_STILL_AVAILABLE_WITH_VERMORY_DOWN
```

Gateway logs recorded both bounded failures:

```text
Vermory prepare failed; continuing without external continuity context.
Vermory completion persistence failed; OpenClaw result remains available but was not confirmed as persisted.
```

For outage run `6732f89f-57e2-4f20-9980-62fa3b20841f`, PostgreSQL contained `0` conversation turns. Chat availability therefore failed open, while persistence claims failed closed.

## Lifecycle Summary

```json
{
  "turns_completed": 18,
  "turns_failed": 1,
  "completed_wrong_model": 0,
  "deliveries": 19,
  "observations": 43,
  "active_memories": 3,
  "superseded_memories": 1,
  "deleted_memories": 1,
  "active_bridges": 0,
  "reversed_bridges": 1,
  "outage_turns": 0
}
```

The one failed turn was retained rather than hidden: a plain factual message triggered OpenClaw's default workspace instruction to write a local memory file while native tools were disabled. It remained a failed draft and was never governed. Subsequent benchmark turns used an explicit text-only response contract.

## Invalid Or Non-Gate Probes

Two model outputs were retained but excluded from acceptance claims:

- An ambiguous request for the "current visit time" was interpreted as the current clock time. A more specific plumbing-appointment prompt succeeded.
- A Chinese query did not retrieve English governed memory because current conversation retrieval is lexical. The fresh delivery contained only the Global Default. This is evidence for the later multilingual/hybrid-retrieval track, not an OpenClaw lifecycle or injection success.

These results are why model self-report is not used as authority evidence.

## Known Unrelated Runtime Warnings

OpenClaw emitted missing generated-module warnings for bundled `imessage` and `telegram` channel setup. Neither channel was configured or used. OpenClaw also reported `2026.7.1` as available; the replay intentionally stayed on the package-locked and tested `2026.6.11` target.

## Release Verification

Passed after the final replay and deletion fix:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
  ./internal/runtime ./internal/webchat ./internal/operatorcli \
  ./cmd/vermory ./internal/provider

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-openclaw-release ./cmd/vermory
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw check
PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw pack --dry-run
git diff --check
```

Results: all Go packages passed, all selected race-enabled packages passed, plugin tests passed `37/37`, TypeScript typecheck/build passed, and the publishable tarball contained only `dist`, `openclaw.plugin.json`, and `package.json`.

No credentials, auth tokens, private user content, or Mac mini NewAPI configuration are included in this evidence.
