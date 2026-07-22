#!/bin/sh

set -eu

if command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  systemctl daemon-reload
fi
