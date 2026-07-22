#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "usage: $0 <repository-root> <evidence-directory>" >&2
  exit 2
}

fail() {
  echo "I05 acceptance: $*" >&2
  exit 1
}

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  fail "effective UID 0 is required"
fi
[[ $# -eq 2 ]] || usage
[[ -n "${SOURCE_SHA:-}" ]] || fail "SOURCE_SHA is required"
[[ "$SOURCE_SHA" =~ ^[a-f0-9]{40}$ ]] || fail "SOURCE_SHA is invalid"
[[ -n "${POSTGRES_PASSWORD:-}" ]] || fail "POSTGRES_PASSWORD is required"

REPOSITORY_ROOT=$(cd "$1" && pwd)
EVIDENCE_DIRECTORY=$2
[[ "$(git -C "$REPOSITORY_ROOT" rev-parse HEAD)" == "$SOURCE_SHA" ]] || fail "repository is not at SOURCE_SHA"

SERVICE_NAME=vermory-i05.service
SERVICE_USER=vermory-i05
SERVICE_GROUP=vermory-i05
INSTALL_ROOT=/opt/vermory-i05
CONFIG_ROOT=/etc/vermory-i05
ENVIRONMENT_FILE=$CONFIG_ROOT/vermory.env
SYSTEMD_UNIT=/etc/systemd/system/$SERVICE_NAME
LISTEN=127.0.0.1:18788
BASE_URL=http://$LISTEN
HEALTH_URL=$BASE_URL/v1/defaults
SOURCE_DATABASE=vermory_i05_source
RESTORE_DATABASE=vermory_i05_restore
SOURCE_RUNTIME_ROLE=vermory_i05_source_runtime
RESTORE_RUNTIME_ROLE=vermory_i05_restore_runtime
SOURCE_RUNTIME_PASSWORD=i05-source-runtime-password
RESTORE_RUNTIME_PASSWORD=i05-restore-runtime-password
TENANT_ID=i05-tenant
DEFAULT_MARKER=I05-GOVERNED-DEFAULT-7319
WORK_ROOT=$(mktemp -d /var/tmp/vermory-i05.XXXXXX)
ADMIN_SERVICE_FILE=$WORK_ROOT/admin.pg_service.conf
ADMIN_PASS_FILE=$WORK_ROOT/admin.pgpass
BUILD_ROOT=$WORK_ROOT/build
BACKUP_ROOT=$WORK_ROOT/backups
FAILED_UPGRADE_LOG=$WORK_ROOT/failed-upgrade.log
JOURNAL_LOG=$WORK_ROOT/service.journal
UNIT_DUMP=$WORK_ROOT/service.unit
PROCESS_ARGS=$WORK_ROOT/service.process

cleanup() {
  systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
  systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
  rm -f "$SYSTEMD_UNIT"
  systemctl daemon-reload >/dev/null 2>&1 || true
}
trap cleanup EXIT

install -d -m 0700 "$WORK_ROOT" "$BUILD_ROOT" "$BACKUP_ROOT" "$EVIDENCE_DIRECTORY"
install -d -o root -g root -m 0755 "$CONFIG_ROOT"

cat >"$ADMIN_SERVICE_FILE" <<EOF
[i05_cluster_admin]
host=127.0.0.1
port=5432
user=postgres
dbname=postgres
sslmode=disable

[i05_source_admin]
host=127.0.0.1
port=5432
user=postgres
dbname=$SOURCE_DATABASE
sslmode=disable

[i05_restore_admin]
host=127.0.0.1
port=5432
user=postgres
dbname=$RESTORE_DATABASE
sslmode=disable
EOF
printf '127.0.0.1:5432:*:postgres:%s\n' "$POSTGRES_PASSWORD" >"$ADMIN_PASS_FILE"
chmod 0600 "$ADMIN_SERVICE_FILE" "$ADMIN_PASS_FILE"
export PGSERVICEFILE=$ADMIN_SERVICE_FILE
export PGPASSFILE=$ADMIN_PASS_FILE

psql "service=i05_cluster_admin" -X -v ON_ERROR_STOP=1 \
  --set=source_password="$SOURCE_RUNTIME_PASSWORD" \
  --set=restore_password="$RESTORE_RUNTIME_PASSWORD" <<'SQL' >/dev/null
CREATE ROLE vermory_i05_source_runtime LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD :'source_password';
CREATE ROLE vermory_i05_restore_runtime LOGIN NOSUPERUSER NOBYPASSRLS PASSWORD :'restore_password';
SQL
createdb --maintenance-db="service=i05_cluster_admin" "$SOURCE_DATABASE"
createdb --maintenance-db="service=i05_cluster_admin" "$RESTORE_DATABASE"

build_release() {
  local release_id=$1
  local release_dir=$BUILD_ROOT/$release_id
  local archive=$BUILD_ROOT/$release_id.tar.gz
  install -d -m 0755 "$release_dir"
  (
    cd "$REPOSITORY_ROOT"
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X vermory/internal/brand.Version=$release_id -X vermory/internal/brand.Revision=$SOURCE_SHA" \
      -o "$release_dir/vermory" ./cmd/vermory
  )
  chmod 0755 "$release_dir/vermory"
  tar --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 \
    -czf "$archive" -C "$release_dir" vermory
  printf '%s\n' "$archive"
}

build_failing_release() {
  local release_id=$1
  local release_dir=$BUILD_ROOT/$release_id
  local archive=$BUILD_ROOT/$release_id.tar.gz
  install -d -m 0755 "$release_dir"
  printf '#!/bin/sh\nexit 42\n' >"$release_dir/vermory"
  chmod 0755 "$release_dir/vermory"
  tar --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 \
    -czf "$archive" -C "$release_dir" vermory
  printf '%s\n' "$archive"
}

archive_digest() {
  sha256sum "$1" | awk '{print $1}'
}

write_service_environment() {
  local database=$1
  local role=$2
  local password=$3
  cat >"$ENVIRONMENT_FILE" <<EOF
VERMORY_DATABASE_URL=postgresql://$role:$password@127.0.0.1:5432/$database?sslmode=disable
VERMORY_LISTEN=$LISTEN
VERMORY_PROVIDER=mock
EOF
  chown root:root "$ENVIRONMENT_FILE"
  chmod 0600 "$ENVIRONMENT_FILE"
}

install_release() {
  local archive=$1
  local release_id=$2
  local expected_sha256=${3:-$(archive_digest "$archive")}
  env \
    VERMORY_INSTALL_ROOT="$INSTALL_ROOT" \
    VERMORY_ENVIRONMENT_FILE="$ENVIRONMENT_FILE" \
    VERMORY_SYSTEMD_UNIT="$SYSTEMD_UNIT" \
    VERMORY_SERVICE_NAME="$SERVICE_NAME" \
    VERMORY_SERVICE_USER="$SERVICE_USER" \
    VERMORY_SERVICE_GROUP="$SERVICE_GROUP" \
    VERMORY_UNIT_TEMPLATE="$REPOSITORY_ROOT/deploy/linux/vermory.service" \
    VERMORY_HEALTH_URL="$HEALTH_URL" \
    VERMORY_HEALTH_ATTEMPTS=20 \
    VERMORY_HEALTH_INTERVAL=0.5 \
    "$REPOSITORY_ROOT/deploy/linux/install-system-service.sh" \
      "$archive" "$release_id" "$expected_sha256"
}

rollback_release() {
  env \
    VERMORY_INSTALL_ROOT="$INSTALL_ROOT" \
    VERMORY_SERVICE_NAME="$SERVICE_NAME" \
    VERMORY_HEALTH_URL="$HEALTH_URL" \
    VERMORY_HEALTH_ATTEMPTS=20 \
    VERMORY_HEALTH_INTERVAL=0.5 \
    "$REPOSITORY_ROOT/deploy/linux/rollback-system-service.sh"
}

http_status() {
  curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$1"
}

authenticated_defaults() {
  curl --fail --silent --show-error \
    --header "Authorization: Bearer $OPERATOR_TOKEN" \
    "$BASE_URL/v1/defaults"
}

assert_systemd_property() {
  local property=$1
  local expected=$2
  local actual
  actual=$(systemctl show "$SERVICE_NAME" --property="$property" --value)
  [[ "$actual" == "$expected" ]] || fail "systemd $property=$actual, want $expected"
}

assert_address_families() {
  local actual normalized
  actual=$(systemctl show "$SERVICE_NAME" --property=RestrictAddressFamilies --value)
  normalized=$(tr ' ' '\n' <<<"$actual" | sed '/^$/d' | LC_ALL=C sort | paste -sd ' ' -)
  [[ "$normalized" == "AF_INET AF_INET6 AF_UNIX" ]] || fail "unexpected systemd address families: $actual"
}

assert_restricted_role() {
  local admin_service=$1
  local role=$2
  local state
  state=$(psql "service=$admin_service" -X -v ON_ERROR_STOP=1 -At \
    --set=runtime_role="$role" -c \
    "SELECT rolcanlogin::int || ':' || rolsuper::int || ':' || rolbypassrls::int FROM pg_roles WHERE rolname = :'runtime_role'")
  [[ "$state" == "1:0:0" ]] || fail "runtime role $role is not restricted"
}

SOURCE_ADMIN_DSN='service=i05_source_admin'
RESTORE_ADMIN_DSN='service=i05_restore_admin'
V1_ARCHIVE=$(build_release i05-v1)
V2_ARCHIVE=$(build_release i05-v2)
FAILING_ARCHIVE=$(build_failing_release i05-failing)
V1_BINARY_SHA256=$(sha256sum "$BUILD_ROOT/i05-v1/vermory" | awk '{print $1}')
V2_BINARY_SHA256=$(sha256sum "$BUILD_ROOT/i05-v2/vermory" | awk '{print $1}')
[[ "$V1_BINARY_SHA256" != "$V2_BINARY_SHA256" ]] || fail "release binaries are not distinct"

"$BUILD_ROOT/i05-v1/vermory" database migrate --database-url "$SOURCE_ADMIN_DSN" >/dev/null
"$BUILD_ROOT/i05-v1/vermory" database grant-runtime \
  --database-url "$SOURCE_ADMIN_DSN" --role "$SOURCE_RUNTIME_ROLE" >/dev/null
assert_restricted_role i05_source_admin "$SOURCE_RUNTIME_ROLE"
write_service_environment "$SOURCE_DATABASE" "$SOURCE_RUNTIME_ROLE" "$SOURCE_RUNTIME_PASSWORD"

if install_release "$V1_ARCHIVE" i05-v1 "$(printf '0%.0s' {1..64})" >/dev/null 2>&1; then
  fail "installer accepted an incorrect release digest"
fi
[[ ! -e "$INSTALL_ROOT/releases/i05-v1" ]] || fail "digest rejection installed release bytes"
install_release "$V1_ARCHIVE" i05-v1 >/dev/null
[[ "$(http_status "$HEALTH_URL")" == 401 ]] || fail "initial unauthenticated boundary is not 401"
assert_systemd_property User "$SERVICE_USER"
assert_systemd_property Group "$SERVICE_GROUP"
assert_systemd_property NoNewPrivileges yes
assert_systemd_property PrivateTmp yes
assert_systemd_property ProtectSystem strict
assert_systemd_property ProtectHome yes
assert_systemd_property RestrictSUIDSGID yes
assert_address_families
MAIN_PID=$(systemctl show "$SERVICE_NAME" --property=MainPID --value)
[[ "$MAIN_PID" =~ ^[1-9][0-9]*$ ]] || fail "service has no main process"
[[ "$(ps -o user= -p "$MAIN_PID" | xargs)" == "$SERVICE_USER" ]] || fail "process user mismatch"
[[ "$(stat -c '%u:%a' "$INSTALL_ROOT/releases/i05-v1")" == "0:755" ]] || fail "release directory is not root-owned and read-only to service"
[[ "$(stat -c '%u:%a' "$INSTALL_ROOT/releases/i05-v1/vermory")" == "0:755" ]] || fail "release binary is not root-owned and read-only to service"
[[ "$(stat -c '%u:%a' "$ENVIRONMENT_FILE")" == "0:600" ]] || fail "environment file is not root-only"
grep -Fxq "$LISTEN" <<<"$(ss -ltn | awk '{print $4}')" || fail "service is not listening only on the configured loopback address"

EXPIRES_AT=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
TOKEN_JSON=$("$BUILD_ROOT/i05-v1/vermory" identity token issue \
  --database-url "$SOURCE_ADMIN_DSN" \
  --operation-id i05-token-issue \
  --tenant-id "$TENANT_ID" \
  --subject-id i05-operator \
  --role operator \
  --expires-at "$EXPIRES_AT")
OPERATOR_TOKEN=$(jq -er '.token' <<<"$TOKEN_JSON")
[[ -n "$OPERATOR_TOKEN" ]] || fail "operator token was not issued"

curl --fail --silent --show-error \
  --header "Authorization: Bearer $OPERATOR_TOKEN" \
  --header 'Content-Type: application/json' \
  --data "{\"operation_id\":\"i05-default-set\",\"key\":\"deployment_marker\",\"content\":\"$DEFAULT_MARKER\"}" \
  "$BASE_URL/v1/defaults/set" >/dev/null
[[ "$(authenticated_defaults)" == *"$DEFAULT_MARKER"* ]] || fail "initial governed default is unavailable"

install_release "$V2_ARCHIVE" i05-v2 >/dev/null
[[ "$(basename "$(readlink -f "$INSTALL_ROOT/current")")" == i05-v2 ]] || fail "successful upgrade did not activate v2"
[[ "$(authenticated_defaults)" == *"$DEFAULT_MARKER"* ]] || fail "upgrade lost governed state"

if install_release "$FAILING_ARCHIVE" i05-failing >"$FAILED_UPGRADE_LOG" 2>&1; then
  fail "failing release was activated"
fi
grep -Fq "automatic rollback restored i05-v2" "$FAILED_UPGRADE_LOG" || fail "automatic rollback evidence is missing"
[[ "$(basename "$(readlink -f "$INSTALL_ROOT/current")")" == i05-v2 ]] || fail "failed upgrade changed current release"
[[ "$(systemctl is-active "$SERVICE_NAME")" == active ]] || fail "service did not recover after failed upgrade"

rollback_release >/dev/null
[[ "$(basename "$(readlink -f "$INSTALL_ROOT/current")")" == i05-v1 ]] || fail "explicit rollback did not activate v1"
rollback_release >/dev/null
[[ "$(basename "$(readlink -f "$INSTALL_ROOT/current")")" == i05-v2 ]] || fail "second rollback did not return to v2"

BACKUP_PATH=$("$REPOSITORY_ROOT/deploy/linux/backup-postgresql.sh" i05_source_admin "$BACKUP_ROOT" i05-source)
[[ -f "$BACKUP_PATH" && -f "$BACKUP_PATH.sha256" && -f "$BACKUP_ROOT/i05-source.json" ]] || fail "backup artifacts are incomplete"
[[ "$(stat -c '%a' "$BACKUP_PATH")" == 600 ]] || fail "backup dump is not private"
[[ "$(stat -c '%a' "$BACKUP_PATH.sha256")" == 600 ]] || fail "backup checksum is not private"
[[ "$(stat -c '%a' "$BACKUP_ROOT/i05-source.json")" == 600 ]] || fail "backup metadata is not private"
if "$REPOSITORY_ROOT/deploy/linux/backup-postgresql.sh" i05_source_admin "$BACKUP_ROOT" i05-source >/dev/null 2>&1; then
  fail "backup overwrote an existing backup ID"
fi

TAMPERED_BACKUP=$BACKUP_ROOT/i05-tampered.dump
cp "$BACKUP_PATH" "$TAMPERED_BACKUP"
printf 'tampered\n' >>"$TAMPERED_BACKUP"
sed "s/i05-source.dump/i05-tampered.dump/" "$BACKUP_PATH.sha256" >"$TAMPERED_BACKUP.sha256"
if "$REPOSITORY_ROOT/deploy/linux/restore-postgresql.sh" \
  "$TAMPERED_BACKUP" i05_restore_admin "$RESTORE_RUNTIME_ROLE" "$BUILD_ROOT/i05-v2/vermory" >/dev/null 2>&1; then
  fail "tampered backup passed checksum verification"
fi

psql "$RESTORE_ADMIN_DSN" -X -v ON_ERROR_STOP=1 -c 'CREATE TABLE i05_nonempty_probe (id integer)' >/dev/null
if "$REPOSITORY_ROOT/deploy/linux/restore-postgresql.sh" \
  "$BACKUP_PATH" i05_restore_admin "$RESTORE_RUNTIME_ROLE" "$BUILD_ROOT/i05-v2/vermory" >/dev/null 2>&1; then
  fail "restore accepted a non-empty target"
fi
psql "$RESTORE_ADMIN_DSN" -X -v ON_ERROR_STOP=1 -c 'DROP TABLE i05_nonempty_probe' >/dev/null

"$REPOSITORY_ROOT/deploy/linux/restore-postgresql.sh" \
  "$BACKUP_PATH" i05_restore_admin "$RESTORE_RUNTIME_ROLE" "$BUILD_ROOT/i05-v2/vermory" >/dev/null
assert_restricted_role i05_restore_admin "$RESTORE_RUNTIME_ROLE"
write_service_environment "$RESTORE_DATABASE" "$RESTORE_RUNTIME_ROLE" "$RESTORE_RUNTIME_PASSWORD"
systemctl restart "$SERVICE_NAME"
for _ in $(seq 1 20); do
  [[ "$(http_status "$HEALTH_URL" 2>/dev/null || true)" == 401 ]] && break
  sleep 0.5
done
[[ "$(http_status "$HEALTH_URL")" == 401 ]] || fail "restored service did not recover its authentication boundary"
[[ "$(authenticated_defaults)" == *"$DEFAULT_MARKER"* ]] || fail "restored token or governed default probe failed"

systemctl cat "$SERVICE_NAME" >"$UNIT_DUMP"
ps -o args= -p "$(systemctl show "$SERVICE_NAME" --property=MainPID --value)" >"$PROCESS_ARGS"
journalctl --unit "$SERVICE_NAME" --no-pager >"$JOURNAL_LOG"
for secret in "$POSTGRES_PASSWORD" "$SOURCE_RUNTIME_PASSWORD" "$RESTORE_RUNTIME_PASSWORD" "$OPERATOR_TOKEN"; do
  for inspected in "$UNIT_DUMP" "$PROCESS_ARGS" "$JOURNAL_LOG"; do
    if grep -Fq "$secret" "$inspected"; then
      fail "credential material appeared in service evidence"
    fi
  done
done
if grep -Fq 'postgresql://' "$UNIT_DUMP" || grep -Fq 'postgresql://' "$PROCESS_ARGS"; then
  fail "database URL appeared in unit or process arguments"
fi

FAILED_UPGRADE_SHA256=$(sha256sum "$FAILED_UPGRADE_LOG" | awk '{print $1}')
BACKUP_SHA256=$(jq -er '.sha256' "$BACKUP_ROOT/i05-source.json")
BACKUP_BYTES=$(jq -er '.bytes' "$BACKUP_ROOT/i05-source.json")
SCHEMA_VERSION=$(jq -er '.schema_version' "$BACKUP_ROOT/i05-source.json")
UNIT_PROPERTIES=$(systemctl show "$SERVICE_NAME" \
  --property=User,Group,NoNewPrivileges,PrivateTmp,ProtectSystem,ProtectHome,RestrictSUIDSGID,RestrictAddressFamilies \
  | LC_ALL=C sort | jq -Rn '[inputs | select(length > 0) | split("=") | {(.[0]): (.[1:] | join("="))}] | add')

jq -n \
  --arg source_sha "$SOURCE_SHA" \
  --arg service "$SERVICE_NAME" \
  --arg service_user "$SERVICE_USER" \
  --arg v1_binary_sha256 "$V1_BINARY_SHA256" \
  --arg v2_binary_sha256 "$V2_BINARY_SHA256" \
  --arg failed_release_sha256 "$(archive_digest "$FAILING_ARCHIVE")" \
  --arg failed_upgrade_log_sha256 "$FAILED_UPGRADE_SHA256" \
  --arg backup_sha256 "$BACKUP_SHA256" \
  --argjson backup_bytes "$BACKUP_BYTES" \
  --argjson schema_version "$SCHEMA_VERSION" \
  --argjson unit_properties "$UNIT_PROPERTIES" \
  '{
    version: 1,
    case_id: "I05-durable-linux-service-lifecycle",
    source_sha: $source_sha,
    qualification: "github-hosted-ubuntu-systemd-amd64",
    service: {name: $service, user: $service_user, listen: "127.0.0.1:18788", unauthenticated_status: 401, properties: $unit_properties},
    releases: {
      initial: {id: "i05-v1", binary_sha256: $v1_binary_sha256},
      upgraded: {id: "i05-v2", binary_sha256: $v2_binary_sha256},
      rejected: {id: "i05-failing", archive_sha256: $failed_release_sha256, log_sha256: $failed_upgrade_log_sha256},
      final_active: "i05-v2"
    },
    backup: {format: "postgresql-custom", bytes: $backup_bytes, sha256: $backup_sha256, schema_version: $schema_version, encrypted_by_format: false},
    restore: {target_started_empty: true, runtime_role_reprovisioned: true, projections_rebuilt: true, restored_token_authenticated: true, governed_default_visible: true},
    hard_gates: {
      exact_head: true,
      dedicated_service_identity: true,
      restricted_runtime_database_role: true,
      root_owned_versioned_releases: true,
      authenticated_loopback_boundary: true,
      successful_upgrade_preserved_state: true,
      failed_upgrade_rolled_back: true,
      explicit_rollback_bidirectional: true,
      native_private_backup: true,
      digest_mismatch_rejected: true,
      nonempty_target_rejected: true,
      empty_target_restored: true,
      runtime_role_reprovisioned: true,
      projections_rebuilt: true,
      restored_token_and_default_verified: true,
      unit_process_journal_secret_free: true
    },
    limitations: [
      "The GitHub-hosted Ubuntu runner is ephemeral and does not prove long-duration uptime or an SLA.",
      "This run qualifies Linux AMD64 systemd, not native Linux ARM64 or distribution packages.",
      "PostgreSQL custom format is not encrypted by the dump format.",
      "Binary pointer rollback does not reverse PostgreSQL migrations."
    ]
  }' >"$EVIDENCE_DIRECTORY/report.json"

chmod 0644 "$EVIDENCE_DIRECTORY/report.json"
for secret in "$POSTGRES_PASSWORD" "$SOURCE_RUNTIME_PASSWORD" "$RESTORE_RUNTIME_PASSWORD" "$OPERATOR_TOKEN"; do
  grep -Fq "$secret" "$EVIDENCE_DIRECTORY/report.json" && fail "credential material appeared in normalized report"
done
jq -e '.hard_gates | to_entries | all(.value == true)' "$EVIDENCE_DIRECTORY/report.json" >/dev/null
echo "I05 acceptance passed for $SOURCE_SHA"
