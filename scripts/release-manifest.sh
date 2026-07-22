#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "usage: release-manifest.sh <create|verify> <dist-directory>" >&2
  exit 2
}

mode="${1:-}"
dist="${2:-}"
[[ "$mode" == "create" || "$mode" == "verify" ]] || usage
[[ -n "$dist" && -d "$dist" ]] || usage

manifest="$dist/release-manifest.sha256"

collect_payloads() {
  local path
  local -a go_archives deb_packages rpm_packages openclaw hermes hermes_sidecar

  shopt -s nullglob
  go_archives=("$dist"/vermory_*_*.tar.gz)
  deb_packages=("$dist"/vermory_*_linux_*.deb)
  rpm_packages=("$dist"/vermory_*_linux_*.rpm)
  openclaw=("$dist"/vermory-openclaw-*.tgz)
  hermes=("$dist"/vermory-hermes-*.tar.gz)
  hermes_sidecar=("$dist"/vermory-hermes-*.tar.gz.sha256)
  shopt -u nullglob

  [[ ${#go_archives[@]} -eq 4 ]] || return 1
  [[ ${#deb_packages[@]} -eq 2 ]] || return 1
  [[ ${#rpm_packages[@]} -eq 2 ]] || return 1
  [[ ${#openclaw[@]} -eq 1 ]] || return 1
  [[ ${#hermes[@]} -eq 1 ]] || return 1
  [[ ${#hermes_sidecar[@]} -eq 1 ]] || return 1
  [[ -f "$dist/checksums.txt" ]] || return 1

  for path in "${go_archives[@]}" "${deb_packages[@]}" "${rpm_packages[@]}" "$dist/checksums.txt" "${openclaw[@]}" "${hermes[@]}" "${hermes_sidecar[@]}"; do
    basename "$path"
  done | LC_ALL=C sort
}

hash_payloads() {
  local payload
  while IFS= read -r payload; do
    if command -v sha256sum >/dev/null 2>&1; then
      (cd "$dist" && sha256sum "$payload")
    else
      (cd "$dist" && shasum -a 256 "$payload")
    fi
  done
}

verify_hashes() {
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$dist" && sha256sum --check "$(basename "$manifest")")
  else
    (cd "$dist" && shasum -a 256 -c "$(basename "$manifest")")
  fi
}

expected="$(collect_payloads)" || {
  echo "release payload set is incomplete or ambiguous" >&2
  exit 1
}
[[ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" == "12" ]]

case "$mode" in
  create)
    printf '%s\n' "$expected" | hash_payloads > "$manifest"
    ;;
  verify)
    [[ -f "$manifest" ]] || {
      echo "release manifest is missing" >&2
      exit 1
    }
    actual="$(sed -E 's/^[a-f0-9]{64}  //' "$manifest")"
    [[ "$actual" == "$expected" ]] || {
      echo "release manifest payload inventory mismatch" >&2
      exit 1
    }
    verify_hashes
    ;;
esac
