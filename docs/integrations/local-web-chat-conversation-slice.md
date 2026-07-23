# Local Web Chat Conversation Continuity

Vermory's local Web Chat runtime provides persistent conversation continuity to any client that can call a loopback JSON API.

It keeps two layers separate:

- recent conversation observations provide natural short-horizon continuation;
- explicitly confirmed governed memory provides durable reusable state.

Assistant output remains an observation until the user confirms it. Correction and forgetting always target a specific governed memory ID.

## Start The Server

Build:

```bash
go build -o /tmp/vermory ./cmd/vermory
```

Run with the authenticated local Grok CLI:

```bash
/tmp/vermory web-chat \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local-user \
  --provider grok-cli \
  --model grok-4.5 \
  --listen 127.0.0.1:8787
```

The command rejects non-loopback listeners. Tenant, provider, model, continuity ID, authority, and lifecycle kind are server-owned and are not accepted from normal chat requests.

The same server command supports:

- `mock`
- `grok-cli`
- `openai-compatible`
- `siliconflow`
- `duojie`

For OpenAI-compatible providers, supply `--model`, `--base-url` where required, and `--api-key-env`. API keys are read from the named environment variable and are never accepted in request JSON.

## Normal Chat Turn

```bash
curl -sS -X POST http://127.0.0.1:8787/v1/chat/turn \
  -H 'Content-Type: application/json' \
  --data '{
    "operation_id": "turn-001",
    "channel": "web_chat",
    "thread_id": "device-maintenance",
    "message": "Continue from the latest verified state."
  }'
```

An unseen exact `(channel, thread_id)` creates an isolated conversation continuity. Repeating the same operation with identical input returns the persisted receipt and does not call the provider again. Reusing an operation ID with changed content or another continuity is rejected.

## Inspect And Confirm

Inspect the exact continuity:

```bash
curl -sS 'http://127.0.0.1:8787/v1/conversations/inspect?channel=web_chat&thread_id=device-maintenance'
```

Confirm a specific user or assistant observation:

```bash
curl -sS -X POST http://127.0.0.1:8787/v1/memories/confirm \
  -H 'Content-Type: application/json' \
  --data '{
    "operation_id": "confirm-001",
    "channel": "web_chat",
    "thread_id": "device-maintenance",
    "observation_id": "<observation-uuid>"
  }'
```

Confirmation writes a content-free user-confirmation event and creates active governed memory whose content-bearing origin remains the targeted observation.

## Correct A Memory

```bash
curl -sS -X POST http://127.0.0.1:8787/v1/memories/correct \
  -H 'Content-Type: application/json' \
  --data '{
    "operation_id": "correct-001",
    "channel": "web_chat",
    "thread_id": "device-maintenance",
    "memory_id": "<active-memory-uuid>",
    "content": "The verified current state is 82 percent used with 87 GB free."
  }'
```

The old active memory becomes superseded and is removed from the current search projection in the same PostgreSQL transaction that creates the replacement.

## Forget A Memory

```bash
curl -sS -X POST http://127.0.0.1:8787/v1/memories/forget \
  -H 'Content-Type: application/json' \
  --data '{
    "operation_id": "forget-001",
    "channel": "web_chat",
    "thread_id": "device-maintenance",
    "memory_id": "<memory-uuid>"
  }'
```

The request has no free-text reason or old-content field. The runtime redacts the governed memory, its content-bearing origin observation, matching delivery content, and any originating assistant turn receipt, then removes the search projection.

## Failure Behavior

Provider failure leaves the user observation and a durable failed turn receipt. No assistant observation is fabricated and no memory is promoted. Repeating the same failed operation returns the same failed receipt; intentional retry uses a new operation ID.

The HTTP response exposes a stable `failure_code` but not raw CLI output, database URLs, SQL, credentials, or internal provider diagnostics.

## Cross-client Conversation Bridges

Web Chat, Hermes, and OpenClaw anchors remain isolated even when their visible
thread names match. Create each continuity through its normal turn route, then
use `POST /v1/bridges/link` to connect only the named pair. Linked clients
receive governed active memory, not another client's raw conversation history.

Each link has its own durable bridge ID. Replaying the same link operation is
idempotent. `POST /v1/bridges/reverse` stops future sharing through that bridge
without deleting either client's own continuity or transcript.

The frozen `B03-three-client-conversation-bridge` acceptance case exercises
the Web Chat, Hermes, OpenClaw, link, replay, completion, and reversal HTTP
contracts together. This deterministic contract test is separate from the
real-client Hermes and OpenClaw qualifications recorded in their own evidence.

## Verification Evidence

Automated acceptance:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 ./internal/runtime ./internal/webchat ./cmd/vermory -count=1
```

Frozen cases:

- C01 resumes a device-maintenance matter from the verified final state after reopening PostgreSQL.
- S01 removes a synthetic target from observations, governed memory, search projection, delivery history, turn receipts, inspection, and post-rebuild recall while retaining independent rotation guidance.
- B03 keeps same-named Web Chat, Hermes, and OpenClaw matters isolated until explicit links are created, then removes future sharing after reversal.

Real Grok request/response evidence is retained under `artifacts/runtime/C04-grok-webchat-runtime/`.

## Security Boundary

This command is a loopback local runtime. Before any non-loopback or multi-user deployment, the platform still requires authenticated principals, PostgreSQL RLS, transport authentication, origin policy, rate limits, and production secret management.
