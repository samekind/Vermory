# Identity, Authorization, And RLS Evidence

Execution date: 2026-07-14 Asia/Shanghai

Scope: authenticated multi-tenant HTTP profile, PostgreSQL runtime identity, tenant RLS, token lifecycle, and one real authenticated OpenClaw/Grok replay

Repository commit used for the process replay: `b4b521537ea35d6e99bfa984295d5227d33b42c6`

Replay binary SHA-256:

```text
0c1d87655d218547524eab58dc736caa88a86d8346606215a762dd12dfb9aadc
```

## Claim Boundary

Deterministic authority comes from PostgreSQL catalog state, HTTP status/receipt assertions, token lifecycle rows, RLS queries, foreign-key rejection, and persisted conversation rows. Grok output proves that the real OpenClaw/model path consumed the governed packet and remained available after token revocation; model wording is not used as database authority.

No Gemini CLI or Mac mini NewAPI route was used.

## Runtime Inventory

| Component | Version or value |
|---|---|
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL | `18.4 (Homebrew)` |
| Schema | `9` |
| Node.js | `v24.18.0` |
| pnpm | `11.12.0` |
| OpenClaw | `2026.6.11 (e085fa1)` |
| Grok CLI | `0.2.99 (b1b49ccb71a7)` |
| Model | `grok-cli/grok-4.5` |
| Database | `vermory_identity_i06_20260714` |
| Runtime role | `vermory_i06_runtime` |
| Vermory listener | `127.0.0.1:8788` |
| OpenClaw Gateway | `127.0.0.1:18790` |
| OpenClaw state | `/tmp/vermory-openclaw-auth-state` |
| OpenClaw config | `/tmp/vermory-openclaw-auth-config/openclaw.json` |

The dedicated database, role, ports, state, and config were separate from the earlier unauthenticated OpenClaw replay.

## Deterministic I01-I05 Acceptance

Command:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/webchat -run 'TestI01Authenticated' -v
```

Result:

```text
--- PASS: TestI01AuthenticatedTenantIsolationAndLifecycle
PASS
```

The acceptance test used actual random API tokens, an actual non-owner PostgreSQL login, the RLS-aware pool, and the authenticated HTTP handler. It proved:

- client/operator role separation;
- tenant A and tenant B resolve the same OpenClaw `session_key` to different continuity IDs;
- A confirms `TENANT-A-ONLY`, A receives it in a fresh governed-memory delivery, and B does not;
- B cannot correct A's memory even when given its UUID;
- request JSON cannot select `tenant_id`;
- expired and revoked tokens receive `401`;
- direct queries that omit tenant predicates see only the current tenant;
- missing tenant context sees zero rows;
- cross-tenant continuity, memory, delivery, and bridge foreign-key attacks fail;
- concurrent A/B requests through a two-connection pool do not carry stale tenant settings.

The first acceptance attempt intentionally used a query with no lexical overlap and received no governed memory. The case was corrected to use a real retrievable query and to assert only the `Governed memory` section. This retained the current retrieval boundary rather than converting RLS acceptance into a false unconditional-recall claim.

## Database Boundary

The process replay produced this safe catalog and lifecycle summary:

```json
{
  "postgresql_version": "18.4 (Homebrew)",
  "schema_version": 9,
  "role": {
    "name": "vermory_i06_runtime",
    "can_login": true,
    "superuser": false,
    "bypass_rls": false,
    "owned_served_tables": 0,
    "can_execute_auth": true,
    "can_read_auth_table": false,
    "can_read_legacy_table": false
  },
  "rls_table_count": 12,
  "rls_policy_count": 12,
  "token_status_counts": {
    "active": 1,
    "revoked": 1
  },
  "recall_run": {
    "rows": 1,
    "status": "completed",
    "provider_model": "grok-cli/grok-4.5",
    "answer": "ORBIT-7319",
    "context_match": 1
  },
  "revoked_run_rows": 0,
  "no_tenant_visible_rows": 0,
  "authenticated_visible_rows": 2,
  "active_governed_memory_count": 1
}
```

The active token was the operator token. The OpenClaw client token was revoked. No raw token, digest, public token ID, or runtime password is included in this document.

RLS policy inventory:

```text
bridge_events:bridge_events_tenant_isolation
bridge_memory_effects:bridge_memory_effects_tenant_isolation
bridge_operations:bridge_operations_tenant_isolation
continuity_bindings:continuity_bindings_tenant_isolation
continuity_spaces:continuity_spaces_tenant_isolation
conversation_bindings:conversation_bindings_tenant_isolation
conversation_links:conversation_links_tenant_isolation
conversation_turns:conversation_turns_tenant_isolation
governed_memories:governed_memories_tenant_isolation
memory_deliveries:memory_deliveries_tenant_isolation
memory_search_documents:memory_search_documents_tenant_isolation
observations:observations_tenant_isolation
```

## Real Authenticated OpenClaw/Grok Replay

The plugin ran inside a new OpenClaw Gateway process with `VERMORY_API_TOKEN` in the process environment and no token field in plugin config. Grok used the existing isolated wrapper:

```text
HOME=/tmp/vermory-home
GROK_HOME=/tmp/vermory-grok-home
sessionMode=none
--no-subagents
--disable-web-search
--no-memory
--tools ""
```

The synthetic governed fact was seeded and explicitly confirmed through the authenticated API:

```text
The current deployment code is ORBIT-7319.
```

Recall run:

```text
runId: e1c30145-cd99-403d-a244-3f5dd91657a5
sessionKey: agent:main:i06-authenticated
provider/model: grok-cli/grok-4.5
visible answer: ORBIT-7319
```

The final prompt contained:

```text
Vermory reference data follows.
...
Governed memory:
The current deployment code is ORBIT-7319.
```

PostgreSQL contained exactly one completed row for `openclaw:e1c30145-cd99-403d-a244-3f5dd91657a5`, model `grok-cli/grok-4.5`, answer `ORBIT-7319`, and one delivery whose context contained the governed code.

## Revocation And Fail-Open Chat

The client token was revoked while the Gateway remained online. A new session then ran:

```text
runId: 7f303199-964f-41f3-8f59-b9ae515b2b98
sessionKey: agent:main:i06-revoked
visible answer: AUTH_REVOKED_CHAT_AVAILABLE
```

The final prompt contained only the current user request and no Vermory reference-data wrapper. PostgreSQL contained zero conversation-turn rows for `openclaw:7f303199-964f-41f3-8f59-b9ae515b2b98`.

Gateway logs recorded both bounded messages:

```text
Vermory prepare failed; continuing without external continuity context.
Vermory completion persistence failed; OpenClaw result remains available but was not confirmed as persisted.
```

This proves chat availability failed open while persistence claims failed closed after revocation.

## Known Runtime Warnings

OpenClaw emitted missing generated-module warnings for bundled `imessage` and `telegram` channel setup. Neither channel was configured or used. OpenClaw also reported `2026.7.1` as available; the replay stayed on the package-locked `2026.6.11` version.

All temporary Vermory and OpenClaw listeners were stopped after evidence collection. Ports `8788` and `18790` had no remaining listeners.

## Release Verification

Fresh verification after the authenticated replay passed:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
  ./internal/authn ./internal/runtime ./internal/webchat \
  ./internal/identitycli ./internal/operatorcli ./cmd/vermory ./internal/provider

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-identity-release ./cmd/vermory

PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw check

PATH=/opt/homebrew/opt/node@24/bin:/opt/homebrew/bin:/usr/bin:/bin \
  pnpm -C integrations/openclaw pack --dry-run

git diff --check
```

Results:

- every Go package passed;
- selected identity/runtime/client packages passed with the race detector;
- `go vet` passed;
- `go mod tidy` changed neither `go.mod` nor `go.sum`;
- the release binary built with SHA-256 `0c1d87655d218547524eab58dc736caa88a86d8346606215a762dd12dfb9aadc`;
- OpenClaw passed `43/43` tests, strict TypeScript checking, and build;
- package dry-run contained only `dist`, `openclaw.plugin.json`, and `package.json`;
- Git whitespace checks passed.
