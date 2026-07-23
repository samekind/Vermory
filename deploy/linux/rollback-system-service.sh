#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "vermory rollback: $*" >&2
  exit 1
}

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  fail "effective UID 0 is required"
fi
[[ $# -eq 0 ]] || {
  echo "usage: $0" >&2
  exit 2
}

INSTALL_ROOT=${VERMORY_INSTALL_ROOT:-/opt/vermory}
SERVICE_NAME=${VERMORY_SERVICE_NAME:-vermory.service}
HEALTH_URL=${VERMORY_HEALTH_URL:-http://127.0.0.1:8788/v1/defaults}
VERMORY_HEALTH_ATTEMPTS=${VERMORY_HEALTH_ATTEMPTS:-30}
HEALTH_INTERVAL=${VERMORY_HEALTH_INTERVAL:-1}

[[ -L "$INSTALL_ROOT/current" ]] || fail "current release is unavailable"
[[ -L "$INSTALL_ROOT/previous" ]] || fail "previous release is unavailable"
starting_release=$(readlink -f "$INSTALL_ROOT/current")
rollback_release=$(readlink -f "$INSTALL_ROOT/previous")
[[ -x "$starting_release/vermory" && -x "$rollback_release/vermory" ]] || fail "release pointers are invalid"

set_release_link() {
  local name=$1
  local target=$2
  local temporary="$INSTALL_ROOT/.$name.new.$$"
  ln -s "$target" "$temporary"
  mv -Tf "$temporary" "$INSTALL_ROOT/$name"
}

probe_service() {
  local attempt status
  for ((attempt = 1; attempt <= VERMORY_HEALTH_ATTEMPTS; attempt++)); do
    status=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$HEALTH_URL" 2>/dev/null || true)
    [[ "$status" == 401 ]] && return 0
    sleep "$HEALTH_INTERVAL"
  done
  return 1
}

set_release_link current "$rollback_release"
set_release_link previous "$starting_release"
systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
if systemctl restart "$SERVICE_NAME" && probe_service; then
  echo "activated rollback release $(basename "$rollback_release")"
  exit 0
fi

set_release_link current "$starting_release"
set_release_link previous "$rollback_release"
systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
systemctl restart "$SERVICE_NAME"
if probe_service; then
  echo "rollback restored starting release $(basename "$starting_release")" >&2
  exit 1
fi
fail "rollback target failed and starting release could not be recovered"
