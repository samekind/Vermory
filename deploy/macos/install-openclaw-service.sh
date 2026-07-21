#!/bin/sh

set -eu

umask 077

if [ "$#" -ne 1 ]; then
  echo "usage: $0 /path/to/openclaw-plugin" >&2
  exit 2
fi

PLUGIN_DIR=$1
OPENCLAW_CLI="$PLUGIN_DIR/node_modules/.bin/openclaw"
if [ ! -x "$OPENCLAW_CLI" ] || [ ! -f "$PLUGIN_DIR/dist/index.js" ]; then
  echo "OpenClaw runtime or built Vermory plugin is missing: $PLUGIN_DIR" >&2
  exit 2
fi

APP_DIR=${VERMORY_APP_DIR:-"$HOME/Library/Application Support/Vermory"}
OPENCLAW_DIR="$APP_DIR/openclaw"
STATE_DIR="$OPENCLAW_DIR/state"
CONFIG_DIR="$OPENCLAW_DIR/config"
WORKSPACE_DIR="$OPENCLAW_DIR/workspace"
CONFIG_PATH="$CONFIG_DIR/openclaw.json"
WRAPPER_PATH="$OPENCLAW_DIR/openclaw-vermory"
GROK_WRAPPER_PATH=${VERMORY_GROK_WRAPPER_PATH:-"$APP_DIR/grok/grok-vermory-isolated"}
PORT=${OPENCLAW_GATEWAY_PORT:-18789}
JQ=${JQ:-/usr/bin/jq}
LSOF=${LSOF:-/usr/sbin/lsof}
VERMORY_BASE_URL=${VERMORY_OPENCLAW_BASE_URL:-http://127.0.0.1:8787}
TOOL_ALLOWLIST_JSON=${VERMORY_OPENCLAW_TOOL_ALLOWLIST_JSON:-[]}

case "$PORT" in
  *[!0-9]*|'') echo "invalid OPENCLAW_GATEWAY_PORT" >&2; exit 2 ;;
esac
if [ ! -x "$JQ" ]; then
  echo "jq is unavailable: $JQ" >&2
  exit 2
fi
if [ ! -x "$LSOF" ]; then
  echo "lsof is unavailable: $LSOF" >&2
  exit 2
fi
if ! "$JQ" -en --arg url "$VERMORY_BASE_URL" '
  $url
  | capture("^http://(?:127\\.0\\.0\\.1|localhost):(?<port>[0-9]{1,5})$")
  | (.port | tonumber) >= 1 and (.port | tonumber) <= 65535
' >/dev/null; then
  echo "VERMORY_OPENCLAW_BASE_URL must be a loopback HTTP URL with a valid port" >&2
  exit 2
fi
if ! /usr/bin/printf '%s\n' "$TOOL_ALLOWLIST_JSON" | "$JQ" -e '
  type == "array"
  and length <= 64
  and all(.[]; type == "string" and test("^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$"))
  and (unique | length) == length
' >/dev/null; then
  echo "VERMORY_OPENCLAW_TOOL_ALLOWLIST_JSON must be a unique array of valid tool names" >&2
  exit 2
fi

/usr/bin/install -d -m 0755 \
  "$STATE_DIR" "$CONFIG_DIR" "$WORKSPACE_DIR" \
  "$APP_DIR/corepack" "$APP_DIR/cache" "$APP_DIR/pnpm-store" \
  "$HOME/Library/Logs/Vermory"

merge_openclaw_config() {
  input_path=$CONFIG_PATH
  empty_input="$CONFIG_DIR/.openclaw-empty.$$.json"
  merged_path="$CONFIG_DIR/.openclaw-merged.$$.json"

  if [ -f "$input_path" ]; then
    if ! "$JQ" empty "$input_path" >/dev/null 2>&1; then
      echo "refusing to overwrite invalid OpenClaw config: $input_path" >&2
      exit 1
    fi
  else
    /usr/bin/printf '{}\n' >"$empty_input"
    input_path=$empty_input
  fi

  # The filter is single-quoted so jq, not the shell, expands its variables.
  # shellcheck disable=SC2016
  "$JQ" \
    --arg workspace "$WORKSPACE_DIR" \
    --arg grokCommand "$GROK_WRAPPER_PATH" \
    --arg vermoryBaseURL "$VERMORY_BASE_URL" \
    --argjson toolAllowlist "$TOOL_ALLOWLIST_JSON" \
    --argjson grokEnabled "$(if [ -x "$GROK_WRAPPER_PATH" ]; then echo true; else echo false; fi)" \
    --argjson port "$PORT" \
    '
      .gateway = ((.gateway // {}) + {
        mode: "local",
        bind: "loopback",
        port: $port
      })
      | .agents = (.agents // {})
      | .agents.defaults = ((.agents.defaults // {}) + {
          workspace: $workspace
        })
      | if $grokEnabled then
          .agents.defaults.cliBackends = (.agents.defaults.cliBackends // {})
          | .agents.defaults.cliBackends."grok-cli" = ((.agents.defaults.cliBackends."grok-cli" // {}) * {
              command: $grokCommand,
              args: ["--output-format", "json", "--single", "{prompt}"],
              output: "json",
              input: "arg",
              modelArg: "--model",
              sessionMode: "none",
              serialize: true,
              clearEnv: ["XAI_API_KEY", "GROK_AUTH_PROVIDER_COMMAND", "GROK_DEPLOYMENT_KEY"]
            })
        else . end
      | .plugins = (.plugins // {})
      | .plugins.allow = (((.plugins.allow // []) + ["vermory"]) | unique)
      | .plugins.entries = (.plugins.entries // {})
      | .plugins.entries.vermory = ((.plugins.entries.vermory // {}) * {
          enabled: true,
          hooks: {
            allowPromptInjection: true,
            allowConversationAccess: true,
            timeouts: {
              before_prompt_build: 15000,
              after_tool_call: 15000,
              agent_end: 30000
            }
          },
          config: {
            baseUrl: $vermoryBaseURL,
            timeoutMs: 5000,
            toolAllowlist: $toolAllowlist
          }
        })
    ' "$input_path" >"$merged_path"

  "$JQ" empty "$merged_path" >/dev/null
  /usr/bin/install -m 0600 "$merged_path" "$CONFIG_PATH"
  /bin/rm -f "$merged_path" "$empty_input"
}

merge_openclaw_config

export PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
export COREPACK_HOME="$APP_DIR/corepack"
export XDG_CACHE_HOME="$APP_DIR/cache"
export OPENCLAW_STATE_DIR="$STATE_DIR"
export OPENCLAW_CONFIG_PATH="$CONFIG_PATH"

if ! "$OPENCLAW_CLI" plugins inspect vermory --json >/dev/null 2>&1; then
  "$OPENCLAW_CLI" plugins install --link "$PLUGIN_DIR"
fi

merge_openclaw_config

cat >"$WRAPPER_PATH" <<EOF
#!/bin/sh
export PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
export COREPACK_HOME="$APP_DIR/corepack"
export XDG_CACHE_HOME="$APP_DIR/cache"
export OPENCLAW_STATE_DIR="$STATE_DIR"
export OPENCLAW_CONFIG_PATH="$CONFIG_PATH"
exec "$OPENCLAW_CLI" "\$@"
EOF
/bin/chmod 0755 "$WRAPPER_PATH"

"$WRAPPER_PATH" config validate
"$WRAPPER_PATH" plugins inspect vermory --runtime --json >"$OPENCLAW_DIR/vermory-plugin-runtime.json"
"$WRAPPER_PATH" gateway install --force --runtime node --port "$PORT" --wrapper "$WRAPPER_PATH" --json >/dev/null

if ! "$WRAPPER_PATH" gateway stop --json >/dev/null; then
  if "$LSOF" -nP -iTCP:"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo "OpenClaw Gateway stop failed while port $PORT remained occupied" >&2
    exit 1
  fi
fi

attempt=0
while [ "$attempt" -lt 30 ] && "$LSOF" -nP -iTCP:"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  /bin/sleep 1
done
if "$LSOF" -nP -iTCP:"$PORT" -sTCP:LISTEN -t >/dev/null 2>&1; then
  echo "OpenClaw Gateway port $PORT did not become free after stop" >&2
  exit 1
fi

"$WRAPPER_PATH" gateway start --json >/dev/null

attempt=0
while [ "$attempt" -lt 30 ]; do
  if "$WRAPPER_PATH" gateway health --json >"$OPENCLAW_DIR/gateway-health.json" 2>/dev/null; then
    echo "service=openclaw-gateway state=running port=$PORT plugin=vermory"
    exit 0
  fi
  attempt=$((attempt + 1))
  /bin/sleep 1
done

echo "OpenClaw Gateway did not become healthy" >&2
"$WRAPPER_PATH" gateway status >&2 || true
exit 1
