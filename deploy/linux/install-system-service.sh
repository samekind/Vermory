#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "usage: $0 <release.tar.gz> <release-id> <expected-sha256>" >&2
  exit 2
}

fail() {
  echo "vermory install: $*" >&2
  exit 1
}

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  fail "effective UID 0 is required"
fi
[[ $# -eq 3 ]] || usage

ARCHIVE=$1
RELEASE_ID=$2
EXPECTED_SHA256=${3,,}
INSTALL_ROOT=${VERMORY_INSTALL_ROOT:-/opt/vermory}
ENVIRONMENT_FILE=${VERMORY_ENVIRONMENT_FILE:-/etc/vermory/vermory.env}
SYSTEMD_UNIT=${VERMORY_SYSTEMD_UNIT:-/etc/systemd/system/vermory.service}
SERVICE_NAME=${VERMORY_SERVICE_NAME:-vermory.service}
SERVICE_USER=${VERMORY_SERVICE_USER:-vermory}
SERVICE_GROUP=${VERMORY_SERVICE_GROUP:-vermory}
UNIT_TEMPLATE=${VERMORY_UNIT_TEMPLATE:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/vermory.service}
HEALTH_URL=${VERMORY_HEALTH_URL:-http://127.0.0.1:8788/v1/defaults}
VERMORY_HEALTH_ATTEMPTS=${VERMORY_HEALTH_ATTEMPTS:-30}
HEALTH_INTERVAL=${VERMORY_HEALTH_INTERVAL:-1}

[[ -f "$ARCHIVE" ]] || fail "release archive does not exist"
[[ "$RELEASE_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || fail "release ID is invalid"
[[ "$EXPECTED_SHA256" =~ ^[a-f0-9]{64}$ ]] || fail "expected SHA-256 is invalid"
[[ "$SERVICE_USER" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,31}$ ]] || fail "service user is invalid"
[[ "$SERVICE_GROUP" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,31}$ ]] || fail "service group is invalid"
[[ "$SERVICE_NAME" =~ ^[A-Za-z0-9_.@-]+\.service$ ]] || fail "service name is invalid"
[[ "$VERMORY_HEALTH_ATTEMPTS" =~ ^[1-9][0-9]*$ ]] || fail "VERMORY_HEALTH_ATTEMPTS is invalid"
[[ "$HEALTH_INTERVAL" =~ ^[0-9]+([.][0-9]+)?$ ]] || fail "HEALTH_INTERVAL is invalid"
for path in "$INSTALL_ROOT" "$ENVIRONMENT_FILE" "$SYSTEMD_UNIT"; do
  [[ "$path" =~ ^/[A-Za-z0-9._/-]+$ ]] || fail "deployment path is unsafe"
  [[ "$path" != *"/../"* && "$path" != */.. && "$path" != *"/./"* && "$path" != */. ]] || fail "deployment path is unsafe"
done
[[ -f "$UNIT_TEMPLATE" ]] || fail "systemd unit template does not exist"
[[ -f "$ENVIRONMENT_FILE" && ! -L "$ENVIRONMENT_FILE" ]] || fail "protected environment file does not exist"
[[ $(stat -c '%u' "$ENVIRONMENT_FILE") == 0 ]] || fail "environment file must be owned by root"
[[ $(stat -c '%a' "$ENVIRONMENT_FILE") == 600 ]] || fail "environment file mode must be 0600"

ACTUAL_SHA256=$(sha256sum "$ARCHIVE" | awk '{print $1}')
[[ "$ACTUAL_SHA256" == "$EXPECTED_SHA256" ]] || fail "release archive SHA-256 mismatch"

while IFS= read -r member; do
  [[ -n "$member" ]] || fail "archive contains an empty path"
  [[ "$member" != /* ]] || fail "archive contains an unsafe path"
  IFS=/ read -r -a segments <<<"$member"
  for segment in "${segments[@]}"; do
    [[ "$segment" != ".." ]] || fail "archive contains an unsafe path"
  done
done < <(tar -tzf "$ARCHIVE")

PAYLOAD_COUNT=$(tar -tzf "$ARCHIVE" | awk '$0 == "vermory" { count++ } END { print count + 0 }')
[[ "$PAYLOAD_COUNT" == 1 ]] || fail "archive must contain exactly one root vermory payload"
if tar -tvzf "$ARCHIVE" | awk '$1 !~ /^[-d]/ { found=1 } END { exit found ? 0 : 1 }'; then
  fail "archive contains a link or special file"
fi

if ! getent group "$SERVICE_GROUP" >/dev/null; then
  groupadd --system "$SERVICE_GROUP"
fi
if ! getent passwd "$SERVICE_USER" >/dev/null; then
  useradd --system --gid "$SERVICE_GROUP" --home-dir "$INSTALL_ROOT" --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
fi

install -d -o root -g root -m 0755 "$INSTALL_ROOT" "$INSTALL_ROOT/releases"
TARGET="$INSTALL_ROOT/releases/$RELEASE_ID"
STAGING="$INSTALL_ROOT/releases/.$RELEASE_ID.new.$$"
# Called through EXIT traps before and after unit rendering.
# shellcheck disable=SC2329
rm_staging() {
  if [[ -n "${STAGING:-}" && -d "$STAGING" ]]; then
    find "$STAGING" -depth -delete
  fi
}
trap rm_staging EXIT

if [[ -e "$TARGET" ]]; then
  [[ -d "$TARGET" && ! -L "$TARGET" ]] || fail "existing release target is unsafe"
  [[ -f "$TARGET/.archive.sha256" ]] || fail "existing release has no archive digest"
  [[ $(cat "$TARGET/.archive.sha256") == "$EXPECTED_SHA256" ]] || fail "refusing different payload under existing release ID"
else
  install -d -o root -g root -m 0755 "$STAGING"
  tar -xzf "$ARCHIVE" --no-same-owner --no-same-permissions -C "$STAGING"
  [[ -f "$STAGING/vermory" && ! -L "$STAGING/vermory" && -x "$STAGING/vermory" ]] || fail "archive root vermory payload is not executable"
  chown -R root:root "$STAGING"
  find "$STAGING" -type d -exec chmod 0755 {} +
  find "$STAGING" -type f ! -name vermory -exec chmod 0644 {} +
  chmod 0755 "$STAGING/vermory"
  printf '%s\n' "$EXPECTED_SHA256" >"$STAGING/.archive.sha256"
  chmod 0644 "$STAGING/.archive.sha256"
  mv "$STAGING" "$TARGET"
fi

rendered_unit=$(mktemp "${SYSTEMD_UNIT}.new.XXXXXX")
cleanup_unit() {
  rm -f "$rendered_unit"
}
trap 'rm_staging; cleanup_unit' EXIT
sed \
  -e "s|@SERVICE_USER@|$SERVICE_USER|g" \
  -e "s|@SERVICE_GROUP@|$SERVICE_GROUP|g" \
  -e "s|@ENVIRONMENT_FILE@|$ENVIRONMENT_FILE|g" \
  -e "s|@INSTALL_ROOT@|$INSTALL_ROOT|g" \
  "$UNIT_TEMPLATE" >"$rendered_unit"
install -o root -g root -m 0644 "$rendered_unit" "$SYSTEMD_UNIT"
cleanup_unit
rendered_unit=""

current_target=""
previous_target=""
[[ ! -L "$INSTALL_ROOT/current" ]] || current_target=$(readlink -f "$INSTALL_ROOT/current")
[[ ! -L "$INSTALL_ROOT/previous" ]] || previous_target=$(readlink -f "$INSTALL_ROOT/previous")

set_release_link() {
  local name=$1
  local target=$2
  local temporary="$INSTALL_ROOT/.$name.new.$$"
  ln -s "$target" "$temporary"
  mv -Tf "$temporary" "$INSTALL_ROOT/$name"
}

restore_previous_link() {
  if [[ -n "$previous_target" ]]; then
    set_release_link previous "$previous_target"
  else
    rm -f "$INSTALL_ROOT/previous"
  fi
}

probe_service() {
  local attempt status
  for ((attempt = 1; attempt <= VERMORY_HEALTH_ATTEMPTS; attempt++)); do
    status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$HEALTH_URL" 2>/dev/null || true)
    if [[ "$status" == 401 ]]; then
      return 0
    fi
    sleep "$HEALTH_INTERVAL"
  done
  return 1
}

if [[ -n "$current_target" && "$current_target" != "$TARGET" ]]; then
  set_release_link previous "$current_target"
fi
set_release_link current "$TARGET"

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
if systemctl restart "$SERVICE_NAME" && probe_service; then
  trap - EXIT
  echo "activated release $RELEASE_ID"
  exit 0
fi

if [[ -n "$current_target" ]]; then
  set_release_link current "$current_target"
  restore_previous_link
  systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
  systemctl restart "$SERVICE_NAME"
  if probe_service; then
    echo "automatic rollback restored $(basename "$current_target")" >&2
    exit 1
  fi
  fail "new release failed and automatic rollback did not recover the service"
fi

rm -f "$INSTALL_ROOT/current"
systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
fail "initial release failed its authenticated-boundary probe"
