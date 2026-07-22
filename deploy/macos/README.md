# macOS User Service

This profile installs Vermory as a per-user `launchd` service. It requires no
root privileges and keeps PostgreSQL, service state, logs, and client runtimes
on the target Mac rather than on the invoking workstation.

Requirements:

- PostgreSQL 18 listening on the local Unix socket;
- a Darwin arm64 or amd64 Vermory binary;
- an active graphical user launchd domain.

Install or update:

```bash
VERMORY_DATABASE_NAME=vermory \
VERMORY_TENANT_ID=local \
VERMORY_LISTEN=127.0.0.1:8787 \
VERMORY_PROVIDER=external \
  ./deploy/macos/install-user-service.sh /path/to/vermory
```

The installer:

- copies the binary to `~/.local/bin/vermory`;
- creates the database if needed and applies migrations;
- installs `~/Library/LaunchAgents/org.vermory.web-chat.plist`;
- stores logs under `~/Library/Logs/Vermory`;
- verifies the real loopback Web Chat endpoint before returning success.

The default service intentionally listens only on loopback. Remote clients
should use SSH stdio or an operator-controlled SSH tunnel instead of exposing
the unauthenticated local Web Chat profile to a LAN or public network.

Run an independent loopback instance without replacing the stable binary or
sharing log files by setting a separate label, tenant, listen address, install
path, and log basename:

```bash
VERMORY_TENANT_ID=hermes-canary \
VERMORY_LISTEN=127.0.0.1:8789 \
VERMORY_PROVIDER=external \
VERMORY_LAUNCHD_LABEL=org.vermory.hermes-canary \
VERMORY_INSTALL_BINARY="$HOME/.vermory/vermory-hermes/vermory" \
VERMORY_LOG_BASENAME=hermes-canary \
  ./deploy/macos/install-user-service.sh /path/to/vermory
```

Every install path must remain inside the current user's home directory. The
installer only operates in the user's `gui/<uid>` launchd domain and never
invokes `sudo`.

## Authenticated Web Chat

`install-authenticated-user-service.sh` installs the multi-tenant `serve`
profile as a separate unprivileged LaunchAgent. It does not migrate the
database or grant PostgreSQL privileges. Provision those boundaries with an
administrator connection first, then place only runtime settings and provider
credentials in a `0600` environment file under the service user's home.

```bash
chmod 600 "$HOME/Library/Application Support/Vermory/vermory-authenticated.env"
./deploy/macos/install-authenticated-user-service.sh \
  ./bin/vermory \
  "$HOME/Library/Application Support/Vermory/vermory-authenticated.env"
```

The LaunchAgent plist contains the environment-file path, not its contents.
The service remains on loopback so a separately managed HTTPS entrypoint can
terminate TLS without exposing a direct cleartext listener.

An update is accepted only after the candidate binary reports its version,
passes the read-only database compatibility check, starts through the existing
user LaunchAgent, and reaches the authenticated health boundary. A complete
previous installation is retained in one protected rollback slot. If candidate
activation fails, the installer restores and verifies that previous
installation before returning an error.

Roll back explicitly to the immediately preceding compatible installation:

```bash
./deploy/macos/rollback-authenticated-user-service.sh
```

Rollback checks schema compatibility before changing live files, restores the
binary, runner, and environment together, and never performs a database down
migration. The same `VERMORY_APP_DIR`, path, label, health-attempt, and command
override variables accepted by the installer can be supplied to an isolated
qualification run.

## OpenClaw Gateway

After installing dependencies and building `integrations/openclaw`, install the
official OpenClaw Gateway with the Vermory lifecycle plugin:

```bash
./deploy/macos/install-openclaw-service.sh /path/to/integrations/openclaw
```

The installer keeps OpenClaw state, config, workspace, package caches, runtime
inspection, and health evidence under `~/Library/Application Support/Vermory`.
It calls OpenClaw's official Gateway service installer rather than maintaining
a second custom Gateway plist. Re-running the installer merges Vermory's required
settings into the existing OpenClaw config, preserves Gateway authentication,
CLI backends, channels, and unrelated plugins, and keeps the config mode at
`0600`. The Gateway and Vermory API remain loopback-only.

## Grok CLI Runtime

Install the official signed Grok binary and an existing authenticated
`auth.json` into an isolated Vermory-owned runtime on the target Mac:

```bash
./deploy/macos/install-grok-runtime.sh /path/to/grok /path/to/auth.json
./deploy/macos/install-openclaw-service.sh /path/to/integrations/openclaw
```

The Grok installer stores the binary, mutable auth state, and runtime home under
`~/Library/Application Support/Vermory/grok`. `grok-vermory` supplies the
isolated home, protected auth, proxy, and disabled ambient memory environment
for Vermory's provider adapter, which owns its own no-tool arguments.
`grok-vermory-isolated` additionally enforces one-turn, no-plan, no-subagent,
no-web, and no-tool arguments for OpenClaw. Reinstalling updates the binary but
preserves the target host's refreshed auth file unless
`VERMORY_GROK_REPLACE_AUTH=1` is explicitly set. The OpenClaw installer detects
the wrapper and registers the isolated `grok-cli` backend without replacing
other backend configuration.

If the target host requires a local HTTP proxy, pass an unauthenticated loopback
URL when installing. Non-loopback and credential-bearing proxy URLs are rejected:

```bash
VERMORY_GROK_PROXY_URL=http://127.0.0.1:6152 \
  ./deploy/macos/install-grok-runtime.sh /path/to/grok /path/to/auth.json
```

## W19 Formal Qualification

The macOS formal runner reads the direct SiliconFlow credential from Keychain,
requires a clean exact Git revision, starts a disposable PostgreSQL 18 cluster,
and keeps the database and reports under the target Mac's Vermory application
support directory. The credential is never accepted as a command-line flag or
written to the report.

Add or update the Keychain item interactively. Keep `-w` as the final option so
the value is entered through the hidden prompt:

```bash
security add-generic-password -U -a "$USER" -s vermory-siliconflow -w
```

Run the profile from the exact checkout that will be reported:

```bash
VERMORY_W19_RUN_ID=w19-formal-20260717 \
VERMORY_W19_POSTGRES_ROOT="$HOME/Library/Application Support/Vermory/evidence/w19/w19-formal-20260717/postgres" \
  ./deploy/macos/run-w19-formal.sh
```

The runner refuses dirty worktrees, unsafe run identifiers, existing report
paths, missing PostgreSQL 18 binaries, and missing or empty Keychain items.
