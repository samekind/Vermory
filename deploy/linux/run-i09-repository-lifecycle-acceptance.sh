#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "I09 repository lifecycle acceptance: $*" >&2
  exit 1
}

[[ ${EUID:-$(id -u)} -eq 0 ]] || fail "effective UID 0 is required"
[[ $# -eq 4 ]] || fail "usage: $0 <repository-root> <apt|dnf> <amd64|arm64> <evidence-directory>"

repository_root=$1
repository_kind=$2
architecture=$3
evidence_directory=$4
candidate_source_sha=${SOURCE_SHA:-}
expected_machine=${I09_EXPECTED_MACHINE:-}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
case_file=$root/runtime/cases/I09-linux-repository-lifecycle/case.json
manifest=$repository_root/repository.json

[[ -d "$repository_root" && -f "$manifest" ]] || fail "repository lifecycle output is incomplete"
[[ -f "$case_file" ]] || fail "I09 case contract is missing"
[[ "$repository_kind" == apt || "$repository_kind" == dnf ]] || fail "unsupported repository kind"
[[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "unsupported architecture"
[[ "$candidate_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail "SOURCE_SHA is required"
[[ $(uname -m) == "$expected_machine" ]] || fail "runner machine does not match the qualification leg"
[[ ! -e /usr/bin/vermory ]] || fail "vermory binary already exists"
[[ ! -e /etc/vermory/vermory.env ]] || fail "vermory protected environment already exists"
getent passwd vermory >/dev/null && fail "vermory service user already exists"

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

for command in getent gpg gpgv jq sha256sum tar gzip; do
  command -v "$command" >/dev/null || fail "$command is required"
done
command -v "$package_manager" >/dev/null || fail "$package_manager is required"

hash_file() {
  sha256sum "$1" | awk '{print $1}'
}

safe_relative_path() {
  local relative=$1
  [[ "$relative" != /* && "$relative" != *..* ]] || fail "repository manifest path is unsafe"
}

assert_service_dormant() {
  if command -v systemctl >/dev/null; then
    if systemctl is-active --quiet vermory.service 2>/dev/null; then
      fail "service became active"
    fi
    if systemctl is-enabled --quiet vermory.service 2>/dev/null; then
      fail "service became enabled"
    fi
  fi
}

installed_revision() {
  [[ -x /usr/bin/vermory ]] || fail "installed Vermory binary is missing"
  /usr/bin/vermory version | jq -r '.revision'
}

service_identity() {
  local passwd_entry
  local group_entry
  passwd_entry=$(getent passwd vermory) || fail "vermory service identity is missing"
  group_entry=$(getent group vermory) || fail "vermory service group is missing"
  printf '%s|%s\n' "$passwd_entry" "$group_entry"
}

assert_operator_state() {
  local expected_identity=$1
  [[ -f /etc/vermory/vermory.env ]] || fail "operator configuration is missing"
  [[ $(cat /etc/vermory/vermory.env) == operator-owned-i09 ]] || fail "operator configuration changed"
  [[ $(service_identity) == "$expected_identity" ]] || fail "service identity changed"
  assert_service_dormant
}

base_source_sha=$(jq -r '.base_source_sha' "$case_file")
base_qualification_version=$(jq -r '.base_qualification_version' "$case_file")
candidate_qualification_version=$(jq -r '.candidate_qualification_version' "$case_file")
jq -e \
  --arg repository_kind "$repository_kind" \
  --arg architecture "$architecture" \
  --arg package_format "$package_format" \
  --arg base_source_sha "$base_source_sha" \
  --arg candidate_source_sha "$candidate_source_sha" \
  --arg base_qualification_version "$base_qualification_version" \
  --arg candidate_qualification_version "$candidate_qualification_version" \
  '.version == "1" and
   .case_id == "I09-linux-repository-lifecycle" and
   .repository.kind == $repository_kind and
   .repository.architecture == $architecture and
   .repository.package_format == $package_format and
   .repository.key_scope == "ephemeral-ci-qualification" and
   .repository.key_relationship == "same-ephemeral-key" and
   .sources.base == $base_source_sha and
   .sources.candidate == $candidate_source_sha and
   .versions.base_qualification_version == $base_qualification_version and
   .versions.candidate_qualification_version == $candidate_qualification_version and
   .snapshots.base.package_count == 1 and
   .snapshots.full.package_count == 2 and
   .qualification_boundaries.stable_production_signing_key == false and
   .qualification_boundaries.public_hosted_repository == false and
   .qualification_boundaries.database_schema_migration == false and
   .qualification_boundaries.rpm_payload_signature == false and
   .qualification_boundaries.unattended_update_policy == false' \
  "$manifest" >/dev/null || fail "repository manifest does not match this qualification leg"

base_native_version=$(jq -r '.versions.base_native_version' "$manifest")
candidate_native_version=$(jq -r '.versions.candidate_native_version' "$manifest")
base_package_name=$(jq -r '.packages.base.file' "$manifest")
candidate_package_name=$(jq -r '.packages.candidate.file' "$manifest")
base_package_sha256=$(jq -r '.packages.base.sha256' "$manifest")
candidate_package_sha256=$(jq -r '.packages.candidate.sha256' "$manifest")
public_key_relative=$(jq -r '.repository.public_key' "$manifest")
key_fingerprint=$(jq -r '.repository.key_fingerprint' "$manifest")
base_snapshot_relative=$(jq -r '.snapshots.base.path' "$manifest")
full_snapshot_relative=$(jq -r '.snapshots.full.path' "$manifest")
safe_relative_path "$public_key_relative"
safe_relative_path "$base_snapshot_relative"
safe_relative_path "$full_snapshot_relative"
public_key=$repository_root/$public_key_relative
base_repository=$repository_root/$base_snapshot_relative
full_repository=$repository_root/$full_snapshot_relative
[[ -f "$public_key" && -d "$base_repository" && -d "$full_repository" ]] || fail "repository snapshots are incomplete"

actual_fingerprint=$(gpg --batch --show-keys --with-colons "$public_key" \
  | awk -F: '$1 == "fpr" { print $10; exit }')
[[ "$actual_fingerprint" == "$key_fingerprint" ]] || fail "bundled public key fingerprint mismatch"

verify_snapshot() {
  local snapshot_label=$1
  local repository=$2
  local metadata_relative
  local signature_relative
  local metadata_sha256
  local signature_sha256

  metadata_relative=$(jq -r ".snapshots.${snapshot_label}.metadata_path" "$manifest")
  signature_relative=$(jq -r ".snapshots.${snapshot_label}.signature_path" "$manifest")
  metadata_sha256=$(jq -r ".snapshots.${snapshot_label}.metadata_sha256" "$manifest")
  signature_sha256=$(jq -r ".snapshots.${snapshot_label}.signature_sha256" "$manifest")
  safe_relative_path "$metadata_relative"
  safe_relative_path "$signature_relative"
  [[ -f "$repository/$metadata_relative" && -f "$repository/$signature_relative" ]] \
    || fail "$snapshot_label snapshot metadata is missing"
  [[ $(hash_file "$repository/$metadata_relative") == "$metadata_sha256" ]] \
    || fail "$snapshot_label metadata hash mismatch"
  [[ $(hash_file "$repository/$signature_relative") == "$signature_sha256" ]] \
    || fail "$snapshot_label signature hash mismatch"
  case "$repository_kind" in
    apt)
      gpgv --keyring "$public_key" "$repository/dists/stable/InRelease" >/dev/null 2>&1 \
        || fail "$snapshot_label APT signature verification failed"
      ;;
    dnf)
      gpgv --keyring "$public_key" \
        "$repository/$signature_relative" \
        "$repository/$metadata_relative" >/dev/null 2>&1 \
        || fail "$snapshot_label DNF signature verification failed"
      ;;
  esac
}

verify_snapshot base "$base_repository"
verify_snapshot full "$full_repository"

mapfile -t base_snapshot_packages < <(find "$base_repository" -type f -name "$base_package_name" -print)
mapfile -t full_base_packages < <(find "$full_repository" -type f -name "$base_package_name" -print)
mapfile -t full_candidate_packages < <(find "$full_repository" -type f -name "$candidate_package_name" -print)
[[ ${#base_snapshot_packages[@]} -eq 1 ]] || fail "base snapshot does not contain exactly one base package"
[[ ${#full_base_packages[@]} -eq 1 && ${#full_candidate_packages[@]} -eq 1 ]] \
  || fail "full snapshot does not retain base and candidate packages"
[[ $(hash_file "${base_snapshot_packages[0]}") == "$base_package_sha256" ]] || fail "base snapshot package hash mismatch"
[[ $(hash_file "${full_base_packages[0]}") == "$base_package_sha256" ]] || fail "retained base package hash mismatch"
[[ $(hash_file "${full_candidate_packages[0]}") == "$candidate_package_sha256" ]] || fail "candidate package hash mismatch"

private_material=$(find "$repository_root" \
  \( -name private-keys-v1.d -o -name secring.gpg -o -name '*.key' -o -name '*.pem' \) \
  -print -quit)
[[ -z "$private_material" ]] || fail "repository bundle contains private signing key material"

work=$(mktemp -d "${TMPDIR:-/tmp}/vermory-i09-acceptance.XXXXXX")
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

base_repository_url=file://$(realpath "$base_repository")
full_repository_url=file://$(realpath "$full_repository")
metadata_signature_enforced=false
unrelated_repositories_disabled=false

case "$repository_kind" in
  apt)
    for command in apt-get dpkg dpkg-query; do
      command -v "$command" >/dev/null || fail "$command is required"
    done
    dpkg --compare-versions "$base_native_version" lt "$candidate_native_version" \
      || fail "native APT candidate version is not newer than the base version"

    base_source=$work/base.list
    full_source=$work/full.list
    printf 'deb [arch=%s signed-by=%s] %s stable main\n' \
      "$architecture" "$public_key" "$base_repository_url" >"$base_source"
    printf 'deb [arch=%s signed-by=%s] %s stable main\n' \
      "$architecture" "$public_key" "$full_repository_url" >"$full_source"
    grep -Fq 'signed-by=' "$base_source"
    grep -Fq 'signed-by=' "$full_source"
    metadata_signature_enforced=true
    unrelated_repositories_disabled=true

    apt_command() {
      local source=$1
      local state=$2
      shift 2
      mkdir -p "$work/$state/lists/partial" "$work/$state/archives/partial"
      apt-get \
        -o "Dir::Etc::sourcelist=$source" \
        -o "Dir::Etc::sourceparts=-" \
        -o "Dir::State::lists=$work/$state/lists" \
        -o "Dir::Cache::archives=$work/$state/archives" \
        -o "Acquire::Languages=none" \
        -o "APT::Sandbox::User=root" \
        "$@"
    }

    apt_command "$base_source" base update >/dev/null
    apt_command "$base_source" base --yes --no-install-recommends install vermory >/dev/null
    installed_base_version=$(dpkg-query --show --showformat='${Version}' vermory)
    [[ "$installed_base_version" == "$base_native_version" ]] || fail "APT did not install the base version"
    [[ $(installed_revision) == "$base_source_sha" ]] || fail "installed base binary revision mismatch"
    assert_service_dormant

    install -d -o root -g root -m 0755 /etc/vermory
    printf '%s\n' 'operator-owned-i09' >/etc/vermory/vermory.env
    chmod 0600 /etc/vermory/vermory.env
    stable_identity=$(service_identity)

    apt_command "$full_source" full update >/dev/null
    apt_command "$full_source" full --yes --no-install-recommends upgrade >/dev/null
    [[ $(dpkg-query --show --showformat='${Version}' vermory) == "$candidate_native_version" ]] \
      || fail "normal APT upgrade did not select the candidate"
    [[ $(installed_revision) == "$candidate_source_sha" ]] || fail "upgraded binary revision mismatch"
    assert_operator_state "$stable_identity"

    apt_command "$base_source" base update >/dev/null
    apt_command "$base_source" base --yes --no-install-recommends upgrade >/dev/null
    [[ $(dpkg-query --show --showformat='${Version}' vermory) == "$candidate_native_version" ]] \
      || fail "normal APT update implicitly downgraded the candidate"
    assert_operator_state "$stable_identity"

    apt_command "$full_source" full --yes --no-install-recommends --allow-downgrades \
      install "vermory=$base_native_version" >/dev/null
    [[ $(dpkg-query --show --showformat='${Version}' vermory) == "$base_native_version" ]] \
      || fail "explicit APT rollback did not select the retained base"
    [[ $(installed_revision) == "$base_source_sha" ]] || fail "rolled-back binary revision mismatch"
    assert_operator_state "$stable_identity"

    apt_command "$full_source" full --yes --no-install-recommends upgrade >/dev/null
    [[ $(dpkg-query --show --showformat='${Version}' vermory) == "$candidate_native_version" ]] \
      || fail "second normal APT upgrade did not return to the candidate"
    [[ $(installed_revision) == "$candidate_source_sha" ]] || fail "re-upgraded binary revision mismatch"
    assert_operator_state "$stable_identity"

    apt_command "$full_source" full --yes remove vermory >/dev/null
    ;;
  dnf)
    command -v rpm >/dev/null || fail "rpm is required"
    base_config_directory=$work/base-repos
    full_config_directory=$work/full-repos
    mkdir -p "$base_config_directory" "$full_config_directory"
    cat >"$base_config_directory/vermory.repo" <<EOF
[vermory-base]
name=Vermory I09 Base
baseurl=$base_repository_url
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=file://$public_key
metadata_expire=0
skip_if_unavailable=0
EOF
    cat >"$full_config_directory/vermory.repo" <<EOF
[vermory-full]
name=Vermory I09 Full
baseurl=$full_repository_url
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=file://$public_key
metadata_expire=0
skip_if_unavailable=0
EOF
    grep -Fq 'repo_gpgcheck=1' "$base_config_directory/vermory.repo"
    grep -Fq 'repo_gpgcheck=1' "$full_config_directory/vermory.repo"
    metadata_signature_enforced=true
    unrelated_repositories_disabled=true

    dnf_command() {
      local label=$1
      local config_directory=$2
      shift 2
      mkdir -p "$work/$label/cache" "$work/$label/persist" "$work/$label/log"
      dnf \
        --assumeyes \
        --refresh \
        --disablerepo=\* \
        --enablerepo="vermory-$label" \
        --setopt="reposdir=$config_directory" \
        --setopt="cachedir=$work/$label/cache" \
        --setopt="persistdir=$work/$label/persist" \
        --setopt="logdir=$work/$label/log" \
        --setopt=install_weak_deps=False \
        "$@"
    }

    dnf_command base "$base_config_directory" install vermory >/dev/null
    installed_base_version=$(rpm --query --queryformat '%{VERSION}-%{RELEASE}' vermory)
    [[ "$installed_base_version" == "$base_native_version" ]] || fail "DNF did not install the base version"
    [[ $(installed_revision) == "$base_source_sha" ]] || fail "installed base binary revision mismatch"
    assert_service_dormant

    install -d -o root -g root -m 0755 /etc/vermory
    printf '%s\n' 'operator-owned-i09' >/etc/vermory/vermory.env
    chmod 0600 /etc/vermory/vermory.env
    stable_identity=$(service_identity)

    dnf_command full "$full_config_directory" upgrade vermory >/dev/null
    [[ $(rpm --query --queryformat '%{VERSION}-%{RELEASE}' vermory) == "$candidate_native_version" ]] \
      || fail "normal DNF upgrade did not select the candidate"
    [[ $(installed_revision) == "$candidate_source_sha" ]] || fail "upgraded binary revision mismatch"
    assert_operator_state "$stable_identity"

    dnf_command base "$base_config_directory" upgrade vermory >/dev/null
    [[ $(rpm --query --queryformat '%{VERSION}-%{RELEASE}' vermory) == "$candidate_native_version" ]] \
      || fail "normal DNF update implicitly downgraded the candidate"
    assert_operator_state "$stable_identity"

    dnf_command full "$full_config_directory" downgrade \
      "vermory-$base_native_version.$package_architecture" >/dev/null
    [[ $(rpm --query --queryformat '%{VERSION}-%{RELEASE}' vermory) == "$base_native_version" ]] \
      || fail "explicit DNF rollback did not select the retained base"
    [[ $(installed_revision) == "$base_source_sha" ]] || fail "rolled-back binary revision mismatch"
    assert_operator_state "$stable_identity"

    dnf_command full "$full_config_directory" upgrade vermory >/dev/null
    [[ $(rpm --query --queryformat '%{VERSION}-%{RELEASE}' vermory) == "$candidate_native_version" ]] \
      || fail "second normal DNF upgrade did not return to the candidate"
    [[ $(installed_revision) == "$candidate_source_sha" ]] || fail "re-upgraded binary revision mismatch"
    assert_operator_state "$stable_identity"

    dnf_command full "$full_config_directory" remove vermory >/dev/null
    ;;
esac

[[ ! -e /usr/bin/vermory ]] || fail "final package removal retained the binary"
[[ -f /etc/vermory/vermory.env && $(cat /etc/vermory/vermory.env) == operator-owned-i09 ]] \
  || fail "final package removal changed operator configuration"
[[ $(service_identity) == "$stable_identity" ]] || fail "final package removal changed the service identity"
assert_service_dormant

mkdir -p "$evidence_directory"
[[ -z $(find "$evidence_directory" -mindepth 1 -print -quit) ]] || fail "evidence directory must be empty"
bundle_name=vermory-repository-lifecycle-$repository_kind-$architecture-$candidate_source_sha.tar.gz
bundle_stage=$work/bundle/vermory-repository-lifecycle-$repository_kind-$architecture
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
repository_manifest_sha256=$(hash_file "$manifest")

jq -n \
  --arg repository_kind "$repository_kind" \
  --arg package_format "$package_format" \
  --arg architecture "$architecture" \
  --arg machine "$expected_machine" \
  --arg package_manager "$package_manager" \
  --arg base_source_sha "$base_source_sha" \
  --arg candidate_source_sha "$candidate_source_sha" \
  --arg base_qualification_version "$base_qualification_version" \
  --arg candidate_qualification_version "$candidate_qualification_version" \
  --arg base_native_version "$base_native_version" \
  --arg candidate_native_version "$candidate_native_version" \
  --arg key_fingerprint "$key_fingerprint" \
  --arg service_identity "$stable_identity" \
  --arg repository_manifest_sha256 "$repository_manifest_sha256" \
  --arg bundle_file "$bundle_name" \
  --arg bundle_sha256 "$bundle_sha256" \
  --argjson metadata_signature_enforced "$metadata_signature_enforced" \
  --argjson unrelated_repositories_disabled "$unrelated_repositories_disabled" \
  '{
    version: "1",
    case_id: "I09-linux-repository-lifecycle",
    sources: {base: $base_source_sha, candidate: $candidate_source_sha},
    versions: {
      base_qualification: $base_qualification_version,
      candidate_qualification: $candidate_qualification_version,
      base_native: $base_native_version,
      candidate_native: $candidate_native_version
    },
    repository: {
      kind: $repository_kind,
      package_format: $package_format,
      architecture: $architecture,
      machine: $machine,
      native: true,
      package_manager: $package_manager,
      transport: "file://",
      metadata_signature_enforced: $metadata_signature_enforced,
      unrelated_repositories_disabled: $unrelated_repositories_disabled,
      key_fingerprint: $key_fingerprint,
      key_scope: "ephemeral-ci-qualification",
      manifest_sha256: $repository_manifest_sha256
    },
    lifecycle: {
      operator_configuration: "preserved",
      service_identity: $service_identity,
      service_state: "inactive-and-disabled",
      database_migration_performed: false
    },
    bundle: {file: $bundle_file, sha256: $bundle_sha256},
    hard_gates: {
      exact_candidate_head: true,
      frozen_base_source: true,
      qualification_versions_explicit: true,
      native_version_order: true,
      native_architecture: true,
      base_snapshot_isolated: true,
      base_package_retained: true,
      same_ephemeral_key: true,
      metadata_signatures_enforced: true,
      unrelated_repositories_disabled: true,
      base_install: true,
      base_revision_bound: true,
      base_service_dormant: true,
      operator_configuration_created: true,
      normal_candidate_upgrade: true,
      candidate_revision_bound: true,
      service_identity_stable: true,
      implicit_downgrade_rejected: true,
      explicit_rollback: true,
      rollback_revision_bound: true,
      rollback_state_preserved: true,
      candidate_reupgrade: true,
      final_removal_state_preserved: true,
      service_always_dormant: true,
      private_signing_key_absent: true,
      report_bundle_sha256_bound: true
    },
    qualification_boundaries: {
      release_tag: false,
      public_version_promise: false,
      stable_production_signing_key: false,
      public_hosted_repository: false,
      database_schema_migration: false,
      rpm_payload_signature: false,
      unattended_update_policy: false,
      long_duration_retention_sla: false
    }
  }' >"$evidence_directory/report.json"

jq -e '(.hard_gates | length == 26) and (.hard_gates | to_entries | all(.value == true))' \
  "$evidence_directory/report.json" >/dev/null || fail "normalized report does not contain 26 passing hard gates"
if grep -E -i '(postgresql://|api[_-]?key|password|private key|sk-[a-z0-9])' \
  "$evidence_directory/report.json"; then
  fail "normalized report contains credential material"
fi
if tar -tzf "$evidence_directory/$bundle_name" \
  | grep -E -i '(private-keys-v1\.d|secring\.gpg|\.key$|\.pem$)' >/dev/null; then
  fail "lifecycle bundle contains private signing key material"
fi

chmod 0644 "$evidence_directory/report.json" "$evidence_directory/$bundle_name"
chmod 0755 "$evidence_directory"
find /etc/vermory -depth -delete

echo "I09 repository lifecycle acceptance: pass ($repository_kind/$architecture)"
