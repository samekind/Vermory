#!/bin/sh

set -eu
umask 077

die() {
  printf '%s\n' "install-hermes: $*" >&2
  exit 2
}

commit=${VERMORY_HERMES_COMMIT:-0f102fa4dc04b7dfdab048169aaaa640d09d7523}
case "$commit" in
  *[!0-9a-f]*) die "VERMORY_HERMES_COMMIT must be lowercase hexadecimal" ;;
esac
[ "${#commit}" -eq 40 ] || die "VERMORY_HERMES_COMMIT must contain 40 hex characters"

installer_sha256=${VERMORY_HERMES_INSTALLER_SHA256:-c2e4326c1660bd45f64321996eb15bda35e7a4649e32a310495a61972a2804c8}
case "$installer_sha256" in
  *[!0-9a-f]*) die "VERMORY_HERMES_INSTALLER_SHA256 must be lowercase hexadecimal" ;;
esac
[ "${#installer_sha256}" -eq 64 ] || die "VERMORY_HERMES_INSTALLER_SHA256 must contain 64 hex characters"

root=${VERMORY_HERMES_ROOT:-"$HOME/.vermory/hermes"}
case "$root" in
  /*) ;;
  *) die "VERMORY_HERMES_ROOT must be absolute" ;;
esac
[ "$root" != "/" ] || die "VERMORY_HERMES_ROOT cannot be /"

install_dir="$root/hermes-agent"
installer="$root/setup-hermes-$commit.sh"
lock_dir="$root/install.lock"

mkdir -p "$root"
if ! mkdir "$lock_dir" 2>/dev/null; then
  die "another Hermes install is already running"
fi
cleanup() {
  rmdir "$lock_dir" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$root/cache/uv" "$root/cache/npm" "$root/python"
export XDG_CACHE_HOME="$root/cache"
export UV_CACHE_DIR="$root/cache/uv"
export UV_PYTHON_INSTALL_DIR="$root/python"
export npm_config_cache="$root/cache/npm"

if [ -n "${VERMORY_HERMES_PROXY:-}" ]; then
  export HTTP_PROXY="$VERMORY_HERMES_PROXY"
  export HTTPS_PROXY="$VERMORY_HERMES_PROXY"
  export ALL_PROXY="$VERMORY_HERMES_PROXY"
fi

curl_bin=${VERMORY_CURL_BIN:-/usr/bin/curl}
[ -x "$curl_bin" ] || die "curl is unavailable: $curl_bin"

installer_url="https://hermes-agent.nousresearch.com/install.sh"
"$curl_bin" --connect-timeout 15 --max-time 120 -fsSL "$installer_url" -o "$installer"
actual_installer_sha256=$(shasum -a 256 "$installer" | awk '{print $1}')
[ "$actual_installer_sha256" = "$installer_sha256" ] || die "Hermes installer checksum mismatch"
chmod 0700 "$installer"

export HERMES_HOME="$root"
export HERMES_INSTALL_DIR="$install_dir"
for stage in repository venv python-deps path config complete; do
  /bin/bash "$installer" \
    --stage "$stage" \
    --dir "$install_dir" \
    --hermes-home "$root" \
    --commit "$commit" \
    --skip-setup \
    --skip-browser \
    --no-skills \
    --non-interactive
done

[ -d "$install_dir/.git" ] || die "Hermes checkout was not created"
revision=$(git -C "$install_dir" rev-parse HEAD)
[ "$revision" = "$commit" ] || die "Hermes checkout revision mismatch: $revision"
[ -x "$install_dir/venv/bin/hermes" ] || die "Hermes entry point is unavailable"

printf '%s\n' "install-hermes: installed revision $revision"
printf '%s\n' "install-hermes: root $root"
