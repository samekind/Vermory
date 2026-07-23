# Durable Bridges Runtime Evidence

Date: 2026-07-14

Runtime: Vermory loopback HTTP, Operator CLI, external MCP stdio client, PostgreSQL

Provider: authenticated Grok CLI `0.2.99`, model `grok-4.5`

Replay tenant: `bridge-real-20260714`

## Automated Gates

- B01 freezes selected conversation-to-workspace promotion, bounded export, and promotion reversal.
- B02 freezes conversation link/reversal and workspace rebind/reversal.
- Six public reality cases now validate with fixture locks; two explicitly cover bridge continuity.
- Promote accepts only named active source memory, creates target observations/memory/projections, and reverses without altering source.
- Link shares active governed memory across a bounded root group while recent raw conversation remains anchor-local.
- Export contains selected semantic content only and revocation redacts the internal body.
- Adopt adds a reversible alias; Rebind moves and restores an exact path without changing continuity identity.
- HTTP and CLI cover all five actions, inspect, reversal, server-owned tenant, conflicting replay, and restart durability.

## Real Promote And MCP Replay

Grok produced the exact synthetic release fact:

```text
Use checkout_eta_v2 for the staged checkout release.
```

The assistant observation was explicitly confirmed as active memory `ae930d2e-732f-4fcc-afb6-d87a87031484`. Promote bridge `37d06761-94d3-4a0b-b706-bb953fa2e435` copied it into confirmed workspace continuity `22239238-1c25-464b-8e07-df435edf6ddc`.

An external MCP client launched the built `vermory mcp-stdio` binary and received:

```json
{
  "context": "Governed memory:\nUse checkout_eta_v2 for the staged checkout release.",
  "delivery_id": "7331226c-da3e-43fb-9ff6-26a7b9735aaf",
  "status": "resolved"
}
```

After bridge reversal, a fresh MCP operation returned:

```json
{
  "context": "",
  "delivery_id": "46ca60c7-1c89-44ba-812d-dcbcfbf6aa89",
  "status": "resolved"
}
```

The source conversation memory remained active. The generated workspace memory became deleted and its projection was removed.

## Real Link And Grok Replay

Link bridge `5a7e4636-4878-4c50-8109-97f589253c01` connected:

- primary: `web_chat/release-planning-real`;
- linked: `openclaw_dm/release-linked-real`.

The linked thread asked which checkout flag to use. Its fresh delivery contained the primary continuity's governed memory, and Grok answered:

```text
checkout_eta_v2
```

After reversal, PostgreSQL evidence was:

```json
{
  "active_links": 0,
  "active_promoted_targets": 0,
  "before_has_governed_memory": true,
  "after_has_governed_memory": false,
  "after_has_local_history": true
}
```

This is the correct reversal boundary. The post-reversal delivery no longer received cross-continuity `Governed memory`, but it retained its own local `Recent conversation`, including the answer generated while the link was valid. Grok therefore repeated `checkout_eta_v2` after reversal. That model output is preserved as evidence that bridge reversal stops future sharing but does not falsify or erase already-created local conversation history.

If previously shared content must be removed for privacy, the relevant governed memory and affected local memory/history require explicit deletion governance. Link reversal alone is not advertised as forgetting.

## Release Verification

Passed:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/operatorcli ./cmd/vermory ./internal/provider
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-bridges-release ./cmd/vermory
git diff --check
```

No credentials, private source content, or sensitive deleted values are retained in this evidence.
