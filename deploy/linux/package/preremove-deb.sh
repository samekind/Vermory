#!/bin/sh

set -eu

if [ "${1:-}" = remove ] && command -v systemctl >/dev/null; then
  systemctl disable --now vermory.service >/dev/null 2>&1 || true
fi
