#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: cursor-client-qualification.sh \
  --cursor-agent /path/to/cursor-agent \
  --workspace /absolute/disposable/workspace \
  --mcp-command /absolute/temporary/mcp-wrapper \
  --ledger-command /absolute/temporary/ledger-wrapper \
  --artifact-root /absolute/evidence/root \
  [--run-id id] [--model model]

The workspace must be disposable and must not already contain .cursor/mcp.json.
The MCP wrapper must not print credentials or diagnostics to stdout.
EOF
  exit 2
}

CURSOR_AGENT=""
WORKSPACE=""
MCP_COMMAND=""
LEDGER_COMMAND=""
ARTIFACT_ROOT=""
RUN_ID=""
MODEL="gpt-5.3-codex"
SERVER="vermory-w25"

while (($# > 0)); do
  case "$1" in
    --cursor-agent) CURSOR_AGENT="$2"; shift 2 ;;
    --workspace) WORKSPACE="$2"; shift 2 ;;
    --mcp-command) MCP_COMMAND="$2"; shift 2 ;;
    --ledger-command) LEDGER_COMMAND="$2"; shift 2 ;;
    --artifact-root) ARTIFACT_ROOT="$2"; shift 2 ;;
    --run-id) RUN_ID="$2"; shift 2 ;;
    --model) MODEL="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; usage ;;
  esac
done

if [[ -z "$CURSOR_AGENT" || -z "$MCP_COMMAND" || -z "$LEDGER_COMMAND" || -z "$WORKSPACE" || -z "$ARTIFACT_ROOT" ]]; then
  printf 'all required arguments must be provided\n' >&2
  usage
fi
for path_arg in "$WORKSPACE" "$ARTIFACT_ROOT"; do
  [[ "$path_arg" == /* ]] || { printf 'paths must be absolute\n' >&2; exit 2; }
done
[[ -x "$CURSOR_AGENT" ]] || { printf 'cursor agent is not executable\n' >&2; exit 2; }
[[ -x "$MCP_COMMAND" ]] || { printf 'MCP command is not executable\n' >&2; exit 2; }
[[ -x "$LEDGER_COMMAND" ]] || { printf 'ledger command is not executable\n' >&2; exit 2; }
[[ -d "$WORKSPACE" ]] || { printf 'workspace does not exist\n' >&2; exit 2; }
[[ ! -e "$WORKSPACE/.cursor/mcp.json" ]] || { printf 'refusing to overwrite project MCP configuration\n' >&2; exit 2; }
if [[ -z "$RUN_ID" ]]; then
  RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
fi
[[ "$RUN_ID" =~ ^[A-Za-z0-9._-]{1,48}$ ]] || { printf 'invalid run id\n' >&2; exit 2; }
[[ "$MODEL" =~ ^[A-Za-z0-9._:/-]+$ ]] || { printf 'invalid model\n' >&2; exit 2; }
[[ ! -e "$WORKSPACE/continuity-report.md" ]] || { printf 'refusing to reuse an existing qualification artifact\n' >&2; exit 2; }

CASE_DIR="$(cd "$(dirname "$0")/.." && pwd)/reality/cases/W04-canonical-repository-cross-client"
CASE_PROMPT="$(jq -er '.task.prompt' "$CASE_DIR/manifest.json")"
RUN_DIR="$ARTIFACT_ROOT/$RUN_ID"
[[ ! -e "$RUN_DIR" ]] || { printf 'refusing to overwrite run directory\n' >&2; exit 2; }
umask 077
mkdir -p "$RUN_DIR" "$WORKSPACE/.cursor"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PREPARE_OPERATION_ID="w25-$RUN_ID-prepare"
OBSERVATION_OPERATION_ID="w25-$RUN_ID-observation"

cleanup() {
  if [[ "${APPROVAL_TOUCHED:-false}" == true ]]; then
    (cd "$WORKSPACE" && "$CURSOR_AGENT" mcp disable "$SERVER") >/dev/null 2>&1 || true
  fi
  rm -f "$WORKSPACE/.cursor/mcp.json"
}
trap cleanup EXIT

jq -n --arg command "$MCP_COMMAND" --arg server "$SERVER" \
  '{mcpServers:{($server):{command:$command,args:[]}}}' \
  > "$WORKSPACE/.cursor/mcp.json"

version_output="$("$CURSOR_AGENT" --version 2>&1 || true)"
printf '%s\n' "$version_output" | tr -cd '0-9A-Za-z._-\n' | head -1 > "$RUN_DIR/client-version.txt"

logged_in=false
status_output="$($CURSOR_AGENT status 2>"$RUN_DIR/status.stderr" || true)"
if grep -q 'Logged in as' <<<"$status_output"; then
  logged_in=true
fi
model_available=false
if "$CURSOR_AGENT" models >"$RUN_DIR/models.raw" 2>"$RUN_DIR/models.stderr"; then
  grep -Fq "$MODEL -" "$RUN_DIR/models.raw" && model_available=true || true
fi

APPROVAL_TOUCHED=false
if (cd "$WORKSPACE" && "$CURSOR_AGENT" mcp enable "$SERVER") >"$RUN_DIR/mcp-enable.raw" 2>"$RUN_DIR/mcp-enable.stderr"; then
  APPROVAL_TOUCHED=true
fi

set +e
(cd "$WORKSPACE" && "$CURSOR_AGENT" mcp list) >"$RUN_DIR/mcp-list.raw" 2>"$RUN_DIR/mcp-list.stderr"
mcp_list_rc=$?
(cd "$WORKSPACE" && "$CURSOR_AGENT" mcp list-tools "$SERVER") >"$RUN_DIR/mcp-tools.raw" 2>"$RUN_DIR/mcp-tools.stderr"
mcp_tools_rc=$?
set -e

mcp_ready=false
[[ "$mcp_list_rc" == 0 ]] && grep -Fqx "$SERVER: ready" "$RUN_DIR/mcp-list.raw" && mcp_ready=true || true
mcp_tools_valid=false
if [[ "$mcp_tools_rc" == 0 ]] && grep -Fq 'prepare_context' "$RUN_DIR/mcp-tools.raw" && grep -Fq 'commit_observation' "$RUN_DIR/mcp-tools.raw"; then
  if ! grep -Eiq 'tenant|binding|authority|supersede|target_memory|kind' "$RUN_DIR/mcp-tools.raw"; then
    mcp_tools_valid=true
  fi
fi

set +e
"$LEDGER_COMMAND" "$PREPARE_OPERATION_ID" "$OBSERVATION_OPERATION_ID" >"$RUN_DIR/ledger-before.json" 2>"$RUN_DIR/ledger-before.stderr"
ledger_before_rc=$?
set -e
preflight_valid=false
if [[ "$ledger_before_rc" == 0 ]] && jq -e '
  .deliveries == 0 and
  .agent_results == 0 and
  .proposed_memories == 0 and
  .active_agent_memories == 0 and
  .canonical_delivered == 0 and
  .marker_delivered == 0 and
  .stale_delivered == 0 and
  .distractor_delivered == 0 and
  .source_ref_matches == 0
' "$RUN_DIR/ledger-before.json" >/dev/null; then
  preflight_valid=true
fi

prompt="$(cat <<EOF
Execute the frozen W25 Cursor Agent real-client qualification in this exact workspace.

First call the $SERVER.prepare_context MCP tool with:
- operation_id: $PREPARE_OPERATION_ID
- repo_root: $WORKSPACE
- task: $CASE_PROMPT

Use only the returned governed context. Do not inspect git remotes, repository history, the Vermory source tree, or the W04 case. Do not guess values that are not in governed context. If the tool does not return resolved status, stop without writing an artifact.

When resolved, create continuity-report.md in the workspace. It must contain exactly two semantic lines named canonical_repository and continuation_marker, populated only from governed context. Verify that the report contains the current values and does not contain a superseded personal repository or an unrelated same-name workspace marker. Then call $SERVER.commit_observation with:
- operation_id: $OBSERVATION_OPERATION_ID
- the delivery_id returned by prepare_context
- content: W25 created and verified continuity-report.md.
- source_ref: artifact:continuity-report.md

Call commit_observation a second time with the exact same arguments and verify
that the second response reports replayed=true.

Do not attempt to accept, correct, delete, rebind, or otherwise govern the observation.
EOF
)"
printf '%s' "$prompt" | shasum -a 256 | awk '{print $1}' > "$RUN_DIR/prompt-sha256.txt"

if [[ "$preflight_valid" == true ]]; then
  set +e
  (cd "$WORKSPACE" && "$CURSOR_AGENT" --print --output-format stream-json --trust --auto-review --sandbox enabled --approve-mcps --model "$MODEL" --workspace "$WORKSPACE" "$prompt") \
    >"$RUN_DIR/client-stream.jsonl" 2>"$RUN_DIR/client.stderr"
  client_rc=$?
  set -e
else
  client_rc=1
  : >"$RUN_DIR/client-stream.jsonl"
  printf 'qualification authority preflight is not empty\n' >"$RUN_DIR/client.stderr"
fi
printf '%s\n' "$client_rc" > "$RUN_DIR/client-exit-status.txt"

artifact_present=false
artifact_valid=false
artifact_sha256=""
if [[ -f "$WORKSPACE/continuity-report.md" ]]; then
  artifact_present=true
  artifact_sha256="$(shasum -a 256 "$WORKSPACE/continuity-report.md" | awk '{print $1}')"
  cp "$WORKSPACE/continuity-report.md" "$RUN_DIR/continuity-report.md"
  if grep -Fxq 'canonical_repository=https://github.com/samekind/Vermory' "$WORKSPACE/continuity-report.md" &&
    grep -Fxq 'continuation_marker=samekind-w25-current' "$WORKSPACE/continuity-report.md" &&
    ! grep -Fq 'https://github.com/jstar0/Vermory' "$WORKSPACE/continuity-report.md" &&
    ! grep -Fq 'distractor-w25-only' "$WORKSPACE/continuity-report.md" &&
    [[ "$(awk 'NF { count++ } END { print count + 0 }' "$WORKSPACE/continuity-report.md")" == 2 ]]; then
    artifact_valid=true
  fi
fi

set +e
"$LEDGER_COMMAND" "$PREPARE_OPERATION_ID" "$OBSERVATION_OPERATION_ID" >"$RUN_DIR/ledger.json" 2>"$RUN_DIR/ledger.stderr"
ledger_rc=$?
set -e
ledger_valid=false
if [[ "$ledger_rc" == 0 ]] && jq -e '
  .deliveries == 1 and
  .agent_results == 1 and
  .proposed_memories == 1 and
  .active_agent_memories == 0 and
  .canonical_delivered == 1 and
  .marker_delivered == 1 and
  .stale_delivered == 0 and
  .distractor_delivered == 0 and
  .source_ref_matches == 1
' "$RUN_DIR/ledger.json" >/dev/null; then
  ledger_valid=true
fi
replay_seen=false
if grep -Eq '"replayed"[[:space:]]*:[[:space:]]*true|\\"replayed\\"[[:space:]]*:[[:space:]]*true' "$RUN_DIR/client-stream.jsonl"; then
  replay_seen=true
fi

failure_class=""
if [[ "$preflight_valid" != true ]]; then
  failure_class="authority_preflight_failed"
elif [[ "$client_rc" != 0 ]]; then
  if grep -Eiq 'unpaid invoice|pay your invoice|stripe' "$RUN_DIR/client.stderr"; then
    failure_class="client_account_blocked"
  elif grep -Eiq 'quota|rate limit|provider|request failed' "$RUN_DIR/client.stderr"; then
    failure_class="client_quota_or_provider_failed"
  elif grep -Eiq 'MCP|mcp' "$RUN_DIR/client.stderr"; then
    failure_class="mcp_or_client_runtime_failed"
  else
    failure_class="client_generation_failed"
  fi
elif [[ "$mcp_list_rc" != 0 || "$mcp_tools_rc" != 0 || "$mcp_ready" != true || "$mcp_tools_valid" != true ]]; then
  failure_class="mcp_discovery_failed"
elif [[ "$artifact_present" != true || "$artifact_valid" != true ]]; then
  failure_class="artifact_failed"
elif [[ "$ledger_valid" != true || "$replay_seen" != true ]]; then
  failure_class="governance_boundary_failed"
fi

pass=false
if [[ "$preflight_valid" == true && "$client_rc" == 0 && "$mcp_ready" == true && "$mcp_tools_valid" == true && "$artifact_valid" == true && "$ledger_valid" == true && "$replay_seen" == true ]]; then
  pass=true
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg started_at "$started_at" \
  --arg server "$SERVER" \
  --arg model "$MODEL" \
  --arg prepare_operation_id "$PREPARE_OPERATION_ID" \
  --arg observation_operation_id "$OBSERVATION_OPERATION_ID" \
  --arg workspace_name "$(basename "$WORKSPACE")" \
  --arg prompt_sha256 "$(cat "$RUN_DIR/prompt-sha256.txt")" \
  --arg client_version "$(cat "$RUN_DIR/client-version.txt")" \
  --arg failure_class "$failure_class" \
  --arg artifact_sha256 "$artifact_sha256" \
  --argjson logged_in "$logged_in" \
  --argjson model_available "$model_available" \
  --argjson mcp_ready "$mcp_ready" \
  --argjson mcp_tools_valid "$mcp_tools_valid" \
  --argjson mcp_list_exit "$mcp_list_rc" \
  --argjson mcp_tools_exit "$mcp_tools_rc" \
  --argjson ledger_before_exit "$ledger_before_rc" \
  --argjson preflight_valid "$preflight_valid" \
  --argjson client_exit "$client_rc" \
  --argjson artifact_present "$artifact_present" \
  --argjson artifact_valid "$artifact_valid" \
  --argjson ledger_exit "$ledger_rc" \
  --argjson ledger_valid "$ledger_valid" \
  --argjson replay_seen "$replay_seen" \
  --argjson pass "$pass" \
  '{version:1,run_id:$run_id,started_at:$started_at,client:"cursor-agent",client_version:$client_version,model:$model,prepare_operation_id:$prepare_operation_id,observation_operation_id:$observation_operation_id,workspace_name:$workspace_name,server:$server,prompt_sha256:$prompt_sha256,logged_in:$logged_in,model_available:$model_available,mcp_ready:$mcp_ready,mcp_tools_valid:$mcp_tools_valid,mcp_list_exit:$mcp_list_exit,mcp_tools_exit:$mcp_tools_exit,ledger_before_exit:$ledger_before_exit,preflight_valid:$preflight_valid,client_exit:$client_exit,artifact_present:$artifact_present,artifact_valid:$artifact_valid,artifact_sha256:$artifact_sha256,ledger_exit:$ledger_exit,ledger_valid:$ledger_valid,replay_seen:$replay_seen,pass:$pass,failure_class:$failure_class}' \
  > "$RUN_DIR/summary.json"

printf 'run=%s client_exit=%s preflight=%s mcp_ready=%s artifact_valid=%s ledger_valid=%s replay_seen=%s pass=%s summary=%s\n' \
  "$RUN_ID" "$client_rc" "$preflight_valid" "$mcp_ready" "$artifact_valid" "$ledger_valid" "$replay_seen" "$pass" "$RUN_DIR/summary.json"

if [[ "$pass" != true ]]; then
  if [[ "$client_rc" != 0 ]]; then
    exit "$client_rc"
  fi
  exit 1
fi
