#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

required_files=(
  AGENTS.md
  ARCHITECTURE.md
  CODE_OF_CONDUCT.md
  CONTRIBUTING.md
  DEVELOPMENT.md
  GOVERNANCE.md
  LICENSE
  README.md
  README.zh-CN.md
  SECURITY.md
  .github/CODEOWNERS
  .github/dependabot.yml
  .github/pull_request_template.md
  .github/ISSUE_TEMPLATE/bug_report.yml
  .github/ISSUE_TEMPLATE/capability_proposal.yml
  .github/ISSUE_TEMPLATE/reality_case.yml
  .github/ISSUE_TEMPLATE/config.yml
  docs/capability-evidence-matrix.md
  docs/collaboration/repository-workflow.md
  scripts/capability-matrix-check.sh
  scripts/b03-real-client-evidence-check.sh
)

for path in "${required_files[@]}"; do
  [[ -f "$path" ]] || {
    echo "repository policy: missing $path" >&2
    exit 1
  }
done

grep -Fq '* @jstar0' .github/CODEOWNERS
grep -Fq 'PostgreSQL remains the only native semantic authority.' .github/pull_request_template.md
grep -Fq 'This pull request does not prove:' .github/pull_request_template.md
grep -Fq 'blank_issues_enabled: false' .github/ISSUE_TEMPLATE/config.yml
grep -Fq 'sign-snapshot:' .github/workflows/ci.yml
grep -Fq 'id-token: write' .github/workflows/ci.yml
grep -Fq 'cursor-agent' docs/superpowers/specs/2026-07-11-vermory-reality-program.md
bash scripts/capability-matrix-check.sh
bash scripts/b03-real-client-evidence-check.sh

scan_file="${TMPDIR:-/tmp}/vermory-repository-secret-scan.$$"
trap 'rm -f "$scan_file"' EXIT

if rg -n 'sk-[A-Za-z0-9]{12,}|BEGIN [A-Z ]*PRIVATE KEY|github_pat_[A-Za-z0-9_]+|gh[opusr]_[A-Za-z0-9]+' \
  AGENTS.md ARCHITECTURE.md CODE_OF_CONDUCT.md CONTRIBUTING.md DEVELOPMENT.md \
  GOVERNANCE.md SECURITY.md .github docs/collaboration >"$scan_file"; then
  cat "$scan_file" >&2
  echo "repository policy: credential-like content found" >&2
  exit 1
fi

echo "repository policy: pass"
