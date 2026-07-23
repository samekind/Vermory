#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I06 package acceptance: $*" >&2
  exit 1
}

[[ ${EUID:-$(id -u)} -eq 0 ]] || fail "effective UID 0 is required"
[[ $# -eq 5 ]] || fail "usage: $0 <repository-root> <dist> <deb|rpm> <amd64|arm64> <evidence-directory>"

REPOSITORY_ROOT=$1
DIST=$2
FORMAT=$3
ARCH=$4
EVIDENCE_DIRECTORY=$5
SOURCE_SHA=${SOURCE_SHA:-}
EXPECTED_MACHINE=${I06_EXPECTED_MACHINE:-}

[[ -d "$REPOSITORY_ROOT" && -d "$DIST" ]] || fail "repository and dist directories are required"
[[ "$FORMAT" == deb || "$FORMAT" == rpm ]] || fail "unsupported package format"
case "$ARCH:$EXPECTED_MACHINE" in
  amd64:x86_64)
    PACKAGE_ARCH=amd64
    RPM_ARCH=x86_64
    FILE_ARCH='x86-64'
    ;;
  arm64:aarch64)
    PACKAGE_ARCH=arm64
    RPM_ARCH=aarch64
    FILE_ARCH='ARM aarch64'
    ;;
  *) fail "unsupported architecture and machine pair" ;;
esac
[[ "$SOURCE_SHA" =~ ^[a-f0-9]{40}$ ]] || fail "SOURCE_SHA is required"
[[ $(uname -m) == "$EXPECTED_MACHINE" ]] || fail "runner machine does not match the qualification leg"
[[ $(go env GOARCH) == "$ARCH" && $(go env GOHOSTARCH) == "$ARCH" ]] || fail "Go runtime is not native"

shopt -s nullglob
packages=("$DIST"/vermory_*_linux_"$ARCH"."$FORMAT")
shopt -u nullglob
[[ ${#packages[@]} -eq 1 ]] || fail "expected exactly one package for the qualification leg"
PACKAGE=${packages[0]}
PACKAGE_SHA256=$(sha256sum "$PACKAGE" | awk '{print $1}')

[[ ! -e /usr/bin/vermory ]] || fail "vermory binary already exists"
[[ ! -e /usr/lib/systemd/system/vermory.service ]] || fail "vermory unit already exists"
[[ ! -e /etc/vermory/vermory.env ]] || fail "vermory environment already exists"
getent passwd vermory >/dev/null && fail "vermory service user already exists"

case "$FORMAT" in
  deb)
    [[ $(dpkg-deb --field "$PACKAGE" Architecture) == "$PACKAGE_ARCH" ]] || fail "DEB architecture mismatch"
    dpkg --install "$PACKAGE"
    ;;
  rpm)
    [[ $(rpm --query --package --queryformat '%{ARCH}' "$PACKAGE") == "$RPM_ARCH" ]] || fail "RPM architecture mismatch"
    rpm --install --nodeps "$PACKAGE"
    ;;
esac

[[ -x /usr/bin/vermory ]] || fail "package binary is missing"
/usr/bin/vermory --help >/dev/null
VERSION_JSON=$(/usr/bin/vermory version)
[[ $(jq -r '.revision' <<<"$VERSION_JSON") == "$SOURCE_SHA" ]] || fail "installed binary is not bound to SOURCE_SHA"
file /usr/bin/vermory | grep -Fq "$FILE_ARCH" || fail "installed binary architecture mismatch"

IDENTITY=$(getent passwd vermory)
IFS=: read -r _ _ SERVICE_UID SERVICE_GID _ SERVICE_HOME SERVICE_SHELL <<<"$IDENTITY"
GROUP_GID=$(getent group vermory | cut -d: -f3)
[[ "$SERVICE_UID" != 0 && "$SERVICE_GID" == "$GROUP_GID" ]] || fail "service identity is privileged or mis-grouped"
[[ "$SERVICE_HOME" == /nonexistent ]] || fail "service identity home is unsafe"
[[ "$SERVICE_SHELL" == /usr/sbin/nologin || "$SERVICE_SHELL" == /sbin/nologin ]] || fail "service identity can log in"

UNIT=/usr/lib/systemd/system/vermory.service
EXAMPLE=/usr/share/vermory/vermory.env.example
[[ -f "$UNIT" && -f "$EXAMPLE" ]] || fail "package service assets are missing"
grep -Fq 'ConditionPathExists=/etc/vermory/vermory.env' "$UNIT"
grep -Fq 'User=vermory' "$UNIT"
grep -Fq 'ExecStart=/usr/bin/vermory serve' "$UNIT"
grep -Fq 'NoNewPrivileges=true' "$UNIT"
[[ ! -e /etc/vermory/vermory.env ]] || fail "package created a protected environment"
if systemctl is-active --quiet vermory.service; then
  fail "package activated the service"
fi
if systemctl is-enabled --quiet vermory.service; then
  fail "package enabled the service"
fi

PACKAGE_SCRIPT_ROOT=$REPOSITORY_ROOT/deploy/linux/package
if grep -R -F 'sudo ' "$PACKAGE_SCRIPT_ROOT"/*.sh; then
  fail "package script invokes sudo"
fi
if grep -R -F 'database migrate' "$PACKAGE_SCRIPT_ROOT"/*.sh; then
  fail "package script runs a database migration"
fi
if grep -R -E '(postgresql://|API_KEY|PASSWORD=|PRIVATE KEY|sk-[A-Za-z0-9])' "$PACKAGE_SCRIPT_ROOT"/*.sh; then
  fail "package script contains credential material"
fi
grep -Fq "if [ \"\${1:-}\" = remove ]" "$PACKAGE_SCRIPT_ROOT/preremove-deb.sh"
grep -Fq "if [ \"\${1:-1}\" = 0 ]" "$PACKAGE_SCRIPT_ROOT/preremove-rpm.sh"

install -d -o root -g root -m 0755 /etc/vermory
printf '%s\n' 'operator-owned-i06' >/etc/vermory/vermory.env
chmod 0600 /etc/vermory/vermory.env

case "$FORMAT" in
  deb) dpkg --remove vermory ;;
  rpm) rpm --erase vermory ;;
esac

[[ ! -e /usr/bin/vermory ]] || fail "package removal retained the binary"
[[ ! -e "$UNIT" && ! -e "$EXAMPLE" ]] || fail "package removal retained package-owned service assets"
[[ -f /etc/vermory/vermory.env && $(cat /etc/vermory/vermory.env) == operator-owned-i06 ]] || fail "package removal changed operator configuration"
getent passwd vermory >/dev/null || fail "package removal deleted the service identity"

install -d -o root -g root -m 0700 "$EVIDENCE_DIRECTORY"
jq -n \
  --arg source_sha "$SOURCE_SHA" \
  --arg format "$FORMAT" \
  --arg architecture "$ARCH" \
  --arg machine "$EXPECTED_MACHINE" \
  --arg package "$(basename "$PACKAGE")" \
  --arg package_sha256 "$PACKAGE_SHA256" \
  '{
    version: "1",
    case_id: "I06-linux-native-packages",
    source_sha: $source_sha,
    package: {
      format: $format,
      architecture: $architecture,
      machine: $machine,
      file: $package,
      sha256: $package_sha256,
      native: true
    },
    hard_gates: {
      exact_source_head: true,
      expected_format: true,
      native_architecture: true,
      executable_binary: true,
      restricted_service_identity: true,
      hardened_systemd_unit: true,
      environment_example: true,
      protected_environment_absent: true,
      service_not_activated: true,
      scripts_without_sudo: true,
      scripts_without_migrations: true,
      scripts_without_credentials: true,
      true_removal_guard: true,
      package_owned_files_removed: true,
      operator_state_preserved: true,
      report_credential_free: true
    }
  }' >"$EVIDENCE_DIRECTORY/report.json"

if grep -E -i '(postgresql://|api[_-]?key|password|private key|sk-[a-z0-9])' "$EVIDENCE_DIRECTORY/report.json"; then
  fail "normalized report contains credential material"
fi
install -m 0644 "$PACKAGE" "$EVIDENCE_DIRECTORY/$(basename "$PACKAGE")"
chmod 0644 "$EVIDENCE_DIRECTORY/report.json"
chmod 0755 "$EVIDENCE_DIRECTORY"

find /etc/vermory -depth -delete
