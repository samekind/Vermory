#!/bin/sh

set -eu

if [ "$#" -eq 0 ]; then
  echo "usage: $0 command [args...]" >&2
  exit 2
fi

KEYCHAIN_ACCOUNT=${VERMORY_KEYCHAIN_ACCOUNT:-$USER}
KEYCHAIN_SERVICE=${VERMORY_KEYCHAIN_SERVICE:-vermory-siliconflow}

SILICONFLOW_API_KEY=$(
  /usr/bin/security find-generic-password \
    -a "$KEYCHAIN_ACCOUNT" \
    -s "$KEYCHAIN_SERVICE" \
    -w
) || {
  echo "SiliconFlow credential is unavailable from the login Keychain" >&2
  exit 1
}
if [ -z "$SILICONFLOW_API_KEY" ]; then
  echo "SiliconFlow credential is empty" >&2
  exit 1
fi
export SILICONFLOW_API_KEY

exec "$@"
