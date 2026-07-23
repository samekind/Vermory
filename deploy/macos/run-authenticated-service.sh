#!/bin/sh

set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 /path/to/vermory-authenticated.env /path/to/vermory" >&2
  exit 2
fi

ENV_FILE=$1
BINARY=$2

case "$ENV_FILE" in
  "$HOME"/*) ;;
  *) echo "authenticated service environment must be inside HOME" >&2; exit 2 ;;
esac
case "$BINARY" in
  "$HOME"/*) ;;
  *) echo "authenticated service binary must be inside HOME" >&2; exit 2 ;;
esac

if [ ! -f "$ENV_FILE" ]; then
  echo "authenticated service environment does not exist" >&2
  exit 2
fi
if [ "$(/usr/bin/stat -f '%Lp' "$ENV_FILE")" != "600" ]; then
  echo "authenticated service environment must have mode 600" >&2
  exit 2
fi
if [ ! -x "$BINARY" ]; then
  echo "authenticated service binary is not executable" >&2
  exit 2
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

exec "$BINARY" serve
