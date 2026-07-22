#!/usr/bin/env bash

set -euo pipefail
umask 077

usage() {
  echo "usage: $0 <backup.dump> <target-libpq-service-name> <runtime-role> <vermory-binary>" >&2
  exit 2
}

fail() {
  echo "vermory restore: $*" >&2
  exit 1
}

[[ $# -eq 4 ]] || usage
BACKUP=$1
TARGET_PG_SERVICE=$2
RUNTIME_ROLE=$3
VERMORY_BINARY=$4
CHECKSUM="$BACKUP.sha256"

[[ -f "$BACKUP" && ! -L "$BACKUP" ]] || fail "backup does not exist or is unsafe"
[[ -f "$CHECKSUM" && ! -L "$CHECKSUM" ]] || fail "backup checksum sidecar is missing or unsafe"
[[ "$TARGET_PG_SERVICE" =~ ^[A-Za-z0-9_.-]+$ ]] || fail "target libpq service name is invalid"
[[ "$RUNTIME_ROLE" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,62}$ ]] || fail "runtime role is invalid"
[[ -x "$VERMORY_BINARY" ]] || fail "vermory binary is not executable"

CHECKSUM_LINE=$(cat "$CHECKSUM")
[[ "$CHECKSUM_LINE" =~ ^([a-f0-9]{64})[[:space:]][[:space:]]([^/]+)$ ]] || fail "backup checksum sidecar is malformed"
[[ "${BASH_REMATCH[2]}" == "$(basename "$BACKUP")" ]] || fail "backup checksum sidecar names a different artifact"
(
  cd "$(dirname "$BACKUP")"
  sha256sum --check "$(basename "$CHECKSUM")"
) >/dev/null || fail "backup checksum verification failed"

TARGET_TABLES=$(psql "service=$TARGET_PG_SERVICE" -X -v ON_ERROR_STOP=1 -Atqc \
  "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r','p') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname !~ '^pg_toast'")
[[ "$TARGET_TABLES" == 0 ]] || fail "target database is not empty"

ROLE_STATE=$(psql "service=$TARGET_PG_SERVICE" -X -v ON_ERROR_STOP=1 -At \
  --set=runtime_role="$RUNTIME_ROLE" -c \
  "SELECT rolcanlogin::int || ':' || rolsuper::int || ':' || rolbypassrls::int FROM pg_roles WHERE rolname = :'runtime_role'")
[[ "$ROLE_STATE" == "1:0:0" ]] || fail "runtime role must already exist as LOGIN NOSUPERUSER NOBYPASSRLS"

pg_restore --exit-on-error --no-owner --no-acl --dbname "service=$TARGET_PG_SERVICE" "$BACKUP"
"$VERMORY_BINARY" database migrate --database-url "service=$TARGET_PG_SERVICE"
"$VERMORY_BINARY" database grant-runtime --database-url "service=$TARGET_PG_SERVICE" --role "$RUNTIME_ROLE"
"$VERMORY_BINARY" database rebuild-projections --database-url "service=$TARGET_PG_SERVICE"
echo "restored $(basename "$BACKUP") into libpq service $TARGET_PG_SERVICE"
