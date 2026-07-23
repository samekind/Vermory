#!/bin/sh

set -eu
umask 022

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(git -C "$script_dir" rev-parse --show-toplevel)
out_dir=${1:-"$repo_root/dist"}
treeish=${VERMORY_HERMES_PACKAGE_TREEISH:-HEAD}
version=$(git -C "$repo_root" show "$treeish:integrations/hermes/pyproject.toml" \
  | sed -n 's/^version = "\([^"]*\)"$/\1/p' \
  | head -n 1)

[ -n "$version" ] || {
  printf '%s\n' "package-hermes: project version is missing" >&2
  exit 2
}

name="vermory-hermes-$version"
output="$out_dir/$name.tar.gz"
checksum_output="$output.sha256"
temporary=$(mktemp -d "${TMPDIR:-/tmp}/vermory-hermes-package.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

mkdir -p "$out_dir"
git -C "$repo_root" archive \
  --format=tar \
  --prefix="$name/" \
  "$treeish" \
  LICENSE \
  integrations/hermes/README.md \
  integrations/hermes/pyproject.toml \
  integrations/hermes/uv.lock \
  integrations/hermes/vermory \
  >"$temporary/package.tar"
gzip -n -9 <"$temporary/package.tar" >"$temporary/package.tar.gz"
mv "$temporary/package.tar.gz" "$output"
checksum=$(sha256_file "$output")
printf '%s  %s\n' "$checksum" "$(basename "$output")" >"$checksum_output.new"
mv "$checksum_output.new" "$checksum_output"

printf 'artifact=%s sha256=%s\n' \
  "$output" \
  "$checksum"
