#!/bin/sh

set -eu
umask 077

if [ "$#" -ne 0 ]; then
  echo "usage: $0" >&2
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

current_count=0
for path in "$INSTALL_BINARY" "$INSTALL_RUNNER" "$INSTALL_ENV_FILE"; do
  if [ -e "$path" ]; then
    current_count=$((current_count + 1))
  fi
done
if [ "$current_count" -ne 3 ]; then
  echo "current authenticated service installation is incomplete" >&2
  exit 1
fi

rollback_count=0
for path in "$ROLLBACK_DIR/vermory" "$ROLLBACK_DIR/run-authenticated-service.sh" "$ROLLBACK_DIR/vermory-authenticated.env"; do
  if [ -e "$path" ]; then
    rollback_count=$((rollback_count + 1))
  fi
done
if [ "$rollback_count" -ne 3 ]; then
  echo "rollback slot is incomplete" >&2
  exit 1
fi
if [ ! -f "$PLIST_PATH" ]; then
  echo "authenticated service LaunchAgent is not installed" >&2
  exit 1
fi
if [ "$(/usr/bin/stat -f '%Lp' "$ROLLBACK_DIR/vermory-authenticated.env")" != "600" ]; then
  echo "rollback environment must have mode 600" >&2
  exit 1
fi

unset VERMORY_DATABASE_URL VERMORY_LISTEN
set -a
# shellcheck disable=SC1091
. "$ROLLBACK_DIR/vermory-authenticated.env"
set +a
: "${VERMORY_DATABASE_URL:?VERMORY_DATABASE_URL is required}"
: "${VERMORY_LISTEN:?VERMORY_LISTEN is required}"
case "$VERMORY_LISTEN" in
  127.0.0.1:[0-9]*|localhost:[0-9]*|'[::1]':[0-9]*) ;;
  *) echo "VERMORY_LISTEN must use a loopback address" >&2; exit 1 ;;
esac

if ! "$ROLLBACK_DIR/vermory" version >/dev/null 2>&1; then
  echo "rollback binary did not report a valid Vermory version" >&2
  exit 1
fi
if ! "$ROLLBACK_DIR/vermory" database compatibility --database-url "$VERMORY_DATABASE_URL" >/dev/null 2>&1; then
  echo "rollback database compatibility preflight failed" >&2
  exit 1
fi

RECOVERY_DIR="$APP_DIR/rollback-recovery.$$"
/usr/bin/install -d -m 0700 "$RECOVERY_DIR"
# Invoked by the trap below.
# shellcheck disable=SC2329
cleanup() {
  /bin/rm -rf "$RECOVERY_DIR"
}
trap cleanup EXIT HUP INT TERM
/usr/bin/install -m 0755 "$INSTALL_BINARY" "$RECOVERY_DIR/vermory"
/usr/bin/install -m 0755 "$INSTALL_RUNNER" "$RECOVERY_DIR/run-authenticated-service.sh"
/usr/bin/install -m 0600 "$INSTALL_ENV_FILE" "$RECOVERY_DIR/vermory-authenticated.env"

restore_files_from() {
  source_dir=$1
  /usr/bin/install -m 0755 "$source_dir/vermory" "$INSTALL_BINARY.new"
  /usr/bin/install -m 0755 "$source_dir/run-authenticated-service.sh" "$INSTALL_RUNNER.new"
  /usr/bin/install -m 0600 "$source_dir/vermory-authenticated.env" "$INSTALL_ENV_FILE.new"
  /bin/mv "$INSTALL_BINARY.new" "$INSTALL_BINARY"
  /bin/mv "$INSTALL_RUNNER.new" "$INSTALL_RUNNER"
  /bin/mv "$INSTALL_ENV_FILE.new" "$INSTALL_ENV_FILE"
}

start_service() {
  "$LAUNCHCTL" bootout "$USER_DOMAIN" "$PLIST_PATH" >/dev/null 2>&1 || true
  "$LAUNCHCTL" bootstrap "$USER_DOMAIN" "$PLIST_PATH" || return 1
  "$LAUNCHCTL" enable "$USER_DOMAIN/$LABEL" || return 1
  "$LAUNCHCTL" kickstart -k "$USER_DOMAIN/$LABEL" || return 1
}

wait_for_health() {
  unset VERMORY_DATABASE_URL VERMORY_LISTEN
  set -a
  # shellcheck disable=SC1090
  . "$INSTALL_ENV_FILE"
  set +a
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

restore_files_from "$ROLLBACK_DIR"
if start_service && wait_for_health; then
  /bin/rm -rf "$ROLLBACK_DIR"
  VERSION=$("$INSTALL_BINARY" version 2>/dev/null | /usr/bin/tr '\n' ' ')
  echo "service=$LABEL state=rolled-back listen=$VERMORY_LISTEN root_code=200 session_code=401 version=$VERSION"
  exit 0
fi

restore_files_from "$RECOVERY_DIR"
if start_service && wait_for_health; then
  echo "rollback activation failed; current installation restored" >&2
  exit 1
fi

echo "rollback activation failed and current installation could not be restored" >&2
"$LAUNCHCTL" print "$USER_DOMAIN/$LABEL" >&2 || true
/usr/bin/tail -n 80 "$LOG_DIR/$LOG_BASENAME.stderr.log" >&2 || true
exit 1
