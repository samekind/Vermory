#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I09 lifecycle repository builder: $*" >&2
  exit 1
}

[[ $# -eq 5 ]] || fail "usage: $0 <versioned-package-set> <apt|dnf> <amd64|arm64> <candidate-source-sha> <output-directory>"

package_set_root=$1
repository_kind=$2
architecture=$3
candidate_source_sha=$4
output=$5
package_set_manifest=$package_set_root/package-set.json

[[ -d "$package_set_root" && -f "$package_set_manifest" ]] || fail "versioned package set is incomplete"
[[ "$repository_kind" == apt || "$repository_kind" == dnf ]] || fail "unsupported repository kind"
[[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "unsupported architecture"
[[ "$candidate_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "candidate source SHA is invalid"
[[ ! -e "$output" ]] || fail "output directory already exists"

for command in gpg gpgv jq sha256sum; do
  command -v "$command" >/dev/null || fail "$command is required"
done
case "$repository_kind" in
  apt)
    package_format=deb
    package_architecture=$architecture
    for command in apt-ftparchive dpkg-deb dpkg-scanpackages gzip; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    ;;
  dnf)
    package_format=rpm
    case "$architecture" in
      amd64) package_architecture=x86_64 ;;
      arm64) package_architecture=aarch64 ;;
    esac
    for command in createrepo_c gzip rpm; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    ;;
esac

base_source_sha=$(jq -r '.base_source_sha' "$package_set_manifest")
base_qualification_version=$(jq -r '.base_qualification_version' "$package_set_manifest")
candidate_qualification_version=$(jq -r '.candidate_qualification_version' "$package_set_manifest")
jq -e \
  --arg architecture "$architecture" \
  --arg candidate_source_sha "$candidate_source_sha" \
  '.version == "1" and
   .case_id == "I09-linux-repository-lifecycle" and
   .architecture == $architecture and
   .candidate_source_sha == $candidate_source_sha and
   .qualification_boundaries.release_tag == false and
   .qualification_boundaries.public_version_promise == false' \
  "$package_set_manifest" >/dev/null || fail "package-set manifest does not match this qualification leg"
[[ "$base_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "base source SHA is invalid"
[[ -n "$base_qualification_version" && -n "$candidate_qualification_version" ]] || fail "qualification versions are missing"

hash_file() {
  sha256sum "$1" | awk '{print $1}'
}

read_primary_metadata() {
  case "$1" in
    *.gz) gzip -dc "$1" ;;
    *.zst) zstd -dc "$1" ;;
    *.xz) xz -dc "$1" ;;
    *.bz2) bzip2 -dc "$1" ;;
    *.xml) cat "$1" ;;
    *) fail "unsupported DNF primary metadata compression" ;;
  esac
}

safe_package_path() {
  local relative=$1
  [[ "$relative" != /* && "$relative" != *..* ]] || fail "package-set path is unsafe"
  [[ -f "$package_set_root/$relative" ]] || fail "package-set payload is missing: $relative"
}

base_relative=$(jq -r ".packages.${package_format}.base.file" "$package_set_manifest")
candidate_relative=$(jq -r ".packages.${package_format}.candidate.file" "$package_set_manifest")
base_sha256=$(jq -r ".packages.${package_format}.base.sha256" "$package_set_manifest")
candidate_sha256=$(jq -r ".packages.${package_format}.candidate.sha256" "$package_set_manifest")
base_native_version=$(jq -r ".packages.${package_format}.base.native_version" "$package_set_manifest")
candidate_native_version=$(jq -r ".packages.${package_format}.candidate.native_version" "$package_set_manifest")
safe_package_path "$base_relative"
safe_package_path "$candidate_relative"
base_package=$package_set_root/$base_relative
candidate_package=$package_set_root/$candidate_relative
[[ "$(hash_file "$base_package")" == "$base_sha256" ]] || fail "base package hash mismatch"
[[ "$(hash_file "$candidate_package")" == "$candidate_sha256" ]] || fail "candidate package hash mismatch"

case "$repository_kind" in
  apt)
    [[ $(dpkg-deb --field "$base_package" Architecture) == "$package_architecture" ]] || fail "base DEB architecture mismatch"
    [[ $(dpkg-deb --field "$candidate_package" Architecture) == "$package_architecture" ]] || fail "candidate DEB architecture mismatch"
    [[ $(dpkg-deb --field "$base_package" Version) == "$base_native_version" ]] || fail "base DEB version mismatch"
    [[ $(dpkg-deb --field "$candidate_package" Version) == "$candidate_native_version" ]] || fail "candidate DEB version mismatch"
    ;;
  dnf)
    [[ $(rpm --query --package --queryformat '%{ARCH}' "$base_package") == "$package_architecture" ]] || fail "base RPM architecture mismatch"
    [[ $(rpm --query --package --queryformat '%{ARCH}' "$candidate_package") == "$package_architecture" ]] || fail "candidate RPM architecture mismatch"
    [[ $(rpm --query --package --queryformat '%{VERSION}-%{RELEASE}' "$base_package") == "$base_native_version" ]] || fail "base RPM version mismatch"
    [[ $(rpm --query --package --queryformat '%{VERSION}-%{RELEASE}' "$candidate_package") == "$candidate_native_version" ]] || fail "candidate RPM version mismatch"
    ;;
esac

umask 077
mkdir -p "$(dirname "$output")"
mkdir "$output"

key_home=$(mktemp -d "${TMPDIR:-/tmp}/vermory-i09-key.XXXXXX")
cleanup() {
  rm -rf "$key_home"
}
trap cleanup EXIT
chmod 0700 "$key_home"

key_identity="Vermory I09 Ephemeral Repository <i09@samekind.invalid>"
gpg --batch \
  --homedir "$key_home" \
  --pinentry-mode loopback \
  --passphrase '' \
  --quick-generate-key "$key_identity" rsa3072 sign 0 >/dev/null 2>&1
key_fingerprint=$(gpg --batch --homedir "$key_home" --with-colons --list-keys "$key_identity" \
  | awk -F: '$1 == "fpr" { print $10; exit }')
[[ "$key_fingerprint" =~ ^[A-F0-9]{40}$ ]] || fail "repository signing key fingerprint is invalid"
public_key_relative=vermory-repository-key.gpg
gpg --batch --homedir "$key_home" --export "$key_fingerprint" >"$output/$public_key_relative"

build_snapshot() {
  local snapshot_name=$1
  shift
  local snapshot_root=$output/$snapshot_name
  local repository=$snapshot_root/repository
  local expected_count=$#
  local package
  local package_name
  local package_sha256

  mkdir -p "$repository"
  case "$repository_kind" in
    apt)
      local index_directory=$repository/dists/stable/main/binary-$architecture
      mkdir -p "$repository/pool/main/v/vermory" "$index_directory"
      for package in "$@"; do
        package_name=$(basename "$package")
        install -m 0644 "$package" "$repository/pool/main/v/vermory/$package_name"
      done
      (
        cd "$repository"
        dpkg-scanpackages --multiversion --arch "$architecture" pool /dev/null
      ) >"$index_directory/Packages"
      [[ $(grep -c '^Package: vermory$' "$index_directory/Packages") -eq $expected_count ]] \
        || fail "$snapshot_name APT metadata does not contain the expected versions"
      for package in "$@"; do
        package_sha256=$(hash_file "$package")
        grep -F "SHA256: $package_sha256" "$index_directory/Packages" >/dev/null \
          || fail "$snapshot_name APT metadata does not bind a package digest"
      done
      gzip -n -9 -c "$index_directory/Packages" >"$index_directory/Packages.gz"
      apt-ftparchive \
        -o APT::FTPArchive::Release::Origin=Vermory \
        -o APT::FTPArchive::Release::Label=Vermory \
        -o APT::FTPArchive::Release::Suite=stable \
        -o APT::FTPArchive::Release::Codename=stable \
        -o APT::FTPArchive::Release::Architectures="$architecture" \
        -o APT::FTPArchive::Release::Components=main \
        release "$repository/dists/stable" >"$repository/dists/stable/Release"
      gpg --batch --yes --homedir "$key_home" --local-user "$key_fingerprint" \
        --armor --detach-sign \
        --output "$repository/dists/stable/Release.gpg" \
        "$repository/dists/stable/Release"
      gpg --batch --yes --homedir "$key_home" --local-user "$key_fingerprint" \
        --clearsign \
        --output "$repository/dists/stable/InRelease" \
        "$repository/dists/stable/Release"
      gpgv --keyring "$output/$public_key_relative" "$repository/dists/stable/InRelease" >/dev/null 2>&1 \
        || fail "$snapshot_name APT signature verification failed"
      printf '%s\n' 'dists/stable/InRelease' >"$snapshot_root/metadata.path"
      printf '%s\n' 'dists/stable/Release.gpg' >"$snapshot_root/signature.path"
      ;;
    dnf)
      mkdir -p "$repository/packages"
      for package in "$@"; do
        package_name=$(basename "$package")
        install -m 0644 "$package" "$repository/packages/$package_name"
      done
      createrepo_c --checksum sha256 --compress-type gz "$repository" >/dev/null
      local primary_metadata
      primary_metadata=$(find "$repository/repodata" -maxdepth 1 -type f \
        \( -name '*-primary.xml' -o -name '*-primary.xml.*' \) -print -quit)
      [[ -n "$primary_metadata" ]] || fail "$snapshot_name DNF primary metadata is missing"
      [[ $(read_primary_metadata "$primary_metadata" | grep -c '<name>vermory</name>') -eq $expected_count ]] \
        || fail "$snapshot_name DNF metadata does not contain the expected versions"
      for package in "$@"; do
        package_sha256=$(hash_file "$package")
        read_primary_metadata "$primary_metadata" | grep -F "$package_sha256" >/dev/null \
          || fail "$snapshot_name DNF metadata does not bind a package digest"
      done
      gpg --batch --yes --homedir "$key_home" --local-user "$key_fingerprint" \
        --armor --detach-sign \
        --output "$repository/repodata/repomd.xml.asc" \
        "$repository/repodata/repomd.xml"
      gpgv --keyring "$output/$public_key_relative" \
        "$repository/repodata/repomd.xml.asc" \
        "$repository/repodata/repomd.xml" >/dev/null 2>&1 \
        || fail "$snapshot_name DNF signature verification failed"
      printf '%s\n' 'repodata/repomd.xml' >"$snapshot_root/metadata.path"
      printf '%s\n' 'repodata/repomd.xml.asc' >"$snapshot_root/signature.path"
      ;;
  esac
}

build_snapshot base-snapshot "$base_package"
build_snapshot full-snapshot "$base_package" "$candidate_package"

base_metadata_relative=$(cat "$output/base-snapshot/metadata.path")
base_signature_relative=$(cat "$output/base-snapshot/signature.path")
full_metadata_relative=$(cat "$output/full-snapshot/metadata.path")
full_signature_relative=$(cat "$output/full-snapshot/signature.path")
rm "$output/base-snapshot/metadata.path" "$output/base-snapshot/signature.path"
rm "$output/full-snapshot/metadata.path" "$output/full-snapshot/signature.path"

package_set_sha256=$(hash_file "$package_set_manifest")
jq -n \
  --arg repository_kind "$repository_kind" \
  --arg architecture "$architecture" \
  --arg package_format "$package_format" \
  --arg base_source_sha "$base_source_sha" \
  --arg candidate_source_sha "$candidate_source_sha" \
  --arg base_qualification_version "$base_qualification_version" \
  --arg candidate_qualification_version "$candidate_qualification_version" \
  --arg base_native_version "$base_native_version" \
  --arg candidate_native_version "$candidate_native_version" \
  --arg base_file "$(basename "$base_package")" \
  --arg candidate_file "$(basename "$candidate_package")" \
  --arg base_sha256 "$base_sha256" \
  --arg candidate_sha256 "$candidate_sha256" \
  --arg package_set_sha256 "$package_set_sha256" \
  --arg public_key "$public_key_relative" \
  --arg key_fingerprint "$key_fingerprint" \
  --arg base_metadata "$base_metadata_relative" \
  --arg base_metadata_sha256 "$(hash_file "$output/base-snapshot/repository/$base_metadata_relative")" \
  --arg base_signature "$base_signature_relative" \
  --arg base_signature_sha256 "$(hash_file "$output/base-snapshot/repository/$base_signature_relative")" \
  --arg full_metadata "$full_metadata_relative" \
  --arg full_metadata_sha256 "$(hash_file "$output/full-snapshot/repository/$full_metadata_relative")" \
  --arg full_signature "$full_signature_relative" \
  --arg full_signature_sha256 "$(hash_file "$output/full-snapshot/repository/$full_signature_relative")" \
  '{
    version: "1",
    case_id: "I09-linux-repository-lifecycle",
    repository: {
      kind: $repository_kind,
      architecture: $architecture,
      package_format: $package_format,
      public_key: $public_key,
      key_fingerprint: $key_fingerprint,
      key_scope: "ephemeral-ci-qualification",
      key_relationship: "same-ephemeral-key"
    },
    sources: {
      base: $base_source_sha,
      candidate: $candidate_source_sha
    },
    versions: {
      base_qualification_version: $base_qualification_version,
      candidate_qualification_version: $candidate_qualification_version,
      base_native_version: $base_native_version,
      candidate_native_version: $candidate_native_version
    },
    packages: {
      base: {file: $base_file, sha256: $base_sha256},
      candidate: {file: $candidate_file, sha256: $candidate_sha256},
      package_set_manifest_sha256: $package_set_sha256
    },
    snapshots: {
      base: {
        path: "base-snapshot/repository",
        package_count: 1,
        metadata_path: $base_metadata,
        metadata_sha256: $base_metadata_sha256,
        signature_path: $base_signature,
        signature_sha256: $base_signature_sha256
      },
      full: {
        path: "full-snapshot/repository",
        package_count: 2,
        metadata_path: $full_metadata,
        metadata_sha256: $full_metadata_sha256,
        signature_path: $full_signature,
        signature_sha256: $full_signature_sha256
      }
    },
    qualification_boundaries: {
      stable_production_signing_key: false,
      public_hosted_repository: false,
      database_schema_migration: false,
      rpm_payload_signature: false,
      unattended_update_policy: false
    }
  }' >"$output/repository.json"

# Only public verification material may leave the builder; private signing key material stays in key_home.
if [[ -n $(find "$output" \
  \( -name private-keys-v1.d -o -name secring.gpg -o -name '*.key' -o -name '*.pem' \) \
  -print -quit) ]]; then
  fail "private signing key material entered the repository output"
fi

find "$output" -type d -exec chmod 0755 {} +
find "$output" -type f -exec chmod 0644 {} +

echo "I09 lifecycle repository builder: pass ($repository_kind/$architecture)"
