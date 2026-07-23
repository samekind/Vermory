#!/bin/sh

set -eu

umask 077

if [ "$#" -ne 2 ]; then
  echo "usage: $0 /path/to/grok /path/to/auth.json" >&2
  exit 2
fi

SOURCE_BINARY=$1
SOURCE_AUTH=$2
JQ=${JQ:-/usr/bin/jq}
REQUIRE_SIGNATURE=${VERMORY_GROK_REQUIRE_SIGNATURE:-1}
PROXY_URL=${VERMORY_GROK_PROXY_URL:-}

if [ ! -x "$SOURCE_BINARY" ]; then
  echo "Grok binary is not executable: $SOURCE_BINARY" >&2
  exit 2
fi
if [ ! -f "$SOURCE_AUTH" ]; then
  echo "Grok auth file is missing: $SOURCE_AUTH" >&2
  exit 2
fi
if [ ! -x "$JQ" ]; then
  echo "jq is unavailable: $JQ" >&2
  exit 2
fi
if ! "$JQ" -e '
  type == "object" and length > 0 and
  all(.[]; type == "object" and (.key | type == "string") and (.refresh_token | type == "string"))
' "$SOURCE_AUTH" >/dev/null; then
  echo "Grok auth file has an unsupported shape" >&2
  exit 2
fi

case "$REQUIRE_SIGNATURE" in
  0) ;;
  1)
    if ! /usr/bin/codesign --verify --strict "$SOURCE_BINARY" >/dev/null 2>&1; then
      echo "Grok binary does not have a valid macOS code signature" >&2
      exit 2
    fi
    ;;
  *) echo "VERMORY_GROK_REQUIRE_SIGNATURE must be 0 or 1" >&2; exit 2 ;;
esac
case "$PROXY_URL" in
  '') ;;
  http://127.0.0.1:[0-9]*|http://localhost:[0-9]*) ;;
  *) echo "VERMORY_GROK_PROXY_URL must be an unauthenticated loopback HTTP proxy" >&2; exit 2 ;;
esac

APP_DIR=${VERMORY_APP_DIR:-"$HOME/Library/Application Support/Vermory"}
GROK_DIR="$APP_DIR/grok"
BIN_DIR="$GROK_DIR/bin"
HOME_DIR="$GROK_DIR/home"
STATE_DIR="$GROK_DIR/state"
INSTALL_BINARY="$BIN_DIR/grok"
INSTALL_AUTH="$STATE_DIR/auth.json"
WRAPPER_PATH="$GROK_DIR/grok-vermory"
ISOLATED_WRAPPER_PATH="$GROK_DIR/grok-vermory-isolated"

/usr/bin/install -d -m 0700 "$GROK_DIR" "$BIN_DIR" "$HOME_DIR" "$STATE_DIR"
/usr/bin/install -m 0755 "$SOURCE_BINARY" "$INSTALL_BINARY.new"
/bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"

if [ ! -f "$INSTALL_AUTH" ] || [ "${VERMORY_GROK_REPLACE_AUTH:-0}" = "1" ]; then
  /usr/bin/install -m 0600 "$SOURCE_AUTH" "$INSTALL_AUTH.new"
  /bin/mv "$INSTALL_AUTH.new" "$INSTALL_AUTH"
fi
/bin/chmod 0600 "$INSTALL_AUTH"

cat >"$WRAPPER_PATH" <<EOF
#!/bin/sh
set -eu
export PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
export HOME="$HOME_DIR"
export GROK_HOME="$STATE_DIR"
export GROK_MEMORY=0
export GROK_SUBAGENTS=0
export GROK_WEB_FETCH=0
export GROK_FEEDBACK_ENABLED=0
export RUST_LOG=error
unset XAI_API_KEY GROK_AUTH_PROVIDER_COMMAND GROK_DEPLOYMENT_KEY
if [ -n "$PROXY_URL" ]; then
  export HTTP_PROXY="$PROXY_URL"
  export HTTPS_PROXY="$PROXY_URL"
  export http_proxy="$PROXY_URL"
  export https_proxy="$PROXY_URL"
  export NO_PROXY="127.0.0.1,localhost"
  export no_proxy="127.0.0.1,localhost"
fi
exec "$INSTALL_BINARY" "\$@"
EOF
/bin/chmod 0755 "$WRAPPER_PATH"

cat >"$ISOLATED_WRAPPER_PATH" <<EOF
#!/bin/sh
set -eu
exec "$WRAPPER_PATH" \
  --no-memory \
  --no-plan \
  --no-subagents \
  --disable-web-search \
  --tools todo_write \
  --disallowed-tools todo_write,update_goal,search_tool,use_tool,CallMcpTool,Agent \
  --permission-mode dontAsk \
  --max-turns 1 \
  --verbatim \
  "\$@"
EOF
/bin/chmod 0755 "$ISOLATED_WRAPPER_PATH"

VERSION=$("$WRAPPER_PATH" --version 2>&1 | /usr/bin/head -n 1)
echo "runtime=grok state=installed auth=protected wrapper=$WRAPPER_PATH isolated_wrapper=$ISOLATED_WRAPPER_PATH version=$VERSION"
