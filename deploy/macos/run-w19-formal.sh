#!/bin/sh

set -eu
umask 077

die() {
  printf '%s\n' "run-w19-formal: $*" >&2
  exit 2
}

repo_root=$(unset CDPATH; cd -- "$(dirname "$0")/../.." && pwd)
run_id=${VERMORY_W19_RUN_ID:-w19-formal-$(date -u '+%Y%m%dT%H%M%SZ')}
case "$run_id" in
  ''|*[!A-Za-z0-9._-]*) die "VERMORY_W19_RUN_ID contains unsafe characters" ;;
esac

artifact_root=${VERMORY_W19_ARTIFACT_ROOT:-"$HOME/Library/Application Support/Vermory/evidence/w19/$run_id"}
case "$artifact_root" in
  /*) ;;
  *) die "VERMORY_W19_ARTIFACT_ROOT must be absolute" ;;
esac
[ "$artifact_root" != "/" ] || die "VERMORY_W19_ARTIFACT_ROOT cannot be /"

postgres_root=${VERMORY_W19_POSTGRES_ROOT:-"$artifact_root/postgres"}
postgres_bin=${VERMORY_POSTGRES18_BIN:-/opt/homebrew/opt/postgresql@18/bin}
[ -x "$postgres_bin/postgres" ] || die "PostgreSQL 18 binaries are unavailable: $postgres_bin"
[ -x "$postgres_bin/initdb" ] || die "initdb is unavailable: $postgres_bin"

[ -d "$repo_root/.git" ] || die "repository is not a Git checkout: $repo_root"
if [ -n "$(git -C "$repo_root" status --porcelain)" ]; then
  die "formal W19 run requires a clean worktree"
fi

revision=${VERMORY_IMPLEMENTATION_REVISION:-$(git -C "$repo_root" rev-parse HEAD)}
case "$revision" in
  *[!0-9a-f]*) die "implementation revision is not lowercase hexadecimal" ;;
esac
[ "${#revision}" -eq 40 ] || die "implementation revision must contain 40 hex characters"

[ ! -e "$artifact_root/$run_id/report.json" ] || die "artifact already exists for run: $run_id"
mkdir -p "$artifact_root"

user_name=${USER:-$(id -un)}
api_key=$(/usr/bin/security find-generic-password -a "$user_name" -s vermory-siliconflow -w 2>/dev/null) ||
  die "Keychain item vermory-siliconflow is missing"
[ -n "$api_key" ] || die "Keychain item vermory-siliconflow is empty"
export SILICONFLOW_API_KEY="$api_key"
unset api_key

go_bin=${VERMORY_GO_BIN:-$(command -v go || true)}
[ -x "$go_bin" ] || die "Go executable is unavailable"

export VERMORY_W19_FORMAL_PROFILE=1
export VERMORY_W19_RUN_ID="$run_id"
export VERMORY_W19_ARTIFACT_ROOT="$artifact_root"
export VERMORY_W19_POSTGRES_ROOT="$postgres_root"
export VERMORY_POSTGRES18_BIN="$postgres_bin"
export VERMORY_IMPLEMENTATION_REVISION="$revision"

printf '%s\n' "run-w19-formal: starting run $run_id"
cd "$repo_root"
exec "$go_bin" test -p 1 -count=1 ./internal/runtime \
  -run TestMemoryEligibilityFormalProfile -v -timeout 60m
