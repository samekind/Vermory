# Three-client Conversation Bridge Contract

Date: 2026-07-21

Case: `B03-three-client-conversation-bridge`

Status: deterministic HTTP and PostgreSQL contract passed

## Boundary Exercised

The accepted run used a fresh dedicated PostgreSQL database migrated from
schema 1 through schema 23. One Web Chat continuity contained a confirmed
synthetic current fact. Same-named Hermes and OpenClaw continuities were then
created through their normal prepare/complete HTTP routes.

Before governance, neither external continuity received the Web Chat fact.
The test then created two explicit bridges through `POST /v1/bridges/link`:

```text
web_chat/release-matter -> hermes/release-matter
web_chat/release-matter -> openclaw/release-matter
```

Both integration prepare routes received the confirmed governed memory:

```text
The current release bundle is vermory-v8.tgz.
```

Neither delivery contained the Web Chat-only raw marker or the unrelated
matter marker. Replaying the Hermes link returned the original bridge ID with
`replayed=true` rather than creating a second bridge.

Both links were then reversed through the served bridge endpoint. Fresh Hermes
and OpenClaw prepare operations no longer received the Web Chat fact. Reversal
did not delete any source continuity or external-client history.

## Verification

```bash
VERMORY_TEST_DATABASE_URL='postgresql://127.0.0.1:5432/<fresh-db>?sslmode=disable' \
  go test -count=1 ./internal/webchat \
  -run TestB03ThreeClientConversationBridgeAcceptance -v

go test -count=1 ./internal/reality -v
```

Results:

```text
TestB03ThreeClientConversationBridgeAcceptance PASS
internal/reality                           PASS
frozen public cases                        20
conversation coverage                      12
bridge coverage                             7
```

## Claim Boundary

This run proves the shared Vermory HTTP, authority, bridge, replay, delivery,
and reversal contract. The provider used by the Web Chat test is deterministic,
and the external completion routes use explicit test-only model labels. This
document does not claim a new live Hermes, OpenClaw, Grok, or DeepSeek model
execution. Those real-client qualifications remain separate evidence and are
not replaced by this contract run.
