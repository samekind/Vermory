#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "usage: $0 <libpq-service-name> <backup-directory> [backup-id]" >&2
  exit 2
}

fail() {
  echo "vermory backup: $*" >&2
  exit 1
}

[[ $# -ge 2 && $# -le 3 ]] || usage
PG_SERVICE=$1
BACKUP_DIRECTORY=$2
BACKUP_ID=${3:-$(date -u +%Y%m%dT%H%M%SZ)}
[[ "$PG_SERVICE" =~ ^[A-Za-z0-9_.-]+$ ]] || fail "libpq service name is invalid"
[[ "$BACKUP_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || fail "backup ID is invalid"

install -d -m 0700 "$BACKUP_DIRECTORY"
DUMP="$BACKUP_DIRECTORY/$BACKUP_ID.dump"
CHECKSUM="$DUMP.sha256"
METADATA="$BACKUP_DIRECTORY/$BACKUP_ID.json"
if [[ -e "$DUMP" || -e "$CHECKSUM" || -e "$METADATA" ]]; then
  fail "refusing to overwrite backup $BACKUP_ID"
fi

TEMP_DUMP=$(mktemp "$BACKUP_DIRECTORY/.$BACKUP_ID.dump.XXXXXX")
TEMP_CHECKSUM=$(mktemp "$BACKUP_DIRECTORY/.$BACKUP_ID.sha256.XXXXXX")
TEMP_METADATA=$(mktemp "$BACKUP_DIRECTORY/.$BACKUP_ID.json.XXXXXX")
cleanup() {
  rm -f "$TEMP_DUMP" "$TEMP_CHECKSUM" "$TEMP_METADATA"
}
trap cleanup EXIT

pg_dump --format=custom --no-owner --no-acl --file="$TEMP_DUMP" "service=$PG_SERVICE"
DIGEST=$(sha256sum "$TEMP_DUMP" | awk '{print $1}')
BYTES=$(stat -c '%s' "$TEMP_DUMP")
SCHEMA_VERSION=$(psql "service=$PG_SERVICE" -X -v ON_ERROR_STOP=1 -Atqc \
  "SELECT COALESCE(max(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version")
SERVER_VERSION=$(psql "service=$PG_SERVICE" -X -v ON_ERROR_STOP=1 -Atqc "SHOW server_version_num")
PG_DUMP_VERSION=$(LC_ALL=C pg_dump --version | sed -nE 's/^pg_dump \(PostgreSQL\) ([0-9]+(\.[0-9]+)*)( .*)?$/\1/p')
CREATED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
[[ "$SCHEMA_VERSION" =~ ^[0-9]+$ ]] || fail "schema_version is not numeric"
[[ "$SERVER_VERSION" =~ ^[0-9]+$ ]] || fail "PostgreSQL server version is not numeric"
[[ "$PG_DUMP_VERSION" =~ ^[0-9]+([.][0-9]+)*$ ]] || fail "pg_dump version is invalid"

printf '%s  %s\n' "$DIGEST" "$(basename "$DUMP")" >"$TEMP_CHECKSUM"
printf '{\n  "backup_id": "%s",\n  "bytes": %s,\n  "sha256": "%s",\n  "schema_version": %s,\n  "postgres_server_version_num": %s,\n  "pg_dump_version": "%s",\n  "created_at": "%s"\n}\n' \
  "$BACKUP_ID" "$BYTES" "$DIGEST" "$SCHEMA_VERSION" "$SERVER_VERSION" "$PG_DUMP_VERSION" "$CREATED_AT" >"$TEMP_METADATA"

chmod 0600 "$TEMP_DUMP" "$TEMP_CHECKSUM" "$TEMP_METADATA"
mv "$TEMP_CHECKSUM" "$CHECKSUM"
mv "$TEMP_METADATA" "$METADATA"
mv "$TEMP_DUMP" "$DUMP"
trap - EXIT
echo "$DUMP"
