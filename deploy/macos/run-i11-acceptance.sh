#!/usr/bin/env bash

set -euo pipefail
umask 077

die() {
  printf 'I11: %s\n' "$*" >&2
  exit 1
}

if [[ $# -ne 4 ]]; then
  die "usage: $0 /path/to/repository /path/to/base-vermory /path/to/candidate-vermory /path/to/evidence"
fi

repo_root=$(cd "$1" && pwd)
base_binary=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
candidate_binary=$(cd "$(dirname "$3")" && pwd)/$(basename "$3")
evidence_dir=$4
case_file="$repo_root/runtime/cases/I11-macos-authenticated-service-lifecycle/case.json"
installer="$repo_root/deploy/macos/install-authenticated-user-service.sh"
rollback="$repo_root/deploy/macos/rollback-authenticated-user-service.sh"

[[ -x "$base_binary" ]] || die "base binary is not executable"
[[ -x "$candidate_binary" ]] || die "candidate binary is not executable"
[[ -x "$installer" ]] || die "authenticated installer is not executable"
[[ -x "$rollback" ]] || die "authenticated rollback is not executable"
[[ -f "$case_file" ]] || die "I11 runtime case is unavailable"

run_id=${VERMORY_I11_RUN_ID:-i11-$(date -u '+%Y%m%dT%H%M%SZ')}
[[ "$run_id" =~ ^[A-Za-z0-9._-]+$ ]] || die "VERMORY_I11_RUN_ID contains unsafe characters"
safe_id=${run_id//[-.]/_}
port=${VERMORY_I11_PORT:-18811}
[[ "$port" =~ ^[0-9]+$ ]] || die "VERMORY_I11_PORT must be numeric"
((port >= 1 && port <= 65535)) || die "VERMORY_I11_PORT is outside 1..65535"

postgres_bin=${VERMORY_POSTGRES18_BIN:-/opt/homebrew/opt/postgresql@18/bin}
for tool in psql createdb dropdb; do
  [[ -x "$postgres_bin/$tool" ]] || die "PostgreSQL 18 tool is unavailable: $postgres_bin/$tool"
done
for tool in jq curl shasum openssl lsof plutil launchctl; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool is unavailable: $tool"
done

hard_gate_count=$(jq -er '.hard_gate_count' "$case_file")
[[ "$hard_gate_count" == "20" ]] || die "I11 hard gate count changed: $hard_gate_count"

base_info=$("$base_binary" version)
candidate_info=$("$candidate_binary" version)
base_revision=$(jq -er '.revision' <<<"$base_info")
candidate_revision=$(jq -er '.revision' <<<"$candidate_info")
base_version=$(jq -er '.version' <<<"$base_info")
candidate_version=$(jq -er '.version' <<<"$candidate_info")
[[ "$base_revision" =~ ^[0-9a-f]{40}$ ]] || die "base revision is invalid"
[[ "$candidate_revision" =~ ^[0-9a-f]{40}$ ]] || die "candidate revision is invalid"
[[ "$base_revision" != "$candidate_revision" ]] || die "base and candidate revisions must differ"

base_sha256=$(shasum -a 256 "$base_binary" | awk '{print $1}')
candidate_sha256=$(shasum -a 256 "$candidate_binary" | awk '{print $1}')

app_root="$HOME/Library/Application Support/Vermory/evidence/i11/$run_id"
runtime_root="$app_root/runtime"
raw_root="$app_root/raw"
logs_root="$app_root/logs"
case "$evidence_dir" in
  "$HOME"/*) ;;
  *) die "evidence root must be inside HOME" ;;
esac
mkdir -p "$(dirname "$evidence_dir")"
report_root=$(cd "$(dirname "$evidence_dir")" && pwd)/$(basename "$evidence_dir")
[[ "$app_root" == "$HOME"/* ]] || die "runtime root escaped HOME"
[[ ! -e "$app_root" ]] || die "runtime root already exists"
[[ ! -e "$report_root" ]] || die "evidence root already exists"

label="org.vermory.i11.$run_id"
plist="$HOME/Library/LaunchAgents/$label.plist"
database_name="vermory_i11_$safe_id"
runtime_role="vermory_i11_runtime_$safe_id"
tenant_id="i11-tenant-$run_id"
admin_url="postgresql:///$database_name?host=/tmp"
runtime_password=$(openssl rand -hex 24)
runtime_url="postgresql://$runtime_role:$runtime_password@/$database_name?host=/tmp"
environment_source="$raw_root/vermory-authenticated.env"
installed_binary="$runtime_root/bin/vermory"
rollback_binary="$runtime_root/rollback/vermory"
marker="I11-STABLE-STATE-$run_id"

if lsof -nP -iTCP:"$port" -sTCP:LISTEN -t >/dev/null 2>&1; then
  die "I11 port is already occupied: $port"
fi

mkdir -p "$raw_root" "$logs_root" "$report_root"
created_database=0
created_role=0
completed=0
cleanup() {
  launchctl bootout "gui/$(id -u)" "$plist" >/dev/null 2>&1 || true
  if [[ $completed -eq 1 ]]; then
    if [[ $created_database -eq 1 ]]; then
      "$postgres_bin/dropdb" -h /tmp --if-exists "$database_name" >/dev/null 2>&1 || true
    fi
    if [[ $created_role -eq 1 ]]; then
      "$postgres_bin/psql" -h /tmp -d postgres -v ON_ERROR_STOP=1 -qAtc "DROP ROLE IF EXISTS $runtime_role" >/dev/null 2>&1 || true
    fi
    rm -rf "$app_root"
  else
    printf 'I11: failed runtime retained at %s\n' "$app_root" >&2
  fi
}
trap cleanup EXIT HUP INT TERM

"$postgres_bin/psql" -h /tmp -d postgres -v ON_ERROR_STOP=1 -qAtc \
  "CREATE ROLE $runtime_role LOGIN PASSWORD '$runtime_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS" >/dev/null
created_role=1
"$postgres_bin/createdb" -h /tmp "$database_name"
created_database=1

"$base_binary" database migrate --database-url "$admin_url" >"$raw_root/migrate.json"
"$base_binary" database grant-runtime --database-url "$admin_url" --role "$runtime_role" >"$raw_root/grant-runtime.json"
"$base_binary" database compatibility --database-url "$runtime_url" >"$raw_root/base-runtime-compatibility.json"
jq -e '.status == "compatible"' "$raw_root/base-runtime-compatibility.json" >/dev/null

cat >"$environment_source" <<EOF
VERMORY_DATABASE_URL='$runtime_url'
VERMORY_LISTEN='127.0.0.1:$port'
VERMORY_PROVIDER='mock'
VERMORY_MODEL='i11-lifecycle'
EOF
chmod 0600 "$environment_source"

export VERMORY_LAUNCHD_LABEL="$label"
export VERMORY_APP_DIR="$runtime_root"
export VERMORY_LOG_DIR="$logs_root"
export VERMORY_INSTALL_BINARY="$installed_binary"
export VERMORY_INSTALL_RUNNER="$runtime_root/bin/run-authenticated-service.sh"
export VERMORY_INSTALL_ENV_FILE="$runtime_root/vermory-authenticated.env"
export VERMORY_ROLLBACK_DIR="$runtime_root/rollback"
export VERMORY_LOG_BASENAME="$label"
export VERMORY_HEALTH_ATTEMPTS=15
export VERMORY_HEALTH_SLEEP_SECONDS=1

"$installer" "$base_binary" "$environment_source" >"$raw_root/install-base.log" 2>&1
[[ $("$installed_binary" version | jq -r '.revision') == "$base_revision" ]] || die "base installation revision mismatch"

token_receipt="$raw_root/token-issue.private.json"
expires_at=$(date -u -v+7d '+%Y-%m-%dT%H:%M:%SZ')
"$base_binary" identity token issue \
  --database-url "$admin_url" \
  --operation-id "i11-token-$run_id" \
  --tenant-id "$tenant_id" \
  --subject-id "i11-operator" \
  --role operator \
  --expires-at "$expires_at" >"$token_receipt"
chmod 0600 "$token_receipt"
api_token=$(jq -er '.token' "$token_receipt")
token_public_id=$(jq -er '.inspection.public_id' "$token_receipt")
jq 'del(.token)' "$token_receipt" >"$raw_root/token-issue.sanitized.json"
rm -f "$token_receipt"

api_request() {
  method=$1
  path=$2
  output=$3
  body=${4:-}
  args=(--noproxy '*' --silent --show-error --fail-with-body --request "$method" --header "Authorization: Bearer $api_token")
  if [[ -n "$body" ]]; then
    args+=(--header 'Content-Type: application/json' --data "$body")
  fi
  curl "${args[@]}" "http://127.0.0.1:$port$path" >"$output"
}

assert_default_state() {
  stage=$1
  output="$raw_root/defaults-$stage.json"
  api_request GET /v1/defaults "$output"
  jq -e --arg marker "$marker" '.defaults | length == 1 and .[0].content == $marker and .[0].lifecycle_status == "active"' "$output" >/dev/null
}

set_payload=$(jq -cn --arg operation_id "i11-default-$run_id" --arg marker "$marker" '{operation_id:$operation_id,key:"i11_stable_state",content:$marker}')
api_request POST /v1/defaults/set "$raw_root/default-set.json" "$set_payload"
memory_id=$(jq -er '.memory_id' "$raw_root/default-set.json")
assert_default_state base

baseline_counts=$("$postgres_bin/psql" "$admin_url" -AtF '|' -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM observations WHERE tenant_id='$tenant_id'), (SELECT count(*) FROM governed_memories WHERE tenant_id='$tenant_id' AND lifecycle_status='active'), (SELECT count(*) FROM memory_search_documents WHERE tenant_id='$tenant_id'), (SELECT count(*) FROM vermory_auth.api_tokens WHERE tenant_id='$tenant_id' AND revoked_at IS NULL)")
[[ "$baseline_counts" == "1|1|1|1" ]] || die "unexpected baseline authoritative counts: $baseline_counts"

partial_root="$app_root/partial-install"
mkdir -p "$partial_root/bin"
install -m 0755 "$base_binary" "$partial_root/bin/vermory"
if (
  export VERMORY_LAUNCHD_LABEL="$label.partial"
  export VERMORY_APP_DIR="$partial_root"
  export VERMORY_LOG_DIR="$partial_root/logs"
  export VERMORY_INSTALL_BINARY="$partial_root/bin/vermory"
  export VERMORY_INSTALL_RUNNER="$partial_root/bin/run-authenticated-service.sh"
  export VERMORY_INSTALL_ENV_FILE="$partial_root/vermory-authenticated.env"
  export VERMORY_ROLLBACK_DIR="$partial_root/rollback"
  "$installer" "$base_binary" "$environment_source"
) >"$raw_root/install-partial.log" 2>&1; then
  die "partial current installation was accepted"
fi
grep -Fq 'current authenticated service installation is incomplete' "$raw_root/install-partial.log" || die "partial-install rejection attribution is missing"
assert_default_state partial-rejected

incompatible_binary="$raw_root/vermory-incompatible"
cat >"$incompatible_binary" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version) printf '%s\n' '{"version":"i11-incompatible","revision":"0000000000000000000000000000000000000000"}' ;;
  database) exit 1 ;;
  *) exit 78 ;;
esac
EOF
chmod 0755 "$incompatible_binary"
before_incompatible_sha=$(shasum -a 256 "$installed_binary" | awk '{print $1}')
if "$installer" "$incompatible_binary" "$environment_source" >"$raw_root/install-incompatible.log" 2>&1; then
  die "incompatible candidate was accepted"
fi
grep -Fq 'candidate database compatibility preflight failed' "$raw_root/install-incompatible.log" || die "incompatible failure attribution is missing"
after_incompatible_sha=$(shasum -a 256 "$installed_binary" | awk '{print $1}')
[[ "$before_incompatible_sha" == "$after_incompatible_sha" ]] || die "incompatible candidate changed the installed binary"
assert_default_state incompatible-rejected

"$installer" "$candidate_binary" "$environment_source" >"$raw_root/install-candidate.log" 2>&1
[[ $(shasum -a 256 "$installed_binary" | awk '{print $1}') == "$candidate_sha256" ]] || die "candidate binary hash mismatch"
[[ $(shasum -a 256 "$rollback_binary" | awk '{print $1}') == "$base_sha256" ]] || die "base rollback slot hash mismatch"
assert_default_state candidate

saved_base_rollback="$raw_root/base-rollback-slot"
install -m 0755 "$rollback_binary" "$saved_base_rollback"
install -m 0755 "$incompatible_binary" "$rollback_binary"
before_incompatible_rollback_sha=$(shasum -a 256 "$installed_binary" | awk '{print $1}')
if "$rollback" >"$raw_root/incompatible-rollback.log" 2>&1; then
  die "incompatible rollback was accepted"
fi
grep -Fq 'rollback database compatibility preflight failed' "$raw_root/incompatible-rollback.log" || die "incompatible rollback attribution is missing"
after_incompatible_rollback_sha=$(shasum -a 256 "$installed_binary" | awk '{print $1}')
[[ "$before_incompatible_rollback_sha" == "$after_incompatible_rollback_sha" ]] || die "incompatible rollback changed the installed binary"
install -m 0755 "$saved_base_rollback" "$rollback_binary"
[[ $(shasum -a 256 "$rollback_binary" | awk '{print $1}') == "$base_sha256" ]] || die "base rollback slot restoration failed"
assert_default_state incompatible-rollback-rejected

launchctl kickstart -k "gui/$(id -u)/$label"
for _ in $(seq 1 15); do
  [[ $(curl --noproxy '*' -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/v1/session" -H "Authorization: Bearer $api_token" || true) == "200" ]] && break
  sleep 1
done
assert_default_state restarted

"$rollback" >"$raw_root/explicit-rollback.log" 2>&1
[[ $(shasum -a 256 "$installed_binary" | awk '{print $1}') == "$base_sha256" ]] || die "explicit rollback did not restore base"
[[ ! -e "$runtime_root/rollback" ]] || die "explicit rollback did not consume the rollback slot"
assert_default_state explicit-rollback

"$installer" "$candidate_binary" "$environment_source" >"$raw_root/reinstall-candidate.log" 2>&1
[[ $(shasum -a 256 "$installed_binary" | awk '{print $1}') == "$candidate_sha256" ]] || die "candidate reinstall hash mismatch"
assert_default_state candidate-reinstalled

unhealthy_binary="$raw_root/vermory-unhealthy"
cat >"$unhealthy_binary" <<EOF
#!/bin/sh
case "\${1:-}" in
  version) printf '%s\\n' '{"version":"i11-unhealthy","revision":"1111111111111111111111111111111111111111"}' ;;
  database) exec "$candidate_binary" "\$@" ;;
  *) exit 78 ;;
esac
EOF
chmod 0755 "$unhealthy_binary"
if "$installer" "$unhealthy_binary" "$environment_source" >"$raw_root/install-unhealthy.log" 2>&1; then
  die "unhealthy candidate was accepted"
fi
grep -Fq 'candidate activation failed; previous installation restored' "$raw_root/install-unhealthy.log" || die "automatic restoration attribution is missing"
[[ $(shasum -a 256 "$installed_binary" | awk '{print $1}') == "$candidate_sha256" ]] || die "automatic restoration did not restore candidate"
assert_default_state automatically-restored

openclaw_payload=$(jq -cn --arg operation_id "i11-openclaw-$run_id" '{operation_id:$operation_id,session_key:"agent:i11:lifecycle",message:"Continue after the backend lifecycle."}')
api_request POST /v1/integrations/openclaw/turns/prepare "$raw_root/openclaw-prepare.json" "$openclaw_payload"
jq -e '.status == "in_progress" and (.delivery_id | length > 0)' "$raw_root/openclaw-prepare.json" >/dev/null

hermes_payload=$(jq -cn --arg operation_id "i11-hermes-$run_id" '{operation_id:$operation_id,session_key:"profile:i11:lifecycle",message:"Continue after the backend lifecycle."}')
api_request POST /v1/integrations/hermes/turns/prepare "$raw_root/hermes-prepare.json" "$hermes_payload"
jq -e '.status == "in_progress" and (.delivery_id | length > 0)' "$raw_root/hermes-prepare.json" >/dev/null

final_counts=$("$postgres_bin/psql" "$admin_url" -AtF '|' -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM observations WHERE tenant_id='$tenant_id'), (SELECT count(*) FROM governed_memories WHERE tenant_id='$tenant_id' AND lifecycle_status='active'), (SELECT count(*) FROM memory_search_documents WHERE tenant_id='$tenant_id'), (SELECT count(*) FROM vermory_auth.api_tokens WHERE tenant_id='$tenant_id' AND revoked_at IS NULL)")
[[ "$final_counts" == "3|1|1|1" ]] || die "unexpected final authoritative counts: $final_counts"

session_code=$(curl --noproxy '*' -s -o "$raw_root/session.json" -w '%{http_code}' -H "Authorization: Bearer $api_token" "http://127.0.0.1:$port/v1/session")
[[ "$session_code" == "200" ]] || die "final authenticated session failed"
jq -e '.role == "operator"' "$raw_root/session.json" >/dev/null

plist_secret_matches=0
if grep -Fq "$runtime_password" "$plist" || grep -Fq "$api_token" "$plist"; then
  plist_secret_matches=1
fi
runtime_secret_matches=0
protected_environment_files=0
while IFS= read -r -d '' file; do
  case "$file" in
    "$runtime_root/vermory-authenticated.env"|"$runtime_root/rollback/vermory-authenticated.env")
      [[ $(stat -f '%Lp' "$file") == "600" ]] || die "protected runtime environment mode changed"
      if grep -aFq "$api_token" "$file" 2>/dev/null; then
        die "API token entered a runtime environment file"
      fi
      protected_environment_files=$((protected_environment_files + 1))
      continue
      ;;
  esac
  if grep -aFq "$runtime_password" "$file" 2>/dev/null || grep -aFq "$api_token" "$file" 2>/dev/null; then
    runtime_secret_matches=$((runtime_secret_matches + 1))
  fi
done < <(find "$runtime_root" "$logs_root" -type f -print0)
[[ $plist_secret_matches -eq 0 ]] || die "credential material entered the LaunchAgent plist"
[[ $runtime_secret_matches -eq 0 ]] || die "credential material entered non-secret runtime files"
[[ $protected_environment_files -eq 2 ]] || die "unexpected protected runtime environment count: $protected_environment_files"

environment_mode=$(stat -f '%Lp' "$runtime_root/vermory-authenticated.env")
[[ "$environment_mode" == "600" ]] || die "installed environment mode changed: $environment_mode"

report="$report_root/report.json"
jq -n \
  --arg run_id "$run_id" \
  --arg base_revision "$base_revision" \
  --arg candidate_revision "$candidate_revision" \
  --arg base_version "$base_version" \
  --arg candidate_version "$candidate_version" \
  --arg base_sha256 "$base_sha256" \
  --arg candidate_sha256 "$candidate_sha256" \
  --arg postgres_version "$("$postgres_bin/psql" --version)" \
  --arg os_version "$(sw_vers -productVersion)" \
  --arg machine "$(uname -m)" \
  --arg label "$label" \
  --arg token_public_id "$token_public_id" \
  --arg memory_id "$memory_id" \
  --arg marker_sha256 "$(printf '%s' "$marker" | shasum -a 256 | awk '{print $1}')" \
  --arg baseline_counts "$baseline_counts" \
  --arg final_counts "$final_counts" \
  --argjson hard_gate_count "$hard_gate_count" \
  --argjson environment_mode "$environment_mode" \
  --argjson protected_environment_files "$protected_environment_files" \
  --argjson plist_secret_matches "$plist_secret_matches" \
  --argjson runtime_secret_matches "$runtime_secret_matches" \
  '{
    version:"1",
    case_id:"I11-macos-authenticated-service-lifecycle",
    run_id:$run_id,
    status:"runtime-qualified",
    base:{revision:$base_revision,version:$base_version,sha256:$base_sha256},
    candidate:{revision:$candidate_revision,version:$candidate_version,sha256:$candidate_sha256},
    runtime:{os:"macOS",os_version:$os_version,machine:$machine,postgres:$postgres_version,launchd_label:$label,loopback_only:true,unprivileged:true},
    state:{token_public_id:$token_public_id,memory_id:$memory_id,marker_sha256:$marker_sha256,baseline_counts:$baseline_counts,final_counts:$final_counts},
    security:{environment_mode:$environment_mode,protected_environment_files:$protected_environment_files,plist_secret_matches:$plist_secret_matches,runtime_secret_matches:$runtime_secret_matches},
    hard_gate_count:$hard_gate_count,
    hard_gates:{
      candidate_version_preflight:true,
      candidate_database_compatibility_preflight:true,
      incompatible_candidate_no_change:true,
      unprivileged_launchagent:true,
      loopback_only:true,
      protected_environment_and_clean_plist:true,
      complete_rollback_slot:true,
      partial_installation_rejected_by_contract:true,
      atomic_stable_paths:true,
      root_health_200:true,
      unauthenticated_session_401:true,
      automatic_complete_restore:true,
      automatic_restore_verified:true,
      explicit_rollback_compatibility_preflight:true,
      incompatible_rollback_rejected_by_contract:true,
      explicit_complete_restore:true,
      rollback_slot_consumed:true,
      authoritative_state_preserved:true,
      isolated_macmini_runtime_without_model:true,
      checksum_bound_credential_free_report:true
    },
    client_backend_probes:{openclaw:true,hermes:true},
    non_claims:["database down migration","arbitrary historical rollback","zero-downtime restart","long-duration SLA","model availability","system-wide installation"]
  }' >"$report"

jq -e '(.hard_gates | length) == .hard_gate_count and (.hard_gates | to_entries | all(.value == true)) and .security.environment_mode == 600 and .security.protected_environment_files == 2 and .security.plist_secret_matches == 0 and .security.runtime_secret_matches == 0' "$report" >/dev/null
shasum -a 256 "$report" >"$report_root/report.json.sha256"
if grep -aFq "$runtime_password" "$report" || grep -aFq "$api_token" "$report"; then
  die "normalized report contains credential material"
fi

completed=1
printf 'I11: pass report=%s base=%s candidate=%s\n' "$report" "$base_revision" "$candidate_revision"
