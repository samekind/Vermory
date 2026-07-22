#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

matrix="docs/capability-evidence-matrix.md"
[[ -f "$matrix" ]] || {
  echo "capability matrix: missing $matrix" >&2
  exit 1
}

case_list="$(mktemp "${TMPDIR:-/tmp}/vermory-capability-cases.XXXXXX")"
trap 'rm -f "$case_list"' EXIT

find reality/cases -mindepth 2 -maxdepth 2 -name manifest.json -print |
  sort |
  while IFS= read -r manifest; do
    jq -r '.id' "$manifest"
  done >"$case_list"

case_count="$(wc -l <"$case_list" | tr -d ' ')"
[[ "$case_count" -eq 21 ]] || {
  echo "capability matrix: expected 21 frozen cases, found $case_count" >&2
  exit 1
}

grep -Fq 'Frozen public reality cases: `21`.' "$matrix"

while IFS= read -r case_id; do
  count="$(grep -Fc "| \`$case_id\` |" "$matrix")"
  [[ "$count" -eq 1 ]] || {
    echo "capability matrix: expected one row for $case_id, found $count" >&2
    exit 1
  }
done <"$case_list"

check_line_count() {
  local line="$1"
  local label="$2"
  local count
  count="$(jq -s --arg line "$line" '[.[] | select(.continuity_lines | index($line))] | length' reality/cases/*/manifest.json)"
  grep -Fq "| $label | $count |" "$matrix" || {
    echo "capability matrix: $label count drifted from $count" >&2
    exit 1
  }
}

check_line_count workspace Workspace
check_line_count conversation Conversation
check_line_count global_defaults "Global Defaults"
check_line_count bridge Bridge
check_line_count security Security

while IFS= read -r relative_path; do
  [[ -f "docs/$relative_path" ]] || {
    echo "capability matrix: broken evidence link docs/$relative_path" >&2
    exit 1
  }
done < <(rg -o '\]\((evidence/[^)#]+\.md)\)' "$matrix" | sed -E 's/^.*\]\(([^)]+)\)$/\1/' | sort -u)

grep -Fq '| `B03-three-client-conversation-bridge` | `client-qualified` |' "$matrix"
grep -Fq '| `W04-canonical-repository-cross-client` | `external-blocked` |' "$matrix"
grep -Fq '[remote Git topology](evidence/2026-07-22-remote-git-workspace-topology.md)' "$matrix"
grep -Fq '| `I05-durable-linux-service-lifecycle` | `runtime-qualified` |' "$matrix"
i05_snapshot="docs/evidence/snapshots/2026-07-22-durable-linux-service-lifecycle.json"
jq -e '
  .github.repository == "samekind/Vermory" and
  .github.run_id == 29916232568 and
  .github.job_id == 88910623425 and
  .github.head_sha == .report.source_sha and
  .github.artifact_sha256 == .github.downloaded_zip_sha256 and
  .report.case_id == "I05-durable-linux-service-lifecycle" and
  .report.qualification == "github-hosted-ubuntu-systemd-amd64" and
  (.report.hard_gates | length == 16) and
  (.report.hard_gates | to_entries | all(.value == true))
' "$i05_snapshot" >/dev/null
i05_arm64_snapshot="docs/evidence/snapshots/2026-07-22-durable-linux-service-lifecycle-arm64.json"
jq -e '
  .github.repository == "samekind/Vermory" and
  .github.run_id == 29918264712 and
  .github.job_id == 88917132816 and
  .github.artifact_id == 8528827014 and
  .github.head_sha == .report.source_sha and
  .github.artifact_sha256 == "983c9b32c6d0291978a6c52e5676dc8d685b3f5cb62d5eb0a879a56bfebe10cb" and
  .github.report_sha256 == "f41fe1d54d035beb3d6cc6509bf0e15cb3d13a67c095feb694be02476bd84fcf" and
  .github.artifact_sha256 == .github.downloaded_zip_sha256 and
  .report.case_id == "I05-durable-linux-service-lifecycle" and
  .report.qualification == "github-hosted-ubuntu-systemd-arm64" and
  .report.runtime.machine == "aarch64" and
  .report.runtime.goarch == "arm64" and
  .report.runtime.native == true and
  (.report.hard_gates | length == 16) and
  (.report.hard_gates | to_entries | all(.value == true))
' "$i05_arm64_snapshot" >/dev/null
grep -Fq '| `I06-linux-native-packages` | `runtime-qualified` |' "$matrix"
i06_snapshot="docs/evidence/snapshots/2026-07-22-linux-native-packages.json"
jq -e '
  .source_head as $source_head |
  .case_id == "I06-linux-native-packages" and
  .status == "runtime-qualified" and
  .github.repository == "samekind/Vermory" and
  .github.run_id == 29923986517 and
  .github.run_conclusion == "success" and
  .github.sign_snapshot_job_id == 88937403534 and
  (.package_legs | length == 4) and
  (.package_legs | map(.report.package.format + "/" + .report.package.architecture) | sort == ["deb/amd64", "deb/arm64", "rpm/amd64", "rpm/arm64"]) and
  (.package_legs | all(
    .artifact_sha256 == .downloaded_zip_sha256 and
    .signed_snapshot_hash_match == true and
    .report.case_id == "I06-linux-native-packages" and
    .report.source_sha == $source_head and
    .report.package.native == true and
    (.report.hard_gates | length == 16) and
    (.report.hard_gates | to_entries | all(.value == true))
  )) and
  .signed_snapshot.artifact_id == 8531264312 and
  .signed_snapshot.artifact_sha256 == .signed_snapshot.downloaded_zip_sha256 and
  .signed_snapshot.manifest_entries == 12 and
  .signed_snapshot.checksums_entries == 8 and
  .signed_snapshot.all_payload_hashes_verified == true and
  .signed_snapshot.all_accepted_package_hashes_match == true and
  .signed_snapshot.sigstore.workflow_identity == "https://github.com/samekind/Vermory/.github/workflows/ci.yml@refs/pull/1/merge" and
  .signed_snapshot.sigstore.positive_verification == "pass" and
  .signed_snapshot.sigstore.modified_manifest_rejection == "pass" and
  .signed_snapshot.sigstore.wrong_identity_rejection == "pass" and
  (.retained_failures | map(.run_id) == [29922576223, 29922904137])
' "$i06_snapshot" >/dev/null
grep -Fq '[Capability And Evidence Matrix](docs/capability-evidence-matrix.md)' README.md
grep -Fq '[能力与证据矩阵](docs/capability-evidence-matrix.md)' README.zh-CN.md
grep -Fq '[Capability And Evidence Matrix](docs/capability-evidence-matrix.md)' ARCHITECTURE.md

echo "capability matrix: pass ($case_count cases)"
