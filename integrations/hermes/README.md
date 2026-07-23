# Hermes Integration

This integration connects the official
[`NousResearch/hermes-agent`](https://github.com/NousResearch/hermes-agent)
memory-provider lifecycle to Vermory conversation continuity.

Hermes calls Vermory before and after each completed agent turn:

```text
Hermes MemoryProvider.prefetch
-> POST /v1/integrations/hermes/turns/prepare
-> governed current context is injected as reference data
-> Hermes produces its answer
-> MemoryProvider.sync_turn
-> POST /v1/integrations/hermes/turns/complete
-> user and assistant observations remain governed in Vermory
```

The provider exposes no model tools. Hermes cannot confirm, correct, forget,
link, promote, or rebind memory through this adapter. Those remain explicit
Vermory governance operations.

## Continuity Identity

Gateway sessions use Hermes' durable `gateway_session_key`. Internal session
rotation and context compression therefore do not split the same messaging
thread. CLI sessions use the Hermes session ID and remain isolated by default.
Hermes and OpenClaw use separate conversation channels even when their raw
session keys are identical; linking them is an explicit bridge action.

## Install On A Hermes Host

On the Mac mini, install the pinned official Hermes checkout as a normal user:

```bash
VERMORY_HERMES_PROXY=http://127.0.0.1:6152 \
  deploy/macos/install-hermes.sh
```

The runner installs under `$HOME/.vermory/hermes`, never invokes `sudo`, uses
the official staged installer to skip Node/browser/desktop dependencies and the
interactive provider wizard, and verifies the exact upstream revision. Then
copy the provider from the repository or extracted release package into the
active `HERMES_HOME`:

```bash
mkdir -p "${HERMES_HOME:-$HOME/.hermes}/plugins/vermory"
cp integrations/hermes/vermory/__init__.py \
  integrations/hermes/vermory/plugin.yaml \
  "${HERMES_HOME:-$HOME/.hermes}/plugins/vermory/"
```

Build the deterministic release package from a clean committed revision:

```bash
integrations/hermes/package.sh dist
tar -tzf dist/vermory-hermes-0.1.0.tar.gz
(cd dist && shasum -a 256 -c vermory-hermes-0.1.0.tar.gz.sha256)
```

The archive contains the repository license and the exact Hermes provider
source, metadata, lock file, and integration guide. It contains no virtual
environment, bytecode, credentials, user configuration, or transcript data.

Start a loopback Vermory service with the external-provider mode, then select
the provider:

```bash
hermes memory setup vermory
hermes memory status
```

The default API URL is `http://127.0.0.1:8787`. The setup flow writes
non-secret settings to `$HERMES_HOME/vermory.json`; an optional client token is
read from `VERMORY_API_TOKEN`. URLs containing embedded credentials, query
parameters, or fragments are rejected.

Hermes currently invokes `MemoryProvider.on_turn_start` without the documented
model keyword on its CLI path. A wrapper that already selects the model should
also set `HERMES_INFERENCE_MODEL` in the same process so Vermory can record the
actual client-reported model. If neither the hook, process environment, nor
provider config reports a model, Vermory records `hermes/unreported` rather
than guessing.

For the pinned Hermes v0.18.2 CLI, top-level `--oneshot` enters the one-shot
runner before processing `--resume`. Continuity tests must create the first
turn normally and use `hermes chat -Q --resume <session-id> --query <prompt>`
for the resumed turn. Using `--oneshot --resume` creates a new session and is
not valid continuity evidence.

## Failure Behavior

Prepare failures return no external context and do not block the Hermes turn.
Completion failures do not hide the visible Hermes answer. Responses are
bounded to 256 KiB, and raw HTTP bodies or credentials are never logged.

## Verify

```bash
PYTHONDONTWRITEBYTECODE=1 \
UV_PROJECT_ENVIRONMENT=/tmp/vermory-hermes \
uv run --project integrations/hermes --locked python -m unittest discover \
  -s integrations/hermes/tests -v
```

The frozen `H01-hermes-linked-sessions` contract additionally requires two
independent real Hermes sessions, explicit link and reversal evidence, direct
post-reversal delivery inspection, fail-open model availability, and a privacy
scan. Unit tests alone do not satisfy that contract.

## Qualified Real-Client Run

The accepted W20 run used official Hermes `v0.18.2`, a fresh isolated
`HERMES_HOME` for session B, and direct SiliconFlow
`deepseek-ai/DeepSeek-V4-Flash`. Session B received the confirmed current fact
only after an explicit Vermory link, returned `thesis-defense-v7.zip`, and
received zero context bytes after reversal. With only the Hermes-specific
Vermory canary unavailable, Hermes returned `FAIL-OPEN-OK` while Vermory wrote
no false persistence receipt. The model audit, user-level LaunchAgent restart,
deterministic six-file package, and zero-credential-leak gates passed.

See [Hermes Real-Client Continuity Qualification](../../docs/evidence/2026-07-18-hermes-real-client.md)
for the accepted trajectory, rejected false positive, failure ledger, package
inventory, and explicit non-claims.
