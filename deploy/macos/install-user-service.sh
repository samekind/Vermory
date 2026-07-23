#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 /path/to/vermory" >&2
  exit 2
fi

SOURCE_BINARY=$1
if [ ! -x "$SOURCE_BINARY" ]; then
  echo "vermory binary is not executable: $SOURCE_BINARY" >&2
  exit 2
fi

DATABASE_NAME=${VERMORY_DATABASE_NAME:-vermory}
TENANT_ID=${VERMORY_TENANT_ID:-local}
LISTEN=${VERMORY_LISTEN:-127.0.0.1:8787}
PROVIDER=${VERMORY_PROVIDER:-external}
LABEL=${VERMORY_LAUNCHD_LABEL:-org.vermory.web-chat}
POSTGRES_BIN=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@18/bin}

case "$DATABASE_NAME" in
  *[!A-Za-z0-9_-]*|'') echo "invalid VERMORY_DATABASE_NAME" >&2; exit 2 ;;
esac
case "$TENANT_ID" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_TENANT_ID" >&2; exit 2 ;;
esac
case "$LABEL" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LAUNCHD_LABEL" >&2; exit 2 ;;
esac
case "$LISTEN" in
  127.0.0.1:[0-9]*|localhost:[0-9]*) ;;
  *) echo "VERMORY_LISTEN must be loopback" >&2; exit 2 ;;
esac

BIN_DIR="$HOME/.local/bin"
APP_DIR="$HOME/Library/Application Support/Vermory"
LOG_DIR="$HOME/Library/Logs/Vermory"
LAUNCH_AGENTS="$HOME/Library/LaunchAgents"
INSTALL_BINARY=${VERMORY_INSTALL_BINARY:-"$BIN_DIR/vermory"}
LOG_BASENAME=${VERMORY_LOG_BASENAME:-$LABEL}
PLIST_PATH="$LAUNCH_AGENTS/$LABEL.plist"
DATABASE_URL="postgresql:///$DATABASE_NAME?host=/tmp"
USER_DOMAIN="gui/$(id -u)"

case "$INSTALL_BINARY" in
  "$HOME"/*) ;;
  *) echo "VERMORY_INSTALL_BINARY must be inside HOME" >&2; exit 2 ;;
esac
case "$INSTALL_BINARY" in
  */../*|*/./*|*/..|*/.) echo "VERMORY_INSTALL_BINARY must not contain dot path segments" >&2; exit 2 ;;
esac
case "$LOG_BASENAME" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LOG_BASENAME" >&2; exit 2 ;;
esac

/usr/bin/install -d -m 0755 "$(dirname "$INSTALL_BINARY")" "$APP_DIR" "$LOG_DIR" "$LAUNCH_AGENTS"
/usr/bin/install -m 0755 "$SOURCE_BINARY" "$INSTALL_BINARY.new"
/bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"

if [ ! -x "$POSTGRES_BIN/psql" ] || [ ! -x "$POSTGRES_BIN/createdb" ]; then
  echo "PostgreSQL client tools are unavailable at $POSTGRES_BIN" >&2
  exit 1
fi
if ! "$POSTGRES_BIN/psql" -h /tmp -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$DATABASE_NAME'" | /usr/bin/grep -qx 1; then
  "$POSTGRES_BIN/createdb" -h /tmp "$DATABASE_NAME"
fi

"$INSTALL_BINARY" database migrate --database-url "$DATABASE_URL"

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
    <string>$INSTALL_BINARY</string>
    <string>web-chat</string>
    <string>--database-url</string>
    <string>$DATABASE_URL</string>
    <string>--tenant-id</string>
    <string>$TENANT_ID</string>
    <string>--listen</string>
    <string>$LISTEN</string>
    <string>--provider</string>
    <string>$PROVIDER</string>
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

HEALTH_URL="http://$LISTEN/v1/defaults"
attempt=0
while [ "$attempt" -lt 30 ]; do
  if /usr/bin/curl -fsS --noproxy '*' "$HEALTH_URL" >/dev/null 2>&1; then
    VERSION=$($INSTALL_BINARY version 2>/dev/null | /usr/bin/tr '\n' ' ')
    echo "service=$LABEL state=running database=$DATABASE_NAME listen=$LISTEN version=$VERSION"
    exit 0
  fi
  attempt=$((attempt + 1))
  /bin/sleep 1
done

echo "Vermory did not become healthy at $HEALTH_URL" >&2
/bin/launchctl print "$USER_DOMAIN/$LABEL" >&2 || true
/usr/bin/tail -n 80 "$LOG_DIR/$LOG_BASENAME.stderr.log" >&2 || true
exit 1
