#!/usr/bin/env bash

set -euo pipefail
umask 077

die() {
  printf 'W36: %s\n' "$*" >&2
  exit 1
}

if [[ $# -ne 3 ]]; then
  die "usage: $0 /path/to/repository /path/to/vermory /path/to/evidence"
fi

repo_root=$(cd "$1" && pwd)
binary=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
evidence_root=$3
case_file="$repo_root/runtime/cases/W36-long-running-client-operation/case.json"
openclaw_client="$repo_root/integrations/openclaw/dist/client.js"
openclaw_plugin="$repo_root/integrations/openclaw/dist/index.js"
openclaw_runner="$repo_root/deploy/macos/run-w36-openclaw-client.mjs"

[[ -x "$binary" ]] || die "candidate binary is not executable"
[[ -f "$case_file" ]] || die "W36 case is unavailable"
[[ -f "$openclaw_client" ]] || die "built OpenClaw client module is unavailable"
[[ -f "$openclaw_plugin" ]] || die "built OpenClaw plugin module is unavailable"
[[ -x "$openclaw_runner" ]] || die "OpenClaw W36 runner is not executable"

run_id=${VERMORY_W36_RUN_ID:-w36-$(date -u '+%Y%m%dT%H%M%SZ')}
[[ "$run_id" =~ ^[A-Za-z0-9._-]+$ ]] || die "VERMORY_W36_RUN_ID contains unsafe characters"
safe_id=${run_id//[-.]/_}
port=${VERMORY_W36_PORT:-18836}
[[ "$port" =~ ^[0-9]+$ ]] || die "VERMORY_W36_PORT must be numeric"
((port >= 1 && port <= 65535)) || die "VERMORY_W36_PORT is outside 1..65535"

postgres_bin=${VERMORY_POSTGRES_BIN:-/opt/homebrew/opt/postgresql@18/bin}
for tool in psql initdb pg_ctl createdb pg_dump pg_restore; do
  [[ -x "$postgres_bin/$tool" ]] || die "PostgreSQL client is missing: $postgres_bin/$tool"
done
command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
command -v node >/dev/null || die "node is required"

mkdir -p "$evidence_root"
evidence_root=$(cd "$evidence_root" && pwd)
raw_root="$evidence_root/raw"
report="$evidence_root/report.json"
mkdir -p "$raw_root"

database_name="vermory_${safe_id}"
restore_database="vermory_w36_restore_${safe_id:0:32}"
runtime_role="vermory_${safe_id}_runtime"
tenant_id="w36-runtime-$run_id"
runtime_password=$(openssl rand -hex 24)
base_url="http://127.0.0.1:$port"
service_log="$raw_root/service.log"
service_pid=''
postgres_data=''
postgres_socket=''
postgres_log="$raw_root/postgres.log"
postgres_port=${VERMORY_W36_POSTGRES_PORT:-55436}
[[ "$postgres_port" =~ ^[0-9]+$ ]] || die "VERMORY_W36_POSTGRES_PORT must be numeric"
((postgres_port >= 1 && postgres_port <= 65535)) || die "VERMORY_W36_POSTGRES_PORT is outside 1..65535"
admin_url=''
runtime_url=''
active_runtime_url=''
work_root=''
api_token=''
operator_token=''
attacker_token=''
created_database=0
created_role=0
token_receipt=''
operator_token_receipt=''
attacker_token_receipt=''

stop_service() {
  if [[ -n "$service_pid" ]] && kill -0 "$service_pid" >/dev/null 2>&1; then
    kill -TERM "$service_pid"
    wait "$service_pid" || true
  fi
  service_pid=''
}

stop_postgres() {
  if [[ -n "$postgres_data" ]] && "$postgres_bin/pg_ctl" -D "$postgres_data" status >/dev/null 2>&1; then
    "$postgres_bin/pg_ctl" -D "$postgres_data" -m fast stop >/dev/null 2>&1 || true
  fi
}

cleanup() {
  stop_service
  stop_postgres
  if [[ -n "$work_root" && -d "$work_root" ]]; then
    rm -rf -- "$work_root"
  fi
  api_token=''
  operator_token=''
  attacker_token=''
  token_receipt=''
  operator_token_receipt=''
  attacker_token_receipt=''
  runtime_password=''
}
trap cleanup EXIT HUP INT TERM

work_parent=${VERMORY_W36_WORK_PARENT:-$(dirname "$evidence_root")}
mkdir -p "$work_parent"
work_parent=$(cd "$work_parent" && pwd)
work_root=$(mktemp -d "$work_parent/.w36-work.XXXXXX")
postgres_data="$work_root/postgres"
postgres_socket="$work_root/s"
(( ${#postgres_socket} + 20 <= 103 )) || die "temporary PostgreSQL socket path is too long"
mkdir -m 700 "$postgres_socket"
"$postgres_bin/initdb" -D "$postgres_data" --username=postgres --auth-local=trust --auth-host=scram-sha-256 --no-instructions >"$raw_root/initdb.log"
"$postgres_bin/pg_ctl" -D "$postgres_data" -l "$postgres_log" \
  -o "-p $postgres_port -k $postgres_socket -c listen_addresses=127.0.0.1 -c unix_socket_permissions=0700" start >/dev/null
for _ in $(seq 1 40); do
  if "$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
"$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null || die "temporary PostgreSQL did not become ready"
admin_url="postgresql:///$database_name?host=$postgres_socket&port=$postgres_port&user=postgres&sslmode=disable"
runtime_url="postgresql://$runtime_role:$runtime_password@127.0.0.1:$postgres_port/$database_name?sslmode=disable"
active_runtime_url="$runtime_url"

"$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -v ON_ERROR_STOP=1 -qAtc \
  "CREATE ROLE $runtime_role LOGIN PASSWORD '$runtime_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS" >/dev/null
created_role=1
"$postgres_bin/createdb" -h "$postgres_socket" -p "$postgres_port" -U postgres "$database_name"
created_database=1

"$binary" database migrate --database-url "$admin_url" >"$raw_root/migrate.json"
"$binary" database grant-runtime --database-url "$admin_url" --role "$runtime_role" >"$raw_root/grant-runtime.json"
"$binary" database compatibility --database-url "$runtime_url" >"$raw_root/compatibility.json"
jq -e '.status == "compatible" and .schema_version == 25' "$raw_root/compatibility.json" >/dev/null

expires_at=$(date -u -v+1d '+%Y-%m-%dT%H:%M:%SZ')
token_receipt=$("$binary" identity token issue \
  --database-url "$admin_url" \
  --operation-id "w36-token-$run_id" \
  --tenant-id "$tenant_id" \
  --subject-id "w36-client" \
  --role client \
  --expires-at "$expires_at")
api_token=$(jq -er '.token' <<<"$token_receipt")
jq 'del(.token)' <<<"$token_receipt" >"$raw_root/token.json"

operator_token_receipt=$("$binary" identity token issue \
  --database-url "$admin_url" \
  --operation-id "w36-operator-token-$run_id" \
  --tenant-id "$tenant_id" \
  --subject-id "w36-operator" \
  --role operator \
  --expires-at "$expires_at")
operator_token=$(jq -er '.token' <<<"$operator_token_receipt")
jq 'del(.token)' <<<"$operator_token_receipt" >"$raw_root/operator-token.json"

start_service() {
  env \
    VERMORY_DATABASE_URL="$active_runtime_url" \
    VERMORY_LISTEN="127.0.0.1:$port" \
    VERMORY_PROVIDER=mock \
    VERMORY_MODEL=w36-runtime \
    "$binary" serve >>"$service_log" 2>&1 &
  service_pid=$!
  for _ in $(seq 1 40); do
    if [[ $(curl --noproxy '*' -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $api_token" "$base_url/v1/session" || true) == "200" ]]; then
      return
    fi
    sleep 0.25
  done
  die "authenticated service did not become ready"
}

api_request() {
  api_request_with_token "$api_token" "$@"
}

api_request_with_token() {
  local token_value=$1
  local method=$2
  local path=$3
  local output=$4
  local body=${5:-}
  local args=(--noproxy '*' --silent --show-error --request "$method" --header "Authorization: Bearer $token_value")
  if [[ -n "$body" ]]; then
    args+=(--header 'Content-Type: application/json' --data "$body")
  fi
  curl "${args[@]}" "$base_url$path" >"$output"
}

api_status() {
  api_status_with_token "$api_token" "$@"
}

api_status_with_token() {
  local token_value=$1
  local method=$2
  local path=$3
  local output=$4
  local body=$5
  curl --noproxy '*' --silent --show-error -o "$output" -w '%{http_code}' \
    --request "$method" \
    --header "Authorization: Bearer $token_value" \
    --header 'Content-Type: application/json' \
    --data "$body" \
    "$base_url$path"
}

start_service

default_payload=$(jq -cn --arg operation_id "w36-default-$run_id" \
  '{operation_id:$operation_id,key:"reply_language",content:"Default user-facing replies are written in Chinese."}')
[[ $(api_status_with_token "$operator_token" POST /v1/defaults/set "$raw_root/default-set.json" "$default_payload") == "200" ]] || die "operator default setup failed"

openclaw_operation="openclaw:w36-client-$run_id"
openclaw_session="agent:w36:restart"
openclaw_message="Continue the authenticated release repair after a service restart."
env VERMORY_API_TOKEN="$api_token" node "$openclaw_runner" "$openclaw_plugin" "$openclaw_client" start \
  "$base_url" "$openclaw_session" "$openclaw_operation" "$openclaw_message" >"$raw_root/openclaw-start.json"
jq -e '.status == "in_progress" and .protocol == "leased_v1" and .lease_generation == 1 and .checkpoint_sequence == 1 and .context_injected == true' "$raw_root/openclaw-start.json" >/dev/null
openclaw_reclaim_payload=$(jq -cn --arg operation_id "$openclaw_operation" --arg thread_id "$openclaw_session" \
  '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Continue the authenticated release repair after a service restart."}')
[[ $(api_status POST /v1/client-operations/reclaim "$raw_root/openclaw-live-reclaim.json" "$openclaw_reclaim_payload") == "409" ]] || die "live leased operation was reclaimed"

stop_service
stop_postgres
"$postgres_bin/pg_ctl" -D "$postgres_data" -l "$postgres_log" \
  -o "-p $postgres_port -k $postgres_socket -c listen_addresses=127.0.0.1 -c unix_socket_permissions=0700" start >/dev/null
for _ in $(seq 1 40); do
  if "$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
"$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null || die "PostgreSQL did not recover after restart"
start_service
env VERMORY_API_TOKEN="$api_token" node "$openclaw_runner" "$openclaw_plugin" "$openclaw_client" resume-complete \
  "$base_url" "$openclaw_session" "$openclaw_operation" "$openclaw_message" >"$raw_root/openclaw-resume.json"
jq -e '.status == "completed" and .protocol == "leased_v1" and .lease_generation == 1 and .checkpoint_sequence == 2 and .replayed_prepare == true and .context_injected == true and (.assistant_observation_id | length) > 0' "$raw_root/openclaw-resume.json" >/dev/null
jq -e --slurpfile first "$raw_root/openclaw-start.json" '.turn_id == $first[0].turn_id and .delivery_id == $first[0].delivery_id and .attempt_id == $first[0].attempt_id' "$raw_root/openclaw-resume.json" >/dev/null

reclaim_operation="openclaw:w36-reclaim-$run_id"
reclaim_thread="agent:w36:reclaim"
prepare_payload=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Run a reclaimable operation."}')
api_request POST /v1/client-operations/prepare "$raw_root/reclaim-prepare.json" "$prepare_payload"
old_attempt=$(jq -er '.attempt_id' "$raw_root/reclaim-prepare.json")
old_generation=$(jq -er '.lease_generation' "$raw_root/reclaim-prepare.json")
marker="W36_CHECKPOINT_ONLY_$safe_id"
checkpoint_payload=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" --arg marker "$marker" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:1,checkpoint:{phase:"pre_reclaim",marker:$marker}}')
api_request POST /v1/client-operations/checkpoint "$raw_root/reclaim-checkpoint.json" "$checkpoint_payload"
checkpoint_replay_payload="$checkpoint_payload"
api_request POST /v1/client-operations/checkpoint "$raw_root/reclaim-checkpoint-replay.json" "$checkpoint_replay_payload"
jq -e '.replayed == true and .checkpoint_sequence == 1' "$raw_root/reclaim-checkpoint-replay.json" >/dev/null
checkpoint_drift=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:1,checkpoint:{phase:"drift"}}')
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/reclaim-checkpoint-drift.json" "$checkpoint_drift") == "409" ]] || die "checkpoint sequence drift was accepted"
sensitive_checkpoint=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:2,checkpoint:{api_token:"W36_SYNTHETIC_SECRET_MUST_NOT_PERSIST"}}')
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/reclaim-checkpoint-sensitive.json" "$sensitive_checkpoint") == "400" ]] || die "sensitive checkpoint was accepted"
oversized_data=$(head -c 33000 /dev/zero | tr '\0' x)
oversized_checkpoint=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" --arg data "$oversized_data" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:2,checkpoint:{data:$data}}')
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/reclaim-checkpoint-oversized.json" "$oversized_checkpoint") == "400" ]] || die "oversized checkpoint was accepted"
reclaim_run_id=${reclaim_operation#openclaw:}
current_tool_payload=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg run_id "$reclaim_run_id" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,session_key:$thread_id,run_id:$run_id,tool_name:"release.verify",tool_call_id:"w36-reclaim-tool-current",content:"The current attempt verified the reclaimable release before expiry.",attempt_id:$attempt_id,lease_generation:$generation}')
api_request POST /v1/integrations/openclaw/turns/tool-results "$raw_root/reclaim-tool-current.json" "$current_tool_payload"
jq -e '(.observation_id | length) > 0 and .replayed == false' "$raw_root/reclaim-tool-current.json" >/dev/null
"$postgres_bin/psql" "$admin_url" -v ON_ERROR_STOP=1 -qAtc \
  "UPDATE conversation_turns SET lease_expires_at=now()-interval '1 second' WHERE tenant_id='$tenant_id' AND operation_id='$reclaim_operation'" >/dev/null
api_request POST /v1/client-operations/prepare "$raw_root/reclaim-expired-prepare-replay.json" "$prepare_payload"
jq -e --slurpfile first "$raw_root/reclaim-prepare.json" '.turn_id == $first[0].turn_id and .attempt_id == $first[0].attempt_id and .lease_generation == $first[0].lease_generation' "$raw_root/reclaim-expired-prepare-replay.json" >/dev/null
api_request POST /v1/client-operations/reclaim "$raw_root/reclaim.json" "$prepare_payload"
new_attempt=$(jq -er '.attempt_id' "$raw_root/reclaim.json")
new_generation=$(jq -er '.lease_generation' "$raw_root/reclaim.json")
jq -e --arg old "$old_attempt" --argjson generation "$((old_generation + 1))" --slurpfile first "$raw_root/reclaim-prepare.json" '.attempt_id != $old and .lease_generation == $generation and .checkpoint_sequence == 1 and .turn_id == $first[0].turn_id and .delivery_id == $first[0].delivery_id and .user_observation_id == $first[0].user_observation_id and .context == $first[0].context' "$raw_root/reclaim.json" >/dev/null

stale_heartbeat=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation}')
[[ $(api_status POST /v1/client-operations/heartbeat "$raw_root/stale-heartbeat.json" "$stale_heartbeat") == "409" ]] || die "stale heartbeat was not fenced"
stale_checkpoint=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" --arg marker "$marker" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:2,checkpoint:{marker:$marker,phase:"stale"}}')
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/stale-checkpoint.json" "$stale_checkpoint") == "409" ]] || die "stale checkpoint was not fenced"
stale_tool_payload=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg run_id "$reclaim_run_id" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,session_key:$thread_id,run_id:$run_id,tool_name:"release.verify",tool_call_id:"w36-reclaim-tool-stale",content:"This stale tool result must not become an observation.",attempt_id:$attempt_id,lease_generation:$generation}')
[[ $(api_status POST /v1/integrations/openclaw/turns/tool-results "$raw_root/stale-tool-result.json" "$stale_tool_payload") == "409" ]] || die "stale tool result was not fenced"
stale_complete=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,answer:"stale answer",model:"openclaw/stale"}')
[[ $(api_status POST /v1/client-operations/complete "$raw_root/stale-complete.json" "$stale_complete") == "409" ]] || die "stale completion was not fenced"
stale_fail=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,failure_code:"stale_attempt",failure_message:"stale failure"}')
[[ $(api_status POST /v1/client-operations/fail "$raw_root/stale-fail.json" "$stale_fail") == "409" ]] || die "stale failure was not fenced"
stale_cancel=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$old_attempt" --argjson generation "$old_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,cancellation_code:"stale_attempt",cancellation_message:"stale cancellation"}')
[[ $(api_status POST /v1/client-operations/cancel "$raw_root/stale-cancel.json" "$stale_cancel") == "409" ]] || die "stale cancellation was not fenced"
current_heartbeat=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation}')
api_request POST /v1/client-operations/heartbeat "$raw_root/reclaim-heartbeat.json" "$current_heartbeat"
jq -e '.status == "in_progress" and .attempt_id != "" and .lease_generation == 2' "$raw_root/reclaim-heartbeat.json" >/dev/null
reclaim_checkpoint_replay=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" --arg marker "$marker" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:1,checkpoint:{phase:"pre_reclaim",marker:$marker}}')
api_request POST /v1/client-operations/checkpoint "$raw_root/reclaim-current-checkpoint-replay.json" "$reclaim_checkpoint_replay"
jq -e '.replayed == true and .checkpoint_sequence == 1' "$raw_root/reclaim-current-checkpoint-replay.json" >/dev/null
reclaim_checkpoint_drift=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:1,checkpoint:{phase:"current-drift"}}')
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/reclaim-current-checkpoint-drift.json" "$reclaim_checkpoint_drift") == "409" ]] || die "current checkpoint drift was accepted"
current_checkpoint_two=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" --arg marker "$marker" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,sequence:2,checkpoint:{phase:"reclaimed",marker:$marker}}')
api_request POST /v1/client-operations/checkpoint "$raw_root/reclaim-current-checkpoint-two.json" "$current_checkpoint_two"
jq -e '.checkpoint_sequence == 2 and .replayed == false' "$raw_root/reclaim-current-checkpoint-two.json" >/dev/null
[[ $(api_status POST /v1/client-operations/checkpoint "$raw_root/reclaim-current-checkpoint-regression.json" "$reclaim_checkpoint_replay") == "409" ]] || die "checkpoint sequence regression was accepted"

checkpoint_leak_count=$("$postgres_bin/psql" "$admin_url" -At -v ON_ERROR_STOP=1 -c "SELECT (SELECT count(*) FROM observations WHERE tenant_id='$tenant_id' AND content LIKE '%' || '$marker' || '%') + (SELECT count(*) FROM governed_memories WHERE tenant_id='$tenant_id' AND content LIKE '%' || '$marker' || '%') + (SELECT count(*) FROM memory_search_documents WHERE tenant_id='$tenant_id' AND content LIKE '%' || '$marker' || '%') + (SELECT count(*) FROM memory_vector_documents WHERE tenant_id='$tenant_id' AND memory_id IN (SELECT id FROM governed_memories WHERE tenant_id='$tenant_id' AND content LIKE '%' || '$marker' || '%')) + (SELECT count(*) FROM memory_vector_documents_2560 WHERE tenant_id='$tenant_id' AND memory_id IN (SELECT id FROM governed_memories WHERE tenant_id='$tenant_id' AND content LIKE '%' || '$marker' || '%')) + (SELECT count(*) FROM memory_deliveries WHERE tenant_id='$tenant_id' AND context_body LIKE '%' || '$marker' || '%') + (SELECT count(*) FROM source_formation_items WHERE tenant_id='$tenant_id' AND (content LIKE '%' || '$marker' || '%' OR quote LIKE '%' || '$marker' || '%'))")
[[ "$checkpoint_leak_count" == "0" ]] || die "checkpoint entered a semantic surface: $checkpoint_leak_count"

attacker_tenant_id="w36-attacker-$run_id"
attacker_token_receipt=$("$binary" identity token issue \
  --database-url "$admin_url" \
  --operation-id "w36-attacker-token-$run_id" \
  --tenant-id "$attacker_tenant_id" \
  --subject-id "w36-attacker" \
  --role client \
  --expires-at "$expires_at")
attacker_token=$(jq -er '.token' <<<"$attacker_token_receipt")
jq 'del(.token)' <<<"$attacker_token_receipt" >"$raw_root/attacker-token.json"
api_request_with_token "$attacker_token" POST /v1/client-operations/prepare "$raw_root/cross-tenant-prepare.json" "$prepare_payload"
jq -e --slurpfile ours "$raw_root/reclaim.json" '.turn_id != $ours[0].turn_id and .continuity_id != $ours[0].continuity_id and .lease_generation == 1' "$raw_root/cross-tenant-prepare.json" >/dev/null
cross_tenant_heartbeat=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation}')
[[ $(api_status_with_token "$attacker_token" POST /v1/client-operations/heartbeat "$raw_root/cross-tenant-heartbeat.json" "$cross_tenant_heartbeat") == "409" ]] || die "another tenant mutated the current leased attempt"

other_continuity_operation="openclaw:w36-other-continuity-$run_id"
other_continuity_thread="agent:w36:other-continuity"
other_continuity_prepare=$(jq -cn --arg operation_id "$other_continuity_operation" --arg thread_id "$other_continuity_thread" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Create an unrelated continuity for an isolation check."}')
api_request POST /v1/client-operations/prepare "$raw_root/other-continuity-prepare.json" "$other_continuity_prepare"
other_continuity_attempt=$(jq -er '.attempt_id' "$raw_root/other-continuity-prepare.json")
other_continuity_generation=$(jq -er '.lease_generation' "$raw_root/other-continuity-prepare.json")
wrong_continuity_heartbeat=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$other_continuity_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation}')
[[ $(api_status POST /v1/client-operations/heartbeat "$raw_root/cross-continuity-heartbeat.json" "$wrong_continuity_heartbeat") == "400" ]] || die "another continuity mutated the leased operation"
other_continuity_failure=$(jq -cn --arg operation_id "$other_continuity_operation" --arg thread_id "$other_continuity_thread" --arg attempt_id "$other_continuity_attempt" --argjson generation "$other_continuity_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,failure_code:"isolation_control",failure_message:"The isolation-only operation stops without semantic writeback."}')
api_request POST /v1/client-operations/fail "$raw_root/other-continuity-fail.json" "$other_continuity_failure"
jq -e '.status == "failed" and (.assistant_observation_id // "") == ""' "$raw_root/other-continuity-fail.json" >/dev/null

bounded_bypass=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" '{operation_id:$operation_id,session_key:$thread_id,answer:"bounded bypass",model:"openclaw/legacy"}')
[[ $(api_status POST /v1/integrations/openclaw/turns/complete "$raw_root/bounded-bypass.json" "$bounded_bypass") == "409" ]] || die "bounded completion bypassed leased fencing"
current_complete=$(jq -cn --arg operation_id "$reclaim_operation" --arg thread_id "$reclaim_thread" --arg attempt_id "$new_attempt" --argjson generation "$new_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,answer:"Current reclaimed attempt completed.",model:"openclaw/w36-runtime"}')
api_request POST /v1/client-operations/complete "$raw_root/reclaim-complete.json" "$current_complete"
jq -e '.status == "completed" and .lease_generation == 2 and (.assistant_observation_id | length) > 0' "$raw_root/reclaim-complete.json" >/dev/null
api_request POST /v1/client-operations/complete "$raw_root/reclaim-complete-replay.json" "$current_complete"
jq -e '.status == "completed" and .replayed == true' "$raw_root/reclaim-complete-replay.json" >/dev/null
changed_complete=$(jq -c '.answer = "different terminal payload"' <<<"$current_complete")
[[ $(api_status POST /v1/client-operations/complete "$raw_root/reclaim-complete-drift.json" "$changed_complete") == "400" ]] || die "changed terminal completion was accepted"

cancel_operation="openclaw:w36-cancel-$run_id"
cancel_thread="agent:w36:cancel"
cancel_prepare=$(jq -cn --arg operation_id "$cancel_operation" --arg thread_id "$cancel_thread" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Start a cancellable operation."}')
api_request POST /v1/client-operations/prepare "$raw_root/cancel-prepare.json" "$cancel_prepare"
cancel_attempt=$(jq -er '.attempt_id' "$raw_root/cancel-prepare.json")
cancel_generation=$(jq -er '.lease_generation' "$raw_root/cancel-prepare.json")
cancel_payload=$(jq -cn --arg operation_id "$cancel_operation" --arg thread_id "$cancel_thread" --arg attempt_id "$cancel_attempt" --argjson generation "$cancel_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,cancellation_code:"user_cancelled",cancellation_message:"Cancelled before completion."}')
api_request POST /v1/client-operations/cancel "$raw_root/cancel.json" "$cancel_payload"
jq -e '.status == "cancelled" and .cancellation_code == "user_cancelled" and (.assistant_observation_id // "") == ""' "$raw_root/cancel.json" >/dev/null
api_request POST /v1/client-operations/cancel "$raw_root/cancel-replay.json" "$cancel_payload"
jq -e '.status == "cancelled" and .replayed == true' "$raw_root/cancel-replay.json" >/dev/null
cancel_drift=$(jq -c '.cancellation_message = "different cancellation payload"' <<<"$cancel_payload")
[[ $(api_status POST /v1/client-operations/cancel "$raw_root/cancel-drift.json" "$cancel_drift") == "400" ]] || die "changed cancellation replay was accepted"
late_complete=$(jq -cn --arg operation_id "$cancel_operation" --arg thread_id "$cancel_thread" --arg attempt_id "$cancel_attempt" --argjson generation "$cancel_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,answer:"late answer",model:"openclaw/late"}')
[[ $(api_status POST /v1/client-operations/complete "$raw_root/cancel-late-complete.json" "$late_complete") == "409" ]] || die "late completion after cancellation was not rejected"

failure_operation="openclaw:w36-failure-$run_id"
failure_thread="agent:w36:failure"
failure_prepare=$(jq -cn --arg operation_id "$failure_operation" --arg thread_id "$failure_thread" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Stop this operation with a bounded failure."}')
api_request POST /v1/client-operations/prepare "$raw_root/failure-prepare.json" "$failure_prepare"
failure_attempt=$(jq -er '.attempt_id' "$raw_root/failure-prepare.json")
failure_generation=$(jq -er '.lease_generation' "$raw_root/failure-prepare.json")
failure_payload=$(jq -cn --arg operation_id "$failure_operation" --arg thread_id "$failure_thread" --arg attempt_id "$failure_attempt" --argjson generation "$failure_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,failure_code:"client_runtime_failed",failure_message:"The external client stopped before a visible answer."}')
api_request POST /v1/client-operations/fail "$raw_root/failure.json" "$failure_payload"
jq -e '.status == "failed" and .failure_code == "client_runtime_failed" and (.assistant_observation_id // "") == ""' "$raw_root/failure.json" >/dev/null
api_request POST /v1/client-operations/fail "$raw_root/failure-replay.json" "$failure_payload"
jq -e '.status == "failed" and .replayed == true' "$raw_root/failure-replay.json" >/dev/null
failure_drift=$(jq -c '.failure_message = "different failure payload"' <<<"$failure_payload")
[[ $(api_status POST /v1/client-operations/fail "$raw_root/failure-drift.json" "$failure_drift") == "400" ]] || die "changed failure replay was accepted"

race_operation="openclaw:w36-terminal-race-$run_id"
race_thread="agent:w36:terminal-race"
race_prepare=$(jq -cn --arg operation_id "$race_operation" --arg thread_id "$race_thread" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,message:"Race completion against cancellation."}')
api_request POST /v1/client-operations/prepare "$raw_root/race-prepare.json" "$race_prepare"
race_attempt=$(jq -er '.attempt_id' "$raw_root/race-prepare.json")
race_generation=$(jq -er '.lease_generation' "$raw_root/race-prepare.json")
race_complete=$(jq -cn --arg operation_id "$race_operation" --arg thread_id "$race_thread" --arg attempt_id "$race_attempt" --argjson generation "$race_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,answer:"The concurrent operation completed.",model:"openclaw/w36-race"}')
race_cancel=$(jq -cn --arg operation_id "$race_operation" --arg thread_id "$race_thread" --arg attempt_id "$race_attempt" --argjson generation "$race_generation" '{operation_id:$operation_id,channel:"openclaw",thread_id:$thread_id,attempt_id:$attempt_id,lease_generation:$generation,cancellation_code:"operator_cancelled",cancellation_message:"The operator cancelled concurrently."}')
api_status POST /v1/client-operations/complete "$raw_root/race-complete.json" "$race_complete" >"$raw_root/race-complete.status" &
race_complete_pid=$!
api_status POST /v1/client-operations/cancel "$raw_root/race-cancel.json" "$race_cancel" >"$raw_root/race-cancel.status" &
race_cancel_pid=$!
wait "$race_complete_pid"
wait "$race_cancel_pid"
race_statuses=$(sort "$raw_root/race-complete.status" "$raw_root/race-cancel.status" | paste -sd, -)
[[ "$race_statuses" == "200,409" ]] || die "terminal race did not produce one winner: $race_statuses"
race_terminal=$("$postgres_bin/psql" "$admin_url" -AtF '|' -v ON_ERROR_STOP=1 -c "SELECT status, COALESCE(assistant_observation_id::text, '') FROM conversation_turns WHERE tenant_id='$tenant_id' AND operation_id='$race_operation'")
[[ "$race_terminal" == completed\|* || "$race_terminal" == "cancelled|" ]] || die "terminal race stored invalid state: $race_terminal"
if [[ "$race_terminal" == completed\|* ]]; then
  api_request POST /v1/client-operations/complete "$raw_root/race-terminal-replay.json" "$race_complete"
else
  api_request POST /v1/client-operations/cancel "$raw_root/race-terminal-replay.json" "$race_cancel"
fi
jq -e '.replayed == true' "$raw_root/race-terminal-replay.json" >/dev/null

hermes_operation="hermes:w36-bounded-$run_id"
hermes_prepare=$(jq -cn --arg operation_id "$hermes_operation" '{operation_id:$operation_id,session_key:"session:w36:bounded",message:"Run the bounded compatibility control."}')
api_request POST /v1/integrations/hermes/turns/prepare "$raw_root/hermes-prepare.json" "$hermes_prepare"
hermes_complete=$(jq -cn --arg operation_id "$hermes_operation" '{operation_id:$operation_id,session_key:"session:w36:bounded",answer:"Hermes bounded control completed.",model:"hermes/w36-control"}')
api_request POST /v1/integrations/hermes/turns/complete "$raw_root/hermes-complete.json" "$hermes_complete"
jq -e '.status == "completed" and .protocol == "bounded_v1"' "$raw_root/hermes-complete.json" >/dev/null

hermes_failure_operation="hermes:w36-bounded-failure-$run_id"
hermes_failure_prepare=$(jq -cn --arg operation_id "$hermes_failure_operation" '{operation_id:$operation_id,session_key:"session:w36:bounded-failure",message:"Run the bounded failure compatibility control."}')
api_request POST /v1/integrations/hermes/turns/prepare "$raw_root/hermes-failure-prepare.json" "$hermes_failure_prepare"
hermes_fail=$(jq -cn --arg operation_id "$hermes_failure_operation" '{operation_id:$operation_id,session_key:"session:w36:bounded-failure",failure_code:"hermes_control_failed",failure_message:"Hermes bounded failure control."}')
api_request POST /v1/integrations/hermes/turns/fail "$raw_root/hermes-fail.json" "$hermes_fail"
jq -e '.status == "failed" and .protocol == "bounded_v1" and (.assistant_observation_id // "") == ""' "$raw_root/hermes-fail.json" >/dev/null

stop_service
stop_postgres
"$postgres_bin/pg_ctl" -D "$postgres_data" -l "$postgres_log" \
  -o "-p $postgres_port -k $postgres_socket -c listen_addresses=127.0.0.1 -c unix_socket_permissions=0700" start >/dev/null
for _ in $(seq 1 40); do
  if "$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
"$postgres_bin/psql" -h "$postgres_socket" -p "$postgres_port" -U postgres -d postgres -Atc 'SELECT 1' >/dev/null || die "PostgreSQL did not recover terminal state after restart"
start_service
api_request POST /v1/client-operations/prepare "$raw_root/openclaw-terminal-after-restart.json" "$openclaw_reclaim_payload"
jq -e --slurpfile first "$raw_root/openclaw-resume.json" '.status == "completed" and .turn_id == $first[0].turn_id and .delivery_id == $first[0].delivery_id and .attempt_id == $first[0].attempt_id and .checkpoint_sequence == $first[0].checkpoint_sequence' "$raw_root/openclaw-terminal-after-restart.json" >/dev/null
api_request POST /v1/client-operations/prepare "$raw_root/cancel-terminal-after-restart.json" "$cancel_prepare"
jq -e --slurpfile first "$raw_root/cancel.json" '.status == "cancelled" and .turn_id == $first[0].turn_id and .attempt_id == $first[0].attempt_id' "$raw_root/cancel-terminal-after-restart.json" >/dev/null
api_request POST /v1/client-operations/prepare "$raw_root/failure-terminal-after-restart.json" "$failure_prepare"
jq -e --slurpfile first "$raw_root/failure.json" '.status == "failed" and .turn_id == $first[0].turn_id and .attempt_id == $first[0].attempt_id' "$raw_root/failure-terminal-after-restart.json" >/dev/null

dump_path="$raw_root/w36.dump"
"$postgres_bin/pg_dump" --format=custom --no-owner --no-acl --file "$dump_path" "$admin_url"
dump_sha256=$(shasum -a 256 "$dump_path" | awk '{print $1}')
dump_bytes=$(stat -f '%z' "$dump_path")
"$postgres_bin/createdb" -h "$postgres_socket" -p "$postgres_port" -U postgres "$restore_database"
restore_url="postgresql:///$restore_database?host=$postgres_socket&port=$postgres_port&user=postgres&sslmode=disable"
restore_runtime_url="postgresql://$runtime_role:$runtime_password@127.0.0.1:$postgres_port/$restore_database?sslmode=disable"
"$postgres_bin/pg_restore" --exit-on-error --no-owner --no-acl --dbname "$restore_url" "$dump_path" >"$raw_root/restore.log"
"$binary" database migrate --database-url "$restore_url" >"$raw_root/restore-migrate.json"
"$binary" database grant-runtime --database-url "$restore_url" --role "$runtime_role" >"$raw_root/restore-grant-runtime.json"
"$binary" database compatibility --database-url "$restore_runtime_url" >"$raw_root/restore-compatibility.json"
jq -e '.status == "compatible" and .schema_version == 25' "$raw_root/restore-compatibility.json" >/dev/null

stop_service
active_runtime_url="$restore_runtime_url"
start_service
api_request POST /v1/client-operations/prepare "$raw_root/openclaw-terminal-after-restore.json" "$openclaw_reclaim_payload"
jq -e --slurpfile first "$raw_root/openclaw-resume.json" '.status == "completed" and .turn_id == $first[0].turn_id and .delivery_id == $first[0].delivery_id and .attempt_id == $first[0].attempt_id and .checkpoint_sequence == $first[0].checkpoint_sequence' "$raw_root/openclaw-terminal-after-restore.json" >/dev/null
api_request POST /v1/client-operations/prepare "$raw_root/cancel-terminal-after-restore.json" "$cancel_prepare"
jq -e --slurpfile first "$raw_root/cancel.json" '.status == "cancelled" and .turn_id == $first[0].turn_id and .attempt_id == $first[0].attempt_id' "$raw_root/cancel-terminal-after-restore.json" >/dev/null
api_request POST /v1/client-operations/prepare "$raw_root/failure-terminal-after-restore.json" "$failure_prepare"
jq -e --slurpfile first "$raw_root/failure.json" '.status == "failed" and .turn_id == $first[0].turn_id and .attempt_id == $first[0].attempt_id' "$raw_root/failure-terminal-after-restore.json" >/dev/null
admin_url="$restore_url"

semantic_absence_count=$("$postgres_bin/psql" "$admin_url" -At -v ON_ERROR_STOP=1 -c "WITH silent_turns AS (SELECT continuity_id FROM conversation_turns WHERE tenant_id='$tenant_id' AND operation_id IN ('$cancel_operation','$failure_operation','$other_continuity_operation')) SELECT (SELECT count(*) FROM observations WHERE tenant_id='$tenant_id' AND operation_id IN ('$cancel_operation:assistant','$failure_operation:assistant','$other_continuity_operation:assistant')) + (SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id='$tenant_id' AND continuity_id IN (SELECT continuity_id FROM silent_turns)) + (SELECT count(*) FROM governed_memories WHERE tenant_id='$tenant_id' AND continuity_id IN (SELECT continuity_id FROM silent_turns)) + (SELECT count(*) FROM memory_search_documents WHERE tenant_id='$tenant_id' AND continuity_id IN (SELECT continuity_id FROM silent_turns)) + (SELECT count(*) FROM memory_vector_documents WHERE tenant_id='$tenant_id' AND continuity_id IN (SELECT continuity_id FROM silent_turns)) + (SELECT count(*) FROM memory_vector_documents_2560 WHERE tenant_id='$tenant_id' AND continuity_id IN (SELECT continuity_id FROM silent_turns))")
[[ "$semantic_absence_count" == "0" ]] || die "failure or cancellation created semantic writeback: $semantic_absence_count"

counts=$("$postgres_bin/psql" "$admin_url" -At -v ON_ERROR_STOP=1 -c "SELECT json_build_object('leased_turns',(SELECT count(*) FROM conversation_turns WHERE tenant_id='$tenant_id' AND operation_protocol='leased_v1'),'completed_turns',(SELECT count(*) FROM conversation_turns WHERE tenant_id='$tenant_id' AND status='completed'),'failed_turns',(SELECT count(*) FROM conversation_turns WHERE tenant_id='$tenant_id' AND status='failed'),'cancelled_turns',(SELECT count(*) FROM conversation_turns WHERE tenant_id='$tenant_id' AND status='cancelled'),'assistant_observations',(SELECT count(*) FROM observations WHERE tenant_id='$tenant_id' AND observation_kind='assistant_message'),'tool_result_observations',(SELECT count(*) FROM observations WHERE tenant_id='$tenant_id' AND observation_kind='tool_result'),'formation_schedules',(SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id='$tenant_id'),'race_assistant_observations',(SELECT count(*) FROM observations WHERE tenant_id='$tenant_id' AND operation_id='$race_operation:assistant'),'race_formation_schedules',(SELECT count(*) FROM conversation_formation_schedules WHERE tenant_id='$tenant_id' AND continuity_id=(SELECT continuity_id FROM conversation_turns WHERE tenant_id='$tenant_id' AND operation_id='$race_operation')))::text")
jq -e '.leased_turns == 6 and .failed_turns == 3 and .tool_result_observations == 3 and (.completed_turns + .failed_turns + .cancelled_turns) == 8 and .assistant_observations == .completed_turns and .formation_schedules == .completed_turns and ((.race_assistant_observations == 1 and .race_formation_schedules == 1) or (.race_assistant_observations == 0 and .race_formation_schedules == 0))' <<<"$counts" >/dev/null || die "unexpected authoritative counts: $counts"

revision=$("$binary" version | jq -er '.revision')
case_sha256=$(shasum -a 256 "$case_file" | awk '{print $1}')
hard_gates=$(jq -c '.hard_gates' "$case_file")
jq -n \
  --arg run_id "$run_id" \
  --arg revision "$revision" \
  --arg case_sha256 "$case_sha256" \
  --arg dump_sha256 "$dump_sha256" \
  --argjson dump_bytes "$dump_bytes" \
  --argjson counts "$counts" \
  --argjson gates "$hard_gates" \
  '{
    schema_version:"w36-runtime-evidence/v1",
    run_id:$run_id,
    case_id:"W36-long-running-client-operation",
    implementation_revision:$revision,
    case_sha256:$case_sha256,
    status:"runtime-qualified",
    protocol:"leased_v1",
    client_surfaces:{openclaw_plugin_hooks:true,openclaw_client_module:true,hermes_bounded_control:true},
    process_restart_count:2,
    postgres_restart_count:2,
    native_dump_restore:{passed:true,sha256:$dump_sha256,bytes:$dump_bytes,restored_token_authenticated:true},
    authoritative_counts:$counts,
    hard_gate_count:($gates | length),
    hard_gates:($gates | map({assertion:.,passed:true})),
    evidence_integrity:{credentials_persisted:false,temporary_database_removed_on_exit:true}
  }' >"$report"

if grep -R -aFq "$api_token" "$evidence_root" || grep -R -aFq "$operator_token" "$evidence_root" || \
  grep -R -aFq "$attacker_token" "$evidence_root" || grep -R -aFq "$runtime_password" "$evidence_root"; then
  die "credential entered normalized report"
fi
printf 'W36: runtime-qualified report=%s\n' "$report"
