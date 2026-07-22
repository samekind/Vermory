#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I08 repository acceptance: $*" >&2
  exit 1
}

[[ ${EUID:-$(id -u)} -eq 0 ]] || fail "effective UID 0 is required"
[[ $# -eq 4 ]] || fail "usage: $0 <repository-root> <apt|dnf> <amd64|arm64> <evidence-directory>"

repository_root=$1
repository_kind=$2
architecture=$3
evidence_directory=$4
source_sha=${SOURCE_SHA:-}
expected_machine=${I08_EXPECTED_MACHINE:-}

[[ -d "$repository_root/repository" && -f "$repository_root/repository.json" ]] || fail "repository output is incomplete"
[[ "$repository_kind" == apt || "$repository_kind" == dnf ]] || fail "unsupported repository kind"
[[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "unsupported architecture"
[[ "$source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "SOURCE_SHA is required"
[[ $(uname -m) == "$expected_machine" ]] || fail "runner machine does not match the qualification leg"
[[ ! -e /usr/bin/vermory ]] || fail "vermory binary already exists"
[[ ! -e /etc/vermory/vermory.env ]] || fail "vermory protected environment already exists"

case "$repository_kind:$architecture:$expected_machine" in
  apt:amd64:x86_64)
    package_format=deb
    package_architecture=amd64
    package_manager=apt
    ;;
  apt:arm64:aarch64)
    package_format=deb
    package_architecture=arm64
    package_manager=apt
    ;;
  dnf:amd64:x86_64)
    package_format=rpm
    package_architecture=x86_64
    package_manager=dnf
    ;;
  dnf:arm64:aarch64)
    package_format=rpm
    package_architecture=aarch64
    package_manager=dnf
    ;;
  *) fail "unsupported repository, architecture, and machine combination" ;;
esac

for command in gpg sha256sum jq tar gzip; do
  command -v "$command" >/dev/null || fail "$command is required"
done
command -v "$package_manager" >/dev/null || fail "$package_manager is required"

hash_file() {
  sha256sum "$1" | awk '{print $1}'
}

manifest=$repository_root/repository.json
repository=$repository_root/repository
jq -e \
  --arg source_sha "$source_sha" \
  --arg repository_kind "$repository_kind" \
  --arg architecture "$architecture" \
  --arg package_format "$package_format" \
  '.version == "1" and
   .case_id == "I08-linux-package-repository" and
   .source_sha == $source_sha and
   .repository.kind == $repository_kind and
   .repository.architecture == $architecture and
   .repository.package_format == $package_format and
   .repository.key_scope == "ephemeral-ci-qualification" and
   .qualification_boundaries.stable_production_signing_key == false and
   .qualification_boundaries.public_hosted_repository == false and
   .qualification_boundaries.cross_version_lifecycle == false and
   .qualification_boundaries.rpm_payload_signature == false' \
  "$manifest" >/dev/null || fail "repository manifest does not match this qualification leg"

package_relative=$(jq -r '.repository.package_path' "$manifest")
package_name=$(jq -r '.repository.package_file' "$manifest")
package_sha256=$(jq -r '.repository.package_sha256' "$manifest")
metadata_relative=$(jq -r '.repository.metadata_path' "$manifest")
metadata_sha256=$(jq -r '.repository.metadata_sha256' "$manifest")
signature_relative=$(jq -r '.repository.signature_path' "$manifest")
signature_sha256=$(jq -r '.repository.signature_sha256' "$manifest")
public_key_relative=$(jq -r '.repository.public_key' "$manifest")
key_fingerprint=$(jq -r '.repository.key_fingerprint' "$manifest")

for relative in "$package_relative" "$metadata_relative" "$signature_relative" "$public_key_relative"; do
  [[ "$relative" != /* && "$relative" != *..* ]] || fail "repository manifest path is unsafe"
done
package=$repository/$package_relative
metadata=$repository/$metadata_relative
signature=$repository/$signature_relative
public_key=$repository/$public_key_relative
[[ -f "$package" && -f "$metadata" && -f "$signature" && -f "$public_key" ]] || fail "repository payload is incomplete"
[[ "$(basename "$package")" == "$package_name" ]] || fail "package filename mismatch"
[[ "$(hash_file "$package")" == "$package_sha256" ]] || fail "accepted package bytes changed in the repository"
[[ "$(hash_file "$metadata")" == "$metadata_sha256" ]] || fail "repository metadata hash mismatch"
[[ "$(hash_file "$signature")" == "$signature_sha256" ]] || fail "repository signature hash mismatch"

actual_fingerprint=$(gpg --batch --show-keys --with-colons "$public_key" \
  | awk -F: '$1 == "fpr" { print $10; exit }')
[[ "$actual_fingerprint" == "$key_fingerprint" ]] || fail "bundled public key fingerprint mismatch"

case "$repository_kind" in
  apt)
    [[ $(dpkg-deb --field "$package" Architecture) == "$package_architecture" ]] || fail "DEB architecture mismatch"
    gpgv --keyring "$public_key" "$repository/dists/stable/InRelease" >/dev/null 2>&1 \
      || fail "APT repository signature is invalid"
    awk -v expected="$package_sha256" \
      '$1 == "SHA256:" && $2 == expected { found = 1 } END { exit found ? 0 : 1 }' \
      "$repository/dists/stable/main/binary-$architecture/Packages" \
      || fail "APT metadata does not bind the accepted package digest"
    ;;
  dnf)
    [[ $(rpm --query --package --queryformat '%{ARCH}' "$package") == "$package_architecture" ]] || fail "RPM architecture mismatch"
    gpgv --keyring "$public_key" "$signature" "$metadata" >/dev/null 2>&1 \
      || fail "DNF repository signature is invalid"
    primary_metadata=$(find "$repository/repodata" -maxdepth 1 -type f -name '*-primary.xml.gz' -print -quit)
    [[ -n "$primary_metadata" ]] || fail "DNF primary metadata is missing"
    gzip -dc "$primary_metadata" | grep -Fq "$package_sha256" \
      || fail "DNF metadata does not bind the accepted package digest"
    ;;
esac

private_material=$(find "$repository_root" \
  \( -name private-keys-v1.d -o -name secring.gpg -o -name '*.key' -o -name '*.pem' \) \
  -print -quit)
[[ -z "$private_material" ]] || fail "repository bundle contains private signing key material"

work=$(mktemp -d "${TMPDIR:-/tmp}/vermory-i08-acceptance.XXXXXX")
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

downloads=$work/downloads
mkdir -p "$downloads"
repository_url=file://$(realpath "$repository")

case "$repository_kind" in
  apt)
    for command in apt-get dpkg-query; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    apt_source=$work/vermory.list
    printf 'deb [arch=%s signed-by=%s] %s stable main\n' \
      "$architecture" "$public_key" "$repository_url" >"$apt_source"
    apt_lists=$work/apt-lists
    apt_archives=$work/apt-archives
    mkdir -p "$apt_lists/partial" "$apt_archives/partial"
    apt_options=(
      -o "Dir::Etc::sourcelist=$apt_source"
      -o "Dir::Etc::sourceparts=-"
      -o "Dir::State::lists=$apt_lists"
      -o "Dir::Cache::archives=$apt_archives"
      -o "Acquire::Languages=none"
    )
    apt-get "${apt_options[@]}" update >/dev/null
    apt-get "${apt_options[@]}" --download-only --yes --no-install-recommends install vermory >/dev/null
    shopt -s nullglob
    downloaded_packages=("$apt_archives"/vermory_*.deb)
    shopt -u nullglob
    [[ ${#downloaded_packages[@]} -eq 1 ]] || fail "APT did not download exactly one Vermory package"
    downloaded_package=${downloaded_packages[0]}
    [[ "$(hash_file "$downloaded_package")" == "$package_sha256" ]] || fail "APT downloaded package hash mismatch"
    apt-get "${apt_options[@]}" --yes --no-install-recommends install vermory >/dev/null
    dpkg-query --show --showformat='${Status}' vermory | grep -Fq 'install ok installed' \
      || fail "APT did not record Vermory as installed"

    tampered_repository=$work/tampered-repository
    cp -a "$repository" "$tampered_repository"
    tampered_packages=$tampered_repository/dists/stable/main/binary-$architecture/Packages
    printf '\n# tampered\n' >>"$tampered_packages"
    gzip -n -9 -c "$tampered_packages" >"$tampered_packages.gz"
    tampered_source=$work/tampered.list
    printf 'deb [arch=%s signed-by=%s] file://%s stable main\n' \
      "$architecture" "$tampered_repository/$public_key_relative" "$tampered_repository" >"$tampered_source"
    tampered_lists=$work/tampered-apt-lists
    tampered_archives=$work/tampered-apt-archives
    mkdir -p "$tampered_lists/partial" "$tampered_archives/partial"
    if apt-get \
      -o "Dir::Etc::sourcelist=$tampered_source" \
      -o "Dir::Etc::sourceparts=-" \
      -o "Dir::State::lists=$tampered_lists" \
      -o "Dir::Cache::archives=$tampered_archives" \
      -o "Acquire::Languages=none" \
      update >/dev/null 2>&1; then
      fail "APT accepted tampered repository metadata"
    fi
    ;;
  dnf)
    repository_config_directory=$work/repos
    mkdir -p "$repository_config_directory"
    repository_config=$repository_config_directory/vermory.repo
    cat >"$repository_config" <<EOF
[vermory]
name=Vermory I08
baseurl=$repository_url
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=file://$public_key
EOF
    dnf_cache=$work/dnf-cache
    dnf_persist=$work/dnf-persist
    dnf_log=$work/dnf-log
    mkdir -p "$dnf_cache" "$dnf_persist" "$dnf_log"
    dnf_options=(
      --assumeyes
      --disablerepo=\*
      --enablerepo=vermory
      --setopt="reposdir=$repository_config_directory"
      --setopt="cachedir=$dnf_cache"
      --setopt="persistdir=$dnf_persist"
      --setopt="logdir=$dnf_log"
      --setopt=install_weak_deps=False
    )
    dnf "${dnf_options[@]}" makecache >/dev/null
    dnf "${dnf_options[@]}" install --downloadonly --downloaddir="$downloads" vermory >/dev/null
    shopt -s nullglob
    downloaded_packages=("$downloads"/vermory-*.rpm)
    shopt -u nullglob
    [[ ${#downloaded_packages[@]} -eq 1 ]] || fail "DNF did not download exactly one Vermory package"
    downloaded_package=${downloaded_packages[0]}
    [[ "$(hash_file "$downloaded_package")" == "$package_sha256" ]] || fail "DNF downloaded package hash mismatch"
    dnf "${dnf_options[@]}" install vermory >/dev/null
    rpm --query vermory >/dev/null || fail "DNF did not record Vermory as installed"

    tampered_repository=$work/tampered-repository
    cp -a "$repository" "$tampered_repository"
    printf '\n<!-- tampered -->\n' >>"$tampered_repository/repodata/repomd.xml"
    tampered_config_directory=$work/tampered-repos
    mkdir -p "$tampered_config_directory"
    cat >"$tampered_config_directory/vermory.repo" <<EOF
[vermory]
name=Vermory I08 Tampered
baseurl=file://$tampered_repository
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=file://$tampered_repository/$public_key_relative
EOF
    if dnf \
      --assumeyes \
      --disablerepo=\* \
      --enablerepo=vermory \
      --setopt="reposdir=$tampered_config_directory" \
      --setopt="cachedir=$work/tampered-dnf-cache" \
      --setopt="persistdir=$work/tampered-dnf-persist" \
      --setopt="logdir=$work/tampered-dnf-log" \
      makecache >/dev/null 2>&1; then
      fail "DNF accepted tampered repository metadata"
    fi
    ;;
esac

[[ -x /usr/bin/vermory ]] || fail "repository installation did not provide the Vermory binary"
version_json=$(/usr/bin/vermory version)
[[ $(jq -r '.revision' <<<"$version_json") == "$source_sha" ]] || fail "installed binary is not bound to SOURCE_SHA"
if command -v systemctl >/dev/null; then
  systemctl is-active --quiet vermory.service && fail "repository installation activated the service"
  systemctl is-enabled --quiet vermory.service && fail "repository installation enabled the service"
fi

case "$repository_kind" in
  apt) dpkg --remove vermory >/dev/null ;;
  dnf) rpm --erase vermory >/dev/null ;;
esac

mkdir -p "$evidence_directory"
[[ -z $(find "$evidence_directory" -mindepth 1 -print -quit) ]] || fail "evidence directory must be empty"
bundle_name=vermory-repository-$repository_kind-$architecture-$source_sha.tar.gz
bundle_stage=$work/bundle/vermory-repository-$repository_kind-$architecture
mkdir -p "$bundle_stage"
cp -a "$repository_root/." "$bundle_stage/"
tar \
  --sort=name \
  --mtime='@0' \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$work/bundle" \
  -cf - "$(basename "$bundle_stage")" \
  | gzip -n -9 >"$evidence_directory/$bundle_name"
bundle_sha256=$(hash_file "$evidence_directory/$bundle_name")

jq -n \
  --arg source_sha "$source_sha" \
  --arg repository_kind "$repository_kind" \
  --arg package_format "$package_format" \
  --arg architecture "$architecture" \
  --arg machine "$expected_machine" \
  --arg package_manager "$package_manager" \
  --arg package_file "$package_name" \
  --arg package_sha256 "$package_sha256" \
  --arg key_fingerprint "$key_fingerprint" \
  --arg bundle_file "$bundle_name" \
  --arg bundle_sha256 "$bundle_sha256" \
  '{
    version: "1",
    case_id: "I08-linux-package-repository",
    source_sha: $source_sha,
    repository: {
      kind: $repository_kind,
      package_format: $package_format,
      architecture: $architecture,
      machine: $machine,
      native: true,
      package_manager: $package_manager,
      transport: "file://",
      package_file: $package_file,
      package_sha256: $package_sha256,
      metadata_signature_enforced: true,
      key_fingerprint: $key_fingerprint,
      key_scope: "ephemeral-ci-qualification"
    },
    bundle: {
      file: $bundle_file,
      sha256: $bundle_sha256
    },
    hard_gates: {
      exact_source_head: true,
      exact_i06_package_bytes: true,
      expected_repository_kind: true,
      native_architecture: true,
      native_package_manager: true,
      file_repository_only: true,
      native_repository_metadata: true,
      metadata_package_digest_bound: true,
      repository_signature_valid: true,
      client_signature_enforced: true,
      downloaded_package_digest_bound: true,
      installed_revision_bound: true,
      service_not_activated: true,
      tampered_metadata_rejected: true,
      private_signing_key_absent: true,
      ephemeral_key_scope_declared: true,
      repository_bundle_hashed: true,
      report_credential_free: true
    },
    qualification_boundaries: {
      stable_production_signing_key: false,
      public_hosted_repository: false,
      cross_version_lifecycle: false,
      rpm_payload_signature: false
    }
  }' >"$evidence_directory/report.json"

if grep -E -i '(postgresql://|api[_-]?key|password|private key|sk-[a-z0-9])' "$evidence_directory/report.json"; then
  fail "normalized report contains credential material"
fi
chmod 0644 "$evidence_directory/report.json" "$evidence_directory/$bundle_name"
chmod 0755 "$evidence_directory"

echo "I08 repository acceptance: pass ($repository_kind/$architecture)"
