#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I09 versioned package builder: $*" >&2
  exit 1
}

[[ $# -eq 3 ]] || fail "usage: $0 <amd64|arm64> <candidate-source-sha> <output-directory>"

architecture=$1
candidate_source_sha=$2
output=$3
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case_file=$root/runtime/cases/I09-linux-repository-lifecycle/case.json

[[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "unsupported architecture"
[[ "$candidate_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "candidate source SHA must contain 40 lowercase hexadecimal characters"
[[ -f "$case_file" ]] || fail "I09 case contract is missing"
[[ ! -e "$output" ]] || fail "output directory already exists"

for command in dpkg dpkg-deb git goreleaser jq rpm sha256sum; do
  command -v "$command" >/dev/null || fail "$command is required"
done

base_source_sha=$(jq -r '.base_source_sha' "$case_file")
base_qualification_version=$(jq -r '.base_qualification_version' "$case_file")
candidate_qualification_version=$(jq -r '.candidate_qualification_version' "$case_file")
[[ "$base_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "base source SHA is invalid"
[[ "$base_source_sha" != "$candidate_source_sha" ]] || fail "base and candidate source revisions must differ"
[[ -n "$base_qualification_version" && -n "$candidate_qualification_version" ]] || fail "qualification versions are missing"
dpkg --compare-versions "$base_qualification_version" lt "$candidate_qualification_version" \
  || fail "candidate qualification version must be newer than the base version"

git -C "$root" cat-file -e "$base_source_sha^{commit}" 2>/dev/null \
  || fail "base source revision is unavailable"
git -C "$root" cat-file -e "$candidate_source_sha^{commit}" 2>/dev/null \
  || fail "candidate source revision is unavailable"
[[ $(git -C "$root" rev-parse "$candidate_source_sha^{commit}") == "$candidate_source_sha" ]] \
  || fail "candidate source revision did not resolve exactly"

case "$architecture" in
  amd64)
    deb_architecture=amd64
    rpm_architecture=x86_64
    ;;
  arm64)
    deb_architecture=arm64
    rpm_architecture=aarch64
    ;;
esac

hash_file() {
  sha256sum "$1" | awk '{print $1}'
}

work=$(mktemp -d "${TMPDIR:-/tmp}/vermory-i09-packages.XXXXXX")
base_worktree=$work/base
candidate_worktree=$work/candidate
cleanup() {
  git -C "$root" worktree remove --force "$base_worktree" >/dev/null 2>&1 || true
  git -C "$root" worktree remove --force "$candidate_worktree" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

# Detached worktrees keep both builds exact while leaving the caller's checkout and dist directory untouched.
(
  cd "$root"
  git worktree add --detach "$base_worktree" "$base_source_sha" >/dev/null
  git worktree add --detach "$candidate_worktree" "$candidate_source_sha" >/dev/null
)

build_snapshot() {
  local source_root=$1
  local qualification_version=$2

  (
    cd "$source_root"
    env \
      GOOS=linux \
      GOARCH="$architecture" \
      VERMORY_SNAPSHOT_VERSION="$qualification_version" \
      goreleaser release \
        --snapshot \
        --clean \
        --skip=publish \
        --config "$root/.goreleaser.yaml"
  )
}

build_snapshot "$base_worktree" "$base_qualification_version"
build_snapshot "$candidate_worktree" "$candidate_qualification_version"

mkdir -p "$output/packages/base" "$output/packages/candidate"

copy_package() {
  local source_root=$1
  local generation=$2
  local format=$3
  local -a matches=()

  mapfile -t matches < <(find "$source_root/dist" -maxdepth 1 -type f -name "vermory_*_linux_${architecture}.${format}" -print)
  [[ ${#matches[@]} -eq 1 ]] || fail "expected one $generation $format package, found ${#matches[@]}"
  install -m 0644 "${matches[0]}" "$output/packages/$generation/$(basename "${matches[0]}")"
}

for format in deb rpm; do
  copy_package "$base_worktree" base "$format"
  copy_package "$candidate_worktree" candidate "$format"
done

base_deb=$(find "$output/packages/base" -maxdepth 1 -type f -name '*.deb' -print -quit)
candidate_deb=$(find "$output/packages/candidate" -maxdepth 1 -type f -name '*.deb' -print -quit)
base_rpm=$(find "$output/packages/base" -maxdepth 1 -type f -name '*.rpm' -print -quit)
candidate_rpm=$(find "$output/packages/candidate" -maxdepth 1 -type f -name '*.rpm' -print -quit)

[[ $(dpkg-deb --field "$base_deb" Architecture) == "$deb_architecture" ]] || fail "base DEB architecture mismatch"
[[ $(dpkg-deb --field "$candidate_deb" Architecture) == "$deb_architecture" ]] || fail "candidate DEB architecture mismatch"
[[ $(rpm --query --package --queryformat '%{ARCH}' "$base_rpm") == "$rpm_architecture" ]] || fail "base RPM architecture mismatch"
[[ $(rpm --query --package --queryformat '%{ARCH}' "$candidate_rpm") == "$rpm_architecture" ]] || fail "candidate RPM architecture mismatch"

base_deb_version=$(dpkg-deb --field "$base_deb" Version)
candidate_deb_version=$(dpkg-deb --field "$candidate_deb" Version)
base_rpm_version=$(rpm --query --package --queryformat '%{VERSION}-%{RELEASE}' "$base_rpm")
candidate_rpm_version=$(rpm --query --package --queryformat '%{VERSION}-%{RELEASE}' "$candidate_rpm")
dpkg --compare-versions "$base_deb_version" lt "$candidate_deb_version" \
  || fail "native DEB candidate version is not newer than the base version"
[[ "$base_rpm_version" != "$candidate_rpm_version" ]] || fail "native RPM package versions are identical"

jq -n \
  --arg architecture "$architecture" \
  --arg base_source_sha "$base_source_sha" \
  --arg candidate_source_sha "$candidate_source_sha" \
  --arg base_qualification_version "$base_qualification_version" \
  --arg candidate_qualification_version "$candidate_qualification_version" \
  --arg base_deb_file "packages/base/$(basename "$base_deb")" \
  --arg base_deb_sha256 "$(hash_file "$base_deb")" \
  --arg base_deb_version "$base_deb_version" \
  --arg candidate_deb_file "packages/candidate/$(basename "$candidate_deb")" \
  --arg candidate_deb_sha256 "$(hash_file "$candidate_deb")" \
  --arg candidate_deb_version "$candidate_deb_version" \
  --arg base_rpm_file "packages/base/$(basename "$base_rpm")" \
  --arg base_rpm_sha256 "$(hash_file "$base_rpm")" \
  --arg base_rpm_version "$base_rpm_version" \
  --arg candidate_rpm_file "packages/candidate/$(basename "$candidate_rpm")" \
  --arg candidate_rpm_sha256 "$(hash_file "$candidate_rpm")" \
  --arg candidate_rpm_version "$candidate_rpm_version" \
  '{
    version: "1",
    case_id: "I09-linux-repository-lifecycle",
    architecture: $architecture,
    base_source_sha: $base_source_sha,
    candidate_source_sha: $candidate_source_sha,
    base_qualification_version: $base_qualification_version,
    candidate_qualification_version: $candidate_qualification_version,
    packages: {
      deb: {
        base: {file: $base_deb_file, sha256: $base_deb_sha256, native_version: $base_deb_version},
        candidate: {file: $candidate_deb_file, sha256: $candidate_deb_sha256, native_version: $candidate_deb_version}
      },
      rpm: {
        base: {file: $base_rpm_file, sha256: $base_rpm_sha256, native_version: $base_rpm_version},
        candidate: {file: $candidate_rpm_file, sha256: $candidate_rpm_sha256, native_version: $candidate_rpm_version}
      }
    },
    qualification_boundaries: {
      release_tag: false,
      public_version_promise: false
    }
  }' >"$output/package-set.json"

if grep -E -i '(postgresql://|api[_-]?key|password|private key|sk-[a-z0-9])' "$output/package-set.json"; then
  fail "package-set manifest contains credential material"
fi

find "$output" -type d -exec chmod 0755 {} +
find "$output" -type f -exec chmod 0644 {} +

echo "I09 versioned package builder: pass ($architecture)"
