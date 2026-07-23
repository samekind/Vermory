#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "qualified package assembly: $*" >&2
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
  "deb amd64 x86_64"
  "rpm amd64 x86_64"
  "deb arm64 aarch64"
  "rpm arm64 aarch64"
)

downloaded_artifacts=()
for artifact in "$evidence_root"/vermory-i06-linux-package-*-"$source_sha"; do
  [[ -e "$artifact" ]] || continue
  downloaded_artifacts+=("$artifact")
done
[[ ${#downloaded_artifacts[@]} -eq 4 ]] || fail "expected exactly four native package artifacts"

accepted_paths=()
accepted_names=()

for spec in "${specs[@]}"; do
  read -r format architecture machine <<<"$spec"
  artifact="$evidence_root/vermory-i06-linux-package-$format-$architecture-$source_sha"
  report="$artifact/report.json"
  [[ -d "$artifact" && -f "$report" ]] || fail "missing $format/$architecture acceptance report"

  jq -e \
    --arg source_sha "$source_sha" \
    --arg format "$format" \
    --arg architecture "$architecture" \
    --arg machine "$machine" \
    '.version == "1" and
     .case_id == "I06-linux-native-packages" and
     .source_sha == $source_sha and
     .package.format == $format and
     .package.architecture == $architecture and
     .package.machine == $machine and
     .package.native == true and
     (.hard_gates | length == 16) and
     (.hard_gates | to_entries | all(.value == true))' \
    "$report" >/dev/null || fail "invalid $format/$architecture acceptance report"

  package_name=$(jq -r '.package.file' "$report")
  [[ "$package_name" == "$(basename "$package_name")" ]] || fail "package filename escapes its artifact directory"
  [[ "$package_name" == vermory_*_linux_"$architecture"."$format" ]] || fail "unexpected $format/$architecture package filename"
  package="$artifact/$package_name"
  [[ -f "$package" ]] || fail "accepted package bytes are missing for $format/$architecture"
  expected_hash=$(jq -r '.package.sha256' "$report")
  [[ "$(hash_file "$package")" == "$expected_hash" ]] || fail "accepted package hash mismatch for $format/$architecture"

  accepted_paths+=("$package")
  accepted_names+=("$package_name")
done

for index in "${!accepted_paths[@]}"; do
  install -m 0644 "${accepted_paths[$index]}" "$dist/${accepted_names[$index]}"
done

shopt -s nullglob
go_archives=("$dist"/vermory_*_darwin_*.tar.gz "$dist"/vermory_*_linux_*.tar.gz)
deb_packages=("$dist"/vermory_*_linux_*.deb)
rpm_packages=("$dist"/vermory_*_linux_*.rpm)
shopt -u nullglob
[[ ${#go_archives[@]} -eq 4 ]] || fail "expected four Go release archives"
[[ ${#deb_packages[@]} -eq 2 && ${#rpm_packages[@]} -eq 2 ]] || fail "expected four assembled Linux packages"

for path in "${go_archives[@]}" "${deb_packages[@]}" "${rpm_packages[@]}"; do
  basename "$path"
done | LC_ALL=C sort | while IFS= read -r name; do
  printf '%s  %s\n' "$(hash_file "$dist/$name")" "$name"
done >"$dist/checksums.txt"

echo "qualified package assembly: pass (4 accepted packages)"
