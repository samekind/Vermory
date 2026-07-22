#!/usr/bin/env bash

set -euo pipefail

if [[ ${1:-1} == 0 ]] && command -v systemctl >/dev/null; then
  systemctl disable --now vermory.service >/dev/null 2>&1 || true
fi
