#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

snapshot="docs/evidence/snapshots/2026-07-22-three-client-conversation-bridge-real-clients.json"
evidence="docs/evidence/2026-07-22-three-client-conversation-bridge-real-clients.md"
matrix="docs/capability-evidence-matrix.md"

for path in "$snapshot" "$evidence" "$matrix"; do
  [[ -f "$path" ]] || {
    echo "B03 evidence: missing $path" >&2
    exit 1
  }
done

jq -e '
  .case_id == "B03-three-client-conversation-bridge" and
  .status == "client-qualified" and
  .hard_gates.passed == 30 and
  .hard_gates.total == 30 and
  (.hard_gates.results | length) == 30 and
  ([.hard_gates.results[]] | all) and
  .implementation.revision == "82609f095c32db1935d27ecb569557847a78c337" and
  .deployment.postgresql == "18.3" and
  .deployment.schema_version == 23 and
  .deployment.loopback_only == true and
  .deployment.sudo_used == false and
  .clients.web_chat.real_model_turns == 3 and
  .clients.web_chat.source_active_memories == 1 and
  .clients.web_chat.unrelated_active_memories == 0 and
  .clients.hermes.accepted_real_model_turns == 4 and
  .clients.hermes.primary_session_observations_after_reversal == 8 and
  .clients.hermes.unrelated_session_observations == 2 and
  .clients.hermes.prelink_answer == "NO_LINKED_BUNDLE" and
  .clients.hermes.linked_answer == "vermory-v8.tgz" and
  .clients.hermes.unrelated_answer == "NO_LINKED_BUNDLE" and
  .clients.hermes.postreverse_answer == "NO_LINKED_BUNDLE" and
  .clients.openclaw.accepted_surface == "Gateway OpenAI-compatible HTTP agent path" and
  .clients.openclaw.accepted_real_model_turns == 4 and
  .clients.openclaw.primary_session_observations_after_reversal == 10 and
  .clients.openclaw.unrelated_session_observations == 2 and
  .clients.openclaw.prelink_answer == "NO_LINKED_BUNDLE" and
  .clients.openclaw.linked_answer == "vermory-v8.tgz" and
  .clients.openclaw.unrelated_answer == "NO_LINKED_BUNDLE" and
  .clients.openclaw.postreverse_answer == "NO_LINKED_BUNDLE" and
  .clients.openclaw.embedded_fallback_accepted == false and
  .model_execution.accepted_real_model_turns == 11 and
  .model_execution.logged_successful_requests == 14 and
  .model_execution.logged_failed_requests == 0 and
  .model_execution.newapi_used == false and
  .model_execution.relay_persisted_as_product_provider == false and
  .protected_input.github_actions_run == 29903259886 and
  .protected_input.signed_artifact_id == 8522876048 and
  .protected_input.signed_artifact_name == "vermory-pr-snapshot-82609f095c32db1935d27ecb569557847a78c337" and
  .protected_input.darwin_arm64_archive_sha256 == "fbeff2afce1d9f9068def55f869478008cc0de2b6205b941757a6885cc408163" and
  .database_assertions.external_turns == 11 and
  .database_assertions.external_completed_turns == 11 and
  .database_assertions.linked_bundle_deliveries == 2 and
  .database_assertions.external_raw_marker_deliveries == 0 and
  .database_assertions.external_unrelated_marker_deliveries == 0 and
  .database_assertions.postreverse_nonempty_deliveries == 0 and
  .database_assertions.source_active_memories == 1 and
  .database_assertions.retained_server_failed_turns == 3 and
  .bridge_assertions.bridge_operations == 2 and
  .bridge_assertions.bridge_events == 4 and
  .bridge_assertions.hermes_replay_same_bridge_id == true and
  .bridge_assertions.hermes_replay_reported_replayed == true and
  .bridge_assertions.active_conversation_links_after_reversal == 0 and
  .bridge_assertions.reversed_conversation_links == 2 and
  .bridge_assertions.source_memory_retained == true and
  .bridge_assertions.external_transcripts_retained == true and
  .failure_ledger.retained_server_provider_failures == 3 and
  .failure_ledger.rejected_openclaw_embedded_fallback_attempts == 1 and
  .privacy.raw_transcripts_committed == false and
  .privacy.provider_bodies_committed == false and
  .privacy.credentials_committed == false and
  .privacy.database_urls_committed == false and
  .privacy.private_host_references_committed == false and
  .privacy.temporary_relay_source_committed == false
' "$snapshot" >/dev/null

grep -Fq '| `B03-three-client-conversation-bridge` | `client-qualified` |' "$matrix"
grep -Fq '[Three-client real-client run](evidence/2026-07-22-three-client-conversation-bridge-real-clients.md)' "$matrix"
grep -Fq '[B03 real-client snapshot](snapshots/2026-07-22-three-client-conversation-bridge-real-clients.json)' "$evidence"

if rg -n 'sk-[A-Za-z0-9]{12,}|postgresql://|frp\.|/Users/[A-Za-z0-9_.-]+|openclaw-gateway-token' "$snapshot" "$evidence"; then
  echo "B03 evidence: private route or credential-like content found" >&2
  exit 1
fi

echo "B03 real-client evidence: pass (30/30)"
