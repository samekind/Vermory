#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I08 repository builder: $*" >&2
  exit 1
}

[[ $# -eq 5 ]] || fail "usage: $0 <I06-evidence-directory> <apt|dnf> <amd64|arm64> <source-sha> <output-directory>"

i06_evidence=$1
repository_kind=$2
architecture=$3
source_sha=$4
output=$5

[[ -d "$i06_evidence" ]] || fail "I06 evidence directory is required"
[[ "$repository_kind" == apt || "$repository_kind" == dnf ]] || fail "unsupported repository kind"
[[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "unsupported architecture"
[[ "$source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "source SHA must contain 40 lowercase hexadecimal characters"
[[ ! -e "$output" ]] || fail "output directory already exists"

case "$repository_kind:$architecture" in
  apt:amd64)
    package_format=deb
    package_architecture=amd64
    ;;
  apt:arm64)
    package_format=deb
    package_architecture=arm64
    ;;
  dnf:amd64)
    package_format=rpm
    package_architecture=x86_64
    ;;
  dnf:arm64)
    package_format=rpm
    package_architecture=aarch64
    ;;
esac

for command in gpg gpgv jq sha256sum; do
  command -v "$command" >/dev/null || fail "$command is required"
done
case "$repository_kind" in
  apt)
    for command in apt-ftparchive dpkg-deb dpkg-scanpackages gzip; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    ;;
  dnf)
    for command in createrepo_c gzip rpm; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    ;;
esac

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

i06_report=$i06_evidence/report.json
[[ -f "$i06_report" ]] || fail "I06 acceptance report is missing"
jq -e \
  --arg source_sha "$source_sha" \
  --arg package_format "$package_format" \
  --arg architecture "$architecture" \
  '.version == "1" and
   .case_id == "I06-linux-native-packages" and
   .source_sha == $source_sha and
   .package.format == $package_format and
   .package.architecture == $architecture and
   .package.native == true and
   (.hard_gates | length == 16) and
   (.hard_gates | to_entries | all(.value == true))' \
  "$i06_report" >/dev/null || fail "I06 acceptance report does not match this repository leg"

package_name=$(jq -r '.package.file' "$i06_report")
[[ "$package_name" == "$(basename "$package_name")" ]] || fail "I06 package filename escapes its evidence directory"
source_package=$i06_evidence/$package_name
[[ -f "$source_package" ]] || fail "I06 accepted package bytes are missing"
package_sha256=$(jq -r '.package.sha256' "$i06_report")
[[ "$(hash_file "$source_package")" == "$package_sha256" ]] || fail "I06 accepted package hash mismatch"

case "$repository_kind" in
  apt)
    [[ $(dpkg-deb --field "$source_package" Architecture) == "$package_architecture" ]] || fail "DEB architecture mismatch"
    ;;
  dnf)
    [[ $(rpm --query --package --queryformat '%{ARCH}' "$source_package") == "$package_architecture" ]] || fail "RPM architecture mismatch"
    ;;
esac

umask 077
mkdir -p "$(dirname "$output")"
mkdir "$output"
repository=$output/repository
mkdir "$repository"

key_home=$(mktemp -d "${TMPDIR:-/tmp}/vermory-i08-key.XXXXXX")
cleanup() {
  rm -rf "$key_home"
}
trap cleanup EXIT
chmod 0700 "$key_home"

key_identity="Vermory I08 Ephemeral Repository <i08@samekind.invalid>"
gpg --batch \
  --homedir "$key_home" \
  --pinentry-mode loopback \
  --passphrase '' \
  --quick-generate-key "$key_identity" rsa3072 sign 0 >/dev/null 2>&1
key_fingerprint=$(gpg --batch --homedir "$key_home" --with-colons --list-keys "$key_identity" \
  | awk -F: '$1 == "fpr" { print $10; exit }')
[[ "$key_fingerprint" =~ ^[A-F0-9]{40}$ ]] || fail "repository signing key fingerprint is invalid"
public_key=vermory-repository-key.gpg
gpg --batch --homedir "$key_home" --export "$key_fingerprint" >"$repository/$public_key"

case "$repository_kind" in
  apt)
    package_relative=pool/main/v/vermory/$package_name
    package_directory=$repository/$(dirname "$package_relative")
    index_directory=$repository/dists/stable/main/binary-$architecture
    mkdir -p "$package_directory" "$index_directory"
    install -m 0644 "$source_package" "$repository/$package_relative"

    (
      cd "$repository"
      dpkg-scanpackages --arch "$architecture" pool /dev/null
    ) >"$index_directory/Packages"
    gzip -n -9 -c "$index_directory/Packages" >"$index_directory/Packages.gz"
    awk -v expected="$package_sha256" \
      '$1 == "SHA256:" && $2 == expected { found = 1 } END { exit found ? 0 : 1 }' \
      "$index_directory/Packages" || fail "APT metadata does not contain the accepted package digest"

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
    gpgv --keyring "$repository/$public_key" "$repository/dists/stable/InRelease" >/dev/null 2>&1 \
      || fail "APT InRelease signature verification failed"
    metadata_relative=dists/stable/InRelease
    signature_relative=dists/stable/Release.gpg
    ;;
  dnf)
    package_relative=packages/$package_name
    mkdir -p "$repository/packages"
    install -m 0644 "$source_package" "$repository/$package_relative"
    createrepo_c --checksum sha256 --compress-type gz "$repository" >/dev/null
    primary_metadata=$(find "$repository/repodata" -maxdepth 1 -type f \
      \( -name '*-primary.xml' -o -name '*-primary.xml.*' \) -print -quit)
    [[ -n "$primary_metadata" ]] || fail "DNF primary metadata is missing"
    read_primary_metadata "$primary_metadata" | grep -F "$package_sha256" >/dev/null \
      || fail "DNF metadata does not contain the accepted package digest"
    gpg --batch --yes --homedir "$key_home" --local-user "$key_fingerprint" \
      --armor --detach-sign \
      --output "$repository/repodata/repomd.xml.asc" \
      "$repository/repodata/repomd.xml"
    gpgv --keyring "$repository/$public_key" \
      "$repository/repodata/repomd.xml.asc" \
      "$repository/repodata/repomd.xml" >/dev/null 2>&1 \
      || fail "DNF repomd signature verification failed"
    metadata_relative=repodata/repomd.xml
    signature_relative=repodata/repomd.xml.asc
    ;;
esac

metadata_sha256=$(hash_file "$repository/$metadata_relative")
signature_sha256=$(hash_file "$repository/$signature_relative")
i06_report_sha256=$(hash_file "$i06_report")

if [[ -n $(find "$output" \
  \( -name private-keys-v1.d -o -name secring.gpg -o -name '*.key' -o -name '*.pem' \) \
  -print -quit) ]]; then
  fail "private signing key material entered the repository output"
fi

jq -n \
  --arg source_sha "$source_sha" \
  --arg repository_kind "$repository_kind" \
  --arg architecture "$architecture" \
  --arg package_format "$package_format" \
  --arg package_file "$package_name" \
  --arg package_relative "$package_relative" \
  --arg package_sha256 "$package_sha256" \
  --arg i06_report_sha256 "$i06_report_sha256" \
  --arg metadata_relative "$metadata_relative" \
  --arg metadata_sha256 "$metadata_sha256" \
  --arg signature_relative "$signature_relative" \
  --arg signature_sha256 "$signature_sha256" \
  --arg public_key "$public_key" \
  --arg key_fingerprint "$key_fingerprint" \
  '{
    version: "1",
    case_id: "I08-linux-package-repository",
    source_sha: $source_sha,
    repository: {
      kind: $repository_kind,
      architecture: $architecture,
      package_format: $package_format,
      package_file: $package_file,
      package_path: $package_relative,
      package_sha256: $package_sha256,
      i06_report_sha256: $i06_report_sha256,
      metadata_path: $metadata_relative,
      metadata_sha256: $metadata_sha256,
      signature_path: $signature_relative,
      signature_sha256: $signature_sha256,
      public_key: $public_key,
      key_fingerprint: $key_fingerprint,
      key_scope: "ephemeral-ci-qualification"
    },
    qualification_boundaries: {
      stable_production_signing_key: false,
      public_hosted_repository: false,
      cross_version_lifecycle: false,
      rpm_payload_signature: false
    }
  }' >"$output/repository.json"

find "$output" -type d -exec chmod 0755 {} +
find "$output" -type f -exec chmod 0644 {} +

echo "I08 repository builder: pass ($repository_kind/$architecture)"
