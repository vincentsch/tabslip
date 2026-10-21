#!/bin/sh
# Usage: curl -fsSL https://raw.githubusercontent.com/vincentsch/tabslip/main/install.sh | sh
set -eu

fail() { printf '%s\n' "Tabslip: $*" >&2; exit 1; }
work=
staged=
cleanup() {
    [ -z "$staged" ] || rm -f "$staged"
    [ -z "$work" ] || rm -rf "$work"
    return 0
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

os=$(uname -s)
case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW*|MSYS*|CYGWIN*) os=windows ;;
    *) fail "Unsupported operating system: $os" ;;
esac
arch=$(uname -m)
case "$arch" in
    x86_64|amd64|AMD64) arch=amd64 ;;
    aarch64|arm64|ARM64) arch=arm64 ;;
    *) fail "Unsupported architecture: $arch (need amd64 or arm64)" ;;
esac
asset=tabslip-$os-$arch
binary=tabslip
if [ "$os" = windows ]; then asset=$asset.exe; binary=$binary.exe; fi
base=${TABSLIP_DOWNLOAD_BASE:-https://github.com/vincentsch/tabslip}
if [ -n "${TABSLIP_VERSION:-}" ]; then
    url=${base%/}/releases/download/$TABSLIP_VERSION
else
    url=${base%/}/releases/latest/download
fi
command -v curl >/dev/null 2>&1 || fail 'curl is required to download the release'
work=$(mktemp -d) || fail 'Cannot create temporary download directory'
curl -fsSL "$url/$asset" -o "$work/$asset" || fail "Download failed for $asset"
curl -fsSL "$url/checksums.txt" -o "$work/checksums.txt" || fail 'Checksum manifest download failed'
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$work/checksums.txt")
[ -n "$expected" ] || fail "Checksum entry missing for $asset"
[ "${#expected}" -eq 64 ] || fail "Invalid checksum entry for $asset"
case "$expected" in *[!0-9a-fA-F]*) fail "Invalid checksum entry for $asset" ;; esac
if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum "$work/$asset") || fail 'SHA-256 calculation failed'
elif command -v shasum >/dev/null 2>&1; then
    digest=$(shasum -a 256 "$work/$asset") || fail 'SHA-256 calculation failed'
else
    fail 'Need sha256sum or shasum to verify the download'
fi
actual=${digest%% *}
[ "$actual" = "$expected" ] || fail "Checksum mismatch for $asset"

install_dir=${INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$install_dir" || fail 'Cannot create installation directory; choose a writable INSTALL_DIR'
# Stage beside the destination so replacement is a rename on the same filesystem.
staged=$(mktemp "$install_dir/.tabslip.XXXXXX") || fail 'Cannot write to installation directory'
cp "$work/$asset" "$staged" || fail 'Cannot stage executable'
chmod 755 "$staged" || fail 'Cannot make downloaded binary executable'
mv -f "$staged" "$install_dir/$binary" || fail 'Cannot replace installed executable (stop Tabslip if it is running)'
staged=
printf 'Installed %s.\nNext: tabslip demo\n' "$binary"
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Add the installation directory to PATH:\n  export PATH="%s:$PATH"\n' "$install_dir" ;;
esac
