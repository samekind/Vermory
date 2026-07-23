#!/bin/sh

set -eu

if command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  systemctl daemon-reload
fi

echo "Vermory is installed but not enabled. Configure /etc/vermory/vermory.env and the database before activation."
