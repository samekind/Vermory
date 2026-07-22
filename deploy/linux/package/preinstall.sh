#!/usr/bin/env bash

set -euo pipefail

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "vermory package: effective UID 0 is required" >&2
  exit 1
fi

if ! getent group vermory >/dev/null; then
  groupadd --system vermory
fi

if ! getent passwd vermory >/dev/null; then
  useradd \
    --system \
    --gid vermory \
    --home-dir /nonexistent \
    --no-create-home \
    --shell /usr/sbin/nologin \
    vermory
fi

entry=$(getent passwd vermory)
IFS=: read -r _ _ uid gid _ home shell <<<"$entry"
group_gid=$(getent group vermory | cut -d: -f3)
[[ "$uid" != 0 && "$gid" == "$group_gid" ]] || {
  echo "vermory package: existing service identity is unsafe" >&2
  exit 1
}
[[ "$home" == /nonexistent ]] || {
  echo "vermory package: existing service home is unsafe" >&2
  exit 1
}
[[ "$shell" == /usr/sbin/nologin || "$shell" == /sbin/nologin ]] || {
  echo "vermory package: existing service shell is unsafe" >&2
  exit 1
}
