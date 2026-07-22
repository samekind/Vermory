#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "qualified repository assembly: $*" >&2
  exit 1
}

[[ $# -eq 3 ]] || fail "usage: $0 <evidence-root> <dist-directory> <source-sha>"

evidence_root=$1
dist=$2
source_sha=$3

[[ -d "$evidence_root" && -d "$dist" ]] || fail "evidence and dist directories are required"
[[ "$source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "source SHA must contain 40 lowercase hexadecimal characters"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

specs=(
  "apt deb amd64 x86_64"
  "dnf rpm amd64 x86_64"
  "apt deb arm64 aarch64"
  "dnf rpm arm64 aarch64"
)

downloaded_artifacts=()
for artifact in "$evidence_root"/vermory-i08-linux-repository-*-"$source_sha"; do
  [[ -e "$artifact" ]] || continue
  downloaded_artifacts+=("$artifact")
done
[[ ${#downloaded_artifacts[@]} -eq 4 ]] || fail "expected exactly four native repository artifacts"

accepted_paths=()
accepted_names=()
for spec in "${specs[@]}"; do
  read -r repository_kind package_format architecture machine <<<"$spec"
  artifact=$evidence_root/vermory-i08-linux-repository-$repository_kind-$architecture-$source_sha
  report=$artifact/report.json
  [[ -d "$artifact" && -f "$report" ]] || fail "missing $repository_kind/$architecture acceptance report"

  jq -e \
    --arg source_sha "$source_sha" \
    --arg repository_kind "$repository_kind" \
    --arg package_format "$package_format" \
    --arg architecture "$architecture" \
    --arg machine "$machine" \
    '.version == "1" and
     .case_id == "I08-linux-package-repository" and
     .source_sha == $source_sha and
     .repository.kind == $repository_kind and
     .repository.package_format == $package_format and
     .repository.architecture == $architecture and
     .repository.machine == $machine and
     .repository.native == true and
     .repository.transport == "file://" and
     .repository.metadata_signature_enforced == true and
     .repository.key_scope == "ephemeral-ci-qualification" and
     (.hard_gates | length == 18) and
     (.hard_gates | to_entries | all(.value == true)) and
     .qualification_boundaries.stable_production_signing_key == false and
     .qualification_boundaries.public_hosted_repository == false and
     .qualification_boundaries.cross_version_lifecycle == false and
     .qualification_boundaries.rpm_payload_signature == false' \
    "$report" >/dev/null || fail "invalid $repository_kind/$architecture acceptance report"

  bundle_name=$(jq -r '.bundle.file' "$report")
  expected_name=vermory-repository-$repository_kind-$architecture-$source_sha.tar.gz
  [[ "$bundle_name" == "$expected_name" ]] || fail "unexpected $repository_kind/$architecture bundle filename"
  bundle=$artifact/$bundle_name
  [[ -f "$bundle" ]] || fail "accepted repository bytes are missing for $repository_kind/$architecture"
  expected_hash=$(jq -r '.bundle.sha256' "$report")
  [[ "$(hash_file "$bundle")" == "$expected_hash" ]] || fail "accepted repository hash mismatch for $repository_kind/$architecture"

  accepted_paths+=("$bundle")
  accepted_names+=("$bundle_name")
done

for index in "${!accepted_paths[@]}"; do
  install -m 0644 "${accepted_paths[$index]}" "$dist/${accepted_names[$index]}"
done

shopt -s nullglob
repository_bundles=("$dist"/vermory-repository-*.tar.gz)
shopt -u nullglob
[[ ${#repository_bundles[@]} -eq 4 ]] || fail "expected four assembled repository bundles"

echo "qualified repository assembly: pass (4 accepted repositories)"
