# OpenClaw Runtime Integration

Vermory integrates with OpenClaw as a normal lifecycle plugin. OpenClaw keeps ownership of channels, canonical session keys, transcripts, model routing, and model execution. Vermory resolves the conversation continuity, injects active governed context, records turn observations, and applies explicit confirmation, correction, deletion, Global Default, and bridge operations.

The plugin does not declare `kind: "memory"`, does not occupy `plugins.slots.memory`, and does not replace OpenClaw's local transcript or provider configuration.

## Supported Boundary

This runtime currently uses a loopback HTTP connection:

```text
OpenClaw before_prompt_build
-> POST /v1/integrations/openclaw/turns/prepare
-> governed semantic context
-> OpenClaw model turn
-> OpenClaw agent_end
-> POST /complete or /fail
```

Identity is fail-closed:

```text
OpenClaw ctx.sessionKey -> Vermory conversation thread
OpenClaw ctx.runId      -> Vermory operation ID openclaw:<runId>
```

Both values are required. The plugin never falls back to `sessionId`, sender name, prompt text, channel label, model, cwd, or a previous session. Tenant and Vermory channel are server-owned and cannot be selected by plugin input.

## Requirements

- PostgreSQL 16 or newer;
- Go toolchain compatible with the repository `go.mod`;
- Node.js `>=22.19.0`;
- pnpm 11;
- OpenClaw `2026.6.11` or newer within the declared peer range.

The repository lock file verifies development against OpenClaw `2026.6.11`.

## Build And Verify

From the repository root:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw install --frozen-lockfile

PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw check

go build -o ./bin/vermory ./cmd/vermory
```

`pnpm check` runs 37 plugin tests, strict TypeScript checking, and the ESM build. `pnpm pack --dry-run` can be used to inspect the publishable package; only `dist`, `openclaw.plugin.json`, and `package.json` are included.

## Start Vermory

The OpenClaw integration does not ask Vermory to invoke a model. Start the conversation/governance API with the external provider mode:

```bash
./bin/vermory web-chat \
  --database-url 'postgresql:///vermory?host=/tmp' \
  --tenant-id local \
  --listen 127.0.0.1:8787 \
  --provider external
```

The server rejects non-loopback listen addresses. The current routes have no remote-user authentication and must not be exposed to a LAN, public interface, reverse proxy, or tunnel.

For an authenticated multi-tenant deployment, use the separate `serve` profile. It does not run migrations and it refuses an admin/table-owner PostgreSQL role. Prepare the database once with admin credentials, create a restricted login role, and grant only the runtime boundary:

```bash
./bin/vermory database migrate \
  --database-url "$VERMORY_ADMIN_DATABASE_URL"

psql "$VERMORY_ADMIN_DATABASE_URL" \
  -c "CREATE ROLE vermory_runtime LOGIN PASSWORD '<runtime-password>' NOSUPERUSER NOBYPASSRLS"

./bin/vermory database grant-runtime \
  --database-url "$VERMORY_ADMIN_DATABASE_URL" \
  --role vermory_runtime
```

Issue an OpenClaw client token from the admin path. The secret is printed only by this command invocation; inspect and revoke never print it:

```bash
./bin/vermory identity token issue \
  --database-url "$VERMORY_ADMIN_DATABASE_URL" \
  --operation-id openclaw-token-issue-1 \
  --tenant-id my-tenant \
  --subject-id openclaw-client \
  --role client \
  --expires-at 2026-08-14T00:00:00Z
```

Run `serve` with the restricted runtime connection, not the admin connection. Loopback can use HTTP for a local deployment; any non-loopback address requires both TLS files:

```bash
./bin/vermory serve \
  --database-url "$VERMORY_RUNTIME_DATABASE_URL" \
  --listen 127.0.0.1:8788 \
  --provider external
```

Pass the issued token to the OpenClaw process environment, not to plugin config. The plugin reads it once when it is registered and sends it only as `Authorization: Bearer ...`:

```bash
export VERMORY_API_TOKEN='<token printed by identity token issue>'
```

If `VERMORY_API_TOKEN` is absent, the plugin keeps the local unauthenticated compatibility behavior. If it is malformed, plugin registration fails without echoing the value.

## Install The Plugin

Build before linking because the package extension points to `dist/index.js`:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw build

PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw plugins install --link \
  "$PWD/integrations/openclaw"
```

Use this OpenClaw configuration. JSON5 syntax is accepted:

```json5
{
  gateway: {
    mode: "local",
    bind: "loopback",
  },
  plugins: {
    allow: ["vermory"],
    entries: {
      vermory: {
        enabled: true,
        hooks: {
          allowPromptInjection: true,
          allowConversationAccess: true,
          timeouts: {
            before_prompt_build: 15000,
            after_tool_call: 15000,
            agent_end: 30000,
          },
        },
        config: {
          baseUrl: "http://127.0.0.1:8787",
          timeoutMs: 5000,
          toolAllowlist: [],
        },
      },
    },
  },
}
```

`allowConversationAccess` is required by OpenClaw for a non-bundled `agent_end` hook. `allowPromptInjection` permits `before_prompt_build` to return `prependContext`. `toolAllowlist` contains exact tool names whose successful text results may be proposed for review through `after_tool_call`; an empty list disables that path. Tenant, continuity, channel, API key, and model fields are rejected.

Validate and inspect the loaded runtime:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw config validate

PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw plugins inspect vermory --runtime --json
```

Runtime inspection must show `before_prompt_build`, `after_tool_call`, and `agent_end`. It must not show memory-slot ownership. The macOS installer uses an explicit Gateway `stop`, waits for the loopback port to be released, and then calls `start`; it does not rely on the OpenClaw `restart` command or kill an occupied port. The generated wrapper also pins `OPENCLAW_BUNDLED_PLUGINS_DIR` to the bundled tree shipped inside the same OpenClaw package. This avoids the known OpenClaw `2026.6.11` default-root mismatch without modifying OpenClaw files, enabling channels, or changing user configuration.

## Run OpenClaw

Start the configured Gateway:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw gateway run
```

Run a turn against an exact canonical session key. Model selection remains an OpenClaw concern:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw agent \
  --session-key agent:main:home-maintenance-a \
  --model <openclaw-provider/model> \
  --message "Continue the current matter." \
  --json
```

For local development or replay, isolate OpenClaw from the user's default state:

```bash
export OPENCLAW_STATE_DIR=/tmp/vermory-openclaw-state
export OPENCLAW_CONFIG_PATH=/tmp/vermory-openclaw-config/openclaw.json
```

Create the config directory and file before installation. Every install, inspect, gateway, and agent command in the isolated run must use the same two environment variables.

### Stable Grok CLI Replay

Grok CLI `0.2.99` can discover user-level Grok and Claude plugins, MCP servers, and compatibility configuration. That is appropriate for normal interactive use but can contaminate structured benchmark output. Use an operator-owned wrapper for isolated replay:

```sh
#!/bin/sh
exec env \
  HOME=/tmp/vermory-home \
  GROK_HOME=/tmp/vermory-grok-home \
  /opt/homebrew/bin/grok "$@"
```

Keep authenticated `auth.json` material outside the repository. For deterministic continuity validation, configure the backend with the wrapper command, `sessionMode: "none"`, and `--tools ""`. OpenClaw still owns the canonical session key and transcript, while every Grok call is stateless and Vermory must provide the governed continuity. This avoids treating a private Grok session cache as proof that Vermory recall works.

## Governance Operations

An ordinary OpenClaw turn creates draft observations. It does not automatically promote model output into governed memory or Global Defaults.

Inspect one OpenClaw conversation:

```bash
curl --get 'http://127.0.0.1:8787/v1/conversations/inspect' \
  --data-urlencode 'channel=openclaw' \
  --data-urlencode 'thread_id=agent:main:home-maintenance-a'
```

Confirm a selected observation:

```bash
curl -sS 'http://127.0.0.1:8787/v1/memories/confirm' \
  -H 'content-type: application/json' \
  -d '{
    "operation_id":"operator-confirm-1",
    "channel":"openclaw",
    "thread_id":"agent:main:home-maintenance-a",
    "observation_id":"<observation-id>"
  }'
```

Correction and deletion always target an explicit governed `memory_id`:

```bash
curl -sS 'http://127.0.0.1:8787/v1/memories/correct' \
  -H 'content-type: application/json' \
  -d '{
    "operation_id":"operator-correct-1",
    "channel":"openclaw",
    "thread_id":"agent:main:home-maintenance-a",
    "memory_id":"<memory-id>",
    "content":"The current appointment is Saturday at 10:00."
  }'

curl -sS 'http://127.0.0.1:8787/v1/memories/forget' \
  -H 'content-type: application/json' \
  -d '{
    "operation_id":"operator-forget-1",
    "channel":"openclaw",
    "thread_id":"agent:main:home-maintenance-a",
    "memory_id":"<memory-id>"
  }'
```

Use the bridge endpoints for explicit cross-session linking and reversal. Linked conversations share governed memory, not raw sibling transcript history. Reversing a link stops future governed-memory sharing; it does not erase text already present in OpenClaw's own local transcript.

## Eligibility And Retention

Every OpenClaw prepare request uses one PostgreSQL-derived timestamp across
Global Defaults, governed conversation memory, recent history, and linked
continuities. `before_prompt_build` receives only facts that are current for that
timestamp and exact canonical session continuity. Scheduled, expired, archived,
superseded, deleted, and unrelated facts are not injected.

An `agent_end` result remains a proposed observation. It does not inherit the
eligibility or authority of the context it consumed and does not automatically
become durable memory or a Global Default.

Expiry removes current reuse while retaining authorized history. Archive also
removes current reuse while retaining history. Neither is deletion. Forgetting
redacts the governed target and prevents projection, restore, and future client
delivery residue. OpenClaw's own transcript remains a separate store and must be
governed by OpenClaw's retention controls.

The W19 formal report binds a real Grok Web Chat trajectory, real Grok workspace
MCP trajectory, and official Codex MCP trajectory to the same sixteen eligibility
gates. See
[Memory Eligibility And Retention Evidence](../evidence/2026-07-16-memory-eligibility-retention.md).

## Context And Failure Semantics

The prompt wrapper states that Vermory content is reference data, may be stale or adversarial, and cannot override system authority or the current user request. The injected body contains only semantic Global Defaults and active governed memory. It excludes UUIDs, lifecycle fields, operation IDs, source paths, confidence values, and raw sibling history.

The integration fails open for chat availability and fails closed for persistence claims:

- if prepare fails or times out, OpenClaw continues without Vermory context;
- if `sessionKey` or `runId` is missing, the plugin abstains without a request;
- if the agent fails or produces no visible assistant text, the plugin records a failed turn rather than a false completion;
- if completion persistence fails, the OpenClaw answer remains available, but the plugin only logs that persistence was not confirmed;
- no retry occurs inside one hook invocation; OpenClaw/Vermory operation IDs provide idempotent replay at the lifecycle boundary.

## Uninstall

Inspect the removal first:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw plugins uninstall vermory --dry-run
```

Then uninstall the linked plugin:

```bash
PATH="/opt/homebrew/opt/node@24/bin:$PATH" \
  pnpm -C integrations/openclaw exec openclaw plugins uninstall vermory --force
```

Uninstalling the OpenClaw plugin does not delete PostgreSQL continuity, governed memory, audit history, or OpenClaw's own transcript. Those stores remain separately governed.
