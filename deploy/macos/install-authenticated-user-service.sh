#!/bin/sh

set -eu
umask 077

if [ "$#" -ne 2 ]; then
  echo "usage: $0 /path/to/vermory /path/to/vermory-authenticated.env" >&2
  exit 2
fi

SOURCE_BINARY=$1
SOURCE_ENV_FILE=$2
SCRIPT_DIR=$(unset CDPATH; cd -- "$(dirname "$0")" && pwd)
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
APP_DIR=${VERMORY_APP_DIR:-"$HOME/Library/Application Support/Vermory/Authenticated"}
LOG_DIR=${VERMORY_LOG_DIR:-"$HOME/Library/Logs/Vermory"}
LAUNCH_AGENTS=${VERMORY_LAUNCH_AGENTS_DIR:-"$HOME/Library/LaunchAgents"}
INSTALL_BINARY=${VERMORY_INSTALL_BINARY:-"$APP_DIR/bin/vermory"}
INSTALL_RUNNER=${VERMORY_INSTALL_RUNNER:-"$APP_DIR/bin/run-authenticated-service.sh"}
INSTALL_ENV_FILE=${VERMORY_INSTALL_ENV_FILE:-"$APP_DIR/vermory-authenticated.env"}
ROLLBACK_DIR=${VERMORY_ROLLBACK_DIR:-"$APP_DIR/rollback"}
LOG_BASENAME=${VERMORY_LOG_BASENAME:-$LABEL}
PLIST_PATH="$LAUNCH_AGENTS/$LABEL.plist"
USER_DOMAIN="gui/$(id -u)"
LAUNCHCTL=${LAUNCHCTL:-/bin/launchctl}
CURL=${CURL:-/usr/bin/curl}
SLEEP=${SLEEP:-/bin/sleep}
HEALTH_ATTEMPTS=${VERMORY_HEALTH_ATTEMPTS:-30}
HEALTH_SLEEP_SECONDS=${VERMORY_HEALTH_SLEEP_SECONDS:-1}

case "$LABEL" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LAUNCHD_LABEL" >&2; exit 2 ;;
esac
case "$LOG_BASENAME" in
  *[!A-Za-z0-9._-]*|'') echo "invalid VERMORY_LOG_BASENAME" >&2; exit 2 ;;
esac
case "$HEALTH_ATTEMPTS" in
  *[!0-9]*|'0'|'') echo "VERMORY_HEALTH_ATTEMPTS must be a positive integer" >&2; exit 2 ;;
esac
case "$HEALTH_SLEEP_SECONDS" in
  *[!0-9]*|'') echo "VERMORY_HEALTH_SLEEP_SECONDS must be a non-negative integer" >&2; exit 2 ;;
esac
for command_path in "$LAUNCHCTL" "$CURL" "$SLEEP"; do
  if [ ! -x "$command_path" ]; then
    echo "required executable is unavailable: $command_path" >&2
    exit 2
  fi
done
for path in "$APP_DIR" "$LOG_DIR" "$LAUNCH_AGENTS" "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE" "$ROLLBACK_DIR"; do
  case "$path" in
    "$HOME"/*) ;;
    *) echo "authenticated service paths must be inside HOME" >&2; exit 2 ;;
  esac
  case "$path" in
    */../*|*/./*|*/..|*/.) echo "authenticated service paths must not contain dot segments" >&2; exit 2 ;;
  esac
done
case "$ROLLBACK_DIR" in
  "$APP_DIR"/*) ;;
  *) echo "VERMORY_ROLLBACK_DIR must be inside VERMORY_APP_DIR" >&2; exit 2 ;;
esac
for live_path in "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE"; do
  case "$live_path" in
    "$ROLLBACK_DIR"|"$ROLLBACK_DIR"/*) echo "VERMORY_ROLLBACK_DIR must not contain live service files" >&2; exit 2 ;;
  esac
done

/usr/bin/install -d -m 0755 \
  "$APP_DIR" "$(dirname "$INSTALL_BINARY")" "$(dirname "$INSTALL_RUNNER")" \
  "$(dirname "$INSTALL_ENV_FILE")" "$LOG_DIR" "$LAUNCH_AGENTS"

STAGE_DIR="$APP_DIR/stage.$$"
/usr/bin/install -d -m 0700 "$STAGE_DIR"
# Invoked by the trap below.
# shellcheck disable=SC2329
cleanup() {
  /bin/rm -rf "$STAGE_DIR"
}
trap cleanup EXIT HUP INT TERM

STAGED_BINARY="$STAGE_DIR/vermory"
STAGED_RUNNER="$STAGE_DIR/run-authenticated-service.sh"
STAGED_ENV_FILE="$STAGE_DIR/vermory-authenticated.env"
/usr/bin/install -m 0755 "$SOURCE_BINARY" "$STAGED_BINARY"
/usr/bin/install -m 0755 "$SOURCE_RUNNER" "$STAGED_RUNNER"
/usr/bin/install -m 0600 "$SOURCE_ENV_FILE" "$STAGED_ENV_FILE"

load_service_environment() {
  environment_file=$1
  if [ ! -f "$environment_file" ]; then
    echo "authenticated service environment does not exist" >&2
    return 1
  fi
  if [ "$(/usr/bin/stat -f '%Lp' "$environment_file")" != "600" ]; then
    echo "authenticated service environment must have mode 600" >&2
    return 1
  fi
  unset VERMORY_DATABASE_URL VERMORY_LISTEN
  set -a
  # shellcheck disable=SC1090
  . "$environment_file"
  set +a
  : "${VERMORY_DATABASE_URL:?VERMORY_DATABASE_URL is required}"
  : "${VERMORY_LISTEN:?VERMORY_LISTEN is required}"
  case "$VERMORY_LISTEN" in
    127.0.0.1:[0-9]*|localhost:[0-9]*|'[::1]':[0-9]*) ;;
    *) echo "VERMORY_LISTEN must use a loopback address" >&2; return 1 ;;
  esac
}

load_service_environment "$STAGED_ENV_FILE"
if ! "$STAGED_BINARY" version >/dev/null 2>&1; then
  echo "candidate binary did not report a valid Vermory version" >&2
  exit 1
fi
if ! "$STAGED_BINARY" database compatibility --database-url "$VERMORY_DATABASE_URL" >/dev/null 2>&1; then
  echo "candidate database compatibility preflight failed" >&2
  exit 1
fi

current_count=0
for path in "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE"; do
  if [ -e "$path" ]; then
    current_count=$((current_count + 1))
  fi
done
if [ "$current_count" -ne 0 ] && [ "$current_count" -ne 3 ]; then
  echo "current authenticated service installation is incomplete" >&2
  exit 1
fi
if [ "$current_count" -eq 3 ]; then
  if [ ! -x "$INSTALL_BINARY" ] || [ ! -x "$INSTALL_RUNNER" ]; then
    echo "current authenticated service installation is not executable" >&2
    exit 1
  fi
  if [ "$(/usr/bin/stat -f '%Lp' "$INSTALL_ENV_FILE")" != "600" ]; then
    echo "current authenticated service environment must have mode 600" >&2
    exit 1
  fi
fi

had_previous=0
if [ "$current_count" -eq 3 ]; then
  rollback_new="$ROLLBACK_DIR.new.$$"
  /bin/rm -rf "$rollback_new"
  /usr/bin/install -d -m 0700 "$rollback_new"
  /usr/bin/install -m 0755 "$INSTALL_BINARY" "$rollback_new/vermory"
  /usr/bin/install -m 0755 "$INSTALL_RUNNER" "$rollback_new/run-authenticated-service.sh"
  /usr/bin/install -m 0600 "$INSTALL_ENV_FILE" "$rollback_new/vermory-authenticated.env"
  /bin/rm -rf "$ROLLBACK_DIR"
  /bin/mv "$rollback_new" "$ROLLBACK_DIR"
  had_previous=1
fi

install_staged_files() {
  /usr/bin/install -m 0755 "$STAGED_BINARY" "$INSTALL_BINARY.new"
  /usr/bin/install -m 0755 "$STAGED_RUNNER" "$INSTALL_RUNNER.new"
  /usr/bin/install -m 0600 "$STAGED_ENV_FILE" "$INSTALL_ENV_FILE.new"
  /bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"
  /bin/mv "$INSTALL_RUNNER.new" "$INSTALL_RUNNER"
  /bin/mv "$INSTALL_ENV_FILE.new" "$INSTALL_ENV_FILE"
}

restore_previous_installation() {
  if [ "$had_previous" -ne 1 ]; then
    return 1
  fi
  /usr/bin/install -m 0755 "$ROLLBACK_DIR/vermory" "$INSTALL_BINARY.new"
  /usr/bin/install -m 0755 "$ROLLBACK_DIR/run-authenticated-service.sh" "$INSTALL_RUNNER.new"
  /usr/bin/install -m 0600 "$ROLLBACK_DIR/vermory-authenticated.env" "$INSTALL_ENV_FILE.new"
  /bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"
  /bin/mv "$INSTALL_RUNNER.new" "$INSTALL_RUNNER"
  /bin/mv "$INSTALL_ENV_FILE.new" "$INSTALL_ENV_FILE"
}

install_staged_files

TEMP_PLIST="$STAGE_DIR/$LABEL.plist"
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
/usr/bin/install -m 0644 "$TEMP_PLIST" "$PLIST_PATH.new"
/bin/mv "$PLIST_PATH.new" "$PLIST_PATH"

start_service() {
  "$LAUNCHCTL" bootout "$USER_DOMAIN" "$PLIST_PATH" >/dev/null 2>&1 || true
  "$LAUNCHCTL" bootstrap "$USER_DOMAIN" "$PLIST_PATH" || return 1
  "$LAUNCHCTL" enable "$USER_DOMAIN/$LABEL" || return 1
  "$LAUNCHCTL" kickstart -k "$USER_DOMAIN/$LABEL" || return 1
}

wait_for_health() {
  load_service_environment "$INSTALL_ENV_FILE" || return 1
  ROOT_URL="http://$VERMORY_LISTEN/"
  SESSION_URL="http://$VERMORY_LISTEN/v1/session"
  attempt=0
  while [ "$attempt" -lt "$HEALTH_ATTEMPTS" ]; do
    root_code=$("$CURL" --noproxy '*' -s -o /dev/null -w '%{http_code}' --max-time 3 "$ROOT_URL" || true)
    session_code=$("$CURL" --noproxy '*' -s -o /dev/null -w '%{http_code}' --max-time 3 "$SESSION_URL" || true)
    if [ "$root_code" = "200" ] && [ "$session_code" = "401" ]; then
      return 0
    fi
    attempt=$((attempt + 1))
    "$SLEEP" "$HEALTH_SLEEP_SECONDS"
  done
  return 1
}

if start_service && wait_for_health; then
  VERSION=$("$INSTALL_BINARY" version 2>/dev/null | /usr/bin/tr '\n' ' ')
  echo "service=$LABEL state=running listen=$VERMORY_LISTEN root_code=200 session_code=401 version=$VERSION"
  exit 0
fi

if restore_previous_installation && start_service && wait_for_health; then
  echo "candidate activation failed; previous installation restored" >&2
  exit 1
fi

if [ "$had_previous" -eq 0 ]; then
  "$LAUNCHCTL" bootout "$USER_DOMAIN" "$PLIST_PATH" >/dev/null 2>&1 || true
  /bin/rm -f "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE" "$PLIST_PATH"
  echo "candidate activation failed; incomplete first installation removed" >&2
  exit 1
fi

echo "authenticated Vermory service did not become ready and automatic restoration failed" >&2
"$LAUNCHCTL" print "$USER_DOMAIN/$LABEL" >&2 || true
/usr/bin/tail -n 80 "$LOG_DIR/$LOG_BASENAME.stderr.log" >&2 || true
exit 1
