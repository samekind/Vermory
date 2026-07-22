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
grep -Fq '| `I05-durable-linux-service-lifecycle` | `contract-only` |' "$matrix"
grep -Fq '[Capability And Evidence Matrix](docs/capability-evidence-matrix.md)' README.md
grep -Fq '[能力与证据矩阵](docs/capability-evidence-matrix.md)' README.zh-CN.md
grep -Fq '[Capability And Evidence Matrix](docs/capability-evidence-matrix.md)' ARCHITECTURE.md

echo "capability matrix: pass ($case_count cases)"
