# Grok Global Defaults Runtime Evidence

Replay completed: 2026-07-14 Asia/Shanghai

Runtime: Vermory loopback Web Chat API plus external MCP stdio client

Provider: authenticated Grok CLI `0.2.99`

Model: `grok-4.5`

Authority: PostgreSQL under isolated tenant `g01-grok-20260714`

## Result

| Gate | Result |
|---|---|
| Explicit set | `reply_language` created as one active global default with durable continuity, observation, and memory receipts |
| Task-local override | A real Grok turn explicitly limited to one English task returned `The table-facing deliverable will be English.` |
| No global mutation | Immediate inspection still showed the original Chinese default as the only active revision |
| Later unrelated task | A new thread returned `局部英文要求只作用于当前任务，不会修改全局默认（默认仍用中文回复）。` |
| Explicit correction | The original revision became superseded and a corrected Chinese plus concise-Markdown revision became active |
| Corrected real behavior | Grok replied in Chinese with one concise Markdown list item |
| Workspace consumption | An external MCP stdio client received only the corrected semantic default under `Global defaults:` |
| Metadata isolation | The MCP context contained no key, UUID, lifecycle field, or audit metadata |
| Explicit deletion | The corrected active revision returned `memory_status=deleted` and its content became `[redacted]` |
| Fresh workspace after deletion | A new MCP `prepare_context` returned `context:""` |
| Fresh chat after deletion | The persisted post-delete chat delivery had an empty context body |
| Authority after deletion | Active global-default count was `0`; active global-default projection count was `0` |

## Durable Receipts

The replay produced these non-sensitive identifiers:

- global-default continuity: `1925dced-6f79-4faa-ab9e-245bed3838ed`;
- initial active memory: `d1ff177d-4afb-4c1f-836e-2411c3d0ce46`;
- corrected active memory: `a6601267-2f66-4d6e-8dfe-60bbd78d0cc6`;
- English local-override turn: `4431f5e8-f2ad-44b1-a4c9-0d3dd7b406c7`;
- unrelated Chinese turn: `ffeb894d-e3a7-4a05-b0cf-58dfa696625a`;
- corrected-default Grok turn: `a3ab34fa-2161-495e-8b73-6d8b26bd4666`;
- corrected workspace MCP delivery: `16912372-7032-4bfa-94c7-444292172d2d`;
- post-delete workspace MCP delivery: `b2195fc6-6094-4cae-9f65-e9453dde50c9`.

## External MCP Proof

Before deletion, the external MCP client launched the built `vermory mcp-stdio` binary through the official Go MCP `CommandTransport` and called `prepare_context` for a confirmed workspace. The structured result was:

```json
{
  "context": "Global defaults:\nDefault user-facing replies to Chinese with concise Markdown unless the active task explicitly requests another language.",
  "delivery_id": "16912372-7032-4bfa-94c7-444292172d2d",
  "status": "resolved"
}
```

After deletion, the same external client used a fresh operation ID and received:

```json
{
  "context": "",
  "delivery_id": "b2195fc6-6094-4cae-9f65-e9453dde50c9",
  "status": "resolved"
}
```

This proves the workspace path consumed the global layer through the actual MCP process boundary rather than an in-process helper.

## PostgreSQL Post-Delete Proof

A direct read-only probe against the replay tenant returned:

```json
{
  "active_default_projections": 0,
  "active_defaults": 0,
  "post_delete_chat_context": ""
}
```

The result distinguishes authority from model behavior: deletion success is established by authoritative lifecycle state, the rebuildable projection, and fresh consumer deliveries.

## Non-Diagnostic Model Probe Preserved

A post-delete prompt asked Grok to answer `HAS_DEFAULT` or `NO_DEFAULT` by introspecting whether reference context contained a stable language rule. Grok answered `HAS_DEFAULT` even though the persisted delivery context was empty and both authority counts were zero.

That probe is retained as a failed evaluation design, not reported as a platform failure or success. The conversation system prompt itself mentions the concept of global defaults, so the model could infer the label without seeing an active semantic default. Vermory therefore does not use model self-report as deletion evidence.

## Automated Hard Gates

The real replay is complemented by deterministic G01 and S01 acceptance:

- G01 injects the same active default into Web Chat and workspace consumers, proves local override non-mutation, then replays both paths after correction and deletion.
- S01 imports an instruction asking for promotion through both conversation and workspace source paths and proves the Global Defaults layer remains empty.
- Store tests enforce singleton continuity, one active value per key, idempotent receipts, cross-tenant rejection, redaction, and projection rebuild behavior.

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
```

No API keys, login material, private source content, or deleted sensitive values are present in this evidence.
