# Conversation Web Chat Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a persistent, loopback Web Chat/API vertical slice that resumes an exact conversation thread, separates recent working context from governed memory, supports explicit confirm/correct/forget, and passes C01/S01 plus a real Grok CLI replay.

**Architecture:** Extend the existing PostgreSQL authority model with conversation bindings and durable chat-turn receipts. A `runtime.ConversationService` owns continuity resolution, bounded context assembly, provider invocation, idempotent write-back, and governance; a focused `internal/webchat` package exposes strict JSON HTTP handlers. The existing `provider.Provider`, governed-memory lifecycle, delivery ledger, and rebuildable search projection remain shared with workspace continuity.

**Tech Stack:** Go, PostgreSQL, pgx, goose migrations, Go `net/http`, Cobra, existing provider interface, authenticated local Grok CLI.

## Global Constraints

- PostgreSQL is the only authority; providers and search documents are not authority.
- Exact `(channel, thread_id)` creates or resolves one isolated conversation continuity.
- Requests cannot select tenant, continuity ID, authority, provider, or model.
- Assistant output never becomes active memory without explicit user confirmation.
- Deleted content must be absent from governed memory, origin observations, recent context, projections, inspection content, and rebuilt projections.
- The server defaults to `127.0.0.1:8787`; non-loopback hosted security is out of scope.
- Use TDD for every production behavior and commit each independently reviewable task.
- Do not add Redis, embeddings, automatic memory extraction, bridge operations, Global Defaults, OpenClaw, Streamable HTTP, or browser UI.

---

## File Structure

- `internal/store/postgres/migrations/00004_conversation_continuity.sql`: conversation constraints, bindings, and observation kinds.
- `internal/store/postgres/migrations/00005_conversation_turns.sql`: durable chat-turn receipts.
- `internal/runtime/conversation_types.go`: validated conversation anchors, chat requests/receipts, inspection and governance request types.
- `internal/runtime/conversation_store.go`: conversation binding, recent observation, turn idempotency, completion/failure, and confirmation persistence.
- `internal/runtime/conversation_service.go`: provider-independent chat orchestration, context assembly, inspection, confirm, correct, and forget.
- `internal/runtime/conversation_store_test.go`: PostgreSQL authority and lifecycle tests.
- `internal/runtime/conversation_service_test.go`: deterministic orchestration, isolation, context, provider failure, and idempotency tests.
- `internal/webchat/handler.go`: strict loopback JSON API handler.
- `internal/webchat/handler_test.go`: HTTP contract and security-boundary tests.
- `internal/webchat/acceptance_test.go`: persistent C01/S01 runtime acceptance using frozen reality fixtures.
- `cmd/vermory/web_chat.go`: Cobra command, provider construction, loopback server startup.
- `cmd/vermory/web_chat_test.go`: command validation and default-listener tests.
- `docs/integrations/local-web-chat-conversation-slice.md`: operator runbook and real replay instructions/results.

---

### Task 1: Conversation Authority Schema And Exact Binding

**Files:**
- Create: `internal/store/postgres/migrations/00004_conversation_continuity.sql`
- Create: `internal/runtime/conversation_types.go`
- Create: `internal/runtime/conversation_store.go`
- Create: `internal/runtime/conversation_store_test.go`
- Modify: `internal/runtime/postgres_store.go`

**Interfaces:**
- Produces: `ConversationAnchor.Normalized()`, `Store.ResolveOrCreateConversation(ctx, tenantID, anchor)`, `Store.ListRecentConversationObservations(ctx, tenantID, continuityID, beforeObservationID, limit)`.
- Produces: `ConversationResolution`, `ConversationObservation`, and conversation-aware continuity validation shared by later tasks.

- [x] **Step 1: Write failing exact-binding and history tests**

Add tests that require persistence and isolation:

```go
func TestResolveOrCreateConversationUsesExactChannelAndThread(t *testing.T) {
    store := openTestStore(t)
    ctx := context.Background()
    first, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{Channel: "web_chat", ThreadID: "matter-1"})
    requireNoError(t, err)
    again, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{Channel: "web_chat", ThreadID: "matter-1"})
    requireNoError(t, err)
    other, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{Channel: "other", ThreadID: "matter-1"})
    requireNoError(t, err)
    if first.ContinuityID != again.ContinuityID || first.ContinuityID == other.ContinuityID {
        t.Fatalf("exact anchor isolation failed: first=%#v again=%#v other=%#v", first, again, other)
    }
}

func TestConversationAnchorRejectsMissingThread(t *testing.T) {
    _, err := (ConversationAnchor{Channel: "web_chat"}).Normalized()
    if err == nil {
        t.Fatal("missing thread_id must be rejected")
    }
}
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -run 'TestResolveOrCreateConversation|TestConversationAnchor' -count=1
```

Expected: compile failure because the conversation types and store methods do not exist.

- [x] **Step 3: Add the migration and minimal store implementation**

The migration must:

```sql
ALTER TABLE continuity_spaces DROP CONSTRAINT continuity_spaces_continuity_line_check;
ALTER TABLE continuity_spaces ADD CONSTRAINT continuity_spaces_continuity_line_check
  CHECK (continuity_line IN ('workspace', 'conversation'));

CREATE TABLE conversation_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  channel TEXT NOT NULL,
  thread_id TEXT NOT NULL,
  binding_state TEXT NOT NULL CHECK (binding_state IN ('confirmed', 'retired')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX conversation_bindings_confirmed_anchor_idx
  ON conversation_bindings (tenant_id, channel, thread_id)
  WHERE binding_state = 'confirmed';
```

Extend the observation-kind check with `user_message`, `assistant_message`, and `user_confirmation`. Update `ResetForTest` so the new relations are truncated. Add a shared store check that accepts either an active workspace or active conversation continuity instead of the current workspace-only SQL.

- [x] **Step 4: Add operation replay fingerprint validation**

When an observation operation already exists, compare continuity, kind, content, and source reference. Return `Replayed: true` only for the identical logical request; reject changed content, kind, source, or continuity.

- [x] **Step 5: Run runtime tests and verify GREEN**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -count=1
```

Expected: PASS, including all existing workspace tests.

- [x] **Step 6: Commit**

```bash
git add internal/store/postgres/migrations/00004_conversation_continuity.sql internal/runtime/conversation_types.go internal/runtime/conversation_store.go internal/runtime/conversation_store_test.go internal/runtime/postgres_store.go
git commit -m "feat: add conversation authority storage"
```

---

### Task 2: Durable Chat Turns And Two-Layer Context

**Files:**
- Create: `internal/store/postgres/migrations/00005_conversation_turns.sql`
- Modify: `internal/runtime/conversation_types.go`
- Modify: `internal/runtime/conversation_store.go`
- Create: `internal/runtime/conversation_service.go`
- Create: `internal/runtime/conversation_service_test.go`

**Interfaces:**
- Consumes: exact conversation binding and recent observations from Task 1.
- Produces: `NewConversationService(store, tenantID, provider, model, config)`, `ConversationService.Chat(ctx, ChatTurnRequest)`, and `ChatTurnReceipt`.

- [x] **Step 1: Write failing persistence, context, and provider-idempotency tests**

Use a counting provider that records requests:

```go
type recordingProvider struct {
    calls []provider.GenerateRequest
    output string
    err error
}

func (p *recordingProvider) Generate(_ context.Context, req provider.GenerateRequest) (provider.GenerateResponse, error) {
    p.calls = append(p.calls, req)
    if p.err != nil {
        return provider.GenerateResponse{}, p.err
    }
    return provider.GenerateResponse{Output: p.output, Model: "test-model"}, nil
}
```

Tests must prove:

- a second turn receives the prior user and assistant observations;
- a different thread does not receive them;
- repeating the same operation returns the same receipt with one provider call;
- a provider failure persists the user observation and no assistant observation;
- redacted observations are absent from context.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -run 'TestConversationService' -count=1
```

Expected: compile failure because `ConversationService` and chat-turn persistence do not exist.

- [x] **Step 3: Add durable `conversation_turns` receipts**

Create `00005_conversation_turns.sql` with:

```sql
CREATE TABLE conversation_turns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  continuity_id UUID NOT NULL REFERENCES continuity_spaces(id) ON DELETE CASCADE,
  operation_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('in_progress', 'completed', 'failed')),
  user_observation_id UUID NOT NULL REFERENCES observations(id),
  delivery_id UUID REFERENCES memory_deliveries(id),
  assistant_observation_id UUID REFERENCES observations(id),
  answer TEXT NOT NULL DEFAULT '',
  provider_model TEXT NOT NULL DEFAULT '',
  failure_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation_id)
);
```

Implement begin, complete, fail, and lookup methods. `BeginConversationTurn` inserts the user observation and `in_progress` receipt transactionally. `CompleteConversationTurn` inserts the assistant observation and updates the receipt in one transaction.

- [x] **Step 4: Implement minimal orchestration and semantic context formatting**

`Chat` must:

```go
resolution := store.ResolveOrCreateConversation(...)
turn := store.BeginConversationTurn(...)
if turn.Replayed || turn.Status != ChatTurnInProgress { return turn, nil }
memories := store.SearchActiveMemory(...)
recent := store.ListRecentConversationObservations(...)
delivery := store.RecordDelivery(...)
generated, err := llm.Generate(ctx, provider.GenerateRequest{
    Model: model,
    System: conversationSystemPrompt,
    Prompt: request.Message,
    ContextPacket: BuildConversationContext(memories, recent),
})
```

On provider error, persist `failed`; on success, persist the assistant observation and completed response. The model-facing packet must contain only `Governed memory:` and `Recent conversation:` semantic sections.

- [x] **Step 5: Run focused and package tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -count=1
```

Expected: PASS with exactly one provider call for an idempotent replay.

- [x] **Step 6: Commit**

```bash
git add internal/store/postgres/migrations/00005_conversation_turns.sql internal/runtime/conversation_types.go internal/runtime/conversation_store.go internal/runtime/conversation_service.go internal/runtime/conversation_service_test.go
git commit -m "feat: add persistent conversation turns"
```

---

### Task 3: Explicit Conversation Memory Governance

**Files:**
- Modify: `internal/runtime/conversation_types.go`
- Modify: `internal/runtime/conversation_store.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/conversation_store_test.go`
- Modify: `internal/runtime/conversation_service_test.go`

**Interfaces:**
- Produces: `ConversationService.Confirm`, `ConversationService.Correct`, `ConversationService.Forget`, and `ConversationService.Inspect`.

- [x] **Step 1: Write failing confirmation, correction, deletion, and rebuild tests**

Tests must assert:

```go
confirmed, err := service.Confirm(ctx, ConfirmConversationMemoryRequest{
    OperationID: "confirm-1",
    Anchor: ConversationAnchor{Channel: "web_chat", ThreadID: "matter-1"},
    ObservationID: first.AssistantObservationID,
})
```

- the assistant observation remains non-authoritative before confirmation;
- confirmation creates an active memory from the exact target observation;
- confirmation rejects an observation from another continuity;
- correction targets one active memory and supersedes it;
- forget redacts the memory and original content-bearing observation;
- the forgotten content is absent from recent history and search after projection rebuild;
- replayed governance operations do not duplicate effects.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -run 'TestConversationGovernance|TestConversationForget' -count=1
```

Expected: compile failure because the governance methods do not exist.

- [x] **Step 3: Implement content-free confirmation with targeted origin**

In one transaction:

- resolve the exact conversation continuity;
- verify the target observation is a non-redacted `user_message` or `assistant_message` in that continuity;
- write `user_confirmation` with fixed content `User confirmed an observation.` and source reference `observation:<uuid>`;
- create active governed memory using the target observation as `origin_observation_id` and its exact content;
- create the search projection.

- [x] **Step 4: Reuse lifecycle operations for correction and forget**

Correction uses `user_correction` and the exact target memory ID. Forget uses the fixed `Operator requested deletion.` observation and encodes only the target memory ID in the operation source reference. All ownership and lifecycle checks remain in PostgreSQL transactions.

- [x] **Step 5: Implement inspection without deleted-content disclosure**

Return recent observations and governed memory receipts for the exact continuity. Deleted content may appear only as `[redacted]`; raw provider artifacts and delivery audit metadata are excluded.

- [x] **Step 6: Run tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/runtime -count=1
```

Expected: PASS, including workspace governance regressions.

- [x] **Step 7: Commit**

```bash
git add internal/runtime/conversation_types.go internal/runtime/conversation_store.go internal/runtime/conversation_service.go internal/runtime/conversation_store_test.go internal/runtime/conversation_service_test.go
git commit -m "feat: govern conversation memory"
```

---

### Task 4: Strict Loopback Web Chat API

**Files:**
- Create: `internal/webchat/handler.go`
- Create: `internal/webchat/handler_test.go`

**Interfaces:**
- Consumes: `runtime.ConversationService` public methods.
- Produces: `webchat.NewHandler(service)` implementing `http.Handler`.

- [x] **Step 1: Write failing HTTP contract tests**

Use `httptest.NewServer` and verify:

- valid `POST /v1/chat/turn` returns the specified receipt fields;
- missing channel/thread/message/operation returns `400`;
- unknown JSON fields and trailing JSON are rejected;
- request bodies over the configured limit return `413`;
- confirm/correct/forget require exact target IDs;
- inspect returns only the requested continuity;
- errors never include database URL or provider raw output.

- [x] **Step 2: Run tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/webchat -count=1
```

Expected: package or symbol missing.

- [x] **Step 3: Implement the minimal handler**

Register only:

```go
mux.HandleFunc("POST /v1/chat/turn", h.chatTurn)
mux.HandleFunc("POST /v1/memories/confirm", h.confirmMemory)
mux.HandleFunc("POST /v1/memories/correct", h.correctMemory)
mux.HandleFunc("POST /v1/memories/forget", h.forgetMemory)
mux.HandleFunc("GET /v1/conversations/inspect", h.inspectConversation)
```

Use `http.MaxBytesReader`, `json.Decoder.DisallowUnknownFields`, one JSON value per body, `Content-Type: application/json`, and stable error codes. Do not expose internal error strings for `500` responses.

- [x] **Step 4: Run tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/webchat -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/webchat/handler.go internal/webchat/handler_test.go
git commit -m "feat: expose local web chat api"
```

---

### Task 5: CLI Server Assembly And Provider Configuration

**Files:**
- Create: `cmd/vermory/web_chat.go`
- Create: `cmd/vermory/web_chat_test.go`
- Modify: `cmd/vermory/main.go`

**Interfaces:**
- Consumes: `runtime.NewConversationService`, `webchat.NewHandler`, existing `provider.Provider` implementations.
- Produces: `vermory web-chat`.

- [x] **Step 1: Write failing command tests**

Tests must prove:

- default listener is `127.0.0.1:8787`;
- empty database URL or tenant is rejected;
- non-loopback listener is rejected in this slice;
- `mock`, `grok-cli`, `openai-compatible`, `siliconflow`, and `duojie` provider names build through the existing provider interface;
- request clients cannot override the configured model.

- [x] **Step 2: Run tests and verify RED**

```bash
go test ./cmd/vermory -run 'TestWebChat' -count=1
```

Expected: command constructor missing.

- [x] **Step 3: Implement `newWebChatCommand`**

Required flags:

```text
--database-url
--tenant-id
--listen (default 127.0.0.1:8787)
--provider (default mock)
--model
--base-url
--api-key-env
```

Construct the provider at server startup, migrate PostgreSQL, build `ConversationService`, create an `http.Server` with bounded header/read/write/idle timeouts, and shut it down when the command context is cancelled.

- [x] **Step 4: Run command and full unit tests**

```bash
go test ./cmd/vermory ./internal/webchat ./internal/runtime -count=1
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add cmd/vermory/web_chat.go cmd/vermory/web_chat_test.go cmd/vermory/main.go
git commit -m "feat: run conversation web chat server"
```

---

### Task 6: C01 And S01 Persistent Acceptance

**Files:**
- Create: `internal/webchat/acceptance_test.go`
- Modify: `internal/runtime/conversation_service_test.go`

**Interfaces:**
- Consumes: the public HTTP contract only for end-to-end assertions.
- Produces: reproducible C01 and S01 hard-gate evidence in automated tests.

- [x] **Step 1: Write C01 acceptance through HTTP**

Load `reality/cases/C01-device-maintenance-continuity/events.jsonl`, submit chronological turns, explicitly confirm the current facts that should survive as governed memory, close and reopen the store/service, then submit the frozen final prompt. Assert the deterministic required and forbidden strings from the manifest.

- [x] **Step 2: Run C01 and verify RED where behavior is incomplete**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/webchat -run TestC01 -count=1
```

Expected: initial failure until restart-safe context and governance are correctly wired.

- [x] **Step 3: Make the minimal runtime corrections and verify C01 GREEN**

Do not weaken fixture assertions. Fix context ordering, persistence, or API behavior only.

- [x] **Step 4: Write S01 acceptance through HTTP**

Create a conversation observation containing the synthetic target, confirm it, add independent rotation guidance, forget the target, rebuild projection, restart the service, and run exact/paraphrased/related probes. Assert the target is absent from answers, inspection, recent context, search, and rebuilt projection while `rotated after use` remains available.

- [x] **Step 5: Run S01 and verify RED then GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test ./internal/webchat -run TestS01 -count=1
```

Expected final result: PASS without removing or loosening forbidden-content checks.

- [x] **Step 6: Run the combined deterministic suite**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
```

Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/webchat/acceptance_test.go internal/runtime/conversation_service_test.go
git commit -m "test: prove persistent conversation hard gates"
```

---

### Task 7: Real Grok Replay, Verification, Documentation, And Push

**Files:**
- Create: `docs/integrations/local-web-chat-conversation-slice.md`
- Create: `artifacts/runtime/C04-grok-webchat-runtime/` retained redacted replay artifacts where repository policy permits.

**Interfaces:**
- Consumes: built `vermory web-chat`, authenticated local `grok` CLI, local PostgreSQL.
- Produces: reproducible real-provider evidence and an updated Draft PR.

- [x] **Step 1: Build the binary and start the local server**

```bash
go build -o /tmp/vermory-webchat ./cmd/vermory
/tmp/vermory-webchat web-chat \
  --database-url 'postgresql:///vermory_grok_webchat?host=/tmp' \
  --tenant-id local-grok \
  --provider grok-cli \
  --model grok-4.5 \
  --listen 127.0.0.1:8787
```

- [x] **Step 2: Replay multi-turn, restart, confirmation, deletion, and idempotency**

Use `curl` with stable operation IDs. Preserve redacted request/response JSON and PostgreSQL assertions. Repeat one completed operation and verify the response receipt is replayed without a second provider invocation.

- [x] **Step 3: Document exact user flow and evidence**

The runbook must explain normal chat, inspection, confirm, correction, forget, restart, provider failure, and the local-only security boundary. It must not claim Global Defaults, bridges, OpenClaw, remote multi-tenancy, or original benchmark completion.

- [x] **Step 4: Run final verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 ./internal/runtime ./internal/webchat ./cmd/vermory -count=1
go vet ./...
go mod tidy -diff
go build -o /tmp/vermory-final-check ./cmd/vermory
git diff --check
```

Expected: every command exits zero and the worktree contains only intended evidence/doc changes before commit.

- [x] **Step 5: Commit and push**

```bash
git add docs/integrations/local-web-chat-conversation-slice.md docs/evidence/2026-07-13-grok-webchat-runtime.md docs/superpowers/plans/2026-07-13-conversation-webchat-runtime.md
git commit -m "docs: add real conversation web chat replay"
git push origin agent/grok-cli-runtime
```

- [x] **Step 6: Update Draft PR evidence**

Keep PR 1 in Draft state. Add the new conversation runtime scope, C01/S01 results, real Grok replay, exact verification commands, and remaining platform boundaries to the PR description.

---

## Plan Self-Review

- Every design requirement maps to Tasks 1-7.
- The plan adds no automatic promotion, bridge, Global Defaults, OpenClaw, vector, Redis, or UI work.
- All new runtime behavior begins with a failing test.
- Exact identifiers are carried consistently: `ConversationAnchor`, `ConversationService`, `ChatTurnRequest`, `ChatTurnReceipt`, `ConfirmConversationMemoryRequest`.
- C01 and S01 remain frozen external assertions rather than implementation-authored success criteria.
- Real Grok evidence is separate from deterministic hard-gate authority.
