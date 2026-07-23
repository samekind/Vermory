# OpenClaw Runtime Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect an official OpenClaw plugin to Vermory's conversation continuity so real OpenClaw turns receive governed semantic context and persist draft observations without ceding identity, governance, or model ownership to either side.

**Architecture:** Vermory exposes three loopback external-turn routes backed by the existing PostgreSQL conversation lifecycle. A standalone TypeScript OpenClaw plugin maps canonical `sessionKey` and `runId` to those routes from `before_prompt_build` and `agent_end`. OpenClaw owns transcript and inference; Vermory injects only Global Defaults and active governed memory, then records user/assistant observations for later explicit governance.

**Tech Stack:** Go, PostgreSQL 16+, pgx v5, `net/http`, TypeScript 5, Vitest, pnpm 11, OpenClaw 2026.6.11, Node.js 24, local authenticated Grok CLI.

## Global Constraints

- OpenClaw is a client, not Vermory's platform definition.
- The plugin must not declare `kind: "memory"` or occupy `plugins.slots.memory`.
- Tenant and Vermory channel are server-owned; plugin input cannot select either.
- Canonical `sessionKey` and `runId` are both mandatory; missing identity causes abstention.
- Ordinary OpenClaw messages create observations only and never auto-confirm memory.
- OpenClaw owns raw transcript history; Vermory injects only Global Defaults and active governed memory.
- Linked raw conversation history is never pooled.
- Model-facing packets contain semantic content only.
- Plugin failures may not break OpenClaw chat, but may not be reported as successful persistence.
- Real OpenClaw runs use isolated `OPENCLAW_STATE_DIR` and `OPENCLAW_CONFIG_PATH`.
- No Gemini CLI and no Mac mini NewAPI routing.
- Database-mutating Go tests run serially with `VERMORY_TEST_DATABASE_URL`.

---

### Task 1: Freeze O01 Everyday-Use Case

**Files:**
- Create: `reality/cases/O01-openclaw-home-maintenance/manifest.json`
- Create: `reality/cases/O01-openclaw-home-maintenance/events.jsonl`
- Create: `reality/cases/O01-openclaw-home-maintenance/fixtures/home-maintenance-thread.md`
- Create: `reality/cases/O01-openclaw-home-maintenance/fixtures/governance-actions.md`
- Create: `reality/cases/O01-openclaw-home-maintenance/fixture-lock.json`
- Modify: `internal/reality/validate_test.go`
- Modify: `internal/reality/experiment0.go`
- Modify: `internal/reality/experiment0_test.go`

**Interfaces:**
- Consumes: existing reality case schema and fixture-lock format.
- Produces: immutable O01 facts and deterministic checks used by Go acceptance and real OpenClaw replay.

- [x] **Step 1: Write the O01 manifest and trajectory**

Use stable anchors `agent:main:home-maintenance-a`, `agent:main:home-maintenance-b`, and `agent:main:unrelated-c`. Include the current Saturday 10:00 appointment, obsolete Friday 15:30 appointment, concierge check-in, temporary code `CEDAR-4826`, Chinese reply default, explicit A-B link/reversal, and unrelated C isolation.

- [x] **Step 2: Add a failing reality validation test**

Add a table entry that loads O01 and asserts its continuity line includes `conversation`, its pressures include `restart`, `cross_channel_link`, `correction`, `deletion`, `global_default_override`, and `link_reversal`, and all fixture hashes validate.

- [x] **Step 3: Run the test and verify RED**

```bash
go test -count=1 ./internal/reality -run 'Test.*O01'
```

Expected: FAIL because O01 or its fixture lock is absent/incomplete.

- [x] **Step 4: Generate exact SHA-256 lock values and make the test pass**

Use `shasum -a 256` for the manifest, events, and both fixtures. Store byte counts in the same format as existing public cases.

- [x] **Step 5: Verify and commit**

```bash
go test -count=1 ./internal/reality
git diff --check
git add reality/cases/O01-openclaw-home-maintenance internal/reality/validate_test.go internal/reality/experiment0.go internal/reality/experiment0_test.go
git commit -m "test: freeze OpenClaw continuity case"
```

### Task 2: External Conversation Turn Lifecycle

**Files:**
- Modify: `internal/runtime/conversation_types.go`
- Modify: `internal/runtime/conversation_store.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/conversation_service_test.go`

**Interfaces:**
- Consumes: `ConversationAnchor`, `BeginConversationTurn`, `RecordDelivery`, Global Defaults, linked conversation memory search, and existing completion/failure storage.
- Produces:

```go
type ExternalConversationTurnRequest struct {
    OperationID string
    Anchor      ConversationAnchor
    Message     string
}

type PreparedConversationTurn struct {
    ChatTurnReceipt
    Context string `json:"context"`
}

type CompleteExternalConversationTurnRequest struct {
    OperationID string
    Anchor      ConversationAnchor
    Answer      string
    Model       string
}

type FailExternalConversationTurnRequest struct {
    OperationID   string
    Anchor        ConversationAnchor
    FailureCode   string
    FailureMessage string
}

func (s *ConversationService) PrepareExternalTurn(context.Context, ExternalConversationTurnRequest) (PreparedConversationTurn, error)
func (s *ConversationService) CompleteExternalTurn(context.Context, CompleteExternalConversationTurnRequest) (ChatTurnReceipt, error)
func (s *ConversationService) FailExternalTurn(context.Context, FailExternalConversationTurnRequest) (ChatTurnReceipt, error)
```

- [x] **Step 1: Write failing service tests**

Cover:

- prepare stores exactly one user observation and one delivery;
- prepared context contains active Global Defaults and linked governed memory;
- prepared context excludes `Recent conversation:` and all raw sibling observations;
- repeated prepare returns the same turn, delivery, context, and `replayed=true`;
- reusing an operation ID with a different anchor or message fails;
- complete finds the prepared turn by operation ID and exact anchor, stores one assistant observation, and replays idempotently;
- complete rejects another continuity and an unprepared operation;
- fail marks only the exact in-progress turn failed and stores no assistant observation;
- correction/deletion changes later fresh preparations but never mutates an already recorded delivery.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestConversationExternal|TestPreparedConversation'
```

Expected: FAIL because external-turn types and methods do not exist.

- [x] **Step 3: Implement minimal store helpers**

Add:

```go
func (s *Store) AttachConversationTurnDelivery(ctx context.Context, tenantID, turnID, deliveryID string) (ChatTurnReceipt, error)
func (s *Store) LookupConversationTurn(ctx context.Context, tenantID, operationID string) (ChatTurnReceipt, error)
```

`AttachConversationTurnDelivery` must lock the turn, verify delivery tenant/continuity ownership, reject a conflicting delivery, and replay the same binding. `LookupConversationTurn` returns a not-found error instead of an empty receipt.

- [x] **Step 4: Implement prepare/complete/fail and refactor Chat**

`PrepareExternalTurn` performs the existing pre-provider half of `Chat`, but calls `BuildConversationContext(defaults, memories, nil)`. `Chat` calls `PrepareExternalTurn`, invokes its provider only for an in-progress turn, then calls `CompleteExternalTurn`. Provider configuration is required only by `Chat`; external methods require only store and tenant.

- [x] **Step 5: Verify runtime behavior and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime
git diff --check
git add internal/runtime/conversation_types.go internal/runtime/conversation_store.go internal/runtime/conversation_service.go internal/runtime/conversation_service_test.go
git commit -m "feat: add external conversation turns"
```

### Task 3: Server-Owned OpenClaw HTTP Routes

**Files:**
- Modify: `internal/webchat/handler.go`
- Modify: `internal/webchat/handler_test.go`
- Modify: `internal/webchat/acceptance_test.go`
- Modify: `cmd/vermory/web_chat.go`
- Modify: `cmd/vermory/web_chat_test.go`

**Interfaces:**
- Consumes: Task 2 external-turn service.
- Produces:

```text
POST /v1/integrations/openclaw/turns/prepare
POST /v1/integrations/openclaw/turns/complete
POST /v1/integrations/openclaw/turns/fail
```

All routes map `session_key` to `ConversationAnchor{Channel: "openclaw", ThreadID: sessionKey}` inside the server.

- [x] **Step 1: Write failing HTTP tests**

Test valid prepare/complete/fail, replay, conflicting reuse, missing/oversized fields, unknown JSON fields, body-size limit, and absence of accepted `tenant_id`, `continuity_id`, `channel`, `status`, or lifecycle override fields.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat ./cmd/vermory -run 'Test.*OpenClaw|Test.*External'
```

Expected: FAIL with route not found or missing provider mode.

- [x] **Step 3: Implement strict route decoding**

Use the existing bounded decoder and service error mapper. Keep failure text at 512 bytes internally and never serialize it in a turn receipt. Return `200` when prepare/complete/fail state was persisted, including a `status=failed` receipt from the fail route; use non-2xx only for invalid/conflicting requests or Vermory service failure.

- [x] **Step 4: Add `external` provider mode**

`vermory web-chat --provider external` starts the same loopback API with no in-process model provider. `/v1/chat/turn` returns a safe configuration error, while OpenClaw external-turn and governance routes remain available. Existing provider modes and defaults remain unchanged.

- [x] **Step 5: Verify and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat ./cmd/vermory
git diff --check
git add internal/webchat/handler.go internal/webchat/handler_test.go internal/webchat/acceptance_test.go cmd/vermory/web_chat.go cmd/vermory/web_chat_test.go
git commit -m "feat: expose OpenClaw turn API"
```

### Task 4: Official OpenClaw Plugin

**Files:**
- Create: `integrations/openclaw/package.json`
- Create: `integrations/openclaw/pnpm-lock.yaml`
- Create: `integrations/openclaw/tsconfig.json`
- Create: `integrations/openclaw/openclaw.plugin.json`
- Create: `integrations/openclaw/src/config.ts`
- Create: `integrations/openclaw/src/identity.ts`
- Create: `integrations/openclaw/src/messages.ts`
- Create: `integrations/openclaw/src/client.ts`
- Create: `integrations/openclaw/src/index.ts`
- Create: `integrations/openclaw/test/config.test.ts`
- Create: `integrations/openclaw/test/identity.test.ts`
- Create: `integrations/openclaw/test/messages.test.ts`
- Create: `integrations/openclaw/test/client.test.ts`
- Create: `integrations/openclaw/test/plugin.test.ts`

**Interfaces:**
- Consumes: Task 3 HTTP routes and OpenClaw `definePluginEntry`/typed hooks.
- Produces: package `@vermory/openclaw`, plugin id `vermory`, and runtime registrations `before_prompt_build` plus `agent_end`.

- [x] **Step 1: Create package metadata only**

Use Node `>=22.19.0`, pnpm 11, TypeScript ESM, peer dependency `openclaw >=2026.6.11`, dev dependency `openclaw 2026.6.11`, and scripts `test`, `typecheck`, `build`, and `check`. The package extension points to `dist/index.js`. The plugin manifest has no `kind` and no provider/channel ownership.

- [x] **Step 2: Write failing config and identity tests**

Assert config normalization defaults to loopback, rejects non-HTTP(S) and credential-bearing URLs, bounds timeout, and never accepts tenant/continuity fields. Assert identity requires trimmed `sessionKey` plus `runId` and returns operation ID `openclaw:<runId>` without using fallback metadata.

- [x] **Step 3: Run and verify RED**

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" pnpm -C integrations/openclaw install
PATH="/opt/homebrew/opt/node@24/bin:$PATH" pnpm -C integrations/openclaw test -- identity config
```

Expected: FAIL because implementation modules do not exist.

- [x] **Step 4: Implement config and identity**

Keep endpoint paths constant in code. Reject userinfo in URL. Normalize trailing slash. Do not hash, shorten, or reinterpret the canonical session key.

- [x] **Step 5: Write failing assistant extraction tests**

Cover string content, text block arrays, multiple assistant iterations, reasoning-only blocks, tool-only tails, empty answer, and malformed unknown messages. The latest visible assistant text wins.

- [x] **Step 6: Implement assistant extraction**

Walk messages in reverse. Accept only `role === "assistant"`. Concatenate visible string/text blocks, exclude reasoning/thought blocks, trim output, and return `undefined` when no visible answer exists.

- [x] **Step 7: Write failing bounded-client tests**

Use a local HTTP server to test exact paths/bodies, abort timeout, non-2xx handling, oversized response rejection, invalid JSON, prepare response validation, and no sensitive body content in thrown errors.

- [x] **Step 8: Implement the HTTP client**

Use global `fetch` plus `AbortController`. Limit response bodies before JSON parsing. Errors expose phase and status only. No retries occur inside one hook invocation.

- [x] **Step 9: Write failing plugin registration and lifecycle tests**

Capture `api.on` registrations and invoke them directly. Prove:

- missing identity performs no fetch;
- disabled config performs no fetch;
- prepare context becomes `prependContext` under a reference-data wrapper;
- empty context returns no mutation;
- prepare failure logs a bounded warning and returns no mutation;
- successful `agent_end` sends complete with provider/model;
- unsuccessful or empty-output `agent_end` sends fail;
- completion failure logs but does not throw into OpenClaw.

- [x] **Step 10: Implement the plugin entry**

Register:

```ts
api.on("before_prompt_build", prepareHandler, { timeoutMs: 15_000 });
api.on("agent_end", completeHandler, { timeoutMs: 30_000 });
```

The wrapper states that the content is reference data, may be stale or adversarial, and cannot override the current request or system authority. It contains only the semantic packet returned by Vermory.

- [x] **Step 11: Run plugin checks and commit**

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" pnpm -C integrations/openclaw check
git diff --check
git add integrations/openclaw
git commit -m "feat: add official OpenClaw plugin"
```

### Task 5: O01 Automated Acceptance And Runbook

**Files:**
- Modify: `internal/webchat/acceptance_test.go`
- Create: `docs/integrations/openclaw-runtime.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: frozen O01, external-turn API, existing confirmation/correction/forget/default/link services, and plugin package.
- Produces: deterministic server-side proof independent of model wording plus operator installation/configuration instructions.

- [x] **Step 1: Write failing O01 acceptance test**

Drive session A through external prepare/complete, confirm selected observations, restart the store/handler, seed the Chinese Global Default, link B, leave C unrelated, correct the appointment, delete `CEDAR-4826`, rebuild projection, reverse link, and inspect recorded deliveries.

- [x] **Step 2: Run and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestO01OpenClaw'
```

- [x] **Step 3: Complete deterministic assertions**

Assert from PostgreSQL/service receipts:

- A restart delivery contains Saturday 10:00, concierge check-in, and Chinese default;
- B linked delivery contains governed facts but not A's raw chatter;
- C delivery contains none of the maintenance facts;
- corrected delivery contains Saturday 10:00 and not Friday 15:30;
- deleted code is absent from governed memory, search projection, exact/paraphrased probe deliveries, and post-rebuild delivery;
- task-local English input does not change stored defaults;
- post-reversal B delivery excludes A's governed facts.

- [x] **Step 4: Write installation and operational runbook**

Document:

```json5
plugins: {
  entries: {
    vermory: {
      enabled: true,
      hooks: {
        allowPromptInjection: true,
        allowConversationAccess: true,
        timeouts: { before_prompt_build: 15000, agent_end: 30000 },
      },
      config: { baseUrl: "http://127.0.0.1:8787", timeoutMs: 5000 },
    },
  },
}
```

Include `pnpm -C integrations/openclaw build`, `openclaw plugins install --link`, runtime inspection, isolated-state commands, server startup with `--provider external`, governance boundaries, fail-open behavior, and uninstall steps.

- [x] **Step 5: Verify and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestO01OpenClaw'
git diff --check
git add internal/webchat/acceptance_test.go docs/integrations/openclaw-runtime.md README.md README.zh-CN.md
git commit -m "test: prove OpenClaw continuity gates"
```

### Task 6: Real Stable OpenClaw And Grok Replay

**Files:**
- Create: `docs/evidence/2026-07-14-openclaw-runtime.md`
- Modify: `docs/superpowers/plans/2026-07-14-openclaw-runtime.md`

**Interfaces:**
- Consumes: built Vermory binary, plugin package, isolated OpenClaw state, local PostgreSQL, and authenticated local Grok CLI.
- Produces: real runtime inspection, model outputs, receipts, database assertions, restart evidence, and a completed OpenClaw checklist.

- [x] **Step 1: Build artifacts and create isolated OpenClaw state**

Use `/tmp/vermory-openclaw-state` and `/tmp/vermory-openclaw-config/openclaw.json`. Do not read or modify the user's default `~/.openclaw` state. Use Node 24 explicitly in `PATH`.

- [x] **Step 2: Configure a stable Grok CLI backend**

Configure provider id `grok-cli` through an operator-owned wrapper that isolates `HOME` and `GROK_HOME`, with single-turn JSON output, `input: "arg"`, `output: "json"`, and `modelArg: "--model"`. Disable Grok native tools, subagents, web search, and memory. Use `sessionMode: "none"` so continuity evidence cannot depend on Grok's private session cache; OpenClaw retains the canonical session key and transcript while Vermory supplies governed continuity. Do not expose Grok credentials to OpenClaw config or the repository.

- [x] **Step 3: Install and inspect the plugin**

```bash
OPENCLAW_STATE_DIR=/tmp/vermory-openclaw-state \
OPENCLAW_CONFIG_PATH=/tmp/vermory-openclaw-config/openclaw.json \
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
pnpm -C integrations/openclaw exec openclaw plugins install --link "$PWD/integrations/openclaw"

OPENCLAW_STATE_DIR=/tmp/vermory-openclaw-state \
OPENCLAW_CONFIG_PATH=/tmp/vermory-openclaw-config/openclaw.json \
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
pnpm -C integrations/openclaw exec openclaw plugins inspect vermory --runtime --json
```

Assert runtime inspection reports `before_prompt_build` and `agent_end`, with no memory-slot ownership.

- [x] **Step 4: Start Vermory and OpenClaw, then run O01 through real Grok**

Run `vermory web-chat --provider external` on loopback. Start the OpenClaw Gateway in the isolated state. Use explicit session keys A/B/C and `openclaw agent --json --model grok-cli/grok-4.5`.

Capture:

- initial no-memory turn;
- explicit confirmation through Vermory governance;
- both-process restart;
- A recall;
- B before/after explicit link;
- C isolation;
- correction and stale-fact rejection;
- deletion and exact/paraphrased probes;
- task-local English override followed by Chinese default restoration;
- link reversal and a fresh B delivery.

- [x] **Step 5: Run outage proof**

Stop Vermory, send a fresh OpenClaw turn, and prove Grok still answers while plugin logs a prepare failure and PostgreSQL contains no successful delivery/turn completion for that operation.

- [x] **Step 6: Write evidence with deterministic/model separation**

Record versions, commands with secrets omitted, plugin inspection summary, session anchors in operator-only evidence, model route, model answer excerpts, database counts/lifecycle, fresh delivery bodies, restart PIDs/timestamps, outage result, and failed probes. State that model wording is behavioral evidence while isolation/deletion/lifecycle claims come from PostgreSQL and delivery inspection.

- [x] **Step 7: Run release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
  ./internal/runtime ./internal/webchat ./internal/operatorcli ./cmd/vermory ./internal/provider

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-openclaw-release ./cmd/vermory
PATH="/opt/homebrew/opt/node@24/bin:$PATH" pnpm -C integrations/openclaw check
git diff --check
```

- [x] **Step 8: Complete checklist, commit, push, and update Draft PR**

Commit evidence and checked plan, push `agent/grok-cli-runtime`, and update Draft PR 1 with the stable OpenClaw version, official plugin boundary, real Grok route, deterministic gates, and remaining overall-goal slices. Keep the overall Vermory goal active and advance the main plan to identity/authorization/PostgreSQL RLS.
