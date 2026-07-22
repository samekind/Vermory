#!/bin/sh

set -eu

if [ "$(id -u)" -ne 0 ]; then
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
uid=$(printf '%s\n' "$entry" | cut -d: -f3)
gid=$(printf '%s\n' "$entry" | cut -d: -f4)
home=$(printf '%s\n' "$entry" | cut -d: -f6)
shell=$(printf '%s\n' "$entry" | cut -d: -f7)
group_gid=$(getent group vermory | cut -d: -f3)
[ "$uid" != 0 ] && [ "$gid" = "$group_gid" ] || {
  echo "vermory package: existing service identity is unsafe" >&2
  exit 1
}
[ "$home" = /nonexistent ] || {
  echo "vermory package: existing service home is unsafe" >&2
  exit 1
}
case "$shell" in
  /usr/sbin/nologin | /sbin/nologin) ;;
  *)
    echo "vermory package: existing service shell is unsafe" >&2
    exit 1
    ;;
esac
