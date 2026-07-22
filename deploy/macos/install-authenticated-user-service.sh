#!/bin/sh

set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 /path/to/vermory /path/to/vermory-authenticated.env" >&2
  exit 2
fi

SOURCE_BINARY=$1
SOURCE_ENV_FILE=$2
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
SOURCE_RUNNER="$SCRIPT_DIR/run-authenticated-service.sh"

if [ ! -x "$SOURCE_BINARY" ]; then
  echo "vermory binary is not executable" >&2
  exit 2
fi
if [ ! -x "$SOURCE_RUNNER" ]; then
  echo "authenticated service runner is not executable" >&2
  exit 2
fi
if [ ! -f "$SOURCE_ENV_FILE" ]; then
  echo "authenticated service environment does not exist" >&2
  exit 2
fi
if [ "$(/usr/bin/stat -f '%Lp' "$SOURCE_ENV_FILE")" != "600" ]; then
  echo "authenticated service environment must have mode 600" >&2
  exit 2
fi

LABEL=${VERMORY_LAUNCHD_LABEL:-org.vermory.authenticated-web-chat}
APP_DIR="$HOME/Library/Application Support/Vermory/Authenticated"
LOG_DIR="$HOME/Library/Logs/Vermory"
LAUNCH_AGENTS="$HOME/Library/LaunchAgents"
INSTALL_BINARY=${VERMORY_INSTALL_BINARY:-"$APP_DIR/bin/vermory"}
INSTALL_RUNNER="$APP_DIR/bin/run-authenticated-service.sh"
INSTALL_ENV_FILE=${VERMORY_INSTALL_ENV_FILE:-"$APP_DIR/vermory-authenticated.env"}
LOG_BASENAME=${VERMORY_LOG_BASENAME:-$LABEL}
PLIST_PATH="$LAUNCH_AGENTS/$LABEL.plist"
USER_DOMAIN="gui/$(id -u)"

case "$LABEL" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LAUNCHD_LABEL" >&2; exit 2 ;;
esac
for path in "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE"; do
  case "$path" in
    "$HOME"/*) ;;
    *) echo "authenticated service paths must be inside HOME" >&2; exit 2 ;;
  esac
  case "$path" in
    */../*|*/./*|*/..|*/.) echo "authenticated service paths must not contain dot segments" >&2; exit 2 ;;
  esac
done
case "$LOG_BASENAME" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LOG_BASENAME" >&2; exit 2 ;;
esac

/usr/bin/install -d -m 0755 "$(dirname "$INSTALL_BINARY")" "$LOG_DIR" "$LAUNCH_AGENTS"
/usr/bin/install -m 0755 "$SOURCE_BINARY" "$INSTALL_BINARY.new"
/bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"
/usr/bin/install -m 0755 "$SOURCE_RUNNER" "$INSTALL_RUNNER.new"
/bin/mv "$INSTALL_RUNNER.new" "$INSTALL_RUNNER"
/usr/bin/install -m 0600 "$SOURCE_ENV_FILE" "$INSTALL_ENV_FILE.new"
/bin/mv "$INSTALL_ENV_FILE.new" "$INSTALL_ENV_FILE"

set -a
. "$INSTALL_ENV_FILE"
set +a

: "${VERMORY_DATABASE_URL:?VERMORY_DATABASE_URL is required}"
: "${VERMORY_LISTEN:?VERMORY_LISTEN is required}"
case "$VERMORY_LISTEN" in
  127.0.0.1:[0-9]*|localhost:[0-9]*|'[::1]':[0-9]*) ;;
  *) echo "VERMORY_LISTEN must use a loopback address" >&2; exit 2 ;;
esac

TEMP_PLIST="$APP_DIR/$LABEL.plist.new"
cat >"$TEMP_PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$INSTALL_RUNNER</string>
    <string>$INSTALL_ENV_FILE</string>
    <string>$INSTALL_BINARY</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>ThrottleInterval</key>
  <integer>5</integer>
  <key>StandardOutPath</key>
  <string>$LOG_DIR/$LOG_BASENAME.stdout.log</string>
  <key>StandardErrorPath</key>
  <string>$LOG_DIR/$LOG_BASENAME.stderr.log</string>
</dict>
</plist>
EOF

/usr/bin/plutil -lint "$TEMP_PLIST" >/dev/null
/usr/bin/install -m 0644 "$TEMP_PLIST" "$PLIST_PATH"
/bin/rm "$TEMP_PLIST"

/bin/launchctl bootout "$USER_DOMAIN" "$PLIST_PATH" >/dev/null 2>&1 || true
/bin/launchctl bootstrap "$USER_DOMAIN" "$PLIST_PATH"
/bin/launchctl enable "$USER_DOMAIN/$LABEL"
/bin/launchctl kickstart -k "$USER_DOMAIN/$LABEL"

ROOT_URL="http://$VERMORY_LISTEN/"
SESSION_URL="http://$VERMORY_LISTEN/v1/session"
attempt=0
while [ "$attempt" -lt 30 ]; do
  root_code=$(/usr/bin/curl --noproxy '*' -s -o /dev/null -w '%{http_code}' --max-time 3 "$ROOT_URL" || true)
  session_code=$(/usr/bin/curl --noproxy '*' -s -o /dev/null -w '%{http_code}' --max-time 3 "$SESSION_URL" || true)
  if [ "$root_code" = "200" ] && [ "$session_code" = "401" ]; then
    VERSION=$("$INSTALL_BINARY" version 2>/dev/null | /usr/bin/tr '\n' ' ')
    echo "service=$LABEL state=running listen=$VERMORY_LISTEN root_code=$root_code session_code=$session_code version=$VERSION"
    exit 0
  fi
  attempt=$((attempt + 1))
  /bin/sleep 1
done

echo "authenticated Vermory service did not become ready" >&2
/bin/launchctl print "$USER_DOMAIN/$LABEL" >&2 || true
/usr/bin/tail -n 80 "$LOG_DIR/$LOG_BASENAME.stderr.log" >&2 || true
exit 1
